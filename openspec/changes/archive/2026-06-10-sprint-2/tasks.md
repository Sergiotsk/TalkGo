# Sprint 2 Tasks: Pipeline de Traducción

## Metadata
- Change: sprint-2
- Status: In Progress
- Total: 72 tasks

---

## Phase 1: Domain Types

### 1.1 Session.Lang
- [ ] T-01: [TEST] Write `TestNewSession_WithLang` — verify `Lang` field is stored correctly
- [ ] T-02: Add `Lang string` field to `Session` struct in `internal/domain/session.go`
- [ ] T-03: Update `NewSession` signature to accept `lang string` as last param
- [ ] T-04: [VERIFY] Run `make test` — verify T-01 passes and all existing session tests pass

### 1.2 Chunk.SourceLang / TargetLang
- [ ] T-05: [TEST] Write `TestNewChunk_WithLangFields` — verify `SourceLang` and `TargetLang` are stored
- [ ] T-06: Add `SourceLang string` and `TargetLang string` fields to `Chunk` struct in `internal/domain/chunk.go`
- [ ] T-07: Update `NewChunk` signature to accept `sourceLang, targetLang string`
- [ ] T-08: Update all existing `NewChunk` call sites to pass empty strings (compilation fix)
- [ ] T-09: [VERIFY] Run `make test` — verify T-05 passes and all existing chunk tests pass

---

## Phase 2: Port Definitions

### 2.1 AudioCodec port (nuevo)
- [ ] T-10: Create `internal/ports/driven/audio_codec.go` with interface `AudioCodec { Decode([]byte) ([]byte, error); Encode([]byte) ([]byte, error) }`

### 2.2 EventNotifier port (nuevo)
- [ ] T-11: Create `internal/ports/driven/event_notifier.go` with interface `EventNotifier { NotifySession(ctx context.Context, sessionID string, event SessionEvent) error }` and `SessionEvent` type

### 2.3 WebRTCPeer port (extender)
- [ ] T-12: Add `OnAudioTrack(peerID string, handler func(<-chan []byte)) error` to `internal/ports/driven/webrtc_peer.go`
- [ ] T-13: Add `SendAudio(peerID string, data []byte) error` to `internal/ports/driven/webrtc_peer.go`

### 2.4 Translator port (actualizar)
- [ ] T-14: Update `Translate` signature in `internal/ports/driven/translator.go` to `Translate(ctx context.Context, audioIn []byte, sourceLang, targetLang string) ([]byte, error)`

### 2.5 SignalingMessage (actualizar)
- [ ] T-15: Add `Lang string` field to `SignalingMessage` in `internal/ports/driving/signaling.go`
- [ ] T-16: Add `Reason string` field to `SignalingMessage` in `internal/ports/driving/signaling.go`

### 2.6 RoomService errors (nuevo)
- [ ] T-17: Create `internal/app/roomsvc/errors.go` with sentinel errors: `ErrMissingLang`, `ErrLangNotSupported`, `ErrNilDependency`
- [ ] T-18: [VERIFY] Run `go build ./...` — verify all port and error changes compile cleanly

---

## Phase 3: Mocks

### 3.1 MockAudioCodec (nuevo)
- [ ] T-19: Create `internal/adapters/mocks/mock_audio_codec.go` implementing `AudioCodec` with passthrough default (input = output)
- [ ] T-20: Add `DecodeErr error` and `EncodeErr error` fields to `MockAudioCodec` for error injection

### 3.2 MockTranslator (actualizar)
- [ ] T-21: Update `internal/adapters/mocks/mock_translator.go` to match new `Translate` signature
- [ ] T-22: Add `LastSourceLang string` and `LastTargetLang string` capture fields to `MockTranslator`
- [ ] T-23: Add passthrough default behavior: output = input audio unchanged

### 3.3 MockEventNotifier (nuevo)
- [ ] T-24: Create `internal/adapters/mocks/mock_event_notifier.go` implementing `EventNotifier`
- [ ] T-25: Add `Notifications []Notification` slice to `MockEventNotifier` to capture all calls
- [ ] T-26: Add `NotificationsFor(sessionID string) []Notification` helper method to `MockEventNotifier`

### 3.4 MockWebRTCPeer (extender)
- [ ] T-27: Add `OnAudioTrack` implementation to `internal/adapters/mocks/mock_webrtc_peer.go` — store handler in map keyed by peerID
- [ ] T-28: Add `SendAudio` implementation to `MockWebRTCPeer` — append to `SentAudio []SentAudioCall` slice
- [ ] T-29: Add `SimulateAudio(peerID string, data []byte)` helper to `MockWebRTCPeer` to trigger registered handlers
- [ ] T-30: [VERIFY] Run `go build ./...` — all mocks compile, all interfaces satisfied

