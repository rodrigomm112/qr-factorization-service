// Demo session. On mount the SPA asks qr-api for an anonymous demo token, refreshes it silently
// shortly before it expires, and re-issues one when an API answers 401. The token lives ONLY in
// React state — never localStorage, sessionStorage or a cookie — so no XSS-readable storage holds
// it and a reload simply asks for another one. The confidential client-credentials grant still
// exists (see `TechDetails`), but a public SPA cannot keep a client secret, so it is curl-only.
import { useCallback, useEffect, useRef, useState } from 'react';
import { issueDemoToken } from '../api/endpoints';
import { errorMessage, isNotFound } from '../api/problem';

export interface Session {
  readonly token: string;
  /** JWT `sub`, `"demo"` for this grant; `null` when the payload cannot be read. */
  readonly subject: string | null;
  readonly scope: string;
  readonly issuedAt: string;
  /** Epoch milliseconds. */
  readonly expiresAt: number;
  /** Lifetime the server granted, in seconds. */
  readonly expiresIn: number;
}

/**
 * `disabled` is the one state the UI explains instead of retrying: the instance was started with
 * `DEMO_TOKEN_ENABLED=false` and no amount of retrying will mint a token.
 */
export type AuthStatus = 'pending' | 'active' | 'disabled' | 'error';

/** No rejection to forget: every caller has to look at `ok` before using the token. */
export type TokenAttempt =
  | { readonly ok: true; readonly token: string }
  | { readonly ok: false; readonly error: Error };

export interface AuthApi {
  readonly session: Session | null;
  readonly status: AuthStatus;
  readonly error: Error | null;
  readonly secondsLeft: number;
  /** The live token, or a freshly issued one when there is none. */
  acquire: () => Promise<TokenAttempt>;
  /** Drops the session and issues another one: the 401 recovery path. */
  renew: () => Promise<TokenAttempt>;
}

/** Renew this long before the deadline, or at half the lifetime for very short tokens. */
export const REFRESH_LEAD_MS = 60_000;

/** A floor for the refresh timer, so a tiny `expiresIn` cannot spin the endpoint. */
const MIN_REFRESH_DELAY_MS = 1_000;

export const DEMO_TOKEN_DISABLED_MESSAGE = 'Esta instancia no emite tokens de demo';

const asError = (caught: unknown): Error =>
  caught instanceof Error ? caught : new Error(errorMessage(caught));

/** `sub` out of the JWT payload. Display only — the APIs verify the signature, this never does. */
export function readSubject(token: string): string | null {
  const payload = token.split('.')[1];
  if (payload === undefined || payload === '') return null;
  try {
    const base64 = payload.replaceAll('-', '+').replaceAll('_', '/');
    const decoded: unknown = JSON.parse(atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, '=')));
    if (typeof decoded !== 'object' || decoded === null) return null;
    const subject = (decoded as Record<string, unknown>).sub;
    return typeof subject === 'string' ? subject : null;
  } catch {
    return null;
  }
}

export function useAuth(qrApiBaseUrl: string): AuthApi {
  const [session, setSession] = useState<Session | null>(null);
  const [status, setStatus] = useState<AuthStatus>('pending');
  const [error, setError] = useState<Error | null>(null);
  const [now, setNow] = useState(() => Date.now());

  // One in-flight request at a time: StrictMode's double mount, a silent refresh and two
  // simultaneous 401s all share the same promise instead of racing for four tokens.
  const inFlight = useRef<Promise<TokenAttempt> | null>(null);
  const sessionRef = useRef<Session | null>(null);

  const request = useCallback((): Promise<TokenAttempt> => {
    const pending = inFlight.current;
    if (pending !== null) return pending;

    setStatus('pending');
    setError(null);
    const attempt = issueDemoToken(qrApiBaseUrl).then(
      (token): TokenAttempt => {
        const next: Session = {
          token: token.accessToken,
          subject: readSubject(token.accessToken),
          scope: token.scope,
          issuedAt: token.issuedAt,
          expiresAt: Date.now() + token.expiresIn * 1000,
          expiresIn: token.expiresIn,
        };
        sessionRef.current = next;
        setNow(Date.now());
        setSession(next);
        setStatus('active');
        return { ok: true, token: next.token };
      },
      (caught: unknown): TokenAttempt => {
        const failure = asError(caught);
        sessionRef.current = null;
        setSession(null);
        setStatus(isNotFound(failure) ? 'disabled' : 'error');
        setError(failure);
        return { ok: false, error: failure };
      },
    );

    inFlight.current = attempt;
    void attempt.finally(() => {
      if (inFlight.current === attempt) inFlight.current = null;
    });
    return attempt;
  }, [qrApiBaseUrl]);

  const acquire = useCallback((): Promise<TokenAttempt> => {
    const current = sessionRef.current;
    if (current !== null && current.expiresAt > Date.now()) {
      return Promise.resolve({ ok: true, token: current.token });
    }
    return request();
  }, [request]);

  const renew = useCallback((): Promise<TokenAttempt> => {
    // Only drop the session when nothing is already on its way back with a fresh one.
    if (inFlight.current === null) {
      sessionRef.current = null;
      setSession(null);
    }
    return request();
  }, [request]);

  // Auto session: no form, no click. StrictMode mounts twice; `inFlight` collapses that.
  useEffect(() => {
    void request();
  }, [request]);

  // The countdown ticker plus the silent refresh, both owned by the current session.
  useEffect(() => {
    if (session === null) return;
    const ticker = setInterval(() => {
      setNow(Date.now());
    }, 1000);
    const lead = Math.min(REFRESH_LEAD_MS, (session.expiresIn * 1000) / 2);
    const delay = Math.max(MIN_REFRESH_DELAY_MS, session.expiresAt - lead - Date.now());
    const refresh = setTimeout(() => {
      void request();
    }, delay);
    return () => {
      clearInterval(ticker);
      clearTimeout(refresh);
    };
  }, [session, request]);

  const secondsLeft =
    session === null ? 0 : Math.max(0, Math.ceil((session.expiresAt - now) / 1000));

  return { session, status, error, secondsLeft, acquire, renew };
}
