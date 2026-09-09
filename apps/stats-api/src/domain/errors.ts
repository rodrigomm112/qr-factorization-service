/** Domain failures. Which status code each maps to is the inbound adapter's decision. */
export class DomainError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = new.target.name;
  }
}

// Unreachable over HTTP: the request schema rejects empty input as `empty_matrix` / `empty_row`.
export class EmptyMatrixError extends DomainError {
  public constructor(message = 'statistics require at least one value') {
    super(message);
  }
}

// A saturated sum would serialize as `null` where the schema promises a number. The offending
// values are the caller's, so the adapter renders this as 422 `numerical_overflow`, never a 500.
export class NumericalOverflowError extends DomainError {
  public constructor(message = 'the sum of the values overflows the double range') {
    super(message);
  }
}
