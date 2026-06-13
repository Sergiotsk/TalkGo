import { act } from '@testing-library/react-native';

// Re-require modules after each reset to get fresh instances
let useUserStore: typeof import('../userStore').useUserStore;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let AsyncStorage: any;

beforeEach(() => {
  jest.resetModules();
  jest.clearAllMocks();
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  AsyncStorage = require('@react-native-async-storage/async-storage').default;
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  useUserStore = require('../userStore').useUserStore;
});

describe('useUserStore', () => {
  it('starts with empty name and default lang', () => {
    const { name, localLang } = useUserStore.getState();
    expect(name).toBe('');
    expect(localLang).toBe('es');
  });

  it('setName updates the store', () => {
    act(() => {
      useUserStore.getState().setName('Alice');
    });
    expect(useUserStore.getState().name).toBe('Alice');
  });

  it('setLocalLang updates the store', () => {
    act(() => {
      useUserStore.getState().setLocalLang('pt');
    });
    expect(useUserStore.getState().localLang).toBe('pt');
  });

  it('setName persists to AsyncStorage', async () => {
    await act(async () => {
      await useUserStore.getState().setName('Bob');
    });
    expect(AsyncStorage.setItem).toHaveBeenCalledWith('user:name', 'Bob');
  });

  it('setLocalLang persists to AsyncStorage', async () => {
    await act(async () => {
      await useUserStore.getState().setLocalLang('en');
    });
    expect(AsyncStorage.setItem).toHaveBeenCalledWith('user:localLang', 'en');
  });

  it('hydrate loads name and lang from AsyncStorage', async () => {
    AsyncStorage.getItem.mockImplementation((key: string) => {
      if (key === 'user:name') return Promise.resolve('Carol');
      if (key === 'user:localLang') return Promise.resolve('fr');
      return Promise.resolve(null);
    });

    await act(async () => {
      await useUserStore.getState().hydrate();
    });

    expect(useUserStore.getState().name).toBe('Carol');
    expect(useUserStore.getState().localLang).toBe('fr');
  });

  it('hydrate with empty AsyncStorage keeps defaults', async () => {
    AsyncStorage.getItem.mockResolvedValue(null);

    await act(async () => {
      await useUserStore.getState().hydrate();
    });

    expect(useUserStore.getState().name).toBe('');
    expect(useUserStore.getState().localLang).toBe('es');
  });
});
