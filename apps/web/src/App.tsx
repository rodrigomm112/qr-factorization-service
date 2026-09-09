import { useCallback, useMemo, useRef, useState } from 'react';
import { computeQr, computeStatistics } from './api/endpoints';
import { errorMessage, isUnauthorized } from './api/problem';
import type { Matrix, QrMode, QrResponse, StatisticsResponse } from './api/types';
import { SessionChip } from './auth/SessionChip';
import { useAuth } from './auth/useAuth';
import type { TokenAttempt } from './auth/useAuth';
import { config } from './config';
import { HealthBadges } from './health/HealthBadges';
import { useHealth } from './health/useHealth';
import type { HealthTarget } from './health/useHealth';
import { MatrixEditor } from './matrix/MatrixEditor';
import { useMatrixState } from './matrix/useMatrixState';
import { MatrixView } from './results/MatrixView';
import { ProblemAlert } from './results/ProblemAlert';
import { StatisticsView } from './results/StatisticsView';
import { TechDetails } from './tech/TechDetails';

// Module scope: a stable reference keeps the polling effect from restarting.
const HEALTH_TARGETS: readonly HealthTarget[] = [
  { name: 'qr-api', baseUrl: config.qrApiBaseUrl },
  { name: 'stats-api', baseUrl: config.statsApiBaseUrl },
];

// Everything a `/qr` result depends on: displayed signature != editor signature means stale.
// A `null` matrix (invalid cell) never equals a signature that was sent, so it counts as changed.
const inputSignature = (values: Matrix | null, mode: QrMode): string =>
  JSON.stringify({ matrix: values, mode });

const asError = (caught: unknown): Error =>
  caught instanceof Error ? caught : new Error(errorMessage(caught));

