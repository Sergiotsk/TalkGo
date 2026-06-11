// Session-related types for the TalkGo mobile app.

export type { ConnectionState, ReconnectionState } from './signaling';

export interface RoomInfo {
  roomId: string;
  shortCode: string;
  sourceLang: string;
  targetLang: string;
}

export interface SessionInfo {
  sessionId: string;
  userId: string;
  roomId: string;
  localLang: string;
}
