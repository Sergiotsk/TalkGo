// TASK-076 + TASK-086: Tests for ConversationScreen

import React from 'react';
import { render, act } from '@testing-library/react-native';
import { ConversationScreen } from '../../src/screens/ConversationScreen';
import { useSessionStore } from '../../src/store/sessionStore';

// Mock all hooks used by the screen
jest.mock('../../src/hooks/useWebRTC', () => ({
  useWebRTC: () => ({
    localStream: null,
    remoteStream: null,
    iceConnectionState: 'new',
    createOffer: jest.fn().mockResolvedValue({ type: 'offer', sdp: 'mock-sdp' }),
    setRemoteAnswer: jest.fn().mockResolvedValue(undefined),
    addIceCandidate: jest.fn().mockResolvedValue(undefined),
    close: jest.fn(),
  }),
}));

jest.mock('../../src/hooks/useSignaling', () => ({
  useSignaling: () => ({
    isConnected: true,
    sendJoin: jest.fn(),
    sendOffer: jest.fn(),
    sendIceCandidate: jest.fn(),
    sendLeave: jest.fn(),
    reconnect: jest.fn(),
    close: jest.fn(),
  }),
}));

jest.mock('../../src/hooks/useReconnection', () => ({
  useReconnection: () => ({
    state: 'connected',
    attempt: 0,
    trigger: jest.fn(),
    cancel: jest.fn(),
    reset: jest.fn(),
  }),
}));

jest.mock('../../src/hooks/useAudioLevel', () => ({
  useAudioLevel: () => ({ localSpeaking: false, peerSpeaking: false }),
}));

jest.mock('../../src/hooks/useKeepAwake', () => ({
  useKeepAwake: jest.fn(),
}));

jest.mock('../../src/hooks/useSessionTimer', () => ({
  useSessionTimer: jest.fn(),
}));

jest.mock('../../src/services/api', () => ({
  findRoomByCode: jest.fn().mockResolvedValue({ room_id: 'room-1', short_code: 'ABC123' }),
  createRoom: jest.fn().mockResolvedValue({ room_id: 'room-1', short_code: 'ABC123' }),
  deleteRoom: jest.fn().mockResolvedValue(undefined),
  ApiError: class ApiError extends Error {
    statusCode: number;
    constructor(statusCode: number, message: string) {
      super(message);
      this.statusCode = statusCode;
    }
  },
}));

jest.mock('../../src/services/signalingService', () => ({
  initAudioService: jest.fn().mockResolvedValue(undefined),
  startAudioService: jest.fn(),
  stopAudioService: jest.fn(),
}));

const defaultProps = {
  roomId: 'room-1',
  shortCode: 'ABC123',
  userId: 'user-1',
  serverUrl: 'ws://localhost:8080',
  localLang: 'es',
  peerLang: 'en',
};

describe('ConversationScreen', () => {
  beforeEach(() => {
    act(() => {
      useSessionStore.getState().connect(
        'room-1',
        'ABC123',
        'sess-1',
        'es',
        'en'
      );
    });
  });

  afterEach(() => {
    act(() => {
      useSessionStore.getState().disconnect();
    });
  });

  it('renders without crashing in connected state', () => {
    const { getByText } = render(<ConversationScreen {...defaultProps} />);
    // Should show Finalizar button
    expect(getByText('Finalizar')).toBeTruthy();
  });

  it('shows VU meters in connected state', () => {
    const { getByText } = render(<ConversationScreen {...defaultProps} />);
    expect(getByText('Vos')).toBeTruthy();
    expect(getByText('Ellos')).toBeTruthy();
  });

  it('shows mute button', () => {
    const { getByTestId } = render(<ConversationScreen {...defaultProps} />);
    expect(getByTestId('mute-button-unmuted')).toBeTruthy();
  });

  it('shows connection status indicator', () => {
    const { getByText } = render(
      <ConversationScreen {...defaultProps} />
    );
    // On mount, ConversationScreen sets state to 'connecting' via useEffect.
    // The ConnectionStatus should show one of the known states.
    const knownStates = ['Conectando...', 'En línea', 'Reconectando...', 'Conexión perdida', 'Desconectado'];
    const found = knownStates.some((label) => {
      try {
        getByText(label);
        return true;
      } catch {
        return false;
      }
    });
    expect(found).toBe(true);
  });
});
