import { z } from 'zod';
import type { LabeledMatrix } from '../../../../application/compute-statistics.usecase.js';
import { ProblemError, type ValidationCode, type ValidationIssue } from '../problem.js';

export interface ValidationLimits {
  readonly maxMatrices: number;
  readonly maxTotalElements: number;
  readonly maxMatrixRows: number;
  readonly maxMatrixCols: number;
}

// Bounded input (8 x 100 x 100) can still fail 80 000 times; echoing all of it would turn a bad
// request into a multi-megabyte response.
const MAX_REPORTED_ISSUES = 200;

// Zod checks the envelope only. The contract fixes the exact `code` and RFC 6901 `pointer` of every
// failure below it, and mapping Zod's issue tree onto that would be more code, not less.
const envelopeSchema = z.object({ matrices: z.array(z.unknown()) });

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function plural(count: number, singular: string): string {
  return count === 1 ? singular : `${singular}s`;
}

function typeNameOf(value: unknown): string {
  if (value === null) return 'null';
  if (Array.isArray(value)) return 'an array';
  switch (typeof value) {
    case 'string':
      return 'a string';
    case 'boolean':
      return 'a boolean';
    case 'object':
      return 'an object';
    case 'undefined':
      return 'null';
    default:
      return `a ${typeof value}`;
  }
}

/** `/matrices/0/values` -> `matrices[0].values`, for the `detail` sentence. */
function dottedPath(pointer: string): string {
  const segments = pointer.split('/').slice(1);
  let path = '';
  for (const segment of segments) {
    if (/^\d+$/.test(segment)) {
      path += `[${segment}]`;
    } else {
      path = path.length === 0 ? segment : `${path}.${segment}`;
    }
  }
  return path.length === 0 ? 'the request body' : path;
}

function parentPointer(pointer: string): string {
  return pointer.slice(0, Math.max(0, pointer.lastIndexOf('/')));
}

/** Numeric-aware comparison so `/matrices/2` sorts before `/matrices/10`. */
function comparePointers(a: string, b: string): number {
  const left = a.split('/');
  const right = b.split('/');
  const shared = Math.min(left.length, right.length);
  for (let i = 0; i < shared; i += 1) {
    const x = left[i] ?? '';
    const y = right[i] ?? '';
    if (x === y) continue;
    const bothNumeric = /^\d+$/.test(x) && /^\d+$/.test(y);
    // Byte order, not locale: qr-api sorts these pointers with strings.Compare.
    return bothNumeric ? Number(x) - Number(y) : x < y ? -1 : x > y ? 1 : 0;
  }
  return left.length - right.length;
}

class IssueCollector {
  private readonly issues: ValidationIssue[] = [];
  private total = 0;

  public add(pointer: string, code: ValidationCode, message: string): void {
    this.total += 1;
    if (this.issues.length < MAX_REPORTED_ISSUES) {
      this.issues.push({ pointer, code, message });
    }
  }

  public get empty(): boolean {
    return this.total === 0;
  }

  public toProblem(): ProblemError {
    const sorted = [...this.issues].sort((a, b) => comparePointers(a.pointer, b.pointer));
    return new ProblemError('validation-error', { detail: this.detail(sorted), issues: sorted });
  }

  private detail(sorted: readonly ValidationIssue[]): string {
    const first = sorted[0];
    if (first === undefined) return 'The request body failed validation.';
    if (this.total > sorted.length) {
      return `The request body contains ${String(this.total)} validation errors; the first ${String(sorted.length)} are listed.`;
    }
    if (this.total > 1) {
      return `The request body contains ${String(this.total)} validation errors.`;
    }
    return first.code === 'ragged_row'
      ? `${dottedPath(parentPointer(first.pointer))} must be rectangular: ${first.message}`
      : first.message;
  }
}

