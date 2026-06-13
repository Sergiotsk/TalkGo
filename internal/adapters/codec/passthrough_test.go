package codec_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/adapters/codec"
)

func TestPassthroughCodec_Decode_PassesFrames(t *testing.T) {
	t.Parallel()

	c := codec.NewPassthroughCodec()
	ctx := context.Background()

	in := make(chan []byte, 1)
	in <- []byte("test-frame")
	close(in)

	out, err := c.Decode(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case frame := <-out:
		if string(frame) != "test-frame" {
			t.Errorf("Decode: got %q, want %q", frame, "test-frame")
		}
	case <-time.After(time.Second):
		t.Fatal("Decode: timeout waiting for frame")
	}
}

func TestPassthroughCodec_Encode_PassesFrames(t *testing.T) {
	t.Parallel()

	c := codec.NewPassthroughCodec()
	ctx := context.Background()

	in := make(chan []byte, 1)
	in <- []byte("pcm-frame")
	close(in)

	out, err := c.Encode(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case frame := <-out:
		if string(frame) != "pcm-frame" {
			t.Errorf("Encode: got %q, want %q", frame, "pcm-frame")
		}
	case <-time.After(time.Second):
		t.Fatal("Encode: timeout waiting for frame")
	}
}

func TestPassthroughCodec_Decode_ClosesOutputWhenInputClosed(t *testing.T) {
	t.Parallel()

	c := codec.NewPassthroughCodec()
	ctx := context.Background()

	in := make(chan []byte)
	close(in) // closed immediately, no frames

	out, err := c.Decode(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("Decode: expected output channel to be closed, but received a frame")
		}
	case <-time.After(time.Second):
		t.Fatal("Decode: timeout — output channel not closed after input closed")
	}
}

func TestPassthroughCodec_CancelContext_ClosesOutput(t *testing.T) {
	t.Parallel()

	c := codec.NewPassthroughCodec()
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan []byte) // never sends

	out, err := c.Decode(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	cancel() // trigger cancellation

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("Decode: expected output channel to be closed after ctx cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("Decode: timeout — output channel not closed after context cancelled")
	}
}

func TestPassthroughCodec_MultipleFrames(t *testing.T) {
	t.Parallel()

	c := codec.NewPassthroughCodec()
	ctx := context.Background()

	frames := [][]byte{
		[]byte("frame-1"),
		[]byte("frame-2"),
		[]byte("frame-3"),
	}

	in := make(chan []byte, len(frames))
	for _, f := range frames {
		in <- f
	}
	close(in)

	out, err := c.Encode(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	for i, want := range frames {
		select {
		case got, ok := <-out:
			if !ok {
				t.Fatalf("MultipleFrames: channel closed early at frame %d", i)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("frame %d: got %q, want %q", i, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("MultipleFrames: timeout at frame %d", i)
		}
	}

	// output channel must be closed after all frames
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("MultipleFrames: expected channel closed after all frames")
		}
	case <-time.After(time.Second):
		t.Fatal("MultipleFrames: timeout waiting for channel close")
	}
}
