# Sprint 1 Spec: WebRTC Signaling & Room Management

**Change**: sprint-1
**Status**: spec
**Date**: 2026-06-10
**Module**: github.com/Sergiotsk/TalkGo

---

## Overview

This spec defines the behavioral requirements for Sprint 1: the minimum viable signaling path that allows two clients to connect via WebSocket, join a room, and establish a WebRTC peer connection through Pion. All requirements MUST be satisfied before the change is considered complete.

RFC 2119 keywords apply throughout: MUST, SHALL, SHOULD, MAY.

---

## 1. Domain: Room

### 1.1 Room Creation

**Invariants**:
- A Room MUST have a non-empty ID, a valid ISO 639-1 source language code, and a valid ISO 639-1 target language code.
- A Room MUST be created in `Active = true` state with zero participants.
- A Room MUST have a maximum capacity. For Sprint 1 MVP the capacity MUST be exactly **2** participants.
- `NewRoom` MUST return `ErrInvalidLanguageCode` if either language code is not exactly 2 ASCII characters.

**Scenarios**:

```
Given valid id "room-1", sourceLang "es", targetLang "en"
When  NewRoom("room-1", "es", "en") is called
Then  a Room is returned with Active=true, len(Participants)==0, Capacity==2
```

```
Given sourceLang "spa" (3 chars)
When  NewRoom is called
Then  ErrInvalidLanguageCode is returned
```

```
Given targetLang "eng" (3 chars)
When  NewRoom is called
Then  ErrInvalidLanguageCode is returned
```

```
Given sourceLang "" (empty)
When  NewRoom is called
Then  ErrInvalidLanguageCode is returned
```

### 1.2 Joining a Room

**Invariants**:
- `Room.Join(userID string)` MUST add the userID to `Participants` and return nil on success.
- `Room.Join` MUST return `ErrRoomFull` when `len(Participants) >= Capacity` before the call.
- `Room.Join` MUST return `ErrAlreadyInRoom` when the userID is already present in `Participants`.
- `Room.Join` MUST return `ErrRoomClosed` when `Active == false`.
- `Room.Join` MUST be safe for concurrent calls (protected by a `sync.Mutex` or `sync.RWMutex`).

**Scenarios**:

```
Given an active empty room with Capacity=2
When  room.Join("user-1") is called
Then  Participants contains "user-1", no error returned
```

```
Given an active room with 1 participant "user-1"
When  room.Join("user-2") is called
Then  Participants contains both "user-1" and "user-2", no error returned
```

```
Given an active room with 2 participants (at capacity)
When  room.Join("user-3") is called
Then  ErrRoomFull is returned, Participants unchanged
```

```
Given an active room with participant "user-1"
When  room.Join("user-1") is called again
Then  ErrAlreadyInRoom is returned, Participants unchanged
```

```
Given a room with Active=false
When  room.Join("user-1") is called
Then  ErrRoomClosed is returned
```

### 1.3 Leaving a Room

**Invariants**:
- `Room.Leave(userID string)` MUST remove the userID from `Participants` and return nil on success.
- `Room.Leave` MUST return `ErrNotInRoom` if the userID is not in `Participants`.
- `Room.Leave` MUST be safe for concurrent calls.
- Leaving a room does NOT change `Active` state automatically.

**Scenarios**:

```
Given a room with participant "user-1"
When  room.Leave("user-1") is called
Then  Participants no longer contains "user-1", no error returned
```

```
Given an active empty room
When  room.Leave("user-1") is called
Then  ErrNotInRoom is returned
```

```
Given a room with participants "user-1" and "user-2"
When  room.Leave("user-1") is called
Then  only "user-2" remains, no error returned
```

### 1.4 Room State Transitions

**Invariants**:
- A Room starts as `Active = true`.
- `Room.Close()` MUST set `Active = false` and remove all participants.
- Once `Active = false`, `Room.Join` MUST return `ErrRoomClosed`.
- There is no re-activation: once closed, a room MUST remain closed.

**Sentinel errors** (MUST be defined in `internal/domain/room/`):
```go
var (
    ErrInvalidLanguageCode = errors.New("invalid language code: must be ISO 639-1 (2 characters)")
    ErrRoomFull            = errors.New("room is full")
    ErrRoomClosed          = errors.New("room is closed")
    ErrAlreadyInRoom       = errors.New("user is already in this room")
    ErrNotInRoom           = errors.New("user is not in this room")
)
```

