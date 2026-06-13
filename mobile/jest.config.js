module.exports = {
  preset: 'jest-expo',
  moduleNameMapper: {
    'react-native-webrtc': '<rootDir>/__mocks__/react-native-webrtc.js',
    'expo-keep-awake': '<rootDir>/__mocks__/expo-keep-awake.ts',
  },
  transformIgnorePatterns: [
    'node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|react-native-svg|react-native-webrtc)/)',
  ],
};
