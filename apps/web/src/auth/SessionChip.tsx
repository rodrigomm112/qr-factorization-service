import { DEMO_TOKEN_DISABLED_MESSAGE } from './useAuth';
import type { AuthApi } from './useAuth';

/** `mm:ss`, zero-padded so the chip never changes width. */
export function formatCountdown(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
}

export interface SessionChipProps {
  readonly auth: AuthApi;
  /** Opens the technical details at the foot, where the client-credentials curl lives. */
  readonly onShowDetails: () => void;
}

/**
 * The whole session UI: there is no login form, so the header only reports what the automatic
 * demo grant did. `disabled` (404) is the one branch that offers an explanation instead of a
 * retry, because the instance was started with `DEMO_TOKEN_ENABLED=false`.
 */
export function SessionChip({ auth, onShowDetails }: SessionChipProps) {
  const { status, session, secondsLeft } = auth;

  if (status === 'disabled') {
    return (
      <p className="session-notice" role="status">
        {DEMO_TOKEN_DISABLED_MESSAGE}{' '}
        <button type="button" className="linklike" onClick={onShowDetails}>
          Ver detalles técnicos
        </button>
      </p>
    );
  }

  if (status === 'error' || session === null) {
    const pending = status === 'pending';
    return (
      <p className="session-chip" role="status">
        <span>Sesión de demo</span>
        <span aria-hidden="true">·</span>
        <span className="mono">{pending ? 'solicitando…' : 'sin token'}</span>
        {!pending && (
          <button
            type="button"
            className="linklike"
            onClick={() => {
              void auth.renew();
            }}
          >
            Reintentar
          </button>
        )}
      </p>
    );
  }

  return (
    <p
      className="session-chip"
      role="status"
      title={`Token en memoria, emitido ${session.issuedAt}`}
    >
      <span>Sesión de demo</span>
      <span aria-hidden="true">·</span>
      <span className="mono">JWT</span>
      <span aria-hidden="true">·</span>
      <span className="countdown mono">{formatCountdown(secondsLeft)}</span>
    </p>
  );
}
