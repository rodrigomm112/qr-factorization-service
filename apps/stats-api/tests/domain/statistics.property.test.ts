import fc from 'fast-check';
import { describe, expect, it } from 'vitest';
import type { Matrix } from '../../src/domain/matrix.js';
import {
  aggregate,
  computeMatrixStatistics,
  diagonalThreshold,
  NeumaierSummation,
} from '../../src/domain/statistics.js';
import type { DiagonalTolerance } from '../../src/domain/statistics.js';

// 200 cases already shrink to a minimal counterexample, and this suite runs on every commit.
const RUNS = { numRuns: 200 } as const;

// Test-only shims: production never sums a flat list nor asks isDiagonal on its own.
function neumaierSum(numbers: Iterable<number>): number {
  const accumulator = new NeumaierSummation();
  for (const value of numbers) accumulator.add(value);
  return accumulator.total;
}
function isDiagonal(matrix: Matrix, tolerance: DiagonalTolerance): boolean {
  return computeMatrixStatistics(matrix, tolerance).isDiagonal;
}

const TOLERANCE: DiagonalTolerance = { absTol: 1e-12, relTol: 1e-9 };

// `m * 2^e` with a 21-bit mantissa is exact, and dyadic terms share `2^min(e)`, so BigInt sums
// them without rounding — that is what makes an integer reference possible.
interface Dyadic {
  readonly mantissa: number;
  readonly exponent: number;
}

const toDouble = (term: Dyadic): number => term.mantissa * 2 ** term.exponent;

const smallTerm = fc.record<Dyadic>({
  mantissa: fc.integer({ min: -(2 ** 20), max: 2 ** 20 }),
  exponent: fc.integer({ min: -8, max: 8 }),
});

const hugeTerm = fc.record<Dyadic>({
  mantissa: fc.integer({ min: 1, max: 2 ** 20 }),
  exponent: fc.integer({ min: 30, max: 70 }),
});

// A huge term, its exact negation and small ones, shuffled; the naive loop loses the small part.
const cancellingTerms = fc
  .tuple(hugeTerm, fc.array(smallTerm, { minLength: 1, maxLength: 8 }))
  .chain(([huge, smalls]) => {
    const terms: Dyadic[] = [huge, { ...huge, mantissa: -huge.mantissa }, ...smalls];
    return fc.shuffledSubarray(terms, { minLength: terms.length, maxLength: terms.length });
  });

function exactSum(terms: readonly Dyadic[]): { numerator: bigint; shift: number } {
  const shift = Math.min(...terms.map((term) => term.exponent));
  let numerator = 0n;
  for (const term of terms) {
    numerator += BigInt(term.mantissa) << BigInt(term.exponent - shift);
  }
  return { numerator, shift };
}

function significantBits(value: bigint): number {
  if (value === 0n) return 0;
  const magnitude = value < 0n ? -value : value;
  return magnitude.toString(2).replace(/0+$/, '').length;
}

/** Upper bound of one unit in the last place of `value` (exact within a factor of two). */
const ulp = (value: number): number => Math.abs(value) * Number.EPSILON;

const wellConditioned = fc.array(fc.double({ min: 1, max: 1e6, noNaN: true }), {
  minLength: 1,
  maxLength: 64,
});

const nonZero = fc.oneof(
  fc.double({ min: 1e-6, max: 1e6, noNaN: true }),
  fc.double({ min: -1e6, max: -1e-6, noNaN: true }),
);

function diag(vector: readonly number[]): number[][] {
  return vector.map((value, row) => vector.map((_, col) => (row === col ? value : 0)));
}

const cell = fc.double({ min: -1e9, max: 1e9, noNaN: true });

const rectangular = fc
  .integer({ min: 1, max: 6 })
  .chain((cols) =>
    fc.array(fc.array(cell, { minLength: cols, maxLength: cols }), { minLength: 1, maxLength: 6 }),
  );

describe('numeric properties', () => {
  it('sums dyadic rationals exactly whenever the result is representable', () => {
    fc.assert(
      fc.property(cancellingTerms, (terms) => {
        const { numerator, shift } = exactSum(terms);
        // The construction keeps the total under 53 bits; this documents that, it does not filter.
        fc.pre(significantBits(numerator) <= 53);
        const exact = Number(numerator) * 2 ** shift;

        expect(neumaierSum(terms.map(toDouble))).toBe(exact);
      }),
      RUNS,
    );
  });

  it('is permutation invariant within one ulp for well-conditioned inputs', () => {
    fc.assert(
      fc.property(
        wellConditioned.chain((values) =>
          fc.tuple(
            fc.constant(values),
            fc.shuffledSubarray(values, { minLength: values.length, maxLength: values.length }),
          ),
        ),
        ([values, shuffled]) => {
          const sum = neumaierSum(values);
          const permuted = neumaierSum(shuffled);

          expect(Math.abs(sum - permuted)).toBeLessThanOrEqual(ulp(Math.max(sum, permuted)));
        },
      ),
      RUNS,
    );
  });

  it('reports every diagonal matrix as diagonal', () => {
    fc.assert(
      fc.property(fc.array(nonZero, { minLength: 1, maxLength: 8 }), (vector) => {
        expect(isDiagonal(diag(vector), TOLERANCE)).toBe(true);
      }),
      RUNS,
    );
  });

  it('rejects a matrix with a single off-diagonal entry above the tolerance', () => {
    const perturbed = fc
      .array(nonZero, { minLength: 2, maxLength: 8 })
      .chain((vector) =>
        fc.tuple(
          fc.constant(vector),
          fc.nat({ max: vector.length - 1 }),
          fc.nat({ max: vector.length - 2 }),
          fc.double({ min: 2, max: 1000, noNaN: true }),
        ),
      );

    fc.assert(
      fc.property(perturbed, ([vector, row, offset, factor]) => {
        // `offset` addresses the columns other than `row`, so the entry is never on the diagonal.
        const col = offset >= row ? offset + 1 : offset;
        const matrix = diag(vector);
        const maxAbs = Math.max(...vector.map(Math.abs));
        const threshold = diagonalThreshold(maxAbs, TOLERANCE);
        // Above the tolerance yet far below `maxAbs`, so the threshold itself does not move.
        const entry = threshold * factor;
        matrix[row]![col] = entry;

        expect(entry).toBeGreaterThan(diagonalThreshold(Math.max(maxAbs, entry), TOLERANCE));
        expect(isDiagonal(matrix, TOLERANCE)).toBe(false);
      }),
      RUNS,
    );
  });

  it('aggregates count, max and min like the trivial reference', () => {
    fc.assert(
      fc.property(fc.array(rectangular, { minLength: 1, maxLength: 4 }), (matrices) => {
        const summary = aggregate(
          matrices.map((matrix: Matrix) => computeMatrixStatistics(matrix, TOLERANCE)),
        );
        const flat = matrices.flatMap((matrix) => matrix.flat());

        expect(summary.count).toBe(flat.length);
        expect(summary.max).toBe(Math.max(...flat));
        expect(summary.min).toBe(Math.min(...flat));
      }),
      RUNS,
    );
  });
});
