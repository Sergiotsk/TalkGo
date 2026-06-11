package room_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
)

// ---------------------------------------------------------------------------
// GenerateShortCode
// ---------------------------------------------------------------------------

func TestGenerateShortCode_Length(t *testing.T) {
	code, err := room.GenerateShortCode(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected length 6, got %d (%q)", len(code), code)
	}
}

func TestGenerateShortCode_Alphabet(t *testing.T) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for i := 0; i < 50; i++ {
		code, err := room.GenerateShortCode(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, ch := range code {
			if !strings.ContainsRune(alphabet, ch) {
				t.Errorf("char %q not in alphabet (code=%q)", ch, code)
			}
		}
	}
}

func TestGenerateShortCode_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		code, err := room.GenerateShortCode(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[code] = true
	}
	// With 32^6 ≈ 1B combinations we expect all 100 to be unique.
	if len(seen) < 99 {
		t.Errorf("expected ≥99 unique codes out of 100, got %d", len(seen))
	}
}

func TestGenerateShortCode_ReaderError(t *testing.T) {
	// errReader always returns an error — simulates crypto/rand failure.
	errReader := &errReader{}
	_, err := room.GenerateShortCode(errReader)
	if err == nil {
		t.Error("expected error from failing reader, got nil")
	}
}

// errReader is an io.Reader that always fails.
type errReader struct{}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read error")
}

// ---------------------------------------------------------------------------
// GenerateShortCode — no forbidden chars (0, O, 1, I)
// ---------------------------------------------------------------------------

func TestGenerateShortCode_NoForbiddenChars(t *testing.T) {
	const forbidden = "01OI"
	for i := 0; i < 200; i++ {
		code, err := room.GenerateShortCode(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, ch := range code {
			if strings.ContainsRune(forbidden, ch) {
				t.Errorf("forbidden char %q found in code %q", ch, code)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Room.TouchActivity
// ---------------------------------------------------------------------------

func TestRoom_TouchActivity_UpdatesLastActivity(t *testing.T) {
	r, err := room.NewRoom("r1", "es", "en")
	if err != nil {
		t.Fatalf("NewRoom: %v", err)
	}

	before := time.Now()
	r.TouchActivity()
	after := time.Now()

	if r.LastActivity.IsZero() {
		t.Fatal("LastActivity should not be zero after TouchActivity")
	}
	if r.LastActivity.Before(before) || r.LastActivity.After(after) {
		t.Errorf("LastActivity %v not in [%v, %v]", r.LastActivity, before, after)
	}
}

func TestRoom_TouchActivity_Idempotent(t *testing.T) {
	r, _ := room.NewRoom("r1", "es", "en")
	r.TouchActivity()
	first := r.LastActivity

	time.Sleep(1 * time.Millisecond)
	r.TouchActivity()
	second := r.LastActivity

	if !second.After(first) {
		t.Error("second TouchActivity should update LastActivity to a later time")
	}
}

// ---------------------------------------------------------------------------
// Room.ShortCode field
// ---------------------------------------------------------------------------

func TestNewRoom_ShortCodeEmptyByDefault(t *testing.T) {
	r, _ := room.NewRoom("r1", "es", "en")
	if r.ShortCode != "" {
		t.Errorf("expected empty ShortCode from NewRoom, got %q", r.ShortCode)
	}
}

// ---------------------------------------------------------------------------
// ErrShortCodeExhausted sentinel
// ---------------------------------------------------------------------------

func TestErrShortCodeExhausted_IsSentinel(t *testing.T) {
	err := room.ErrShortCodeExhausted
	if err == nil {
		t.Fatal("ErrShortCodeExhausted should not be nil")
	}
	if !errors.Is(err, room.ErrShortCodeExhausted) {
		t.Error("errors.Is should match ErrShortCodeExhausted")
	}
}

// ---------------------------------------------------------------------------
// GenerateShortCode — deterministic reader (for coverage)
// ---------------------------------------------------------------------------

func TestGenerateShortCode_DeterministicReader(t *testing.T) {
	// A reader that always returns index 0 → first char of alphabet 'A' repeated.
	r := bytes.NewReader(make([]byte, 6))
	code, err := room.GenerateShortCode(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected length 6, got %d", len(code))
	}
	// All bytes 0 → index 0 in alphabet → 'A'
	if code != "AAAAAA" {
		t.Errorf("expected AAAAAA from zero reader, got %q", code)
	}
}
