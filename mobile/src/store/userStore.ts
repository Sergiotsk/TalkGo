import AsyncStorage from '@react-native-async-storage/async-storage';
import { create } from 'zustand';

const KEYS = {
  name: 'user:name',
  localLang: 'user:localLang',
} as const;

interface UserState {
  name: string;
  localLang: string;
  setName: (name: string) => Promise<void>;
  setLocalLang: (lang: string) => Promise<void>;
  hydrate: () => Promise<void>;
}

export const useUserStore = create<UserState>((set) => ({
  name: '',
  localLang: 'es',

  setName: async (name: string) => {
    set({ name });
    await AsyncStorage.setItem(KEYS.name, name);
  },

  setLocalLang: async (lang: string) => {
    set({ localLang: lang });
    await AsyncStorage.setItem(KEYS.localLang, lang);
  },

  hydrate: async () => {
    const [name, localLang] = await Promise.all([
      AsyncStorage.getItem(KEYS.name),
      AsyncStorage.getItem(KEYS.localLang),
    ]);
    set({
      name: name ?? '',
      localLang: localLang ?? 'es',
    });
  },
}));
