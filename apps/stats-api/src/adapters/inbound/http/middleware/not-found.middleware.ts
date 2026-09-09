import type { NextFunction, Request, RequestHandler, Response } from 'express';
import { ProblemError } from '../problem.js';

/** Express 5 removed the `'*'` pattern, so the 404 is a plain `app.use` after every route. */
export function notFoundMiddleware(): RequestHandler {
  return (req: Request, _res: Response, next: NextFunction): void => {
    next(
      new ProblemError('not-found', {
        detail: `No resource matches ${req.method} on this path.`,
      }),
    );
  };
}

/** Registered after the real verbs of a path so a wrong method is a 405, not a 404. */
export function methodNotAllowed(allowed: readonly string[]): RequestHandler {
  const allowHeader = allowed.join(', ');
  return (req: Request, _res: Response, next: NextFunction): void => {
    next(
      new ProblemError('method-not-allowed', {
        detail: `${req.method} is not supported on this resource; allowed: ${allowHeader}.`,
        headers: { Allow: allowHeader },
      }),
    );
  };
}
