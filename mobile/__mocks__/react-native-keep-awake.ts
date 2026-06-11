// Mock for react-native-keep-awake
// Requires native linking — mocked for Jest/Windows environments.

const KeepAwake = {
  activate: jest.fn(),
  deactivate: jest.fn(),
};

export const activate = jest.fn();
export const deactivate = jest.fn();

export default KeepAwake;
