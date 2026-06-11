// TASK-074: Tests for PipelineErrorBanner component

import React from 'react';
import { render } from '@testing-library/react-native';
import { PipelineErrorBanner } from '../../src/components/PipelineErrorBanner';

describe('PipelineErrorBanner', () => {
  it('is not visible when pipelineError=null and consecutiveErrors=0', () => {
    const { queryByTestId } = render(
      <PipelineErrorBanner pipelineError={null} consecutiveErrors={0} />
    );
    expect(queryByTestId('pipeline-error-banner')).toBeNull();
  });

  it('shows banner when pipelineError is set', () => {
    const { getByTestId, getByText } = render(
      <PipelineErrorBanner
        pipelineError="translation failed"
        consecutiveErrors={1}
      />
    );
    expect(getByTestId('pipeline-error-banner')).toBeTruthy();
    expect(getByText(/Error de traducción/)).toBeTruthy();
  });

  it('shows fallback text when consecutiveErrors >= 3', () => {
    const { getByText } = render(
      <PipelineErrorBanner
        pipelineError="translation failed"
        consecutiveErrors={3}
      />
    );
    expect(getByText(/Traducción no disponible temporalmente/)).toBeTruthy();
  });

  it('shows normal banner for consecutiveErrors < 3', () => {
    const { queryByText } = render(
      <PipelineErrorBanner
        pipelineError="translation failed"
        consecutiveErrors={2}
      />
    );
    expect(
      queryByText(/Traducción no disponible temporalmente/)
    ).toBeNull();
  });
});
