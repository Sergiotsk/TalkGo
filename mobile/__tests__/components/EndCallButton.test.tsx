// TASK-072: Tests for EndCallButton component

import React from 'react';
import { render, fireEvent } from '@testing-library/react-native';
import { EndCallButton } from '../../src/components/EndCallButton';

describe('EndCallButton', () => {
  it('renders "Finalizar" button', () => {
    const { getByText } = render(<EndCallButton onConfirm={jest.fn()} />);
    expect(getByText('Finalizar')).toBeTruthy();
  });

  it('shows confirmation dialog when pressed', () => {
    const { getByText, queryByText } = render(
      <EndCallButton onConfirm={jest.fn()} />
    );

    expect(queryByText('Cancelar')).toBeNull();

    fireEvent.press(getByText('Finalizar'));
    expect(getByText('Cancelar')).toBeTruthy();
    expect(getByText('Confirmar')).toBeTruthy();
  });

  it('closes dialog on cancel without calling onConfirm', () => {
    const onConfirm = jest.fn();
    const { getByText, queryByText } = render(
      <EndCallButton onConfirm={onConfirm} />
    );

    fireEvent.press(getByText('Finalizar'));
    fireEvent.press(getByText('Cancelar'));

    expect(onConfirm).not.toHaveBeenCalled();
    expect(queryByText('Cancelar')).toBeNull();
  });

  it('calls onConfirm when confirmed', () => {
    const onConfirm = jest.fn();
    const { getByText } = render(<EndCallButton onConfirm={onConfirm} />);

    fireEvent.press(getByText('Finalizar'));
    fireEvent.press(getByText('Confirmar'));

    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
