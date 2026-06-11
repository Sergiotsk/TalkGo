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

  // Test helper: simulate receiving raw (non-JSON) data
  simulateRawMessage(raw: string): void {
    this.onmessage?.({ data: raw });
  }

  // Test helper: simulate an error event
  simulateError(): void {
    this.onerror?.(new Event('error'));
  }
}

let mockWsInstance: MockWebSocket | null = null;

// Replace global WebSocket with mock
const MockWebSocketCtor = jest.fn().mockImplementation((url: string) => {
  mockWsInstance = new MockWebSocket(url);
  return mockWsInstance;
}) as unknown as typeof WebSocket;

// Preserve static properties needed by the hook (WebSocket.OPEN, etc.)
MockWebSocketCtor.CONNECTING = 0;
MockWebSocketCtor.OPEN = 1;
MockWebSocketCtor.CLOSING = 2;
MockWebSocketCtor.CLOSED = 3;

global.WebSocket = MockWebSocketCtor;

describe('useSignaling', () => {
  beforeEach(() => {
    mockWsInstance = null;
    (global.WebSocket as jest.Mock).mockClear();
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

  // ========== NEW TESTS FOR COVERAGE GAPS ==========

  it('isConnected becomes false on WS error (onerror path)', async () => {
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

    // Wait for initial connection (onopen fires)
    await act(async () => {
      jest.runAllTimers();
    });

    expect(result.current.isConnected).toBe(true);

    // Now fire onerror — should set isConnected to false
    await act(async () => {
      mockWsInstance?.simulateError();
    });

    expect(result.current.isConnected).toBe(false);
  });

  it('dispatches onRoomClosed callback for room-closed message', async () => {
    const onRoomClosed = jest.fn();
    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined: jest.fn(),
        onAnswer: jest.fn(),
        onIceCandidate: jest.fn(),
        onPeerLeft: jest.fn(),
        onRoomClosed,
        onError: jest.fn(),
      })
    );

    await act(async () => {
      jest.runAllTimers();
      mockWsInstance?.simulateMessage({
        type: 'room-closed',
        reason: 'Room expired',
      });
    });

    expect(onRoomClosed).toHaveBeenCalledWith('Room expired');
  });

  it('handles invalid JSON gracefully without calling any callback', async () => {
    const onJoined = jest.fn();
    const onAnswer = jest.fn();
    const onIceCandidate = jest.fn();
    const onPeerLeft = jest.fn();
    const onRoomClosed = jest.fn();
    const onError = jest.fn();

    renderHook(() =>
      useSignaling({
        serverUrl: 'ws://localhost:8080',
        roomId: 'room-1',
        onJoined,
        onAnswer,
        onIceCandidate,
        onPeerLeft,
        onRoomClosed,
        onError,
      })
    );

    await act(async () => {
      jest.runAllTimers();
      // Send invalid JSON — raw string that won't parse
      mockWsInstance?.simulateRawMessage('this is not valid json');
    });

    // None of the callbacks should have been called
    expect(onJoined).not.toHaveBeenCalled();
    expect(onAnswer).not.toHaveBeenCalled();
    expect(onIceCandidate).not.toHaveBeenCalled();
    expect(onPeerLeft).not.toHaveBeenCalled();
    expect(onRoomClosed).not.toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it('sendJoin calls WebSocket send with correct payload', async () => {
    const sendJoinSpy = jest.fn();

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

    // Wait for WS open
    await act(async () => {
      jest.runAllTimers();
    });

    expect(mockWsInstance).not.toBeNull();

    // Call sendJoin
    await act(async () => {
      result.current.sendJoin('user-42', 'es');
    });

    // WS mock's send should have been called with the right JSON
    expect(mockWsInstance!.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'join',
        room_id: 'room-1',
        user_id: 'user-42',
        lang: 'es',
      })
    );
  });

  it('sendOffer calls WebSocket send with correct payload', async () => {
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

    await act(async () => {
      result.current.sendOffer('sess-abc', 'mock-sdp-offer');
    });

    expect(mockWsInstance!.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'offer',
        session_id: 'sess-abc',
        sdp: 'mock-sdp-offer',
      })
    );
  });

  it('sendIceCandidate calls WebSocket send with correct payload', async () => {
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

    await act(async () => {
      result.current.sendIceCandidate('sess-abc', 'candidate:1 1 UDP ...');
    });

    expect(mockWsInstance!.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'ice-candidate',
        session_id: 'sess-abc',
        candidate: 'candidate:1 1 UDP ...',
      })
    );
  });

  it('sendLeave calls WebSocket send with correct payload', async () => {
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

    await act(async () => {
      result.current.sendLeave('sess-abc');
    });

    expect(mockWsInstance!.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'leave',
        session_id: 'sess-abc',
      })
    );
  });

  it('sendJoin does not throw when WebSocket is closed or null', async () => {
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

    // Wait for WS to open
    await act(async () => {
      jest.runAllTimers();
    });

    // Close the connection — wsRef.current will be set to null by close()
    await act(async () => {
      result.current.close();
    });

    // Now wsRef.current is null — calling sendJoin should NOT throw
    await act(async () => {
      expect(() => {
        result.current.sendJoin('user-42', 'es');
      }).not.toThrow();
    });
  });

  it('close() sets wsRef to null and triggers cleanup', async () => {
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

    // isConnected should be true
    expect(result.current.isConnected).toBe(true);

    // Close — this sets wsRef.current = null internally
    await act(async () => {
      result.current.close();
    });

    // After close, isConnected should be false
    expect(result.current.isConnected).toBe(false);

    // Now sending anything should not throw (wsRef.current is null)
    await act(async () => {
      expect(() => {
        result.current.sendJoin('user-42', 'es');
      }).not.toThrow();
    });
  });

  it('reconnect closes old WS and creates a new one', async () => {
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
    expect(global.WebSocket).toHaveBeenCalledTimes(1);

    const firstWs = mockWsInstance;

    // Reconnect
    await act(async () => {
      result.current.reconnect();
    });

    // The old WS should be closed (readyState = CLOSED)
    expect(firstWs?.readyState).toBe(MockWebSocket.CLOSED);

    // A new WebSocket should have been created
    expect(global.WebSocket).toHaveBeenCalledTimes(2);

    // Advance timers to let new WS open
    await act(async () => {
      jest.runAllTimers();
    });

    expect(result.current.isConnected).toBe(true);
  });
});
