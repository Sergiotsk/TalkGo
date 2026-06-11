# Sprint 3 Technical Design: UX & Edge Cases

**Change**: sprint-3
**Status**: design
**Date**: 2026-06-11
**Inputs**: proposal (engram #219), spec (engram #222), codebase analysis

---

## Section 1: Workstream A — Backend Go

### 1.1 Domain Changes (`internal/domain/room/room.go`)

**Current state**: `Room` struct has `ID`, `SourceLang`, `TargetLang`, `CreatedAt`, `Active`, `Participants`, `Capacity`, `mu`.

**New fields**:

```go
type Room struct {
    ID           string
    ShortCode    string    // 6-char alphanumeric, set at creation, immutable after
    SourceLang   string
    TargetLang   string
    CreatedAt    time.Time
    LastActivity time.Time // updated on Join, Leave, and by service on signaling activity
    Active       bool
    Participants map[string]struct{}
    Capacity     int
    mu           sync.Mutex
}
```

**New domain errors**:

```go
var ErrShortCodeExhausted = errors.New("short code generation exhausted after max retries")
```

**New domain constants and functions**:

```go
// shortCodeAlphabet excludes 0/O/1/I to avoid visual ambiguity.
const shortCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
const shortCodeLength = 6

// GenerateShortCode produces a 6-char code using crypto/rand.
// This is a pure function — collision detection is the caller's responsibility.
func GenerateShortCode() (string, error)
```

**Design decision**: `GenerateShortCode()` is a package-level function, NOT a method on Room. The Room domain object should not know about persistence or collision detection. The service layer calls `GenerateShortCode()`, checks for collision via `repo.FindByShortCode`, and retries up to 5 times. This keeps the domain pure.

**Modified `NewRoom` signature** — no change. ShortCode is set externally by the service after creation, before Save. This avoids polluting domain construction with retry logic.

**`LastActivity` updates within domain**:

```go
// TouchActivity updates LastActivity to time.Now().
// Called internally by Join and Leave.
func (r *Room) TouchActivity()
```

Both `Join()` and `Leave()` call `r.TouchActivity()` at the end of their happy path (inside the existing mutex lock). The service layer also calls `TouchActivity()` on signaling events (offer, ice-candidate) to keep the room alive during active WebRTC sessions.

**Why LastActivity lives in the domain**: It represents a business invariant — rooms expire after inactivity. The domain owns the concept; the service owns the sweep policy.

---

### 1.2 Changes to Driving Ports (`internal/ports/driving/`)

#### `signaling.go` — Add `OnDisconnect` to `SignalingHandler`

```go
type SignalingHandler interface {
    HandleSignaling(ctx context.Context, msg SignalingMessage) (SignalingMessage, error)

    // OnDisconnect is called by the Hub when a client's WebSocket connection drops
    // (abrupt or clean close). The handler starts the grace period for the room.
    // Returns ErrSessionNotFound if sessionID is unknown.
    OnDisconnect(ctx context.Context, sessionID string) error
}
```

**Design decision**: `OnDisconnect` is on `SignalingHandler` (not `RoomManager`) because it is triggered by the signaling infrastructure (Hub), not by HTTP endpoints. The Hub already holds a `driving.SignalingHandler` reference, so no new wiring is needed.

**OQ-02 resolution**: Hub calls `handler.OnDisconnect(sessionID)` — it does NOT call `LeaveRoom` directly. OnDisconnect starts the grace timer; LeaveRoom is the voluntary/clean exit path (no grace period). These are distinct flows.

#### `room_manager.go` — Add `FindByShortCode` and `UpdateLastActivity`

```go
type RoomManager interface {
    // ... existing methods ...

    // FindByShortCode retrieves a room by its 6-char short code.
    // Returns ErrRoomNotFound if no room matches.
    // Code is normalized to uppercase before lookup.
    FindByShortCode(ctx context.Context, code string) (*room.Room, error)

    // UpdateLastActivity refreshes the LastActivity timestamp for the given room.
    // Returns ErrRoomNotFound if the room does not exist.
    UpdateLastActivity(ctx context.Context, roomID string) error
}
```

---

### 1.3 Changes to Driven Ports (`internal/ports/driven/`)

#### `room_repository.go` — Three new methods

```go
type RoomRepository interface {
    // ... existing: Save, FindByID, Delete, ListActive ...

    // FindByShortCode retrieves a room by its short code.
    // Returns driving.ErrRoomNotFound if not found.
    // Implementations MUST normalize code to uppercase before comparison.
    FindByShortCode(ctx context.Context, code string) (*room.Room, error)

    // UpdateLastActivity sets room.LastActivity = time.Now() for the given roomID.
    // Returns driving.ErrRoomNotFound if roomID does not exist.
    UpdateLastActivity(ctx context.Context, roomID string) error

    // ListExpired returns all active rooms where LastActivity < before.
    // Used by the expiration sweep goroutine.
    ListExpired(ctx context.Context, before time.Time) ([]*room.Room, error)
}
```

**Design decision**: `ListExpired` is a driven port method (not a service-level filter) because in a future SQL implementation, this should be a WHERE clause, not a full table scan + filter. Even for the in-memory store, having the interface right from the start avoids a breaking change later.

**OQ-03 resolution**: Short codes are case-insensitive. `FindByShortCode` normalizes to uppercase via `strings.ToUpper(code)` before lookup. The alphabet is already uppercase-only, so stored codes are always uppercase.

---

### 1.4 Service Changes (`internal/app/roomsvc/service.go`)

#### `ServiceConfig` struct

```go
// ServiceConfig holds tunable parameters for the room service.
// All fields have sensible defaults via DefaultServiceConfig().
type ServiceConfig struct {
    GracePeriod   time.Duration // Time before closing room after disconnect (default: 30s)
    RoomTTL       time.Duration // Max inactivity before room expiry (default: 10min)
    SweepInterval time.Duration // How often the sweep goroutine runs (default: 60s)
    MaxShortCodeRetries int     // Max collision retries for short code (default: 5)
}

func DefaultServiceConfig() ServiceConfig {
    return ServiceConfig{
        GracePeriod:         30 * time.Second,
        RoomTTL:             10 * time.Minute,
        SweepInterval:       60 * time.Second,
        MaxShortCodeRetries: 5,
    }
}
```

#### Modified `Service` struct

```go
type Service struct {
    cfg        ServiceConfig
    repo       driven.RoomRepository
    peer       driven.WebRTCPeer
    translator driven.Translator
    codec      driven.AudioCodec
    notifier   driven.EventNotifier
    sessions   map[string]*session.Session
    lookup     map[string]string
    pipelines  map[string]*pipeline
    graceTimers map[string]*time.Timer  // roomID → grace period timer
    mu         sync.RWMutex
}
```

#### Modified `NewService` signature

```go
func NewService(
    cfg ServiceConfig,
    repo driven.RoomRepository,
    peer driven.WebRTCPeer,
    translator driven.Translator,
    codec driven.AudioCodec,
    notifier driven.EventNotifier,
) (*Service, error)
```

**Breaking change**: `cfg` is now the first parameter. All callers (main.go, tests) must be updated.

#### `OnDisconnect(ctx context.Context, sessionID string) error`

Flow:
1. `s.mu.RLock()` — look up session by sessionID.
2. If not found → return `driving.ErrSessionNotFound`.
3. Get `roomID` from session. Count remaining participants in the room.
4. `s.mu.RUnlock()`.
5. If room has 0 remaining participants (both disconnected) → do NOT start grace timer. Call `DeleteRoom` immediately. Clean up graceTimers entry if exists.
6. If room has 1 remaining participant → start grace timer:
   ```go
   s.mu.Lock()
   if existing, ok := s.graceTimers[roomID]; ok {
       existing.Stop()
   }
   s.graceTimers[roomID] = time.AfterFunc(s.cfg.GracePeriod, func() {
       // Notify remaining peer
       s.notifyRoomPeers(roomID, "room-closed", map[string]string{"reason": "peer-timeout"})
       // Delete room
       _ = s.DeleteRoom(context.Background(), roomID)
       s.mu.Lock()
       delete(s.graceTimers, roomID)
       s.mu.Unlock()
   })
   s.mu.Unlock()
   ```
7. Do NOT call `LeaveRoom` — the session stays in the lookup so that reconnection can cancel the timer.

**Critical design decision**: `OnDisconnect` does NOT remove the session from internal maps. The session remains registered so that if the same user reconnects (new WS, same userID), `JoinRoom` detects the existing session via the lookup map and cancels the grace timer. This is the reconnection path.

#### Grace timer cancellation in `JoinRoom`

After the existing `r.Join(userID)` succeeds, add:

```go
s.mu.Lock()
if timer, ok := s.graceTimers[roomID]; ok {
    timer.Stop()
    delete(s.graceTimers, roomID)
}
s.mu.Unlock()
```

This cancels any pending grace timer when a peer reconnects.

**Edge case**: If the reconnecting user is a NEW userID (not the same as who disconnected), the grace timer should still be cancelled because the room now has 2 participants again.

#### Modified `CreateRoom` — ShortCode generation

```go
func (s *Service) CreateRoom(ctx context.Context, sourceLang, targetLang string) (string, error) {
    r, err := room.NewRoom(uuid.NewString(), sourceLang, targetLang)
    if err != nil {
        return "", fmt.Errorf("roomsvc.CreateRoom: %w", err)
    }

    // Generate collision-free short code
    for attempt := 0; attempt < s.cfg.MaxShortCodeRetries; attempt++ {
        code, err := room.GenerateShortCode()
        if err != nil {
            return "", fmt.Errorf("roomsvc.CreateRoom: generating short code: %w", err)
        }
        _, findErr := s.repo.FindByShortCode(ctx, code)
        if errors.Is(findErr, driving.ErrRoomNotFound) {
            // No collision — use this code
            r.ShortCode = code
            break
        }
        if findErr != nil {
            return "", fmt.Errorf("roomsvc.CreateRoom: checking short code: %w", findErr)
        }
        // Collision — retry
    }
    if r.ShortCode == "" {
        return "", fmt.Errorf("roomsvc.CreateRoom: %w", room.ErrShortCodeExhausted)
    }

    r.LastActivity = time.Now()
    if err := s.repo.Save(ctx, r); err != nil {
        return "", fmt.Errorf("roomsvc.CreateRoom: saving room: %w", err)
    }
    return r.ID, nil
}
```

**Return value change**: `CreateRoom` currently returns `(string, error)` where string is the room ID. The HTTP handler also needs the short code. Two options:

- **Option A**: Return a struct `CreateRoomResult{ID, ShortCode}` — cleaner but breaks the interface.
- **Option B**: The HTTP handler calls `FindByShortCode` or `FindByID` after creation — extra round-trip.
- **Chosen: Option A**. Modify the `RoomManager` interface:

```go
type CreateRoomResult struct {
    RoomID    string
    ShortCode string
}

CreateRoom(ctx context.Context, sourceLang, targetLang string) (CreateRoomResult, error)
```

#### `FindByShortCode(ctx context.Context, code string) (*room.Room, error)`

Delegates to `s.repo.FindByShortCode(ctx, strings.ToUpper(code))`. Also checks `r.Active` — if room is inactive, returns `room.ErrRoomClosed`.

#### `UpdateLastActivity(ctx context.Context, roomID string) error`

Delegates to `s.repo.UpdateLastActivity(ctx, roomID)`.

Also called internally by `HandleSignaling` on "offer" and "ice-candidate" messages to keep the room alive.

#### `startExpirationSweep(ctx context.Context)`

```go
func (s *Service) startExpirationSweep(ctx context.Context) {
    ticker := time.NewTicker(s.cfg.SweepInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            cutoff := time.Now().Add(-s.cfg.RoomTTL)
            expired, err := s.repo.ListExpired(ctx, cutoff)
            if err != nil {
                slog.Error("expiration sweep: listing expired rooms", slog.Any("err", err))
                continue
            }
            for _, r := range expired {
                slog.Info("expiring room", slog.String("roomID", r.ID),
                    slog.Time("lastActivity", r.LastActivity))
                // Notify all connected peers before deletion
                s.notifyRoomPeers(r.ID, "room-closed", map[string]string{"reason": "expired"})
                if err := s.DeleteRoom(ctx, r.ID); err != nil {
                    slog.Error("expiration sweep: deleting room",
                        slog.String("roomID", r.ID), slog.Any("err", err))
                }
            }
        }
    }
}
```

**Design decision**: Sweep-based (single goroutine) not per-room timers. With N rooms, per-room timers create N goroutines. The sweep is O(rooms) every 60s — negligible for the expected scale (< 1000 rooms). The sweep also doubles as a health-check mechanism.

#### Helper: `notifyRoomPeers`

```go
// notifyRoomPeers sends a notification to all connected peers in a room.
// excludeSessionID can be empty to notify all.
func (s *Service) notifyRoomPeers(roomID, msgType string, fields map[string]string) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for sessID, sess := range s.sessions {
        if sess.RoomID == roomID {
            s.notifier.NotifySession(sessID, msgType, fields)
        }
    }
}
```

#### New error sentinels (`errors.go`)

```go
var ErrShortCodeExhausted = errors.New("short code generation exhausted")
// Note: this duplicates room.ErrShortCodeExhausted — keep only in domain.
```

Actually, keep `ErrShortCodeExhausted` ONLY in `internal/domain/room/` since it's a domain constraint. Service wraps it with `fmt.Errorf`.

---

### 1.5 Hub Changes (`internal/adapters/signaling/hub.go`)

**Current state of `Client`**: Already has `sessionID string` field (confirmed in `client.go` line 26) and `roomID string` (line 25). The Hub already has `sessionClients map[string]*Client` (line 31).

#### `unregister` case — Notify peer-left + call OnDisconnect

```go
case c := <-h.unregister:
    h.mu.Lock()
    if _, ok := h.clients[c]; ok {
        delete(h.clients, c)
        close(c.send)
    }

    sessionID := c.sessionID
    roomID := c.roomID

    if sessionID != "" {
        delete(h.sessionClients, sessionID)

        // Notify other peers in the same room that this peer left
        for other := range h.clients {
            if other.roomID == roomID && other != c {
                msg, _ := json.Marshal(map[string]string{
                    "type":       "peer-left",
                    "session_id": sessionID,
                })
                select {
                case other.send <- msg:
                default:
                    // buffer full — drop
                }
            }
        }
    }
    h.mu.Unlock()

    // Call OnDisconnect OUTSIDE the lock to avoid deadlock
    // (OnDisconnect may call NotifySession which acquires mu.RLock)
    if sessionID != "" {
        handler := h.getHandler()
        if handler != nil {
            if err := handler.OnDisconnect(context.Background(), sessionID); err != nil {
                slog.Error("OnDisconnect", slog.String("sessionID", sessionID),
                    slog.Any("err", err))
            }
        }
    }
```

**Critical design decision**: `OnDisconnect` is called OUTSIDE `h.mu.Lock()` to prevent deadlock. The flow is: Hub.unregister holds `mu.Lock` → calls `OnDisconnect` → service calls `notifier.NotifySession` → Hub.NotifySession acquires `mu.RLock` → DEADLOCK if called inside Lock. Solution: release the lock first, then call OnDisconnect.

**Helper method** to safely read handler:
```go
func (h *Hub) getHandler() driving.SignalingHandler {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return h.handler
}
```

#### `dispatch` — Handle "leave" message (voluntary)

The existing "leave" case in `HandleSignaling` already calls `LeaveRoom`. No grace period for voluntary leave. No changes needed in the Hub for this path.

#### `Run()` — Accept context for graceful shutdown

```go
func (h *Hub) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case c := <-h.register:
            // ...
        case c := <-h.unregister:
            // ...
        }
    }
}
```

This allows `main.go` to cancel the Hub goroutine on shutdown.

---

### 1.6 Repository Changes (`internal/app/roomsvc/repository.go`)

The `InMemoryRoomRepository` needs three new methods:

#### `FindByShortCode`

```go
func (r *InMemoryRoomRepository) FindByShortCode(_ context.Context, code string) (*room.Room, error) {
    normalized := strings.ToUpper(code)
    r.mu.RLock()
    defer r.mu.RUnlock()
    for _, rm := range r.rooms {
        if rm.ShortCode == normalized {
            return rm, nil
        }
    }
    return nil, fmt.Errorf("roomsvc.FindByShortCode: %w", driving.ErrRoomNotFound)
}
```

**Complexity**: O(n) scan. Acceptable for in-memory MVP. A SQL implementation would use an indexed column.

#### `UpdateLastActivity`

```go
func (r *InMemoryRoomRepository) UpdateLastActivity(_ context.Context, roomID string) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    rm, ok := r.rooms[roomID]
    if !ok {
        return fmt.Errorf("roomsvc.UpdateLastActivity: %w", driving.ErrRoomNotFound)
    }
    rm.LastActivity = time.Now()
    return nil
}
```

#### `ListExpired`

```go
func (r *InMemoryRoomRepository) ListExpired(_ context.Context, before time.Time) ([]*room.Room, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    var expired []*room.Room
    for _, rm := range r.rooms {
        if rm.Active && rm.LastActivity.Before(before) {
            expired = append(expired, rm)
        }
    }
    return expired, nil
}
```

---

### 1.7 HTTP Adapter Changes (`internal/adapters/http/server.go`)

#### New routes

```go
func (s *Server) registerRoutes() {
    s.mux.HandleFunc("GET /health", s.healthHandler)
    s.mux.HandleFunc("POST /rooms", s.createRoomHandler)
    s.mux.HandleFunc("DELETE /rooms/{id}", s.deleteRoomHandler)
    s.mux.HandleFunc("GET /rooms/code/{code}", s.findByShortCodeHandler) // NEW
    s.mux.HandleFunc("GET /ws/{roomID}", s.wsHandler)
}
```

#### `createRoomHandler` — Include short_code in response

```go
func (s *Server) createRoomHandler(w http.ResponseWriter, r *http.Request) {
    // ... decode request ...
    result, err := s.manager.CreateRoom(r.Context(), req.SourceLang, req.TargetLang)
    // ... error handling ...
    writeJSON(w, http.StatusCreated, map[string]string{
        "room_id":    result.RoomID,
        "short_code": result.ShortCode,
    })
}
```

#### `findByShortCodeHandler`

```go
func (s *Server) findByShortCodeHandler(w http.ResponseWriter, r *http.Request) {
    code := r.PathValue("code")
    rm, err := s.manager.FindByShortCode(r.Context(), code)
    if err != nil {
        if errors.Is(err, driving.ErrRoomNotFound) {
            writeError(w, http.StatusNotFound, "room not found")
            return
        }
        if errors.Is(err, room.ErrRoomClosed) {
            writeError(w, http.StatusGone, "Esta sala expiró. Creá una nueva.")
            return
        }
        writeError(w, http.StatusInternalServerError, "internal server error")
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{
        "room_id":    rm.ID,
        "short_code": rm.ShortCode,
    })
}
```

#### Error mapping for room full (REQ-A05)

In `wsHandler` or wherever JoinRoom errors surface over HTTP:

```go
if errors.Is(err, room.ErrRoomFull) {
    writeError(w, http.StatusConflict, "Esta sala ya tiene 2 participantes")
    return
}
```

For WS signaling, the existing `HandleSignaling` error path already sends `{"type":"error","message":"..."}` — the domain error message propagates. The specific message "Esta sala ya tiene 2 participantes" should be set in the service layer when wrapping `room.ErrRoomFull`.

---

### 1.8 `cmd/server/main.go` Changes

```go
func main() {
    // ... logger setup ...

    cfg := roomsvc.DefaultServiceConfig()
    // Override from env if needed:
    // cfg.GracePeriod = parseDuration(os.Getenv("GRACE_PERIOD"), 30*time.Second)

    // ... driven adapters (same as before) ...

    hub := signaling.NewHub(nil)

    svc, err := roomsvc.NewService(cfg, repo, peer, tr, codec, hub)
    if err != nil {
        slog.Error("creating service", slog.Any("err", err))
        os.Exit(1)
    }

    hub.SetHandler(svc)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go hub.Run(ctx)
    go svc.StartExpirationSweep(ctx) // exported method that calls startExpirationSweep

    srv := httpserver.NewServer(httpserver.DefaultConfig(), svc, hub)

    slog.Info("TalkGo starting")
    if err := srv.ListenAndServe(ctx); err != nil {
        slog.Error("server error", slog.Any("err", err))
        os.Exit(1)
    }
}
```

**`StartExpirationSweep` is exported** so main.go can launch it. Internally it's just:
```go
func (s *Service) StartExpirationSweep(ctx context.Context) {
    s.startExpirationSweep(ctx)
}
```

---

## Section 2: Workstream B — React Native

### 2.1 Project Structure

```
mobile/
  src/
    screens/
      ConversationScreen.tsx    # Main call screen
    components/
      VUMeter.tsx               # Animated voice level indicator
      ConnectionStatus.tsx      # "Conectando...", "Conectado", "Reconectando..."
      MuteButton.tsx            # Toggle mute with visual feedback
      SessionTimer.tsx          # MM:SS elapsed timer
      EndCallButton.tsx         # "Finalizar" with confirmation dialog
      PipelineErrorBanner.tsx   # Translation error fallback indicator
    hooks/
      useWebRTC.ts              # RTCPeerConnection lifecycle
      useSignaling.ts           # WebSocket client for signaling protocol
      useAudioLevel.ts          # VAD-based speaking detection via getStats
      useReconnection.ts        # Exponential backoff state machine
      useKeepAwake.ts           # Thin wrapper around react-native-keep-awake
      useSessionTimer.ts        # MM:SS timer with Zustand integration
    store/
      sessionStore.ts           # Zustand store — single source of truth
    services/
      api.ts                    # HTTP client: createRoom, findByShortCode
      signalingService.ts       # WebSocket message serialization/deserialization
    types/
      signaling.ts              # Message types matching Go SignalingMessage
      session.ts                # Session-related types
    native/
      android/
        CallForegroundService.kt
      ios/
        AudioSessionManager.swift
  ios/                          # RN native iOS project
  android/                      # RN native Android project
  package.json
  tsconfig.json
  babel.config.js
  metro.config.js
  index.js
  App.tsx
```

### 2.2 Session State (Zustand Store)

```typescript
// store/sessionStore.ts

type ConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'failed';

interface SessionState {
  // Connection
  connectionState: ConnectionState;
  roomId: string | null;
  shortCode: string | null;
  sessionId: string | null;

  // Languages
  localLang: string;     // ISO 639-1
  peerLang: string;      // ISO 639-1

  // Audio state
  isMuted: boolean;
  localSpeaking: boolean;   // boolean, NOT float — avoids re-renders at 10Hz
  peerSpeaking: boolean;    // boolean, NOT float

  // Error state
  pipelineError: string | null;
  consecutiveErrors: number;

  // Timer
  elapsedSeconds: number;

  // Reconnection
  reconnectAttempt: number;  // 0 = not reconnecting
}

interface SessionActions {
  // Lifecycle
  connect: (roomId: string, shortCode: string, sessionId: string, localLang: string, peerLang: string) => void;
  disconnect: () => void;
  setConnectionState: (state: ConnectionState) => void;

  // Audio
  setMuted: (muted: boolean) => void;
  setLocalSpeaking: (speaking: boolean) => void;
  setPeerSpeaking: (speaking: boolean) => void;

  // Errors
  setPipelineError: (error: string | null) => void;
  incrementErrors: () => void;
  resetErrors: () => void;

  // Timer
  tick: () => void;
  resetTimer: () => void;

  // Reconnection
  setReconnectAttempt: (attempt: number) => void;
}

type SessionStore = SessionState & SessionActions;
```

**Design decision**: `localSpeaking` and `peerSpeaking` are `boolean`, not `number`. The VU meter component interpolates visually via CSS animations or Reanimated, but the store only tracks whether someone IS speaking. This reduces re-renders from 10Hz to event-based (speaking start/stop).

**Selector pattern** for VU meters:

```typescript
// In VUMeter.tsx — subscribes to ONLY the boolean it needs
const localSpeaking = useSessionStore(s => s.localSpeaking);
```

This is the key performance optimization. Without selectors, every `tick()` call (every second for the timer) would re-render the VU meters.

### 2.3 Hook: `useWebRTC`

```typescript
interface UseWebRTCReturn {
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  iceConnectionState: RTCIceConnectionState;
  createOffer: (options?: RTCOfferOptions) => Promise<RTCSessionDescriptionInit>;
  setRemoteAnswer: (sdp: string) => Promise<void>;
  addIceCandidate: (candidate: string) => Promise<void>;
  close: () => void;
}

function useWebRTC(config?: { iceServers?: RTCIceServer[] }): UseWebRTCReturn;
```

**Responsibilities**:
1. Create `RTCPeerConnection` on mount with STUN-only ICE config (no TURN — Sprint 4).
2. Request microphone permission, create `MediaStream`, add audio track.
3. Listen for `ontrack` → set `remoteStream`.
4. Listen for `oniceconnectionstatechange` → update `iceConnectionState`.
5. Expose `createOffer` with optional `iceRestart: true` for reconnection.
6. On unmount: close PC, stop all tracks, release mic.

**ICE servers config**:
```typescript
const defaultIceServers: RTCIceServer[] = [
  { urls: 'stun:stun.l.google.com:19302' },
  { urls: 'stun:stun1.l.google.com:19302' },
];
```

**react-native-webrtc specifics**:
- Import `RTCPeerConnection`, `RTCSessionDescription`, `RTCIceCandidate`, `mediaDevices` from `react-native-webrtc`.
- Audio constraints: `{ audio: true, video: false }`.
- The hook does NOT manage WS messages — that is `useSignaling`'s job.

### 2.4 Hook: `useSignaling`

```typescript
interface UseSignalingConfig {
  serverUrl: string;      // e.g. "ws://192.168.1.10:8080"
  roomId: string;
  onJoined: (sessionId: string) => void;
  onAnswer: (sdp: string) => void;
  onIceCandidate: (candidate: string) => void;
  onPeerLeft: (sessionId: string) => void;
  onRoomClosed: (reason: string) => void;
  onError: (message: string) => void;
}

interface UseSignalingReturn {
  isConnected: boolean;
  sendJoin: (userId: string, lang: string) => void;
  sendOffer: (sessionId: string, sdp: string) => void;
  sendIceCandidate: (sessionId: string, candidate: string) => void;
  sendLeave: (sessionId: string) => void;
  reconnect: () => void;
  close: () => void;
}

function useSignaling(config: UseSignalingConfig): UseSignalingReturn;
```

**Responsibilities**:
1. Open WebSocket to `${serverUrl}/ws/${roomId}` on mount.
2. Parse inbound messages → dispatch to appropriate callback.
3. Handle WS `onclose` and `onerror` → update `isConnected`, trigger reconnection flow.
4. Ping/pong: the Go server sends pings every 30s. The WS client should respond automatically (browser/RN default behavior). No explicit pong needed.

**Message type mapping** (matches Go `SignalingMessage` struct):
```typescript
// types/signaling.ts
interface SignalingMessage {
  type: 'join' | 'joined' | 'offer' | 'answer' | 'ice-candidate' | 'leave' | 'peer-left' | 'room-closed' | 'error';
  room_id?: string;
  user_id?: string;
  session_id?: string;
  sdp?: string;
  candidate?: string;
  message?: string;
  lang?: string;
  reason?: string;
}
```

### 2.5 Hook: `useReconnection`

```typescript
type ReconnectionState = 'connected' | 'reconnecting' | 'failed';

interface UseReconnectionConfig {
  maxAttempts: number;          // default: 3
  baseDelay: number;            // default: 1000 (ms)
  onReconnect: () => Promise<void>;  // called to attempt WS + ICE restart
  onFailed: () => void;
}

interface UseReconnectionReturn {
  state: ReconnectionState;
  attempt: number;
  trigger: () => void;        // called when disconnect detected
  cancel: () => void;         // called on voluntary leave
  reset: () => void;          // called on successful reconnect
}

function useReconnection(config: UseReconnectionConfig): UseReconnectionReturn;
```

**State machine**:

```
                    WS close / ICE failed
    CONNECTED ──────────────────────────────> RECONNECTING
        ^                                         |
        |    reconnect success                    | attempt <= maxAttempts
        |<────────────────────────────────────────|
        |                                         |
        |                                         v
        |                               attempt > maxAttempts
        |                                         |
        |                                         v
        |                                      FAILED
        |                                         |
        |    user initiates new connection        |
        |<────────────────────────────────────────|
```

**Backoff**: delays = [1s, 2s, 4s]. Formula: `baseDelay * 2^(attempt - 1)`.

**Reconnection flow** (what `onReconnect` does):
1. Re-open WebSocket to same room.
2. Re-send `join` message.
3. On `joined` response, create new offer with `{ iceRestart: true }`.
4. Process answer, exchange ICE candidates.
5. If all succeeds → `reset()` → `CONNECTED`.
6. If any step fails → next attempt or `FAILED`.

**Design decision**: User-initiated `leave` does NOT trigger reconnection. The hook exposes `cancel()` which prevents any pending reconnection attempt.

### 2.6 Background Mode — iOS

#### `Info.plist` additions

```xml
<key>UIBackgroundModes</key>
<array>
    <string>audio</string>
</array>
```

#### `AudioSessionManager.swift` (native module)

```swift
@objc(AudioSessionManager)
class AudioSessionManager: NSObject {

    @objc func activate() {
        let session = AVAudioSession.sharedInstance()
        try? session.setCategory(
            .playAndRecord,
            mode: .voiceChat,
            options: [.allowBluetooth, .defaultToSpeaker]
        )
        try? session.setActive(true)
    }

    @objc func deactivate() {
        try? AVAudioSession.sharedInstance().setActive(false,
            options: .notifyOthersOnDeactivation)
    }
}
```

**Lifecycle**:
- `activate()` called when `ConversationScreen` mounts (session starts).
- `deactivate()` called when session ends (voluntary or room-closed).
- The `.voiceChat` mode optimizes for voice (echo cancellation, noise reduction).
- `.allowBluetooth` enables BT headsets; `.defaultToSpeaker` falls back to speaker.

**Bridging to RN**: Standard RN native module pattern with `RCT_EXTERN_MODULE`.

### 2.7 Background Mode — Android

#### `CallForegroundService.kt`

```kotlin
// android/app/src/main/java/com/talkgo/CallForegroundService.kt

class CallForegroundService : Service() {
    companion object {
        const val CHANNEL_ID = "talkgo_call_channel"
        const val NOTIFICATION_ID = 1
        const val ACTION_STOP = "com.talkgo.STOP_CALL"
    }

    private var wakeLock: PowerManager.WakeLock? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }

        val notification = buildNotification()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }

        acquireWakeLock()
        return START_STICKY
    }

    override fun onDestroy() {
        wakeLock?.release()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun buildNotification(): Notification {
        val stopIntent = Intent(this, CallForegroundService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPending = PendingIntent.getService(this, 0, stopIntent,
            PendingIntent.FLAG_IMMUTABLE)

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("TalkGo")
            .setContentText("Conversación activa")
            .setSmallIcon(R.drawable.ic_notification)
            .setOngoing(true)
            .addAction(R.drawable.ic_stop, "Finalizar", stopPending)
            .build()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID, "Llamada activa",
                NotificationManager.IMPORTANCE_LOW
            )
            getSystemService(NotificationManager::class.java)
                .createNotificationChannel(channel)
        }
    }

    private fun acquireWakeLock() {
        val pm = getSystemService(POWER_SERVICE) as PowerManager
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK,
            "talkgo:call").apply { acquire(60 * 60 * 1000L) } // 1 hour max
    }
}
```

#### `AndroidManifest.xml` additions

```xml
<uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
<uses-permission android:name="android.permission.FOREGROUND_SERVICE_MICROPHONE" />
<uses-permission android:name="android.permission.RECORD_AUDIO" />
<uses-permission android:name="android.permission.WAKE_LOCK" />

<service
    android:name=".CallForegroundService"
    android:foregroundServiceType="microphone"
    android:exported="false" />
```

#### `CallServiceModule.kt` — NativeModules bridge

```kotlin
@ReactModule(name = "CallService")
class CallServiceModule(reactContext: ReactApplicationContext) :
    ReactContextBaseJavaModule(reactContext) {

    override fun getName() = "CallService"

    @ReactMethod
    fun start() {
        val intent = Intent(reactApplicationContext, CallForegroundService::class.java)
        ContextCompat.startForegroundService(reactApplicationContext, intent)
    }

    @ReactMethod
    fun stop() {
        val intent = Intent(reactApplicationContext, CallForegroundService::class.java)
        reactApplicationContext.stopService(intent)
    }
}
```

**Usage from RN**:
```typescript
import { NativeModules, Platform } from 'react-native';

export function startCallService() {
    if (Platform.OS === 'android') {
        NativeModules.CallService.start();
    }
    // iOS: AVAudioSession background mode handles this automatically
}
```

### 2.8 VU Meters

#### `useAudioLevel` hook

```typescript
interface AudioLevels {
  localSpeaking: boolean;
  peerSpeaking: boolean;
}

function useAudioLevel(
  peerConnection: RTCPeerConnection | null,
  intervalMs?: number  // default: 100 (10Hz)
): AudioLevels;
```

**Implementation approach**:

```typescript
function useAudioLevel(pc: RTCPeerConnection | null, intervalMs = 100): AudioLevels {
  const [levels, setLevels] = useState<AudioLevels>({
    localSpeaking: false,
    peerSpeaking: false,
  });

  useEffect(() => {
    if (!pc) return;
    const interval = setInterval(async () => {
      const stats = await pc.getStats();
      let localVAD = false;
      let remoteVAD = false;

      stats.forEach((report) => {
        if (report.type === 'outbound-rtp' && report.kind === 'audio') {
          // voiceActivityFlag from RTP header extension
          localVAD = report.voiceActivityFlag ?? false;
        }
        if (report.type === 'inbound-rtp' && report.kind === 'audio') {
          remoteVAD = report.voiceActivityFlag ?? false;
        }
      });

      setLevels(prev => {
        if (prev.localSpeaking === localVAD && prev.peerSpeaking === remoteVAD) {
          return prev; // no change — avoid re-render
        }
        return { localSpeaking: localVAD, peerSpeaking: remoteVAD };
      });
    }, intervalMs);

    return () => clearInterval(interval);
  }, [pc, intervalMs]);

  return levels;
}
```

**Fallback**: If `voiceActivityFlag` is not available in react-native-webrtc's getStats implementation, fall back to `audioLevel` field (0.0-1.0) with threshold (e.g., > 0.01 = speaking). If neither is available, use `bytesReceived` delta as a proxy (bytes increasing = audio flowing = speaking).

**Integration with Zustand**: The `ConversationScreen` calls `useAudioLevel`, then syncs to store:
```typescript
useEffect(() => {
    sessionStore.getState().setLocalSpeaking(levels.localSpeaking);
    sessionStore.getState().setPeerSpeaking(levels.peerSpeaking);
}, [levels.localSpeaking, levels.peerSpeaking]);
```

### 2.9 npm Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `react-native` | `~0.76.x` | Framework (bare workflow, CLI init) |
| `react-native-webrtc` | `^118.0.7` | WebRTC for RN — known-good with RN 0.76 |
| `zustand` | `^5.0.0` | State management with selector performance |
| `react-native-keep-awake` | `^4.0.0` | Prevent screen sleep during call |
| `typescript` | `^5.5.0` | Type safety |
| `@types/react` | `^18.x` | Types for React |
| `@types/react-native` | Not needed with RN 0.76+ (bundled types) | — |

**NOT included** (out of scope):
- `@react-native-community/blur` — optional, not needed for MVP modal
- Navigation library — single screen for Sprint 3, no routing needed
- `react-native-reanimated` — VU meter animation can use simple Animated API

**react-native-webrtc compatibility note**: Version 118.0.7 is the latest release compatible with RN 0.76. It uses the M118 Chromium WebRTC. Must verify with `npx react-native-webrtc-check` after project init. If incompatible, pin to `^111.0.6` (M111, widely tested).

---

## Section 3: File Change Table

| File | Action | What Changes |
|------|--------|-------------|
| `internal/domain/room/room.go` | modify | +ShortCode field, +LastActivity field, +TouchActivity(), +GenerateShortCode(), +ErrShortCodeExhausted |
| `internal/domain/room/room_test.go` | modify | +TestGenerateShortCode, +TestTouchActivity, +TestJoinUpdatesLastActivity, +TestLeaveUpdatesLastActivity |
| `internal/ports/driving/signaling.go` | modify | +OnDisconnect method on SignalingHandler interface |
| `internal/ports/driving/room_manager.go` | modify | +FindByShortCode, +UpdateLastActivity, CreateRoom returns CreateRoomResult |
| `internal/ports/driven/room_repository.go` | modify | +FindByShortCode, +UpdateLastActivity, +ListExpired |
| `internal/app/roomsvc/service.go` | modify | +ServiceConfig, +graceTimers, +OnDisconnect, +startExpirationSweep, +StartExpirationSweep, +notifyRoomPeers, +FindByShortCode, +UpdateLastActivity, CreateRoom generates ShortCode, JoinRoom cancels grace timer, NewService takes cfg param |
| `internal/app/roomsvc/service_test.go` | modify | +TestOnDisconnect_StartsGracePeriod, +TestOnDisconnect_ReconnectCancelsGrace, +TestOnDisconnect_BothDisconnect_NoGrace, +TestExpirationSweep_DeletesExpiredRooms, +TestExpirationSweep_ActiveRoomNotExpired, +TestShortCode_Uniqueness, +TestCreateRoom_ShortCodeCollisionRetry |
| `internal/app/roomsvc/service_signaling_test.go` | modify | Update NewService calls to include ServiceConfig |
| `internal/app/roomsvc/repository.go` | modify | +FindByShortCode, +UpdateLastActivity, +ListExpired |
| `internal/app/roomsvc/repository_test.go` | modify | +TestFindByShortCode, +TestUpdateLastActivity, +TestListExpired |
| `internal/app/roomsvc/errors.go` | modify | Remove ErrShortCodeExhausted if added here (keep in domain only) |
| `internal/app/roomsvc/pipeline.go` | no change | Pipeline code unaffected |
| `internal/adapters/signaling/hub.go` | modify | unregister: +peer-left notification +OnDisconnect call, Run accepts context, +getHandler helper |
| `internal/adapters/signaling/client.go` | no change | Client struct already has sessionID and roomID |
| `internal/adapters/http/server.go` | modify | +findByShortCodeHandler, +GET /rooms/code/{code} route, createRoom response includes short_code, +ErrRoomFull→409, +ErrRoomClosed→410 |
| `internal/adapters/http/server_test.go` | modify | +TestFindByShortCode_Hit, +TestFindByShortCode_Miss, +TestCreateRoom_IncludesShortCode, +TestJoinRoom_RoomFull_409 |
| `cmd/server/main.go` | modify | +ServiceConfig construction, +ctx/cancel, +sweep goroutine, Hub.Run(ctx) |
| `mobile/` | create | Full React Native bare workflow project |
| `mobile/src/store/sessionStore.ts` | create | Zustand store |
| `mobile/src/hooks/useWebRTC.ts` | create | WebRTC lifecycle hook |
| `mobile/src/hooks/useSignaling.ts` | create | WS signaling hook |
| `mobile/src/hooks/useReconnection.ts` | create | Reconnection state machine |
| `mobile/src/hooks/useAudioLevel.ts` | create | VAD-based speaking detection |
| `mobile/src/hooks/useKeepAwake.ts` | create | Keep-awake wrapper |
| `mobile/src/hooks/useSessionTimer.ts` | create | MM:SS timer |
| `mobile/src/screens/ConversationScreen.tsx` | create | Main conversation UI |
| `mobile/src/components/VUMeter.tsx` | create | Voice level indicator |
| `mobile/src/components/ConnectionStatus.tsx` | create | Connection state display |
| `mobile/src/components/MuteButton.tsx` | create | Mute toggle |
| `mobile/src/components/SessionTimer.tsx` | create | Timer display |
| `mobile/src/components/EndCallButton.tsx` | create | End call with confirmation |
| `mobile/src/components/PipelineErrorBanner.tsx` | create | Error fallback UI |
| `mobile/src/services/api.ts` | create | HTTP client |
| `mobile/src/services/signalingService.ts` | create | WS message serialization |
| `mobile/src/types/signaling.ts` | create | TypeScript types |
| `mobile/src/types/session.ts` | create | Session types |
| `mobile/src/native/android/CallForegroundService.kt` | create | Android foreground service |
| `mobile/src/native/android/CallServiceModule.kt` | create | RN bridge for service |
| `mobile/ios/TalkGo/AudioSessionManager.swift` | create | iOS audio session management |
| `mobile/android/app/src/main/AndroidManifest.xml` | modify | +permissions, +service declaration |
| `mobile/ios/TalkGo/Info.plist` | modify | +UIBackgroundModes |

---

## Section 4: Testing Strategy

### Go (Strict TDD — tests first)

#### Domain tests (`internal/domain/room/room_test.go`)

| Test | Scenario |
|------|----------|
| `TestGenerateShortCode_Length` | Output is exactly 6 chars |
| `TestGenerateShortCode_Alphabet` | All chars are in allowed set |
| `TestGenerateShortCode_Uniqueness` | 1000 calls produce no duplicates (probabilistic) |
| `TestJoin_UpdatesLastActivity` | Join sets LastActivity > CreatedAt |
| `TestLeave_UpdatesLastActivity` | Leave updates LastActivity |
| `TestTouchActivity` | TouchActivity sets to time.Now() |

#### Repository tests (`internal/app/roomsvc/repository_test.go`)

| Test | Scenario |
|------|----------|
| `TestFindByShortCode_Hit` | Save room with ShortCode, find by code → match |
| `TestFindByShortCode_Miss` | FindByShortCode("XXXXXX") → ErrRoomNotFound |
| `TestFindByShortCode_CaseInsensitive` | Save with "ABC123", find with "abc123" → match |
| `TestUpdateLastActivity_Success` | Save room, UpdateLastActivity, verify time changed |
| `TestUpdateLastActivity_NotFound` | UpdateLastActivity("nonexistent") → ErrRoomNotFound |
| `TestListExpired_MixedRooms` | 2 expired + 1 active → returns only 2 |
| `TestListExpired_NoneExpired` | All rooms active → returns empty slice |

#### Service tests (`internal/app/roomsvc/service_test.go`)

| Test | Scenario | Setup |
|------|----------|-------|
| `TestOnDisconnect_StartsGracePeriod` | Disconnect one peer → room deleted after GracePeriod | GracePeriod=1ms, create room, 2 peers join, one disconnects, sleep 5ms, verify room gone |
| `TestOnDisconnect_ReconnectCancelsGrace` | Reconnect before grace → room survives | GracePeriod=50ms, disconnect, rejoin within 10ms, sleep 100ms, verify room exists |
| `TestOnDisconnect_BothDisconnect_NoGrace` | Both peers disconnect → immediate cleanup, no timer | Disconnect both, verify no graceTimers entry |
| `TestOnDisconnect_SessionNotFound` | Unknown sessionID → ErrSessionNotFound | Call OnDisconnect("bogus") |
| `TestExpirationSweep_DeletesExpiredRooms` | Stale room swept away | RoomTTL=1ms, SweepInterval=1ms, create room, wait 10ms, verify gone |
| `TestExpirationSweep_ActiveRoomNotExpired` | Recent activity → room survives | RoomTTL=1h, create room, run sweep, verify room exists |
| `TestCreateRoom_GeneratesShortCode` | CreateRoom returns non-empty ShortCode | Call CreateRoom, check result.ShortCode != "" |
| `TestCreateRoom_ShortCodeCollision` | Repo always finds collision → retries until success | Mock FindByShortCode to return collision N-1 times, then ErrRoomNotFound |
| `TestCreateRoom_ShortCodeExhausted` | 5 collisions → ErrShortCodeExhausted | Mock FindByShortCode to always return a room |
| `TestFindByShortCode_Active` | Find active room by code | Create room, FindByShortCode → success |
| `TestFindByShortCode_Expired` | Find expired room → ErrRoomClosed | Create room, close it, FindByShortCode → error |
| `TestJoinRoom_CancelsGraceTimer` | Join cancels pending grace timer | Start grace (OnDisconnect), then JoinRoom, verify timer cancelled |

**Test helpers needed**:
- `testServiceConfig()` → returns `ServiceConfig{GracePeriod: 1*time.Millisecond, RoomTTL: 1*time.Millisecond, SweepInterval: 1*time.Millisecond, MaxShortCodeRetries: 5}`
- All existing service tests must be updated to pass `ServiceConfig` as first argument to `NewService`.

#### Hub tests (if not already covered)

| Test | Scenario |
|------|----------|
| `TestUnregister_NotifiesPeerLeft` | Client disconnects → other client in same room receives peer-left |
| `TestUnregister_CallsOnDisconnect` | Client with sessionID disconnects → handler.OnDisconnect called |
| `TestUnregister_NoSessionID_NoOnDisconnect` | Client without sessionID → OnDisconnect NOT called |

#### HTTP handler tests (`internal/adapters/http/server_test.go`)

| Test | Scenario |
|------|----------|
| `TestCreateRoom_IncludesShortCode` | POST /rooms → response has short_code field |
| `TestFindByShortCode_200` | GET /rooms/code/ABC123 → 200 with room_id |
| `TestFindByShortCode_404` | GET /rooms/code/XXXXXX → 404 |
| `TestFindByShortCode_410` | GET /rooms/code/{expired} → 410 "Esta sala expiró" |
| `TestJoinRoom_Full_409` | Join when room has 2 → 409 "Esta sala ya tiene 2 participantes" |

### React Native (Jest + RNTL)

#### Store tests

| Test | Scenario |
|------|----------|
| `sessionStore.connect` | Sets all fields correctly, connectionState = 'connected' |
| `sessionStore.disconnect` | Resets to idle state |
| `sessionStore.tick` | Increments elapsedSeconds by 1 |
| `sessionStore.incrementErrors` | consecutiveErrors goes 0→1→2→3 |
| `sessionStore.resetErrors` | consecutiveErrors back to 0 |

#### Hook tests

| Test | Scenario |
|------|----------|
| `useReconnection.trigger` | State goes to 'reconnecting', attempt=1 |
| `useReconnection.maxAttempts` | After 3 failures, state = 'failed' |
| `useReconnection.cancel` | Voluntary leave prevents reconnection |
| `useReconnection.backoff` | Delays are 1000, 2000, 4000 ms |
| `useSignaling.onMessage` | Dispatches correct callback for each message type |
| `useSignaling.onClose` | isConnected = false |

#### Component tests (RNTL)

| Test | Scenario |
|------|----------|
| `ConversationScreen` — connected state | Shows VU meters, timer, mute button |
| `ConversationScreen` — reconnecting state | Shows "Reconectando..." indicator |
| `ConversationScreen` — failed state | Shows error UI with retry option |
| `EndCallButton` — confirmation dialog | Press → dialog appears; confirm → disconnect called |
| `PipelineErrorBanner` — visible on error | pipelineError set → banner shows |
| `PipelineErrorBanner` — hidden on recovery | resetErrors → banner hidden |
| `SessionTimer` — format | 65 seconds → "01:05" |

---

## Section 5: Flow Sequences

### 5.1 Full Connection Flow

```
Mobile App                    Go Server (HTTP)         Hub (WS)              Service
    |                              |                      |                      |
    |--- POST /rooms ------------->|                      |                      |
    |    {source_lang, target_lang}|                      |                      |
    |                              |--- CreateRoom ------>|                      |
    |                              |                      |--- GenerateShortCode |
    |                              |                      |--- repo.Save ------->|
    |<-- 201 {room_id, short_code}-|                      |                      |
    |                              |                      |                      |
    |  (User shares short_code verbally to peer)          |                      |
    |                              |                      |                      |
    |--- GET /ws/{roomID} -------->|                      |                      |
    |    (WS upgrade)              |--- ServeWS --------->|                      |
    |<========= WS OPEN ==========>|                      |                      |
    |                              |                      |                      |
    |--- {type:"join", user_id,    |                      |                      |
    |     room_id, lang} --------->|--- dispatch -------->|--- HandleSignaling ->|
    |                              |                      |   JoinRoom()         |
    |                              |                      |   room.Join()        |
    |                              |                      |   peer.CreateSession |
    |<-- {type:"joined",           |                      |   repo.Save          |
    |     session_id} -------------|<---------------------|<---------------------|
    |                              |                      |                      |
    |--- {type:"offer",            |                      |                      |
    |     session_id, sdp} ------->|--- dispatch -------->|--- HandleSignaling ->|
    |                              |                      |   peer.HandleOffer   |
    |                              |                      |   peer.CreateAnswer  |
    |<-- {type:"answer",           |                      |                      |
    |     session_id, sdp} --------|<---------------------|<---------------------|
    |                              |                      |                      |
    |--- {type:"ice-candidate",    |                      |                      |
    |     candidate} ------------->|--- dispatch -------->|--- AddICECandidate ->|
    |<-- (empty ACK) --------------|                      |                      |
    |                              |                      |                      |
    |  <=== WebRTC media flowing ===>                     |                      |
    |  (Opus audio in both dirs)   |                      |                      |
```

### 5.2 Abrupt Disconnect + Grace Period + Reconnection

```
Peer A (connected)       Hub                    Service                  Peer B (connected)
    |                      |                      |                         |
    X WS drops             |                      |                         |
    | (readPump exits)     |                      |                         |
    |                      |--- unregister(A) --->|                         |
    |                      |                      |                         |
    |                      |--- peer-left ------->|------------------------>|
    |                      |   {type:"peer-left", |  Peer B sees            |
    |                      |    session_id: A}    |  "Esperando reconexión" |
    |                      |                      |                         |
    |                      |--- OnDisconnect(A)-->|                         |
    |                      |                      |--- start grace timer    |
    |                      |                      |    (30s)                |
    |                      |                      |                         |
    | (network recovers)   |                      |                         |
    |                      |                      |                         |
    |--- WS reconnect ---->|                      |                         |
    |--- {type:"join"} --->|--- dispatch -------->|                         |
    |                      |                      |--- cancel grace timer!  |
    |                      |                      |--- JoinRoom (re-join)   |
    |<-- {type:"joined"} --|<---------------------|                         |
    |                      |                      |                         |
    |--- {type:"offer",    |                      |                         |
    |     iceRestart:true}->|--- HandleOffer ----->|                         |
    |<-- {type:"answer"} --|<---------------------|                         |
    |                      |                      |                         |
    |  <=== Media flowing again ===>              |                         |
    |                      |                      |--- notify Peer B:       |
    |                      |                      |    peer-joined         ->|
    |                      |                      |                         |
```

### 5.3 Room Expiration (Sweep)

```
                          Service (sweep goroutine)       Repo
    Tick (every 60s)          |                             |
         |                    |                             |
         |--- ListExpired --->|--- ListExpired(cutoff) ---->|
         |                    |                             |
         |                    |<-- [room-X, room-Y] --------|
         |                    |                             |
         |                    |--- for each expired room:   |
         |                    |    notifyRoomPeers(         |
         |                    |      "room-closed",         |
         |                    |      {reason: "expired"})   |
         |                    |    DeleteRoom(roomID)       |
         |                    |                             |
         |                    |                             |

Client tries to join expired room:

Mobile App                    HTTP Server               Service
    |                              |                      |
    |--- GET /rooms/code/ABC123 ->|                      |
    |                              |--- FindByShortCode ->|
    |                              |                      |--- repo.FindByShortCode
    |                              |                      |<-- room (Active=false)
    |                              |<-- ErrRoomClosed ----|
    |<-- 410 {"error":            |                      |
    |    "Esta sala expiró.       |                      |
    |     Creá una nueva."} ------|                      |
```

### 5.4 Voluntary Leave (No Grace Period)

```
Peer A                     Hub                    Service                  Peer B
    |                        |                      |                        |
    |--- {type:"leave",      |                      |                        |
    |     session_id} ------>|--- dispatch -------->|                        |
    |                        |                      |--- HandleSignaling     |
    |                        |                      |    case "leave":       |
    |                        |                      |    LeaveRoom()         |
    |                        |                      |    (NO grace period)   |
    |                        |                      |    peer.CloseSession   |
    |                        |                      |    room.Leave          |
    |                        |                      |    cancel pipeline     |
    |                        |                      |                        |
    |  (WS stays open)       |                      |--- notifyRoomPeers:   |
    |                        |                      |    "peer-left" ------->|
    |                        |                      |                        |
    |--- WS close ---------->|                      |                        |
    |                        |--- unregister ------>|                        |
    |                        |                      |--- OnDisconnect       |
    |                        |                      |    (session not found  |
    |                        |                      |     — already cleaned  |
    |                        |                      |     up by LeaveRoom)   |
```

---

## Key Architectural Decisions Summary

| # | Decision | Rationale |
|---|----------|-----------|
| AD-01 | `OnDisconnect` on `SignalingHandler`, not synthetic leave | Separate semantics: disconnect = involuntary (grace period), leave = voluntary (immediate cleanup). Different flows. |
| AD-02 | Grace timer in service, not in hub | Hub is an adapter — business logic (grace period) belongs in the app layer. Hub only notifies. |
| AD-03 | `OnDisconnect` called OUTSIDE hub mutex | Prevents deadlock: OnDisconnect → notifyRoomPeers → NotifySession → hub.mu.RLock would deadlock if called inside hub.mu.Lock. |
| AD-04 | `OnDisconnect` does NOT remove session from maps | Enables reconnection: same userID can re-join and cancel the grace timer. Session cleanup only happens when grace expires or on voluntary leave. |
| AD-05 | Sweep-based expiration, not per-room timers | Single goroutine vs N goroutines. Simpler, predictable resource usage. O(rooms) every 60s is negligible. |
| AD-06 | `GenerateShortCode` is a domain function, collision check in service | Domain stays pure (no persistence awareness). Service owns the retry loop. |
| AD-07 | `CreateRoom` returns `CreateRoomResult` struct | Avoids extra round-trip. Breaking change to interface, but Sprint 3 is the right time. |
| AD-08 | `ListExpired` as driven port method | Future-proofs for SQL WHERE clause. Avoids loading all rooms into memory for filtering. |
| AD-09 | VU meters use boolean (not float) in Zustand | Reduces re-renders from continuous (10Hz float updates) to event-based (speaking start/stop transitions). |
| AD-10 | Short codes case-insensitive, normalized to uppercase | Users may type lowercase on mobile. Alphabet is uppercase-only, so normalize at lookup. |
| AD-11 | `Hub.Run` accepts `context.Context` | Enables graceful shutdown. Breaking change but necessary for production readiness. |
| AD-12 | Reconnection uses same `JoinRoom` path | No special "reconnect" message type. Re-join + ICE restart achieves the same result. Simpler protocol. |
