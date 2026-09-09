import { describe, expect, it } from 'vitest';
import { EmptyMatrixError } from '../../src/domain/errors.js';
import type { Matrix } from '../../src/domain/matrix.js';
import {
  aggregate,
  computeMatrixStatistics,
  diagonalThreshold,
  NeumaierSummation,
} from '../../src/domain/statistics.js';
import type { DiagonalTolerance } from '../../src/domain/statistics.js';

const TOLERANCE: DiagonalTolerance = { absTol: 1e-12, relTol: 1e-9 };

// Test-only shims: production never sums a flat list nor asks isDiagonal on its own.
function* values(matrix: Matrix): Generator<number> {
  for (const row of matrix) yield* row;
}
function neumaierSum(numbers: Iterable<number>): number {
  const accumulator = new NeumaierSummation();
  for (const value of numbers) accumulator.add(value);
  return accumulator.total;
}
function isDiagonal(matrix: Matrix, tolerance: DiagonalTolerance): boolean {
  return computeMatrixStatistics(matrix, tolerance).isDiagonal;
}

function naiveSum(numbers: readonly number[]): number {
  return numbers.reduce((total, value) => total + value, 0);
}

describe('neumaierSum', () => {
  it('recovers the exact total where the naive loop cancels catastrophically', () => {
    const input = [1e16, 1, -1e16];
    expect(naiveSum(input)).toBe(0);
    expect(neumaierSum(input)).toBe(1);
  });

  it('saturates to an infinity instead of a NaN when the total overflows', () => {
    // Naive compensation would answer NaN (Infinity - Infinity), which serializes as `null`.
    expect(neumaierSum([1e308, 1e308])).toBe(Number.POSITIVE_INFINITY);
    expect(neumaierSum([-1e308, -1e308])).toBe(Number.NEGATIVE_INFINITY);
  });

  it('reports an overflow even when the exact sum would be finite', () => {
    const input = [1e308, 1e308, -1e308, -1e308];
    expect(naiveSum(input)).toBe(Number.POSITIVE_INFINITY);
    expect(neumaierSum(input)).toBe(Number.POSITIVE_INFINITY);
  });

  it('is insensitive to the order of the addends', () => {
    expect(neumaierSum([-1e16, 1, 1e16])).toBe(1);
    expect(neumaierSum([1, -1e16, 1e16])).toBe(1);
  });

  it('sums ten times 0.1 to exactly 1', () => {
    const tenth = Array.from({ length: 10 }, () => 0.1);
    expect(naiveSum(tenth)).not.toBe(1);
    expect(neumaierSum(tenth)).toBe(1);
  });

  it('sums the empty sequence to zero', () => {
    expect(neumaierSum([])).toBe(0);
  });

  it('handles negative and mixed magnitudes', () => {
    expect(neumaierSum([-1, -2, -3])).toBe(-6);
    expect(neumaierSum([1e100, 1, -1e100])).toBe(1);
    // 2.78e-17 is the exact sum of the three doubles, not an error to be hidden.
    expect(neumaierSum([0.1, 0.2, -0.3])).toBeCloseTo(0, 16);
  });

  it('accepts any iterable, including the lazy matrix walk', () => {
    const matrix: Matrix = [
      [1, 2],
      [3, 4],
    ];
    expect(neumaierSum(values(matrix))).toBe(10);
  });
});

describe('computeMatrixStatistics', () => {
  it('computes max, min, sum, average and count in one pass', () => {
    const stats = computeMatrixStatistics(
      [
        [2, 3],
        [0, 4],
      ],
      TOLERANCE,
    );
    expect(stats).toMatchObject({
      rows: 2,
      cols: 2,
      count: 4,
      max: 4,
      min: 0,
      sum: 9,
      average: 2.25,
      maxAbs: 4,
      isDiagonal: false,
    });
  });

  it('handles negative values and a single cell', () => {
    expect(computeMatrixStatistics([[-5]], TOLERANCE)).toMatchObject({
      max: -5,
      min: -5,
      sum: -5,
      average: -5,
      isDiagonal: true,
    });
  });

  it('rejects a matrix with no values', () => {
    expect(() => computeMatrixStatistics([], TOLERANCE)).toThrow(EmptyMatrixError);
    expect(() => computeMatrixStatistics([[]], TOLERANCE)).toThrow(EmptyMatrixError);
  });
});

describe('isDiagonal', () => {
  const cases: ReadonlyArray<readonly [string, Matrix, boolean]> = [
    ['the 3x3 identity', [[1, 0, 0], [0, 1, 0], [0, 0, 1]], true],
    ['a negative diagonal', [[-2, 0], [0, -7]], true],
    ['the zero matrix', [[0, 0], [0, 0]], true],
    ['a single cell', [[5]], true],
    ['a single row with off-diagonal values', [[1, 2, 3]], false],
    ['a 3x2 diagonal block', [[1, 0], [0, 2], [0, 0]], true],
    ['a 3x2 with an off-diagonal entry', [[1, 0], [3, 2], [0, 0]], false],
    ['a 2x3 diagonal block', [[1, 0, 0], [0, 2, 0]], true],
    ['a 2x3 with an off-diagonal entry', [[1, 0, 4], [0, 2, 0]], false],
    ['noise below the absolute tolerance', [[1, 1e-13], [0, 1]], true],
    ['noise above the relative tolerance', [[1, 1e-8], [0, 1]], false],
    ['QR-sized residue at scale 1e9', [[1e9, 0.5], [0, 1e9]], true],
    ['a real off-diagonal value at scale 1e9', [[1e9, 1e3], [0, 1e9]], false],
  ];

  it.each(cases)('is %s -> %o', (_name, matrix, expected) => {
    expect(isDiagonal(matrix, TOLERANCE)).toBe(expected);
    expect(computeMatrixStatistics(matrix, TOLERANCE).isDiagonal).toBe(expected);
  });


  it('applies the hybrid tolerance max(abs, rel * maxAbs)', () => {
    expect(diagonalThreshold(0, TOLERANCE)).toBe(1e-12);
    expect(diagonalThreshold(1e9, TOLERANCE)).toBe(1);
    expect(diagonalThreshold(1, TOLERANCE)).toBe(1e-9);
  });
});

describe('aggregate', () => {
  it('folds several matrices into the global summary', () => {
    const q = computeMatrixStatistics([[1, 0], [0, 1]], TOLERANCE);
    const r = computeMatrixStatistics([[2, 3], [0, 4]], TOLERANCE);
    expect(aggregate([q, r])).toEqual({
      max: 4,
      min: 0,
      sum: 11,
      average: 1.375,
      count: 8,
      anyDiagonal: true,
    });
  });

  it('reports anyDiagonal false when no matrix is diagonal', () => {
    const a = computeMatrixStatistics([[1, 2], [3, 4]], TOLERANCE);
    const b = computeMatrixStatistics([[5, 6], [7, 8]], TOLERANCE);
    expect(aggregate([a, b])).toMatchObject({ max: 8, min: 1, count: 8, anyDiagonal: false });
  });

  it('keeps the compensated total across matrices', () => {
    const a = computeMatrixStatistics([[1e16, 1]], TOLERANCE);
    const b = computeMatrixStatistics([[-1e16, 0]], TOLERANCE);
    expect(aggregate([a, b]).sum).toBe(1);
  });

  it('refuses to aggregate nothing', () => {
    expect(() => aggregate([])).toThrow(EmptyMatrixError);
  });
});
