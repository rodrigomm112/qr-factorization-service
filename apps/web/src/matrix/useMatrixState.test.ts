import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MAX_DIMENSION, useMatrixState } from './useMatrixState';

describe('useMatrixState', () => {
  it('starts on the tall 3x2 preset from the shared fixtures', () => {
    const { result } = renderHook(() => useMatrixState());

    expect(result.current.rows).toBe(3);
    expect(result.current.cols).toBe(2);
    expect(result.current.values).toEqual([
      [1, 2],
      [3, 4],
      [5, 6],
    ]);
    expect(result.current.issues).toHaveLength(0);
  });

  it('preserves the edited cells when growing and pads the new ones with 0', () => {
    const { result } = renderHook(() => useMatrixState());

    act(() => { result.current.setCell(0, 1, '9.5'); });
    act(() => { result.current.setRows(4); });
    act(() => { result.current.setCols(3); });

    expect(result.current.values).toEqual([
      [1, 9.5, 0],
      [3, 4, 0],
      [5, 6, 0],
      [0, 0, 0],
    ]);
  });

  it('drops the trailing rows when shrinking and clamps to the editor bounds', () => {
    const { result } = renderHook(() => useMatrixState());

    act(() => { result.current.setRows(1); });
    expect(result.current.values).toEqual([[1, 2]]);

    act(() => { result.current.setRows(0); });
    expect(result.current.rows).toBe(1);

    act(() => { result.current.setRows(999); });
    expect(result.current.rows).toBe(MAX_DIMENSION);
  });

  it('applies a preset, replacing every cell and remembering the selection', () => {
    const { result } = renderHook(() => useMatrixState());

    act(() => { result.current.applyPreset('identity-3x3'); });

    expect(result.current.presetId).toBe('identity-3x3');
    expect(result.current.values).toEqual([
      [1, 0, 0],
      [0, 1, 0],
      [0, 0, 1],
    ]);

    act(() => { result.current.setCell(0, 0, '7'); });
    expect(result.current.presetId).toBeNull();
  });

  it('mirrors the server contract for a cell that is not a finite number', () => {
    const { result } = renderHook(() => useMatrixState());

    act(() => { result.current.setCell(1, 0, 'abc'); });
    act(() => { result.current.setCell(2, 1, '1e999'); });

    expect(result.current.values).toBeNull();
    expect(result.current.issues).toEqual([
      { pointer: '/matrix/1/0', code: 'invalid_type', message: '"abc" no es un número.' },
      { pointer: '/matrix/2/1', code: 'non_finite_value', message: 'El valor no es finito.' },
    ]);
  });
});
