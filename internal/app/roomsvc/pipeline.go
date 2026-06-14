package roomsvc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/domain/session"
)

// pipeline represents an active bidirectional translation pipeline for a room.
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
	dir          string          // "AtoB" or "BtoA"
	tracker      *LatencyTracker // per-half latency tracker
	totalChunks  atomic.Int64    // chunks processed successfully
	errorChunks  atomic.Int64    // chunks with errors
}

// drainOldest drops the oldest item from ch if it is full, then sends v.
// This ensures the translator always receives the most recent audio frame
// without unbounded backlog accumulation.
func drainOldest(ch chan []byte, v []byte) {
	select {
	case ch <- v:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}

// startPipeline launches a bidirectional translation pipeline for the given room.
// It is called in a goroutine once the room reaches full capacity.
func (s *Service) startPipeline(r *room.Room, sessA, sessB *session.Session) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &pipeline{
		ctx:    ctx,
		cancel: cancel,
		roomID: r.ID,
		sessA:  sessA,
		sessB:  sessB,
	}

	s.mu.Lock()
	s.pipelines[r.ID] = p
	s.mu.Unlock()

	halfAtoB := pipelineHalf{
		sourceSessID: sessA.ID,
		targetSessID: sessB.ID,
		sourceLang:   sessA.Lang,
		targetLang:   sessB.Lang,
		dir:          "AtoB",
		tracker:      NewLatencyTracker(),
	}
	halfBtoA := pipelineHalf{
		sourceSessID: sessB.ID,
		targetSessID: sessA.ID,
		sourceLang:   sessB.Lang,
		targetLang:   sessA.Lang,
		dir:          "BtoA",
		tracker:      NewLatencyTracker(),
	}

	slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
		slog.String("event", "session_start"),
		slog.String("session_id", sessA.ID),
		slog.String("user_id", sessA.UserID),
		slog.String("lang", sessA.Lang),
		slog.String("room_id", r.ID),
		slog.String("component", "service"),
	)
	slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
		slog.String("event", "session_start"),
		slog.String("session_id", sessB.ID),
		slog.String("user_id", sessB.UserID),
		slog.String("lang", sessB.Lang),
		slog.String("room_id", r.ID),
		slog.String("component", "service"),
	)

	slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
		slog.String("event", "pipeline_start"),
		slog.String("room_id", r.ID),
		slog.String("sessA", sessA.ID),
		slog.String("sessB", sessB.ID),
		slog.String("langA", sessA.Lang),
		slog.String("langB", sessB.Lang),
		slog.String("component", "pipeline"),
	)

	p.wg.Add(2)
	go s.runHalf(ctx, p, &halfAtoB)
	go s.runHalf(ctx, p, &halfBtoA)

	// Wait for both halves to complete, then emit pipeline_stop.
	go func() {
		p.wg.Wait()
		chunksAtoB := halfAtoB.totalChunks.Load()
		chunksBtoA := halfBtoA.totalChunks.Load()
		slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
			slog.String("event", "pipeline_stop"),
			slog.String("room_id", r.ID),
			slog.Int64("total_chunks_AtoB", chunksAtoB),
			slog.Int64("total_chunks_BtoA", chunksBtoA),
			slog.String("component", "pipeline"),
		)
	}()
}

