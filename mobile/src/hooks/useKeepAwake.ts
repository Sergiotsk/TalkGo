import { useKeepAwake as useExpoKeepAwake } from 'expo-keep-awake';

/**
 * useKeepAwake — thin wrapper around expo-keep-awake.
 * Activates screen keep-awake when `active` is true.
 * Used by ConversationScreen to prevent the screen from sleeping during a call.
 */
export function useKeepAwake(active: boolean): void {
  if (active) {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useExpoKeepAwake();
  }
}
