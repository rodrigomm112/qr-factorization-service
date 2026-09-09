import type { NextFunction, Request, RequestHandler, Response } from 'express';
import jwt from 'jsonwebtoken';
import { ProblemError } from '../problem.js';

export interface JwtVerifierOptions {
  readonly secret: string;
  readonly issuer: string;
  readonly audience: string;
}

const REALM = 'Bearer realm="proyectot"';

/** RFC 6750: no credentials advertises the scheme; a rejected credential is `invalid_token`. */
function unauthorized(tokenSupplied: boolean): ProblemError {
  return new ProblemError('unauthorized', {
    headers: {
      'WWW-Authenticate': tokenSupplied ? `${REALM}, error="invalid_token"` : REALM,
    },
  });
}

function bearerToken(req: Request): string | undefined {
  const header = req.headers.authorization;
  if (header === undefined) return undefined;
  const match = /^Bearer[ ]+(?<token>\S+)$/i.exec(header.trim());
  return match?.groups?.['token'];
}

// The algorithm allow-list stops `alg: none` and RS256 confusion; issuer and audience stop a token
// minted for another service from being replayed here.
export function jwtMiddleware(options: JwtVerifierOptions): RequestHandler {
  return (req: Request, _res: Response, next: NextFunction): void => {
    const supplied = req.headers.authorization !== undefined;
    const token = bearerToken(req);

    if (token === undefined) {
      next(unauthorized(supplied));
      return;
    }

    let payload: string | jwt.JwtPayload;
    try {
      payload = jwt.verify(token, options.secret, {
        algorithms: ['HS256'],
        issuer: options.issuer,
        audience: options.audience,
        clockTolerance: 30,
      });
    } catch {
      // One answer whichever check failed; the caller cannot tell them apart.
      next(unauthorized(true));
      return;
    }

    // jsonwebtoken only checks `exp` when the claim is present, so a token minted without one would
    // be accepted forever. qr-api requires it too (jwt.WithExpirationRequired).
    if (typeof payload === 'string' || typeof payload.exp !== 'number') {
      next(unauthorized(true));
      return;
    }

    next();
  };
}
