// Mock for react-native-webrtc
// On Windows/CI environments, react-native-webrtc requires native linking.
// This mock provides a complete Jest-compatible substitute.

export type RTCIceConnectionState =
  | 'new'
  | 'checking'
  | 'connected'
  | 'completed'
  | 'failed'
  | 'disconnected'
  | 'closed';

export interface RTCIceServer {
  urls: string | string[];
  username?: string;
  credential?: string;
}

export interface RTCConfiguration {
  iceServers?: RTCIceServer[];
}

export interface RTCOfferOptions {
  iceRestart?: boolean;
  offerToReceiveAudio?: boolean;
  offerToReceiveVideo?: boolean;
}

export interface RTCSessionDescriptionInit {
  type: 'offer' | 'answer' | 'pranswer' | 'rollback';
  sdp?: string;
}

export interface RTCIceCandidateInit {
  candidate?: string;
  sdpMLineIndex?: number | null;
  sdpMid?: string | null;
}

class MockRTCPeerConnection {
  private _iceConnectionState: RTCIceConnectionState = 'new';
  ontrack: ((event: { streams: MediaStream[] }) => void) | null = null;
  oniceconnectionstatechange: (() => void) | null = null;
  onicecandidate: ((event: { candidate: RTCIceCandidateInit | null }) => void) | null = null;
  localDescription: RTCSessionDescriptionInit | null = null;
  remoteDescription: RTCSessionDescriptionInit | null = null;

  get iceConnectionState(): RTCIceConnectionState {
    return this._iceConnectionState;
  }

  addStream(_stream: MediaStream): void {}

  createOffer(_options?: RTCOfferOptions): Promise<RTCSessionDescriptionInit> {
    return Promise.resolve({ type: 'offer', sdp: 'mock-sdp-offer' });
  }

  createAnswer(): Promise<RTCSessionDescriptionInit> {
    return Promise.resolve({ type: 'answer', sdp: 'mock-sdp-answer' });
  }

  setLocalDescription(desc: RTCSessionDescriptionInit): Promise<void> {
    this.localDescription = desc;
    return Promise.resolve();
  }

  setRemoteDescription(desc: RTCSessionDescriptionInit): Promise<void> {
    this.remoteDescription = desc;
    return Promise.resolve();
  }

  addIceCandidate(_candidate: RTCIceCandidateInit): Promise<void> {
    return Promise.resolve();
  }

  getStats(): Promise<Map<string, Record<string, unknown>>> {
    const statsMap = new Map<string, Record<string, unknown>>();
    statsMap.set('outbound-1', {
      type: 'outbound-rtp',
      kind: 'audio',
      voiceActivityFlag: false,
      audioLevel: 0,
    });
    statsMap.set('inbound-1', {
      type: 'inbound-rtp',
      kind: 'audio',
      voiceActivityFlag: false,
      audioLevel: 0,
    });
    return Promise.resolve(statsMap);
  }

  close(): void {
    this._iceConnectionState = 'closed';
  }

  // Allow tests to simulate state changes
  _setIceConnectionState(state: RTCIceConnectionState): void {
    this._iceConnectionState = state;
    if (this.oniceconnectionstatechange) {
      this.oniceconnectionstatechange();
    }
  }
}

export class RTCPeerConnection extends MockRTCPeerConnection {}

export class RTCSessionDescription {
  type: string;
  sdp: string;
  constructor(init: RTCSessionDescriptionInit) {
    this.type = init.type;
    this.sdp = init.sdp ?? '';
  }
}

export class RTCIceCandidate {
  candidate: string;
  sdpMLineIndex: number | null;
  sdpMid: string | null;
  constructor(init: RTCIceCandidateInit) {
    this.candidate = init.candidate ?? '';
    this.sdpMLineIndex = init.sdpMLineIndex ?? null;
    this.sdpMid = init.sdpMid ?? null;
  }
}

export interface MediaTrack {
  stop(): void;
  enabled: boolean;
  kind: string;
  id: string;
}

export interface MediaStream {
  id: string;
  getTracks(): MediaTrack[];
  getAudioTracks(): MediaTrack[];
  getVideoTracks(): MediaTrack[];
  addTrack(_track: MediaTrack): void;
  removeTrack(_track: MediaTrack): void;
}

export const mediaDevices = {
  getUserMedia: jest.fn(
    (_constraints: { audio: boolean; video: boolean }): Promise<MediaStream> => {
      const mockTrack: MediaTrack = {
        stop: jest.fn(),
        enabled: true,
        kind: 'audio',
        id: 'mock-track-id',
      };
      const mockStream: MediaStream = {
        id: 'mock-stream-id',
        getTracks: () => [mockTrack],
        getAudioTracks: () => [mockTrack],
        getVideoTracks: () => [],
        addTrack: jest.fn(),
        removeTrack: jest.fn(),
      };
      return Promise.resolve(mockStream);
    }
  ),
};

export const registerGlobals = jest.fn();
