import { useEffect } from 'react';
import { useSessionStore } from '../store/sessionStore';

/**
 * useSessionTimer — increments elapsedSeconds in the Zustand store every 1000ms.
 * Only runs while connectionState === 'connected'.
 * Automatically clears on unmount.
 */
export function useSessionTimer(): void {
  const connectionState = useSessionStore((s) => s.connectionState);
  const tick = useSessionStore((s) => s.tick);

  useEffect(() => {
    if (connectionState !== 'connected') return;

    const intervalId = setInterval(() => {
      tick();
    }, 1000);

    return () => {
      clearInterval(intervalId);
    };
  }, [connectionState, tick]);
}
