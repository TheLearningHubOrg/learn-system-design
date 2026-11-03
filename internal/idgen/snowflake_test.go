package idgen

import (
	"errors"
	"testing"
	"time"
)

// steppingClock returns base for the first `hold` calls, then base+1ms.
type steppingClock struct {
	base  time.Time
	hold  int
	calls int
}

func (c *steppingClock) Now() time.Time {
	c.calls++
	if c.calls <= c.hold {
		return c.base
	}
	return c.base.Add(time.Millisecond)
}

func TestNodeValidation(t *testing.T) {
	if _, err := New(MaxNode+1, DefaultEpoch, time.Now); err == nil {
		t.Fatal("node 1024 accepted")
	}
	if _, err := New(-1, DefaultEpoch, time.Now); err == nil {
		t.Fatal("negative node accepted")
	}
}

func TestIDsIncreaseAndDecode(t *testing.T) {
	at := DefaultEpoch.Add(1234 * time.Hour)
	g, _ := New(42, DefaultEpoch, func() time.Time { return at })
	var prev int64 = -1
	for i := range 100 {
		id, err := g.Next()
		if err != nil {
			t.Fatal(err)
		}
		if id <= prev {
			t.Fatalf("id %d not greater than %d", id, prev)
		}
		prev = id
		p := Decode(id, DefaultEpoch)
		if p.Node != 42 || p.Sequence != int64(i) || !p.Time.Equal(at) {
			t.Fatalf("decoded %+v", p)
		}
	}
}

func TestSequenceOverflowWaitsForNextMillisecond(t *testing.T) {
	clk := &steppingClock{base: DefaultEpoch.Add(time.Hour), hold: 4096 + 1}
	g, _ := New(1, DefaultEpoch, clk.Now)
	seen := map[int64]bool{}
	var last int64
	for range 4097 {
		id, err := g.Next()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = true
		last = id
	}
	p := Decode(last, DefaultEpoch)
	if p.Sequence != 0 || !p.Time.Equal(clk.base.Add(time.Millisecond)) {
		t.Fatalf("ID 4097 should start the next millisecond at sequence 0, got %+v", p)
	}
}

func TestClockMovingBackwardsIsAnError(t *testing.T) {
	now := DefaultEpoch.Add(time.Hour)
	g, _ := New(1, DefaultEpoch, func() time.Time { return now })
	if _, err := g.Next(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(-5 * time.Millisecond)
	if _, err := g.Next(); !errors.Is(err, ErrClockMovedBackwards) {
		t.Fatalf("got %v, want ErrClockMovedBackwards", err)
	}
}
