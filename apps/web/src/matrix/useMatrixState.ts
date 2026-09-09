// Cells stay raw strings so half-typed values (`-`, `1.`, `1e`) survive a re-render; the matrix
// and the issues are derived. Issues use the server's own `pointer`/`code`, but the server stays
// the authority: this only avoids a round-trip that is certain to fail.
import { useCallback, useMemo, useState } from 'react';
import type { ValidationIssue } from '../api/types';
import { DEFAULT_PRESET_ID, findPreset } from './presets';

/** The APIs accept up to 100×100; the editor caps at 20×20 to stay readable. */
export const MIN_DIMENSION = 1;
export const MAX_DIMENSION = 20;
export const SERVER_MAX_DIMENSION = 100;

export type Cells = readonly (readonly string[])[];

const clampDimension = (value: number): number =>
  Math.min(MAX_DIMENSION, Math.max(MIN_DIMENSION, Math.trunc(value)));

/** The shortest round-trippable form: `2` -> `"2"`, `-0.5` -> `"-0.5"`. */
const formatCell = (value: number): string => String(value);

export const toCells = (values: readonly (readonly number[])[]): Cells =>
  values.map((row) => row.map(formatCell));

function resize(cells: Cells, rows: number, cols: number): Cells {
  return Array.from({ length: rows }, (_unused, rowIndex) =>
    Array.from({ length: cols }, (_ignored, colIndex) => cells[rowIndex]?.[colIndex] ?? '0'),
  );
}

interface Derived {
  readonly values: number[][] | null;
  readonly issues: readonly ValidationIssue[];
}

function derive(cells: Cells): Derived {
  const issues: ValidationIssue[] = [];
  const values: number[][] = [];
  cells.forEach((row, rowIndex) => {
    const parsedRow: number[] = [];
    row.forEach((cell, colIndex) => {
      const pointer = `/matrix/${rowIndex}/${colIndex}`;
      const text = cell.trim();
      if (text === '') {
        issues.push({ pointer, code: 'invalid_type', message: 'La celda está vacía.' });
        parsedRow.push(Number.NaN);
        return;
      }
      const parsed = Number(text);
      if (Number.isNaN(parsed)) {
        issues.push({ pointer, code: 'invalid_type', message: `"${cell}" no es un número.` });
      } else if (!Number.isFinite(parsed)) {
        issues.push({ pointer, code: 'non_finite_value', message: 'El valor no es finito.' });
      }
      parsedRow.push(parsed);
    });
    values.push(parsedRow);
  });
  return { values: issues.length === 0 ? values : null, issues };
}

export interface MatrixStateApi {
  readonly cells: Cells;
  readonly rows: number;
  readonly cols: number;
  /** Last preset applied, or `null` once a cell or a dimension changed. */
  readonly presetId: string | null;
  /** The matrix to send, or `null` while `issues` is not empty. */
  readonly values: number[][] | null;
  readonly issues: readonly ValidationIssue[];
  setRows: (rows: number) => void;
  setCols: (cols: number) => void;
  setCell: (row: number, col: number, value: string) => void;
  applyPreset: (presetId: string) => void;
}

export function useMatrixState(initialPresetId: string = DEFAULT_PRESET_ID): MatrixStateApi {
  const [cells, setCells] = useState<Cells>(() =>
    toCells(findPreset(initialPresetId)?.build(3, 2) ?? [[1, 2], [3, 4], [5, 6]]),
  );
  const [presetId, setPresetId] = useState<string | null>(initialPresetId);

  const rows = cells.length;
  const cols = cells[0]?.length ?? 0;

  const setRows = useCallback((next: number) => {
    setPresetId(null);
    setCells((current) => resize(current, clampDimension(next), current[0]?.length ?? 1));
  }, []);

  const setCols = useCallback((next: number) => {
    setPresetId(null);
    setCells((current) => resize(current, current.length, clampDimension(next)));
  }, []);

  const setCell = useCallback((row: number, col: number, value: string) => {
    setPresetId(null);
    setCells((current) =>
      current.map((currentRow, rowIndex) =>
        rowIndex === row
          ? currentRow.map((cell, colIndex) => (colIndex === col ? value : cell))
          : currentRow,
      ),
    );
  }, []);

  const applyPreset = useCallback((id: string) => {
    const preset = findPreset(id);
    if (preset === undefined) return;
    setPresetId(id);
    setCells((current) =>
      toCells(preset.build(current.length, current[0]?.length ?? 1)),
    );
  }, []);

  const { values, issues } = useMemo(() => derive(cells), [cells]);

  return { cells, rows, cols, presetId, values, issues, setRows, setCols, setCell, applyPreset };
}
