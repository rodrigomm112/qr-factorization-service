import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MatrixEditor } from './MatrixEditor';
import { useMatrixState } from './useMatrixState';

function Harness({ onSubmit = vi.fn() }: { onSubmit?: () => void }) {
  const matrix = useMatrixState();
  return (
    <MatrixEditor
      matrix={matrix}
      mode="full"
      onModeChange={vi.fn()}
      onSubmit={onSubmit}
      pending={false}
    />
  );
}

const cells = (): HTMLElement[] => screen.getAllByLabelText(/^Fila \d+, columna \d+$/);

describe('MatrixEditor', () => {
  it('renders one input per cell of the default 3x2 matrix', () => {
    render(<Harness />);

    expect(cells()).toHaveLength(6);
    expect(screen.getByLabelText('Fila 2, columna 1')).toHaveValue('6');
    expect(screen.getByRole('spinbutton', { name: 'Filas' })).toHaveValue(3);
    expect(screen.getByRole('button', { name: 'Calcular QR' })).toBeEnabled();
  });

  it('grows and shrinks the grid from the steppers, keeping the edited values', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getByRole('button', { name: 'Añadir fila' }));
    expect(cells()).toHaveLength(8);
    expect(screen.getByLabelText('Fila 3, columna 0')).toHaveValue('0');

    await user.click(screen.getByRole('button', { name: 'Añadir columna' }));
    expect(cells()).toHaveLength(12);

    await user.click(screen.getByRole('button', { name: 'Quitar fila' }));
    expect(cells()).toHaveLength(9);
    expect(screen.getByLabelText('Fila 0, columna 1')).toHaveValue('2');
  });

  it('blocks the submit button and lists the pointer while a cell is invalid', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<Harness onSubmit={onSubmit} />);

    await user.clear(screen.getByLabelText('Fila 0, columna 0'));

    const submit = screen.getByRole('button', { name: 'Calcular QR' });
    expect(submit).toBeDisabled();
    expect(screen.getByText('/matrix/0/0')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
