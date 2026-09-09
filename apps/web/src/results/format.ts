/** Number formatting shared by every result view. */

// Fallback for a response with no tolerance: the views pass the server's own
// `meta.diagonalTolerance.absolute` so the grid's "0" cannot drift from its diagonality verdict.
export const ZERO_TOLERANCE = 1e-12;

export const DECIMALS = 6;

export const isDisplayZero = (
  value: number,
  tolerance: number = ZERO_TOLERANCE,
): boolean => Number.isFinite(value) && Math.abs(value) <= tolerance;

/** `raw` is what the API sent; otherwise 6 decimals, exponential where fixed notation lies. */
export function formatNumber(
  value: number,
  raw = false,
  tolerance: number = ZERO_TOLERANCE,
): string {
  if (!Number.isFinite(value)) return String(value);
  if (raw) return String(value);
  if (isDisplayZero(value, tolerance)) return "0";
  const magnitude = Math.abs(value);
  if (magnitude >= 1e7 || magnitude < 1e-4)
    return value.toExponential(DECIMALS);
  return value.toFixed(DECIMALS);
}

/** Stable width, e.g. `0.041 ms`. */
export const formatMs = (value: number): string => `${value.toFixed(3)} ms`;

export const formatShape = (rows: number, cols: number): string =>
  `${rows}×${cols}`;
