module.exports = {
  preset: 'react-native',
  moduleNameMapper: {
    '^react-native-webrtc$': '<rootDir>/__mocks__/react-native-webrtc.ts',
    '^react-native-keep-awake$': '<rootDir>/__mocks__/react-native-keep-awake.ts',
  },
  transformIgnorePatterns: [
    'node_modules/(?!(react-native|@react-native|react-native-webrtc|react-native-keep-awake|@react-native-community|zustand)/)',
  ],
  testMatch: [
    '**/__tests__/**/*.{ts,tsx}',
  ],
};
