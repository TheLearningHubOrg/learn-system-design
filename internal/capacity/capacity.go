// Package capacity holds the back-of-the-envelope formulas from Stage 1:
// traffic and storage estimates, availability arithmetic, Little's Law and
// the tail-latency effect of fan-out.
package capacity

import (
	"math"
	"time"
)

// SecondsPerDay is 86,400. Interview arithmetic rounds it to 1e5.
const SecondsPerDay = 86_400

// year uses 365.25 days so leap years are averaged in.
const year = time.Duration(365.25 * 24 * float64(time.Hour))

// Workload describes a product in the units an interviewer usually gives.
type Workload struct {
	DailyActiveUsers float64 `json:"dau"`
	ReadsPerUserDay  float64 `json:"reads_per_user_day"`
	WritesPerUserDay float64 `json:"writes_per_user_day"`
	ObjectBytes      float64 `json:"object_bytes"`    // average size of one written object
	PeakToAverage    float64 `json:"peak_to_average"` // usually 2-5; 0 means 2
	RetentionDays    float64 `json:"retention_days"`  // 0 means 365
	Replication      float64 `json:"replication"`     // copies stored; 0 means 3
}

// Estimate is the result of Workload.Estimate.
type Estimate struct {
	AvgReadQPS        float64 `json:"avg_read_qps"`
	PeakReadQPS       float64 `json:"peak_read_qps"`
	AvgWriteQPS       float64 `json:"avg_write_qps"`
	PeakWriteQPS      float64 `json:"peak_write_qps"`
	IngestBytesPerDay float64 `json:"ingest_bytes_per_day"`
	StorageBytes      float64 `json:"storage_bytes"`        // ingest x retention x replication
	PeakEgressBitsSec float64 `json:"peak_egress_bits_sec"` // peak reads x object size x 8
}

// Estimate turns a workload into request rates, storage and bandwidth.
func (w Workload) Estimate() Estimate {
	peak := orDefault(w.PeakToAverage, 2)
	retention := orDefault(w.RetentionDays, 365)
	copies := orDefault(w.Replication, 3)

	reads := w.DailyActiveUsers * w.ReadsPerUserDay / SecondsPerDay
	writes := w.DailyActiveUsers * w.WritesPerUserDay / SecondsPerDay
	ingest := w.DailyActiveUsers * w.WritesPerUserDay * w.ObjectBytes

	return Estimate{
		AvgReadQPS:        reads,
		PeakReadQPS:       reads * peak,
		AvgWriteQPS:       writes,
		PeakWriteQPS:      writes * peak,
		IngestBytesPerDay: ingest,
		StorageBytes:      ingest * retention * copies,
		PeakEgressBitsSec: reads * peak * w.ObjectBytes * 8,
	}
}

func orDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

// DowntimePerYear converts an availability fraction (0.999) into the outage
// budget it allows over an average year.
func DowntimePerYear(availability float64) time.Duration {
	return time.Duration((1 - availability) * float64(year))
}

// Serial is the availability of components that must all work, such as a
// load balancer in front of an app in front of a database: the product.
func Serial(availabilities ...float64) float64 {
	p := 1.0
	for _, a := range availabilities {
		p *= a
	}
	return p
}

// Parallel is the availability of redundant replicas where any one is
// enough: one minus the chance that every replica is down at once. It
// assumes failures are independent, which shared racks, regions and
// deploys often break.
func Parallel(availabilities ...float64) float64 {
	allDown := 1.0
	for _, a := range availabilities {
		allDown *= 1 - a
	}
	return 1 - allDown
}

// Concurrency applies Little's Law, L = lambda x W: the average number of
// requests in flight for a given arrival rate and time in the system.
func Concurrency(arrivalsPerSec float64, latency time.Duration) float64 {
	return arrivalsPerSec * latency.Seconds()
}

// QueueingFactor is how many times longer than the bare service time a
// request spends in an M/M/1 queue at the given utilisation: 1/(1-rho).
// At 50% busy the wait doubles, at 90% it is ten times. It returns +Inf at
// or above 100%, where the queue grows without bound.
func QueueingFactor(utilisation float64) float64 {
	if utilisation >= 1 {
		return math.Inf(1)
	}
	return 1 / (1 - utilisation)
}

// FanOutSlowProbability is the chance that a request which waits for n
// parallel backend calls sees at least one slow call, when each call is
// slow with probability p (p = 0.01 for "slower than p99").
func FanOutSlowProbability(p float64, n int) float64 {
	return 1 - math.Pow(1-p, float64(n))
}
