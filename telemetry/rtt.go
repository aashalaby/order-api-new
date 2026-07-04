package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// ActivityTracker records the wall-clock time of the most recent real
// database-touching request. Its only job is to gate the RTT probe:
// the database is serverless too (Neon autosuspends when idle), so an
// unconditional keep-alive ping would silently defeat scale-to-zero and
// turn a paused database into an always-on one. The probe therefore
// measures only while traffic is flowing — when the app is idle there is
// no latency to attribute anyway, and Neon is left alone to suspend.
type ActivityTracker struct {
	lastUnixNano atomic.Int64
}

func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{}
}

// Touch marks "a request that (very likely) queried the database just
// happened". Called from HTTP middleware; an atomic store, so the cost
// per request is a few nanoseconds.
func (a *ActivityTracker) Touch() {
	a.lastUnixNano.Store(time.Now().UnixNano())
}

// Track wraps an http.Handler so serving a request counts as activity.
// It touches before serving: even a request that ends up rejected keeps
// the window open briefly, which errs on the side of a few extra probe
// samples rather than missing real traffic.
func (a *ActivityTracker) Track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Touch()
		next.ServeHTTP(w, r)
	})
}

func (a *ActivityTracker) activeWithin(window time.Duration) bool {
	last := a.lastUnixNano.Load()
	if last == 0 {
		return false // never touched — nothing to measure yet
	}
	return time.Since(time.Unix(0, last)) <= window
}

// StartDBRTTProbe periodically measures the round-trip time of a trivial
// database ping and records it as the histogram metric `db.client.rtt`
// (milliseconds) — but only while real traffic has occurred within
// idleWindow, per the ActivityTracker rationale above.
//
// Why this exists: the database is remote (Neon), so every otelpgx query
// span measures *client-observed* latency — network round trip + TLS +
// protocol overhead + actual execution, all in one number. That makes it
// easy to blame the database for time it never spent. A ping executes an
// empty query, so its duration is almost pure network + protocol floor.
// In New Relic (or any OTLP backend):
//
//	query span duration − db.client.rtt (p50) ≈ real server-side work
//
// Compare percentiles, not means: the first sample after Neon resumes
// from autosuspend includes the resume cost and shows up as an outlier —
// which is itself useful to see, just not part of the steady-state floor.
//
// The probe uses the shared pool, so under heavy pool contention a
// sample also includes connection-acquire wait — visible separately via
// the pgxpool metrics from otelpgx.RecordStats, so the signals
// cross-check each other.
//
// Like everything in this package it is a no-op (returns immediately)
// when OTEL_EXPORTER_OTLP_ENDPOINT is unset. The goroutine stops when
// ctx is cancelled.
func StartDBRTTProbe(ctx context.Context, ping func(context.Context) error, activity *ActivityTracker) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return
	}

	hist, err := otel.Meter("order-api/telemetry").Float64Histogram(
		"db.client.rtt",
		metric.WithUnit("ms"),
		metric.WithDescription("Round-trip time of a trivial DB ping: network + protocol floor with ~zero execution time. Sampled only while the service is receiving traffic."),
	)
	if err != nil {
		slog.Warn("db rtt probe disabled: histogram creation failed", "error", err)
		return
	}

	const (
		interval = 30 * time.Second
		// Probe only if a request arrived this recently. Kept well below
		// Neon's default 5-minute autosuspend timeout so the probe can
		// extend the database's wake time by at most idleWindow beyond
		// what real traffic already caused — never indefinitely.
		idleWindow = 2 * time.Minute
	)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if activity != nil && !activity.activeWithin(idleWindow) {
					continue // idle: stay silent, let Neon autosuspend
				}
				pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				start := time.Now()
				err := ping(pingCtx)
				cancel()
				if err != nil {
					// Transient failures are expected (deploys, resumes);
					// log and keep sampling.
					slog.Warn("db rtt probe ping failed", "error", err)
					continue
				}
				hist.Record(ctx, float64(time.Since(start).Microseconds())/1000.0)
			}
		}
	}()

	slog.Info("db rtt probe started", "interval", interval, "idle_window", idleWindow)
}