/** Records every failure instead of throwing, so one response can carry them all. */
function validateMatrix(
  value: unknown,
  pointer: string,
  limits: ValidationLimits,
  issues: IssueCollector,
): number[][] | undefined {
  const label = dottedPath(pointer);

  if (!Array.isArray(value)) {
    issues.add(pointer, 'invalid_type', `${label} must be an array of rows, received ${typeNameOf(value)}`);
    return undefined;
  }
  if (value.length === 0) {
    issues.add(pointer, 'empty_matrix', `${label} must have at least one row`);
    return undefined;
  }
  if (value.length > limits.maxMatrixRows) {
    issues.add(
      pointer,
      'too_many_rows',
      `${label} has ${String(value.length)} rows, at most ${String(limits.maxMatrixRows)} are allowed`,
    );
    return undefined;
  }

  const firstRow: unknown = value[0];
  if (!Array.isArray(firstRow)) {
    issues.add(`${pointer}/0`, 'invalid_type', `row 0 must be an array of numbers, received ${typeNameOf(firstRow)}`);
    return undefined;
  }
  if (firstRow.length === 0) {
    issues.add(`${pointer}/0`, 'empty_row', 'row 0 must have at least one column');
    return undefined;
  }
  if (firstRow.length > limits.maxMatrixCols) {
    issues.add(
      `${pointer}/0`,
      'too_many_cols',
      `row 0 has ${String(firstRow.length)} columns, at most ${String(limits.maxMatrixCols)} are allowed`,
    );
    return undefined;
  }

  const cols = firstRow.length;
  const matrix: number[][] = [];
  let valid = true;

  for (let i = 0; i < value.length; i += 1) {
    const row: unknown = value[i];
    const rowPointer = `${pointer}/${String(i)}`;

    if (!Array.isArray(row)) {
      issues.add(rowPointer, 'invalid_type', `row ${String(i)} must be an array of numbers, received ${typeNameOf(row)}`);
      valid = false;
      continue;
    }
    if (row.length === 0) {
      issues.add(rowPointer, 'empty_row', `row ${String(i)} must have at least one column`);
      valid = false;
      continue;
    }
    if (row.length !== cols) {
      issues.add(
        rowPointer,
        'ragged_row',
        `row ${String(i)} has ${String(row.length)} ${plural(row.length, 'column')}, expected ${String(cols)}`,
      );
      valid = false;
    }

    for (let j = 0; j < row.length; j += 1) {
      const cell: unknown = row[j];
      if (typeof cell !== 'number') {
        issues.add(
          `${rowPointer}/${String(j)}`,
          'invalid_type',
          `row ${String(i)}, column ${String(j)} must be a number, received ${typeNameOf(cell)}`,
        );
        valid = false;
      } else if (!Number.isFinite(cell)) {
        issues.add(
          `${rowPointer}/${String(j)}`,
          'non_finite_value',
          `row ${String(i)}, column ${String(j)} must be a finite number`,
        );
        valid = false;
      }
    }

    if (valid) {
      // Every cell checked immediately above.
      matrix.push(row as number[]);
    }
  }

  return valid ? matrix : undefined;
}

/** Canonical item `{ label?, values }`; returns the matrix when it is valid. */
function validateLabeledItem(
  item: Record<string, unknown>,
  index: number,
  limits: ValidationLimits,
  issues: IssueCollector,
): LabeledMatrix | undefined {
  const pointer = `/matrices/${String(index)}`;
  let label = `matrix-${String(index)}`;

  const rawLabel: unknown = item['label'];
  if (rawLabel !== undefined) {
    if (typeof rawLabel !== 'string') {
      issues.add(`${pointer}/label`, 'invalid_type', `label must be a string, received ${typeNameOf(rawLabel)}`);
    } else if (rawLabel.length > 64) {
      issues.add(`${pointer}/label`, 'invalid_type', 'label must be at most 64 characters');
    } else {
      label = rawLabel;
    }
  }

  if (!('values' in item) || item['values'] === undefined) {
    issues.add(`${pointer}/values`, 'missing_field', `${dottedPath(pointer)}.values is required`);
    return undefined;
  }

  const values = validateMatrix(item['values'], `${pointer}/values`, limits, issues);
  return values === undefined ? undefined : { label, values };
}

/** Accepts `{label, values}` and the `number[][]` shorthand, labelled `matrix-<index>`. */
export function parseStatisticsRequest(body: unknown, limits: ValidationLimits): LabeledMatrix[] {
  const issues = new IssueCollector();
  // `express.json({ strict: true })` passes only objects and arrays; no body arrives `undefined`.
  const envelope = envelopeSchema.safeParse(body ?? {});

  if (!envelope.success) {
    if (body !== undefined && !isRecord(body)) {
      issues.add('', 'invalid_type', 'the request body must be a JSON object');
    } else if (body === undefined || body['matrices'] === undefined) {
      issues.add('/matrices', 'missing_field', 'matrices is required');
    } else {
      issues.add(
        '/matrices',
        'invalid_type',
        `matrices must be an array of matrices, received ${typeNameOf(body['matrices'])}`,
      );
    }
    throw issues.toProblem();
  }

  const items = envelope.data.matrices;

  if (items.length === 0) {
    issues.add('/matrices', 'empty_matrix', 'matrices must contain at least one matrix');
    throw issues.toProblem();
  }
  if (items.length > limits.maxMatrices) {
    issues.add(
      '/matrices',
      'too_many_matrices',
      `matrices contains ${String(items.length)} matrices, at most ${String(limits.maxMatrices)} are allowed`,
    );
    throw issues.toProblem();
  }

  const normalized: LabeledMatrix[] = [];
  for (let index = 0; index < items.length; index += 1) {
    const item: unknown = items[index];
    if (Array.isArray(item)) {
      const values = validateMatrix(item, `/matrices/${String(index)}`, limits, issues);
      if (values !== undefined) normalized.push({ label: `matrix-${String(index)}`, values });
    } else if (isRecord(item)) {
      const labeled = validateLabeledItem(item, index, limits, issues);
      if (labeled !== undefined) normalized.push(labeled);
    } else {
      issues.add(
        `/matrices/${String(index)}`,
        'invalid_type',
        `matrices[${String(index)}] must be an object with "values" or an array of rows, received ${typeNameOf(item)}`,
      );
    }
  }

  if (!issues.empty) {
    throw issues.toProblem();
  }

  const totalElements = normalized.reduce(
    (total, matrix) => total + matrix.values.length * (matrix.values[0]?.length ?? 0),
    0,
  );
  if (totalElements > limits.maxTotalElements) {
    issues.add(
      '/matrices',
      'too_many_elements',
      `matrices contain ${String(totalElements)} elements in total, at most ${String(limits.maxTotalElements)} are allowed`,
    );
    throw issues.toProblem();
  }

  return normalized;
}
