# Spec: UX & Edge Cases for TalkGo

**Status**: Final (archived from Sprint 3)
**Last Updated**: 2026-06-11

---

## Overview

This spec defines the critical robustness fixes to the TalkGo backend and the first mobile client (React Native) for a usable end-to-end experience. Two peers can create a room via short code, join from mobile devices, speak in different languages, survive network hiccups, and maintain sessions in the background on iOS and Android.

---

## Backend Requirements (Workstream A)

### REQ-A01: Peer Disconnect Notification
- **Summary**: Hub notifies the remaining peer in a room when a client disconnects
- **Driver**: Explicit `OnDisconnect(ctx, sessionID)` method on `driving.SignalingHandler`
- **Message Format**: `{"type": "peer-left", "session_id": "..."}`
- **Coverage**: Both voluntary (leave message) and abrupt (WS drop) disconnections

### REQ-A02: Grace Period (30 seconds)
- **Summary**: When one peer disconnects, the room stays alive for 30 seconds to allow reconnection
- **Behavior**:
  - If departed peer rejoins within 30s: timer cancels, room continues
  - If timer expires: remaining peer is notified with `room-closed`, room is deleted
  - Only starts if exactly one participant remains (not if both disconnect)
- **Testability**: `ServiceConfig.GracePeriod` configurable (default 30s, tests use 1ms)

### REQ-A03: Room Expiry (10 minutes inactivity)
- **Summary**: Rooms automatically expire after 10 minutes without activity
- **Mechanism**: Background sweep goroutine runs every 60 seconds
- **Behavior**:
  - `room.LastActivity` updated on Join, Leave, and signaling events
  - Rooms where `time.Since(LastActivity) > 10 minutes` are closed and deleted
  - Client attempting to join expired room receives HTTP 410 Gone
- **Error message**: `"Esta sala expiró. Creá una nueva."`

### REQ-A04: Short Codes (6-char alphanumeric)
- **Summary**: Rooms identified by memorable 6-character codes instead of UUIDs alone
- **Generation**:
  - Alphabet: `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (excludes 0/O/1/I for verbal clarity)
  - Method: `crypto/rand` with collision detection (max 5 retries)
- **Endpoints**:
  - `POST /rooms` returns `{"room_id": "...", "short_code": "..."}`
  - `GET /rooms/code/{code}` returns room details or 404
- **Lookup**: Case-insensitive (normalized to uppercase)

### REQ-A05: Room Full Error (HTTP 409)
- **Summary**: Explicit error when third user attempts to join a 2-person room
- **HTTP**: 409 Conflict with body `{"error": "Esta sala ya tiene 2 participantes"}`
- **WebSocket**: Message format `{"type": "error", "message": "..."}`

---

## Mobile Requirements (Workstream B)

### REQ-B01: React Native Project Setup
- **Framework**: Bare workflow (CLI-init, NOT Expo managed)
- **Language**: TypeScript strict mode, zero compilation errors
- **Structure**: Organized into `screens/`, `hooks/`, `components/`, `store/`, `services/`, `types/`
- **Build**: Compiles on iOS simulator and Android emulator

### REQ-B02: WebSocket + WebRTC Integration
- **Connection**: Clients connect to `ws://<server>/ws/{roomID}` and establish SDP offer/answer exchange
- **Server Role**: SFU (Selective Forwarding Unit) — not peer-to-peer
- **ICE**: STUN-only (no TURN in Sprint 3)
- **Message flow**: join → joined → offer → answer → ice-candidates

### REQ-B03: Conversation Screen UI
- **Components**:
  - VU meters (local and remote audio activity)
  - Connection status indicator (Connecting / Connected / Reconnecting / Failed)
  - Mute toggle (audio track on/off with visual feedback)
  - Session timer (MM:SS format, updates per second)
  - "Finalizar" button with confirmation dialog
- **Keep-awake**: Screen remains on during active session

