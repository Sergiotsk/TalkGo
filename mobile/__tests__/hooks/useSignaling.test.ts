// TASK-054: Tests for useSignaling hook

import { renderHook, act } from '@testing-library/react-native';
import { useSignaling } from '../../src/hooks/useSignaling';

// Mock WebSocket
class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readyState: number = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onclose: ((event: { code: number; reason: string }) => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  url: string;

  constructor(url: string) {
    this.url = url;
    // Simulate async open
    setTimeout(() => {
      this.readyState = MockWebSocket.OPEN;
      this.onopen?.();
    }, 0);
  }

  send = jest.fn();

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code: 1000, reason: 'test close' });
  }

  // Test helper: simulate receiving a message
  simulateMessage(data: object): void {
    this.onmessage?.({ data: JSON.stringify(data) });
  }
}

let mockWsInstance: MockWebSocket | null = null;

// Replace global WebSocket with mock
global.WebSocket = jest.fn().mockImplementation((url: string) => {
  mockWsInstance = new MockWebSocket(url);
  return mockWsInstance;
}) as unknown as typeof WebSocket;

describe('useSignaling', () => {
  beforeEach(() => {
    mockWsInstance = null;
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('connects to correct WebSocket URL', () => {
    const onJoined = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined,
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    expect(global.WebSocket).toHaveBeenCalledWith(
      'ws://localhost:8080/ws/room-1'
    );
  });

  it('dispatches onJoined callback for joined message', async () => {
    const onJoined = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined,
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
      mockWsInstance?.simulateMessage({
        type: 'joined',
        session_id: 'sess-abc',
        room_id: 'room-1',
      });
    });

    expect(onJoined).toHaveBeenCalledWith('sess-abc');
  });

  it('dispatches onAnswer for answer message', async () => {
    const onAnswer = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer,
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
      mockWsInstance?.simulateMessage({
        type: 'answer',
        session_id: 'sess-abc',
        sdp: 'mock-sdp-answer',
      });
    });

    expect(onAnswer).toHaveBeenCalledWith('mock-sdp-answer');
  });

  it('dispatches onIceCandidate for ice-candidate message', async () => {
    const onIceCandidate = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer: jest.fn(),
        onIceCandidate,
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
      mockWsInstance?.simulateMessage({
        type: 'ice-candidate',
        session_id: 'sess-abc',
        candidate: 'candidate:1 1 UDP ...',
      });
    });

    expect(onIceCandidate).toHaveBeenCalledWith('candidate:1 1 UDP ...');
  });

  it('dispatches onPeerLeft for peer-left message', async () => {
    const onPeerLeft = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft,
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
      mockWsInstance?.simulateMessage({ type: 'peer-left', session_id: 'sess-abc' });
    });

    expect(onPeerLeft).toHaveBeenCalledWith('sess-abc');
  });

  it('dispatches onError for error message', async () => {
    const onError = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError,
      })
    );

    await act(async () => {
      jest.runAllTimers();
      mockWsInstance?.simulateMessage({
        type: 'error',
        message: 'Esta sala ya tiene 2 participantes',
      });
    });

    expect(onError).toHaveBeenCalledWith('Esta sala ya tiene 2 participantes');
  });

  it('isConnected becomes false on WS close', async () => {
    const { result } = renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
    });

    expect(result.current.isConnected).toBe(true);

    await act(async () => {
      mockWsInstance?.close();
    });

    expect(result.current.isConnected).toBe(false);
  });

  it('ignores unknown message types without throwing', async () => {
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed: jest.fn(),
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
      // Should not throw
      expect(() => {
        mockWsInstance?.simulateMessage({ type: 'unknown-future-type' });
      }).not.toThrow();
    });
  });
});
