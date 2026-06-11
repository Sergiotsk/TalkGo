import React from 'react';
import { StyleSheet, Text } from 'react-native';
import { useSessionStore } from '../store/sessionStore';

/**
 * SessionTimer — displays elapsed session time in MM:SS format.
 * Subscribes directly to elapsedSeconds via a Zustand selector to
 * ensure re-renders happen only when the timer value changes.
 */
export function SessionTimer(): React.JSX.Element {
  const elapsedSeconds = useSessionStore((s) => s.elapsedSeconds);

  const minutes = Math.floor(elapsedSeconds / 60);
  const seconds = elapsedSeconds % 60;

  const formatted = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;

  return <Text style={styles.text}>{formatted}</Text>;
}

const styles = StyleSheet.create({
  text: {
    fontSize: 24,
    fontWeight: '600',
    fontVariant: ['tabular-nums'],
    color: '#333',
    letterSpacing: 1,
  },
});
