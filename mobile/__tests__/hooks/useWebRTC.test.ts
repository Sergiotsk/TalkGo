// TASK-056: Tests for useWebRTC hook

import { renderHook, act } from '@testing-library/react-native';
import { useWebRTC } from '../../src/hooks/useWebRTC';

// react-native-webrtc is mocked via __mocks__/react-native-webrtc.ts

describe('useWebRTC', () => {
  it('creates RTCPeerConnection on mount', () => {
    const { result } = renderHook(() => useWebRTC());
    expect(result.current).toBeDefined();
  });

  it('starts with null localStream and remoteStream', () => {
    const { result } = renderHook(() => useWebRTC());
    expect(result.current.localStream).toBeNull();
    expect(result.current.remoteStream).toBeNull();
  });

  it('starts with new iceConnectionState', () => {
    const { result } = renderHook(() => useWebRTC());
    expect(result.current.iceConnectionState).toBe('new');
  });

  it('createOffer returns an SDP string', async () => {
    const { result } = renderHook(() => useWebRTC());

    let offer: { sdp?: string } | undefined;
    await act(async () => {
      offer = await result.current.createOffer();
    });

    expect(offer).toBeDefined();
    expect(typeof offer?.sdp).toBe('string');
  });

  it('createOffer with iceRestart option passes through', async () => {
    const { result } = renderHook(() => useWebRTC());

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
      await expect(
        result.current.setRemoteAnswer('mock-sdp-answer')
      ).resolves.toBeUndefined();
    });
  });

  it('addIceCandidate does not throw', async () => {
    const { result } = renderHook(() => useWebRTC());

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

  it('unmount cleans up resources', () => {
    const { unmount } = renderHook(() => useWebRTC());

    expect(() => unmount()).not.toThrow();
  });
});