**Participant tracking** field (MUST be added to `Room` struct):
```go
Participants map[string]struct{}
Capacity     int
mu           sync.Mutex  // unexported, concurrency guard
```

---

## 2. Domain: Session

### 2.1 Session State Machine

**States**:
- `StateConnecting` — session created, WebRTC handshake not yet complete
- `StateActive`     — WebRTC handshake complete, media flowing
- `StateDisconnected` — session ended (graceful or error)

**Invariants**:
- `NewSession(id, roomID, userID string)` MUST return a Session with `State = StateConnecting`.
- `Session.Activate()` MUST transition from `StateConnecting` → `StateActive`. MUST return `ErrInvalidTransition` if called on any other state.
- `Session.Disconnect()` MUST transition from `StateConnecting` or `StateActive` → `StateDisconnected`. MUST be idempotent: calling it on an already-disconnected session MUST NOT return an error.
- `Session.IsActive()` MUST return `true` only when `State == StateActive`.

**Scenarios**:

```
Given NewSession("s1", "room-1", "user-1") is called
Then  session.State == StateConnecting
And   session.IsActive() == false
```

```
Given a session with State=StateConnecting
When  session.Activate() is called
Then  session.State == StateActive
And   session.IsActive() == true
```

```
Given a session with State=StateActive
When  session.Activate() is called again
Then  ErrInvalidTransition is returned
```

```
Given a session with State=StateConnecting
When  session.Disconnect() is called
Then  session.State == StateDisconnected
And   session.IsActive() == false
```

```
Given a session with State=StateActive
When  session.Disconnect() is called
Then  session.State == StateDisconnected
```

```
Given a session with State=StateDisconnected
When  session.Disconnect() is called again
Then  no error is returned (idempotent)
```

**Sentinel errors** (MUST be defined in `internal/domain/session/`):
```go
var ErrInvalidTransition = errors.New("invalid session state transition")
```

**State type** (MUST be defined):
```go
type State int
const (
    StateConnecting   State = iota
    StateActive
    StateDisconnected
)
```

---

## 3. Port: RoomManager (driving)

### 3.1 Interface Contract

The `RoomManager` interface in `internal/ports/driving/room_manager.go` MUST expose:

```go
type RoomManager interface {
    CreateRoom(ctx context.Context, sourceLang, targetLang string) (string, error)
    DeleteRoom(ctx context.Context, roomID string) error
    JoinRoom(ctx context.Context, roomID, userID string) (string, error) // returns sessionID
    LeaveRoom(ctx context.Context, roomID, userID string) error
}
```

**`CreateRoom`**:
- MUST return a unique room ID (UUID v4) on success.
- MUST return an error if language codes are invalid (wrapping `ErrInvalidLanguageCode`).

**`DeleteRoom`**:
- MUST close the room and release all associated resources (sessions, peer connections).
- MUST return `ErrRoomNotFound` if the roomID does not exist.

**`JoinRoom`**:
- MUST create a new `Session` for the user in the room.
- MUST return the new session ID on success.
- MUST propagate `ErrRoomFull`, `ErrRoomClosed`, `ErrAlreadyInRoom` from domain.
- MUST return `ErrRoomNotFound` if the roomID does not exist.

**`LeaveRoom`**:
- MUST disconnect and remove the session for the user.
- MUST propagate `ErrNotInRoom` from domain.
- MUST return `ErrRoomNotFound` if the roomID does not exist.

**Sentinel errors** (MUST be defined in `internal/ports/driving/`):
```go
var ErrRoomNotFound = errors.New("room not found")
```

---

## 4. Port: Signaling (driving)

### 4.1 SignalingMessage Type

The `SignalingHandler` port MUST operate on a structured message type rather than raw `[]byte`:

