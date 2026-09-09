// RFC 9457 problem documents -> `ApiError`, so a network failure, a CORS rejection and a 422
// from either API all reach the components in the same shape.
import { PROBLEM_URN_PREFIX } from './types';
import type { ProblemDetails, ProblemSlug, ValidationIssue } from './types';

/** Where an `ApiError` came from. `http` always carries a status. */
export type ApiErrorKind = 'http' | 'network' | 'timeout' | 'aborted';

export interface ApiErrorInit {
  readonly kind: ApiErrorKind;
  readonly message: string;
  readonly status?: number;
  readonly problem?: ProblemDetails;
  readonly requestId?: string;
  readonly cause?: unknown;
}

export class ApiError extends Error {
  readonly kind: ApiErrorKind;
  readonly status: number | undefined;
  readonly problem: ProblemDetails | undefined;
  readonly requestId: string | undefined;

  constructor(init: ApiErrorInit) {
    super(init.message, init.cause === undefined ? undefined : { cause: init.cause });
    this.name = 'ApiError';
    this.kind = init.kind;
    this.status = init.status;
    this.problem = init.problem;
    // The contract guarantees header == body; the document wins when both exist.
    this.requestId = init.problem?.requestId ?? init.requestId;
  }

  /** Empty unless the server answered `422 validation-error`. */
  get issues(): readonly ValidationIssue[] {
    return this.problem?.errors ?? [];
  }
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

/** Only the members both services always send are required, so an extension still parses. */
export function isProblemDetails(value: unknown): value is ProblemDetails {
  if (!isRecord(value)) return false;
  return (
    typeof value.type === 'string' &&
    typeof value.title === 'string' &&
    typeof value.status === 'number' &&
    typeof value.instance === 'string' &&
    typeof value.requestId === 'string' &&
    typeof value.timestamp === 'string'
  );
}

/** The one failure that must end the session. */
export const isUnauthorized = (error: unknown): boolean =>
  error instanceof ApiError && error.status === 401;

/** `DEMO_TOKEN_ENABLED=false` answers 404: the feature is off, not broken. */
export const isNotFound = (error: unknown): boolean =>
  error instanceof ApiError && error.status === 404;

/** `urn:proyectot:problem:validation-error` -> `validation-error`. */
export function problemSlug(problem: ProblemDetails): string {
  return problem.type.startsWith(PROBLEM_URN_PREFIX)
    ? problem.type.slice(PROBLEM_URN_PREFIX.length)
    : problem.type;
}

/** Clients branch on `type`, never on `title`. */
const MESSAGES: Record<ProblemSlug, string> = {
  'malformed-json': 'El cuerpo enviado no es JSON válido.',
  unauthorized: 'Credenciales inválidas o sesión expirada.',
  'not-found': 'El recurso solicitado no existe en la API.',
  'method-not-allowed': 'Método HTTP no permitido para esa ruta.',
  'payload-too-large': 'La matriz enviada supera el tamaño máximo admitido.',
  'request-header-fields-too-large': 'Las cabeceras de la solicitud son demasiado grandes.',
  'unsupported-media-type': 'Tipo de contenido no admitido: se esperaba application/json.',
  'validation-error': 'La solicitud no superó la validación del servidor.',
  'too-many-requests': 'Demasiadas solicitudes. Espera unos segundos y reinténtalo.',
  'internal-error': 'Error interno del servidor. Cita el requestId al reportarlo.',
  'downstream-unavailable': 'El servicio de estadísticas no está disponible.',
  'downstream-timeout': 'El servicio de estadísticas tardó demasiado en responder.',
};

function messageForStatus(status: number): string {
  if (status === 401 || status === 403) return MESSAGES.unauthorized;
  if (status >= 500) return 'La API respondió con un error inesperado.';
  if (status >= 400) return 'La API rechazó la solicitud.';
  return 'Respuesta inesperada de la API.';
}

export function problemMessage(problem: ProblemDetails): string {
  const slug = problemSlug(problem);
  // `hasOwn`, not `in`: a slug like "constructor" would hit `Object.prototype` and return a fn.
  return Object.hasOwn(MESSAGES, slug)
    ? MESSAGES[slug as ProblemSlug]
    : (problem.title || messageForStatus(problem.status));
}

/** Anything that is not a problem document (a proxy's HTML page, an empty body) uses the status. */
export function toApiError(
  status: number,
  body: unknown,
  requestId: string | undefined,
): ApiError {
  if (isProblemDetails(body)) {
    return new ApiError({
      kind: 'http',
      message: problemMessage(body),
      status,
      problem: body,
      ...(requestId === undefined ? {} : { requestId }),
    });
  }
  return new ApiError({
    kind: 'http',
    message: messageForStatus(status),
    status,
    ...(requestId === undefined ? {} : { requestId }),
  });
}

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  if (error instanceof Error && error.message !== '') return error.message;
  return 'Error desconocido.';
}
