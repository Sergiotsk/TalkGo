// TASK-076 + TASK-086: Tests for ConversationScreen

import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import { ConversationScreen } from '../../src/screens/ConversationScreen';
import { useSessionStore } from '../../src/store/sessionStore';

// ===== Shared mock return values (per-test configurable) =====

const mockSignaling = {
  isConnected: true,
  sendJoin: jest.fn(),
  sendOffer: jest.fn(),
  sendIceCandidate: jest.fn(),
  sendLeave: jest.fn(),
  reconnect: jest.fn(),
  close: jest.fn(),
};

const mockWebRTC = {
  localStream: null,
  remoteStream: null,
  iceConnectionState: 'new',
  createOffer: jest.fn().mockResolvedValue({ type: 'offer', sdp: 'mock-sdp' }),
  setRemoteAnswer: jest.fn().mockResolvedValue(undefined),
  addIceCandidate: jest.fn().mockResolvedValue(undefined),
  close: jest.fn(),
};

// Track useReconnection callbacks for per-test triggering
let reconnectionConfig: {
  onReconnect: () => Promise<void>;
  onFailed: () => void;
  maxAttempts: number;
  baseDelay: number;
} | null = null;

const mockReconnectionReturn = {
  state: 'connected' as const,
  attempt: 0,
  trigger: jest.fn().mockImplementation(() => {
    // When trigger is called, invoke the real onReconnect callback
    // so the ConversationScreen's onReconnect handler gets covered
    if (reconnectionConfig?.onReconnect) {
      return reconnectionConfig.onReconnect();
    }
    return Promise.resolve();
  }),
  cancel: jest.fn(),
  reset: jest.fn(),
};

// Track useSignaling callbacks for per-test triggering
let signalingConfig: Record<string, (...args: any[]) => void> = {};

// ===== Module mocks =====

jest.mock('../../src/hooks/useWebRTC', () => ({
  useWebRTC: () => mockWebRTC,
}));

jest.mock('../../src/hooks/useSignaling', () => ({
  useSignaling: (config: any) => {
    // Store the config so tests can invoke callbacks directly
    signalingConfig = {
      onJoined: config.onJoined,
      onAnswer: config.onAnswer,
      onIceCandidate: config.onIceCandidate,
      onPeerLeft: config.onPeerLeft,
      onRoomClosed: config.onRoomClosed,
      onError: config.onError,
    };
    return mockSignaling;
  },
}));

jest.mock('../../src/hooks/useReconnection', () => ({
  useReconnection: (config: any) => {
    reconnectionConfig = config;
    return mockReconnectionReturn;
  },
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
    jest.clearAllMocks();
    mockReconnectionReturn.state = 'connected';
    mockReconnectionReturn.trigger.mockClear();
    reconnectionConfig = null;
    signalingConfig = {};
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

  // ========== NEW TESTS FOR COVERAGE GAPS ==========

  it('shows reconnecting state in ConnectionStatus', () => {
    const { getByText } = render(<ConversationScreen {...defaultProps} />);

    // After mount, connectionState is 'connecting'. Override to 'reconnecting'.
    act(() => {
      useSessionStore.getState().setConnectionState('reconnecting');
    });

    expect(getByText('Reconectando...')).toBeTruthy();
  });

  it('shows failed state in ConnectionStatus', () => {
    const { getByText } = render(<ConversationScreen {...defaultProps} />);

    // Override to 'failed'
    act(() => {
      useSessionStore.getState().setConnectionState('failed');
    });

    expect(getByText('Conexión perdida')).toBeTruthy();
  });

  it('handleEndCall sends leave and closes connections on confirm', () => {
    const { getByText } = render(<ConversationScreen {...defaultProps} />);

    // Press "Finalizar" to open confirmation modal
    fireEvent.press(getByText('Finalizar'));

    // Confirm in the dialog
    fireEvent.press(getByText('Confirmar'));

    // Verify all cleanup callbacks were called
    expect(mockSignaling.sendLeave).toHaveBeenCalledWith('sess-1');
    expect(mockSignaling.close).toHaveBeenCalled();
    expect(mockWebRTC.close).toHaveBeenCalled();
  });

  it('triggers reconnection onReconnect when useReconnection trigger fires', async () => {
    jest.useFakeTimers();
    render(<ConversationScreen {...defaultProps} />);

    // After mount, signaling is connected. Make it disconnected to trigger
    // the useEffect on line 148-153 which calls reconnection.trigger().
    mockSignaling.isConnected = false;

    // Force a re-render by updating a store value — this causes the component
    // to re-evaluate the effect dependency `signaling.isConnected`.
    act(() => {
      useSessionStore.getState().setConnectionState('connected');
    });

    // Advance timers to let the onReconnect callback execute
    await act(async () => {
      jest.runAllTimers();
    });

    // The onReconnect callback calls signaling.reconnect() and creates an offer
    expect(mockSignaling.reconnect).toHaveBeenCalled();
    expect(mockWebRTC.createOffer).toHaveBeenCalledWith({ iceRestart: true });
    expect(mockSignaling.sendOffer).toHaveBeenCalled();

    jest.useRealTimers();
  });
});
