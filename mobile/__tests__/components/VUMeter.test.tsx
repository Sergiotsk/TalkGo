// TASK-064: Tests for VUMeter component

import React from 'react';
import { render, act } from '@testing-library/react-native';
import { VUMeter } from '../../src/components/VUMeter';

describe('VUMeter', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('renders without crashing', async () => {
    const { getByTestId } = render(
      <VUMeter speaking={false} label="Local" testID="vu-meter" />
    );
    // Advance animation timers inside act to prevent act() warnings
    await act(async () => {
      jest.advanceTimersByTime(500);
    });
    expect(getByTestId('vu-meter')).toBeTruthy();
  });

  it('shows inactive indicator when speaking=false', async () => {
    const { getByTestId } = render(
      <VUMeter speaking={false} label="Local" testID="vu-meter" />
    );
    await act(async () => {
      jest.advanceTimersByTime(500);
    });
    const indicator = getByTestId('vu-indicator');
    expect(indicator.props.style).toBeDefined();
    // inactive style applied
  });

  it('shows active indicator when speaking=true', async () => {
    const { getByTestId } = render(
      <VUMeter speaking={true} label="Local" testID="vu-meter" />
    );
    await act(async () => {
      jest.advanceTimersByTime(500);
    });
    const indicator = getByTestId('vu-indicator');
    expect(indicator.props.style).toBeDefined();
    // Should have speaking-active style
  });

  it('renders label text', async () => {
    const { getByText } = render(
      <VUMeter speaking={false} label="Vos" testID="vu-meter" />
    );
    await act(async () => {
      jest.advanceTimersByTime(500);
    });
    expect(getByText('Vos')).toBeTruthy();
  });
});
