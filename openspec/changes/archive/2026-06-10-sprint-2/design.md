# Sprint 2 Design: Pipeline de Traducción

## Metadata
- Change: sprint-2
- Status: Draft
- Date: 2026-06-10

## 1. Package Structure

### New files

```
internal/
  domain/
    session/
      session.go              # MODIFIED — add Lang field, update NewSession signature
    translation/
      translation.go          # MODIFIED — add SourceLang, TargetLang fields to Chunk
  ports/
    driven/
      audio_codec.go          # NEW — AudioCodec interface
      webrtc_peer.go          # MODIFIED — add OnAudioTrack, SendAudio to WebRTCPeer
      event_notifier.go       # NEW — EventNotifier interface
      translator.go           # MODIFIED — adjust TranslateStream signature per spec
      mocks/
        mock_audio_codec.go   # NEW
        mock_translator.go    # NEW
        mock_event_notifier.go # NEW
        mock_webrtc_peer.go   # MODIFIED — add OnAudioTrack, SendAudio stubs
    driving/
      signaling.go            # MODIFIED — add Lang, Reason fields to SignalingMessage
  app/
    roomsvc/
      service.go              # MODIFIED — new constructor params, startPipeline trigger
      pipeline.go             # NEW — pipeline struct, startPipeline, runHalf, drainOldest
      pipeline_test.go        # NEW — SC-01 through SC-10
      errors.go               # NEW — ErrMissingLang, ErrLangNotSupported, ErrNilDependency
  adapters/
    codec/
      opus_codec.go           # NEW — OpusCodec adapter implementing AudioCodec
      opus_codec_test.go      # NEW
    translator/
      openai_realtime.go      # NEW — OpenAIRealtimeTranslator implementing Translator
      openai_realtime_test.go # NEW
    webrtc/
      pion_peer.go            # MODIFIED — OnAudioTrack, SendAudio, sendrecv transceiver
      pion_peer_test.go       # MODIFIED — tests for new methods
    signaling/
      hub.go                  # MODIFIED — implement EventNotifier by exposing SendToSession
```

## 2. Data Structures

### Domain changes

```go
// internal/domain/session/session.go — updated struct
type Session struct {
    ID       string
    RoomID   string
    UserID   string
    Lang     string    // NEW: ISO 639-1 code of this participant's language
    JoinedAt time.Time
    State    State
}

// NewSession creates and initializes a new Session in StateConnecting.
// lang is the ISO 639-1 language code the participant speaks.
func NewSession(id, roomID, userID, lang string) *Session {
    return &Session{
        ID:       id,
        RoomID:   roomID,
        UserID:   userID,
        Lang:     lang,
        JoinedAt: time.Now(),
        State:    StateConnecting,
    }
}
```

```go
// internal/domain/translation/translation.go — updated struct
type Chunk struct {
    ID         string
    SessionID  string
    Payload    []byte
    SourceLang string // NEW: ISO 639-1 source language
    TargetLang string // NEW: ISO 639-1 target language
    Timestamp  time.Time
}

// NewChunk creates a new translation chunk with language direction.
func NewChunk(id, sessionID string, payload []byte, sourceLang, targetLang string) *Chunk {
    return &Chunk{
        ID:         id,
        SessionID:  sessionID,
        Payload:    payload,
        SourceLang: sourceLang,
        TargetLang: targetLang,
        Timestamp:  time.Now(),
    }
}
```

### Driving port changes

```go
// internal/ports/driving/signaling.go — updated struct
type SignalingMessage struct {
    Type      string `json:"type"`
    RoomID    string `json:"room_id,omitempty"`
    UserID    string `json:"user_id,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    SDP       string `json:"sdp,omitempty"`
    Candidate string `json:"candidate,omitempty"`
    Message   string `json:"message,omitempty"`
    Lang      string `json:"lang,omitempty"`   // NEW: participant language on join
    Reason    string `json:"reason,omitempty"` // NEW: error reason for pipeline errors
}
```

### Application layer — pipeline struct

```go
// internal/app/roomsvc/pipeline.go

