# Sprint 1 Technical Design: WebRTC Signaling & Room Management

## 1. Package Structure

```
TalkGo/
├── cmd/server/
│   └── main.go                              # MODIFY — wire deps, HTTP server, graceful shutdown
├── internal/
│   ├── domain/
│   │   ├── room/
│   │   │   ├── room.go                      # MODIFY — add Participants, Join, Leave, IsFull, Close, capacity, sentinels
│   │   │   └── room_test.go                 # MODIFY — extend with Join/Leave/IsFull/capacity tests
│   │   └── session/
│   │       ├── session.go                   # MODIFY — add State enum, Disconnect(), state transitions
│   │       └── session_test.go              # CREATE — state machine tests
│   ├── ports/
│   │   ├── driving/
│   │   │   ├── room_manager.go              # MODIFY — add JoinRoom, LeaveRoom signatures
│   │   │   └── signaling.go                 # MODIFY — structured SignalingMessage, typed handler
│   │   └── driven/
│   │       ├── webrtc_peer.go               # MODIFY — expand: HandleOffer, CreateAnswer, AddICECandidate, OnICECandidate, OnTrack
│   │       ├── room_repository.go           # CREATE — in-memory room store interface
│   │       └── mocks/
│   │           ├── mock_webrtc_peer.go       # CREATE — hand-written mock for WebRTCPeer
│   │           ├── mock_room_repository.go   # CREATE — hand-written mock for RoomRepository
│   │           ├── mock_translator.go        # CREATE — hand-written mock for Translator
│   │           └── mock_audio_mixer.go       # CREATE — hand-written mock for AudioMixer
│   ├── app/
│   │   └── roomsvc/
│   │       ├── service.go                   # CREATE — RoomService implementing driving ports
│   │       └── service_test.go              # CREATE — orchestration tests with mock ports
│   └── adapters/
│       ├── signaling/
│       │   ├── client.go                    # CREATE — WebSocket client struct + write pump
│       │   ├── handler.go                   # CREATE — WebSocket upgrade, JSON dispatch
│       │   ├── message.go                   # CREATE — SignalingMessage types + marshal/unmarshal
│       │   └── handler_test.go              # CREATE — message parsing, dispatch tests
│       ├── webrtc/
│       │   ├── peer.go                      # CREATE — Pion PeerConnection adapter
│       │   └── peer_test.go                 # CREATE — Pion peer lifecycle tests
│       ├── http/
│       │   ├── server.go                    # CREATE — HTTP server, routes, graceful shutdown
│       │   └── server_test.go               # CREATE — route registration, health endpoint
│       └── storage/
│           └── memory_room.go               # CREATE — in-memory RoomRepository implementation
├── go.mod                                   # MODIFY — add pion/webrtc, gorilla/websocket, google/uuid
└── go.sum                                   # MODIFY — auto-generated
```

**Total**: 11 new files, 8 modified files.

---

## 2. Data Structures

### 2.1 Domain: `room.Room` (enriched)

```go
package room

import (
    "errors"
    "sync"
    "time"

    "github.com/Sergiotsk/TalkGo/internal/domain/session"
)

const DefaultMaxParticipants = 2

var (
    ErrInvalidLanguageCode  = errors.New("invalid language code: must be ISO 639-1 (2 characters)")
    ErrRoomFull             = errors.New("room is full")
    ErrRoomClosed           = errors.New("room is closed")
    ErrDuplicateParticipant = errors.New("participant already in room")
    ErrParticipantNotFound  = errors.New("participant not found in room")
)

type Room struct {
    ID              string
    SourceLang      string
    TargetLang      string
    CreatedAt       time.Time
    Active          bool
    MaxParticipants int
    Participants    map[string]*session.Session  // keyed by session ID

    mu sync.RWMutex  // protects Participants
}
```

**Key methods:**
- `NewRoom(id, sourceLang, targetLang string, opts ...Option) (*Room, error)` — functional options for capacity
- `Join(sess *session.Session) error` — adds participant, checks capacity + active + duplicate
- `Leave(sessionID string) (*session.Session, error)` — removes participant, returns removed session
- `IsFull() bool` — `len(Participants) >= MaxParticipants`
- `Close()` — sets `Active = false`
- `ParticipantCount() int` — thread-safe read
- `GetParticipant(sessionID string) (*session.Session, bool)` — thread-safe lookup

