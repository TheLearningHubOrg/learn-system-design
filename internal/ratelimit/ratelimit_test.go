package ratelimit

import (
	"fmt"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }

func TestTokenBucketBurstThenRefill(t *testing.T) {
	clk := newClock()
	b := NewTokenBucket(3, 1, clk.Now) // burst of 3, one token per second

	for i := range 3 {
		if ok, _ := b.Allow(); !ok {
			t.Fatalf("request %d of the burst was rejected", i+1)
		}
	}
	ok, wait := b.Allow()
	if ok {
		t.Fatal("4th request in the same instant was allowed")
	}
	if wait != time.Second {
		t.Fatalf("retry after %v, want 1s", wait)
	}

	clk.Advance(500 * time.Millisecond)
	if ok, wait := b.Allow(); ok || wait != 500*time.Millisecond {
		t.Fatalf("half a token: ok=%v wait=%v, want rejected with 500ms", ok, wait)
	}
	clk.Advance(500 * time.Millisecond)
	if ok, _ := b.Allow(); !ok {
		t.Fatal("a full token had refilled but the request was rejected")
	}
}

func TestTokenBucketNeverExceedsCapacity(t *testing.T) {
	clk := newClock()
	b := NewTokenBucket(2, 10, clk.Now)
	clk.Advance(time.Hour) // idle for an hour must not bank 36,000 tokens
	allowed := 0
	for range 10 {
		if ok, _ := b.Allow(); ok {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("allowed %d after idling, want capacity 2", allowed)
	}
}

func TestSlidingWindowBlocksBoundaryBurst(t *testing.T) {
	clk := newClock()
	s := NewSlidingWindow(10, time.Minute, clk.Now)

	// 10 requests in the last second of a window...
	clk.Advance(59 * time.Second)
	for i := range 10 {
		if !s.Allow() {
			t.Fatalf("request %d rejected inside the limit", i+1)
		}
	}
	// ...then the window rolls over. A fixed window would allow 10 more
	// immediately (20 in two seconds). One second into the new window the
	// previous window still weighs 59/60, so only one request fits.
	clk.Advance(2 * time.Second)
	if got := burst(s, 10); got != 1 {
		t.Fatalf("allowed %d right after the boundary, want 1", got)
	}
	// Half way through the new window the previous 10 weigh 5, plus the 1
	// already counted, so 4 more fit under the limit of 10.
	clk.Advance(29 * time.Second)
	if got := burst(s, 10); got != 4 {
		t.Fatalf("allowed %d at mid-window, want 4", got)
	}
}

func burst(s *SlidingWindow, n int) int {
	allowed := 0
	for range n {
		if s.Allow() {
			allowed++
		}
	}
	return allowed
}

func TestSlidingWindowResetsAfterIdle(t *testing.T) {
	clk := newClock()
	s := NewSlidingWindow(1, time.Second, clk.Now)
	if !s.Allow() || s.Allow() {
		t.Fatal("limit of 1 not enforced")
	}
	clk.Advance(5 * time.Second)
	if !s.Allow() {
		t.Fatal("request after a long idle period was rejected")
	}
}

func TestPerKeyIsolatesClientsAndSweepsIdleOnes(t *testing.T) {
	clk := newClock()
	p := NewPerKey(1, 1, 3, clk.Now)
	if ok, _ := p.Allow("10.0.0.1"); !ok {
		t.Fatal("first request rejected")
	}
	if ok, _ := p.Allow("10.0.0.1"); ok {
		t.Fatal("second request for the same client allowed")
	}
	if ok, _ := p.Allow("10.0.0.2"); !ok {
		t.Fatal("another client was throttled by the first one")
	}
	p.Allow("10.0.0.3")
	clk.Advance(10 * time.Second) // everyone refills
	p.Allow("10.0.0.4")           // map is at sweepAt, so idle buckets go
	if got := p.Len(); got != 1 {
		t.Fatalf("tracking %d clients after sweep, want 1", got)
	}
}

func ExampleTokenBucket() {
	clk := newClock()
	b := NewTokenBucket(2, 1, clk.Now)
	for i := range 3 {
		ok, wait := b.Allow()
		fmt.Printf("request %d allowed=%v retry_after=%v\n", i+1, ok, wait)
	}
	// Output:
	// request 1 allowed=true retry_after=0s
	// request 2 allowed=true retry_after=0s
	// request 3 allowed=false retry_after=1s
}
