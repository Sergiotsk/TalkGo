# Sprint 1 Proposal: WebRTC Signaling & Room Management

## 1. Intent

TalkGo currently has domain models (Room, Session, Translation) and port interfaces defined, but zero infrastructure to actually run. No HTTP server, no WebSocket signaling, no WebRTC media handling. Without Sprint 1, the app is a collection of types that cannot accept a single connection.

This change delivers the **minimum viable signaling path**: a client connects via WebSocket, joins a room, exchanges SDP offer/answer and ICE candidates with the server, and establishes a stable WebRTC peer connection managed by Pion. This is the foundational transport layer that every future feature (audio streaming, translation pipeline, mixing) depends on.

## 2. Scope

### New packages to CREATE

| Package | Path | Responsibility |
|---------|------|----------------|
| `signaling` adapter | `internal/adapters/signaling/` | WebSocket upgrade, JSON message framing, dispatch to domain |
| `webrtc` adapter | `internal/adapters/webrtc/` | Pion WebRTC peer connection lifecycle, SDP/ICE handling |
| `roomsvc` app service | `internal/app/roomsvc/` | Orchestrates Room + Session + WebRTCPeer, implements driving ports |
| `server` (HTTP) | `internal/adapters/http/` | HTTP server setup, routes, WebSocket upgrade endpoint |

### Existing files to MODIFY

| File | Change |
|------|--------|
| `internal/domain/room/room.go` | Add `Participants map[string]*session.Session`, `Join()`, `Leave()`, `IsFull()`, capacity field, `ErrRoomFull`, `ErrRoomClosed` sentinels |
| `internal/domain/session/session.go` | Add `Disconnect()` method, `State` enum (connecting/connected/disconnected) |
| `internal/ports/driving/room_manager.go` | Add `JoinRoom(ctx, roomID, userID)` and `LeaveRoom(ctx, roomID, userID)` to interface |
| `internal/ports/driving/signaling.go` | Refine `SignalingHandler` to accept structured messages (offer/answer/ice) instead of raw `[]byte` |
| `internal/ports/driven/webrtc_peer.go` | Expand interface: `HandleOffer()`, `CreateAnswer()`, `AddICECandidate()`, `OnICECandidate()`, `OnTrack()` |
| `cmd/server/main.go` | Wire everything: create services, start HTTP server with graceful shutdown |
| `go.mod` / `go.sum` | Add new dependencies |

### New test files

| File | Coverage target |
|------|-----------------|
| `internal/domain/room/room_test.go` | Extend: Join/Leave/IsFull/capacity (80%+) |
| `internal/domain/session/session_test.go` | NEW: state transitions, Disconnect (80%+) |
| `internal/app/roomsvc/roomsvc_test.go` | NEW: service orchestration with mock ports (70%+) |
| `internal/adapters/signaling/handler_test.go` | NEW: WebSocket message parsing, dispatch (60%+) |
| `internal/adapters/webrtc/peer_test.go` | NEW: Pion peer lifecycle (60%+) |
| `internal/adapters/http/server_test.go` | NEW: route registration, health endpoint (60%+) |

## 3. Approach

### Architecture (Hexagonal, inside-out)

```
                    HTTP Server (adapter)
                         |
                    WebSocket Handler (adapter/signaling)
                         |
                    RoomService (app/roomsvc)
                    /          \
            Room Domain      Session Domain
            (join/leave)     (state machine)
                    \          /
                    WebRTCPeer (adapter/webrtc via port)
                         |
                       Pion
```

**Step 1 - Domain enrichment (TDD)**
Extend `Room` with participant management (Join/Leave with capacity checks) and `Session` with a state machine (connecting -> connected -> disconnected). Pure logic, no I/O, easy to test exhaustively.

