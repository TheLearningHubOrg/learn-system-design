package cache

import (
	"math"
	"time"
)

// ShouldRefreshEarly is the XFetch test from "Optimal Probabilistic Cache
// Stampede Prevention" (Vattani, Chierichetti, Lowenstein, VLDB 2015).
//
// Each reader independently decides to recompute a value before it expires
// with a probability that rises as expiry approaches and as the value gets
// more expensive to rebuild:
//
//	refresh if now - delta * beta * ln(rnd) >= expiry
//
// delta is how long a recompute takes, beta tunes eagerness (1 is the
// paper's default; above 1 refreshes earlier) and rnd returns a value in
// (0, 1]. Pass func() float64 { return 1 - rand.Float64() } so ln never
// sees zero.
func ShouldRefreshEarly(now, expiry time.Time, delta time.Duration, beta float64, rnd func() float64) bool {
	if !now.Before(expiry) {
		return true
	}
	gap := -float64(delta) * beta * math.Log(rnd()) // >= 0 because ln(rnd) <= 0
	return gap >= float64(expiry.Sub(now))
}

// JitteredTTL spreads expiry times so keys written together do not expire
// together (a cache avalanche). fraction 0.1 gives base +/- 10%. rnd returns
// a value in [0, 1).
func JitteredTTL(base time.Duration, fraction float64, rnd func() float64) time.Duration {
	return time.Duration(float64(base) * (1 + fraction*(2*rnd()-1)))
}
