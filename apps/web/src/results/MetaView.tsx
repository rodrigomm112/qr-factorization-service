import type { MatrixShape, QrMeta } from "../api/types";
import { formatMs, formatShape } from "./format";

export interface MetaViewProps {
  readonly meta: QrMeta;
  readonly input: MatrixShape;
  readonly requestId: string;
}

/** Provenance: algorithm, shapes, timings, correlation id. */
export function MetaView({ meta, input, requestId }: MetaViewProps) {
  return (
    <dl className="kv meta">
      <dt>Algoritmo</dt>
      <dd className="mono">{meta.algorithm}</dd>
      <dt>Modo</dt>
      <dd className="mono">{meta.mode}</dd>
      <dt>Entrada</dt>
      <dd className="mono">{formatShape(input.rows, input.cols)}</dd>
      <dt>Q</dt>
      <dd className="mono">{formatShape(meta.qShape[0], meta.qShape[1])}</dd>
      <dt>R</dt>
      <dd className="mono">{formatShape(meta.rShape[0], meta.rShape[1])}</dd>
      <dt>Descomposición</dt>
      <dd className="mono">{formatMs(meta.decompositionMs)}</dd>
      <dt>Estadísticas (ida y vuelta a stats-api)</dt>
      <dd className="mono">{formatMs(meta.statisticsMs)}</dd>
      <dt>requestId</dt>
      <dd className="mono">{requestId}</dd>
    </dl>
  );
}
