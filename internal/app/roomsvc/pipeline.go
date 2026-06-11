package roomsvc

import (
	"context"
	"fmt"
	"sync"

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
	}
	halfBtoA := pipelineHalf{
		sourceSessID: sessB.ID,
		targetSessID: sessA.ID,
		sourceLang:   sessB.Lang,
		targetLang:   sessA.Lang,
	}

	p.wg.Add(2)
	go s.runHalf(ctx, p, halfAtoB)
	go s.runHalf(ctx, p, halfBtoA)
}

// runHalf processes one direction of the translation pipeline: receive audio from
// source session, decode, translate, encode, and send to target session.
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