// pipeline represents an active bidirectional translation pipeline for a room.
// It owns two halves (A->B and B->A), each running in its own goroutine tree.
type pipeline struct {
    ctx    context.Context
    cancel context.CancelFunc
    roomID string
    sessA  *session.Session
    sessB  *session.Session
    wg     sync.WaitGroup
}

// pipelineHalf represents one direction of the translation pipeline.
type pipelineHalf struct {
    sourceSessID string
    targetSessID string
    sourceLang   string
    targetLang   string
}
```

### Application layer — Service updated fields

```go
// internal/app/roomsvc/service.go — updated struct
type Service struct {
    repo       driven.RoomRepository
    peer       driven.WebRTCPeer
    translator driven.Translator
    codec      driven.AudioCodec
    notifier   driven.EventNotifier
    sessions   map[string]*session.Session
    lookup     map[string]string       // "roomID:userID" -> sessionID
    pipelines  map[string]*pipeline    // roomID -> active pipeline
    mu         sync.RWMutex
}
```

### Adapter structs

```go
// internal/adapters/codec/opus_codec.go
type OpusCodec struct {
    sampleRate int // default 24000
    channels   int // default 1 (mono)
}

func NewOpusCodec(sampleRate, channels int) *OpusCodec {
    return &OpusCodec{sampleRate: sampleRate, channels: channels}
}
```

```go
// internal/adapters/translator/openai_realtime.go
type OpenAIRealtimeConfig struct {
    APIKey  string
    Model   string        // default "gpt-4o-realtime-preview"
    BaseURL string        // default "wss://api.openai.com/v1/realtime"
    Timeout time.Duration // per-connection timeout, default 30s
}

type OpenAIRealtimeTranslator struct {
    cfg OpenAIRealtimeConfig
}

func NewOpenAIRealtimeTranslator(cfg OpenAIRealtimeConfig) *OpenAIRealtimeTranslator {
    return &OpenAIRealtimeTranslator{cfg: cfg}
}
```

### PionPeer updated struct

```go
// internal/adapters/webrtc/pion_peer.go — updated struct
type PionPeer struct {
    cfg           Config
    api           *pionwebrtc.API
    peers         map[string]*pionwebrtc.PeerConnection
    localTracks   map[string]*pionwebrtc.TrackLocalStaticRTP // sessionID -> outbound track
    audioHandlers map[string]func(<-chan []byte)              // sessionID -> registered handler
    mu            sync.RWMutex
}
```

### Application layer errors

```go
// internal/app/roomsvc/errors.go
package roomsvc

import "errors"

// ErrMissingLang is returned when a join request does not include a language code.
var ErrMissingLang = errors.New("lang is required")

// ErrLangNotSupported is returned when the participant's language does not match
// either the room's SourceLang or TargetLang.
var ErrLangNotSupported = errors.New("lang not supported in room")

// ErrNilDependency is returned when NewService receives a nil driven port.
var ErrNilDependency = errors.New("nil dependency")
```

## 3. Interface Definitions

```go
// internal/ports/driven/audio_codec.go
package driven

import "context"

// AudioCodec defines the driven port for encoding and decoding audio frames.
// The primary implementation converts between Opus and PCM16 at 24 kHz mono.
type AudioCodec interface {
    // Decode converts Opus frames from opusIn into PCM16 frames on the returned channel.
    // The output channel is closed when opusIn is closed or ctx is cancelled.
    Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error)

    // Encode converts PCM16 frames from pcmIn into Opus frames on the returned channel.
    // The output channel is closed when pcmIn is closed or ctx is cancelled.
    Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error)
}
```

```go
// internal/ports/driven/webrtc_peer.go — extended interface (Sprint 2 additions)

// OnAudioTrack registers a handler that receives inbound audio from the peer's media track.
// The handler is called with a read-only channel of Opus frames. The channel is closed
// when the track ends or ctx is cancelled. Must be called before the offer/answer exchange.
OnAudioTrack(ctx context.Context, sessionID string, handler func(<-chan []byte)) error

