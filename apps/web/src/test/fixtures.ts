// Golden fixtures read at test time from contracts/examples, so those files are the single
// source of truth and a rename or a shape change there fails every test that imports one.
// Imported ONLY from *.test.ts(x): `node:fs` never reaches the bundle, and the apps/web Docker
// context (which has no docs/) only ever runs `tsc --noEmit && vite build`.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { ProblemDetails, QrResponse, StatisticsResponse, TokenResponse } from '../api/types';

/** `apps/web/src/test` -> repository root -> the shared contract examples. */
const EXAMPLES_DIR = join(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../contracts/examples',
);

/** A renamed or dropped member throws here, not as an assertion about `undefined` far away. */
function readExample(file: string, requiredKeys: readonly string[]): unknown {
  const path = join(EXAMPLES_DIR, file);
  const parsed: unknown = JSON.parse(readFileSync(path, 'utf8'));
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    throw new Error(`${file}: expected a JSON object`);
  }
  const missing = requiredKeys.filter((key) => !Object.hasOwn(parsed, key));
  if (missing.length > 0) {
    throw new Error(`${file} drifted from src/api/types.ts: missing ${missing.join(', ')}`);
  }
  return parsed;
}

const PROBLEM_KEYS = ['type', 'title', 'status', 'instance', 'requestId', 'timestamp'] as const;
const STATISTICS_KEYS = ['requestId', 'summary', 'matrices', 'meta'] as const;

export const QR_RESPONSE_IDENTITY_3X3 = readExample('qr.response.identity-3x3.json', [
  'requestId',
  'input',
  'q',
  'r',
  'statistics',
  'meta',
]) as QrResponse;

export const QR_RESPONSE_TALL_3X2 = readExample('qr.response.tall-3x2.json', [
  'requestId',
  'input',
  'q',
  'r',
  'statistics',
  'meta',
]) as QrResponse;

export const STATISTICS_RESPONSE_MIXED_2X2 = readExample(
  'statistics.response.mixed-2x2.json',
  STATISTICS_KEYS,
) as StatisticsResponse;

export const STATISTICS_RESPONSE_IDENTITY_3X3 = readExample(
  'statistics.response.identity-3x3.json',
  STATISTICS_KEYS,
) as StatisticsResponse;

export const PROBLEM_VALIDATION_ERROR = readExample('problem.validation-error.json', [
  ...PROBLEM_KEYS,
  'errors',
]) as ProblemDetails;

export const PROBLEM_UNAUTHORIZED = readExample(
  'problem.unauthorized.json',
  PROBLEM_KEYS,
) as ProblemDetails;

export const PROBLEM_DOWNSTREAM_UNAVAILABLE = readExample(
  'problem.downstream-unavailable.json',
  PROBLEM_KEYS,
) as ProblemDetails;

/**
 * Hand-written: the auth endpoints have no shared example file. The `accessToken` is a real
 * base64url JWT shape whose payload carries `sub: "demo"`, so `readSubject` has something to
 * decode; the signature is a placeholder — nothing in the browser verifies it.
 */
export const DEMO_TOKEN_RESPONSE: TokenResponse = {
  accessToken:
    'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJxci1hcGkiLCJzdWIiOiJkZW1vIiwic2NvcGUiOiJxcjpjb21wdXRlIHN0YXRzOmNvbXB1dGUiLCJleHAiOjE3ODg4ODg4ODh9.c2lnbmF0dXJl',
  tokenType: 'Bearer',
  expiresIn: 3600,
  issuedAt: '2026-09-08T18:30:00Z',
  scope: 'qr:compute stats:compute',
};

/** `DEMO_TOKEN_ENABLED=false`: the endpoint is not registered, so the router 404s. */
export const PROBLEM_DEMO_TOKEN_DISABLED: ProblemDetails = {
  type: 'urn:proyectot:problem:not-found',
  title: 'Resource not found',
  status: 404,
  detail: 'POST /api/v1/auth/demo-token is not enabled on this instance',
  instance: '/api/v1/auth/demo-token',
  requestId: '00000000-0000-4000-8000-0000000000d4',
  timestamp: '2026-09-08T18:30:00Z',
};
