import React from 'react';
import { StyleSheet, Text, View } from 'react-native';
import type { ConnectionState } from '../types/signaling';

export interface ConnectionStatusProps {
  connectionState: ConnectionState;
}

const STATE_LABELS: Record<ConnectionState, string> = {
  idle: 'Desconectado',
  connecting: 'Conectando...',
  connected: 'En línea',
  reconnecting: 'Reconectando...',
  failed: 'Conexión perdida',
};

const STATE_COLORS: Record<ConnectionState, string> = {
  idle: '#9E9E9E',
  connecting: '#FF9800',
  connected: '#4CAF50',
  reconnecting: '#FF9800',
  failed: '#F44336',
};

/**
 * ConnectionStatus — presentational component that displays connection state
 * with a colored indicator dot and descriptive text.
 */
export function ConnectionStatus({
  connectionState,
}: ConnectionStatusProps): React.JSX.Element {
  const label = STATE_LABELS[connectionState];
  const color = STATE_COLORS[connectionState];

  return (
    <View style={styles.container}>
      <View style={[styles.dot, { backgroundColor: color }]} />
      <Text style={[styles.text, { color }]}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  text: {
    fontSize: 14,
    fontWeight: '500',
  },
});
