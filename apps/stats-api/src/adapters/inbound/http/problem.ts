import type { Request, Response } from 'express';
import { requestIdOf } from './middleware/request-id.middleware.js';

/** Registry of `contracts/problems.md` §2; titles are copied verbatim. */
export const PROBLEM_TYPE_PREFIX = 'urn:proyectot:problem:';

export type ProblemSlug =
  | 'malformed-json'
  | 'unauthorized'
  | 'not-found'
  | 'method-not-allowed'
  | 'payload-too-large'
  | 'request-header-fields-too-large'
  | 'unsupported-media-type'
  | 'validation-error'
  | 'too-many-requests'
  | 'internal-error';

interface ProblemDefinition {
  readonly status: number;
  readonly title: string;
  readonly detail: string;
}

export const PROBLEM_REGISTRY: Readonly<Record<ProblemSlug, ProblemDefinition>> = {
  'malformed-json': {
    status: 400,
    title: 'Malformed JSON body',
    detail: 'The request body is not valid JSON.',
  },
  unauthorized: {
    status: 401,
    title: 'Authentication required',
    detail: 'The access token is missing, expired or invalid.',
  },
  'not-found': {
    status: 404,
    title: 'Resource not found',
    detail: 'The requested resource does not exist.',
  },
  'method-not-allowed': {
    status: 405,
    title: 'Method not allowed',
    detail: 'This method is not supported on this resource.',
  },
  'payload-too-large': {
    status: 413,
    title: 'Payload too large',
    detail: 'The request body is larger than the configured limit.',
  },
  'request-header-fields-too-large': {
    status: 431,
    title: 'Request header fields too large',
    detail: "The request headers exceed the server's buffer.",
  },
  'unsupported-media-type': {
    status: 415,
    title: 'Unsupported media type',
    detail: 'Content-Type must be application/json.',
  },
  'validation-error': {
    status: 422,
    title: 'The request body failed validation',
    detail: 'The request body failed validation.',
  },
  'too-many-requests': {
    status: 429,
    title: 'Too many requests',
    detail: 'Rate limit exceeded. Retry after the window resets.',
  },
  'internal-error': {
    status: 500,
    title: 'Internal server error',
    detail: 'An unexpected error occurred. Quote the requestId when reporting it.',
  },
};

/** `errors[].code` — the shared enum of `contracts/problems.md` §3. */
export type ValidationCode =
  | 'missing_field'
  | 'invalid_type'
  | 'empty_matrix'
  | 'empty_row'
  | 'ragged_row'
  | 'non_finite_value'
  | 'too_many_rows'
  | 'too_many_cols'
  | 'too_many_matrices'
  | 'too_many_elements'
  | 'numerical_overflow';

export interface ValidationIssue {
  readonly pointer: string;
  readonly code: ValidationCode;
  readonly message: string;
}

export interface ProblemDocument {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail: string;
  readonly instance: string;
  readonly requestId: string;
  readonly timestamp: string;
  readonly errors?: readonly ValidationIssue[];
}

export interface ProblemOptions {
  readonly detail?: string;
  readonly issues?: readonly ValidationIssue[];
  readonly headers?: Readonly<Record<string, string>>;
}

/** A failure that already knows its problem document; the error middleware renders it. */
export class ProblemError extends Error {
  public readonly slug: ProblemSlug;
  public readonly options: ProblemOptions;

  public constructor(slug: ProblemSlug, options: ProblemOptions = {}) {
    super(options.detail ?? PROBLEM_REGISTRY[slug].detail);
    this.name = 'ProblemError';
    this.slug = slug;
    this.options = options;
  }
}

/** Request path without the query string — the `instance` member of the document. */
export function instanceOf(req: Request): string {
  const [path] = req.originalUrl.split('?');
  return path === undefined || path.length === 0 ? req.originalUrl : path;
}

export function buildProblem(
  req: Request,
  res: Response,
  slug: ProblemSlug,
  options: ProblemOptions = {},
): ProblemDocument {
  const definition = PROBLEM_REGISTRY[slug];
  const base = {
    type: `${PROBLEM_TYPE_PREFIX}${slug}`,
    title: definition.title,
    status: definition.status,
    detail: options.detail ?? definition.detail,
    instance: instanceOf(req),
    requestId: requestIdOf(res),
    timestamp: new Date().toISOString(),
  };
  return options.issues === undefined ? base : { ...base, errors: options.issues };
}

// Buffer, not string: `res.send` appends `; charset=utf-8`, and the contract fixes the media type
// at exactly `application/problem+json`.
export function sendProblem(
  req: Request,
  res: Response,
  slug: ProblemSlug,
  options: ProblemOptions = {},
): void {
  const document = buildProblem(req, res, slug, options);
  for (const [name, value] of Object.entries(options.headers ?? {})) {
    res.setHeader(name, value);
  }
  res.setHeader('Content-Type', 'application/problem+json');
  res.status(document.status).send(Buffer.from(JSON.stringify(document), 'utf8'));
}
