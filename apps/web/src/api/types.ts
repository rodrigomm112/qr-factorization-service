// Contract types for both APIs, hand-derived from the two openapi.yaml files and
// the shared error contract (see contracts/examples): no codegen step, and since the fixtures in src/test/fixtures.ts
// are typed with these declarations, a drift between the examples and this file is a build error.

/** Rows of equal length. */
export type Matrix = readonly (readonly number[])[];

/** `[rows, cols]`. */
export type Shape = readonly [number, number];

/* ---- Auth (qr-api) ---- */

export interface TokenRequest {
  readonly clientId: string;
  readonly clientSecret: string;
}

export interface TokenResponse {
  readonly accessToken: string;
  readonly tokenType: 'Bearer';
  /** Lifetime in seconds. */
  readonly expiresIn: number;
  readonly issuedAt: string;
  readonly scope: string;
}

/* ---- QR (qr-api) ---- */

export type QrMode = 'full' | 'reduced';

export interface QrRequest {
  readonly matrix: Matrix;
  readonly mode?: QrMode;
}

export interface MatrixShape {
  readonly rows: number;
  readonly cols: number;
}

export interface QrMeta {
  readonly algorithm: string;
  readonly mode: QrMode;
  readonly qShape: Shape;
  readonly rShape: Shape;
  readonly decompositionMs: number;
  readonly statisticsMs: number;
}

export interface QrResponse {
  readonly requestId: string;
  readonly input: MatrixShape;
  readonly q: Matrix;
  readonly r: Matrix;
  /** Body of stats-api for `[Q, R]`, minus its own `requestId`. */
  readonly statistics: StatisticsReport;
  readonly meta: QrMeta;
}

/* ---- Statistics (stats-api) ---- */

export interface LabeledMatrix {
  readonly label?: string;
  readonly values: Matrix;
}

export interface StatisticsRequest {
  /** Canonical labelled form; the bare `number[][]` shorthand is also accepted. */
  readonly matrices: readonly LabeledMatrix[];
}

/** Aggregated over every matrix of the request. */
export interface StatisticsSummary {
  readonly max: number;
  readonly min: number;
  readonly sum: number;
  readonly average: number;
  readonly count: number;
  readonly anyDiagonal: boolean;
}

export interface MatrixStatistics {
  readonly index: number;
  readonly label: string;
  readonly rows: number;
  readonly cols: number;
  readonly max: number;
  readonly min: number;
  readonly sum: number;
  readonly average: number;
  readonly isDiagonal: boolean;
}

export interface StatisticsMeta {
  readonly summationAlgorithm: string;
  readonly diagonalTolerance: {
    readonly absolute: number;
    readonly relative: number;
  };
  readonly elapsedMs: number;
}

export interface StatisticsReport {
  readonly summary: StatisticsSummary;
  readonly matrices: readonly MatrixStatistics[];
  readonly meta: StatisticsMeta;
}

export interface StatisticsResponse extends StatisticsReport {
  readonly requestId: string;
}

/* ---- Errors, RFC 9457 (the shared error contract (see contracts/examples)) ---- */

export const PROBLEM_URN_PREFIX = 'urn:proyectot:problem:';

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
  | 'internal-error'
  | 'downstream-unavailable'
  | 'downstream-timeout';

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
  | 'invalid_mode'
  | 'numerical_overflow';

export interface ValidationIssue {
  /** JSON Pointer into the request body, e.g. `/matrix/1/2`. */
  readonly pointer: string;
  /** One of {@link ValidationCode}, typed loosely so a new server code still parses. */
  readonly code: string;
  readonly message: string;
}

export interface ProblemDetails {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail?: string;
  readonly instance: string;
  readonly requestId: string;
  readonly timestamp: string;
  /** Present only on `validation-error` (422). */
  readonly errors?: readonly ValidationIssue[];
}

/* ---- Health (both APIs, `application/health+json`) ---- */

export type HealthStatus = 'pass' | 'warn' | 'fail';

export interface HealthCheck {
  readonly status: HealthStatus;
  readonly componentType?: string;
  readonly observedValue?: number;
  readonly observedUnit?: string;
  readonly time?: string;
  readonly output?: string;
}

export interface HealthResponse {
  readonly status: HealthStatus;
  readonly service: string;
  readonly version: string;
  readonly uptimeSeconds: number;
  readonly checks?: Readonly<Record<string, HealthCheck>>;
}