// SendAudio consumes Opus frames from audio and writes them to the peer's outbound track.
// Blocks until audio is closed or ctx is cancelled. Returns nil on clean shutdown.
SendAudio(ctx context.Context, sessionID string, audio <-chan []byte) error
```

```go
// internal/ports/driven/event_notifier.go
package driven

// EventNotifier defines the driven port for sending asynchronous messages to connected clients.
type EventNotifier interface {
    // NotifySession sends a message to the client associated with sessionID.
    // If the session is not connected, the message is silently dropped.
    NotifySession(sessionID string, msgType string, fields map[string]string)
}
```

```go
// internal/ports/driven/translator.go — updated signature
// TranslateStream receives PCM16 audio chunks and returns translated PCM16 audio chunks.
// Parameter order: (ctx, audioIn, sourceLang, targetLang) — normalized from Sprint 1 definition.
TranslateStream(ctx context.Context, audioIn <-chan []byte, sourceLang, targetLang string) (<-chan []byte, error)
```

## 4. Sequence Diagrams

### 4.1 Second participant joins → pipeline startup

```
Client_B          Service               Room         PionPeer     Translator    AudioCodec
  |                  |                    |              |              |             |
  |--join(B,lang)--->|                    |              |              |             |
  |                  |--FindByID(roomID)->|              |              |             |
  |                  |--room.Join(B)----->|              |              |             |
  |                  |--NewSession(B,lang)|              |              |             |
  |                  |--CreateSession(B)->|------------->|              |             |
  |                  |--repo.Save-------->|              |              |             |
  |                  |                    |              |              |             |
  |                  |--[room.IsFull()==true]            |              |             |
  |                  |                    |              |              |             |
  |                  |==startPipeline(room, sessA, sessB)===========>  |             |
  |                  |  |--go runHalf(A->B)              |              |             |
  |                  |  |--go runHalf(B->A)              |              |             |
  |                  |                    |              |              |             |
  |<--joined(sessID)-|                    |              |              |             |
```

### 4.2 Audio flow A → B (steady state)

```
PionPeer(A)     runHalf(A->B)     AudioCodec      Translator     AudioCodec    PionPeer(B)
  |                  |                |                |               |              |
  |--OnAudioTrack--->|                |                |               |              |
  |  opusCh -------->|                |                |               |              |
  |                  |--Decode(opusCh)|                |               |              |
  |                  |  pcmInCh <-----|                |               |              |
  |                  |                |                |               |              |
  |                  | [drainOldest into buffered bpCh]|               |              |
  |                  |                |                |               |              |
  |                  |--TranslateStream(bpCh, es, en)->|               |              |
  |                  |  pcmOutCh <----|----------------|               |              |
  |                  |                |                |               |              |
  |                  |--Encode(pcmOutCh)-------------->|               |              |
  |                  |  opusOutCh <---|----------------|               |              |
  |                  |                |                |               |              |
  |                  |--SendAudio(sessB, opusOutCh)----|-------------->|              |
  |                  |                |                |               |--write RTP-->|
```

### 4.3 Participant leaves → pipeline cancellation

```
Client_A          Service           pipeline          PionPeer
  |                  |                  |                 |
  |--leave(sessA)--->|                  |                 |
  |                  |--sess.Disconnect |                 |
  |                  |                  |                 |
  |                  |--cancelPipeline->|                 |
  |                  |  cancel()        |--ctx.Done()---->goroutines exit
  |                  |  wg.Wait()       |                 |
  |                  |<--all stopped----|                 |
  |                  |                  |                 |
  |                  |--peer.CloseSession(sessA)--------->|
  |                  |--room.Leave(A)-->|                 |
  |                  |--notifier.NotifySession(sessB, "peer_left", ...)
