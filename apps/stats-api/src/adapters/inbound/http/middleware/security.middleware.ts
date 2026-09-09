import cors from 'cors';
import type { CorsOptions } from 'cors';
import type { NextFunction, Request, RequestHandler, Response } from 'express';
import { ipKeyGenerator, rateLimit } from 'express-rate-limit';
import type { AugmentedRequest, RateLimitInfo } from 'express-rate-limit';
import helmet from 'helmet';
import { REQUEST_ID_HEADER } from './request-id.middleware.js';
import { ProblemError, sendProblem } from '../problem.js';

const STRICT_CSP = {
  'default-src': ["'none'"],
  'base-uri': ["'none'"],
  'form-action': ["'none'"],
  'frame-ancestors': ["'none'"],
  'object-src': ["'none'"],
} as const;

// Swagger UI sets element styles at runtime, so `style-src` needs `'unsafe-inline'`; its scripts
// are external files served from this origin, so `script-src` stays clean.
const DOCS_CSP = {
  'default-src': ["'self'"],
  'base-uri': ["'self'"],
  'script-src': ["'self'"],
  'style-src': ["'self'", "'unsafe-inline'"],
  'img-src': ["'self'", 'data:'],
  'font-src': ["'self'", 'data:'],
  'connect-src': ["'self'"],
  'worker-src': ["'self'", 'blob:'],
  'form-action': ["'self'"],
  'frame-ancestors': ["'none'"],
  'object-src': ["'none'"],
} as const;

function helmetFor(directives: Record<string, readonly string[]>, production: boolean): RequestHandler {
  return helmet({
    contentSecurityPolicy: { useDefaults: false, directives },
    crossOriginOpenerPolicy: { policy: 'same-origin' },
    crossOriginResourcePolicy: { policy: 'same-origin' },
    referrerPolicy: { policy: 'no-referrer' },
    frameguard: { action: 'deny' },
    hsts: production ? { maxAge: 31_536_000, includeSubDomains: true, preload: true } : false,
  });
}

/** `/docs` gets the relaxed CSP; every other path gets `default-src 'none'` — the API is JSON. */
export function securityHeadersMiddleware(production: boolean): RequestHandler {
  const strict = helmetFor({ ...STRICT_CSP }, production);
  const docs = helmetFor({ ...DOCS_CSP }, production);
  return (req: Request, res: Response, next: NextFunction): void => {
    const handler = req.path === '/docs' || req.path.startsWith('/docs/') ? docs : strict;
    handler(req, res, next);
  };
}

// A disallowed origin gets no CORS headers rather than a 403: a 403 would leak the allow-list and
// break non-browser clients, which send no `Origin`.
export function corsMiddleware(allowedOrigins: readonly string[]): RequestHandler {
  const options: CorsOptions = {
    origin: (origin, callback) => {
      callback(null, origin !== undefined && allowedOrigins.includes(origin));
    },
    credentials: false,
    methods: ['GET', 'HEAD', 'POST', 'OPTIONS'],
    allowedHeaders: ['Content-Type', 'Authorization', REQUEST_ID_HEADER],
    exposedHeaders: [REQUEST_ID_HEADER, 'RateLimit', 'RateLimit-Policy', 'Retry-After'],
    maxAge: 600,
  };
  const handler = cors(options);
  return (req, res, next) => {
    // A preflight from a disallowed origin is answered here — empty 204, no CORS headers, qr-api's
    // shape — before the limiter (which would spend quota) and the router (which would answer 405).
    const origin = req.headers.origin;
    if (req.method === 'OPTIONS' && origin !== undefined && !allowedOrigins.includes(origin)) {
      res.status(204).end();
      return;
    }
    handler(req, res, next);
  };
}

export interface RateLimitSettings {
  readonly max: number;
  readonly windowMs: number;
}

const RATE_LIMIT_PROPERTY = 'rateLimit';

