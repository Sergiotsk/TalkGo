// Android-only: useForegroundService
// Manages the CallForegroundService via NativeModules bridge.
// This file uses the .android.ts extension so Metro only bundles it on Android.

import { useEffect } from 'react';
import { NativeModules } from 'react-native';

/**
 * start — starts the CallForegroundService via NativeModules.CallService.
 * The service shows a persistent notification "TalkGo — Conversación activa"
 * and acquires a PARTIAL_WAKE_LOCK to keep audio flowing in background.
 */
export function startForegroundService(): void {
  // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access
  NativeModules.CallService?.start?.();
}

export function stopForegroundService(): void {
  // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access
  NativeModules.CallService?.stop?.();
}

/**
 * useForegroundService — starts the service on mount, stops on unmount.
 * Must be called from ConversationScreen on Android.
 */
export function useForegroundService(): void {
  useEffect(() => {
    startForegroundService();
    return () => {
      stopForegroundService();
    };
  }, []);
}
