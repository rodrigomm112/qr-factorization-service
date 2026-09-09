import { randomUUID } from 'node:crypto';
import type { NextFunction, Request, RequestHandler, Response } from 'express';

export const REQUEST_ID_HEADER = 'X-Request-ID';

/** Any RFC 4122 shape is echoed; the service always generates version 4. */
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const NIL_UUID = '00000000-0000-0000-0000-000000000000';

/** Typed accessor over `res.locals`, which is `Record<string, any>`. */
export function requestIdOf(res: Response): string {
  const value: unknown = (res.locals as Record<string, unknown>)['requestId'];
  return typeof value === 'string' ? value : NIL_UUID;
}

function inboundRequestId(req: Request): string | undefined {
  const header = req.headers['x-request-id'];
  const candidate = Array.isArray(header) ? header[0] : header;
  return candidate !== undefined && UUID_PATTERN.test(candidate.trim())
    ? candidate.trim().toLowerCase()
    : undefined;
}

/** Echoes a valid inbound `X-Request-ID`, else mints v4. Runs first so every layer can quote it. */
export function requestIdMiddleware(): RequestHandler {
  return (req: Request, res: Response, next: NextFunction): void => {
    const requestId = inboundRequestId(req) ?? randomUUID();
    (res.locals as Record<string, unknown>)['requestId'] = requestId;
    res.setHeader(REQUEST_ID_HEADER, requestId);
    next();
  };
}
