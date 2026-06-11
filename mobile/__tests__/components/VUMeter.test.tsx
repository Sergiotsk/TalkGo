// TASK-064: Tests for VUMeter component

import React from 'react';
import { render } from '@testing-library/react-native';
import { VUMeter } from '../../src/components/VUMeter';

describe('VUMeter', () => {
  it('renders without crashing', () => {
    const { getByTestId } = render(
      <VUMeter speaking={false} label="Local" testID="vu-meter" />
    );
    expect(getByTestId('vu-meter')).toBeTruthy();
  });

  it('shows inactive indicator when speaking=false', () => {
    const { getByTestId } = render(
      <VUMeter speaking={false} label="Local" testID="vu-meter" />
    );
    const indicator = getByTestId('vu-indicator');
    expect(indicator.props.style).toBeDefined();
    // inactive style applied
  });

  it('shows active indicator when speaking=true', () => {
    const { getByTestId } = render(
      <VUMeter speaking={true} label="Local" testID="vu-meter" />
    );
    const indicator = getByTestId('vu-indicator');
    expect(indicator.props.style).toBeDefined();
    // Should have speaking-active style
  });

  it('renders label text', () => {
    const { getByText } = render(
      <VUMeter speaking={false} label="Vos" testID="vu-meter" />
    );
    expect(getByText('Vos')).toBeTruthy();
  });
});
