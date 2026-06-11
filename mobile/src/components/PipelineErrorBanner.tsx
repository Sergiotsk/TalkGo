import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

export interface PipelineErrorBannerProps {
  pipelineError: string | null;
  consecutiveErrors: number;
}

const FALLBACK_THRESHOLD = 3;

/**
 * PipelineErrorBanner — displays a warning banner when there are translation errors.
 * - 1–2 consecutive errors: shows yellow warning banner.
 * - 3+ consecutive errors: shows fallback message "Traducción no disponible temporalmente".
 * - null / 0 errors: not rendered.
 */
export function PipelineErrorBanner({
  pipelineError,
  consecutiveErrors,
}: PipelineErrorBannerProps): React.JSX.Element | null {
  if (!pipelineError && consecutiveErrors === 0) {
    return null;
  }

  const showFallback = consecutiveErrors >= FALLBACK_THRESHOLD;

  return (
    <View
      testID="pipeline-error-banner"
      style={[styles.banner, showFallback ? styles.bannerError : styles.bannerWarning]}
    >
      {showFallback ? (
        <Text style={styles.text}>
          Traducción no disponible temporalmente
        </Text>
      ) : (
        <Text style={styles.text}>
          Error de traducción — mostrando texto original
        </Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    padding: 10,
    borderRadius: 6,
    marginVertical: 4,
    alignItems: 'center',
  },
  bannerWarning: {
    backgroundColor: '#FFF3E0',
    borderLeftWidth: 4,
    borderLeftColor: '#FF9800',
  },
  bannerError: {
    backgroundColor: '#FFEBEE',
    borderLeftWidth: 4,
    borderLeftColor: '#F44336',
  },
  text: {
    fontSize: 13,
    color: '#555',
  },
});
