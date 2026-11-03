// Package cache holds the Stage 3 building blocks: an LRU cache with TTLs,
// request coalescing (single-flight), probabilistic early refresh (XFetch)
// and TTL jitter.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// LRU is a fixed-size cache that evicts the least recently used entry.
// Get and Set are O(1): a map finds the list element, and the list keeps
// recency order with the most recent entry at the front.
type LRU[K comparable, V any] struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[K]*list.Element
	now      func() time.Time
	evicted  int
}

type entry[K comparable, V any] struct {
	key     K
	value   V
	expires time.Time // zero means no expiry
}

// NewLRU creates a cache that holds at most capacity entries.
func NewLRU[K comparable, V any](capacity int, now func() time.Time) *LRU[K, V] {
	return &LRU[K, V]{
		capacity: max(capacity, 1),
		ll:       list.New(),
		items:    make(map[K]*list.Element),
		now:      now,
	}
}

// Get returns the value and marks it as recently used. Expired entries are
// removed on read (lazy expiry) and reported as a miss.
func (c *LRU[K, V]) Get(key K) (V, bool) {
	v, _, ok := c.GetWithExpiry(key)
	return v, ok
}

// GetWithExpiry also returns the expiry time, which XFetch needs.
func (c *LRU[K, V]) GetWithExpiry(key K) (V, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero V
	el, ok := c.items[key]
	if !ok {
		return zero, time.Time{}, false
	}
	e := el.Value.(*entry[K, V])
	if !e.expires.IsZero() && !c.now().Before(e.expires) {
		c.removeElement(el)
		return zero, time.Time{}, false
	}
	c.ll.MoveToFront(el)
	return e.value, e.expires, true
}

// Set stores a value. A ttl of zero or less means the entry never expires.
func (c *LRU[K, V]) Set(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var expires time.Time
	if ttl > 0 {
		expires = c.now().Add(ttl)
	}
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry[K, V])
		e.value, e.expires = value, expires
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&entry[K, V]{key: key, value: value, expires: expires})
	if c.ll.Len() > c.capacity {
		c.removeElement(c.ll.Back())
		c.evicted++
	}
}

// Delete removes a key; this is what cache-aside does after a write.
func (c *LRU[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

// Len is the number of stored entries, including expired ones not yet read.
func (c *LRU[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// Evictions counts entries pushed out by capacity, a key cache metric.
func (c *LRU[K, V]) Evictions() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evicted
}

func (c *LRU[K, V]) removeElement(el *list.Element) {
	c.ll.Remove(el)
	delete(c.items, el.Value.(*entry[K, V]).key)
}
