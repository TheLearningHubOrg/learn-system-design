package capacity

import (
	"math"
	"testing"
	"time"
)

func near(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance*math.Abs(want)
}

func TestPhotoSharingEstimate(t *testing.T) {
	// The Stage 1 worked example: 500M DAU, 20 views each, 10% upload one
	// 200 KB photo a day, peak is twice the average, kept 5 years, 3 copies.
	w := Workload{
		DailyActiveUsers: 500e6,
		ReadsPerUserDay:  20,
		WritesPerUserDay: 0.1,
		ObjectBytes:      200e3,
		PeakToAverage:    2,
		RetentionDays:    5 * 365,
		Replication:      3,
	}
	e := w.Estimate()

	cases := []struct {
		name      string
		got, want float64
	}{
		{"avg read QPS", e.AvgReadQPS, 115_741},
		{"peak read QPS", e.PeakReadQPS, 231_481},
		{"avg write QPS", e.AvgWriteQPS, 579},
		{"ingest per day", e.IngestBytesPerDay, 10e12},
		{"storage", e.StorageBytes, 10e12 * 5 * 365 * 3},
		{"peak egress", e.PeakEgressBitsSec, 231_481 * 200e3 * 8},
	}
	for _, c := range cases {
		if !near(c.got, c.want, 0.001) {
			t.Errorf("%s = %.0f, want about %.0f", c.name, c.got, c.want)
		}
	}
}

func TestEstimateDefaults(t *testing.T) {
	e := Workload{DailyActiveUsers: 86_400, ReadsPerUserDay: 1, WritesPerUserDay: 1, ObjectBytes: 1}.Estimate()
	if e.PeakReadQPS != 2 {
		t.Errorf("default peak factor: got %v, want 2", e.PeakReadQPS)
	}
	if e.StorageBytes != 86_400*365*3 {
		t.Errorf("default retention and replication: got %v", e.StorageBytes)
	}
}

func TestDowntimePerYear(t *testing.T) {
	cases := []struct {
		availability float64
		want         time.Duration
	}{
		{0.99, 87*time.Hour + 39*time.Minute},
		{0.999, 8*time.Hour + 45*time.Minute},
		{0.9999, 52 * time.Minute},
		{0.99999, 5 * time.Minute},
	}
	for _, c := range cases {
		got := DowntimePerYear(c.availability)
		if got < c.want || got > c.want+time.Minute {
			t.Errorf("DowntimePerYear(%v) = %v, want %v to %v", c.availability, got, c.want, c.want+time.Minute)
		}
	}
}

func TestSerialAndParallel(t *testing.T) {
	// LB (99.99%) -> app (99.9%) -> DB (99.95%) in series.
	if got := Serial(0.9999, 0.999, 0.9995); !near(got, 0.99840, 0.00001) {
		t.Errorf("Serial = %.5f", got)
	}
	// Two independent 99% replicas behind a health-checked LB.
	if got := Parallel(0.99, 0.99); !near(got, 0.9999, 1e-9) {
		t.Errorf("Parallel = %.6f", got)
	}
}

func TestLittlesLawAndQueueing(t *testing.T) {
	if got := Concurrency(2000, 150*time.Millisecond); got != 300 {
		t.Errorf("Concurrency = %v, want 300", got)
	}
	if got := QueueingFactor(0.9); !near(got, 10, 1e-9) {
		t.Errorf("QueueingFactor(0.9) = %v, want 10", got)
	}
	if !math.IsInf(QueueingFactor(1), 1) {
		t.Error("QueueingFactor(1) should be +Inf")
	}
}

func TestFanOutSlowProbability(t *testing.T) {
	// One slow call in a hundred, fanned out to 100 backends: 63% of user
	// requests now see at least one p99-slow dependency.
	if got := FanOutSlowProbability(0.01, 100); !near(got, 0.634, 0.001) {
		t.Errorf("FanOutSlowProbability = %.3f, want 0.634", got)
	}
}