/** Policy name in the draft-8 headers; qr-api uses the same one, so clients parse one dialect. */
const RATE_LIMIT_POLICY = 'default';

/** Seconds until the window resets, floored at 0 (draft-8 `t`). */
function resetSeconds(resetTime: Date | undefined, windowMs: number): number {
  if (resetTime === undefined) return Math.ceil(windowMs / 1000);
  return Math.max(0, Math.ceil((resetTime.getTime() - Date.now()) / 1000));
}

// Hand-written: express-rate-limit 8.7 always appends `pk=:<sha-256 of the client key>:` — a hashed
// IP absent from qr-api's headers — so its emission is off (`standardHeaders: false`) and both
// headers use qr-api's spelling: `"default";q=<max>;w=<window>` and `"default";r=<left>;t=<reset>`.
function setRateLimitHeaders(req: Request, res: Response, settings: RateLimitSettings): void {
  if (res.headersSent) return;
  // Optional because a header writer must never throw; the limiter always fills it in.
  const info: RateLimitInfo | undefined = (req as AugmentedRequest)[RATE_LIMIT_PROPERTY];
  if (info === undefined) return;
  const window = Math.ceil(settings.windowMs / 1000);
  const reset = resetSeconds(info.resetTime, settings.windowMs);
  res.setHeader(
    'RateLimit-Policy',
    `"${RATE_LIMIT_POLICY}";q=${String(settings.max)};w=${String(window)}`,
  );
  res.setHeader(
    'RateLimit',
    `"${RATE_LIMIT_POLICY}";r=${String(info.remaining)};t=${String(reset)}`,
  );
}

// Per-IP fixed window on the API surface only: health and docs are excluded so a probe storm
// cannot lock a caller out.
export function rateLimitMiddleware(settings: RateLimitSettings): RequestHandler {
  const limiter = rateLimit({
    windowMs: settings.windowMs,
    limit: settings.max,
    standardHeaders: false,
    legacyHeaders: false,
    requestPropertyName: RATE_LIMIT_PROPERTY,
    // IPv6 addresses must be bucketed by /56 subnet, not by exact address.
    keyGenerator: (req: Request) => ipKeyGenerator(req.ip ?? ''),
    handler: (req: Request, res: Response, _next: NextFunction, options) => {
      const info: RateLimitInfo | undefined = (req as AugmentedRequest)[
        options.requestPropertyName
      ];
      // `Retry-After: 0` would invite an immediate retry, so it starts at 1.
      const seconds = Math.max(1, resetSeconds(info?.resetTime, options.windowMs));
      setRateLimitHeaders(req, res, settings);
      sendProblem(req, res, 'too-many-requests', { headers: { 'Retry-After': String(seconds) } });
    },
  });

  return (req: Request, res: Response, next: NextFunction): void => {
    limiter(req, res, (error?: unknown) => {
      setRateLimitHeaders(req, res, settings);
      next(error);
    });
  };
}

const JSON_MEDIA_TYPE = 'application/json';

// Single definition of "carries a payload", shared with the statistics route: the media-type check
// skips such a request and the route answers `malformed-json`.
export function hasBody(req: Request): boolean {
  if (req.headers['transfer-encoding'] !== undefined) return true;
  const length = req.headers['content-length'];
  return length !== undefined && length !== '0';
}

/** Rejects a non-JSON body before `express.json` ignores it and it fails as `missing_field`. */
export function contentTypeMiddleware(): RequestHandler {
  return (req: Request, _res: Response, next: NextFunction): void => {
    if (!hasBody(req)) {
      next();
      return;
    }
    const contentType = req.headers['content-type'];
    const mediaType = contentType?.split(';')[0]?.trim().toLowerCase();
    if (mediaType !== JSON_MEDIA_TYPE) {
      next(
        new ProblemError('unsupported-media-type', {
          detail: `Content-Type must be ${JSON_MEDIA_TYPE}.`,
        }),
      );
      return;
    }
    next();
  };
}