```go
// SignalingMessage represents a typed WebRTC signaling message.
type SignalingMessage struct {
    Type      string `json:"type"`       // "join" | "offer" | "answer" | "ice-candidate" | "leave"
    RoomID    string `json:"room_id,omitempty"`
    UserID    string `json:"user_id,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    SDP       string `json:"sdp,omitempty"`
    Candidate string `json:"candidate,omitempty"`
}
```

### 4.2 Interface Contract

```go
type SignalingHandler interface {
    HandleSignaling(ctx context.Context, msg SignalingMessage) (SignalingMessage, error)
}
```

### 4.3 Message Types and Behavior

**Client → Server messages** (MUST be supported):

| Type            | Required fields             | Description |
|-----------------|-----------------------------|-------------|
| `join`          | `room_id`, `user_id`        | Join an existing room |
| `offer`         | `session_id`, `sdp`         | Send SDP offer to server |
| `ice-candidate` | `session_id`, `candidate`   | Send ICE candidate |
| `leave`         | `session_id`                | Leave room gracefully |

**Server → Client messages** (MUST be sent):

| Type            | Fields                        | Trigger |
|-----------------|-------------------------------|---------|
| `joined`        | `session_id`, `room_id`       | Successful join |
| `answer`        | `session_id`, `sdp`           | After processing offer |
| `ice-candidate` | `session_id`, `candidate`     | Server-side ICE candidate |
| `error`         | `message`                     | Any failure |
| `peer-left`     | `session_id`                  | Another participant left |

### 4.4 Error Scenarios

**Scenarios**:

```
Given a "join" message for a non-existent roomID
When  HandleSignaling is called
Then  an "error" message is returned with message="room not found"
```

```
Given a "join" message for a full room
When  HandleSignaling is called
Then  an "error" message is returned with message="room is full"
```

```
Given an "offer" message with an unknown sessionID
When  HandleSignaling is called
Then  an "error" message is returned with message="session not found"
```

```
Given a message with Type="" (unknown type)
When  HandleSignaling is called
Then  ErrUnknownMessageType is returned
```

**Sentinel errors** (MUST be defined in `internal/ports/driving/`):
```go
var (
    ErrUnknownMessageType = errors.New("unknown signaling message type")
    ErrSessionNotFound    = errors.New("session not found")
)
```

---

## 5. Port: WebRTCPeer (driven)

### 5.1 Interface Contract

The `WebRTCPeer` interface in `internal/ports/driven/webrtc_peer.go` MUST be expanded to:

```go
type WebRTCPeer interface {
    CreateSession(ctx context.Context, sessionID string) error
    CloseSession(ctx context.Context, sessionID string) error
    HandleOffer(ctx context.Context, sessionID, sdp string) error
    CreateAnswer(ctx context.Context, sessionID string) (string, error)
    AddICECandidate(ctx context.Context, sessionID, candidate string) error
    OnICECandidate(ctx context.Context, sessionID string, handler func(candidate string)) error
    ConnectionState(ctx context.Context, sessionID string) (PeerConnectionState, error)
}
```

### 5.2 PeerConnectionState

```go
type PeerConnectionState int
const (
    PeerStateNew         PeerConnectionState = iota
    PeerStateConnecting
    PeerStateConnected
    PeerStateDisconnected
    PeerStateFailed
    PeerStateClosed
)
```

### 5.3 Behavioral Contracts

**`CreateSession`**:
- MUST create a new Pion `PeerConnection` for the given sessionID.
- MUST configure STUN-only ICE servers (no TURN for MVP).
- MUST return an error if a session with the same ID already exists.

**`HandleOffer`**:
- MUST set the remote SDP description on the peer connection.
- MUST only be called after `CreateSession`. Returns error otherwise.

**`CreateAnswer`**:
- MUST generate a local SDP answer.
- MUST call `SetLocalDescription` internally.
- MUST return the SDP answer string.
- MUST only be called after `HandleOffer`. Returns error otherwise.

**`AddICECandidate`**:
- MUST parse and add the ICE candidate to the peer connection.
- SHOULD buffer candidates received before remote description is set and apply them once set.

**`OnICECandidate`**:
- MUST register a callback that fires when a local ICE candidate is gathered.
- The callback MUST be called once per gathered candidate (trickle ICE).
- After ICE gathering completes, the callback MUST NOT be called again.

**`CloseSession`**:
- MUST close the Pion peer connection and release all resources.
- MUST remove the session from internal state.
- MUST be idempotent: calling it on a non-existent session MUST NOT return an error.

**`ConnectionState`**:
- MUST return the current `PeerConnectionState` for the given sessionID.
- MUST return `ErrSessionNotFound` if the sessionID does not exist.

---

## 6. Adapter: WebSocket Signaling

**Package**: `internal/adapters/signaling/`

### 6.1 Connection Lifecycle

**Invariants**:
- The adapter MUST upgrade HTTP connections to WebSocket using `gorilla/websocket`.
- On successful upgrade, the adapter MUST start a read loop and a write pump in separate goroutines.
- On any read error (disconnect, timeout), the adapter MUST call `LeaveRoom` and `CloseSession` for the affected session, then close the WebSocket connection.
- The adapter MUST NOT block the HTTP handler goroutine after upgrading — all work happens in background goroutines.

### 6.2 Concurrent Write Safety (Write Pump Pattern)

**Invariants**:
- The adapter MUST use a dedicated write goroutine (write pump) per connection.
- Outbound messages MUST be enqueued to a channel (buffer size ≥ 8) and written by the write pump only.
- Direct `conn.WriteMessage()` calls outside the write pump goroutine are FORBIDDEN.
- The write pump MUST exit when its send channel is closed.
- The adapter MUST call `conn.SetWriteDeadline()` before every write (deadline: 10 seconds).
- The adapter MUST call `conn.SetReadDeadline()` and send WebSocket ping frames every 30 seconds to detect stale connections.

**Scenarios**:

```
Given a connected WebSocket client
When  two goroutines attempt to send messages simultaneously
Then  both messages are delivered correctly without panic or data corruption
```

```
Given a connected client that stops responding
When  30 seconds elapse without a pong
Then  the connection is closed and cleanup is triggered
```

### 6.3 Message Dispatch

**Invariants**:
- Inbound messages MUST be unmarshalled from JSON into `SignalingMessage`.
- Invalid JSON MUST result in an `error` message sent back to the client; the connection MUST remain open.
- Unknown `type` fields MUST result in an `error` message; the connection MUST remain open.
- The adapter MUST dispatch each valid message to `SignalingHandler.HandleSignaling`.
- The adapter MUST send the returned `SignalingMessage` response back to the client.

### 6.4 Graceful Disconnect

**Scenarios**:

```
Given a client that sends a "leave" message
When  HandleSignaling processes it successfully
Then  the WebSocket connection is closed with code 1000 (Normal Closure)
And   room and session state are cleaned up
```

```
Given a client whose TCP connection drops
When  the read loop detects an error
Then  cleanup is performed (LeaveRoom, CloseSession) without leaking goroutines
```

---

## 7. Adapter: WebRTC (Pion)

**Package**: `internal/adapters/webrtc/`

### 7.1 Peer Connection Creation

**Invariants**:
- The Pion adapter MUST implement the `WebRTCPeer` driven port.
- The adapter MUST maintain an internal `map[string]*webrtc.PeerConnection` keyed by sessionID.
- Access to this map MUST be protected by a `sync.RWMutex`.
- The adapter MUST be initialized with an ICE server list via config.

### 7.2 STUN-Only Configuration (MVP)

**Invariants**:
- The adapter MUST configure at least one STUN server at initialization: `stun:stun.l.google.com:19302`.
- No TURN servers SHALL be configured in Sprint 1.
- The ICE server list MUST be injected via a config struct (not hardcoded inline).

**Config struct** (MUST be defined):
```go
type Config struct {
    ICEServers []webrtc.ICEServer
}

