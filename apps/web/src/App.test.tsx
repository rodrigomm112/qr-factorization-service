import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { UserEvent } from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { App } from './App';
import {
  DEMO_TOKEN_RESPONSE,
  PROBLEM_UNAUTHORIZED,
  QR_RESPONSE_IDENTITY_3X3,
  QR_RESPONSE_TALL_3X2,
} from './test/fixtures';
import { headerOf, initOf, jsonResponse } from './test/http';

interface Deferred {
  readonly promise: Promise<Response>;
  readonly settle: (response: Response) => void;
}

function defer(): Deferred {
  let settle!: (response: Response) => void;
  const promise = new Promise<Response>((resolve) => {
    settle = resolve;
  });
  return { promise, settle };
}

const healthResponse = (): Response =>
  jsonResponse(
    { status: 'pass', service: 'qr-api', version: '1.0.0', uptimeSeconds: 12 },
    { contentType: 'application/health+json' },
  );

const unauthorized = (): Response =>
  jsonResponse(PROBLEM_UNAUTHORIZED, { status: 401, contentType: 'application/problem+json' });

/** A second token, so the retry can be told apart from the first attempt. */
const RENEWED_TOKEN = `${DEMO_TOKEN_RESPONSE.accessToken}-renewed`;

interface Stack {
  readonly fetchMock: ReturnType<typeof vi.fn>;
  readonly qrCalls: Deferred[];
}

/** Health and the demo token answer at once; every `/qr` stays pending until the test settles it. */
function stubStack(): Stack {
  const qrCalls: Deferred[] = [];
  const fetchMock = vi.fn((input: unknown) => {
    const url = String(input);
    if (url.endsWith('/health/ready')) return Promise.resolve(healthResponse());
    if (url.endsWith('/api/v1/auth/demo-token')) {
      return Promise.resolve(jsonResponse(DEMO_TOKEN_RESPONSE));
    }
    if (url.endsWith('/api/v1/qr')) {
      const call = defer();
      qrCalls.push(call);
      return call.promise;
    }
    return Promise.reject(new Error(`unexpected request: ${url}`));
  });
  vi.stubGlobal('fetch', fetchMock);
  return { fetchMock, qrCalls };
}

/** The automatic session has landed once the header chip shows the countdown. */
const awaitSession = (): Promise<HTMLElement> => screen.findByText('JWT');

const setCell = async (user: UserEvent, label: string, value: string): Promise<void> => {
  const cell = screen.getByLabelText(label);
  await user.clear(cell);
  await user.type(cell, value);
};

const callsTo = (fetchMock: Stack['fetchMock'], path: string): readonly unknown[][] =>
  fetchMock.mock.calls.filter((call) => String(call[0]).endsWith(path));

const qrSignals = (fetchMock: Stack['fetchMock']): RequestInit['signal'][] =>
  callsTo(fetchMock, '/api/v1/qr').map((call) => initOf(call).signal);

/** jsdom has no Clipboard API; the button hides itself unless one is present. */
function stubClipboard(): { writeText: ReturnType<typeof vi.fn>; restore: () => void } {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'clipboard');
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(window.navigator, 'clipboard', { value: { writeText }, configurable: true });
  return {
    writeText,
    restore: () => {
      if (original === undefined) Reflect.deleteProperty(window.navigator, 'clipboard');
      else Object.defineProperty(window.navigator, 'clipboard', original);
    },
  };
}

