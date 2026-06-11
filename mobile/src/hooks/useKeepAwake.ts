import { useEffect } from 'react';
import KeepAwake from 'react-native-keep-awake';

/**
 * useKeepAwake — thin wrapper around react-native-keep-awake.
 * Activates on mount, deactivates on unmount.
 * Used by ConversationScreen to prevent the screen from sleeping during a call.
 */
export function useKeepAwake(): void {
  useEffect(() => {
    KeepAwake.activate();
    return () => {
      KeepAwake.deactivate();
    };
  }, []);
}
