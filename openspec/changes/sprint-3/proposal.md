# Sprint 3 Proposal: UX & Edge Cases

**Change**: sprint-3
**Status**: proposed
**Date**: 2026-06-11

---

## 1. Intent

TalkGo has a working backend — rooms, signaling, WebRTC SFU, and a bidirectional translation pipeline — but zero client-facing UI and several robustness gaps that make the system unusable in real-world conditions. When a peer disconnects, the other side gets no notification. Rooms never expire. There is no grace period for transient network drops. The only room identifier is a raw UUID, which is hostile to share verbally.

Sprint 3 delivers the **first usable end-to-end experience**: a React Native conversation screen that connects to the existing backend, combined with critical backend fixes that make the system resilient to disconnections, timeouts, and capacity overflows. After this sprint, two people can create a room via short code, join from their phones, talk in different languages, survive network hiccups, and use the app in the background on both iOS and Android.

---

## 2. Scope

### In Scope

| Area | Item | CA |
|------|------|----|
| **Mobile — Setup** | Initialize React Native CLI (bare workflow) project in `mobile/` | — |
| **Mobile — UI** | Conversation screen: VU meters, connection status indicator, mute toggle, session timer, "Finalizar" button with confirmation dialog | CA-01, CA-02, CA-03 |
| **Mobile — WebRTC** | WebSocket signaling client + `react-native-webrtc` integration (offer/answer/ICE flow against the Go SFU) | — |
| **Mobile — Reconnection** | Automatic reconnection with exponential backoff (1s, 2s, 4s — max 3 attempts), ICE restart on failure | CA-05 |
| **Mobile — Background iOS** | `UIBackgroundModes: audio`, AVAudioSession category `.playAndRecord` with `.allowBluetooth` | CA-09 |
| **Mobile — Background Android** | Foreground Service with persistent notification, `FOREGROUND_SERVICE` + `FOREGROUND_SERVICE_MICROPHONE` permissions | CA-10 |
| **Mobile — Error handling** | Pipeline error fallback: visual indicator when translation fails persistently | CA-07 |
| **Mobile — Bluetooth** | Detect Bluetooth disconnection mid-session, fallback to built-in microphone automatically | CA-08 |
| **Mobile — Keep-awake** | Screen stays on during active session | CA-01 |
| **Backend — peer-left** | When a client unregisters from the Hub, notify the other peer in the same room with `{"type":"peer-left"}` | CA-04 |
| **Backend — Grace period** | 30-second timer after peer-left before closing the room; cancelable if the peer reconnects | CA-06 |
| **Backend — Room expiry** | 10-minute inactivity timer on Room; expired rooms return `"Esta sala expiró. Creá una nueva."` | CA-12 |
| **Backend — Short codes** | 6-character alphanumeric codes (e.g., `A3X7K9`) as room aliases; collision-resistant generation; `GET /rooms/{code}` lookup | CA-11 (PRD) |
| **Backend — Room full error** | Third user joining gets clear error: `"Esta sala ya tiene 2 participantes"` | CA-11 |

### Out of Scope

| Item | Reason |
|------|--------|
| Embedded TURN server | Sprint 4 — for now STUN-only in lab environment |
| Home / onboarding / room creation screens | Not in Sprint 3 spec — conversation screen only |
| User authentication | No auth system designed yet |
| Pion WebRTC v3 → v4 migration | Breaking change; defer to dedicated sprint |
| QR code generation | Deferred — short codes alone are sufficient for Sprint 3; QR is additive UI |
| Persistent room storage (database) | In-memory repository is sufficient for current scope |

---

## 3. Architecture Decisions

### 3.1 Backend Fixes (Go)

#### 3.1.1 `peer-left` Notification

**Where**: `internal/adapters/signaling/hub.go` — `Run()` method, `unregister` case.

**How**: When a client with a non-empty `sessionID` is unregistered, iterate `sessionClients` to find all other clients in the same `roomID` and send them `{"type":"peer-left","session_id":"<departed>"}`. This requires the Hub to know roomID→clients mapping, which it already has implicitly (each `Client` carries `roomID`).

