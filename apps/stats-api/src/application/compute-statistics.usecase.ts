import { NumericalOverflowError } from '../domain/errors.js';
import type { Matrix } from '../domain/matrix.js';
import type { DiagonalTolerance, MatrixStatistics, StatisticsSummary } from '../domain/statistics.js';
import { aggregate, computeMatrixStatistics } from '../domain/statistics.js';

/** One matrix of the request, already validated and normalized by the inbound adapter. */
export interface LabeledMatrix {
  readonly label: string;
  readonly values: Matrix;
}

/** Per-matrix breakdown exactly as it appears in the response body. */
export interface MatrixStatisticsView {
  readonly index: number;
  readonly label: string;
  readonly rows: number;
  readonly cols: number;
  readonly max: number;
  readonly min: number;
  readonly sum: number;
  readonly average: number;
  readonly isDiagonal: boolean;
}

export interface StatisticsMetaView {
  readonly summationAlgorithm: 'neumaier';
  readonly diagonalTolerance: {
    readonly absolute: number;
    readonly relative: number;
  };
  readonly elapsedMs: number;
}

/** The response body minus `requestId`, which belongs to the HTTP layer. */
export interface StatisticsResult {
  readonly summary: StatisticsSummary;
  readonly matrices: readonly MatrixStatisticsView[];
  readonly meta: StatisticsMetaView;
}

export interface ComputeStatisticsOptions {
  readonly tolerance: DiagonalTolerance;
  /** Milliseconds; defaults to `performance.now`. */
  readonly clock?: () => number;
}

/** Pure and synchronous: there is no I/O to orchestrate, so the use case is a function. */
export function computeStatistics(
  matrices: readonly LabeledMatrix[],
  options: ComputeStatisticsOptions,
): StatisticsResult {
  const clock = options.clock ?? (() => performance.now());
  const startedAt = clock();

  const perMatrix: MatrixStatistics[] = [];
  const view: MatrixStatisticsView[] = [];

  matrices.forEach((matrix, index) => {
    const statistics = computeMatrixStatistics(matrix.values, options.tolerance);
    perMatrix.push(statistics);
    view.push({
      index,
      label: matrix.label,
      rows: statistics.rows,
      cols: statistics.cols,
      max: statistics.max,
      min: statistics.min,
      sum: statistics.sum,
      average: statistics.average,
      isDiagonal: statistics.isDiagonal,
    });
  });

  const summary = aggregate(perMatrix);
  // `max`/`min` are input entries the schema already checked, and `average` is `sum / count` with
  // `count >= 1`; the sum is the only figure that can leave the double range.
  if (!Number.isFinite(summary.sum) || view.some((matrix) => !Number.isFinite(matrix.sum))) {
    throw new NumericalOverflowError();
  }

  return {
    summary,
    matrices: view,
    meta: {
      summationAlgorithm: 'neumaier',
      diagonalTolerance: {
        absolute: options.tolerance.absTol,
        relative: options.tolerance.relTol,
      },
      elapsedMs: clock() - startedAt,
    },
  };
}
