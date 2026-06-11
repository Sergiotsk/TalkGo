# Sprint 3 Task Breakdown (Archival Summary)

**Change**: sprint-3
**Status**: archived
**Date**: 2026-06-11
**Total Tasks**: 88 (all completed)

---

## Summary by Phase

| Phase | Tasks | Count | Area |
|-------|-------|-------|------|
| 1 | TASK-001..005 | 5 | Domain Go (ShortCode, LastActivity) |
| 2 | TASK-006..009 | 4 | Ports Go (interfaces) |
| 3 | TASK-010..015 | 6 | Repository (3 new methods) |
| 4 | TASK-016..030 | 15 | Service (core logic) |
| 5 | TASK-031..035 | 5 | Hub (peer-left, OnDisconnect) |
| 6 | TASK-036..042 | 7 | HTTP + main.go |
| 7 | TASK-043..049 | 7 | React Native setup |
| 8 | TASK-050..053 | 4 | Zustand store + types |
| 9 | TASK-054..063 | 10 | Hooks (WebRTC, signaling, reconnect, audio, timer) |
| 10 | TASK-064..077 | 14 | Components + ConversationScreen |
| 11 | TASK-078..083 | 6 | Background mode (iOS + Android) |
| 12 | TASK-084..088 | 5 | Integration + wiring |

**Total: 88 tasks**

---

## Backend (Go) — 42 tasks

### Phase 1-2: Domain + Ports (9 tasks)
- Domain: ShortCode generation, LastActivity, errors
- Driving ports: OnDisconnect, FindByShortCode, UpdateLastActivity, CreateRoomResult
- Driven port: FindByShortCode, UpdateLastActivity, ListExpired

### Phase 3-5: Repository + Service + Hub (26 tasks)
- Repository: 3 new methods with tests
- Service: ServiceConfig, OnDisconnect, grace period, expiration sweep, short codes
- Hub: peer-left notification, OnDisconnect call, context-based Run

### Phase 6: HTTP + Main (7 tasks)
- HTTP: findByShortCodeHandler, error mappings
- Main: ServiceConfig wiring, goroutine launch

---

## Mobile (React Native) — 46 tasks

### Phase 7-8: Setup + Store (11 tasks)
- Project initialization (bare workflow)
- Zustand store with boolean speaking state
- TypeScript types and interfaces

### Phase 9: Hooks (10 tasks)
- useSignaling (WS client)
- useWebRTC (RTCPeerConnection)
- useReconnection (exponential backoff)
- useAudioLevel (VAD detection)
- useKeepAwake, useSessionTimer

### Phase 10: Components (14 tasks)
- VUMeter, ConnectionStatus, MuteButton, SessionTimer
- EndCallButton with confirmation
- PipelineErrorBanner
- ConversationScreen composition

### Phase 11-12: Background + Integration (11 tasks)
- iOS: Info.plist, AudioSessionManager.swift
- Android: CallForegroundService.kt, CallServiceModule.kt
- Integration: api.ts, wiring, App.tsx entry

---

## Acceptance Criteria Coverage

| CA | Covered by Tasks |
|----|------------------|
| CA-01 (keep-awake) | TASK-062, TASK-077 |
| CA-02 (VU meters) | TASK-060, TASK-061, TASK-064, TASK-065 |
| CA-03 (confirm) | TASK-072, TASK-073 |
| CA-04 (peer-left) | TASK-031, TASK-033, TASK-076, TASK-077 |
| CA-05 (reconnect) | TASK-058, TASK-059 |
| CA-06 (grace period) | TASK-019, TASK-020, TASK-021, TASK-022 |
| CA-07 (pipeline error) | TASK-074, TASK-075 |
| CA-08 (Bluetooth) | TASK-079, TASK-083 |
| CA-09 (iOS background) | TASK-078, TASK-079, TASK-083 |
| CA-10 (Android FG) | TASK-080, TASK-081, TASK-082, TASK-083 |
| CA-11 (room full 409) | TASK-040, TASK-041 |
| CA-12 (room expired 410) | TASK-023, TASK-024, TASK-038, TASK-039 |

---

## Test Coverage

### Go Backend Tests
- Domain: 6 tests (GenerateShortCode, LastActivity, TouchActivity)
- Repository: 7 tests (FindByShortCode, UpdateLastActivity, ListExpired)
- Service: 12+ tests (OnDisconnect variants, expiration, short codes, grace period)
- Hub: 3 tests (peer-left, OnDisconnect)
- HTTP: 5+ tests (new endpoints, error codes)

### React Native Tests
- Store: 5 tests (connect, disconnect, tick, errors)
- Hooks: 6+ tests (useReconnection backoff, useSignaling dispatch)
- Components: 8+ tests (VUMeter, ConnectionStatus, etc.)
- Integration: Full WS flow with mocks

---

## Dependencies & Order

**Critical path** (longest dependency chain):
1. Domain (TASK-001..005)
2. Ports (TASK-006..009)
3. Repository (TASK-010..015)
4. Service (TASK-016..030)
5. Hub (TASK-031..035)
6. HTTP (TASK-036..042)

**Mobile** can run in parallel to backend (minimal dependencies on HTTP endpoints until integration phase).

---

## Notes

- **Strict TDD**: All backend tasks follow TEST → IMPL pattern
- **Atomic**: Each task ~30-90 minutes of focused work
- **Breaking changes**: ServiceConfig param, CreateRoom return type, Hub.Run(ctx)
- **Platform specifics**: iOS build requires `pod install`, Android requires NDK
