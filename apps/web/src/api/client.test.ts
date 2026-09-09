import { describe, expect, it, vi } from 'vitest';
import { PROBLEM_VALIDATION_ERROR, QR_RESPONSE_IDENTITY_3X3 } from '../test/fixtures';
import { headerOf, initOf, jsonResponse } from '../test/http';
import { apiFetch } from './client';
import { ApiError } from './problem';

describe('apiFetch', () => {
  it('sends JSON with the bearer token and a fresh X-Request-ID', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(QR_RESPONSE_IDENTITY_3X3));
    vi.stubGlobal('fetch', fetchMock);

    const result = await apiFetch({
      baseUrl: 'http://localhost:8080',
      path: '/api/v1/qr',
      token: 'jwt-token',
      body: { matrix: [[1]], mode: 'full' },
    });

    expect(result).toEqual(QR_RESPONSE_IDENTITY_3X3);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url] = fetchMock.mock.calls[0] as [string];
    const init = initOf(fetchMock.mock.calls[0]);
    expect(url).toBe('http://localhost:8080/api/v1/qr');
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{"matrix":[[1]],"mode":"full"}');
    expect(headerOf(init, 'authorization')).toBe('Bearer jwt-token');
    expect(headerOf(init, 'content-type')).toBe('application/json');
    expect(headerOf(init, 'x-request-id')).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
    );
  });

  it('rejects with an ApiError carrying the problem document', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(PROBLEM_VALIDATION_ERROR, {
          status: 422,
          contentType: 'application/problem+json',
        }),
      ),
    );

    const failure = await apiFetch({
      baseUrl: 'http://localhost:8080',
      path: '/api/v1/qr',
      body: { matrix: [[1, 2], [3]] },
    }).catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    const apiError = failure as ApiError;
    expect(apiError.status).toBe(422);
    expect(apiError.problem).toEqual(PROBLEM_VALIDATION_ERROR);
    expect(apiError.issues[0]?.code).toBe('ragged_row');
  });

  it('turns a transport failure into a network ApiError, without a token header', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'));
    vi.stubGlobal('fetch', fetchMock);

    const failure = await apiFetch({
      baseUrl: 'http://localhost:3000',
      path: '/health/ready',
      method: 'GET',
      correlate: false,
    }).catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).kind).toBe('network');
    expect((failure as ApiError).message).toContain('http://localhost:3000');
    const init = initOf(fetchMock.mock.calls[0]);
    expect(headerOf(init, 'authorization')).toBeNull();
    // Health probes stay a simple CORS request: no custom header, no preflight.
    expect(headerOf(init, 'x-request-id')).toBeNull();
  });
});
