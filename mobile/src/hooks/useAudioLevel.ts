import { useEffect, useState } from 'react';
import type { RTCPeerConnection } from 'react-native-webrtc';

export interface AudioLevels {
  localSpeaking: boolean;
  peerSpeaking: boolean;
}

const AUDIO_LEVEL_THRESHOLD = 0.01;

/**
 * useAudioLevel polls RTCPeerConnection.getStats() at the given interval (default 100ms = 10Hz)
 * and returns boolean speaking indicators for local and peer.
 *
 * Design decision (AD-09): Using boolean in Zustand reduces re-renders from
 * continuous 10Hz updates to event-based transitions (speaking ↔ silent).
 */
export function useAudioLevel(
  peerConnection: RTCPeerConnection | null,
  intervalMs = 100
): AudioLevels {
  const [levels, setLevels] = useState<AudioLevels>({
    localSpeaking: false,
    peerSpeaking: false,
  });

  useEffect(() => {
    if (!peerConnection) return;

    const intervalId = setInterval(() => {
      peerConnection
        .getStats()
        .then((stats: Map<string, Record<string, unknown>>) => {
          let localVAD = false;
          let remoteVAD = false;

          stats.forEach((report) => {
            if (report.kind !== 'audio') return;

            if (report.type === 'outbound-rtp') {
              if (typeof report.voiceActivityFlag === 'boolean') {
                localVAD = report.voiceActivityFlag;
              } else if (typeof report.audioLevel === 'number') {
                // Fallback: threshold-based VAD
                localVAD = report.audioLevel > AUDIO_LEVEL_THRESHOLD;
              }
            }

            if (report.type === 'inbound-rtp') {
              if (typeof report.voiceActivityFlag === 'boolean') {
                remoteVAD = report.voiceActivityFlag;
              } else if (typeof report.audioLevel === 'number') {
                remoteVAD = report.audioLevel > AUDIO_LEVEL_THRESHOLD;
              }
            }
          });

          // Only update state if values changed (avoid unnecessary re-renders)
          setLevels((prev) => {
            if (
              prev.localSpeaking === localVAD &&
              prev.peerSpeaking === remoteVAD
            ) {
              return prev;
            }
            return { localSpeaking: localVAD, peerSpeaking: remoteVAD };
          });
        })
        .catch(() => {
          // getStats failed — silently ignore (PC may be closing)
        });
    }, intervalMs);

    return () => {
      clearInterval(intervalId);
    };
  }, [peerConnection, intervalMs]);

  return levels;
}
