// Hand-rolled `Response` stand-in for `fetch` mocks: the suite behaves the same whatever
// globals the jsdom environment happens to expose.
export interface StubResponseInit {
  readonly status?: number;
  readonly contentType?: string;
  readonly requestId?: string;
}

export function jsonResponse(body: unknown, init: StubResponseInit = {}): Response {
  const status = init.status ?? 200;
  const text = typeof body === 'string' ? body : JSON.stringify(body);
  const headers = new Map<string, string>([
    ['content-type', init.contentType ?? 'application/json'],
  ]);
  if (init.requestId !== undefined) headers.set('x-request-id', init.requestId);

  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (name: string) => headers.get(name.toLowerCase()) ?? null },
    text: () => Promise.resolve(text),
  } as unknown as Response;
}

/** The `RequestInit` of one call, narrowed for assertions. */
export const initOf = (call: readonly unknown[] | undefined): RequestInit =>
  (call?.[1] as RequestInit | undefined) ?? {};

export const headerOf = (init: RequestInit, name: string): string | null =>
  init.headers instanceof Headers ? init.headers.get(name) : null;
