import type { ErrorRequestHandler, NextFunction, Request, Response } from 'express';
import { NumericalOverflowError } from '../../../../domain/errors.js';
import type { Logger } from '../../../../platform/logger.js';
import { ProblemError, sendProblem, type ProblemSlug } from '../problem.js';
import { requestIdOf } from './request-id.middleware.js';

/** `body-parser` failures carry a stable `type` string. */
const BODY_PARSER_PROBLEMS: Readonly<Record<string, ProblemSlug>> = {
  'entity.parse.failed': 'malformed-json',
  'entity.verify.failed': 'malformed-json',
  'request.aborted': 'malformed-json',
  'entity.too.large': 'payload-too-large',
  'request.size.invalid': 'payload-too-large',
  'encoding.unsupported': 'unsupported-media-type',
  'charset.unsupported': 'unsupported-media-type',
};

const JWT_ERROR_NAMES = new Set(['JsonWebTokenError', 'TokenExpiredError', 'NotBeforeError']);

function errorType(error: unknown): string | undefined {
  if (typeof error !== 'object' || error === null) return undefined;
  const type: unknown = (error as Record<string, unknown>)['type'];
  return typeof type === 'string' ? type : undefined;
}

/** Terminal handler: an unrecognised failure is a bare 500, stack logged and never returned. */
export function errorMiddleware(logger: Logger, maxBodyBytes: number): ErrorRequestHandler {
  return (error: unknown, req: Request, res: Response, next: NextFunction): void => {
    if (res.headersSent) {
      next(error);
      return;
    }

    if (error instanceof ProblemError) {
      sendProblem(req, res, error.slug, error.options);
      return;
    }

    if (error instanceof NumericalOverflowError) {
      // The values are the caller's, so an overflow is a 422 validation failure, as in qr-api.
      sendProblem(req, res, 'validation-error', {
        detail: error.message,
        issues: [{ pointer: '/matrices', code: 'numerical_overflow', message: error.message }],
      });
      return;
    }

    const slug = BODY_PARSER_PROBLEMS[errorType(error) ?? ''];
    if (slug !== undefined) {
      const detail =
        slug === 'payload-too-large'
          ? `The request body exceeds the ${String(maxBodyBytes)} byte limit.`
          : undefined;
      sendProblem(req, res, slug, detail === undefined ? {} : { detail });
      return;
    }

    if (error instanceof Error && JWT_ERROR_NAMES.has(error.name)) {
      sendProblem(req, res, 'unauthorized', {
        headers: { 'WWW-Authenticate': 'Bearer realm="proyectot", error="invalid_token"' },
      });
      return;
    }

    logger.error({ err: error, requestId: requestIdOf(res) }, 'unhandled error while serving request');
    sendProblem(req, res, 'internal-error');
  };
}
