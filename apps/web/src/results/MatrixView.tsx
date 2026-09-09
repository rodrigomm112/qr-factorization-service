import type { Matrix } from '../api/types';
import { CopyButton } from './CopyButton';
import { formatNumber, isDisplayZero, formatShape } from './format';

export interface MatrixViewProps {
  /** Short name used in the caption and in the copy button's accessible name: `A`, `Q`, `R`. */
  readonly title: string;
  readonly values: Matrix;
  readonly raw: boolean;
  readonly caption?: string;
  /** `meta.diagonalTolerance.absolute`: entries at or below it render as `0`. */
  readonly zeroTolerance?: number;
}

export function MatrixView({ title, values, raw, caption, zeroTolerance }: MatrixViewProps) {
  const rows = values.length;
  const cols = values[0]?.length ?? 0;

  return (
    <figure className="matrix-figure">
      <figcaption>
        <span className="matrix-name">{title}</span>{' '}
        <span className="shape mono">{formatShape(rows, cols)}</span>
        {caption !== undefined && <span className="hint"> · {caption}</span>}
        {/* Raw values, never the rounded display form: what is copied round-trips into an API. */}
        <CopyButton text={JSON.stringify(values)} label={`Copiar ${title} en JSON`} />
      </figcaption>
      <div className="matrix-scroll">
        <table className="matrix-grid readonly">
          <tbody>
            {values.map((row, rowIndex) => (
              <tr key={rowIndex}>
                {row.map((value, colIndex) => (
                  <td
                    key={colIndex}
                    className={isDisplayZero(value, zeroTolerance) ? 'num zero' : 'num'}
                    title={String(value)}
                  >
                    {formatNumber(value, raw, zeroTolerance)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </figure>
  );
}
