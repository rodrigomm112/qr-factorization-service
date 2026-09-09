import type { StatisticsReport } from "../api/types";
import { formatMs, formatNumber, formatShape } from "./format";

export interface StatisticsViewProps {
  readonly title: string;
  readonly report: StatisticsReport;
  readonly raw: boolean;
  readonly source: string;
}

/** The five required figures first, then the per-matrix breakdown stats-api adds on top. */
export function StatisticsView({
  title,
  report,
  raw,
  source,
}: StatisticsViewProps) {
  const { summary, matrices, meta } = report;
  const zero = meta.diagonalTolerance.absolute;

  return (
    <div className="statistics">
      <h3>
        {title} <span className="hint">· {source}</span>
      </h3>
      <ul className="metrics">
        <li>
          <span className="metric-label">Máximo</span>
          <span className="metric-value mono">
            {formatNumber(summary.max, raw, zero)}
          </span>
        </li>
        <li>
          <span className="metric-label">Mínimo</span>
          <span className="metric-value mono">
            {formatNumber(summary.min, raw, zero)}
          </span>
        </li>
        <li>
          <span className="metric-label">Promedio</span>
          <span className="metric-value mono">
            {formatNumber(summary.average, raw, zero)}
          </span>
        </li>
        <li>
          <span className="metric-label">Suma</span>
          <span className="metric-value mono">
            {formatNumber(summary.sum, raw, zero)}
          </span>
        </li>
        <li>
          <span className="metric-label">¿Alguna diagonal?</span>
          <span
            className={
              summary.anyDiagonal ? "metric-value yes" : "metric-value no"
            }
          >
            {summary.anyDiagonal ? "Sí" : "No"}
          </span>
        </li>
        <li>
          <span className="metric-label">Elementos</span>
          <span className="metric-value mono">{summary.count}</span>
        </li>
      </ul>

      <div className="matrix-scroll">
        <table className="data-table">
          <caption className="visually-hidden">Estadísticas por matriz</caption>
          <thead>
            <tr>
              <th scope="col">#</th>
              <th scope="col">Matriz</th>
              <th scope="col">Forma</th>
              <th scope="col">Máx</th>
              <th scope="col">Mín</th>
              <th scope="col">Suma</th>
              <th scope="col">Promedio</th>
              <th scope="col">Diagonal</th>
            </tr>
          </thead>
          <tbody>
            {matrices.map((entry) => (
              <tr key={entry.index}>
                <td className="num mono">{entry.index}</td>
                <th scope="row">{entry.label}</th>
                <td className="mono">{formatShape(entry.rows, entry.cols)}</td>
                <td className="num mono">
                  {formatNumber(entry.max, raw, zero)}
                </td>
                <td className="num mono">
                  {formatNumber(entry.min, raw, zero)}
                </td>
                <td className="num mono">
                  {formatNumber(entry.sum, raw, zero)}
                </td>
                <td className="num mono">
                  {formatNumber(entry.average, raw, zero)}
                </td>
                <td className={entry.isDiagonal ? "yes" : "no"}>
                  {entry.isDiagonal ? "Sí" : "No"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className="hint">
        Suma por <strong>{meta.summationAlgorithm}</strong> · tolerancia
        diagonal{" "}
        <span className="mono">
          máx({meta.diagonalTolerance.absolute},{" "}
          {meta.diagonalTolerance.relative}·máx|aᵢⱼ|)
        </span>{" "}
        · {formatMs(meta.elapsedMs)}
      </p>
    </div>
  );
}
