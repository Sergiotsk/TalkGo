/**
 * App.test.tsx — smoke test for the App entry point.
 * All ConversationScreen hooks are mocked to avoid native dependencies.
 */

import 'react-native';
import React from 'react';
import renderer from 'react-test-renderer';

// Mock ConversationScreen to avoid rendering native hooks in App test
jest.mock('../src/screens/ConversationScreen', () => ({
  ConversationScreen: () => null,
}));

// Mock signalingService to avoid dynamic imports
jest.mock('../src/services/signalingService', () => ({
  initAudioService: jest.fn().mockResolvedValue(undefined),
  startAudioService: jest.fn(),
  stopAudioService: jest.fn(),
}));

import App from '../App';

it('App renders without crashing', () => {
  renderer.create(<App />);
});
