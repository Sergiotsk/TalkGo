# Sprint 3 Technical Design: UX & Edge Cases (Archival Summary)

**Change**: sprint-3
**Status**: design (archived)
**Date**: 2026-06-11

---

## File Change Summary

| Layer | Files Changed | What |
|-------|---------------|------|
| Domain | `internal/domain/room/room.go` | +ShortCode, +LastActivity, +TouchActivity(), +GenerateShortCode() |
| Ports | `internal/ports/driving/signaling.go` | +OnDisconnect method on SignalingHandler |
| Ports | `internal/ports/driving/room_manager.go` | +FindByShortCode, +UpdateLastActivity, CreateRoom returns CreateRoomResult |
| Ports | `internal/ports/driven/room_repository.go` | +FindByShortCode, +UpdateLastActivity, +ListExpired |
| Service | `internal/app/roomsvc/service.go` | +ServiceConfig, +graceTimers, +OnDisconnect, +startExpirationSweep, +notifyRoomPeers, NewService sig change |
| Repository | `internal/app/roomsvc/repository.go` | +FindByShortCode, +UpdateLastActivity, +ListExpired |
| Hub | `internal/adapters/signaling/hub.go` | unregister: +peer-left, +OnDisconnect call; Run: +context |
| HTTP | `internal/adapters/http/server.go` | +findByShortCodeHandler, +GET /rooms/code/{code}, error mappings 409/410 |
| Main | `cmd/server/main.go` | +ServiceConfig wiring, +sweep goroutine, Hub.Run(ctx) |
| Mobile | `mobile/` (full project) | 88 tasks: Zustand, hooks, components, screens, native modules |

---

## Key Service Methods (Backend)

### OnDisconnect(ctx, sessionID)
1. Lookup session by sessionID
2. Count remaining participants
3. If 0 remaining: immediate DeleteRoom
4. If 1 remaining: start grace timer (30s) with AfterFunc
5. Do NOT remove session from maps (enables reconnection)

### JoinRoom (modified)
- After successful `r.Join(userID)`, cancel any pending grace timer

### CreateRoom (modified)
- Collision-resistant short code generation (max 5 retries)
- Returns `CreateRoomResult{RoomID, ShortCode}`

### startExpirationSweep
- Goroutine with 60s ticker
- Calls `repo.ListExpired(before)` with cutoff = now - RoomTTL
- For each expired: notify peers, DeleteRoom

### FindByShortCode (new)
- Case-insensitive lookup (normalize to uppercase)
- Check `room.Active` — return ErrRoomClosed if inactive

---

## Key React Native Architecture

### State Management (Zustand)
- `connectionState`: idle, connecting, connected, reconnecting, failed
- `localSpeaking`, `peerSpeaking`: boolean (not float)
- Selector pattern prevents unnecessary re-renders

### Hooks
- `useWebRTC`: RTCPeerConnection lifecycle, local/remote streams
- `useSignaling`: WebSocket client, message dispatch
- `useReconnection`: Exponential backoff state machine (1s/2s/4s)
- `useAudioLevel`: VAD from getStats(), 100ms polling

### Native Modules
- **iOS**: `AudioSessionManager.swift` — AVAudioSession config
- **Android**: `CallForegroundService.kt` + `CallServiceModule.kt` — Foreground Service + wake lock

### Component Hierarchy
```
ConversationScreen
├── useWebRTC
├── useSignaling
├── useReconnection
├── useAudioLevel
├── useKeepAwake
├── useSessionTimer
├── VUMeter (local + remote)
├── ConnectionStatus
├── MuteButton
├── SessionTimer
├── EndCallButton (+ confirmation dialog)
└── PipelineErrorBanner
```

---

## Critical Design Decisions

| Decision | Rationale |
|----------|-----------|
| OnDisconnect on SignalingHandler | Clear separation: disconnect=involuntary, leave=voluntary |
| Grace timer in Service | Business logic belongs in app layer, not adapter |
| OnDisconnect OUTSIDE hub mutex | Prevents deadlock with notifyRoomPeers |
| Session NOT removed by OnDisconnect | Enables reconnection without creating new session |
| Sweep-based expiry | O(rooms) every 60s vs N timers for N rooms |
| VU meters as boolean | Reduces re-renders from 10Hz float to event-based |
| Zustand selectors | Component isolation from store tick updates |
| ICE restart on reconnect | Pion handles transparently, no special message type |

---

## Testing Approach

### Strict TDD (all Go backend)
- Domain: GenerateShortCode, LastActivity, TouchActivity
- Repository: FindByShortCode, UpdateLastActivity, ListExpired
- Service: OnDisconnect, grace period, expiration, short codes
- Hub: peer-left notification, OnDisconnect calls
- HTTP: new endpoints, error mappings

### React Native
- Jest unit tests: store, hooks
- React Native Testing Library: components
- WS mock for integration tests

---

## Performance Notes

- VU meters use boolean state + selectors to avoid 60fps re-renders
- Sweep-based expiry: single goroutine, no per-room goroutines
- Short code generation: crypto/rand is secure, collision rate negligible at scale

---

## Deployment Changes

- Breaking changes:
  - `ServiceConfig` now required parameter to NewService
  - `CreateRoom` return type changed to CreateRoomResult struct
  - `Hub.Run(ctx context.Context)` requires context parameter
  - All existing NewService calls in tests must be updated

- New environment variables (optional):
  - `GRACE_PERIOD` (default 30s)
  - `ROOM_TTL` (default 10m)
  - `SWEEP_INTERVAL` (default 60s)

---

## Future Escalation Paths

- VU meters: If VAD insufficient, implement native RMS-based modules
- Short codes: If scale grows, add SQL-backed FindByShortCode with index
- Grace period: Could become configurable per-user-preference (sprint 4+)
- TURN server: Deferred to Sprint 4 (currently STUN-only)
