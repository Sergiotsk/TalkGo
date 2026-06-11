package roomsvc

import "testing"

func TestDrainOldest_BufferEmpty(t *testing.T) {
	ch := make(chan []byte, 1)
	drainOldest(ch, []byte("new"))
	got := <-ch
	if string(got) != "new" {
		t.Errorf("expected %q got %q", "new", got)
	}
}

func TestDrainOldest_BufferFull_DropsOldest(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte("old")
	drainOldest(ch, []byte("new"))
	got := <-ch
	if string(got) != "new" {
		t.Errorf("expected newest frame %q, got %q", "new", got)
	}
}
