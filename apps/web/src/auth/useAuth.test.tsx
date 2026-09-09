import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DEMO_TOKEN_RESPONSE, PROBLEM_DEMO_TOKEN_DISABLED } from '../test/fixtures';
import { headerOf, initOf, jsonResponse } from '../test/http';
import { SessionChip } from './SessionChip';
import { useAuth } from './useAuth';

const BASE_URL = 'http://localhost:8080';

/** Renders exactly what the header renders, plus the token so assertions can see it. */
function Harness() {
  const auth = useAuth(BASE_URL);
  return (
    <>
      <SessionChip auth={auth} onShowDetails={vi.fn()} />
      {/* Plain spans, not `<output>`: that element's implicit ARIA role is `status`, which
          would collide with the chip these tests address by role. */}
      <span data-testid="token">{auth.session?.token ?? 'sin token'}</span>
      <span data-testid="subject">{auth.session?.subject ?? 'sin sub'}</span>
      <span data-testid="status">{auth.status}</span>
    </>
  );
}

const demoTokenCalls = (fetchMock: ReturnType<typeof vi.fn>): unknown[][] =>
  fetchMock.mock.calls.filter((call) => String(call[0]).endsWith('/api/v1/auth/demo-token'));

describe('useAuth', () => {
  it('issues a demo token on load with no body, no credentials and no storage', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(DEMO_TOKEN_RESPONSE));
    vi.stubGlobal('fetch', fetchMock);

    render(<Harness />);

    expect(await screen.findByTestId('token')).toHaveTextContent(DEMO_TOKEN_RESPONSE.accessToken);
    expect(screen.getByTestId('subject')).toHaveTextContent('demo');
    expect(screen.getByTestId('status')).toHaveTextContent('active');
    // `Sesión de demo · JWT · 59:59` — the exact second depends on the clock.
    expect(screen.getByRole('status')).toHaveTextContent(/Sesión de demo\s*·\s*JWT\s*·\s*\d\d:\d\d/);

    // StrictMode is off in the test renderer, but the shared promise must still dedupe.
    const calls = demoTokenCalls(fetchMock);
    expect(calls).toHaveLength(1);
    const init = initOf(calls[0]);
    expect(String(calls[0]?.[0])).toBe(`${BASE_URL}/api/v1/auth/demo-token`);
    expect(init.method).toBe('POST');
    expect(init.body).toBeUndefined();
    expect(init.credentials).toBe('omit');
    expect(headerOf(init, 'authorization')).toBeNull();

    // Nothing is persisted, so an XSS has no token to read.
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    expect(document.cookie).toBe('');
  });

  it('explains the 404 of a disabled instance instead of retrying', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(PROBLEM_DEMO_TOKEN_DISABLED, {
        status: 404,
        contentType: 'application/problem+json',
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<Harness />);

    expect(await screen.findByTestId('status')).toHaveTextContent('disabled');
    expect(screen.getByRole('status')).toHaveTextContent('Esta instancia no emite tokens de demo');
    expect(screen.getByRole('button', { name: 'Ver detalles técnicos' })).toBeInTheDocument();
    expect(screen.getByTestId('token')).toHaveTextContent('sin token');
    // A disabled feature is not a transient failure: exactly one attempt.
    expect(demoTokenCalls(fetchMock)).toHaveLength(1);
  });

  it('offers a retry after a transient failure and swaps the token when it succeeds', async () => {
    const user = userEvent.setup();
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValue(jsonResponse({ ...DEMO_TOKEN_RESPONSE, scope: 'qr:compute' }));
    vi.stubGlobal('fetch', fetchMock);

    render(<Harness />);

    expect(await screen.findByTestId('status')).toHaveTextContent('error');
    expect(screen.getByRole('status')).toHaveTextContent('sin token');

    await user.click(screen.getByRole('button', { name: 'Reintentar' }));

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('active');
    });
    expect(screen.getByTestId('token')).toHaveTextContent(DEMO_TOKEN_RESPONSE.accessToken);
    expect(demoTokenCalls(fetchMock)).toHaveLength(2);
  });
});
