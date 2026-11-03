// Package httpapi wires the Stage 1-3 building blocks into one small HTTP
// service: a capacity estimator, a consistent-hash router, a Snowflake ID
// endpoint and a cache-aside product lookup, all behind a per-client rate
// limiter.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TheLearningHubOrg/learn-system-design/internal/cache"
	"github.com/TheLearningHubOrg/learn-system-design/internal/capacity"
	"github.com/TheLearningHubOrg/learn-system-design/internal/hashring"
	"github.com/TheLearningHubOrg/learn-system-design/internal/idgen"
	"github.com/TheLearningHubOrg/learn-system-design/internal/ratelimit"
)

// Product is what the slow "database" returns.
type Product struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
}

// ErrNotFound is returned by an Origin for unknown IDs.
var ErrNotFound = errors.New("not found")

// Origin is the source of truth behind the cache, such as a database.
type Origin func(ctx context.Context, id string) (Product, error)

// Config holds everything the server needs. Zero values get defaults.
type Config struct {
	CacheNodes   []string      // physical nodes on the hash ring
	NodeID       int64         // Snowflake node ID for this process
	RateBurst    int           // token-bucket capacity per client
	RatePerSec   float64       // token-bucket refill per client
	CacheSize    int           // LRU entries
	CacheTTL     time.Duration // base TTL before jitter
	Origin       Origin
	Logger       *slog.Logger
	Now          func() time.Time
	TrustProxyIP bool // use X-Forwarded-For; only behind a proxy you control
}

type server struct {
	cfg      Config
	ring     *hashring.Ring
	ids      *idgen.Snowflake
	limiter  *ratelimit.PerKey
	products *cache.LRU[string, Product]
	flight   cache.Group[string, Product]
}

// New builds the handler.
func New(cfg Config) (http.Handler, error) {
	if len(cfg.CacheNodes) == 0 {
		cfg.CacheNodes = []string{"cache-a", "cache-b", "cache-c"}
	}
	if cfg.RateBurst == 0 {
		cfg.RateBurst = 20
	}
	if cfg.RatePerSec == 0 {
		cfg.RatePerSec = 10
	}
	if cfg.CacheSize == 0 {
		cfg.CacheSize = 1000
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 30 * time.Second
	}
	if cfg.Origin == nil {
		cfg.Origin = SlowCatalog(50 * time.Millisecond)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	ids, err := idgen.New(cfg.NodeID, idgen.DefaultEpoch, cfg.Now)
	if err != nil {
		return nil, err
	}
	s := &server{
		cfg:      cfg,
		ring:     hashring.New(160),
		ids:      ids,
		limiter:  ratelimit.NewPerKey(cfg.RateBurst, cfg.RatePerSec, 10_000, cfg.Now),
		products: cache.NewLRU[string, Product](cfg.CacheSize, cfg.Now),
	}
	for _, n := range cfg.CacheNodes {
		s.ring.Add(n)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/estimate", s.estimate)
	mux.HandleFunc("GET /v1/route/{key}", s.route)
	mux.HandleFunc("GET /v1/ids", s.newIDs)
	mux.HandleFunc("GET /v1/products/{id}", s.product)
	return s.rateLimit(mux), nil
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// estimate: /v1/estimate?dau=500000000&reads=20&writes=0.1&object_bytes=200000
func (s *server) estimate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var wl capacity.Workload
	fields := []struct {
		name string
		dst  *float64
	}{
		{"dau", &wl.DailyActiveUsers},
		{"reads", &wl.ReadsPerUserDay},
		{"writes", &wl.WritesPerUserDay},
		{"object_bytes", &wl.ObjectBytes},
		{"peak", &wl.PeakToAverage},
		{"retention_days", &wl.RetentionDays},
		{"replication", &wl.Replication},
	}
	for _, f := range fields {
		raw := q.Get(f.name)
		if raw == "" {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
			writeError(w, http.StatusBadRequest, f.name+" must be a non-negative number")
			return
		}
		*f.dst = v
	}
	if wl.DailyActiveUsers == 0 {
		writeError(w, http.StatusBadRequest, "dau is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workload": wl, "estimate": wl.Estimate()})
}

func (s *server) route(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	writeJSON(w, http.StatusOK, map[string]any{
		"key":      key,
		"owner":    s.ring.GetN(key, 1)[0],
		"replicas": s.ring.GetN(key, 2),
	})
}

func (s *server) newIDs(w http.ResponseWriter, r *http.Request) {
	n := 1
	if raw := r.URL.Query().Get("n"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 100 {
			writeError(w, http.StatusBadRequest, "n must be between 1 and 100")
			return
		}
		n = v
	}
	type idOut struct {
		ID    string      `json:"id"` // a string: JavaScript loses precision above 2^53
		Parts idgen.Parts `json:"parts"`
	}
	out := make([]idOut, 0, n)
	for range n {
		id, err := s.ids.Next()
		if err != nil {
			s.cfg.Logger.Error("id generation failed", "err", err)
			writeError(w, http.StatusServiceUnavailable, "clock skew, retry")
			return
		}
		out = append(out, idOut{strconv.FormatInt(id, 10), idgen.Decode(id, idgen.DefaultEpoch)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": out})
}

// product is cache-aside: read the cache, on a miss load from the origin
// (coalesced so a hot key triggers one load) and populate the cache with a
// jittered TTL.
func (s *server) product(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if p, ok := s.products.Get(id); ok {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, http.StatusOK, p)
		return
	}
	p, err, shared := s.flight.Do(id, func() (Product, error) {
		// Detach from the first caller's cancellation: other callers share
		// this load, so one client hanging up must not fail all of them.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
		defer cancel()
		p, err := s.cfg.Origin(ctx, id)
		if err == nil {
			ttl := cache.JitteredTTL(s.cfg.CacheTTL, 0.1, rand.Float64)
			s.products.Set(id, p, ttl)
		}
		return p, err
	})
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "product not found")
		return
	case err != nil:
		s.cfg.Logger.Error("origin failed", "id", id, "err", err)
		writeError(w, http.StatusBadGateway, "origin unavailable")
		return
	}
	if shared {
		w.Header().Set("X-Cache", "MISS-COALESCED")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, p)
}

// rateLimit applies one token bucket per client IP. Health checks are
// exempt so a busy client cannot get the instance marked unhealthy.
func (s *server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		ok, wait := s.limiter.Allow(s.clientIP(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxyIP {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// The left-most entry is the original client as seen by the
			// first proxy. Only trust it when that proxy is yours.
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// SlowCatalog is a stand-in database with a fixed latency per query.
func SlowCatalog(latency time.Duration) Origin {
	catalog := map[string]Product{
		"1": {ID: "1", Name: "Mechanical keyboard", PriceCents: 8900},
		"2": {ID: "2", Name: "USB-C dock", PriceCents: 12900},
		"3": {ID: "3", Name: "27-inch monitor", PriceCents: 32900},
	}
	return func(ctx context.Context, id string) (Product, error) {
		select {
		case <-time.After(latency):
		case <-ctx.Done():
			return Product{}, ctx.Err()
		}
		p, ok := catalog[id]
		if !ok {
			return Product{}, ErrNotFound
		}
		return p, nil
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
