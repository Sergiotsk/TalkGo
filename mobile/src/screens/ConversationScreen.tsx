import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Platform, SafeAreaView, StyleSheet, Text, View } from 'react-native';
import { NativeStackScreenProps } from '@react-navigation/native-stack';
import { ConnectionStatus } from '../components/ConnectionStatus';
import { EndCallButton } from '../components/EndCallButton';
import { MuteButton } from '../components/MuteButton';
import { PipelineErrorBanner } from '../components/PipelineErrorBanner';
import { SessionTimer } from '../components/SessionTimer';
import { VUMeter } from '../components/VUMeter';
import { useAudioLevel } from '../hooks/useAudioLevel';
import { useKeepAwake } from '../hooks/useKeepAwake';
import { useReconnection } from '../hooks/useReconnection';
import { useSessionTimer } from '../hooks/useSessionTimer';
import { useSignaling } from '../hooks/useSignaling';
import { useWebRTC } from '../hooks/useWebRTC';
import { initAudioService, startAudioService, stopAudioService } from '../services/signalingService';
import { useSessionStore } from '../store/sessionStore';
import { useUserStore } from '../store/userStore';
import { RootStackParamList } from '../navigation/types';

type ConversationScreenProps = NativeStackScreenProps<RootStackParamList, 'Conversation'>;

/**
 * ConversationScreen — the main active call screen.
 * Receives roomId, shortCode, userId, serverUrl, localLang, peerLang via route.params.
 */
