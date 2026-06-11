// TASK-060: Tests for useAudioLevel hook

import { renderHook, act } from '@testing-library/react-native';
import { useAudioLevel } from '../../src/hooks/useAudioLevel';
import type { RTCPeerConnection } from 'react-native-webrtc';

describe('useAudioLevel', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('returns both false when peerConnection is null', () => {
    const { result } = renderHook(() => useAudioLevel(null));

    expect(result.current.localSpeaking).toBe(false);
    expect(result.current.peerSpeaking).toBe(false);
  });

  it('returns localSpeaking=true when voiceActivityFlag=true on outbound-rtp', async () => {
    const mockStats = new Map<string, Record<string, unknown>>();
    mockStats.set('outbound-1', {
      type: 'outbound-rtp',
      kind: 'audio',
      voiceActivityFlag: true,
      audioLevel: 0.8,
    });
    mockStats.set('inbound-1', {
      type: 'inbound-rtp',
      kind: 'audio',
      voiceActivityFlag: false,
      audioLevel: 0,
    });

    const mockPc = {
      getStats: jest.fn().mockResolvedValue(mockStats),
    } as unknown as RTCPeerConnection;

    const { result } = renderHook(() => useAudioLevel(mockPc, 100));

    await act(async () => {
      jest.advanceTimersByTime(100);
      await Promise.resolve();
    });

    expect(result.current.localSpeaking).toBe(true);
    expect(result.current.peerSpeaking).toBe(false);
  });

  it('returns peerSpeaking=true when voiceActivityFlag=true on inbound-rtp', async () => {
    const mockStats = new Map<string, Record<string, unknown>>();
    mockStats.set('outbound-1', {
      type: 'outbound-rtp',
      kind: 'audio',
      voiceActivityFlag: false,
      audioLevel: 0,
    });
    mockStats.set('inbound-1', {
      type: 'inbound-rtp',
      kind: 'audio',
      voiceActivityFlag: true,
      audioLevel: 0.9,
    });

    const mockPc = {
      getStats: jest.fn().mockResolvedValue(mockStats),
    } as unknown as RTCPeerConnection;

    const { result } = renderHook(() => useAudioLevel(mockPc, 100));

    await act(async () => {
      jest.advanceTimersByTime(100);
      await Promise.resolve();
    });

    expect(result.current.peerSpeaking).toBe(true);
  });

  it('falls back to audioLevel threshold when voiceActivityFlag is absent', async () => {
    const mockStats = new Map<string, Record<string, unknown>>();
    mockStats.set('outbound-1', {
      type: 'outbound-rtp',
      kind: 'audio',
      audioLevel: 0.05, // above threshold of 0.01 → speaking
    });

    const mockPc = {
      getStats: jest.fn().mockResolvedValue(mockStats),
    } as unknown as RTCPeerConnection;

    const { result } = renderHook(() => useAudioLevel(mockPc, 100));

    await act(async () => {
      jest.advanceTimersByTime(100);
      await Promise.resolve();
    });

    expect(result.current.localSpeaking).toBe(true);
  });

  it('returns false for silence — audioLevel=0', async () => {
    const mockStats = new Map<string, Record<string, unknown>>();
    mockStats.set('outbound-1', {
      type: 'outbound-rtp',
      kind: 'audio',
      voiceActivityFlag: false,
      audioLevel: 0,
    });

    const mockPc = {
      getStats: jest.fn().mockResolvedValue(mockStats),
    } as unknown as RTCPeerConnection;

    const { result } = renderHook(() => useAudioLevel(mockPc, 100));

    await act(async () => {
      jest.advanceTimersByTime(100);
      await Promise.resolve();
    });

    expect(result.current.localSpeaking).toBe(false);
  });

  it('clears interval on unmount', () => {
    const mockPc = {
      getStats: jest.fn().mockResolvedValue(new Map()),
    } as unknown as RTCPeerConnection;

    const { unmount } = renderHook(() => useAudioLevel(mockPc, 100));

    const clearIntervalSpy = jest.spyOn(global, 'clearInterval');
    unmount();
    expect(clearIntervalSpy).toHaveBeenCalled();
    clearIntervalSpy.mockRestore();
  });
});
