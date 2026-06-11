import React, { useCallback, useEffect } from 'react';
import { Platform, SafeAreaView, StyleSheet, View } from 'react-native';
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

export interface ConversationScreenProps {
  roomId: string;
  shortCode: string;
  userId: string;
  serverUrl: string;
  localLang: string;
  peerLang: string;
}

/**
 * ConversationScreen — the main active call screen.
 * Composes all hooks and components for a full conversation session:
 * - WebRTC peer connection
 * - WebSocket signaling
 * - Automatic reconnection
 * - Audio level (VAD) detection
 * - Keep-awake
 * - Session timer
 * - Platform background mode (iOS AVAudioSession, Android ForegroundService)
 */
export function ConversationScreen({
  roomId,
  shortCode,
  userId,
  serverUrl,
  localLang,
  peerLang,
}: ConversationScreenProps): React.JSX.Element {
  // Store state
  const connectionState = useSessionStore((s) => s.connectionState);
  const sessionId = useSessionStore((s) => s.sessionId);
  const isMuted = useSessionStore((s) => s.isMuted);
  const localSpeaking = useSessionStore((s) => s.localSpeaking);
  const peerSpeaking = useSessionStore((s) => s.peerSpeaking);
  const pipelineError = useSessionStore((s) => s.pipelineError);
  const consecutiveErrors = useSessionStore((s) => s.consecutiveErrors);

  const {
    connect,
    disconnect,
    setConnectionState,
    setMuted,
    setLocalSpeaking,
    setPeerSpeaking,
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

  // Keep screen on during call
  useKeepAwake();

  // Session timer (increments while connected)
  useSessionTimer();

  // WebRTC
  const webrtc = useWebRTC();

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
    onJoined: (newSessionId) => {
      connect(roomId, shortCode, newSessionId, localLang, peerLang);
      // Send offer after joining
      void webrtc.createOffer().then((offer) => {
        signaling.sendOffer(newSessionId, offer.sdp ?? '');
      });
    },
    onAnswer: (sdp) => {
      void webrtc.setRemoteAnswer(sdp);
      reconnection.reset();
    },
    onIceCandidate: (candidate) => {
      void webrtc.addIceCandidate(candidate);
    },
    onPeerLeft: (_peerSessionId) => {
      // Show peer-left UI — room transitions to idle but doesn't disconnect
      setConnectionState('idle');
    },
    onRoomClosed: (_reason) => {
      disconnect();
    },
    onError: (_message) => {
      // Errors handled by PipelineErrorBanner
    },
  });

  // Join the room on mount
  useEffect(() => {
    setConnectionState('connecting');
    signaling.sendJoin(userId, localLang);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
  }, [sessionId, signaling, webrtc, disconnect, reconnection]);

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
          label="Vos"
          testID="vu-local"
        />
        <VUMeter
          speaking={peerSpeaking}
          label="Ellos"
          testID="vu-peer"
        />
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
    backgroundColor: '#FAFAFA',
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
  controls: {
    flexDirection: 'row',
    justifyContent: 'space-around',
    alignItems: 'center',
    paddingVertical: 24,
  },
});