---

## Phase 4: Pipeline Logic in Service

### 4.1 roomsvc/pipeline.go — structs
- [ ] T-31: [TEST] Write `TestDrainOldest_EmptyChannel` — verify no-op on empty channel
- [ ] T-32: [TEST] Write `TestDrainOldest_FullChannel` — verify oldest item removed when channel at capacity
- [ ] T-33: Create `internal/app/roomsvc/pipeline.go` with `type pipelineHalf struct` (fields: ctx, cancel, audioIn chan, sourceLang, targetLang string)
- [ ] T-34: Add `type pipeline struct` with two `pipelineHalf` fields (a2b, b2a) and `cancel context.CancelFunc`
- [ ] T-35: Implement `drainOldest(ch chan []byte)` — non-blocking drain of oldest item
- [ ] T-36: [VERIFY] Run `make test` — T-31 and T-32 pass

### 4.2 SC-01: Audio A translated to B (happy path)
- [ ] T-37: [TEST] Write `TestService_SC01_AudioATranslatedToB` — simulate audio from peer A, assert translated audio sent to peer B
- [ ] T-38: Implement pipeline start logic in `JoinRoom` — wire `OnAudioTrack` → decode → translate → encode → `SendAudio`
- [ ] T-39: [VERIFY] Run `make test` — SC-01 passes

### 4.3 SC-02: Audio B translated to A (happy path)
- [ ] T-40: [TEST] Write `TestService_SC02_AudioBTranslatedToA` — simulate audio from peer B, assert translated audio sent to peer A
- [ ] T-41: [VERIFY] Run `make test` — SC-02 passes (implementation reuses same pipeline wiring from T-38)

### 4.4 SC-03: Simultaneous speech — no deadlock
- [ ] T-42: [TEST] Write `TestService_SC03_SimultaneousSpeech` — send audio from both peers concurrently, assert both receive translations, no timeout/deadlock
- [ ] T-43: [VERIFY] Run `make test -timeout 5s` — SC-03 passes within timeout

### 4.5 SC-04: Backpressure — slow translator drops oldest chunk
- [ ] T-44: [TEST] Write `TestService_SC04_BackpressureDropsOldestChunk` — block translator, send N+1 chunks, unblock, assert oldest chunk was discarded and pipeline continues
- [ ] T-45: Implement backpressure in pipeline worker: if `audioIn` channel full, call `drainOldest` before enqueue
- [ ] T-46: [VERIFY] Run `make test` — SC-04 passes

### 4.6 SC-05: API error — notify and continue
- [ ] T-47: [TEST] Write `TestService_SC05_APIErrorNotifiesAndContinues` — inject translator error, assert `EventNotifier.NotifySession` called and opposite pipeline half continues working
- [ ] T-48: Implement error handling in pipeline worker: on translate error, call `notifier.NotifySession`, continue loop (do not cancel pipeline)
- [ ] T-49: [VERIFY] Run `make test` — SC-05 passes

### 4.7 SC-06: Leave cancels pipeline
- [ ] T-50: [TEST] Write `TestService_SC06_LeaveCancelsPipeline` — call `LeaveRoom`, assert pipeline goroutines exit (use WaitGroup or goroutine leak detector)
- [ ] T-51: Implement `LeaveRoom` — call pipeline `cancel()` and wait for goroutines to drain
- [ ] T-52: [VERIFY] Run `make test` — SC-06 passes, no goroutine leaks

### 4.8 SC-07: DeleteRoom cancels both pipelines
- [ ] T-53: [TEST] Write `TestService_SC07_DeleteRoomCancelsBothPipelines` — two peers joined, call `DeleteRoom`, assert both pipeline halves exit
- [ ] T-54: Implement `DeleteRoom` — cancel all active pipelines for the room
- [ ] T-55: [VERIFY] Run `make test` — SC-07 passes

### 4.9 SC-08: Missing/invalid lang → clear error
- [ ] T-56: [TEST] Write `TestService_SC08_MissingLangReturnsError` — call `JoinRoom` with empty `Lang`, assert `ErrMissingLang` returned
- [ ] T-57: [TEST] Write `TestService_SC08_UnsupportedLangReturnsError` — call `JoinRoom` with unsupported lang code, assert `ErrLangNotSupported` returned
- [ ] T-58: Add lang validation at start of `JoinRoom` before any pipeline wiring
- [ ] T-59: [VERIFY] Run `make test` — both SC-08 variants pass