describe('App', () => {
  it('drops the response of a superseded /qr run and flags stale results', async () => {
    const user = userEvent.setup();
    const { fetchMock, qrCalls } = stubStack();
    render(<App />);
    await awaitSession();

    await user.click(screen.getByRole('button', { name: 'Calcular QR' }));
    await waitFor(() => {
      expect(qrCalls).toHaveLength(1);
    });

    // The editor stays usable in flight: the window where a late response could overwrite state.
    await setCell(user, 'Fila 0, columna 0', '9');
    await user.click(screen.getByRole('button', { name: 'Calculando…' }));
    await waitFor(() => {
      expect(qrCalls).toHaveLength(2);
    });

    const signals = qrSignals(fetchMock);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals[1]?.aborted).toBe(false);

    qrCalls[1]?.settle(jsonResponse({ ...QR_RESPONSE_TALL_3X2, requestId: 'newest' }));
    expect(await screen.findByText('newest')).toBeInTheDocument();

    // The first response arrives last and must be ignored.
    qrCalls[0]?.settle(jsonResponse({ ...QR_RESPONSE_IDENTITY_3X3, requestId: 'superseded' }));
    await waitFor(() => {
      expect(screen.queryByText('superseded')).not.toBeInTheDocument();
    });
    expect(screen.getByText('newest')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Calcular QR' })).toBeEnabled();

    expect(screen.queryByText(/versión anterior de la matriz/i)).not.toBeInTheDocument();

    await setCell(user, 'Fila 0, columna 0', '4');
    expect(await screen.findByText(/versión anterior de la matriz/i)).toBeInTheDocument();
    expect(screen.getByText('newest')).toBeInTheDocument();
  });

  it('marks the results stale when only the mode changes', async () => {
    const user = userEvent.setup();
    const { qrCalls } = stubStack();
    render(<App />);
    await awaitSession();

    await user.click(screen.getByRole('button', { name: 'Calcular QR' }));
    await waitFor(() => {
      expect(qrCalls).toHaveLength(1);
    });
    qrCalls[0]?.settle(jsonResponse({ ...QR_RESPONSE_TALL_3X2, requestId: 'full-mode' }));
    expect(await screen.findByText('full-mode')).toBeInTheDocument();

    await user.click(screen.getByRole('radio', { name: 'Reducida (Q m×k)' }));
    expect(await screen.findByText(/versión anterior de la matriz/i)).toBeInTheDocument();
  });

  it('issues a new demo token on a 401 and replays the request once', async () => {
    const user = userEvent.setup();
    let tokenCalls = 0;
    let qrCalls = 0;
    const fetchMock = vi.fn((input: unknown) => {
      const url = String(input);
      if (url.endsWith('/health/ready')) return Promise.resolve(healthResponse());
      if (url.endsWith('/api/v1/auth/demo-token')) {
        tokenCalls += 1;
        const accessToken = tokenCalls === 1 ? DEMO_TOKEN_RESPONSE.accessToken : RENEWED_TOKEN;
        return Promise.resolve(jsonResponse({ ...DEMO_TOKEN_RESPONSE, accessToken }));
      }
      if (url.endsWith('/api/v1/qr')) {
        qrCalls += 1;
        return Promise.resolve(
          qrCalls === 1 ? unauthorized() : jsonResponse(QR_RESPONSE_TALL_3X2),
        );
      }
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);
    await awaitSession();
    await user.click(screen.getByRole('button', { name: 'Calcular QR' }));

    expect(await screen.findByText(QR_RESPONSE_TALL_3X2.requestId)).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    const attempts = callsTo(fetchMock, '/api/v1/qr');
    expect(attempts).toHaveLength(2);
    expect(headerOf(initOf(attempts[0]), 'authorization')).toBe(
      `Bearer ${DEMO_TOKEN_RESPONSE.accessToken}`,
    );
    expect(headerOf(initOf(attempts[1]), 'authorization')).toBe(`Bearer ${RENEWED_TOKEN}`);
    expect(callsTo(fetchMock, '/api/v1/auth/demo-token')).toHaveLength(2);
  });

  it('gives up after one retry and shows the failure in the results area', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn((input: unknown) => {
      const url = String(input);
      if (url.endsWith('/health/ready')) return Promise.resolve(healthResponse());
      if (url.endsWith('/api/v1/auth/demo-token')) {
        return Promise.resolve(jsonResponse(DEMO_TOKEN_RESPONSE));
      }
      if (url.endsWith('/api/v1/qr')) return Promise.resolve(unauthorized());
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);
    await awaitSession();
    await setCell(user, 'Fila 1, columna 1', '7');
    await user.click(screen.getByRole('button', { name: 'Calcular QR' }));

    const results = screen.getByRole('region', { name: 'Resultado · Q, R y estadísticas' });
    const alert = await within(results).findByRole('alert');
    expect(alert).toHaveTextContent('Credenciales inválidas o sesión expirada.');
    expect(callsTo(fetchMock, '/api/v1/qr')).toHaveLength(2);

    // The matrix survives the failure: nothing to retype.
    expect(screen.getByLabelText('Fila 1, columna 1')).toHaveValue('7');
    expect(screen.getByLabelText('Fila 2, columna 1')).toHaveValue('6');
  });

  it('copies a rendered matrix to the clipboard as JSON', async () => {
    // `userEvent.setup()` installs a clipboard stub of its own, so ours has to come second.
    const user = userEvent.setup();
    const clipboard = stubClipboard();
    try {
      const { qrCalls } = stubStack();
      render(<App />);
      await awaitSession();

      await user.click(screen.getByRole('button', { name: 'Calcular QR' }));
      await waitFor(() => {
        expect(qrCalls).toHaveLength(1);
      });
      qrCalls[0]?.settle(jsonResponse(QR_RESPONSE_TALL_3X2));

      const copyQ = await screen.findByRole('button', { name: 'Copiar Q en JSON' });
      await user.click(copyQ);

      expect(clipboard.writeText).toHaveBeenCalledWith(JSON.stringify(QR_RESPONSE_TALL_3X2.q));
      expect(await screen.findByRole('button', { name: 'Copiar Q en JSON' })).toHaveTextContent(
        'Copiado',
      );
      expect(screen.getByRole('button', { name: 'Copiar R en JSON' })).toHaveTextContent('Copiar');
    } finally {
      clipboard.restore();
    }
  });
});
