/**
 * App.test.tsx — smoke + navigation guard tests.
 */

import 'react-native';
import React from 'react';
import { render, screen } from '@testing-library/react-native';

const mockHydrate = jest.fn().mockResolvedValue(undefined);

// Default: user has no name (first launch)
let mockName = '';

jest.mock('../src/store/userStore', () => ({
  useUserStore: jest.fn(() => ({
    name: mockName,
    localLang: 'es',
    hydrate: mockHydrate,
    setName: jest.fn(),
    setLocalLang: jest.fn(),
  })),
}));

jest.mock('../src/screens/OnboardingScreen', () => ({
  OnboardingScreen: () => {
    const { Text } = require('react-native');
    return <Text>OnboardingScreen</Text>;
  },
}));

jest.mock('../src/screens/HomeScreen', () => ({
  HomeScreen: () => {
    const { Text } = require('react-native');
    return <Text>HomeScreen</Text>;
  },
}));

jest.mock('../src/screens/ConversationScreen', () => ({
  ConversationScreen: () => {
    const { Text } = require('react-native');
    return <Text>ConversationScreen</Text>;
  },
}));

jest.mock('../src/services/signalingService', () => ({
  initAudioService: jest.fn().mockResolvedValue(undefined),
  startAudioService: jest.fn(),
  stopAudioService: jest.fn(),
}));

import App from '../App';

describe('App — navigation guard', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('calls hydrate on mount', () => {
    render(<App />);
    expect(mockHydrate).toHaveBeenCalled();
  });

  it('shows OnboardingScreen when name is empty (first launch)', () => {
    mockName = '';
    render(<App />);
    expect(screen.getByText('OnboardingScreen')).toBeTruthy();
    expect(screen.queryByText('HomeScreen')).toBeNull();
  });

  it('shows HomeScreen when name is set (returning user)', () => {
    mockName = 'Alice';
    render(<App />);
    expect(screen.getByText('HomeScreen')).toBeTruthy();
    expect(screen.queryByText('OnboardingScreen')).toBeNull();
  });
});
