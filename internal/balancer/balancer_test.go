package balancer

import (
	"slices"
	"sync"
	"testing"
)

func TestRoundRobinRotates(t *testing.T) {
	rr := NewRoundRobin("a", "b", "c")
	var got []string
	for range 6 {
		got = append(got, rr.Pick())
	}
	if want := []string{"a", "b", "c", "a", "b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRoundRobinIsEvenUnderConcurrency(t *testing.T) {
	rr := NewRoundRobin("a", "b", "c", "d")
	var mu sync.Mutex
	counts := map[string]int{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				b := rr.Pick()
				mu.Lock()
				counts[b]++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	for b, n := range counts {
		if n != 2000 {
			t.Errorf("%s got %d picks, want exactly 2000", b, n)
		}
	}
}

func TestSmoothWeightedMatchesNginxSequence(t *testing.T) {
	s := NewSmoothWeighted(Weighted{"a", 5}, Weighted{"b", 1}, Weighted{"c", 1})
	var got []string
	for range 7 {
		got = append(got, s.Pick())
	}
	if want := []string{"a", "a", "b", "a", "c", "a", "a"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLeastConnectionsPrefersIdleBackend(t *testing.T) {
	l := NewLeastConnections("a", "b")
	first, releaseA := l.Acquire()
	second, releaseB := l.Acquire()
	if first != "a" || second != "b" {
		t.Fatalf("got %s then %s, want a then b", first, second)
	}
	// b finishes; the next request must go to b, which is now idle.
	releaseB()
	releaseB() // a second call is a no-op
	if next, _ := l.Acquire(); next != "b" {
		t.Fatalf("got %s, want b", next)
	}
	releaseA()
	if l.Active("a") != 0 {
		t.Fatalf("a still has %d in flight", l.Active("a"))
	}
}
