// The calls this SPA makes, typed against the contracts. Components never build URLs.
import { apiFetch } from './client';
import type {
  HealthResponse,
  Matrix,
  QrMode,
  QrRequest,
  QrResponse,
  StatisticsRequest,
  StatisticsResponse,
  TokenResponse,
} from './types';

/** Anonymous, rate-limited (10/min/IP), disabled with `DEMO_TOKEN_ENABLED=false` (404). */
export const DEMO_TOKEN_PATH = '/api/v1/auth/demo-token';

/** Confidential clients only; the SPA never sends a secret, see `TechDetails`. */
export const CLIENT_CREDENTIALS_PATH = '/api/v1/auth/token';

/**
 * Same `TokenResponse` as the client-credentials grant, with `sub = "demo"`. No body and no
 * credentials: a public SPA cannot hold a client secret, so the evaluator gets a session for
 * free and the confidential-client path stays documented in curl and Swagger.
 */
export const issueDemoToken = (baseUrl: string, signal?: AbortSignal): Promise<TokenResponse> =>
  apiFetch<TokenResponse>({
    baseUrl,
    path: DEMO_TOKEN_PATH,
    method: 'POST',
    ...(signal === undefined ? {} : { signal }),
  });

export const computeQr = (
  baseUrl: string,
  token: string,
  matrix: Matrix,
  mode: QrMode,
  signal?: AbortSignal,
): Promise<QrResponse> =>
  apiFetch<QrResponse>({
    baseUrl,
    path: '/api/v1/qr',
    token,
    body: { matrix, mode } satisfies QrRequest,
    ...(signal === undefined ? {} : { signal }),
  });

export const computeStatistics = (
  baseUrl: string,
  token: string,
  request: StatisticsRequest,
  signal?: AbortSignal,
): Promise<StatisticsResponse> =>
  apiFetch<StatisticsResponse>({
    baseUrl,
    path: '/api/v1/statistics',
    token,
    body: request,
    ...(signal === undefined ? {} : { signal }),
  });

/** Public, outside the rate limiter, `application/health+json`. */
export const fetchReadiness = (
  baseUrl: string,
  signal?: AbortSignal,
  timeoutMs = 5_000,
): Promise<HealthResponse> =>
  apiFetch<HealthResponse>({
    baseUrl,
    path: '/health/ready',
    method: 'GET',
    timeoutMs,
    correlate: false,
    ...(signal === undefined ? {} : { signal }),
  });
