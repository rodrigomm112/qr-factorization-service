// Everything an evaluator needs to audit the client without opening the devtools: the two API
// origins, which token endpoint was used, what the token actually is, and the curl for the
// confidential client-credentials grant this SPA deliberately does not use.
import { Fragment } from 'react';
import type { RefObject } from 'react';
import { CLIENT_CREDENTIALS_PATH, DEMO_TOKEN_PATH } from '../api/endpoints';
import type { QrResponse } from '../api/types';
import type { Session } from '../auth/useAuth';
import type { AppConfig } from '../config';
import type { HealthStates, HealthTarget } from '../health/useHealth';
import { SERVER_MAX_DIMENSION } from '../matrix/useMatrixState';
import { CopyButton } from '../results/CopyButton';
import { MetaView } from '../results/MetaView';

/** Enough to recognise the token in a log line, far too little to replay it. */
export const TOKEN_PREVIEW_CHARS = 12;

export const previewToken = (token: string): string =>
  `${token.slice(0, TOKEN_PREVIEW_CHARS)}…`;

const curlSnippet = (baseUrl: string): string =>
  [
    `curl -sS -X POST ${baseUrl}${CLIENT_CREDENTIALS_PATH} \\`,
    "  -H 'Content-Type: application/json' \\",
    `  -d '{"clientId":"<clientId>","clientSecret":"<clientSecret>"}'`,
  ].join('\n');

export interface TechDetailsProps {
  readonly open: boolean;
  readonly onToggle: (open: boolean) => void;
  readonly config: AppConfig;
  readonly targets: readonly HealthTarget[];
  readonly health: HealthStates;
  readonly session: Session | null;
  /** Response metadata, shown here instead of next to the numbers. */
  readonly qr: QrResponse | null;
  readonly detailsRef: RefObject<HTMLDetailsElement | null>;
}

export function TechDetails({
  open,
  onToggle,
  config,
  targets,
  health,
  session,
  qr,
  detailsRef,
}: TechDetailsProps) {
  const curl = curlSnippet(config.qrApiBaseUrl);

  return (
    <details
      className="tech"
      ref={detailsRef}
      open={open}
      onToggle={(event) => {
        onToggle(event.currentTarget.open);
      }}
    >
      <summary>Detalles técnicos</summary>

      <dl className="kv">
        <dt>qr-api</dt>
        <dd className="mono">{config.qrApiBaseUrl}</dd>
        <dt>stats-api</dt>
        <dd className="mono">{config.statsApiBaseUrl}</dd>
        <dt>Endpoint de token</dt>
        <dd className="mono">
          POST {config.qrApiBaseUrl}
          {DEMO_TOKEN_PATH}
        </dd>
        <dt>Token</dt>
        <dd className="mono">{session === null ? '—' : previewToken(session.token)}</dd>
        <dt>sub</dt>
        <dd className="mono">{session?.subject ?? '—'}</dd>
        <dt>Scope</dt>
        <dd className="mono">{session?.scope ?? '—'}</dd>
        <dt>Emitido</dt>
        <dd className="mono">{session?.issuedAt ?? '—'}</dd>
        <dt>Caduca</dt>
        <dd className="mono">
          {session === null ? '—' : new Date(session.expiresAt).toISOString()}
        </dd>
        {targets.map((target) => (
          <Fragment key={target.name}>
            <dt>Versión de {target.name}</dt>
            <dd className="mono">{health[target.name]?.version ?? '—'}</dd>
          </Fragment>
        ))}
      </dl>

      <p className="hint">
        El editor se queda en 20×20 para seguir siendo legible; las APIs aceptan hasta{' '}
        {SERVER_MAX_DIMENSION}×{SERVER_MAX_DIMENSION} (<code>MAX_MATRIX_ROWS</code>/
        <code>MAX_MATRIX_COLS</code>).
      </p>
      <p className="hint">
        El token vive <strong>solo en memoria</strong>: nada en <code>localStorage</code>,{' '}
        <code>sessionStorage</code> ni cookies, así que recargar la página pide otro.
      </p>
      <p className="hint">
        <strong>Modo reducido:</strong> para una matriz m×n con k = mín(m, n), Q queda de m×k y R
        de k×k — solo las columnas de Q que sostienen R. El modo completo devuelve Q de m×m y R de
        m×n.
      </p>

      <div className="curl">
        <div className="curl-head">
          <h3>Cliente confidencial (client credentials)</h3>
          <CopyButton text={curl} label="Copiar el curl de client credentials" />
        </div>
        <p className="hint">
          Este SPA no usa esta ruta: una aplicación pública no puede guardar un secreto. Sigue
          documentada en <code>openapi.yaml</code> y la ejercitan los tests.
        </p>
        <pre className="code-block">
          <code>{curl}</code>
        </pre>
      </div>

      {qr !== null && (
        <div className="tech-meta">
          <h3>Metadatos de la última respuesta</h3>
          <MetaView meta={qr.meta} input={qr.input} requestId={qr.requestId} />
        </div>
      )}
    </details>
  );
}