**Design decision**: The `sync.RWMutex` lives INSIDE the Room struct. All public methods acquire the lock internally. This keeps the concurrency model encapsulated within the domain entity rather than leaking to the service layer.

### 2.2 Domain: `session.Session` (with state machine)

```go
package session

import (
    "errors"
    "time"
)

type State int

const (
    StateConnecting   State = iota  // WebSocket connected, WebRTC not yet established
    StateConnected                   // WebRTC peer connection established
    StateDisconnected                // Cleanly disconnected or timed out
)

var (
    ErrInvalidTransition = errors.New("invalid state transition")
    ErrSessionClosed     = errors.New("session is already closed")
)

type Session struct {
    ID       string
    RoomID   string
    UserID   string
    JoinedAt time.Time
    Active   bool
    State    State
}
```

**Key methods:**
- `NewSession(id, roomID, userID string) *Session` — starts in `StateConnecting`
- `Connect() error` — transitions `Connecting → Connected`
- `Disconnect() error` — transitions `Connecting|Connected → Disconnected`, sets `Active = false`
- `IsActive() bool` — `Active && State != StateDisconnected`

**State transition table:**

| From | To | Method | Valid |
|------|----|--------|-------|
| Connecting | Connected | `Connect()` | YES |
| Connecting | Disconnected | `Disconnect()` | YES |
| Connected | Disconnected | `Disconnect()` | YES |
| Connected | Connecting | — | NO |
| Disconnected | * | — | NO (terminal) |

### 2.3 Adapter: `SignalingMessage` (JSON envelope)

```go
package signaling

import "encoding/json"

// MessageType enumerates all signaling message types.
type MessageType string

const (
    MsgTypeJoin   MessageType = "join"
    MsgTypeLeave  MessageType = "leave"
    MsgTypeOffer  MessageType = "offer"
    MsgTypeAnswer MessageType = "answer"
    MsgTypeICE    MessageType = "ice"

    // Server → Client
    MsgTypeJoined MessageType = "joined"
    MsgTypeError  MessageType = "error"
)

// SignalingMessage is the JSON envelope for all signaling messages.
type SignalingMessage struct {
    Type      MessageType     `json:"type"`
    RoomID    string          `json:"room_id,omitempty"`
    UserID    string          `json:"user_id,omitempty"`
    SessionID string          `json:"session_id,omitempty"`
    SDP       string          `json:"sdp,omitempty"`
    Candidate string          `json:"candidate,omitempty"`
    Message   string          `json:"message,omitempty"`
    Payload   json.RawMessage `json:"payload,omitempty"`
}
```

**Design note**: A single flat envelope with optional fields is simpler than polymorphic types for Sprint 1. The `Payload` field with `json.RawMessage` allows future extension without breaking the envelope.

### 2.4 Adapter: `WebSocketClient`

```go
package signaling

import (
    "context"
    "sync"

    "github.com/gorilla/websocket"
)

// Client represents a WebSocket connection with a write pump.
type Client struct {
    conn      *websocket.Conn
    sessionID string
    roomID    string
    send      chan []byte      // buffered channel for outgoing messages
    done      chan struct{}     // signals client shutdown
    closeOnce sync.Once
}
```

**Key methods:**
- `NewClient(conn *websocket.Conn) *Client` — initializes with buffered send channel (256 cap)
- `ReadPump(ctx context.Context, handler func(*SignalingMessage))` — blocking loop, reads JSON, calls handler
- `WritePump()` — blocking loop, drains send channel, writes to WebSocket
- `Send(msg *SignalingMessage) error` — JSON-marshals and enqueues to send channel (non-blocking with drop on full)
- `Close()` — idempotent close via `sync.Once`

### 2.5 Adapter: `PionPeer`

