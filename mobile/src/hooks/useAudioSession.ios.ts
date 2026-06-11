// iOS-only: useAudioSession
// AVAudioSession configuration is handled natively by react-native-webrtc's
// built-in audio session management. For additional control (e.g., .voiceChat
// mode, .allowBluetooth, .defaultToSpeaker), use the AudioSessionManager native module.
//
// This file uses the .ios.ts extension so Metro only bundles it on iOS.

import { useEffect } from 'react';
import { NativeModules } from 'react-native';

/**
 * configureAudioSession — calls the native AudioSessionManager module.
 * Requires AudioSessionManager.swift + RCT_EXTERN_MODULE bridge in the iOS project.
 */
export function configureAudioSession(): void {
  // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access
  NativeModules.AudioSessionManager?.activate?.();
}

export function deactivateAudioSession(): void {
  // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access
  NativeModules.AudioSessionManager?.deactivate?.();
}

/**
 * useAudioSession — activates AVAudioSession on mount, deactivates on unmount.
 * Must be called from ConversationScreen on iOS.
 */
export function useAudioSession(): void {
  useEffect(() => {
    configureAudioSession();
    return () => {
      deactivateAudioSession();
    };
  }, []);
}