**Implementation**:
1. In the `unregister` case of `Run()`, after removing the client, iterate remaining `clients` filtered by matching `roomID`.
2. For each peer found, send a `peer-left` message via their `send` channel.
3. Additionally, call `handler.HandleSignaling` with a synthetic `leave` message so the Service layer cleans up the session (or add a dedicated `OnDisconnect` method to the SignalingHandler interface — cleaner separation).

**Decision**: Add an `OnDisconnect(ctx, sessionID)` method to `driving.SignalingHandler` rather than synthesizing a fake `leave` message. Fake messages are brittle; an explicit lifecycle method is hexagonal-clean.

#### 3.1.2 Grace Period (30 seconds)

**Where**: `internal/app/roomsvc/service.go` — new method `startGracePeriod(roomID, departedSessionID)`.

**How**:
1. When `LeaveRoom` is called AND the room still has one remaining participant, start a `time.AfterFunc(30 * time.Second, ...)` goroutine.
2. Store the timer handle in a new `graceTimers map[string]*time.Timer` on `Service`.
3. If the departed peer reconnects (joins again) within 30s, cancel the timer (`timer.Stop()`).
4. If the timer fires: call `DeleteRoom` to close the room and notify the remaining peer with `{"type":"room-closed","reason":"peer-timeout"}`.

**Why goroutine + timer instead of a ticker loop**: Single-fire `time.AfterFunc` is simpler, cancelable, and GC-friendly. No polling overhead.

**Testability**: Inject a clock interface (`type Clock interface { AfterFunc(d, f) *Timer }`) so tests can use a fake clock. Alternatively, accept a `gracePeriod time.Duration` in `ServiceConfig` so tests can set it to 1ms.

**Decision**: Use `ServiceConfig` with configurable duration (simpler than clock injection for this scope). Default 30s, tests override to 1ms.

#### 3.1.3 Room Expiry (10 minutes inactivity)

**Where**: `internal/domain/room/room.go` — add `LastActivity time.Time` field. `internal/app/roomsvc/service.go` — expiry sweep.

