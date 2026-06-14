import { useCallback, useEffect, useRef, useState } from 'react';
import { PermissionsAndroid, Platform } from 'react-native';
import {
  RTCPeerConnection,
  RTCSessionDescription,
  RTCIceCandidate,
  mediaDevices,
} from 'react-native-webrtc';
import type {
  RTCIceConnectionState,
  RTCIceServer,
  MediaStream,
  RTCOfferOptions,
} from 'react-native-webrtc';

const DEFAULT_ICE_SERVERS: RTCIceServer[] = [
  { urls: 'stun:stun.l.google.com:19302' },
  { urls: 'stun:stun1.l.google.com:19302' },
  {
    urls: 'turn:138.201.95.167:3478',
    username: 'talkgo',
    credential: 'Avivirqueson2dias',
  },
];

export interface UseWebRTCConfig {
  iceServers?: RTCIceServer[];
  onIceCandidate?: (candidate: string) => void;
}

export interface UseWebRTCReturn {
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  iceConnectionState: RTCIceConnectionState;
  createOffer: (options?: RTCOfferOptions) => Promise<{ type: string; sdp?: string }>;
  setRemoteAnswer: (sdp: string) => Promise<void>;
  addIceCandidate: (candidate: string) => Promise<void>;
  close: () => void;
}

export function useWebRTC(config?: UseWebRTCConfig, onIceCandidate?: (candidate: string) => void): UseWebRTCReturn {
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [iceConnectionState, setIceConnectionState] =
    useState<RTCIceConnectionState>('new');

  const pcRef = useRef<RTCPeerConnection | null>(null);

  useEffect(() => {
    const iceServers = config?.iceServers ?? DEFAULT_ICE_SERVERS;
    const pc = new RTCPeerConnection({ iceServers });
    pcRef.current = pc;

    // Handle remote stream tracks
    pc.ontrack = (event: { streams: MediaStream[] }) => {
      if (event.streams && event.streams[0]) {
        setRemoteStream(event.streams[0]);
      }
    };

    // Trickle ICE — send candidates to the server as they're gathered
    pc.onicecandidate = (event: { candidate: { candidate: string } | null }) => {
      if (event.candidate?.candidate && onIceCandidate) {
        onIceCandidate(event.candidate.candidate);
      }
    };

    // Track ICE connection state
    pc.oniceconnectionstatechange = () => {
      setIceConnectionState(pc.iceConnectionState as RTCIceConnectionState);
    };

    // Request microphone access and add to peer connection
    const requestAndGetMedia = async () => {
      if (Platform.OS === 'android') {
        const result = await PermissionsAndroid.request(
          PermissionsAndroid.PERMISSIONS.RECORD_AUDIO,
          {
            title: 'Micrófono',
            message: 'TalkGo necesita el micrófono para la conversación.',
            buttonPositive: 'Permitir',
            buttonNegative: 'Cancelar',
          }
        );
        if (result !== PermissionsAndroid.RESULTS.GRANTED) {
          console.error('[WebRTC] mic permission denied, result:', result);
          return;
        }
        console.log('[WebRTC] mic permission granted');
      }

      mediaDevices
        .getUserMedia({ audio: true, video: false })
        .then((stream: MediaStream) => {
          console.log('[WebRTC] getUserMedia OK, tracks:', stream.getTracks().length);
          setLocalStream(stream);
          stream.getTracks().forEach((track) => pc.addTrack(track, stream));
        })
        .catch((err: unknown) => {
          console.error('[WebRTC] getUserMedia FAILED:', err);
        });
    };

    void requestAndGetMedia();

    return () => {
      // Cleanup on unmount
      localStream?.getTracks().forEach((t) => t.stop());
      pc.close();
      pcRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const createOffer = useCallback(
    async (options?: RTCOfferOptions): Promise<{ type: string; sdp?: string }> => {
      const pc = pcRef.current;
      if (!pc) throw new Error('PeerConnection not initialized');

      const offer = await pc.createOffer(options ?? {});
      await pc.setLocalDescription(new RTCSessionDescription(offer));
      return offer;
    },
    []
  );

  const setRemoteAnswer = useCallback(async (sdp: string): Promise<void> => {
    const pc = pcRef.current;
    if (!pc) return;

    await pc.setRemoteDescription(
      new RTCSessionDescription({ type: 'answer', sdp })
    );
  }, []);

  const addIceCandidate = useCallback(
    async (candidate: string): Promise<void> => {
      const pc = pcRef.current;
      if (!pc) return;

      await pc.addIceCandidate(new RTCIceCandidate({ candidate }));
    },
    []
  );

  const close = useCallback(() => {
    const pc = pcRef.current;
    if (!pc) return;

    localStream?.getTracks().forEach((t) => t.stop());
    pc.close();
    setLocalStream(null);
    setRemoteStream(null);
    setIceConnectionState('closed');
    pcRef.current = null;
  }, [localStream]);

  return {
    localStream,
    remoteStream,
    iceConnectionState,
    createOffer,
    setRemoteAnswer,
    addIceCandidate,
    close,
  };
}