```go
package webrtc

import (
    "context"
    "sync"

    "github.com/pion/webrtc/v4"
)

// PionPeer wraps a Pion PeerConnection to implement the driven.WebRTCPeer port.
type PionPeer struct {
    sessionID    string
    pc           *webrtc.PeerConnection
    config       webrtc.Configuration
    onICE        func(candidate *webrtc.ICECandidate)  // callback for trickle ICE
    onTrack      func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)
    mu           sync.Mutex  // protects pc state
    closed       bool
}
```

**Key methods (implements `driven.WebRTCPeer`):**
- `NewPionPeer(sessionID string, config webrtc.Configuration) (*PionPeer, error)` — creates PeerConnection, registers ICE/track callbacks
- `HandleOffer(ctx context.Context, sdp string) (string, error)` — sets remote SDP, creates answer, returns answer SDP
- `CreateAnswer(ctx context.Context) (string, error)` — creates and sets local description
- `AddICECandidate(ctx context.Context, candidate string) error` — parses and adds ICE candidate
- `OnICECandidate(fn func(candidate string))` — registers callback for outgoing ICE candidates
- `OnTrack(fn func(trackID string, payload []byte))` — registers callback for incoming media
- `Close(ctx context.Context) error` — closes PeerConnection, idempotent

**STUN configuration for MVP:**
```go
var DefaultConfig = webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
    },
}
```

### 2.6 App Layer: `RoomService`

```go
package roomsvc

import (
    "context"
    "log/slog"
    "sync"

    "github.com/Sergiotsk/TalkGo/internal/domain/room"
    "github.com/Sergiotsk/TalkGo/internal/domain/session"
    "github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// Service orchestrates room lifecycle and WebRTC signaling.
// Implements driving.RoomManager and driving.SignalingHandler.
type Service struct {
    rooms    driven.RoomRepository
    peers    map[string]driven.WebRTCPeer  // keyed by session ID
    sessions map[string]*session.Session   // keyed by session ID
    logger   *slog.Logger
    mu       sync.RWMutex                  // protects peers + sessions maps
}
```

**Key methods (implements driving ports):**
- `NewService(repo driven.RoomRepository, logger *slog.Logger) *Service`
- `CreateRoom(ctx, sourceLang, targetLang) (string, error)` — creates domain Room, stores in repo
- `DeleteRoom(ctx, roomID) error` — closes room, disconnects all sessions, closes peers
- `JoinRoom(ctx, roomID, userID) (string, error)` — creates Session, adds to Room, returns sessionID
- `LeaveRoom(ctx, roomID, sessionID) error` — removes from Room, disconnects Session, closes peer
- `HandleSignaling(ctx, msg *SignalingMessage) (*SignalingMessage, error)` — routes by message type to internal handlers

### 2.7 Adapter: `Server` (HTTP)

```go
package http

import (
    "context"
    "log/slog"
    "net/http"

    "github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
    "github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
)

// Server is the HTTP server that handles WebSocket upgrades and health checks.
type Server struct {
    httpServer *http.Server
    handler    *signaling.Handler
    service    *roomsvc.Service
    logger     *slog.Logger
}
```

**Key methods:**
- `NewServer(addr string, svc *roomsvc.Service, logger *slog.Logger) *Server`
- `Start() error` — starts HTTP server (blocking)
- `Shutdown(ctx context.Context) error` — graceful shutdown with context deadline

**Routes:**
| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/health` | `healthHandler` | Returns `{"status":"ok"}` |
| GET | `/ws` | `wsHandler` | WebSocket upgrade, starts client read/write pumps |

---

## 3. Key Interfaces (Refined Ports)

### 3.1 Driving: `RoomManager`

```go
package driving

import "context"

// RoomManager defines the driving port to manage TalkGo rooms.
type RoomManager interface {
    // CreateRoom creates a new room with the specified language pair.
    CreateRoom(ctx context.Context, sourceLang, targetLang string) (string, error)

    // JoinRoom adds a participant to an existing room. Returns session ID.
    JoinRoom(ctx context.Context, roomID, userID string) (string, error)

    // LeaveRoom removes a participant from a room by session ID.
    LeaveRoom(ctx context.Context, roomID, sessionID string) error

    // DeleteRoom closes and destroys an existing room.
    DeleteRoom(ctx context.Context, roomID string) error
}
```

### 3.2 Driving: `SignalingHandler`

```go
package driving

