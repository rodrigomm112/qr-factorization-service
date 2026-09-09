import { z } from 'zod';
import type { DiagonalTolerance } from '../domain/statistics.js';

export const LOG_LEVELS = ['fatal', 'error', 'warn', 'info', 'debug', 'trace', 'silent'] as const;
export type LogLevel = (typeof LOG_LEVELS)[number];

export type AppEnvironment = 'development' | 'production';

export interface AppConfig {
  readonly port: number;
  readonly appEnv: AppEnvironment;
  readonly logLevel: LogLevel;
  readonly jwt: {
    readonly secret: string;
    readonly issuer: string;
    readonly audience: string;
  };
  readonly corsAllowedOrigins: readonly string[];
  readonly maxBodyBytes: number;
  readonly rateLimit: {
    readonly max: number;
    readonly windowMs: number;
  };
  readonly diagonalTolerance: DiagonalTolerance;
  readonly limits: {
    readonly maxMatrices: number;
    readonly maxTotalElements: number;
    readonly maxMatrixRows: number;
    readonly maxMatrixCols: number;
  };
  readonly trustProxy: boolean;
  readonly shutdownTimeoutMs: number;
}

export class ConfigurationError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = 'ConfigurationError';
  }
}

function splitList(raw: string): string[] {
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

const port = z.coerce.number().int().min(1).max(65535);
const positiveInt = z.coerce.number().int().positive();
const nonNegative = z.coerce.number().nonnegative();

// Unknown keys are stripped: one `.env` is shared with qr-api, whose names must not be fatal here:
// `RATE_LIMIT_WINDOW=1m` (a Go duration) and the `STATS_*` names are ignored, not fatal.
// Everything has a default except `JWT_SECRET`.
const environmentSchema = z.object({
  PORT: port.default(3000),
  APP_ENV: z.enum(['development', 'production']).default('development'),
  LOG_LEVEL: z.enum(LOG_LEVELS).default('info'),

  JWT_SECRET: z
    .string({ error: 'is required (HS256 signing key, at least 32 characters)' })
    .min(32, 'must be at least 32 characters'),
  JWT_ISSUER: z.string().min(1).default('qr-api'),
  JWT_AUDIENCE: z.string().min(1).default('stats-api'),

  CORS_ALLOWED_ORIGINS: z
    .string()
    .default('')
    .refine(
      (raw) => !splitList(raw).includes('*'),
      'must list explicit origins; the "*" wildcard is not accepted',
    ),

  MAX_BODY_BYTES: positiveInt.default(4_194_304),
  RATE_LIMIT_MAX: positiveInt.default(120),
  RATE_LIMIT_WINDOW_MS: positiveInt.default(60_000),

  DIAGONAL_ABS_TOLERANCE: nonNegative.default(1e-12),
  DIAGONAL_REL_TOLERANCE: nonNegative.default(1e-9),

  MAX_MATRICES: positiveInt.default(8),
  MAX_TOTAL_ELEMENTS: positiveInt.default(100_000),
  MAX_MATRIX_ROWS: positiveInt.default(100),
  MAX_MATRIX_COLS: positiveInt.default(100),

  TRUST_PROXY: z.stringbool().default(false),
  SHUTDOWN_TIMEOUT_MS: z.coerce.number().int().nonnegative().default(10_000),
});

// `LOG_LEVEL=` in a `.env` means "not set": dropping blanks lets defaults apply, not fail coercion.
function withoutBlanks(env: Record<string, string | undefined>): Record<string, string> {
  const cleaned: Record<string, string> = {};
  for (const [key, value] of Object.entries(env)) {
    if (typeof value === 'string' && value.trim().length > 0) {
      cleaned[key] = value;
    }
  }
  return cleaned;
}

/** Throws {@link ConfigurationError} with a readable summary; `main.ts` exits on it. */
export function loadConfig(env: Record<string, string | undefined> = process.env): AppConfig {
  const parsed = environmentSchema.safeParse(withoutBlanks(env));

  if (!parsed.success) {
    const details = parsed.error.issues
      .map((issue) => `  - ${issue.path.join('.') || '(environment)'}: ${issue.message}`)
      .join('\n');
    throw new ConfigurationError(`Invalid environment configuration:\n${details}`);
  }

  const values = parsed.data;

  return {
    port: values.PORT,
    appEnv: values.APP_ENV,
    logLevel: values.LOG_LEVEL,
    jwt: {
      secret: values.JWT_SECRET,
      issuer: values.JWT_ISSUER,
      audience: values.JWT_AUDIENCE,
    },
    corsAllowedOrigins: splitList(values.CORS_ALLOWED_ORIGINS),
    maxBodyBytes: values.MAX_BODY_BYTES,
    rateLimit: {
      max: values.RATE_LIMIT_MAX,
      windowMs: values.RATE_LIMIT_WINDOW_MS,
    },
    diagonalTolerance: {
      absTol: values.DIAGONAL_ABS_TOLERANCE,
      relTol: values.DIAGONAL_REL_TOLERANCE,
    },
    limits: {
      maxMatrices: values.MAX_MATRICES,
      maxTotalElements: values.MAX_TOTAL_ELEMENTS,
      maxMatrixRows: values.MAX_MATRIX_ROWS,
      maxMatrixCols: values.MAX_MATRIX_COLS,
    },
    trustProxy: values.TRUST_PROXY,
    shutdownTimeoutMs: values.SHUTDOWN_TIMEOUT_MS,
  };
}
