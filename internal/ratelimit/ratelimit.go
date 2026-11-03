// Package ratelimit implements two of the Stage 2 rate-limiting algorithms
// (token bucket and sliding-window counter) plus a per-client wrapper used
// by the HTTP middleware. Every limiter takes a clock so tests are exact.
package ratelimit

import (
	"sync"
	"time"
)

// Clock returns the current time. Pass time.Now in production.
type Clock func() time.Time

// TokenBucket allows bursts up to capacity and a long-run rate of
// refillPerSec. State is two numbers: tokens left and the last refill time.
type TokenBucket struct {
	mu       sync.Mutex
	capacity float64
	rate     float64 // tokens per second
	tokens   float64
	last     time.Time
	now      Clock
}

// NewTokenBucket starts full, so a new client can burst immediately.
func NewTokenBucket(capacity int, refillPerSec float64, now Clock) *TokenBucket {
	return &TokenBucket{
		capacity: float64(capacity),
		rate:     refillPerSec,
		tokens:   float64(capacity),
		last:     now(),
		now:      now,
	}
}

// Allow takes one token. When the bucket is empty it reports how long the
// caller should wait, which becomes the HTTP Retry-After header.
func (b *TokenBucket) Allow() (ok bool, retryAfter time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	// Refill lazily: no background goroutine, just arithmetic on elapsed time.
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(b.capacity, b.tokens+elapsed*b.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	missing := 1 - b.tokens
	return false, time.Duration(missing / b.rate * float64(time.Second))
}

// full reports whether the bucket has refilled completely, meaning the
// client has been idle long enough that forgetting it changes nothing.
func (b *TokenBucket) full() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	elapsed := b.now().Sub(b.last).Seconds()
	return b.tokens+elapsed*b.rate >= b.capacity
}

// SlidingWindow approximates a true sliding window with two fixed-window
// counters. The previous window's count is weighted by how much of it still
// overlaps the sliding window, which removes the 2x burst a plain fixed
// window allows at the boundary.
type SlidingWindow struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	currStart time.Time
	curr      int
	prev      int
	now       Clock
}

// NewSlidingWindow allows limit requests per window.
func NewSlidingWindow(limit int, window time.Duration, now Clock) *SlidingWindow {
	return &SlidingWindow{limit: limit, window: window, now: now}
}

// Allow counts the request if the weighted estimate is under the limit.
func (s *SlidingWindow) Allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	start := now.Truncate(s.window)
	switch {
	case start.Equal(s.currStart):
		// still inside the current window
	case start.Sub(s.currStart) == s.window:
		s.prev, s.curr = s.curr, 0 // moved into the next window
		s.currStart = start
	default:
		s.prev, s.curr = 0, 0 // idle for more than a whole window
		s.currStart = start
	}
	overlap := 1 - float64(now.Sub(start))/float64(s.window)
	estimate := float64(s.prev)*overlap + float64(s.curr)
	if estimate >= float64(s.limit) {
		return false
	}
	s.curr++
	return true
}

// PerKey keeps one token bucket per client key (an IP, API key or user).
// Buckets that have refilled completely are dropped during a sweep, so
// memory tracks active clients rather than every client ever seen.
type PerKey struct {
	mu       sync.Mutex
	buckets  map[string]*TokenBucket
	capacity int
	rate     float64
	now      Clock
	sweepAt  int
}

// NewPerKey sweeps idle buckets whenever the map grows past sweepAt.
func NewPerKey(capacity int, refillPerSec float64, sweepAt int, now Clock) *PerKey {
	return &PerKey{
		buckets:  map[string]*TokenBucket{},
		capacity: capacity,
		rate:     refillPerSec,
		now:      now,
		sweepAt:  max(sweepAt, 1),
	}
}

// Allow applies the bucket that belongs to key.
func (p *PerKey) Allow(key string) (bool, time.Duration) {
	p.mu.Lock()
	b, ok := p.buckets[key]
	if !ok {
		if len(p.buckets) >= p.sweepAt {
			for k, old := range p.buckets {
				if old.full() {
					delete(p.buckets, k)
				}
			}
		}
		b = NewTokenBucket(p.capacity, p.rate, p.now)
		p.buckets[key] = b
	}
	p.mu.Unlock()
	return b.Allow()
}

// Len is the number of tracked clients.
func (p *PerKey) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buckets)
}