func DefaultConfig() Config {
    return Config{
        ICEServers: []webrtc.ICEServer{
            {URLs: []string{"stun:stun.l.google.com:19302"}},
        },
    }
}
```

### 7.3 Audio Track Handling (Receive Only, Sprint 1)

**Invariants**:
- The Pion peer connection MUST be created with `RTPTransceiverDirectionRecvonly` for audio.
- No video tracks SHALL be negotiated in Sprint 1.
- The `OnTrack` callback MUST be registered to receive audio RTP packets (drain them — no forwarding in Sprint 1).
- Received audio frames MUST be silently discarded (read and drop); no unbuffered blocking MUST occur.

**Scenarios**:

```
Given a fully connected peer session
When  the remote client starts sending audio
Then  the OnTrack callback fires and audio RTP is drained without blocking
```

---

## 8. App Service: RoomService

**Package**: `internal/app/roomsvc/`

### 8.1 Responsibilities

The `RoomService` struct MUST:
- Implement the `RoomManager` driving port (CreateRoom, DeleteRoom, JoinRoom, LeaveRoom).
- Implement the `SignalingHandler` driving port (HandleSignaling).
- Hold an in-memory `map[string]*room.Room` for room storage.
- Hold an in-memory `map[string]*session.Session` for session storage.
- Inject a `WebRTCPeer` driven port for media operations.
- Both maps MUST be protected by `sync.RWMutex`.

### 8.2 Join Flow Orchestration

**Invariants** (enforced in order):
1. Look up the room by ID — return `ErrRoomNotFound` if missing.
2. Call `room.Join(userID)` — propagate domain errors.
3. Create a new `session.Session` with `uuid.NewString()` as ID.
4. Call `WebRTCPeer.CreateSession(ctx, sessionID)` — on failure, call `room.Leave(userID)` and return error.
5. Store session in sessions map.
6. Return sessionID.

**Scenario**:

```
Given a room "room-1" with 0 participants and a configured WebRTCPeer
When  JoinRoom(ctx, "room-1", "user-1") is called
Then  a sessionID is returned
And   room.Participants contains "user-1"
And   WebRTCPeer.CreateSession was called with that sessionID
```

```
Given a room "room-1" with 2 participants (at capacity)
When  JoinRoom(ctx, "room-1", "user-3") is called
Then  ErrRoomFull is returned
And   WebRTCPeer.CreateSession is NOT called
```

```
Given WebRTCPeer.CreateSession returns an error
When  JoinRoom is called
Then  the error is propagated
And   room.Leave is called to rollback the join
```

### 8.3 Leave Flow Orchestration

**Invariants** (enforced in order):
1. Look up session by userID+roomID mapping — return `ErrSessionNotFound` if missing.
2. Call `session.Disconnect()`.
3. Call `WebRTCPeer.CloseSession(ctx, sessionID)` — log error, do not abort.
4. Call `room.Leave(userID)` — propagate error only if it is NOT `ErrNotInRoom` (idempotent).
5. Remove session from sessions map.

### 8.4 SignalingHandler Dispatch

**Invariants**:
- `"join"` message → call `JoinRoom`, return `"joined"` message with sessionID.
- `"offer"` message → call `WebRTCPeer.HandleOffer` then `WebRTCPeer.CreateAnswer`, return `"answer"` message.
- `"ice-candidate"` message → call `WebRTCPeer.AddICECandidate`, return empty ACK (no response needed — return zero-value message and no error).
- `"leave"` message → call `LeaveRoom`, return empty ACK.
- Unknown type → return `ErrUnknownMessageType`.

**Scenario**:

```
Given a valid "offer" message for session "sess-1"
When  HandleSignaling is called
Then  WebRTCPeer.HandleOffer is called with the SDP
And   WebRTCPeer.CreateAnswer is called
And   the returned message has Type="answer" and a non-empty SDP
```

---

## 9. HTTP Server

**Package**: `internal/adapters/http/`

### 9.1 Routes

| Method | Path             | Handler             | Description |
|--------|------------------|---------------------|-------------|
| GET    | `/health`        | `healthHandler`     | Returns 200 OK with `{"status":"ok"}` |
| POST   | `/rooms`         | `createRoomHandler` | Creates a new room |
| DELETE | `/rooms/{id}`    | `deleteRoomHandler` | Closes and deletes a room |
| GET    | `/ws/{roomID}`   | `wsHandler`         | WebSocket upgrade for signaling |

### 9.2 Route Contracts

**`GET /health`**:
- MUST return HTTP 200 with `Content-Type: application/json` and body `{"status":"ok"}`.
- MUST NOT require authentication.

**`POST /rooms`**:
- Request body MUST be `{"source_lang":"es","target_lang":"en"}`.
- MUST return HTTP 201 with `{"room_id":"<uuid>"}` on success.
- MUST return HTTP 400 with `{"error":"..."}` for invalid language codes.
- MUST return HTTP 500 for unexpected errors.

**`DELETE /rooms/{id}`**:
- MUST return HTTP 204 on success.
- MUST return HTTP 404 with `{"error":"room not found"}` if the room does not exist.

**`GET /ws/{roomID}`**:
- MUST upgrade the connection to WebSocket.
- MUST return HTTP 400 if the roomID path parameter is missing or empty.
- MUST return HTTP 404 if the room does not exist.
- After upgrade, MUST hand off to the signaling adapter.

### 9.3 Graceful Shutdown

**Invariants**:
- The server MUST listen for OS signals `SIGINT` and `SIGTERM`.
- On signal receipt, the server MUST call `http.Server.Shutdown(ctx)` with a 15-second timeout.
- Active WebSocket connections MUST be allowed to complete their current message before shutdown.
- The server MUST log startup address and port via `log/slog` at `INFO` level.
- The server MUST log shutdown initiation and completion via `log/slog` at `INFO` level.

**Scenario**:

```
Given the server is running and handling requests
When  SIGTERM is received
Then  new connections are refused
And   in-flight requests complete within 15 seconds
And   the process exits with code 0
```

### 9.4 Server Configuration

The HTTP server MUST be configurable via a struct (not hardcoded):

```go
type Config struct {
    Addr            string        // e.g. ":8080"
    ReadTimeout     time.Duration // default 10s
    WriteTimeout    time.Duration // default 10s
    ShutdownTimeout time.Duration // default 15s
}
```

---

## 10. Cross-Cutting Concerns

### 10.1 Error Wrapping

- All errors crossing package boundaries MUST be wrapped with context: `fmt.Errorf("roomsvc.JoinRoom: %w", err)`.
- Sentinel errors MUST be unwrappable via `errors.Is()`.

### 10.2 Context Propagation

- All I/O operations MUST accept `context.Context` as first parameter.
- Context cancellation MUST be respected: operations in progress MUST return when context is cancelled.

### 10.3 Logging

- All adapters MUST use `log/slog` (stdlib). No other logging libraries.
- Connection lifecycle events (upgrade, disconnect, error) MUST be logged at `INFO` or `WARN` level.
- Internal errors MUST be logged at `ERROR` level with structured attributes (`slog.String`, `slog.Any`).

### 10.4 No Global State

- No `init()` functions.
- No package-level variables that hold mutable state.
- All dependencies injected via constructors.

### 10.5 Dependency Injection Entry Point

`cmd/server/main.go` MUST wire all components in this order:
1. Create `webrtc.Config` (STUN servers).
2. Create `webrtc.Adapter` (implements `WebRTCPeer` port).
3. Create `roomsvc.Service` injecting the adapter.
4. Create `signaling.Handler` injecting the service.
5. Create `http.Server` with routes pointing to handlers.
6. Start server with graceful shutdown.

---

## 11. Testing Requirements

### Coverage Targets

| Package | Minimum Coverage |
|---------|-----------------|
| `internal/domain/room` | 80% |
| `internal/domain/session` | 80% |
| `internal/app/roomsvc` | 70% |
| `internal/adapters/signaling` | 60% |
| `internal/adapters/webrtc` | 60% |
| `internal/adapters/http` | 60% |

### Testing Rules

- ALL domain tests MUST be table-driven (`t.Run()`).
- App service tests MUST use mock implementations of driven ports (hand-rolled, no mocking framework).
- Adapter tests for WebSocket MUST use `httptest.NewRecorder` and `httptest.NewServer`.
- Pion adapter tests MAY use a loopback peer connection pair to test SDP exchange.
- NO production code MAY be written before a failing test exists for it (Strict TDD).

### Mock Contracts

The following mock types MUST exist (in `_test.go` files or a `testutil` package):

```go
// MockWebRTCPeer for testing RoomService
type MockWebRTCPeer struct {
    CreateSessionFn    func(ctx context.Context, sessionID string) error
    CloseSessionFn     func(ctx context.Context, sessionID string) error
    HandleOfferFn      func(ctx context.Context, sessionID, sdp string) error
    CreateAnswerFn     func(ctx context.Context, sessionID string) (string, error)
    AddICECandidateFn  func(ctx context.Context, sessionID, candidate string) error
    OnICECandidateFn   func(ctx context.Context, sessionID string, handler func(string)) error
    ConnectionStateFn  func(ctx context.Context, sessionID string) (driven.PeerConnectionState, error)
}
```

---

## 12. Out of Scope (Sprint 1)

The following are explicitly NOT required in Sprint 1:

- Audio forwarding between peers (tracks are drained only).
- Translation pipeline integration.
- Authentication / authorization.
- Persistence (all state is in-memory).
- TURN server configuration.
- More than 2 participants per room.
- Client SDK or mobile app.
- Metrics or distributed tracing.