**Step 2 - Port refinement**
Update driving and driven port interfaces to support the signaling flow:
- Driving: `JoinRoom`, `LeaveRoom` on `RoomManager`; structured `SignalingMessage` type on `SignalingHandler`
- Driven: `HandleOffer`, `CreateAnswer`, `AddICECandidate`, `OnICECandidate` on `WebRTCPeer`

**Step 3 - Application service**
`roomsvc.Service` implements `RoomManager` and `SignalingHandler` (driving ports). It holds an in-memory `map[string]*room.Room` (no persistence needed yet) and delegates WebRTC operations to the `WebRTCPeer` driven port.

**Step 4 - Adapters (outside-in)**
- `adapters/webrtc/` implements `WebRTCPeer` using Pion. Each session gets a `peerconnection.PeerConnection`. Configures STUN servers, handles ICE trickle.
- `adapters/signaling/` implements WebSocket message handling. Upgrades HTTP connections, reads/writes JSON frames (`{type: "offer"|"answer"|"ice", payload: ...}`), routes to `SignalingHandler`.
- `adapters/http/` sets up `net/http` server with routes: `GET /health`, `GET /ws` (WebSocket upgrade), and graceful shutdown via `context.Context`.

**Step 5 - Wiring**
`cmd/server/main.go` constructs all dependencies, injects adapters into services, starts the server.

### Why this order?
- Domain first = tests drive the design, no mocks needed
- Ports second = contracts are stable before adapters implement them
- App service third = orchestration logic tested with mock ports
- Adapters last = infrastructure code depends on stable interfaces

### Signaling protocol (JSON over WebSocket)

```json
// Client -> Server
{"type": "join", "room_id": "abc", "user_id": "user-1"}
{"type": "offer", "session_id": "sess-1", "sdp": "..."}
{"type": "ice", "session_id": "sess-1", "candidate": "..."}

// Server -> Client
{"type": "joined", "session_id": "sess-1", "room_id": "abc"}
{"type": "answer", "session_id": "sess-1", "sdp": "..."}
{"type": "ice", "session_id": "sess-1", "candidate": "..."}
{"type": "error", "message": "room is full"}
```

### Room capacity
Default max 10 participants per room (configurable). Sufficient for the translation use case.

## 4. Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/pion/webrtc/v4` | latest v4 | WebRTC peer connections, SDP, ICE |
| `github.com/gorilla/websocket` | v1.5+ | WebSocket upgrade and framing |
| `github.com/google/uuid` | v1.6+ | Session/Room ID generation |

**No other dependencies.** Standard library `net/http` for the HTTP server (no framework needed for this scope). Logging via `log/slog` (stdlib, Go 1.21+).

## 5. Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Pion API complexity | Medium - Pion's API surface is large, easy to misconfigure ICE/DTLS | Start with simplest config (STUN only, no TURN), add complexity later |
| gorilla/websocket concurrency | Medium - concurrent read/write on same conn panics | Use write mutex or channel-based write pump pattern |
| Room state race conditions | High - concurrent Join/Leave on same room | Use `sync.RWMutex` on Room or channel-based access in the service layer |
| No persistence | Low (acceptable for Sprint 1) - server restart loses all rooms | Document as known limitation; persistence is a future sprint |
| Port interface churn | Medium - changing ports affects future adapters | Freeze port interfaces at end of Sprint 1; changes require proposal |

## 6. Rollback Plan

All Sprint 1 work lives in new packages (`internal/adapters/`, `internal/app/`) plus additive changes to existing domain/ports. Rollback strategy:

1. **Git revert**: All changes will be committed incrementally (one commit per logical unit). Revert the sprint branch or individual commits.
2. **No data migrations**: No database, no persistent state to roll back.
3. **Dependency removal**: `go mod tidy` after reverting code removes unused deps.
4. **Port interface rollback**: If port changes cause issues, revert to Sprint 0 interfaces. No downstream adapters depend on them yet.

The rollback is clean because Sprint 0 left the codebase in a stable, minimal state with no runtime behavior to break.
