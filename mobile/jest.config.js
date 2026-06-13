module.exports = {
  preset: 'jest-expo',
  moduleNameMapper: {
    'react-native-webrtc': '<rootDir>/__mocks__/react-native-webrtc.js',
    'expo-keep-awake': '<rootDir>/__mocks__/expo-keep-awake.ts',
    '@react-native-async-storage/async-storage':
      '<rootDir>/__mocks__/@react-native-async-storage/async-storage.ts',
    'react-native-screens': '<rootDir>/__mocks__/react-native-screens.ts',
    'react-native-safe-area-context': '<rootDir>/__mocks__/react-native-safe-area-context.ts',
    '@react-navigation/native': '<rootDir>/__mocks__/@react-navigation/native.ts',
    '@react-navigation/native-stack': '<rootDir>/__mocks__/@react-navigation/native-stack.ts',
  },
  transformIgnorePatterns: [
    'node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|react-native-svg|react-native-webrtc|@react-native-async-storage|expo-modules-core)/)',
  ],
};
