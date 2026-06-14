import { create } from 'zustand';
import type { ConnectionState } from '../types/signaling';

// --- State shape ---

interface SessionState {
  // Connection
  connectionState: ConnectionState;
  roomId: string | null;
  shortCode: string | null;
  sessionId: string | null;

  // Languages
  localLang: string;
  peerLang: string;

  // Audio state — boolean (not float) to minimize re-renders at 10Hz
  isMuted: boolean;
  localSpeaking: boolean;
  peerSpeaking: boolean;

  // Error state
  pipelineError: string | null;
  consecutiveErrors: number;

  // Timer
  elapsedSeconds: number;

  // Reconnection
  reconnectAttempt: number;

  // Transcription
  lastTranscript: string | null;
}

// --- Actions ---

interface SessionActions {
  // Lifecycle
  connect: (
    roomId: string,
    shortCode: string,
    sessionId: string,
    localLang: string,
    peerLang: string
  ) => void;
  disconnect: () => void;
  setConnectionState: (state: ConnectionState) => void;

  // Audio
  setMuted: (muted: boolean) => void;
  setLocalSpeaking: (speaking: boolean) => void;
  setPeerSpeaking: (speaking: boolean) => void;

  // Errors
  setPipelineError: (error: string | null) => void;
  incrementErrors: () => void;
  resetErrors: () => void;

  // Timer
  tick: () => void;
  resetTimer: () => void;

  // Reconnection
  setReconnectAttempt: (attempt: number) => void;

  // Transcription
  setLastTranscript: (text: string) => void;
  clearTranscript: () => void;
}

export type SessionStore = SessionState & SessionActions;

// --- Initial state ---

const initialState: SessionState = {
  connectionState: 'idle',
  roomId: null,
  shortCode: null,
  sessionId: null,
  localLang: '',
  peerLang: '',
  isMuted: false,
  localSpeaking: false,
  peerSpeaking: false,
  pipelineError: null,
  consecutiveErrors: 0,
  elapsedSeconds: 0,
  reconnectAttempt: 0,
  lastTranscript: null,
};

// --- Store ---

export const useSessionStore = create<SessionStore>((set) => ({
  ...initialState,

  connect: (roomId, shortCode, sessionId, localLang, peerLang) =>
    set({
      connectionState: 'connected',
      roomId,
      shortCode,
      sessionId,
      localLang,
      peerLang,
    }),

  disconnect: () => set({ ...initialState }),

  setConnectionState: (state) => set({ connectionState: state }),

  setMuted: (muted) => set({ isMuted: muted }),

  setLocalSpeaking: (speaking) => set({ localSpeaking: speaking }),

  setPeerSpeaking: (speaking) => set({ peerSpeaking: speaking }),

  setPipelineError: (error) => set({ pipelineError: error }),

  incrementErrors: () =>
    set((s) => ({ consecutiveErrors: s.consecutiveErrors + 1 })),

  resetErrors: () => set({ consecutiveErrors: 0, pipelineError: null }),

  tick: () => set((s) => ({ elapsedSeconds: s.elapsedSeconds + 1 })),

  resetTimer: () => set({ elapsedSeconds: 0 }),

  setReconnectAttempt: (attempt) => set({ reconnectAttempt: attempt }),

  setLastTranscript: (text) => set({ lastTranscript: text }),
  clearTranscript: () => set({ lastTranscript: null }),
}));
