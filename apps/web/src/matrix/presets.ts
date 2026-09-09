/** Ready-made inputs that exercise the interesting branches of the decomposition. */
export interface Preset {
  readonly id: string;
  readonly label: string;
  /** Fixed presets ignore the arguments; `random` reuses the current size. */
  build(rows: number, cols: number): number[][];
}

const fixed = (id: string, label: string, values: readonly (readonly number[])[]): Preset => ({
  id,
  label,
  build: () => values.map((row) => [...row]),
});

/** Two decimals in [-9, 9]: readable in the grid and exact in binary64 division. */
const randomCell = (): number => Math.round((Math.random() * 18 - 9) * 100) / 100;

export const PRESETS: readonly Preset[] = [
  fixed('identity-3x3', 'Identidad · 3 × 3', [
    [1, 0, 0],
    [0, 1, 0],
    [0, 0, 1],
  ]),
  fixed('diagonal-3x3', 'Diagonal · 3 × 3', [
    [2, 0, 0],
    [0, -3, 0],
    [0, 0, 4],
  ]),
  fixed('tall-3x2', 'Rectangular · 3 × 2', [
    [1, 2],
    [3, 4],
    [5, 6],
  ]),
  fixed('wide-2x3', 'Rectangular · 2 × 3', [
    [1, 2, 3],
    [4, 5, 6],
  ]),
  fixed('rank-deficient-3x2', 'Rango deficiente · 3 × 2', [
    [1, 2],
    [2, 4],
    [3, 6],
  ]),
  {
    id: 'random',
    label: 'Aleatoria',
    build: (rows, cols) =>
      Array.from({ length: rows }, () => Array.from({ length: cols }, randomCell)),
  },
];

export const DEFAULT_PRESET_ID = 'tall-3x2';

export const findPreset = (id: string): Preset | undefined =>
  PRESETS.find((preset) => preset.id === id);
