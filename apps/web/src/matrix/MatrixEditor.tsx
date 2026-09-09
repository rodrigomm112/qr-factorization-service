import type { QrMode } from '../api/types';
import { PRESETS } from './presets';
import { MAX_DIMENSION, MIN_DIMENSION } from './useMatrixState';
import type { MatrixStateApi } from './useMatrixState';

interface StepperProps {
  readonly id: string;
  readonly label: string;
  /** Singular, for the button labels: "Añadir fila". */
  readonly unit: string;
  readonly value: number;
  readonly onChange: (value: number) => void;
}

function Stepper({ id, label, unit, value, onChange }: StepperProps) {
  return (
    <div className="stepper">
      <label htmlFor={id}>{label}</label>
      <div className="stepper-controls">
        <button
          type="button"
          className="icon-button"
          aria-label={`Quitar ${unit}`}
          disabled={value <= MIN_DIMENSION}
          onClick={() => { onChange(value - 1); }}
        >
          −
        </button>
        <input
          id={id}
          type="number"
          inputMode="numeric"
          className="stepper-value"
          min={MIN_DIMENSION}
          max={MAX_DIMENSION}
          step={1}
          value={value}
          onChange={(event) => {
            const parsed = Number.parseInt(event.target.value, 10);
            if (!Number.isNaN(parsed)) onChange(parsed);
          }}
        />
        <button
          type="button"
          className="icon-button"
          aria-label={`Añadir ${unit}`}
          disabled={value >= MAX_DIMENSION}
          onClick={() => { onChange(value + 1); }}
        >
          +
        </button>
      </div>
    </div>
  );
}

export interface MatrixEditorProps {
  readonly matrix: MatrixStateApi;
  readonly mode: QrMode;
  readonly onModeChange: (mode: QrMode) => void;
  readonly onSubmit: () => void;
  readonly pending: boolean;
}

/**
 * Always editable: the session is automatic, so there is nothing to wait for, and a request in
 * flight does not lock the grid either — App aborts the previous run and ignores its late
 * response (stale-response guard in App.tsx).
 */
export function MatrixEditor({ matrix, mode, onModeChange, onSubmit, pending }: MatrixEditorProps) {
  const { cells, rows, cols, presetId, issues } = matrix;

  const handleSubmit = (): void => {
    if (issues.length === 0) onSubmit();
  };

  return (
    <section className="panel" aria-labelledby="editor-title">
      <h2 id="editor-title">Matriz</h2>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          handleSubmit();
        }}
        aria-busy={pending}
      >
        <div className="editor-controls">
          <Stepper id="rows" label="Filas" unit="fila" value={rows} onChange={matrix.setRows} />
          <Stepper
            id="cols"
            label="Columnas"
            unit="columna"
            value={cols}
            onChange={matrix.setCols}
          />
          <div className="field">
            <label htmlFor="preset">Ejemplo</label>
            <select
              id="preset"
              value={presetId ?? ''}
              onChange={(event) => { matrix.applyPreset(event.target.value); }}
            >
              <option value="" disabled>
                Elige un ejemplo…
              </option>
              {PRESETS.map((preset) => (
                <option key={preset.id} value={preset.id}>
                  {preset.label}
                </option>
              ))}
            </select>
          </div>
          <fieldset className="field mode">
            <legend>Modo</legend>
            {(['full', 'reduced'] as const).map((option) => (
              <label key={option} className="radio">
                <input
                  type="radio"
                  name="mode"
                  value={option}
                  checked={mode === option}
                  onChange={() => { onModeChange(option); }}
                />
                {option === 'full' ? 'Completa (Q m×m)' : 'Reducida (Q m×k)'}
              </label>
            ))}
          </fieldset>
        </div>

        <div className="matrix-scroll">
          <table className="matrix-grid">
            <caption className="visually-hidden">
              Celdas de la matriz de entrada, {rows} filas por {cols} columnas
            </caption>
            <thead>
              <tr>
                <th scope="col">
                  <span className="visually-hidden">Fila</span>
                </th>
                {cells[0]?.map((_cell, colIndex) => (
                  <th scope="col" key={colIndex}>
                    c{colIndex}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {cells.map((row, rowIndex) => (
                <tr key={rowIndex}>
                  <th scope="row">f{rowIndex}</th>
                  {row.map((cell, colIndex) => (
                    <td key={colIndex}>
                      <input
                        type="text"
                        inputMode="decimal"
                        autoComplete="off"
                        spellCheck={false}
                        className="cell"
                        aria-label={`Fila ${rowIndex}, columna ${colIndex}`}
                        value={cell}
                        onChange={(event) => {
                          matrix.setCell(rowIndex, colIndex, event.target.value);
                        }}
                        onFocus={(event) => { event.target.select(); }}
                      />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {issues.length > 0 && (
          <ul className="issues" aria-label="Errores de validación local">
            {issues.map((issue) => (
              <li key={issue.pointer}>
                <code>{issue.pointer}</code> <span className="code">{issue.code}</span>{' '}
                {issue.message}
              </li>
            ))}
          </ul>
        )}

        <div className="editor-footer">
          <p className="hint">
            Puedes editar matrices de hasta {MAX_DIMENSION} × {MAX_DIMENSION}
          </p>
          <button type="submit" className="primary" disabled={issues.length > 0}>
            {pending ? 'Calculando…' : 'Calcular QR'}
          </button>
        </div>
      </form>
    </section>
  );
}