### REQ-B04: VU Meters (Real-time Audio Visualization)
- **Approach**: VAD (Voice Activity Detection) from WebRTC getStats()
- **State**: Boolean (speaking/not-speaking), not float, to avoid excessive re-renders
- **Update rate**: ≥10Hz
- **Optimization**: Zustand selectors prevent component re-renders on every audio tick

### REQ-B05: Automatic Reconnection with Exponential Backoff
- **State machine**: CONNECTED → RECONNECTING → CONNECTED | FAILED
- **Backoff delays**: 1s, 2s, 4s (max 3 attempts)
- **ICE restart**: Each reconnection attempt uses `iceRestart: true` in offer
- **User-initiated leave**: Does NOT trigger reconnection
- **Failure message**: "Conexión perdida" UI with manual retry button

### REQ-B06: iOS Background Mode
- **Config**: `Info.plist` declares `UIBackgroundModes: ["audio"]`
- **AVAudioSession**:
  - Category: `.playAndRecord`
  - Mode: `.voiceChat`
  - Options: `.allowBluetooth`, `.defaultToSpeaker`
- **Behavior**: Session continues when app goes to background; audio flows through

### REQ-B07: Android Background Mode (Foreground Service)
- **Service**: `CallForegroundService` with persistent notification
- **Permissions**: `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_MICROPHONE`, `RECORD_AUDIO`, `WAKE_LOCK`
- **Notification**: Title "TalkGo", text "Conversación activa", ongoing
- **Wake lock**: Partial wake lock prevents deep sleep during call

### REQ-B08: Pipeline Error Fallback
- **Trigger**: Translation pipeline returns error repeatedly
- **Visual**: Error banner visible; on 3+ consecutive errors, show fallback text
- **Recovery**: Banner disappears when pipeline succeeds again

### REQ-B09: Bluetooth Fallback
- **Detection**: Bluetooth disconnect mid-session
- **Fallback**: Audio automatically routed to built-in microphone
- **Notification**: Toast message to user
- **Platforms**: iOS (route change notification) and Android (SCO state change)

---

## Acceptance Criteria (12 total)

| ID | Criterion | Workstream |
|----|-----------|------------|
| CA-01 | Screen stays active (does not sleep) during session | B |
| CA-02 | VU meters respond to audio activity in real time | B |
| CA-03 | "Finalizar" button requires confirmation dialog | B |
| CA-04 | Both peers receive disconnect notification when either closes | A, B |
| CA-05 | Automatic reconnection with 1s/2s/4s backoff, max 3 attempts | B |
| CA-06 | 30-second grace period before room closure after peer disconnect | A |
| CA-07 | Pipeline errors show fallback visual indicator | B |
| CA-08 | Bluetooth disconnect → automatic fallback to built-in mic | B |
| CA-09 | iOS: active session continues in background | B |
| CA-10 | Android: Foreground Service keeps session alive in background | B |
| CA-11 | Third user joining full room → HTTP 409 "Esta sala ya tiene 2 participantes" | A |
| CA-12 | Expired room → HTTP 410 "Esta sala expiró" | A |

---

## Non-Functional Requirements

