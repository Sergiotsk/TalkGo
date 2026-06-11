// TASK-066: Tests for ConnectionStatus component

import React from 'react';
import { render } from '@testing-library/react-native';
import { ConnectionStatus } from '../../src/components/ConnectionStatus';
import type { ConnectionState } from '../../src/types/signaling';

describe('ConnectionStatus', () => {
  const cases: Array<[ConnectionState, string]> = [
    ['connecting', 'Conectando...'],
    ['connected', 'En línea'],
    ['reconnecting', 'Reconectando...'],
    ['failed', 'Conexión perdida'],
    ['idle', 'Desconectado'],
  ];

  it.each(cases)(
    'shows "%s" label for connectionState=%s',
    (state, expectedLabel) => {
      const { getByText } = render(<ConnectionStatus connectionState={state} />);
      expect(getByText(expectedLabel)).toBeTruthy();
    }
  );
});
