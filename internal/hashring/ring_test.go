package hashring

import (
	"fmt"
	"testing"
)

const keys = 100_000

func ownership(r *Ring) map[string]string {
	owner := make(map[string]string, keys)
	for i := range keys {
		k := fmt.Sprintf("user:%d", i)
		owner[k], _ = r.Get(k)
	}
	return owner
}

func TestEmptyRing(t *testing.T) {
	if _, ok := New(10).Get("x"); ok {
		t.Fatal("empty ring returned an owner")
	}
}

func TestLoadIsRoughlyEven(t *testing.T) {
	r := New(160)
	for _, n := range []string{"cache-a", "cache-b", "cache-c"} {
		r.Add(n)
	}
	counts := map[string]int{}
	for _, n := range ownership(r) {
		counts[n]++
	}
	mean := keys / 3
	for n, c := range counts {
		if c < mean*80/100 || c > mean*120/100 {
			t.Errorf("%s owns %d keys, want within 20%% of %d", n, c, mean)
		}
	}
}

func TestAddingANodeMovesAboutOneNth(t *testing.T) {
	r := New(160)
	for _, n := range []string{"cache-a", "cache-b", "cache-c"} {
		r.Add(n)
	}
	before := ownership(r)
	r.Add("cache-d")
	after := ownership(r)

	moved := 0
	for k, was := range before {
		if now := after[k]; now != was {
			moved++
			if now != "cache-d" {
				t.Fatalf("key %s moved %s -> %s; keys may only move to the new node", k, was, now)
			}
		}
	}
	frac := float64(moved) / keys
	if frac < 0.15 || frac > 0.35 {
		t.Errorf("%.1f%% of keys moved, want about 25%%", frac*100)
	}
	t.Logf("adding a 4th node moved %.1f%% of keys (modulo hashing would move 75%%)", frac*100)
}

func TestRemovingANodeOnlyMovesItsKeys(t *testing.T) {
	r := New(160)
	for _, n := range []string{"cache-a", "cache-b", "cache-c"} {
		r.Add(n)
	}
	before := ownership(r)
	r.Remove("cache-b")
	for k, now := range ownership(r) {
		if was := before[k]; was != "cache-b" && now != was {
			t.Fatalf("key %s moved %s -> %s although its node stayed", k, was, now)
		}
	}
}

func TestGetNReturnsDistinctNodesOwnerFirst(t *testing.T) {
	r := New(50)
	for _, n := range []string{"a", "b", "c"} {
		r.Add(n)
	}
	owner, _ := r.Get("order:42")
	got := r.GetN("order:42", 5) // asks for more than exist
	if len(got) != 3 || got[0] != owner {
		t.Fatalf("GetN = %v, owner %s", got, owner)
	}
	if got[0] == got[1] || got[1] == got[2] || got[0] == got[2] {
		t.Fatalf("GetN returned duplicates: %v", got)
	}
}

func TestModuloHashingMovesMostKeys(t *testing.T) {
	// The contrast case from Stage 2: hash(key) % N going from 3 to 4 nodes.
	moved := 0
	for i := range keys {
		h := hashKey(fmt.Sprintf("user:%d", i))
		if h%3 != h%4 {
			moved++
		}
	}
	if frac := float64(moved) / keys; frac < 0.70 {
		t.Errorf("modulo moved only %.1f%%, expected about 75%%", frac*100)
	}
}
