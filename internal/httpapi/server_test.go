package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, url string, into any) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if into != nil {
		if err := json.NewDecoder(res.Body).Decode(into); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return res
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t, Config{})
	var body map[string]string
	if res := getJSON(t, srv.URL+"/health", &body); res.StatusCode != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("status %d body %v", res.StatusCode, body)
	}
}

func TestEstimate(t *testing.T) {
	srv := newTestServer(t, Config{})
	var body struct {
		Estimate struct {
			AvgReadQPS float64 `json:"avg_read_qps"`
		} `json:"estimate"`
	}
	res := getJSON(t, srv.URL+"/v1/estimate?dau=86400000&reads=10", &body)
	if res.StatusCode != http.StatusOK || body.Estimate.AvgReadQPS != 10_000 {
		t.Fatalf("status %d avg_read_qps %v", res.StatusCode, body.Estimate.AvgReadQPS)
	}
	if res := getJSON(t, srv.URL+"/v1/estimate?dau=abc", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad input got %d", res.StatusCode)
	}
}

func TestRouteIsStable(t *testing.T) {
	srv := newTestServer(t, Config{CacheNodes: []string{"n1", "n2", "n3"}})
	var a, b struct {
		Owner    string   `json:"owner"`
		Replicas []string `json:"replicas"`
	}
	getJSON(t, srv.URL+"/v1/route/user:42", &a)
	getJSON(t, srv.URL+"/v1/route/user:42", &b)
	if a.Owner == "" || a.Owner != b.Owner || len(a.Replicas) != 2 || a.Replicas[0] != a.Owner {
		t.Fatalf("unstable or malformed route: %+v vs %+v", a, b)
	}
}

func TestIDs(t *testing.T) {
	srv := newTestServer(t, Config{NodeID: 7})
	var body struct {
		IDs []struct {
			ID    string `json:"id"`
			Parts struct {
				Node int64 `json:"node"`
			} `json:"parts"`
		} `json:"ids"`
	}
	getJSON(t, srv.URL+"/v1/ids?n=5", &body)
	seen := map[string]bool{}
	for _, id := range body.IDs {
		if seen[id.ID] || id.Parts.Node != 7 {
			t.Fatalf("bad id %+v", id)
		}
		seen[id.ID] = true
	}
	if len(seen) != 5 {
		t.Fatalf("got %d ids", len(seen))
	}
	if res := getJSON(t, srv.URL+"/v1/ids?n=1000", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("n=1000 got %d", res.StatusCode)
	}
}

func TestProductCacheAside(t *testing.T) {
	var loads atomic.Int32
	origin := func(ctx context.Context, id string) (Product, error) {
		loads.Add(1)
		if id != "1" {
			return Product{}, ErrNotFound
		}
		return Product{ID: "1", Name: "Keyboard", PriceCents: 100}, nil
	}
	srv := newTestServer(t, Config{Origin: origin})

	first := getJSON(t, srv.URL+"/v1/products/1", nil)
	second := getJSON(t, srv.URL+"/v1/products/1", nil)
	if first.Header.Get("X-Cache") != "MISS" || second.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("X-Cache %q then %q, want MISS then HIT", first.Header.Get("X-Cache"), second.Header.Get("X-Cache"))
	}
	if loads.Load() != 1 {
		t.Fatalf("origin loaded %d times, want 1", loads.Load())
	}
	if res := getJSON(t, srv.URL+"/v1/products/999", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown product got %d", res.StatusCode)
	}
}

func TestProductStampedeIsCoalesced(t *testing.T) {
	var loads atomic.Int32
	origin := func(ctx context.Context, id string) (Product, error) {
		loads.Add(1)
		time.Sleep(200 * time.Millisecond) // a slow query keeps the miss window open
		return Product{ID: id}, nil
	}
	srv := newTestServer(t, Config{Origin: origin, RateBurst: 100})

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			res, err := http.Get(srv.URL + "/v1/products/hot")
			if err != nil {
				t.Error(err)
				return
			}
			res.Body.Close()
		})
	}
	wg.Wait()
	if n := loads.Load(); n != 1 {
		t.Fatalf("20 concurrent misses caused %d origin loads, want 1", n)
	}
}

func TestRateLimitReturns429WithRetryAfter(t *testing.T) {
	srv := newTestServer(t, Config{RateBurst: 2, RatePerSec: 0.5})
	for i := range 2 {
		if res := getJSON(t, srv.URL+"/v1/ids", nil); res.StatusCode != http.StatusOK {
			t.Fatalf("request %d got %d", i+1, res.StatusCode)
		}
	}
	res := getJSON(t, srv.URL+"/v1/ids", nil)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "2" {
		t.Fatalf("got %d Retry-After=%q, want 429 and 2", res.StatusCode, res.Header.Get("Retry-After"))
	}
	// Health checks are never throttled.
	if res := getJSON(t, srv.URL+"/health", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("health got %d while client is throttled", res.StatusCode)
	}
}