```

## 5. Concurrency Model

### Goroutines per pipeline — exactly 6

| # | Owner | Purpose | Lifetime |
|---|-------|---------|----------|
| 1 | `runHalf(A->B)` | Orchestrator + backpressure feeder goroutine | Until `pipeline.ctx` cancelled |
| 2 | `AudioCodec.Decode` (A) | Reads opusIn, writes PCM to pcmCh | Until opusIn closed or ctx cancelled |
| 3 | `AudioCodec.Encode` (A) | Reads PCM from translatedCh, writes Opus | Until translatedCh closed or ctx cancelled |
| 4 | `runHalf(B->A)` | Same as #1, opposite direction | Until `pipeline.ctx` cancelled |
| 5 | `AudioCodec.Decode` (B) | Same as #2 for B | Until closes/cancel |
| 6 | `AudioCodec.Encode` (B) | Same as #3 for B | Until closes/cancel |

`TranslateStream` and `SendAudio` run inline (blocking on channel reads), no extra goroutines.

### Cancellation chain

```
pipeline.ctx (context.WithCancel from context.Background())
    ├── runHalf(A->B)
    │     └── Decode, TranslateStream, Encode, SendAudio (same ctx)
    └── runHalf(B->A)
          └── Decode, TranslateStream, Encode, SendAudio (same ctx)
```

When `pipeline.cancel()` is called:
1. All `ctx.Done()` channels fire simultaneously
2. Each stage detects cancellation via `select` and closes its output channel
3. Downstream stages see closed inputs and exit
4. `runHalf` returns → `wg.Done()`
5. Caller waits on `pipeline.wg.Wait()` with 5-second timeout

### Unexpected goroutine death

If any goroutine panics or exits unexpectedly:
- Output channel is closed via `defer close(outCh)`
- Downstream consumer detects closed channel and exits
- Chain unwinds to `runHalf`, which logs the error and calls `pipeline.cancel()`
- `EventNotifier` sends error message to both participants

## 6. Backpressure Strategy

Buffer of size 1 + drain-oldest between `Decode` output and `Translator` input.

```go
// drainOldest drops the oldest item from ch if it is full, then sends v.
// This ensures the translator always receives the most recent audio frame
// without unbounded backlog accumulation.
func drainOldest(ch chan []byte, v []byte) {
    select {
    case ch <- v:
        return // sent without blocking
    default:
    }
    // channel is full — drain the oldest item
    select {
    case <-ch:
    default:
    }
    // send the new item
    select {
    case ch <- v:
    default:
    }
}
```

### Integration in runHalf

```go
func (s *Service) runHalf(ctx context.Context, p *pipeline, half pipelineHalf) {
    defer p.wg.Done()

    opusCh := make(chan []byte, 8)
    err := s.peer.OnAudioTrack(ctx, half.sourceSessID, func(trackCh <-chan []byte) {
        for frame := range trackCh {
            select {
            case opusCh <- frame:
            case <-ctx.Done():
                return
            }
        }
        close(opusCh)
    })
    if err != nil {
        s.notifier.NotifySession(half.sourceSessID, "error",
            map[string]string{"reason": fmt.Sprintf("audio track setup failed: %v", err)})
        return
    }

    pcmCh, err := s.codec.Decode(ctx, opusCh)
    if err != nil {
        s.notifier.NotifySession(half.sourceSessID, "error",
            map[string]string{"reason": fmt.Sprintf("audio decode failed: %v", err)})
        return
    }

    bpCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(bpCh)
        for {
            select {
            case frame, ok := <-pcmCh:
                if !ok {
                    return
                }
                drainOldest(bpCh, frame)
            case <-ctx.Done():
                return
            }
        }
    }()

    translatedCh, err := s.translator.TranslateStream(ctx, bpCh, half.sourceLang, half.targetLang)
    if err != nil {
        s.notifier.NotifySession(half.sourceSessID, "error",
            map[string]string{"reason": fmt.Sprintf("translation failed: %v", err)})
        return
    }

    opusOutCh, err := s.codec.Encode(ctx, translatedCh)
    if err != nil {
        s.notifier.NotifySession(half.targetSessID, "error",
            map[string]string{"reason": fmt.Sprintf("audio encode failed: %v", err)})
        return
    }

    if err := s.peer.SendAudio(ctx, half.targetSessID, opusOutCh); err != nil {
        s.notifier.NotifySession(half.targetSessID, "error",
            map[string]string{"reason": fmt.Sprintf("send audio failed: %v", err)})
    }
}
```

**Design decision**: Buffer size 1 achieves the "discard if >3s" requirement naturally — when the translator is slow, the next arriving frame triggers the drain without needing an explicit timer.

## 7. Error Strategy

| Stage | Error type | Action | Notify client? | Pipeline continues? |
|-------|-----------|--------|---------------|-------------------|
| `OnAudioTrack` | Setup error | Log + notify source | Yes (source) | No — `runHalf` returns |
| `Decode` | Initial error | Log + notify source | Yes (source) | No — `runHalf` returns |
| `Decode` | Frame-level (channel closes early) | Downstream cascade | Yes (source) | No — cascade closes half |
| `TranslateStream` | Initial error (connection failed) | Log + notify source | Yes (source) | No — `runHalf` returns |
| `TranslateStream` | Output closes unexpectedly | Encode sees closed input | Yes (source) | No — cascade closes half |
| `Encode` | Initial error | Log + notify target | Yes (target) | No — `runHalf` returns |
| `SendAudio` | Write error | Log + notify target | Yes (target) | No — `runHalf` returns |

**Key decisions:**
1. **Errors in one half do NOT affect the other half.** Each `runHalf` is independent.
2. **SC-05 compliance**: `TranslateStream` handles transient API errors internally (drops bad frames, notifies). The output channel closes only on ctx cancel or audioIn close.
3. **No automatic pipeline restart.** If a half fails, it stays down until participants rejoin.

## 8. Mock Strategy

### MockAudioCodec

```go
// internal/ports/driven/mocks/mock_audio_codec.go
type MockAudioCodec struct {
    DecodeFn func(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error)
    EncodeFn func(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error)

    DecodeCalled int
    EncodeCalled int
}