**How**:
1. Add `LastActivity time.Time` to the `Room` struct. Update it on `Join`, `Leave`, and periodically via signaling activity (each dispatched message touches the room's `LastActivity`).
2. Run a background goroutine in `Service` that sweeps every 60 seconds, closing rooms where `time.Since(r.LastActivity) > expiryDuration`.
3. When a client attempts to join an expired (closed) room, return the existing `ErrRoomClosed` — the HTTP layer maps this to `"Esta sala expiró. Creá una nueva."` (HTTP 410 Gone).

**Decision**: Sweep-based expiry (not per-room timer) to avoid N timers for N rooms. A single goroutine with a 60s tick is lightweight and predictable.

#### 3.1.4 Short Codes

**Where**: `internal/domain/room/shortcode.go` (new file in domain), `internal/app/roomsvc/service.go`, `internal/adapters/http/server.go`.

**How**:
1. **Generation**: `GenerateShortCode() string` — 6 characters from alphabet `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (no 0/O/1/I to avoid ambiguity). Uses `crypto/rand`.
2. **Storage**: Add `ShortCode string` field to `Room` struct. Add `FindByShortCode(ctx, code) (*Room, error)` to `driven.RoomRepository` interface.
3. **Collision handling**: On `CreateRoom`, generate code, check `FindByShortCode` — if collision, regenerate (max 5 attempts). With 30 chars^6 = ~729M combinations, collisions are astronomically rare for our scale.
4. **HTTP endpoint**: `GET /rooms/code/{code}` returns `{"room_id":"<uuid>","short_code":"<code>"}` or 404.
5. **CreateRoom response**: Change from `{"room_id":"<uuid>"}` to `{"room_id":"<uuid>","short_code":"<code>"}`.

**Decision**: Short code lives in the domain (`Room` struct) because it is an intrinsic room identifier, not an adapter concern. The alphanumeric alphabet excludes ambiguous characters (0/O, 1/I) for verbal sharing.

### 3.2 Mobile (React Native)

#### 3.2.1 Project Setup

**Toolchain**: React Native CLI (bare workflow), NOT Expo managed. Reason: `react-native-webrtc` requires native modules that Expo Go cannot load. Expo bare workflow is acceptable but adds unnecessary abstraction.

**Structure**:
```
mobile/
├── android/
├── ios/
├── src/
│   ├── screens/
│   │   └── ConversationScreen.tsx
│   ├── hooks/
│   │   ├── useWebRTC.ts
│   │   ├── useSignaling.ts
│   │   ├── useAudioLevel.ts
│   │   └── useReconnection.ts
│   ├── services/
│   │   ├── SignalingService.ts
│   │   └── AudioService.ts
│   ├── state/
│   │   └── sessionStore.ts
│   └── types/
│       └── signaling.ts
├── App.tsx
├── package.json
└── tsconfig.json
```

#### 3.2.2 State Management

**Options considered**:

| Option | Pros | Cons |
|--------|------|------|
| Context + useReducer | Zero deps, built-in React | Re-renders on every state change, verbose boilerplate |
| Zustand | Tiny (1.1kB), no provider, selector-based re-renders | Extra dependency |
| Redux Toolkit | Powerful, middleware for side effects | Overkill for single-screen state |

**Decision**: **Zustand**. The session state (connection status, mute, timer, error) is a single global store used by one screen and multiple hooks. Zustand's selector model prevents unnecessary re-renders on VU meter updates (critical for 60fps). Zero boilerplate. No context provider tree. If the app grows to multiple screens in Sprint 4+, Zustand scales without migration.

#### 3.2.3 WebRTC Integration

The Go backend acts as an SFU — clients do NOT connect to each other peer-to-peer. The mobile client:
1. Opens a WebSocket to `ws://<host>/ws/{roomID}`
2. Sends `{"type":"join","room_id":"...","user_id":"...","lang":"es"}`
3. Receives `{"type":"joined","session_id":"..."}`
4. Creates an `RTCPeerConnection` with the server's STUN config
5. Adds a local audio track (microphone)
6. Creates an SDP offer, sends `{"type":"offer","session_id":"...","sdp":"..."}`
7. Receives `{"type":"answer","session_id":"...","sdp":"..."}`
8. Exchanges ICE candidates bidirectionally
9. Receives remote audio track (translated audio from the other peer via the server pipeline)

Library: `react-native-webrtc` — the only maintained RN library with native WebRTC bindings for both iOS and Android.

#### 3.2.4 VU Meters

**Approach**: Use the `RTCPeerConnection` audio tracks. For the local track, use `react-native-webrtc`'s audio level API or analyze PCM samples from the local stream. For the remote track, same approach on the incoming stream.

If `react-native-webrtc` does not expose audio levels directly (it historically doesn't), use a native module bridge:
- **iOS**: `AVAudioEngine` tap on the input/output nodes → RMS calculation → send to JS via event emitter
- **Android**: `AudioRecord` or `Visualizer` API → RMS → event emitter

**Fallback**: If native module complexity is too high for this sprint, use a simple "speaking/not-speaking" binary indicator based on WebRTC's `voice-activity-detection` (VAD) stats from `getStats()`. This satisfies CA-02 at minimum.

**Decision**: Start with `getStats()` VAD approach (simpler, no native code). If visual quality is insufficient, escalate to native module in a follow-up.

#### 3.2.5 Background Audio — iOS

1. `Info.plist`: Add `UIBackgroundModes: ["audio"]`
2. `AVAudioSession` configuration in the native AppDelegate (or via `react-native-webrtc`'s built-in session management):
   - Category: `.playAndRecord`
   - Options: `.allowBluetooth`, `.defaultToSpeaker`
   - Activate on session start, deactivate on session end
3. `react-native-webrtc` handles most of this automatically when `UIBackgroundModes: audio` is set. Verify with device testing.

#### 3.2.6 Background Audio — Android

1. `AndroidManifest.xml`:
   - `<uses-permission android:name="android.permission.FOREGROUND_SERVICE" />`
   - `<uses-permission android:name="android.permission.FOREGROUND_SERVICE_MICROPHONE" />` (Android 14+)
   - `<uses-permission android:name="android.permission.RECORD_AUDIO" />`
2. Create a `CallForegroundService` (native Java/Kotlin) that:
   - Shows a persistent notification ("TalkGo — Conversación activa")
   - Holds a partial wake lock
   - Starts on session begin, stops on session end
3. Bridge to JS via `NativeModules` — the `useWebRTC` hook starts/stops the service.

**Decision**: Write the foreground service in Kotlin (modern, concise). Use `react-native-foreground-service` package if it meets requirements; otherwise write minimal native code (~50 lines).

#### 3.2.7 Bluetooth Fallback (CA-08)

- iOS: `AVAudioSession` route change notification (`AVAudioSession.routeChangeNotification`). When Bluetooth disconnects, iOS automatically falls back to the built-in mic/speaker. Verify this behavior; if not automatic, explicitly set the audio route.
- Android: `AudioManager.ACTION_SCO_AUDIO_STATE_UPDATED` broadcast. On disconnect, release SCO and route to built-in mic/speaker.

**Decision**: Both platforms handle Bluetooth fallback semi-automatically when `AVAudioSession` / `AudioManager` are configured correctly. The main work is detecting the event and showing a toast notification to the user ("Bluetooth desconectado, usando micrófono integrado").

#### 3.2.8 Reconnection with Exponential Backoff (CA-05)

Implemented in `useReconnection.ts` hook:

```
State machine: CONNECTED → RECONNECTING → CONNECTED | FAILED

On WebSocket close or ICE disconnected:
  1. Set state = RECONNECTING
  2. Attempt 1: wait 1s → reconnect WS + ICE restart
  3. Attempt 2: wait 2s → reconnect WS + ICE restart
  4. Attempt 3: wait 4s → reconnect WS + ICE restart
  5. If all fail: state = FAILED → show "Conexión perdida" UI
```

ICE restart: on reconnection, create a new offer with `iceRestart: true` on the `RTCPeerConnection`. The server-side Pion handles ICE restart transparently.

#### 3.2.9 Keep-Awake (CA-01)

Use `react-native-keep-awake` (or the built-in `useKeepAwake` from `@react-native-community/keep-awake`). Activate on session start, deactivate on session end. Trivial integration.

---

## 4. Dependencies

### Go (go.mod) — No new dependencies

All backend fixes use only stdlib (`time`, `crypto/rand`, `sync`) and existing dependencies. No new modules required.

### React Native (package.json) — New project

| Package | Purpose | Version (approx) |
|---------|---------|-------------------|
| `react-native` | Framework | 0.76.x |
| `react-native-webrtc` | WebRTC native bindings | ^118.x |
| `zustand` | State management | ^5.x |
| `react-native-keep-awake` | Prevent screen sleep | ^4.x |
| `typescript` | Type safety | ^5.x |
| `@types/react` | React type definitions | ^19.x |

**Conditional** (may use native code instead):
| Package | Purpose | Condition |
|---------|---------|-----------|
| `react-native-foreground-service` | Android background service | Only if custom Kotlin service is too complex |

---

## 5. Risks

| # | Risk | Impact | Probability | Mitigation |
|---|------|--------|-------------|------------|
| 1 | **`react-native-webrtc` compatibility with RN 0.76+** | Blocks entire mobile workstream | Medium | Pin to last known-good version. Check GitHub issues before setup. Fallback: RN 0.74 if needed. |
| 2 | **iOS background audio rejection by Apple** | CA-09 fails on real devices | Low | Use only `audio` background mode (not VOIP). Test on physical device before considering complete. |
| 3 | **Android 14 foreground service restrictions** | CA-10 fails on Android 14+ | Medium | Declare `FOREGROUND_SERVICE_MICROPHONE` permission explicitly. Test on Android 14 emulator/device. |
| 4 | **No TURN server — WebRTC fails behind symmetric NAT** | Cannot connect in some network environments | High (in prod) / Low (in lab) | Document constraint. Sprint 4 adds TURN. For Sprint 3, test on same LAN or with port-forwarded STUN. |
| 5 | **VU meters performance on low-end devices** | Janky UI, battery drain | Medium | Start with VAD-based binary indicator. Only escalate to RMS-based meters if performance allows. |
| 6 | **Grace period race conditions** | Room closed prematurely or zombie rooms | Medium | TDD: write table-driven tests for all timer states (cancel, fire, reconnect-before-fire, reconnect-after-fire). Use configurable duration for fast tests. |
| 7 | **Short code collisions under concurrent creation** | Two rooms get same code | Very Low | Retry loop with max 5 attempts. 729M possible codes vs. expected <100 concurrent rooms. Log collision events for monitoring. |

---

## 6. Definition of Done

All criteria reference the Sprint 3 acceptance criteria (CA-01 through CA-12).

### Backend

- [ ] **CA-04**: `peer-left` message sent to remaining peer when a client disconnects. Verified by integration test (two WS clients, one disconnects, other receives `{"type":"peer-left"}`).
- [ ] **CA-06**: Grace period of 30s before room closure after peer disconnect. Verified by unit test with configurable timer.
- [ ] **CA-11**: Third user joining a full room receives `{"type":"error","message":"Esta sala ya tiene 2 participantes"}`. Verified by unit test.
- [ ] **CA-12**: Room expires after 10 min inactivity. Expired room returns appropriate error. Verified by unit test with configurable expiry.
- [ ] Short codes generated on room creation, returned in response, and resolvable via `GET /rooms/code/{code}`. Verified by integration test.
- [ ] All new backend code has table-driven tests with >80% coverage on changed files.
- [ ] `go vet ./...` and `golangci-lint run` pass with zero issues.

### Mobile

- [ ] **CA-01**: Screen stays awake during active session. Verified on physical device.
- [ ] **CA-02**: VU meters (or at minimum, speaking indicator) respond to audio in real time. Verified on physical device.
- [ ] **CA-03**: "Finalizar" button shows confirmation dialog. Both "cancel" and "confirm" paths work correctly.
- [ ] **CA-04** (mobile side): Both devices see end-of-session notification when either user ends the call.
- [ ] **CA-05**: Reconnection with exponential backoff (1s, 2s, 4s). After 3 failures, shows "Conexión perdida". Verified by killing server and observing retry behavior.
- [ ] **CA-07**: When translation pipeline errors reach the client, a fallback visual indicator appears. Verified by simulating server-side pipeline error.
- [ ] **CA-08**: Bluetooth disconnect mid-session falls back to built-in mic. Toast notification shown. Verified on physical device with Bluetooth headset.
- [ ] **CA-09**: iOS session continues in background. Verified on physical iOS device.
- [ ] **CA-10**: Android Foreground Service with persistent notification keeps session alive. Verified on physical Android device (Android 14+).
- [ ] React Native project builds and runs on both iOS simulator and Android emulator.
- [ ] TypeScript strict mode, zero type errors.

### Integration

- [ ] End-to-end: two physical devices create room via short code, join, speak in different languages, hear translated audio, one disconnects, other gets notification, reconnection works within grace period.

---

## Appendix: Task Ordering (high-level)

The recommended implementation order respects dependencies:

1. **Backend fixes first** (no mobile dependency):
   - `peer-left` notification → grace period → room expiry → short codes
2. **Mobile project setup** (parallel with backend fixes):
   - RN CLI init → TypeScript config → `react-native-webrtc` install → basic WS connection
3. **Mobile conversation screen** (depends on backend fixes being testable):
   - Signaling hook → WebRTC hook → UI components → reconnection → background mode → Bluetooth fallback
4. **Integration testing** (depends on everything):
   - End-to-end on physical devices
