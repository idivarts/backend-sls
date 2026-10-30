package elevenlabs

import "testing"

func TestWordMsConvertsSecondsAtTheBoundary(t *testing.T) {
	// The provider speaks float seconds and everything else in this product
	// speaks integer milliseconds. Two units in play is how a trim ends up a
	// thousand times too long, so the conversion happens once, here.
	sec := func(f float64) *float64 { return &f }

	start, end := Word{Start: sec(1.25), End: sec(2.5)}.Ms()
	if start != 1250 || end != 2500 {
		t.Errorf("got %d..%d, want 1250..2500", start, end)
	}

	// A word with no timing is not an error — audio events and spacing can
	// arrive without one — and must not become a negative or a panic.
	start, end = Word{}.Ms()
	if start != 0 || end != 0 {
		t.Errorf("missing timings should be zero, got %d..%d", start, end)
	}

	start, _ = Word{Start: sec(0.0004)}.Ms()
	if start != 0 {
		t.Errorf("sub-millisecond should floor to 0, got %d", start)
	}
}
