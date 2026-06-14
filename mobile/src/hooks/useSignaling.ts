import { useCallback, useEffect, useRef, useState } from 'react';
import type { OutgoingMessage, SignalingMessage } from '../types/signaling';

export interface UseSignalingConfig {
  serverUrl: string;
  roomId: string;
  onJoined: (sessionId: string) => void;
  onAnswer: (sdp: string) => void;
  onIceCandidate: (candidate: string) => void;
  onPeerLeft: (sessionId: string) => void;
  onRoomClosed: (reason: string) => void;
  onError: (message: string) => void;
  onTranscript?: (text: string) => void;
}

export interface UseSignalingReturn {
  isConnected: boolean;
  sendJoin: (userId: string, lang: string) => void;
  sendOffer: (sessionId: string, sdp: string) => void;
  sendIceCandidate: (sessionId: string, candidate: string) => void;
  sendLeave: (sessionId: string) => void;
  reconnect: () => void;
  close: () => void;
}

export function useSignaling(config: UseSignalingConfig): UseSignalingReturn {
  const [isConnected, setIsConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const configRef = useRef(config);
  configRef.current = config;

  const connect = useCallback(() => {
    const { serverUrl, roomId } = configRef.current;
    const url = `${serverUrl}/ws/${roomId}`;
    console.log('[Signaling] connecting to', url);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('[Signaling] connected');
      setIsConnected(true);
    };

    ws.onclose = (e) => {
      console.log('[Signaling] closed', e.code, e.reason);
      setIsConnected(false);
    };

    ws.onerror = (e) => {
      console.error('[Signaling] error', e);
      setIsConnected(false);
    };

    ws.onmessage = (event: MessageEvent<string>) => {
      let msg: SignalingMessage;
      try {
        msg = JSON.parse(event.data) as SignalingMessage;
      } catch {
        return;
      }

      const cb = configRef.current;
      switch (msg.type) {
        case 'joined':
          cb.onJoined(msg.session_id ?? '');
          break;
        case 'answer':
          cb.onAnswer(msg.sdp ?? '');
          break;
        case 'ice-candidate':
          cb.onIceCandidate(msg.candidate ?? '');
          break;
        case 'peer-left':
          cb.onPeerLeft(msg.session_id ?? '');
          break;
        case 'room-closed':
          cb.onRoomClosed(msg.reason ?? '');
          break;
        case 'error':
          cb.onError(msg.message ?? 'Unknown error');
          break;
        case 'transcript':
          cb.onTranscript?.(msg.text ?? '');
          break;
        default:
          // Unknown message type — ignore silently (forward compatibility)
          break;
      }
    };
  }, []);

  useEffect(() => {
    connect();
    return () => {
      wsRef.current?.close();
    };
  }, [connect]);

  const send = useCallback((msg: OutgoingMessage) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(msg));
    }
  }, []);

  const sendJoin = useCallback(
    (userId: string, lang: string) => {
      send({
        type: 'join',
        room_id: config.roomId,
        user_id: userId,
        lang,
      });
    },
    [send, config.roomId]
  );

  const sendOffer = useCallback(
    (sessionId: string, sdp: string) => {
      send({ type: 'offer', session_id: sessionId, sdp });
    },
    [send]
  );

  const sendIceCandidate = useCallback(
    (sessionId: string, candidate: string) => {
      send({ type: 'ice-candidate', session_id: sessionId, candidate });
    },
    [send]
  );

  const sendLeave = useCallback(
    (sessionId: string) => {
      send({ type: 'leave', session_id: sessionId });
    },
    [send]
  );

  const reconnect = useCallback(() => {
    wsRef.current?.close();
    connect();
  }, [connect]);

  const close = useCallback(() => {
    wsRef.current?.close();
    wsRef.current = null;
  }, []);

  return {
    isConnected,
    sendJoin,
    sendOffer,
    sendIceCandidate,
    sendLeave,
    reconnect,
    close,
  };
}
