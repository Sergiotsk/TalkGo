// TASK-068: Tests for MuteButton component

import React from 'react';
import { render, fireEvent } from '@testing-library/react-native';
import { MuteButton } from '../../src/components/MuteButton';

describe('MuteButton', () => {
  it('renders unmuted icon when isMuted=false', () => {
    const { getByTestId } = render(
      <MuteButton isMuted={false} onToggle={jest.fn()} />
    );
    expect(getByTestId('mute-button-unmuted')).toBeTruthy();
  });

  it('renders muted icon when isMuted=true', () => {
    const { getByTestId } = render(
      <MuteButton isMuted={true} onToggle={jest.fn()} />
    );
    expect(getByTestId('mute-button-muted')).toBeTruthy();
  });

  it('calls onToggle when pressed', () => {
    const onToggle = jest.fn();
    const { getByTestId } = render(
      <MuteButton isMuted={false} onToggle={onToggle} />
    );
    fireEvent.press(getByTestId('mute-button-unmuted'));
    expect(onToggle).toHaveBeenCalledTimes(1);
  });
});
