// signalingService.ts — Audio session management bridge.
// Starts/stops platform-specific audio services (iOS AVAudioSession, Android ForegroundService)
// when a signaling session begins and ends.

import { Platform } from 'react-native';

let audioSessionHooks:
  | {
      start: () => void;
      stop: () => void;
    }
  | undefined;

/**
 * initAudioService — dynamically loads platform-specific audio hooks.
 * Must be called before startAudioService / stopAudioService.
 */
export async function initAudioService(): Promise<void> {
  if (Platform.OS === 'ios') {
    const { configureAudioSession, deactivateAudioSession } = await import(
      '../hooks/useAudioSession.ios'
    );
    audioSessionHooks = {
      start: configureAudioSession,
      stop: deactivateAudioSession,
    };
  } else if (Platform.OS === 'android') {
    const { startForegroundService, stopForegroundService } = await import(
      '../hooks/useForegroundService.android'
    );
    audioSessionHooks = {
      start: startForegroundService,
      stop: stopForegroundService,
    };
  }
}

/**
 * startAudioService — activates audio session for the platform.
 * Called when a signaling session is established.
 */
export function startAudioService(): void {
  audioSessionHooks?.start();
}

/**
 * stopAudioService — deactivates audio session for the platform.
 * Called when a signaling session ends.
 */
export function stopAudioService(): void {
  audioSessionHooks?.stop();
}