import "context"

// MessageType enumerates the signaling message types the handler accepts.
type MessageType string

const (
    MsgOffer  MessageType = "offer"
    MsgAnswer MessageType = "answer"
    MsgICE    MessageType = "ice"
)

// SignalingMessage is the structured message the handler processes.
type SignalingMessage struct {
    Type      MessageType
    SessionID string
    SDP       string    // for offer/answer
    Candidate string    // for ice
}

// SignalingResponse is the handler's response.
type SignalingResponse struct {
    Type      MessageType
    SessionID string
    SDP       string
    Candidate string
}

// SignalingHandler defines the driving port for WebRTC signaling exchange.
type SignalingHandler interface {
    // HandleSignaling processes a structured signaling message and returns a response.
    HandleSignaling(ctx context.Context, msg SignalingMessage) (*SignalingResponse, error)
}
```

### 3.3 Driven: `WebRTCPeer`

```go
package driven

import "context"

// WebRTCPeer represents a driven port to manage a single WebRTC peer connection.
type WebRTCPeer interface {
    // HandleOffer sets the remote SDP offer and returns the local SDP answer.
    HandleOffer(ctx context.Context, sdp string) (answerSDP string, err error)

    // CreateAnswer creates a local SDP answer (used when offer was set externally).
    CreateAnswer(ctx context.Context) (answerSDP string, err error)

    // AddICECandidate adds a remote ICE candidate to the peer connection.
    AddICECandidate(ctx context.Context, candidate string) error

    // OnICECandidate registers a callback invoked when a local ICE candidate is gathered.
    OnICECandidate(fn func(candidate string))

    // OnTrack registers a callback invoked when a remote media track is received.
    OnTrack(fn func(trackID string, payload []byte))

    // Close terminates the peer connection and releases resources.
    Close(ctx context.Context) error
}
```

### 3.4 Driven: `RoomRepository` (new)

```go
package driven

import "context"

import "github.com/Sergiotsk/TalkGo/internal/domain/room"

// RoomRepository defines the driven port for room persistence.
type RoomRepository interface {
    // Save persists a room. Creates if new, updates if existing.
    Save(ctx context.Context, r *room.Room) error

    // FindByID retrieves a room by its ID. Returns ErrRoomNotFound if not found.
    FindByID(ctx context.Context, id string) (*room.Room, error)

    // Delete removes a room from the store.
    Delete(ctx context.Context, id string) error

    // List returns all active rooms.
    List(ctx context.Context) ([]*room.Room, error)
}
```

**Domain sentinel for repository:**
```go
// In room package
var ErrRoomNotFound = errors.New("room not found")
```

---

## 4. Sequence Diagrams

### 4.1 Join Room Flow

```
Client              HTTP Server         WS Handler          RoomService          Room Domain       Session Domain
  |                      |                   |                    |                    |                  |
  |-- GET /ws ---------->|                   |                    |                    |                  |
  |<--- 101 Upgrade -----|                   |                    |                    |                  |
  |                      |-- NewClient() --->|                    |                    |                  |
  |                      |   start ReadPump  |                    |                    |                  |
  |                      |   start WritePump |                    |                    |                  |
  |                      |                   |                    |                    |                  |
  |-- {"type":"join",  --|------------------>|                    |                    |                  |
  |    "room_id":"R1",   |                   |                    |                    |                  |
  |    "user_id":"U1"}   |                   |                    |                    |                  |
  |                      |                   |-- JoinRoom(R1,U1)->|                    |                  |
  |                      |                   |                    |-- FindByID(R1) --->|                  |
  |                      |                   |                    |<--- room ----------|                  |
  |                      |                   |                    |                    |                  |
  |                      |                   |                    |-- NewSession() ---------------------->|
  |                      |                   |                    |<--- session -------(StateConnecting)--|
  |                      |                   |                    |                    |                  |
  |                      |                   |                    |-- room.Join(sess)->|                  |
  |                      |                   |                    |   (checks capacity)|                  |
  |                      |                   |                    |<--- ok ------------|                  |
  |                      |                   |                    |                    |                  |
  |                      |                   |<-- sessionID ------|                    |                  |
  |                      |                   |                    |                    |                  |
  |<-- {"type":"joined", |<------------------|                    |                    |                  |
  |     "session_id":"S1",                   |                    |                    |                  |
  |     "room_id":"R1"}  |                   |                    |                    |                  |