// runHalf processes one direction of the translation pipeline: receive audio from
// source session, decode, translate, encode, send to target session, and emit
// chunk_latency timing for each audio frame.
func (s *Service) runHalf(ctx context.Context, p *pipeline, half *pipelineHalf) {
	defer p.wg.Done()

	tracker := half.tracker
	roomID := p.roomID
	frameCount := 0

	opusCh := make(chan []byte, 8)
	err := s.peer.OnAudioTrack(ctx, half.sourceSessID, func(trackCh <-chan []byte) {
		for frame := range trackCh {
			frameCount++
			tracker.Reset()
			tracker.StartStage(StageCapture)
			// Capture is instant — frame is already in memory from trackCh.
			tracker.EndStage(StageCapture)

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

	// Stage 2: Decode
	tracker.StartStage(StageDecode)
	pcmCh, err := s.codec.Decode(ctx, opusCh)
	if err != nil {
		half.errorChunks.Add(int64(frameCount))
		tracker.Emit(ctx, half.dir, roomID, "error")
		s.notifier.NotifySession(half.sourceSessID, "error",
			map[string]string{"code": "codec", "message": "audio processing error", "session_id": half.sourceSessID})
		return
	}
	tracker.EndStage(StageDecode)

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

	// Stage 3: Translate
	tracker.StartStage(StageTranslate)
	result, err := s.translator.TranslateStream(ctx, bpCh, half.sourceLang, half.targetLang)
	if err != nil {
		half.errorChunks.Add(int64(frameCount))
		tracker.Emit(ctx, half.dir, roomID, "error")
		s.logSessionError(half.sourceSessID, StageTranslate, err)
		s.notifier.NotifySession(half.sourceSessID, "error",
			map[string]string{"code": "translation", "message": "translation service error", "session_id": half.sourceSessID})
		return
	}
	tracker.EndStage(StageTranslate)

	// Forward transcripts to the target session as they arrive.
	// If TTS is configured, also synthesize audio and send it via WebRTC.
	targetSessID := half.targetSessID
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for text := range result.Transcript {
			slog.Info("pipeline_transcript", "dir", half.dir, "target_session", targetSessID, "text", text)
			s.notifier.NotifySession(targetSessID, "transcript", map[string]string{
				"text": text,
			})
			if s.tts != nil {
				go s.sendTTSAudio(ctx, targetSessID, text, half.targetLang)
			}
		}
	}()

	// Stage 4: Encode
	tracker.StartStage(StageEncode)
	opusOutCh, err := s.codec.Encode(ctx, result.Audio)
	if err != nil {
		half.errorChunks.Add(int64(frameCount))
		tracker.Emit(ctx, half.dir, roomID, "error")
		s.logSessionError(half.targetSessID, StageEncode, err)
		s.notifier.NotifySession(half.targetSessID, "error", map[string]string{
			"code":       "codec",
			"message":    "audio encoding error",
			"session_id": half.sourceSessID,
		})
		return
	}
	tracker.EndStage(StageEncode)

	// Stage 5: Send
	tracker.StartStage(StageSend)
	if err := s.peer.SendAudio(ctx, half.targetSessID, opusOutCh); err != nil {
		half.errorChunks.Add(int64(frameCount))
		tracker.Emit(ctx, half.dir, roomID, "error")
		s.notifier.NotifySession(half.targetSessID, "error", map[string]string{
			"code":       "server",
			"message":    "audio send error",
			"session_id": half.sourceSessID,
		})
		return
	}
	tracker.EndStage(StageSend)

	// All stages completed successfully.
	half.totalChunks.Add(int64(frameCount))
	tracker.Emit(ctx, half.dir, roomID, "ok")
}

// sendTTSAudio synthesizes text to audio using the TTS adapter, encodes it to
// Opus, and sends it to the target peer via WebRTC. Errors are logged and
// silently dropped — TTS failure must not interrupt the transcript flow.
func (s *Service) sendTTSAudio(ctx context.Context, targetSessID, text, lang string) {
	pcmCh, err := s.tts.Synthesize(ctx, text, lang)
	if err != nil {
		slog.Warn("tts_synthesize_error", "session", targetSessID, "err", err)
		return
	}
	opusCh, err := s.codec.Encode(ctx, pcmCh)
	if err != nil {
		slog.Warn("tts_encode_error", "session", targetSessID, "err", err)
		return
	}
	if err := s.peer.SendAudio(ctx, targetSessID, opusCh); err != nil {
		slog.Warn("tts_send_error", "session", targetSessID, "err", err)
	}
}

// logSessionError emits a session_error event and tracks error count.
func (s *Service) logSessionError(sessionID, stage string, err error) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	sess.ErrorCount++
	slog.LogAttrs(context.Background(), slog.LevelError, "session_event",
		slog.String("event", "session_error"),
		slog.String("session_id", sessionID),
		slog.String("error", err.Error()),
		slog.Int("error_count", sess.ErrorCount),
		slog.String("stage", stage),
		slog.String("component", "pipeline"),
	)
}
