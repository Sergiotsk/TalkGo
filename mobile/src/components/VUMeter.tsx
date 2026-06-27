import React, { useEffect, useRef } from 'react';
import { Animated, StyleSheet, Text, View } from 'react-native';

export interface VUMeterProps {
  speaking: boolean;
  label: string;
  testID?: string;
}

/**
 * VUMeter — animates a circle indicator based on the speaking boolean.
 * Uses React Native's Animated API for smooth transitions.
 * The speaking prop comes from Zustand via a selector to minimize re-renders.
 */
export function VUMeter({ speaking, label, testID }: VUMeterProps): React.JSX.Element {
  const scaleAnim = useRef(new Animated.Value(1)).current;
  const opacityAnim = useRef(new Animated.Value(0.4)).current;

  useEffect(() => {
    if (speaking) {
      Animated.loop(
        Animated.sequence([
          Animated.parallel([
            Animated.timing(scaleAnim, {
              toValue: 1.3,
              duration: 300,
              useNativeDriver: true,
            }),
            Animated.timing(opacityAnim, {
              toValue: 1,
              duration: 300,
              useNativeDriver: true,
            }),
          ]),
          Animated.parallel([
            Animated.timing(scaleAnim, {
              toValue: 1,
              duration: 300,
              useNativeDriver: true,
            }),
            Animated.timing(opacityAnim, {
              toValue: 0.6,
              duration: 300,
              useNativeDriver: true,
            }),
          ]),
        ])
      ).start();
    } else {
      Animated.parallel([
        Animated.timing(scaleAnim, {
          toValue: 1,
          duration: 200,
          useNativeDriver: true,
        }),
        Animated.timing(opacityAnim, {
          toValue: 0.4,
          duration: 200,
          useNativeDriver: true,
        }),
      ]).start();
    }
  }, [speaking, scaleAnim, opacityAnim]);

  return (
    <View style={styles.container} testID={testID}>
      <Animated.View
        testID="vu-indicator"
        style={[
          styles.circle,
          speaking ? styles.circleActive : styles.circleInactive,
          {
            transform: [{ scale: scaleAnim }],
            opacity: opacityAnim,
          },
        ]}
      />
      <Text style={styles.label}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    alignItems: 'center',
    justifyContent: 'center',
    padding: 8,
  },
  circle: {
    width: 80,
    height: 80,
    borderRadius: 40,
  },
  circleActive: {
    backgroundColor: '#4CAF50',
  },
  circleInactive: {
    backgroundColor: '#2a2a2a',
    borderWidth: 2,
    borderColor: '#444',
  },
  label: {
    marginTop: 12,
    fontSize: 15,
    fontWeight: '600',
    color: '#cccccc',
    letterSpacing: 0.3,
  },
});