| ID | Requirement | Layer |
|----|-------------|-------|
| NFR-01 | Linting: `go vet ./...` and `golangci-lint run` pass with zero issues | Backend |
| NFR-02 | Coverage: ≥80% on modified files, measured with `go test -cover` | Backend |
| NFR-03 | Dependencies: No new external modules (stdlib only) | Backend |
| NFR-04 | TypeScript: strict mode, zero tsc errors | Mobile |
| NFR-05 | Build: Compiles without errors on iOS simulator and Android emulator | Mobile |
| NFR-06 | Physical device testing: Background mode verified on real devices | Mobile |
| NFR-07 | Hexagonal architecture: `internal/domain/` does NOT import `internal/adapters/` | Backend |
| NFR-08 | Strict TDD: Tests written before implementation for all backend features | Backend |

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `OnDisconnect` method on `SignalingHandler` | Explicit lifecycle method, not synthetic messages; clear separation between voluntary (leave) and involuntary (disconnect) flows |
| Grace period in Service, not Hub | Business logic (grace period) belongs in app layer, not adapter |
| `OnDisconnect` called outside Hub mutex | Prevents deadlock: OnDisconnect → notifyRoomPeers → Hub.mu.RLock would deadlock if already holding mu.Lock |
| Session not removed on disconnect | Enables reconnection: same userID can re-join and cancel the grace timer without creating new session |
| Sweep-based expiry vs per-room timers | Single goroutine (O(n) per 60s) vs N goroutines (one per room); more predictable resource usage |
| VU meters as boolean state | Reduces re-renders from 10Hz float updates to event-based (speaking start/stop) |
| Zustand for state management | Tiny footprint, selector-based re-render isolation, no provider boilerplate |
| ICE restart on reconnection | Pion SFU handles transparently; no new message type needed, reuses JoinRoom path |
| Short codes case-insensitive | Users may dictate codes verbally or type in lowercase; normalize to uppercase at lookup |
| Bare workflow React Native | Bare workflow required for `react-native-webrtc` native bindings; Expo managed cannot load native modules |

---

## Testing Strategy

### Backend (Strict TDD)
- **Domain**: ShortCode generation, LastActivity updates, TouchActivity
- **Repository**: FindByShortCode (hit/miss/case-insensitive), UpdateLastActivity, ListExpired
- **Service**: OnDisconnect grace period variants, expiration sweep, short code collision retry
- **Hub**: peer-left notification, OnDisconnect invocation, error cases
- **HTTP**: new endpoints, error code mappings (404/409/410)

### Mobile (Jest + React Native Testing Library)
- **Store**: connect/disconnect, tick (timer), incrementErrors/resetErrors
- **Hooks**: useReconnection (backoff delays, max attempts), useSignaling (message dispatch), useWebRTC (lifecycle)
- **Components**: VUMeter (speaking state), ConnectionStatus (state display), EndCallButton (confirmation dialog), PipelineErrorBanner (error visibility)
- **Integration**: Full WS flow with mocked server

---

## Architecture Notes

### Backend Flow (Disconnect + Grace Period)
1. Peer A WS drops → Hub.unregister(A)
2. Hub sends peer-left to Peer B, calls service.OnDisconnect(A)
3. Service starts grace timer (30s)
4. Peer A reconnects within 30s → JoinRoom cancels timer
5. If timer fires → service deletes room, notifies Peer B with room-closed

### Backend Flow (Room Expiry)
1. Service sweep goroutine ticks every 60s
2. Calls repo.ListExpired(now - 10min)
3. For each expired room: notifies peers, deletes room
4. Client joining expired room: service.FindByShortCode returns ErrRoomClosed → HTTP 410

### Mobile Flow (Conversation)
1. ConversationScreen mounts → useWebRTC creates RTCPeerConnection
2. useSignaling opens WS, sends join message
3. Server responds joined → useReconnection cancels any retry logic
4. useWebRTC creates offer with iceRestart (if reconnecting) → exchange answer + ICE candidates
5. Audio flows through SFU; useAudioLevel detects speaking via getStats VAD
6. Session active: keep-awake prevents screen sleep
7. User presses Finalizar → confirmation dialog → sendLeave message → cleanup

---

## Future Escalation Paths

- **VU meters**: If VAD insufficient visual fidelity, implement native RMS-based modules (iOS: AVAudioEngine, Android: Visualizer API)
- **TURN server**: Added in Sprint 4 for NAT traversal (currently STUN-only)
- **Database**: If scale grows, replace InMemoryRoomRepository with SQL backend
- **Multi-screen mobile**: Add home/onboarding/room-creation screens (Sprint 4+)
- **User authentication**: Add auth system (deferred, not in Sprint 3)
