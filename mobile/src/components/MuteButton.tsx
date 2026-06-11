import React from 'react';
import { StyleSheet, Text, TouchableOpacity } from 'react-native';

export interface MuteButtonProps {
  isMuted: boolean;
  onToggle: () => void;
}

/**
 * MuteButton — toggle button for microphone mute state.
 * Uses distinct testIDs to allow test assertions on muted/unmuted state.
 */
export function MuteButton({ isMuted, onToggle }: MuteButtonProps): React.JSX.Element {
  return (
    <TouchableOpacity
      style={[styles.button, isMuted && styles.buttonMuted]}
      onPress={onToggle}
      testID={isMuted ? 'mute-button-muted' : 'mute-button-unmuted'}
      accessibilityLabel={isMuted ? 'Activar micrófono' : 'Silenciar micrófono'}
      accessibilityRole="button"
    >
      <Text style={styles.icon}>{isMuted ? '🔇' : '🎤'}</Text>
      <Text style={styles.label}>{isMuted ? 'Silenciado' : 'Micrófono'}</Text>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  button: {
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#E0E0E0',
    borderRadius: 40,
    width: 80,
    height: 80,
    padding: 8,
  },
  buttonMuted: {
    backgroundColor: '#F44336',
  },
  icon: {
    fontSize: 24,
  },
  label: {
    fontSize: 10,
    marginTop: 2,
  },
});
