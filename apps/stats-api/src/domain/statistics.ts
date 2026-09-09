import { EmptyMatrixError } from './errors.js';
import type { Matrix } from './matrix.js';

// Diagonality tolerance `max(absTol, relTol * max|a_ij|)`: the relative term makes the test scale
// invariant (QR residue is O(||A|| * u)); the absolute one keeps it meaningful for the zero matrix.
export interface DiagonalTolerance {
  readonly absTol: number;
  readonly relTol: number;
}

// Unevaluated sum of two doubles. Keeping both halves lets partial results merge exactly across
// matrices; collapsing to a single `number` first rounds `[[1e16, 1]]` back to `1e16`.
export interface CompensatedTotal {
  readonly value: number;
  readonly compensation: number;
}

/** Statistics of one matrix. `maxAbs` is carried because it sets the diagonal tolerance. */
export interface MatrixStatistics {
  readonly rows: number;
  readonly cols: number;
  readonly count: number;
  readonly max: number;
  readonly min: number;
  readonly sum: number;
  /** `sum` before rounding, for exact aggregation across matrices. */
  readonly compensatedSum: CompensatedTotal;
  readonly average: number;
  readonly maxAbs: number;
  readonly isDiagonal: boolean;
}

export interface StatisticsSummary {
  readonly max: number;
  readonly min: number;
  readonly sum: number;
  readonly average: number;
  readonly count: number;
  readonly anyDiagonal: boolean;
}

// Neumaier, not Kahan: it also compensates when the addend is larger than the running sum, so
// `[1e16, 1, -1e16]` sums to 1 where the naive loop gives 0. On overflow `total` is +-Infinity,
// never NaN.
export class NeumaierSummation {
  private runningSum = 0;
  private compensation = 0;

  public add(value: number): void {
    const t = this.runningSum + value;
    if (Number.isFinite(t)) {
      this.compensation +=
        Math.abs(this.runningSum) >= Math.abs(value)
          ? this.runningSum - t + value
          : value - t + this.runningSum;
    } else {
      // Saturated: any further correction is Infinity - Infinity = NaN, which poisons the
      // accumulator for good. A finite exact sum can still saturate ([1e308, 1e308, -1e308,
      // -1e308]); that is unrecoverable, and the caller maps the infinity to 422.
      this.compensation = 0;
    }
    this.runningSum = t;
  }

  /** Merges another accumulator, both halves, without evaluating it first. */
  public addCompensated(total: CompensatedTotal): void {
    this.add(total.value);
    this.add(total.compensation);
  }

  public get parts(): CompensatedTotal {
    return { value: this.runningSum, compensation: this.compensation };
  }

  public get total(): number {
    return this.runningSum + this.compensation;
  }
}

export function diagonalThreshold(maxAbs: number, tolerance: DiagonalTolerance): number {
  return Math.max(tolerance.absTol, tolerance.relTol * maxAbs);
}

interface MatrixScan {
  readonly rows: number;
  readonly cols: number;
  readonly count: number;
  readonly max: number;
  readonly min: number;
  readonly sum: CompensatedTotal;
  readonly maxAbs: number;
  readonly maxOffDiagonalAbs: number;
}

// One pass: tracking the off-diagonal maximum here lets `isDiagonal` use a `maxAbs`-derived
// tolerance without a second traversal.
function scan(matrix: Matrix): MatrixScan {
  const firstRow = matrix[0];
  const sum = new NeumaierSummation();
  let max = Number.NEGATIVE_INFINITY;
  let min = Number.POSITIVE_INFINITY;
  let maxAbs = 0;
  let maxOffDiagonalAbs = 0;
  let count = 0;

  let i = 0;
  for (const row of matrix) {
    let j = 0;
    for (const value of row) {
      count += 1;
      sum.add(value);
      if (value > max) max = value;
      if (value < min) min = value;
      const magnitude = Math.abs(value);
      if (magnitude > maxAbs) maxAbs = magnitude;
      if (i !== j && magnitude > maxOffDiagonalAbs) maxOffDiagonalAbs = magnitude;
      j += 1;
    }
    i += 1;
  }

  return {
    rows: matrix.length,
    cols: firstRow?.length ?? 0,
    count,
    max,
    min,
    sum: sum.parts,
    maxAbs,
    maxOffDiagonalAbs,
  };
}

/** @throws {EmptyMatrixError} when the matrix holds no values. */
export function computeMatrixStatistics(
  matrix: Matrix,
  tolerance: DiagonalTolerance,
): MatrixStatistics {
  const scanned = scan(matrix);
  if (scanned.count === 0) {
    throw new EmptyMatrixError();
  }
  const sum = scanned.sum.value + scanned.sum.compensation;
  return {
    rows: scanned.rows,
    cols: scanned.cols,
    count: scanned.count,
    max: scanned.max,
    min: scanned.min,
    sum,
    compensatedSum: scanned.sum,
    average: sum / scanned.count,
    maxAbs: scanned.maxAbs,
    isDiagonal: scanned.maxOffDiagonalAbs <= diagonalThreshold(scanned.maxAbs, tolerance),
  };
}

// Merges the per-matrix accumulators unevaluated, so the global sum is exactly what a single
// Neumaier pass over every value would produce. Throws EmptyMatrixError on empty input.
export function aggregate(statistics: readonly MatrixStatistics[]): StatisticsSummary {
  if (statistics.length === 0) {
    throw new EmptyMatrixError('aggregation requires at least one matrix');
  }

  const sum = new NeumaierSummation();
  let max = Number.NEGATIVE_INFINITY;
  let min = Number.POSITIVE_INFINITY;
  let count = 0;
  let anyDiagonal = false;

  for (const item of statistics) {
    sum.addCompensated(item.compensatedSum);
    if (item.max > max) max = item.max;
    if (item.min < min) min = item.min;
    count += item.count;
    anyDiagonal ||= item.isDiagonal;
  }

  const total = sum.total;
  return { max, min, sum: total, average: total / count, count, anyDiagonal };
}
