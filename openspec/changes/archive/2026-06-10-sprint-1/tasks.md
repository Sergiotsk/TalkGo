# Tasks: sprint-1

**Change**: TalkGo Sprint 1 — Room + Session + WebRTC + WebSocket MVP
**Generated**: 2026-06-10
**Total tasks**: 41
**TDD Mode**: Strict (test-first for all domain and port logic)

---

## Phase 1 — Infrastructure / Setup

**[x] 1.1** Add Go module dependencies: `github.com/gorilla/websocket`, `github.com/pion/webrtc/v3`
Run `go get` and verify `go.mod` + `go.sum` are updated.

**[x] 1.2** Create directory skeleton for new packages:
`internal/adapters/signaling/`, `internal/adapters/webrtc/`, `internal/adapters/http/`,
`internal/app/roomsvc/`, `internal/ports/driven/mocks/`
(create placeholder `.gitkeep` or initial `doc.go` files so directories are tracked)

**[x] 1.3** Add `net/http` import audit — verify `cmd/server/main.go` compiles cleanly after directory creation (`go build ./...` must pass with no new errors)

---

## Phase 2 — Domain: Room

**[x] 2.1** Test: Write `room_test.go` cases for `Join` — happy path (first participant), second participant (capacity=2 reached), duplicate participant (`ErrAlreadyInRoom`), room at capacity (`ErrRoomFull`), concurrent join stress
→ depends on: existing `room.go`

**[x] 2.2** Impl: Enrich `internal/domain/room/room.go` — add `Participants map[string]struct{}`, `Capacity int`, `mu sync.Mutex`, capacity constant (2), sentinel errors `ErrRoomFull`, `ErrRoomClosed`, `ErrAlreadyInRoom`, `ErrNotInRoom`; implement `Join(userID string) error`
→ depends on: 2.1 (test must fail first)

**[x] 2.3** Test: Add `room_test.go` cases for `Leave` — happy path (participant present), unknown participant (`ErrNotInRoom`), partial leave (2→1 participants)
→ depends on: 2.2

**[x] 2.4** Impl: Add `Leave(userID string) error`, `IsFull() bool`, `Close()` to `Room`
→ depends on: 2.3 (test must fail first)

**[x] 2.5** Verify: Run `go test ./internal/domain/room/...` — all table-driven cases green; coverage = 100%

---

## Phase 3 — Domain: Session

**[x] 3.1** Test: Write `session_test.go` with table-driven cases for state transitions — `connecting → active` (valid), invalid transitions, idempotent Disconnect, IsActive
→ depends on: existing `session.go`

**[x] 3.2** Impl: Enrich `internal/domain/session/session.go` — add `State` type (StateConnecting, StateActive, StateDisconnected), `ErrInvalidTransition`; implement `Activate()`, `Disconnect()`, `IsActive()`
→ depends on: 3.1 (test must fail first)

**[x] 3.3** Verify: Run `go test ./internal/domain/session/...` — all cases green; coverage = 100%

---

## Phase 4 — Ports

**[x] 4.1** Impl: Extend `internal/ports/driving/room_manager.go` — added `JoinRoom` (returns sessionID), `LeaveRoom`, `ErrRoomNotFound`

**[x] 4.2** Impl: Rewrite `internal/ports/driving/signaling.go` — defined `SignalingMessage` struct (7 fields), `SignalingHandler` interface with typed messages; added `ErrUnknownMessageType`, `ErrSessionNotFound`

**[x] 4.3** Impl: Expand `internal/ports/driven/webrtc_peer.go` — added `PeerConnectionState` type (6 states), `HandleOffer`, `CreateAnswer`, `AddICECandidate`, `OnICECandidate`, `ConnectionState`

**[x] 4.4** Impl: Create `internal/ports/driven/room_repository.go` — new `RoomRepository` driven port with Save, FindByID, Delete, ListActive

---

## Phase 5 — Mocks

**[x] 5.1** Impl: Create `internal/ports/driven/mocks/mock_room_repository.go` — hand-written mock implementing `RoomRepository`; include `SaveCalled`, `FindByIDCalled` bool flags and configurable return values (`SaveErr`, `FindResult`, `FindErr`, `DeleteErr`)

**[x] 5.2** Impl: Create `internal/ports/driven/mocks/mock_webrtc_peer.go` — hand-written mock implementing `WebRTCPeer`; include call tracking fields for each method and configurable return values
→ depends on: 4.3

**[x] 5.3** Verify: Run `go build ./internal/ports/...` — interfaces and mocks compile cleanly with no type mismatches

---

## Phase 6 — App Layer: RoomService

**[x] 6.1** Test: Write `internal/app/roomsvc/service_test.go` — test `Join` happy path using `MockRoomRepository` + `MockWebRTCPeer`; assert `repo.Save` called, `peer.CreateSession` called, session state is `StateActive`
→ depends on: 5.1, 5.2

**[x] 6.2** Test: Add service test cases for `Join` failure paths — repo `FindByID` returns error, room full (`ErrRoomFull`), peer `CreateSession` returns error (rollback: `Leave` called)
→ depends on: 6.1

**[x] 6.3** Impl: Create `internal/app/roomsvc/service.go` — `RoomService` struct with `repo RoomRepository` and `peer WebRTCPeer` fields; implement `Join(ctx, roomID, userID string) (*session.Session, error)` following spec flow: find room → call `room.Join` → save → create peer session → transition session state
→ depends on: 6.1, 6.2 (tests must fail first)

**[x] 6.4** Test: Write service test cases for `Leave` — happy path, session not found, peer `Close` error (non-fatal, logged)
→ depends on: 6.3

