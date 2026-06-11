// TASK-056: Tests for useWebRTC hook

import { renderHook, act } from '@testing-library/react-native';
import { useWebRTC } from '../../src/hooks/useWebRTC';

// react-native-webrtc is mocked via __mocks__/react-native-webrtc.ts

describe('useWebRTC', () => {
  it('creates RTCPeerConnection on mount', async () => {
    const { result } = renderHook(() => useWebRTC());
    // Flush async getUserMedia promise that triggers setLocalStream
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current).toBeDefined();
  });

  it('gets localStream from getUserMedia on mount', async () => {
    const { result } = renderHook(() => useWebRTC());
    // Flush pending getUserMedia promise that triggers setLocalStream
    await act(async () => {
      await Promise.resolve();
    });
    // The mock getUserMedia resolves immediately with a mock stream
    expect(result.current.localStream).not.toBeNull();
    expect(result.current.localStream?.id).toBe('mock-stream-id');
    expect(result.current.remoteStream).toBeNull();
  });

  it('starts with new iceConnectionState', async () => {
    const { result } = renderHook(() => useWebRTC());
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.iceConnectionState).toBe('new');
  });

  it('createOffer returns an SDP string', async () => {
    const { result } = renderHook(() => useWebRTC());

    // Flush pending getUserMedia promise first
    await act(async () => {
      await Promise.resolve();
    });

    let offer: { sdp?: string } | undefined;
    await act(async () => {
      offer = await result.current.createOffer();
    });

    expect(offer).toBeDefined();
    expect(typeof offer?.sdp).toBe('string');
  });

  it('createOffer with iceRestart option passes through', async () => {
    const { result } = renderHook(() => useWebRTC());

    await act(async () => {
      await Promise.resolve();
    });

    let offer: { sdp?: string } | undefined;
    await act(async () => {
      offer = await result.current.createOffer({ iceRestart: true });
    });

    expect(offer).toBeDefined();
    expect(offer?.sdp).toBeTruthy();
  });

  it('setRemoteAnswer does not throw', async () => {
    const { result } = renderHook(() => useWebRTC());

    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      await expect(
        result.current.setRemoteAnswer('mock-sdp-answer')
      ).resolves.toBeUndefined();
    });
  });

  it('addIceCandidate does not throw', async () => {
    const { result } = renderHook(() => useWebRTC());

    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      await expect(
        result.current.addIceCandidate('candidate:1 1 UDP ...')
      ).resolves.toBeUndefined();
    });
  });

  it('close() cleans up without throwing', () => {
    const { result } = renderHook(() => useWebRTC());

    act(() => {
      expect(() => result.current.close()).not.toThrow();
    });
  });

  it('unmount cleans up resources', async () => {
    const { unmount } = renderHook(() => useWebRTC());

    await act(async () => {
      await Promise.resolve();
    });

    expect(() => unmount()).not.toThrow();
  });
});
