import { describe, expect, it } from 'vitest';
import { computeStatistics } from '../../src/application/compute-statistics.usecase.js';
import { EmptyMatrixError } from '../../src/domain/errors.js';

const TOLERANCE = { absTol: 1e-12, relTol: 1e-9 };

describe('computeStatistics', () => {
  it('produces the summary, the per-matrix breakdown and the meta block', () => {
    let ticks = 0;
    const result = computeStatistics(
      [
        { label: 'Q', values: [[1, 0], [0, 1]] },
        { label: 'R', values: [[2, 3], [0, 4]] },
      ],
      { tolerance: TOLERANCE, clock: () => (ticks += 0.5) },
    );

    expect(result.summary).toEqual({
      max: 4,
      min: 0,
      sum: 11,
      average: 1.375,
      count: 8,
      anyDiagonal: true,
    });
    expect(result.matrices).toEqual([
      { index: 0, label: 'Q', rows: 2, cols: 2, max: 1, min: 0, sum: 2, average: 0.5, isDiagonal: true },
      { index: 1, label: 'R', rows: 2, cols: 2, max: 4, min: 0, sum: 9, average: 2.25, isDiagonal: false },
    ]);
    expect(result.meta).toEqual({
      summationAlgorithm: 'neumaier',
      diagonalTolerance: { absolute: 1e-12, relative: 1e-9 },
      elapsedMs: 0.5,
    });
  });

  it('measures elapsed time with the real clock by default', () => {
    const result = computeStatistics([{ label: 'm', values: [[1]] }], { tolerance: TOLERANCE });
    expect(result.meta.elapsedMs).toBeGreaterThanOrEqual(0);
    expect(Number.isFinite(result.meta.elapsedMs)).toBe(true);
  });

  it('handles rectangular matrices of different shapes in one request', () => {
    const result = computeStatistics(
      [
        { label: 'tall', values: [[1], [2], [3]] },
        { label: 'wide', values: [[4, 5, 6]] },
      ],
      { tolerance: TOLERANCE },
    );
    expect(result.summary).toMatchObject({ max: 6, min: 1, sum: 21, count: 6, anyDiagonal: false });
    expect(result.matrices.map((m) => [m.rows, m.cols])).toEqual([
      [3, 1],
      [1, 3],
    ]);
  });

  it('refuses an empty batch', () => {
    expect(() => computeStatistics([], { tolerance: TOLERANCE })).toThrow(EmptyMatrixError);
  });
});
