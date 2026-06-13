import React from 'react';

export const SafeAreaProvider = ({ children }: { children: React.ReactNode }) =>
  React.createElement(React.Fragment, null, children);

export const SafeAreaView = ({ children }: { children: React.ReactNode }) =>
  React.createElement(React.Fragment, null, children);

export const useSafeAreaInsets = jest.fn(() => ({ top: 0, right: 0, bottom: 0, left: 0 }));
export const initialWindowMetrics = null;