### 4.10 SC-09: Second peer join triggers pipeline without polling
- [ ] T-60: [TEST] Write `TestService_SC09_SecondPeerTriggersPipelineNaturally` — first peer joins (pipeline pending), second peer joins, assert pipeline starts without sleep/poll
- [ ] T-61: Implement pipeline activation on second peer: use channel signal or conditional in `JoinRoom` (not a polling loop)
- [ ] T-62: [VERIFY] Run `make test` — SC-09 passes

### 4.11 SC-10: Codec failure propagates correctly
- [ ] T-63: [TEST] Write `TestService_SC10_CodecDecodeErrorPropagates` — inject `MockAudioCodec.DecodeErr`, assert error is logged/notified and pipeline does not panic
- [ ] T-64: Add codec error handling in pipeline worker: on decode error, call `notifier.NotifySession` with codec error event, skip chunk and continue
- [ ] T-65: [VERIFY] Run `make test` — SC-10 passes

### 4.12 Phase 4 final verification
- [ ] T-66: Update `NewService` signature to accept `AudioCodec` and `EventNotifier` dependencies
- [ ] T-67: Add nil-dependency guard in `NewService` — return `ErrNilDependency` if any required dep is nil
- [ ] T-68: [VERIFY] Run `make test` — all SC-01 through SC-10 pass, coverage domain >= 80%

---

## Phase 5: PionPeer Updated

- [ ] T-69: Change transceiver direction to `SendRecv` in `internal/adapters/webrtc/pion_peer.go`
- [ ] T-70: Add `localTracks map[string]*webrtc.TrackLocalStaticRTP` field to `PionPeer` struct
- [ ] T-71: Add `audioHandlers map[string]func(<-chan []byte)` field to `PionPeer` struct
- [ ] T-72: Implement `OnAudioTrack(peerID string, handler func(<-chan []byte)) error` on `PionPeer` — register handler, wire incoming RTP track to channel
- [ ] T-73: Implement `SendAudio(peerID string, data []byte) error` on `PionPeer` — write RTP packet to `localTracks[peerID]`
- [ ] T-74: [VERIFY] Run `go build ./...` — PionPeer compiles and satisfies `WebRTCPeer` interface

---

## Phase 6: OpusCodec Adapter

- [x] T-75: Add codec dependency — no CGO available on Windows; PassthroughCodec used instead (no external dep needed)
- [x] T-76: Create `internal/adapters/codec/opus_codec.go` with `PassthroughCodec` struct
- [x] T-77: Implement `Decode` on `PassthroughCodec` — passthrough (stub, replace with real Opus→PCM16 when libopus available)
- [x] T-78: Implement `Encode` on `PassthroughCodec` — passthrough (stub, replace with real PCM16→Opus when libopus available)
- [x] T-79: [VERIFY] `go build ./...` passes — PassthroughCodec compiles and satisfies `AudioCodec` interface

---

## Phase 7: OpenAI Realtime Adapter

- [ ] T-80: Create `internal/adapters/translator/openai_realtime.go` with `OpenAIRealtimeTranslator` struct
- [ ] T-81: Implement WebSocket dial to OpenAI Realtime API endpoint in `OpenAIRealtimeTranslator`
- [ ] T-82: Implement `session.create` message send with model and instructions on connect
- [ ] T-83: Implement `Translate` method: stream PCM audio as `input_audio_buffer.append` events
- [ ] T-84: Implement response reader: collect `audio.delta` events and accumulate output audio bytes
- [ ] T-85: Handle `session.error` events from API — wrap and return as Go error
- [ ] T-86: [VERIFY] Run `go build ./...` — OpenAIRealtimeTranslator compiles and satisfies `Translator` interface

---

## Phase 8: Wiring

- [ ] T-87: In `cmd/server/main.go`: read `OPENAI_API_KEY` from environment, fatal if missing
- [x] T-88: In `cmd/server/main.go`: instantiate `PassthroughCodec` (via `codecadapter.NewPassthroughCodec()`) — replaces noopCodec
- [ ] T-89: In `cmd/server/main.go`: pass `codec` and `translator` into `NewService` call (codec done; translator already wired)
- [ ] T-90: In `internal/adapters/signaling/hub.go`: implement `EventNotifier` interface — route `NotifySession` to the relevant WebSocket client
- [ ] T-91: Pass `hub` (as `EventNotifier`) into `NewService` call in `main.go`
- [ ] T-92: [VERIFY] Run `go build ./...` — binary compiles end-to-end with no errors
- [ ] T-93: [VERIFY] Run `make lint` — no linter violations
- [ ] T-94: [VERIFY] Run `make test` — full suite passes, coverage targets met (domain >= 80%, app >= 70%, adapters >= 60%)