```

### 4.2 WebRTC Offer/Answer Flow

```
Client              WS Handler          RoomService          WebRTCPeer (Pion)
  |                      |                    |                    |
  |-- {"type":"offer",   |                    |                    |
  |    "session_id":"S1",|                    |                    |
  |    "sdp":"v=0..."} ->|                    |                    |
  |                      |-- HandleSignaling->|                    |
  |                      |   (msg.Type=offer) |                    |
  |                      |                    |-- HandleOffer() -->|
  |                      |                    |   (sets remote SDP)|
  |                      |                    |   (creates answer) |
  |                      |                    |<-- answerSDP ------|
  |                      |                    |                    |
  |                      |                    |-- session.Connect()|
  |                      |                    |   (Connecting →    |
  |                      |                    |    Connected)      |
  |                      |                    |                    |
  |                      |<-- response -------|                    |
  |                      |   (type=answer,    |                    |
  |                      |    sdp=answerSDP)  |                    |
  |<-- {"type":"answer", |                    |                    |
  |     "session_id":"S1"|                    |                    |
  |     "sdp":"v=0..."}  |                    |                    |
  |                      |                    |                    |
  |-- {"type":"ice",     |                    |                    |
  |    "session_id":"S1",|                    |                    |
  |    "candidate":"..."}--->                 |                    |
  |                      |-- HandleSignaling->|                    |
  |                      |   (msg.Type=ice)   |                    |
  |                      |                    |-- AddICECandidate->|
  |                      |                    |<-- ok ------------|
  |                      |<-- nil response ---|                    |
  |                      |                    |                    |
  | (Meanwhile, Pion gathers local candidates)                    |
  |                      |                    |<-- OnICECandidate--|
  |                      |                    |   (callback fires) |
  |                      |<-- pushICE --------|                    |
  |<-- {"type":"ice",    |                    |                    |
  |     "candidate":"..."}                    |                    |
```

### 4.3 Leave / Disconnect Flow

```
Client              WS Handler          RoomService          Room Domain       Session       WebRTCPeer
  |                      |                    |                    |               |              |
  | (A) Explicit leave:  |                    |                    |               |              |
  |-- {"type":"leave",   |                    |                    |               |              |
  |    "room_id":"R1",   |                    |                    |               |              |
  |    "session_id":"S1"}>                    |                    |               |              |
  |                      |-- LeaveRoom() ---->|                    |               |              |
  |                      |                    |                    |               |              |
  | (B) WebSocket close: |                    |                    |               |              |
  |-- [conn closes] ---->|                    |                    |               |              |
  |                      |-- ReadPump exits ->|                    |               |              |
  |                      |   onDisconnect()   |                    |               |              |
  |                      |-- LeaveRoom() ---->|                    |               |              |
  |                      |                    |                    |               |              |
  | Both paths converge: |                    |                    |               |              |
  |                      |                    |-- room.Leave(S1)->|               |              |
  |                      |                    |   removes from map |               |              |
  |                      |                    |<-- session --------|               |              |
  |                      |                    |                    |               |              |
  |                      |                    |-- sess.Disconnect()--------------->|              |
  |                      |                    |   (→ Disconnected) |               |              |
  |                      |                    |                    |               |              |
  |                      |                    |-- peer.Close() ---------------------------------------->|
  |                      |                    |   (closes Pion PC) |               |              |
  |                      |                    |                    |               |              |
  |                      |                    |-- cleanup maps ----|               |              |
  |                      |<-- ok ------------|                    |               |              |
