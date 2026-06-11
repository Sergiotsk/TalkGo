// TASK-050: Tests for sessionStore
// TDD: tests written before implementation

import { act } from 'react-test-renderer';
import { useSessionStore } from '../../src/store/sessionStore';

// Reset store state between tests
beforeEach(() => {
  act(() => {
    useSessionStore.getState().disconnect();
  });
});

describe('sessionStore', () => {
  describe('connect', () => {
    it('sets all fields and connectionState to connected', () => {
      act(() => {
        useSessionStore.getState().connect('room-1', 'ABC123', 'sess-1', 'es', 'en');
      });

      const state = useSessionStore.getState();
      expect(state.connectionState).toBe('connected');
      expect(state.roomId).toBe('room-1');
      expect(state.shortCode).toBe('ABC123');
      expect(state.sessionId).toBe('sess-1');
      expect(state.localLang).toBe('es');
      expect(state.peerLang).toBe('en');
    });
  });

  describe('disconnect', () => {
    it('resets all state to idle/null', () => {
      act(() => {
        useSessionStore.getState().connect('room-1', 'ABC123', 'sess-1', 'es', 'en');
        useSessionStore.getState().disconnect();
      });

      const state = useSessionStore.getState();
      expect(state.connectionState).toBe('idle');
      expect(state.roomId).toBeNull();
      expect(state.shortCode).toBeNull();
      expect(state.sessionId).toBeNull();
      expect(state.elapsedSeconds).toBe(0);
      expect(state.consecutiveErrors).toBe(0);
    });
  });

  describe('tick', () => {
    it('increments elapsedSeconds by 1 each call', () => {
      act(() => {
        useSessionStore.getState().tick();
        useSessionStore.getState().tick();
        useSessionStore.getState().tick();
      });

      expect(useSessionStore.getState().elapsedSeconds).toBe(3);
    });
  });

  describe('incrementErrors', () => {
    it('increments consecutiveErrors from 0 to 3', () => {
      act(() => {
        useSessionStore.getState().incrementErrors();
        useSessionStore.getState().incrementErrors();
        useSessionStore.getState().incrementErrors();
      });

      expect(useSessionStore.getState().consecutiveErrors).toBe(3);
    });
  });

  describe('resetErrors', () => {
    it('resets consecutiveErrors to 0 and clears pipelineError', () => {
      act(() => {
        useSessionStore.getState().incrementErrors();
        useSessionStore.getState().setPipelineError('translation failed');
        useSessionStore.getState().resetErrors();
      });

      const state = useSessionStore.getState();
      expect(state.consecutiveErrors).toBe(0);
      expect(state.pipelineError).toBeNull();
    });
  });

  describe('setMuted', () => {
    it('toggles isMuted', () => {
      act(() => {
        useSessionStore.getState().setMuted(true);
      });
      expect(useSessionStore.getState().isMuted).toBe(true);

      act(() => {
        useSessionStore.getState().setMuted(false);
      });
      expect(useSessionStore.getState().isMuted).toBe(false);
    });
  });

  describe('setLocalSpeaking / setPeerSpeaking', () => {
    it('stores boolean speaking state', () => {
      act(() => {
        useSessionStore.getState().setLocalSpeaking(true);
        useSessionStore.getState().setPeerSpeaking(true);
      });

      const state = useSessionStore.getState();
      expect(state.localSpeaking).toBe(true);
      expect(state.peerSpeaking).toBe(true);
    });
  });

  describe('setConnectionState', () => {
    it.each([
      ['connecting'],
      ['connected'],
      ['reconnecting'],
      ['failed'],
      ['idle'],
    ] as const)('sets connectionState to %s', (targetState) => {
      act(() => {
        useSessionStore.getState().setConnectionState(targetState);
      });
      expect(useSessionStore.getState().connectionState).toBe(targetState);
    });
  });
});
