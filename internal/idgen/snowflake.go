// Package idgen generates 64-bit, time-ordered unique IDs in the Snowflake
// layout: 1 sign bit, 41 bits of milliseconds since a custom epoch, 10 bits
// of node ID and 12 bits of per-millisecond sequence.
package idgen

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	timeBits = 41
	nodeBits = 10
	seqBits  = 12

	MaxNode = 1<<nodeBits - 1 // 1023
	maxSeq  = 1<<seqBits - 1  // 4095 IDs per millisecond per node
	maxTime = 1<<timeBits - 1 // about 69.7 years of milliseconds
)

// DefaultEpoch is 2024-01-01T00:00:00Z. A recent epoch postpones the year
// the 41-bit timestamp runs out (here, around 2093).
var DefaultEpoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// ErrClockMovedBackwards is returned when the wall clock jumps back, for
// example after an NTP step. Issuing IDs then could repeat an earlier ID.
var ErrClockMovedBackwards = errors.New("idgen: clock moved backwards")

// Snowflake is safe for concurrent use. Run exactly one per node ID.
type Snowflake struct {
	mu     sync.Mutex
	epoch  time.Time
	node   int64
	lastMs int64
	seq    int64
	now    func() time.Time
}

// New validates the node ID. Pass time.Now as now outside tests.
func New(node int64, epoch time.Time, now func() time.Time) (*Snowflake, error) {
	if node < 0 || node > MaxNode {
		return nil, fmt.Errorf("idgen: node %d outside 0..%d", node, MaxNode)
	}
	return &Snowflake{epoch: epoch, node: node, lastMs: -1, now: now}, nil
}

func (s *Snowflake) millis() int64 { return s.now().Sub(s.epoch).Milliseconds() }

// Next returns a new ID. IDs from one generator strictly increase.
func (s *Snowflake) Next() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ms := s.millis()
	switch {
	case ms < s.lastMs:
		return 0, fmt.Errorf("%w by %dms", ErrClockMovedBackwards, s.lastMs-ms)
	case ms > maxTime:
		return 0, errors.New("idgen: 41-bit timestamp exhausted; move the epoch")
	case ms == s.lastMs:
		s.seq = (s.seq + 1) & maxSeq
		if s.seq == 0 {
			// 4,096 IDs already issued this millisecond: wait for the next one.
			for ms <= s.lastMs {
				ms = s.millis()
			}
		}
	default:
		s.seq = 0
	}
	s.lastMs = ms
	return ms<<(nodeBits+seqBits) | s.node<<seqBits | s.seq, nil
}

// Parts is a decoded ID.
type Parts struct {
	Time     time.Time `json:"time"`
	Node     int64     `json:"node"`
	Sequence int64     `json:"sequence"`
}

// Decode splits an ID back into its fields, which is how you read the
// creation time of any row keyed by a Snowflake ID without a lookup.
func Decode(id int64, epoch time.Time) Parts {
	return Parts{
		Time:     epoch.Add(time.Duration(id>>(nodeBits+seqBits)) * time.Millisecond).UTC(),
		Node:     (id >> seqBits) & MaxNode,
		Sequence: id & maxSeq,
	}
}
