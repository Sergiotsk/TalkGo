package mocks

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

var _ driven.Translator = (*MockTranslator)(nil)

// MockTranslator is a test double for driven.Translator.
// Configure behaviour by assigning TranslateStreamFn before use.
// When TranslateStreamFn is nil the mock falls back to a passthrough.
type MockTranslator struct {
	TranslateStreamFn func(ctx context.Context, audioIn <-chan []byte, sourceLang, targetLang string) (driven.TranslateResult, error)

	translateStreamCalled atomic.Int64

	lastLangMu     sync.Mutex
	lastSourceLang string
	lastTargetLang string
}

// TranslateStreamCalled returns the number of TranslateStream calls.
func (m *MockTranslator) TranslateStreamCalled() int { return int(m.translateStreamCalled.Load()) }

// LastSourceLang returns the last sourceLang passed to TranslateStream.
func (m *MockTranslator) LastSourceLang() string {
	m.lastLangMu.Lock()
	defer m.lastLangMu.Unlock()
	return m.lastSourceLang
}

// LastTargetLang returns the last targetLang passed to TranslateStream.
func (m *MockTranslator) LastTargetLang() string {
	m.lastLangMu.Lock()
	defer m.lastLangMu.Unlock()
	return m.lastTargetLang
}

// TranslateStream implements driven.Translator.
func (m *MockTranslator) TranslateStream(ctx context.Context, audioIn <-chan []byte, sourceLang, targetLang string) (driven.TranslateResult, error) {
	m.translateStreamCalled.Add(1)
	m.lastLangMu.Lock()
	m.lastSourceLang = sourceLang
	m.lastTargetLang = targetLang
	m.lastLangMu.Unlock()
	if m.TranslateStreamFn != nil {
		return m.TranslateStreamFn(ctx, audioIn, sourceLang, targetLang)
	}
	transcriptCh := make(chan string)
	close(transcriptCh)
	return driven.TranslateResult{Audio: passthrough(ctx, audioIn), Transcript: transcriptCh}, nil
}