```

---

## 5. Concurrency Design

### 5.1 Room State Protection: `sync.RWMutex`

The `Room` struct holds an embedded `sync.RWMutex` to protect the `Participants` map.

**Lock acquisition rules:**
- `Join()`, `Leave()`, `Close()` → acquire **write lock** (`mu.Lock()`)
- `IsFull()`, `ParticipantCount()`, `GetParticipant()` → acquire **read lock** (`mu.RLock()`)
- Lock scope is narrow — acquired at method entry, released via `defer mu.Unlock()`

**Why mutex-in-domain**: The Room is the consistency boundary. Pushing synchronization up to the service layer would require every caller to remember locking, which is error-prone. The mutex does NOT cross domain boundaries — it only protects the in-memory participant map.

### 5.2 RoomService Maps Protection

The `Service` struct uses its own `sync.RWMutex` to protect the `peers` and `sessions` maps:
- `JoinRoom` → write lock (adds to both maps)
- `LeaveRoom` → write lock (removes from both maps)
- `HandleSignaling` → read lock (looks up peer by session ID)

**Important**: Service lock and Room lock are INDEPENDENT. Never hold both simultaneously to avoid deadlocks. The pattern is:
1. Acquire service lock → get/set service maps
2. Release service lock
3. Call room methods (which acquire their own lock internally)

### 5.3 WebSocket Write Pump: Goroutine per Client

Each `Client` starts two goroutines:
- **ReadPump**: blocking `conn.ReadMessage()` loop. Parses JSON. Calls handler. Exits on error/close.
- **WritePump**: blocking select loop on `send` channel and `done` channel. Writes messages. Exits when `done` is closed.

**Write concurrency safety**: All writes to the WebSocket go through the `send` channel. Only the WritePump goroutine calls `conn.WriteMessage()`. This eliminates the gorilla/websocket concurrent write panic.

```
┌──────────────┐     send chan     ┌──────────────┐
│   ReadPump   │ ──────────────>  │  WritePump   │ ──> conn.WriteMessage()
│ (goroutine)  │                  │ (goroutine)  │
│              │                  │              │
│ conn.Read()  │                  │ <-send       │
│ → parse JSON │                  │ <-done       │
│ → handler()  │                  └──────────────┘
└──────────────┘
```

**Channel buffer**: `send` is buffered at 256. If the client can't keep up (buffer full), `Send()` drops the message and logs a warning. This prevents a slow client from blocking the server.

### 5.4 Context Cancellation & Shutdown

```
main()
  │
  ├── ctx, cancel := signal.NotifyContext(ctx, SIGINT, SIGTERM)
  │
  ├── server.Start()  ← blocks in separate goroutine
  │
  ├── <-ctx.Done()    ← waits for signal
  │
  ├── shutdownCtx, _ := context.WithTimeout(ctx, 10s)
  │
  └── server.Shutdown(shutdownCtx)
       ├── httpServer.Shutdown()     ← stops accepting new conns
       ├── close all WebSocket clients  ← triggers ReadPump exit
       └── close all PionPeers      ← releases WebRTC resources
```

**Cancellation propagation:**
1. OS signal → context cancelled
2. `server.Shutdown()` called with 10s deadline
3. HTTP server stops accepting → existing handlers finish
4. All clients closed → ReadPump exits → WritePump exits
5. All PionPeers closed → ICE agents stop
6. `Shutdown()` returns

---

## 6. Error Strategy

### 6.1 Domain Sentinel Errors

```go
// room package
var (
    ErrInvalidLanguageCode  = errors.New("invalid language code: must be ISO 639-1 (2 characters)")
    ErrRoomFull             = errors.New("room is full")
    ErrRoomClosed           = errors.New("room is closed")
    ErrRoomNotFound         = errors.New("room not found")
    ErrDuplicateParticipant = errors.New("participant already in room")
    ErrParticipantNotFound  = errors.New("participant not found in room")
)

