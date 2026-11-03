package cache

import "sync"

// Group coalesces concurrent calls for the same key into one execution.
// When 1,000 requests miss the cache for the same hot key at once, only one
// of them queries the database; the other 999 wait and share its result.
// The zero value is ready to use.
//
// golang.org/x/sync/singleflight is the production-grade version of this
// idea; this one is small enough to read in one sitting.
type Group[K comparable, V any] struct {
	mu    sync.Mutex
	calls map[K]*call[V]
}

type call[V any] struct {
	wg  sync.WaitGroup
	val V
	err error
}

// Do runs fn once per key at a time. shared is true for callers that got
// another caller's result.
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (v V, err error, shared bool) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[K]*call[V])
	}
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := &call[V]{}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	// Forget the key even if fn panics, or every later caller would block.
	defer func() {
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
		c.wg.Done()
	}()
	c.val, c.err = fn()
	return c.val, c.err, false
}
