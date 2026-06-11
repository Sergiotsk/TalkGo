// Signaling message types — must match the Go SignalingMessage struct exactly.

export type ConnectionState =
  | 'idle'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'failed';

export type ReconnectionState = 'connected' | 'reconnecting' | 'failed';

// Outgoing messages (mobile → server)
export interface JoinMessage {
  type: 'join';
  room_id: string;
  user_id: string;
  lang: string;
}

export interface OfferMessage {
  type: 'offer';
  session_id: string;
  sdp: string;
}

export interface IceCandidateMessage {
  type: 'ice-candidate';
  session_id: string;
  candidate: string;
}

export interface LeaveMessage {
  type: 'leave';
  session_id: string;
}

export type OutgoingMessage =
  | JoinMessage
  | OfferMessage
  | IceCandidateMessage
  | LeaveMessage;

// Incoming messages (server → mobile)
export interface JoinedMessage {
  type: 'joined';
  session_id: string;
  room_id: string;
}

export interface AnswerMessage {
  type: 'answer';
  session_id: string;
  sdp: string;
}

export interface IncomingIceCandidateMessage {
  type: 'ice-candidate';
  session_id: string;
  candidate: string;
}

export interface PeerLeftMessage {
  type: 'peer-left';
  session_id?: string;
}

export interface RoomClosedMessage {
  type: 'room-closed';
  reason?: string;
}

export interface ErrorMessage {
  type: 'error';
  message?: string;
  reason?: string;
}

export type IncomingMessage =
  | JoinedMessage
  | AnswerMessage
  | IncomingIceCandidateMessage
  | PeerLeftMessage
  | RoomClosedMessage
  | ErrorMessage;

// Union for parsing — covers all known types
export interface SignalingMessage {
  type:
    | 'join'
    | 'joined'
    | 'offer'
    | 'answer'
    | 'ice-candidate'
    | 'leave'
    | 'peer-left'
    | 'room-closed'
    | 'error';
  room_id?: string;
  user_id?: string;
  session_id?: string;
  sdp?: string;
  candidate?: string;
  message?: string;
  lang?: string;
  reason?: string;
}
