// Package hashring is a consistent-hash ring with virtual nodes (Stage 2).
//
// Each physical node is placed on a 64-bit ring many times. A key belongs to
// the first virtual node at or clockwise after the key's hash. Adding or
// removing one of N nodes moves only about 1/N of the keys.
package hashring

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"
	"sync"
)

type point struct {
	hash uint64
	node string
}

// Ring is safe for concurrent use.
type Ring struct {
	mu       sync.RWMutex
	replicas int
	points   []point // sorted by hash
	nodes    map[string]struct{}
}

// New creates a ring that places each node replicas times. 100-200 virtual
// nodes per physical node keeps the load within a few percent of even.
func New(replicas int) *Ring {
	if replicas < 1 {
		replicas = 1
	}
	return &Ring{replicas: replicas, nodes: map[string]struct{}{}}
}

// hashKey is FNV-1a followed by the SplitMix64 finaliser. FNV on its own
// clusters strings that differ only in their last characters ("node#1",
// "node#2"), which is exactly what virtual-node labels look like.
func hashKey(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	x := h.Sum64()
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func comparePoint(p point, target uint64) int { return cmp.Compare(p.hash, target) }

// Add places a node on the ring. Adding a node twice is a no-op.
func (r *Ring) Add(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[node]; ok {
		return
	}
	r.nodes[node] = struct{}{}
	for i := range r.replicas {
		r.points = append(r.points, point{hashKey(node + "#" + strconv.Itoa(i)), node})
	}
	slices.SortFunc(r.points, func(a, b point) int { return cmp.Compare(a.hash, b.hash) })
}

// Remove takes a node and all of its virtual nodes off the ring.
func (r *Ring) Remove(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[node]; !ok {
		return
	}
	delete(r.nodes, node)
	r.points = slices.DeleteFunc(r.points, func(p point) bool { return p.node == node })
}

// Get returns the node that owns key, or false when the ring is empty.
func (r *Ring) Get(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.points) == 0 {
		return "", false
	}
	i, _ := slices.BinarySearchFunc(r.points, hashKey(key), comparePoint)
	if i == len(r.points) {
		i = 0 // past the largest hash: wrap around to the start of the ring
	}
	return r.points[i].node, true
}

// GetN walks clockwise from key and returns up to n distinct nodes. The
// first is the owner; the rest are where replicas go (a Dynamo-style
// preference list).
func (r *Ring) GetN(key string, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n = min(n, len(r.nodes))
	if n <= 0 {
		return nil
	}
	out := make([]string, 0, n)
	start, _ := slices.BinarySearchFunc(r.points, hashKey(key), comparePoint)
	for i := 0; len(out) < n; i++ {
		node := r.points[(start+i)%len(r.points)].node
		if !slices.Contains(out, node) {
			out = append(out, node)
		}
	}
	return out
}

// Nodes returns the physical nodes in sorted order.
func (r *Ring) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.nodes))
	for n := range r.nodes {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}
