package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"order-api/auth"
	"order-api/db"
	"order-api/handlers"
	"order-api/telemetry"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const serviceName = "order-api"

// The test console (frontend/index.html) is compiled into the binary so
// one static executable serves both UI and API from the same origin —
// which also means no CORS handling is needed anywhere.
//
//go:embed frontend
var frontendFS embed.FS

// version is stamped at build time via
//   -ldflags "-X main.version=$(git rev-parse --short HEAD)"
// and falls back to "dev" for plain `go run`.
var version = "dev"

func main() {
	// Structured logging (slog upgrade riding along with the OTel work).
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(context.Background()); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Config via env only — never hardcode connection strings (the old
	// committed credential was rotated; do not reintroduce secrets).
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		return errors.New("DATABASE_URL environment variable is required")
	}

	// Telemetry first, so the pgx tracer picks up the global provider.
	// No-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset.
	otelShutdown, err := telemetry.Setup(ctx, serviceName, version)
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(flushCtx); err != nil {
			slog.Warn("telemetry shutdown", "error", err)
		}
	}()

	// Connection pool with per-query tracing attached to the pool config.
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return err
	}
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Export pgxpool stats (acquired/idle conns, acquire duration, ...)
	// through the global meter provider.
	if err := otelpgx.RecordStats(pool); err != nil {
		return err
	}

	// Fail fast on unreachable DB instead of erroring on first request.
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		return err
	}

	// Baseline network RTT to the (remote, Neon-hosted) database, so query
	// span durations can be decomposed into network vs. execution time.
	// Gated on real traffic (see the tracker wiring below): Neon is
	// serverless and autosuspends when idle — the probe must never be the
	// thing keeping it awake. No-op unless telemetry is enabled.
	dbActivity := telemetry.NewActivityTracker()
	telemetry.StartDBRTTProbe(ctx, pool.Ping, dbActivity)

	queries := db.New(pool)
	server := &handlers.Server{Queries: queries}

	// Auth, same philosophy as telemetry: no-op unless ZITADEL_DOMAIN is
	// set, fail-fast if half-configured.
	authMW, err := auth.New(ctx)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	// Auth now wraps each API route rather than the whole mux, so the
	// console page itself stays reachable without a token (it's static
	// HTML; the data behind it is still guarded). otelhttp derives the
	// http.route attribute from the ServeMux pattern automatically.
	// The activity tracker wraps the same routes — /orders* are the only
	// handlers that touch the database, so they're exactly the signal that
	// gates the RTT probe (/healthz and the static console don't count).
	protect := func(h http.HandlerFunc) http.Handler {
		return dbActivity.Track(authMW.Require(h))
	}
	mux.Handle("GET /orders", protect(server.GetOrders))
	mux.Handle("GET /orders/{id}", protect(server.GetOrderByID))
	mux.Handle("POST /orders", protect(server.CreateOrder))
	mux.Handle("PUT /orders/{id}", protect(server.UpdateOrder))
	mux.Handle("DELETE /orders/{id}", protect(server.DeleteOrder))

	// Public liveness endpoint for Scaleway health checks — deliberately
	// outside the auth wrapper and filtered out of tracing below.
	mux.Handle("GET /healthz", handlers.Healthz(version))

	// Test console, embedded at build time and served from the same
	// origin as the API.
	consoleFS, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		return err
	}
	mux.Handle("GET /", http.FileServerFS(consoleFS))

	// Middleware order (outermost first): otelhttp -> mux (auth applied
	// per API route above). Rejected requests still produce spans — a
	// burst of 401s is exactly what you want visible in telemetry.
	// /healthz is filtered out: serverless health checks fire constantly
	// and would drown real traffic in the trace view (and the ingest bill).
	handler := otelhttp.NewHandler(mux, serviceName,
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz"
		}),
	)

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: needed so in-flight requests finish and the
	// batching span processor gets flushed on SIGINT/SIGTERM.
	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", httpServer.Addr, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-stopCtx.Done():
		slog.Info("shutdown signal received, draining connections")
		drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(drainCtx)
	}
}