export function App() {
  const auth = useAuth(config.qrApiBaseUrl);
  const health = useHealth(HEALTH_TARGETS);
  const matrix = useMatrixState();
  const [mode, setMode] = useState<QrMode>('full');
  const [raw, setRaw] = useState(false);
  const [techOpen, setTechOpen] = useState(false);
  const techRef = useRef<HTMLDetailsElement | null>(null);

  const [qr, setQr] = useState<QrResponse | null>(null);
  const [sent, setSent] = useState<number[][] | null>(null);
  const [sentSignature, setSentSignature] = useState<string | null>(null);
  const [qrPending, setQrPending] = useState(false);
  const [qrError, setQrError] = useState<Error | null>(null);

  const [direct, setDirect] = useState<StatisticsResponse | null>(null);
  const [directPending, setDirectPending] = useState(false);
  const [directError, setDirectError] = useState<Error | null>(null);

  const { acquire, renew } = auth;

  // Stale-response guard: a run gets the next sequence number and its own AbortController,
  // and only the run still holding the current number may write state.
  const qrSequence = useRef(0);
  const qrAbort = useRef<AbortController | null>(null);
  const directSequence = useRef(0);
  const directAbort = useRef<AbortController | null>(null);

  /**
   * One session-aware call: get a token, and on a 401 (revoked key, restarted API, expired
   * token the clock had not caught up with) issue a new demo token and replay the request
   * exactly once. A second failure reaches the results area unchanged.
   */
  const withSession = useCallback(
    async <T,>(run: (token: string) => Promise<T>): Promise<T> => {
      const first: TokenAttempt = await acquire();
      if (!first.ok) throw first.error;
      try {
        return await run(first.token);
      } catch (caught) {
        if (!isUnauthorized(caught)) throw caught;
        const retry: TokenAttempt = await renew();
        if (!retry.ok) throw retry.error;
        return await run(retry.token);
      }
    },
    [acquire, renew],
  );

  const runQr = useCallback((): void => {
    const values = matrix.values;
    if (values === null) return;

    qrAbort.current?.abort();
    const controller = new AbortController();
    qrAbort.current = controller;
    const sequence = qrSequence.current + 1;
    qrSequence.current = sequence;
    const signature = inputSignature(values, mode);

    setQrPending(true);
    setQrError(null);
    setDirect(null);
    setDirectError(null);
    void withSession((token) =>
      computeQr(config.qrApiBaseUrl, token, values, mode, controller.signal),
    )
      .then((response) => {
        if (sequence !== qrSequence.current) return;
        setQr(response);
        setSent(values);
        setSentSignature(signature);
      })
      .catch((caught: unknown) => {
        if (sequence !== qrSequence.current) return;
        setQr(null);
        setSent(null);
        setSentSignature(null);
        setQrError(asError(caught));
      })
      .finally(() => {
        if (sequence === qrSequence.current) setQrPending(false);
      });
  }, [matrix.values, mode, withSession]);

  const recomputeOnStats = useCallback((): void => {
    if (qr === null) return;

    directAbort.current?.abort();
    const controller = new AbortController();
    directAbort.current = controller;
    const sequence = directSequence.current + 1;
    directSequence.current = sequence;

    setDirectPending(true);
    setDirectError(null);
    void withSession((token) =>
      computeStatistics(
        config.statsApiBaseUrl,
        token,
        {
          matrices: [
            { label: 'Q', values: qr.q },
            { label: 'R', values: qr.r },
          ],
        },
        controller.signal,
      ),
    )
      .then((response) => {
        if (sequence !== directSequence.current) return;
        setDirect(response);
      })
      .catch((caught: unknown) => {
        if (sequence !== directSequence.current) return;
        setDirect(null);
        setDirectError(asError(caught));
      })
      .finally(() => {
        if (sequence === directSequence.current) setDirectPending(false);
      });
  }, [qr, withSession]);

  const openTechDetails = useCallback((): void => {
    setTechOpen(true);
    // jsdom has no layout engine, hence the capability check rather than a bare call.
    const node = techRef.current;
    if (typeof node?.scrollIntoView === 'function') {
      node.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }, []);

  const editorSignature = useMemo(
    () => inputSignature(matrix.values, mode),
    [matrix.values, mode],
  );
  const stale = qr !== null && sentSignature !== null && editorSignature !== sentSignature;

  return (
    <div className="shell">
      <header className="app-header">
        <div className="brand">
          <h1>Factorización QR</h1>
          <p className="tagline">Descompón una matriz y consulta sus estadísticas</p>
        </div>
        <div className="header-status">
          <HealthBadges targets={HEALTH_TARGETS} states={health} />
          <SessionChip auth={auth} onShowDetails={openTechDetails} />
        </div>
      </header>

      <main className="stack">
        <MatrixEditor
          matrix={matrix}
          mode={mode}
          onModeChange={setMode}
          onSubmit={runQr}
          pending={qrPending}
        />

        <section className="panel" aria-labelledby="results-title" aria-busy={qrPending}>
          <div className="panel-head">
            <h2 id="results-title">Resultado · Q, R y estadísticas</h2>
            <label className="toggle">
              <input
                type="checkbox"
                checked={raw}
                onChange={(event) => {
                  setRaw(event.target.checked);
                }}
              />
              Valores crudos
            </label>
          </div>

          {qrError !== null && <ProblemAlert error={qrError} title="No se pudo calcular:" />}

          {stale && (
            <p className="stale-badge" role="status">
              Resultados de una versión anterior de la matriz. Vuelve a pulsar{' '}
              <strong>Calcular QR</strong> para actualizarlos.
            </p>
          )}

          {qr === null && qrError === null && (
            <p className="hint empty-state">
              Aún no hay resultados. Elige una matriz y pulsa <strong>Calcular QR</strong>.
            </p>
          )}

          {qr !== null && (
            <>
              <div className="matrices">
                {sent !== null && (
                  <MatrixView
                    title="A"
                    values={sent}
                    raw={raw}
                    caption="entrada"
                    zeroTolerance={qr.statistics.meta.diagonalTolerance.absolute}
                  />
                )}
                <MatrixView
                  title="Q"
                  values={qr.q}
                  raw={raw}
                  caption="ortogonal"
                  zeroTolerance={qr.statistics.meta.diagonalTolerance.absolute}
                />
                <MatrixView
                  title="R"
                  values={qr.r}
                  raw={raw}
                  caption="triangular superior"
                  zeroTolerance={qr.statistics.meta.diagonalTolerance.absolute}
                />
              </div>

              <StatisticsView
                title="Estadísticas de Q y R"
                report={qr.statistics}
                raw={raw}
                source="qr-api → stats-api"
              />

              {directError !== null && (
                <ProblemAlert error={directError} title="Llamada directa fallida:" />
              )}

              {direct !== null && (
                <StatisticsView
                  title="Estadísticas recalculadas"
                  report={direct}
                  raw={raw}
                  source="stats-api (llamada directa del navegador)"
                />
              )}

              <div className="results-footer">
                <p className="hint">
                  El navegador llama a <code>{config.statsApiBaseUrl}/api/v1/statistics</code> con
                  el mismo token: el web consume las dos APIs.
                </p>
                <button
                  type="button"
                  className="secondary"
                  onClick={recomputeOnStats}
                  disabled={directPending}
                  aria-busy={directPending}
                >
                  {directPending ? 'Recalculando…' : 'Recalcular en stats-api'}
                </button>
              </div>
            </>
          )}
        </section>

        <TechDetails
          open={techOpen}
          onToggle={setTechOpen}
          config={config}
          targets={HEALTH_TARGETS}
          health={health}
          session={auth.session}
          qr={qr}
          detailsRef={techRef}
        />
      </main>
    </div>
  );
}