// session package
var (
    ErrInvalidTransition = errors.New("invalid state transition")
    ErrSessionClosed     = errors.New("session is already closed")
    ErrSessionNotFound   = errors.New("session not found")
)
```

### 6.2 Adapter Error Wrapping

All adapter errors wrap with context using `fmt.Errorf`:

```go
// webrtc adapter
func (p *PionPeer) HandleOffer(ctx context.Context, sdp string) (string, error) {
    offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}
    if err := p.pc.SetRemoteDescription(offer); err != nil {
        return "", fmt.Errorf("webrtc: set remote description: %w", err)
    }
    // ...
}

// storage adapter
func (m *MemoryRoomStore) FindByID(ctx context.Context, id string) (*room.Room, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    r, ok := m.rooms[id]
    if !ok {
        return nil, fmt.Errorf("memory store: %w", room.ErrRoomNotFound)
    }
    return r, nil
}
```

### 6.3 HTTP/WebSocket Error Responses

JSON error body format for WebSocket messages:

```json
{
    "type": "error",
    "message": "room is full",
    "code": "ROOM_FULL"
}
```

Error code mapping:

| Domain Error | Error Code | HTTP Equivalent |
|-------------|------------|-----------------|
| `ErrRoomFull` | `ROOM_FULL` | 409 Conflict |
| `ErrRoomNotFound` | `ROOM_NOT_FOUND` | 404 Not Found |
| `ErrRoomClosed` | `ROOM_CLOSED` | 410 Gone |
| `ErrDuplicateParticipant` | `DUPLICATE_PARTICIPANT` | 409 Conflict |
| `ErrSessionNotFound` | `SESSION_NOT_FOUND` | 404 Not Found |
| `ErrInvalidTransition` | `INVALID_STATE` | 400 Bad Request |
| (unknown) | `INTERNAL_ERROR` | 500 Internal |

For the REST endpoint (`/health`), standard HTTP status codes with JSON body:

```go
type ErrorResponse struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

---

## 7. Architecture Decision Records

### ADR-0004: In-Memory Room Storage for Sprint 1

**Status**: Proposed
**Context**: Sprint 1 needs room persistence. Options: (a) in-memory map, (b) SQLite, (c) Redis.
**Decision**: In-memory `map[string]*room.Room` behind a `RoomRepository` interface. No external dependencies.
**Rationale**: Sprint 1 is about proving the signaling path. Persistence is orthogonal. The `RoomRepository` port allows swapping to a real store in a future sprint without touching application code. Server restart loses all rooms — this is acceptable for MVP.
**Consequences**: No durability. Rooms vanish on restart. Horizontal scaling not possible (no shared state). Both are acceptable for Sprint 1.

### ADR-0005: STUN-Only for MVP (No TURN)

**Status**: Proposed
**Context**: WebRTC requires ICE servers. Options: (a) STUN only (Google public), (b) STUN + self-hosted TURN, (c) STUN + managed TURN (Twilio/Xirsys).
**Decision**: Google public STUN server only (`stun:stun.l.google.com:19302`).
**Rationale**: TURN is needed for clients behind symmetric NATs or restrictive firewalls. For MVP testing (LAN or open NAT), STUN suffices. TURN adds cost (bandwidth relay) and infrastructure complexity. The `WebRTCPeer` config is injected, so adding TURN later is a config change, not a code change.
**Consequences**: Connections will fail for ~8-15% of users behind symmetric NAT. Acceptable for MVP; TURN is a Sprint 2+ concern.

### ADR-0006: gorilla/websocket over nhooyr/websocket

**Status**: Proposed
**Context**: Need a WebSocket library. Options: (a) gorilla/websocket, (b) nhooyr.io/websocket, (c) stdlib (Go 1.22+ has experimental support).
**Decision**: `gorilla/websocket`.
**Rationale**:
- gorilla is the de facto standard in Go WebSocket. Battle-tested, widely documented.
- nhooyr has a nicer API and context support, but gorilla is more commonly understood and has extensive Pion ecosystem examples.
- Go stdlib WebSocket support is still experimental and lacks features (no ping/pong control, no compression).
- gorilla's maintenance status is "archived" but stable — no breaking changes expected, and the API surface is small enough that forking is trivial if needed.
**Consequences**: Must handle concurrent writes manually (write pump pattern). nhooyr would handle this automatically but adds a less familiar dependency.

