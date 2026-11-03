// Package balancer implements the backend-selection algorithms from Stage 2:
// round robin, smooth weighted round robin and least connections.
package balancer

import (
	"sync"
	"sync/atomic"
)

// RoundRobin hands out backends in a fixed rotation. It is lock-free: a
// single atomic counter is the only shared state.
type RoundRobin struct {
	backends []string
	next     atomic.Uint64
}

// NewRoundRobin panics on an empty list because there is nothing to pick.
func NewRoundRobin(backends ...string) *RoundRobin {
	if len(backends) == 0 {
		panic("balancer: no backends")
	}
	return &RoundRobin{backends: backends}
}

// Pick returns the next backend in the rotation.
func (r *RoundRobin) Pick() string {
	n := r.next.Add(1) - 1
	return r.backends[n%uint64(len(r.backends))]
}

// SmoothWeighted is the interleaving weighted round robin that nginx uses.
// With weights a=5, b=1, c=1 it yields a a b a c a a rather than
// a a a a a b c, so a heavy backend never receives a long burst.
type SmoothWeighted struct {
	mu    sync.Mutex
	peers []*peer
	total int
}

type peer struct {
	name    string
	weight  int
	current int
}

// Weighted is one backend and its relative capacity.
type Weighted struct {
	Name   string
	Weight int
}

// NewSmoothWeighted ignores backends with a weight below 1.
func NewSmoothWeighted(backends ...Weighted) *SmoothWeighted {
	s := &SmoothWeighted{}
	for _, b := range backends {
		if b.Weight < 1 {
			continue
		}
		s.peers = append(s.peers, &peer{name: b.Name, weight: b.Weight})
		s.total += b.Weight
	}
	if len(s.peers) == 0 {
		panic("balancer: no backends with positive weight")
	}
	return s
}

// Pick raises every peer's score by its weight, chooses the highest score
// and lowers the winner by the total weight.
func (s *SmoothWeighted) Pick() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best *peer
	for _, p := range s.peers {
		p.current += p.weight
		if best == nil || p.current > best.current {
			best = p
		}
	}
	best.current -= s.total
	return best.name
}

// LeastConnections routes each request to the backend with the fewest
// requests in flight. Callers must call the release function when the
// request finishes, or the count drifts upward forever.
type LeastConnections struct {
	mu     sync.Mutex
	order  []string
	active map[string]int
}

// NewLeastConnections panics on an empty list.
func NewLeastConnections(backends ...string) *LeastConnections {
	if len(backends) == 0 {
		panic("balancer: no backends")
	}
	l := &LeastConnections{order: backends, active: make(map[string]int, len(backends))}
	for _, b := range backends {
		l.active[b] = 0
	}
	return l
}

// Acquire picks the least-loaded backend (ties go to the earliest in the
// list) and returns a function that marks the request as finished.
func (l *LeastConnections) Acquire() (backend string, release func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	best := l.order[0]
	for _, b := range l.order[1:] {
		if l.active[b] < l.active[best] {
			best = b
		}
	}
	l.active[best]++
	var once sync.Once
	return best, func() {
		once.Do(func() {
			l.mu.Lock()
			l.active[best]--
			l.mu.Unlock()
		})
	}
}

// Active reports the in-flight count for one backend.
func (l *LeastConnections) Active(backend string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active[backend]
}
