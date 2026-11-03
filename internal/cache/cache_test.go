package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewLRU[string, int](2, time.Now)
	c.Set("a", 1, 0)
	c.Set("b", 2, 0)
	c.Get("a")       // a is now the most recent
	c.Set("c", 3, 0) // evicts b, not a

	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Error("a should still be cached")
	}
	if c.Evictions() != 1 || c.Len() != 2 {
		t.Errorf("evictions=%d len=%d, want 1 and 2", c.Evictions(), c.Len())
	}
}

func TestLRUExpiresEntries(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := NewLRU[string, string](10, clk.Now)
	c.Set("session", "alice", time.Minute)
	c.Set("config", "v1", 0) // no TTL

	clk.Advance(59 * time.Second)
	if _, ok := c.Get("session"); !ok {
		t.Fatal("expired too early")
	}
	clk.Advance(time.Second)
	if _, ok := c.Get("session"); ok {
		t.Fatal("entry served after its TTL")
	}
	if _, ok := c.Get("config"); !ok {
		t.Fatal("entry without TTL expired")
	}
	if c.Len() != 1 {
		t.Fatalf("expired entry not removed on read, len=%d", c.Len())
	}
}

func TestLRUOverwriteRefreshesValueAndTTL(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := NewLRU[int, string](10, clk.Now)
	c.Set(1, "old", time.Second)
	c.Set(1, "new", time.Hour)
	clk.Advance(2 * time.Second)
	if v, ok := c.Get(1); !ok || v != "new" {
		t.Fatalf("got %q %v, want new", v, ok)
	}
}

func TestGroupCoalescesConcurrentMisses(t *testing.T) {
	var g Group[string, int]
	var calls atomic.Int32
	release := make(chan struct{})

	const callers = 50
	var started, done sync.WaitGroup
	results := make([]int, callers)
	sharedCount := atomic.Int32{}
	for i := range callers {
		started.Add(1)
		done.Go(func() {
			started.Done()
			v, err, shared := g.Do("product:1", func() (int, error) {
				calls.Add(1)
				<-release // hold the "database query" open
				return 42, nil
			})
			if err != nil {
				t.Error(err)
			}
			if shared {
				sharedCount.Add(1)
			}
			results[i] = v
		})
	}
	started.Wait()
	time.Sleep(100 * time.Millisecond) // let every goroutine reach Do
	close(release)
	done.Wait()

	if n := calls.Load(); n != 1 {
		t.Fatalf("loader ran %d times, want 1", n)
	}
	if sharedCount.Load() != callers-1 {
		t.Fatalf("%d callers shared the result, want %d", sharedCount.Load(), callers-1)
	}
	for i, v := range results {
		if v != 42 {
			t.Fatalf("caller %d got %d", i, v)
		}
	}
}

func TestGroupForgetsKeyAfterError(t *testing.T) {
	var g Group[string, int]
	boom := errors.New("db down")
	if _, err, _ := g.Do("k", func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	// The failed call must not be cached: the next caller retries.
	v, err, _ := g.Do("k", func() (int, error) { return 7, nil })
	if err != nil || v != 7 {
		t.Fatalf("retry got %d %v", v, err)
	}
}

func TestShouldRefreshEarly(t *testing.T) {
	now := time.Unix(1000, 0)
	expiry := now.Add(10 * time.Second)
	delta := 2 * time.Second

	// rnd = 1 gives ln(1) = 0: never refresh early.
	if ShouldRefreshEarly(now, expiry, delta, 1, func() float64 { return 1 }) {
		t.Error("refreshed with rnd=1")
	}
	// rnd = e^-6 gives a gap of 12s > 10s left: refresh.
	if !ShouldRefreshEarly(now, expiry, delta, 1, func() float64 { return 0.00247875 }) {
		t.Error("did not refresh with a large gap")
	}
	// Already expired always refreshes.
	if !ShouldRefreshEarly(expiry, expiry, delta, 1, func() float64 { return 1 }) {
		t.Error("did not refresh an expired value")
	}
}

func TestShouldRefreshEarlyGetsLikelierNearExpiry(t *testing.T) {
	// Deterministic sweep over rnd in (0,1]: count how many "readers"
	// would refresh with 10s left versus 1s left.
	count := func(left time.Duration) int {
		now := time.Unix(0, 0)
		n := 0
		for i := 1; i <= 1000; i++ {
			r := float64(i) / 1000
			if ShouldRefreshEarly(now, now.Add(left), time.Second, 1, func() float64 { return r }) {
				n++
			}
		}
		return n
	}
	far, near := count(10*time.Second), count(time.Second)
	if !(far < near) || far != 0 || near < 300 {
		t.Fatalf("refresh counts far=%d near=%d; want 0 far and >30%% near", far, near)
	}
}

func TestJitteredTTLStaysInRange(t *testing.T) {
	base := 10 * time.Minute
	lo := JitteredTTL(base, 0.1, func() float64 { return 0 })
	mid := JitteredTTL(base, 0.1, func() float64 { return 0.5 })
	if lo != 9*time.Minute || mid != base {
		t.Fatalf("lo=%v mid=%v", lo, mid)
	}
}
