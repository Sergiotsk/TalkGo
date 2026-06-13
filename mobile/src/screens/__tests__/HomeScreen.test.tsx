import React from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react-native';

const mockCreateRoom = jest.fn();
const mockFindRoomByCode = jest.fn();
const mockNavigate = jest.fn();

// ApiError must be defined inside the factory (jest.mock is hoisted)
jest.mock('../../services/api', () => {
  class MockApiError extends Error {
    statusCode: number;
    constructor(statusCode: number, message: string) {
      super(message);
      this.name = 'ApiError';
      this.statusCode = statusCode;
    }
  }
  return {
    createRoom: (...args: unknown[]) => mockCreateRoom(...args),
    findRoomByCode: (...args: unknown[]) => mockFindRoomByCode(...args),
    ApiError: MockApiError,
  };
});

class ApiError extends Error {
  statusCode: number;
  constructor(statusCode: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.statusCode = statusCode;
  }
}

jest.mock('../../store/userStore', () => ({
  useUserStore: jest.fn(() => ({
    name: 'Alice',
    localLang: 'es',
  })),
}));

jest.mock('@react-navigation/native', () => ({
  useNavigation: () => ({ navigate: mockNavigate }),
}));

// eslint-disable-next-line @typescript-eslint/no-require-imports
const { HomeScreen } = require('../HomeScreen');

describe('HomeScreen — Crear sala', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders Crear sala button', () => {
    render(<HomeScreen />);
    expect(screen.getByText(/crear sala/i)).toBeTruthy();
  });

  it('shows loading state while creating room', async () => {
    mockCreateRoom.mockReturnValue(new Promise(() => {})); // never resolves
    render(<HomeScreen />);
    fireEvent.press(screen.getByText(/crear sala/i));
    await waitFor(() => {
      expect(screen.getByText(/creando/i)).toBeTruthy();
    });
  });

  it('calls createRoom with localLang and auto as peerLang', async () => {
    mockCreateRoom.mockResolvedValue({ room_id: 'r1', short_code: 'ABC123' });
    render(<HomeScreen />);
    await act(async () => {
      fireEvent.press(screen.getByText(/crear sala/i));
    });
    expect(mockCreateRoom).toHaveBeenCalledWith('es', 'auto');
  });

  it('shows shortCode after successful room creation', async () => {
    mockCreateRoom.mockResolvedValue({ room_id: 'r1', short_code: 'ABC123' });
    render(<HomeScreen />);
    await act(async () => {
      fireEvent.press(screen.getByText(/crear sala/i));
    });
    expect(screen.getByText('ABC123')).toBeTruthy();
  });

  it('navigates to Conversation with correct params after room creation', async () => {
    mockCreateRoom.mockResolvedValue({ room_id: 'r1', short_code: 'XYZ999' });
    render(<HomeScreen />);
    await act(async () => {
      fireEvent.press(screen.getByText(/crear sala/i));
    });
    // Press join button shown after creation
    await act(async () => {
      fireEvent.press(screen.getByText(/unirme/i));
    });
    expect(mockNavigate).toHaveBeenCalledWith(
      'Conversation',
      expect.objectContaining({ roomId: 'r1', shortCode: 'XYZ999', localLang: 'es' })
    );
  });

  it('shows error message on network failure', async () => {
    mockCreateRoom.mockRejectedValue(new Error('Network error'));
    render(<HomeScreen />);
    await act(async () => {
      fireEvent.press(screen.getByText(/crear sala/i));
    });
    expect(screen.getByText(/error/i)).toBeTruthy();
  });
});

describe('HomeScreen — Unirse a sala', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders code input with 6 char limit', () => {
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    expect(input).toBeTruthy();
  });

  it('Unirse button is disabled when code is less than 6 chars', () => {
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    fireEvent.changeText(input, 'ABC');
    fireEvent.press(screen.getByText(/^unirse$/i));
    expect(mockFindRoomByCode).not.toHaveBeenCalled();
  });

  it('shows loading while finding room', async () => {
    mockFindRoomByCode.mockReturnValue(new Promise(() => {}));
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    fireEvent.changeText(input, 'ABC123');
    fireEvent.press(screen.getByText(/^unirse$/i));
    await waitFor(() => {
      expect(screen.getByText(/buscando/i)).toBeTruthy();
    });
  });

  it('calls findRoomByCode with uppercase code', async () => {
    mockFindRoomByCode.mockResolvedValue({ room_id: 'r2', short_code: 'abc123' });
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    fireEvent.changeText(input, 'abc123');
    await act(async () => {
      fireEvent.press(screen.getByText(/^unirse$/i));
    });
    expect(mockFindRoomByCode).toHaveBeenCalledWith('ABC123');
  });

  it('navigates to Conversation on successful join', async () => {
    mockFindRoomByCode.mockResolvedValue({ room_id: 'r2', short_code: 'ABC123' });
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    fireEvent.changeText(input, 'ABC123');
    await act(async () => {
      fireEvent.press(screen.getByText(/^unirse$/i));
    });
    expect(mockNavigate).toHaveBeenCalledWith(
      'Conversation',
      expect.objectContaining({ roomId: 'r2', localLang: 'es' })
    );
  });

  it('shows "Sala no encontrada" on 404', async () => {
    mockFindRoomByCode.mockRejectedValue(new ApiError(404, 'Sala no encontrada. Verificá el código.'));
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    fireEvent.changeText(input, 'ZZZZZZ');
    await act(async () => {
      fireEvent.press(screen.getByText(/^unirse$/i));
    });
    expect(screen.getByText(/no encontrada/i)).toBeTruthy();
  });

  it('shows "Esta sala expiró" on 410', async () => {
    mockFindRoomByCode.mockRejectedValue(new ApiError(410, 'Esta sala expiró. Creá una nueva.'));
    render(<HomeScreen />);
    const input = screen.getByPlaceholderText(/código/i);
    fireEvent.changeText(input, 'OLDONE');
    await act(async () => {
      fireEvent.press(screen.getByText(/^unirse$/i));
    });
    expect(screen.getByText(/expiró/i)).toBeTruthy();
  });
});
