// TASK-070: Tests for SessionTimer component

import React from 'react';
import { render } from '@testing-library/react-native';
import { SessionTimer } from '../../src/components/SessionTimer';

// Mock the Zustand store to control elapsedSeconds
jest.mock('../../src/store/sessionStore', () => ({
  useSessionStore: jest.fn((selector: (s: { elapsedSeconds: number }) => unknown) =>
    selector({ elapsedSeconds: 0 })
  ),
}));

import { useSessionStore } from '../../src/store/sessionStore';

const mockUseSessionStore = useSessionStore as jest.MockedFunction<typeof useSessionStore>;

describe('SessionTimer', () => {
  it.each([
    [0, '00:00'],
    [65, '01:05'],
    [3600, '60:00'],
    [3661, '61:01'],
    [59, '00:59'],
  ])('formats %ds as "%s"', (seconds, expected) => {
    mockUseSessionStore.mockImplementation(
      (selector: (s: { elapsedSeconds: number }) => unknown) =>
        selector({ elapsedSeconds: seconds }) as ReturnType<typeof selector>
    );

    const { getByText } = render(<SessionTimer />);
    expect(getByText(expected)).toBeTruthy();
  });
});
