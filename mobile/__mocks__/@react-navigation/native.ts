import React from 'react';

export const NavigationContainer = ({ children }: { children: React.ReactNode }) =>
  React.createElement(React.Fragment, null, children);

export const useNavigation = jest.fn(() => ({
  navigate: jest.fn(),
  replace: jest.fn(),
  goBack: jest.fn(),
  reset: jest.fn(),
}));

export const useRoute = jest.fn(() => ({ params: {} }));
export const useFocusEffect = jest.fn((cb: () => void) => cb());
