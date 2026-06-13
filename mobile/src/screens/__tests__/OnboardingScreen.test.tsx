import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react-native';

const mockSetName = jest.fn();
const mockSetLocalLang = jest.fn();
const mockReplace = jest.fn();

jest.mock('../../store/userStore', () => ({
  useUserStore: jest.fn(() => ({
    name: '',
    localLang: 'es',
    setName: mockSetName,
    setLocalLang: mockSetLocalLang,
  })),
}));

jest.mock('@react-navigation/native', () => ({
  useNavigation: () => ({ replace: mockReplace }),
}));

// eslint-disable-next-line @typescript-eslint/no-require-imports
const { OnboardingScreen } = require('../OnboardingScreen');

describe('OnboardingScreen', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders welcome title', () => {
    render(<OnboardingScreen />);
    expect(screen.getByText(/bienvenido/i)).toBeTruthy();
  });

  it('renders 3 informational bullets', () => {
    render(<OnboardingScreen />);
    expect(screen.getByText(/tiempo real/i)).toBeTruthy();
    expect(screen.getByText(/privacidad/i)).toBeTruthy();
    expect(screen.getByText(/sin cuenta/i)).toBeTruthy();
  });

  it('renders language options: ES, PT, EN, FR', () => {
    render(<OnboardingScreen />);
    expect(screen.getByText('ES')).toBeTruthy();
    expect(screen.getByText('PT')).toBeTruthy();
    expect(screen.getByText('EN')).toBeTruthy();
    expect(screen.getByText('FR')).toBeTruthy();
  });

  it('renders name input', () => {
    render(<OnboardingScreen />);
    expect(screen.getByPlaceholderText(/nombre/i)).toBeTruthy();
  });

  it('selecting a language calls setLocalLang', () => {
    render(<OnboardingScreen />);
    fireEvent.press(screen.getByText('PT'));
    expect(mockSetLocalLang).toHaveBeenCalledWith('pt');
  });

  it('Continuar button is disabled when name is empty', () => {
    render(<OnboardingScreen />);
    const btn = screen.getByText(/continuar/i);
    fireEvent.press(btn);
    // Should NOT navigate when name is empty
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it('typing name and pressing Continuar calls setName and navigates to Home', async () => {
    mockSetName.mockResolvedValue(undefined);
    render(<OnboardingScreen />);
    const input = screen.getByPlaceholderText(/nombre/i);
    fireEvent.changeText(input, 'Alice');
    fireEvent.press(screen.getByText(/continuar/i));
    expect(mockSetName).toHaveBeenCalledWith('Alice');
    // navigate after async setName
    await Promise.resolve();
    expect(mockReplace).toHaveBeenCalledWith('Home');
  });
});