**[x] 6.5** Impl: Add `Leave(ctx, roomID, userID string) error` to `RoomService` — call `room.Leave` → save → close peer session → transition session to `StateClosed`; release service lock BEFORE calling room methods
→ depends on: 6.4 (test must fail first)

**[x] 6.6** Impl: Create `internal/app/roomsvc/repository.go` — `InMemoryRoomRepository` struct with `sync.RWMutex` and `map[string]*room.Room`; implement all `RoomRepository` methods

**[x] 6.7** Test: Write `internal/app/roomsvc/repository_test.go` — table-driven tests for `Save`, `FindByID` (found/not-found), `Delete`, concurrent access (two goroutines saving simultaneously)
→ depends on: 6.6

**[x] 6.8** Verify: Run `go test ./internal/app/roomsvc/...` — all green; coverage ≥ 70%

---

## Phase 7 — Adapters

### 7a — WebSocket Adapter

**[x] 7.1** Impl: Create `internal/adapters/signaling/client.go` — `Client` struct with `conn *websocket.Conn`, `send chan []byte` (buffered, size 8), `hub *Hub`; implement `writePump()` goroutine (drains `send` channel, handles close on disconnect)
→ depends on: 1.1 (gorilla/websocket must be in go.mod)

**[x] 7.2** Impl: Create `internal/adapters/signaling/hub.go` — `Hub` struct managing `clients map[*Client]bool`, `register`/`unregister` channels, `broadcast chan []byte`; implement `Run()` goroutine; add `ServeWS(hub, w, r)` upgrade function
→ depends on: 7.1

**[x] 7.3** Test: Write `internal/adapters/signaling/hub_test.go` — test client register, unregister, broadcast delivery, graceful disconnect on closed send channel
→ depends on: 7.2 (write test after impl — integration-style; pure unit test is impractical here)

### 7b — WebRTC Adapter (Pion)

**[x] 7.4** Impl: Create `internal/adapters/webrtc/pion_peer.go` — `PionPeer` struct implementing `WebRTCPeer`; configure STUN-only (`stun:stun.l.google.com:19302`), audio receive-only `RTPCodecTypeAudio`; implement `HandleOffer`, `CreateAnswer`, `AddICECandidate`, `Close`; wrap all Pion errors with `fmt.Errorf`
→ depends on: 4.3, 1.1 (pion/webrtc in go.mod)

**[x] 7.5** Test: Write `internal/adapters/webrtc/pion_peer_test.go` — smoke test: construct `PionPeer`, call `HandleOffer` with a minimal SDP offer string, verify no panic and error is nil or well-typed
→ depends on: 7.4 (write test after impl — Pion requires real network setup)

### 7c — HTTP Adapter

**[x] 7.6** Test: Write `internal/adapters/http/server_test.go` — use `httptest.NewRecorder`; test `POST /rooms` returns 201 with `roomID`, `DELETE /rooms/{id}` returns 204, `GET /health` returns 200
→ depends on: 5.1 (mock service needed)

**[x] 7.7** Impl: Create `internal/adapters/http/server.go` — `Server` struct with `svc RoomManager`; register routes using `net/http` stdlib mux; handlers: `createRoom` (decode JSON body → call `svc.CreateRoom` → 201), `deleteRoom` (path param → `svc.DeleteRoom` → 204), `health` (200 OK), `serveWS` (upgrade → hub)
→ depends on: 7.6 (test must fail first), 7.2

**[x] 7.8** Verify: Run `go test ./internal/adapters/...` — all tests green

---

## Phase 8 — Wiring

**[x] 8.1** Impl: Update `cmd/server/main.go` — instantiate `InMemoryRoomRepository`, `PionPeer`, `RoomService`, `Hub`, `Server`; wire dependencies; start `hub.Run()` in goroutine; call `http.ListenAndServe(":8080", ...)`
→ depends on: all previous phases complete

**[x] 8.2** Verify: Run `go build ./...` — clean compilation, zero errors

**[x] 8.3** Verify: Run `go test ./...` — full suite green; check per-package coverage meets thresholds (domain ≥ 80%, app ≥ 70%, adapters ≥ 60%)

**[x] 8.4** Verify: Run `go vet ./...` and `golangci-lint run` — zero warnings; confirm no data races with `go test -race ./...`

---

## Dependency Summary

```
1.1, 1.2, 1.3
    ↓
2.1 → 2.2 → 2.3 → 2.4 → 2.5
3.1 → 3.2 → 3.3
    ↓
4.1, 4.2, 4.3 → 4.4
    ↓
5.1, 5.2 → 5.3
    ↓
6.1 → 6.2 → 6.3 → 6.4 → 6.5
6.6 → 6.7 → 6.8
    ↓
7.1 → 7.2 → 7.3
7.4 → 7.5
7.6 → 7.7 → 7.8
    ↓
8.1 → 8.2 → 8.3 → 8.4
```

---

## TDD Pairs Summary

| Test task | Impl task | Domain/Layer |
|-----------|-----------|--------------|
| 2.1 | 2.2 | Room.Join |
| 2.3 | 2.4 | Room.Leave |
| 3.1 | 3.2 | Session.Transition |
| 6.1, 6.2 | 6.3 | RoomService.Join |
| 6.4 | 6.5 | RoomService.Leave |
| 6.7 | 6.6* | InMemoryRoomRepository |
| 7.6 | 7.7 | HTTP handlers |

*6.6 impl before 6.7 test because repository is a concrete type, not an interface under test — tests drive coverage, not design.
