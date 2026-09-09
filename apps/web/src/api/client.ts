// Typed fetch wrapper for both APIs: JSON in/out, bearer token, deadline, problem+json ->
// ApiError, and an `X-Request-ID` the server echoes so a browser failure maps to a server log.
import { toApiError, ApiError } from './problem';

export type HttpMethod = 'GET' | 'POST';

export interface ApiCall<TBody = unknown> {
  /** API origin, no trailing slash (see `src/config.ts`). */
  readonly baseUrl: string;
  readonly path: string;
  readonly method?: HttpMethod;
  readonly body?: TBody;
  readonly token?: string | null;
  /** Deadline in milliseconds. */
  readonly timeoutMs?: number;
  /** Caller cancellation (component unmount, superseded request). */
  readonly signal?: AbortSignal;
  /** Default `true`. Off for health probes: no custom header, so no preflight every 15 s. */
  readonly correlate?: boolean;
}

export const DEFAULT_TIMEOUT_MS = 15_000;

/** UUID v4 for `X-Request-ID`; `randomUUID` needs a secure context. */
export function newRequestId(): string {
  const cryptoApi = globalThis.crypto as Crypto | undefined;
  if (typeof cryptoApi?.randomUUID === 'function') return cryptoApi.randomUUID();
  // Non-secure origins (plain http on a LAN address) have no `randomUUID`.
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (char) => {
    const random = Math.floor(Math.random() * 16);
    const value = char === 'x' ? random : (random & 0x3) | 0x8;
    return value.toString(16);
  });
}

const JSON_CONTENT = /^application\/(problem\+|health\+)?json\b/i;

async function readBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (text === '') return undefined;
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType !== '' && !JSON_CONTENT.test(contentType)) return text;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

/** Resolves with the parsed body; every failure path rejects with an `ApiError`. */
export async function apiFetch<TResult>(call: ApiCall): Promise<TResult> {
  const {
    baseUrl,
    path,
    method = call.body === undefined ? 'GET' : 'POST',
    body,
    token,
    timeoutMs = DEFAULT_TIMEOUT_MS,
    signal,
    correlate = true,
  } = call;

  const requestId = newRequestId();
  const headers = new Headers({ Accept: 'application/json' });
  if (correlate) headers.set('X-Request-ID', requestId);
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  if (token !== undefined && token !== null && token !== '') {
    headers.set('Authorization', `Bearer ${token}`);
  }

  const controller = new AbortController();
  // A holder, not two `let`s: only callbacks write these, which control-flow analysis cannot see.
  const abort = { timedOut: false, cancelled: false };
  const timer = setTimeout(() => {
    abort.timedOut = true;
    controller.abort();
  }, timeoutMs);
  const forwardAbort = (): void => {
    abort.cancelled = true;
    controller.abort();
  };
  if (signal !== undefined) {
    if (signal.aborted) forwardAbort();
    else signal.addEventListener('abort', forwardAbort, { once: true });
  }

  let response: Response;
  try {
    response = await fetch(`${baseUrl}${path}`, {
      method,
      headers,
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      signal: controller.signal,
      mode: 'cors',
      credentials: 'omit',
      cache: 'no-store',
    });
  } catch (cause) {
    if (abort.timedOut) {
      throw new ApiError({
        kind: 'timeout',
        message: `La API no respondió en ${Math.round(timeoutMs / 1000)} s.`,
        requestId,
        cause,
      });
    }
    if (abort.cancelled) {
      throw new ApiError({ kind: 'aborted', message: 'Solicitud cancelada.', requestId, cause });
    }
    throw new ApiError({
      kind: 'network',
      message: `No se pudo contactar con ${baseUrl}. Comprueba que la API está levantada y que permite este origen (CORS).`,
      requestId,
      cause,
    });
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener('abort', forwardAbort);
  }

  const payload = await readBody(response);
  if (!response.ok) {
    throw toApiError(response.status, payload, response.headers.get('X-Request-ID') ?? requestId);
  }
  return payload as TResult;
}