---

## 8. Mock Strategy

All mocks are hand-written (no code generation dependency). Each mock implements the corresponding interface and records calls for assertion.

### Mock File Locations

| Interface | Mock File | Package |
|-----------|-----------|---------|
| `driven.WebRTCPeer` | `internal/ports/driven/mocks/mock_webrtc_peer.go` | `mocks` |
| `driven.RoomRepository` | `internal/ports/driven/mocks/mock_room_repository.go` | `mocks` |
| `driven.Translator` | `internal/ports/driven/mocks/mock_translator.go` | `mocks` |
| `driven.AudioMixer` | `internal/ports/driven/mocks/mock_audio_mixer.go` | `mocks` |

### Mock Pattern

Every mock follows this pattern:

```go
package mocks

import (
    "context"
    "github.com/Sergiotsk/TalkGo/internal/domain/room"
    "github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// Compile-time interface check
var _ driven.RoomRepository = (*MockRoomRepository)(nil)

type MockRoomRepository struct {
    SaveFn     func(ctx context.Context, r *room.Room) error
    FindByIDFn func(ctx context.Context, id string) (*room.Room, error)
    DeleteFn   func(ctx context.Context, id string) error
    ListFn     func(ctx context.Context) ([]*room.Room, error)

    SaveCalls     []SaveCall
    FindByIDCalls []FindByIDCall
    // ...
}

type SaveCall struct {
    Ctx  context.Context
    Room *room.Room
}

func (m *MockRoomRepository) Save(ctx context.Context, r *room.Room) error {
    m.SaveCalls = append(m.SaveCalls, SaveCall{Ctx: ctx, Room: r})
    if m.SaveFn != nil {
        return m.SaveFn(ctx, r)
    }
    return nil
}
// ... other methods follow the same pattern
```

**Why hand-written**:
- No `go generate` or `mockgen` dependency
- Mocks are simple — each interface has 3-6 methods
- Function fields (`SaveFn`, etc.) allow per-test behavior customization
- Call recording (`SaveCalls`) enables assertion on call count and arguments
- Compile-time `var _ Interface = (*Mock)(nil)` catches interface drift immediately

### Mock Usage in Tests

```go
func TestService_JoinRoom(t *testing.T) {
    repo := &mocks.MockRoomRepository{
        FindByIDFn: func(ctx context.Context, id string) (*room.Room, error) {
            r, _ := room.NewRoom(id, "es", "en")
            return r, nil
        },
        SaveFn: func(ctx context.Context, r *room.Room) error {
            return nil
        },
    }
    svc := roomsvc.NewService(repo, slog.Default())

    sessionID, err := svc.JoinRoom(context.Background(), "room-1", "user-1")
    // assertions...
}
```

---

## 9. Dependency Injection Wiring (`cmd/server/main.go`)

```go
func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    // Driven adapters
    roomRepo := storage.NewMemoryRoomStore()

    // App service
    svc := roomsvc.NewService(roomRepo, logger)

    // HTTP server (which internally creates signaling handler)
    srv := httpAdapter.NewServer(":8080", svc, logger)

    // Graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    go func() {
        logger.Info("starting server", "addr", ":8080")
        if err := srv.Start(); err != nil && err != http.ErrServerClosed {
            logger.Error("server error", "err", err)
            os.Exit(1)
        }
    }()

    <-ctx.Done()

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        logger.Error("shutdown error", "err", err)
    }
    logger.Info("server stopped")
}
```

---

## 10. Capacity Design Decision

**Default room capacity: 2 participants** (configurable via `room.WithMaxParticipants(n)` option).

The translation use case is fundamentally a 2-person conversation. Each room represents a translation session between exactly 2 participants speaking different languages. The default of 2 enforces this constraint at the domain level.

The `WithMaxParticipants` functional option allows future scenarios (e.g., conference mode, observer mode) without modifying the domain model:

```go
type Option func(*Room)

func WithMaxParticipants(n int) Option {
    return func(r *Room) {
        if n > 0 {
            r.MaxParticipants = n
        }
    }
}
```
