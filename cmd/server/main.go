// Command server runs the learn-system-design starter service.
//
//	PORT        listen port (default 8080)
//	NODE_ID     Snowflake node ID, 0-1023 (default 1)
//	CACHE_NODES comma-separated nodes for the hash ring
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TheLearningHubOrg/learn-system-design/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	port := envOr("PORT", "8080")
	nodeID, err := strconv.ParseInt(envOr("NODE_ID", "1"), 10, 64)
	if err != nil {
		return err
	}
	var nodes []string
	if raw := os.Getenv("CACHE_NODES"); raw != "" {
		nodes = strings.Split(raw, ",")
	}

	handler, err := httpapi.New(httpapi.Config{CacheNodes: nodes, NodeID: nodeID, Logger: logger})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              "0.0.0.0:" + port, // all interfaces, so the container port is reachable
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second, // slow-loris protection
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// SIGTERM is what Docker and Kubernetes send before killing the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "node_id", nodeID)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	// Stop accepting connections and let in-flight requests finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("shut down cleanly")
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