func (m *MockAudioCodec) Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error) {
    m.DecodeCalled++
    if m.DecodeFn != nil {
        return m.DecodeFn(ctx, opusIn)
    }
    // Default: passthrough
    return passthrough(ctx, opusIn), nil
}

func (m *MockAudioCodec) Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error) {
    m.EncodeCalled++
    if m.EncodeFn != nil {
        return m.EncodeFn(ctx, pcmIn)
    }
    return passthrough(ctx, pcmIn), nil
}
```

### MockTranslator

```go
// internal/ports/driven/mocks/mock_translator.go
type MockTranslator struct {
    TranslateStreamFn func(ctx context.Context, audioIn <-chan []byte, sourceLang, targetLang string) (<-chan []byte, error)

    TranslateStreamCalled int
    LastSourceLang        string
    LastTargetLang        string
}

func (m *MockTranslator) TranslateStream(ctx context.Context, audioIn <-chan []byte, sourceLang, targetLang string) (<-chan []byte, error) {
    m.TranslateStreamCalled++
    m.LastSourceLang = sourceLang
    m.LastTargetLang = targetLang
    if m.TranslateStreamFn != nil {
        return m.TranslateStreamFn(ctx, audioIn, sourceLang, targetLang)
    }
    return passthrough(ctx, audioIn), nil
}
```

### MockEventNotifier

```go
// internal/ports/driven/mocks/mock_event_notifier.go
type Notification struct {
    SessionID string
    MsgType   string
    Fields    map[string]string
}

type MockEventNotifier struct {
    mu            sync.Mutex
    Notifications []Notification
}

func (m *MockEventNotifier) NotifySession(sessionID string, msgType string, fields map[string]string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.Notifications = append(m.Notifications, Notification{sessionID, msgType, fields})
}

