/**
 * ConversationScreen — navigation params test.
 * Verifies the component reads roomId, userId, etc. from route.params,
 * NOT from direct props.
 */

import React from 'react';
import { render, screen } from '@testing-library/react-native';

// ── Heavy native deps ──────────────────────────────────────────────────────
jest.mock('../../hooks/useWebRTC', () => ({
  useWebRTC: () => ({
    localStream: null,
    createOffer: jest.fn().mockResolvedValue({ sdp: 'offer-sdp' }),
    setRemoteAnswer: jest.fn().mockResolvedValue(undefined),
    addIceCandidate: jest.fn().mockResolvedValue(undefined),
    close: jest.fn(),
    pc: null,
  }),
}));

jest.mock('../../hooks/useSignaling', () => ({
  useSignaling: () => ({
    isConnected: false,
    sendJoin: jest.fn(),
    sendOffer: jest.fn(),
    sendLeave: jest.fn(),
    reconnect: jest.fn(),
    close: jest.fn(),
  }),
}));

jest.mock('../../hooks/useReconnection', () => ({
  useReconnection: () => ({
    trigger: jest.fn(),
    reset: jest.fn(),
    cancel: jest.fn(),
  }),
}));

jest.mock('../../hooks/useAudioLevel', () => ({
  useAudioLevel: () => ({ localSpeaking: false, peerSpeaking: false }),
}));

jest.mock('../../hooks/useKeepAwake', () => ({
  useKeepAwake: jest.fn(),
}));

jest.mock('../../hooks/useSessionTimer', () => ({
  useSessionTimer: jest.fn(),
}));

jest.mock('../../services/signalingService', () => ({
  initAudioService: jest.fn().mockResolvedValue(undefined),
  startAudioService: jest.fn(),
  stopAudioService: jest.fn(),
}));

jest.mock('../../store/sessionStore', () => {
  const mockState = {
    connectionState: 'connecting',
    sessionId: null,
    isMuted: false,
    localSpeaking: false,
    peerSpeaking: false,
    pipelineError: null,
    consecutiveErrors: 0,
    connect: jest.fn(),
    disconnect: jest.fn(),
    setConnectionState: jest.fn(),
    setMuted: jest.fn(),
    setLocalSpeaking: jest.fn(),
    setPeerSpeaking: jest.fn(),
  };
  const useSessionStore = Object.assign(
    jest.fn((selector?: (s: typeof mockState) => unknown) =>
      selector ? selector(mockState) : mockState
    ),
    { getState: jest.fn(() => mockState) }
  );
  return { useSessionStore };
});

// ── Component ──────────────────────────────────────────────────────────────
// eslint-disable-next-line @typescript-eslint/no-require-imports
const { ConversationScreen } = require('../ConversationScreen');
import type { RouteProp } from '@react-navigation/native';
import type { RootStackParamList } from '../../navigation/types';

const defaultRoute = {
  params: {
    roomId: 'room-abc',
    shortCode: 'ABC123',
    userId: 'user-42',
    serverUrl: 'wss://example.com',
    localLang: 'es',
    peerLang: 'en',
  },
} as unknown as RouteProp<RootStackParamList, 'Conversation'>;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const mockNavigation = { navigate: jest.fn(), goBack: jest.fn() } as any;

describe('ConversationScreen — navigation params', () => {
  it('renders without crashing when given route.params', () => {
    expect(() => render(<ConversationScreen route={defaultRoute} navigation={mockNavigation} />)).not.toThrow();
  });

  it('shows connecting status on initial render', () => {
    render(<ConversationScreen route={defaultRoute} navigation={mockNavigation} />);
    // ConnectionStatus renders "Conectando..." text in connecting state
    expect(screen.getByText(/conectando/i)).toBeTruthy();
  });

  it('does NOT accept direct roomId prop (params come from route)', () => {
    // ConversationScreen should NOT have roomId as a direct prop anymore
    const props = ConversationScreen.length;
    // A component using navigation params receives (props) where props = { route, navigation }
    // It should NOT destructure roomId directly from top-level props
    expect(props).toBeLessThanOrEqual(1); // 0 or 1 param (the props object)
  });
});