export function ConversationScreen({ route, navigation }: ConversationScreenProps): React.JSX.Element {
  const { roomId, shortCode, userId, serverUrl, localLang, peerLang } = route.params;
  const { name } = useUserStore();
  const [peerName, setPeerName] = useState('');
  // Store state
  const connectionState = useSessionStore((s) => s.connectionState);
  const sessionId = useSessionStore((s) => s.sessionId);
  const isMuted = useSessionStore((s) => s.isMuted);
  const localSpeaking = useSessionStore((s) => s.localSpeaking);
  const peerSpeaking = useSessionStore((s) => s.peerSpeaking);
  const pipelineError = useSessionStore((s) => s.pipelineError);
  const consecutiveErrors = useSessionStore((s) => s.consecutiveErrors);
  const lastTranscript = useSessionStore((s) => s.lastTranscript);

  const {
    connect,
    disconnect,
    setConnectionState,
    setMuted,
    setLocalSpeaking,
    setPeerSpeaking,
    setLastTranscript,
  } = useSessionStore.getState();

  // Platform background mode
  useEffect(() => {
    void initAudioService().then(() => {
      startAudioService();
    });
    return () => {
      stopAudioService();
    };
  }, []);

  useKeepAwake(true);

  // Session timer (increments while connected)
  useSessionTimer();

  // Ref to signaling to avoid circular dependency with useWebRTC trickle ICE callback
  const signalingRef = useRef<ReturnType<typeof useSignaling> | null>(null);

  // WebRTC — trickle ICE: send candidates to server as they're gathered
  const webrtc = useWebRTC(undefined, (candidate) => {
    const sid = useSessionStore.getState().sessionId;
    if (sid && signalingRef.current) {
      signalingRef.current.sendIceCandidate(sid, candidate);
    }
  });

  // Audio level detection
  const levels = useAudioLevel(
    webrtc.localStream ? (webrtc as unknown as { pc: Parameters<typeof useAudioLevel>[0] }).pc : null
  );

  useEffect(() => {
    setLocalSpeaking(levels.localSpeaking);
    setPeerSpeaking(levels.peerSpeaking);
  }, [levels.localSpeaking, levels.peerSpeaking, setLocalSpeaking, setPeerSpeaking]);

  // Reconnection state machine
  const reconnection = useReconnection({
    maxAttempts: 3,
    baseDelay: 1000,
    onReconnect: async () => {
      signaling.reconnect();
      const sessionIdVal = useSessionStore.getState().sessionId;
      if (sessionIdVal) {
        const offer = await webrtc.createOffer({ iceRestart: true });
        signaling.sendOffer(sessionIdVal, offer.sdp ?? '');
      }
    },
    onFailed: () => {
      setConnectionState('failed');
    },
  });

  // Signaling (WebSocket)
  const signaling = useSignaling({
    serverUrl,
    roomId,
    onJoined: (newSessionId, incomingPeerName) => {
      console.log('[Conv] joined, sessionId:', newSessionId);
      connect(roomId, shortCode, newSessionId, localLang, peerLang);
      if (incomingPeerName) setPeerName(incomingPeerName);
    },
    onAnswer: (sdp) => {
      console.log('[Conv] received answer, sdp length:', sdp.length);
      void webrtc.setRemoteAnswer(sdp);
      reconnection.reset();
    },
    onIceCandidate: (candidate) => {
      console.log('[Conv] received ICE candidate');
      void webrtc.addIceCandidate(candidate);
    },
    onPeerLeft: (_peerSessionId) => {
      setConnectionState('idle');
      setPeerName('');
    },
    onPeerJoined: (incomingName) => {
      setPeerName(incomingName);
    },
    onRoomClosed: (_reason) => {
      disconnect();
      navigation.replace('Home');
    },
    onError: (message) => {
      console.error('[Conv] server error:', message);
    },
    onTranscript: (text) => {
      setLastTranscript(text);
    },
  });

  // Wire signalingRef so the trickle ICE callback in useWebRTC can reach signaling
  signalingRef.current = signaling;

  // Join the room once the WebSocket is connected
  const hasJoinedRef = useRef(false);
  useEffect(() => {
    if (signaling.isConnected && !hasJoinedRef.current) {
      hasJoinedRef.current = true;
      setConnectionState('connecting');
      signaling.sendJoin(userId, localLang, name);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signaling.isConnected]);

  // Send WebRTC offer once joined (sessionId set) AND local audio stream is ready.
  // This prevents sending an empty SDP when getUserMedia hasn't resolved yet.
  const hasSentOfferRef = useRef(false);
  useEffect(() => {
    if (sessionId && webrtc.localStream && !hasSentOfferRef.current) {
      hasSentOfferRef.current = true;
      void webrtc.createOffer().then((offer) => {
        console.log('[Conv] sending offer, sdp length:', offer.sdp?.length);
        signaling.sendOffer(sessionId, offer.sdp ?? '');
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, webrtc.localStream]);

  // Trigger reconnection on WS close
  useEffect(() => {
    if (!signaling.isConnected && connectionState === 'connected') {
      setConnectionState('reconnecting');
      reconnection.trigger();
    }
  }, [signaling.isConnected, connectionState, reconnection, setConnectionState]);

  const handleMuteToggle = useCallback(() => {
    setMuted(!isMuted);
  }, [isMuted, setMuted]);

  const handleEndCall = useCallback(() => {
    reconnection.cancel();
    if (sessionId) {
      signaling.sendLeave(sessionId);
    }
    webrtc.close();
    signaling.close();
    disconnect();
    navigation.replace('Home');
  }, [sessionId, signaling, webrtc, disconnect, reconnection, navigation]);

  return (
    <SafeAreaView style={styles.container}>
      {/* Connection status indicator */}
      <View style={styles.statusRow}>
        <ConnectionStatus connectionState={connectionState} />
        <SessionTimer />
      </View>

      {/* Error banner */}
      <PipelineErrorBanner
        pipelineError={pipelineError}
        consecutiveErrors={consecutiveErrors}
      />

      {/* VU meters */}
      <View style={styles.vuRow}>
        <VUMeter
          speaking={localSpeaking}
          label={name || 'Vos'}
          testID="vu-local"
        />
        <VUMeter
          speaking={peerSpeaking}
          label={peerName || 'Ellos'}
          testID="vu-peer"
        />
      </View>

      {/* Subtítulos */}
      <View style={styles.subtitleArea}>
        {lastTranscript ? (
          <Text style={styles.subtitleText}>{lastTranscript}</Text>
        ) : (
          <Text style={styles.subtitlePlaceholder}>
            {connectionState === 'connected' ? 'Escuchando...' : ''}
          </Text>
        )}
      </View>

      {/* Controls */}
      <View style={styles.controls}>
        {Platform.OS === 'android' || Platform.OS === 'ios' ? (
          <MuteButton isMuted={isMuted} onToggle={handleMuteToggle} />
        ) : null}
        <EndCallButton onConfirm={handleEndCall} />
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0f0f0f',
    padding: 16,
  },
  statusRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  vuRow: {
    flex: 1,
    flexDirection: 'row',
    justifyContent: 'space-around',
    alignItems: 'center',
  },
  subtitleArea: {
    minHeight: 80,
    justifyContent: 'center',
    alignItems: 'center',
    marginHorizontal: -16,
    paddingHorizontal: 24,
    paddingVertical: 20,
    backgroundColor: 'rgba(0,0,0,0.6)',
  },
  subtitleText: {
    color: '#ffffff',
    fontSize: 20,
    fontWeight: '500',
    textAlign: 'center',
    lineHeight: 28,
  },
  subtitlePlaceholder: {
    color: '#444',
    fontSize: 14,
    textAlign: 'center',
  },
  controls: {
    flexDirection: 'row',
    justifyContent: 'space-around',
    alignItems: 'center',
    paddingVertical: 24,
  },
});