func (m *MockEventNotifier) NotificationsFor(sessionID string) []Notification { ... }
```

### Simulating slow translator (SC-04)

```go
translator := &mocks.MockTranslator{
    TranslateStreamFn: func(ctx context.Context, audioIn <-chan []byte, _, _ string) (<-chan []byte, error) {
        out := make(chan []byte, 1)
        go func() {
            defer close(out)
            for frame := range audioIn {
                select {
                case <-time.After(4 * time.Second): // simulates slow API
                    select { case out <- frame: case <-ctx.Done(): return }
                case <-ctx.Done():
                    return
                }
            }
        }()
        return out, nil
    },
}
```

### Simulating error then recovery (SC-05)

```go
callCount := int32(0)
translator := &mocks.MockTranslator{
    TranslateStreamFn: func(ctx context.Context, audioIn <-chan []byte, _, _ string) (<-chan []byte, error) {
        if atomic.AddInt32(&callCount, 1) == 1 {
            return nil, errors.New("openai: rate limit exceeded")
        }
        return passthrough(ctx, audioIn), nil
    },
}
```

## 9. Implementation Order

### Phase 1: Domain types
`internal/domain/session/session.go`, `internal/domain/translation/translation.go`
- Add `Lang` to `Session`, update `NewSession`
- Add `SourceLang`, `TargetLang` to `Chunk`, update `NewChunk`
- Tests first: `TestNewSession_WithLang`, `TestNewChunk_WithLanguages`

### Phase 2: Port definitions
`audio_codec.go`, `webrtc_peer.go` (ext), `event_notifier.go`, `translator.go` (sig update), `signaling.go`, `errors.go`
- Create/extend interfaces
- Compilation test: `go build ./...`

### Phase 3: Mocks
`mock_audio_codec.go`, `mock_translator.go`, `mock_event_notifier.go`, `mock_webrtc_peer.go` (ext)
- All mocks with passthrough defaults
- Verify: `var _ driven.AudioCodec = (*MockAudioCodec)(nil)`

### Phase 4: Pipeline logic in Service (core of the sprint)
`roomsvc/service.go`, `roomsvc/pipeline.go`, `roomsvc/pipeline_test.go`

TDD order:
1. `NewService` + `ErrNilDependency` test
2. `JoinRoom` with `lang` validation (SC-08)
3. `startPipeline` + SC-01/SC-02 happy path
4. SC-03 simultaneous audio
5. `drainOldest` + SC-04 backpressure
6. SC-05 error + notifier
7. SC-06/SC-07 cancellation + goroutine leak check
8. SC-09 ICE state
9. SC-10 codec error

### Phase 5: PionPeer updated
`internal/adapters/webrtc/pion_peer.go`
- Change transceiver to `SendRecv`
- Implement `OnAudioTrack` and `SendAudio`
- Loopback integration test

### Phase 6: OpusCodec adapter
`internal/adapters/codec/opus_codec.go`
- Decode: Opus → PCM16 24kHz mono
- Encode: PCM16 → Opus
- Round-trip test with known test vectors

### Phase 7: OpenAI Realtime adapter
`internal/adapters/translator/openai_realtime.go`
- WebSocket connection + session.create
- Stream PCM as input_audio_buffer.append
- Read response.audio.delta
- Mock WS server tests

### Phase 8: Wiring in main.go
`cmd/server/main.go`, `internal/adapters/signaling/hub.go`
- Instantiate OpusCodec + OpenAIRealtimeTranslator
- Hub implements EventNotifier (sessionClients map)
- Wire everything in NewService

---

## Key Design Decisions

1. **Backpressure via buffer-1 drain-oldest** — simpler and deterministic vs. timer-based eviction
2. **6 goroutines per pipeline** — minimal, no extra goroutines for inline blocking stages
3. **EventNotifier as driven port** — decouples Service from signaling infrastructure
4. **Translator signature normalized** to `(ctx, audioIn, sourceLang, targetLang)`
5. **No automatic pipeline restart** — translator handles transient errors internally per SC-05
6. **Pipeline cancellation via context + WaitGroup** with 5-second timeout
7. **JoinRoom validates Lang** against room's SourceLang/TargetLang before creating session
8. **SC-09 is free** — `OnAudioTrack` fires naturally after ICE completes, no polling needed
