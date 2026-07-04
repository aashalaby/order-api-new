// Package telemetry wires OpenTelemetry tracing and metrics for the service.
//
// Design goals (per project conventions):
//   - Fully optional: when OTEL_EXPORTER_OTLP_ENDPOINT is unset, Setup
//     installs nothing and the binary behaves exactly as before. otelhttp
//     and otelpgx then use the default no-op global providers, so their
//     overhead is negligible and no collector is required.
//   - Pure Go: OTLP/HTTP exporters only, so CGO_ENABLED=0 static builds
//     keep working.
//   - Config via env only. The OTLP exporters read the standard
//     OTEL_EXPORTER_OTLP_* variables themselves (endpoint, headers, TLS —
//     an http:// endpoint implies insecure, which is what local dev wants).
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup configures the global TracerProvider, MeterProvider, and context
// propagators, exporting via OTLP/HTTP.
//
// It returns a shutdown function that flushes and stops the providers;
// call it with a bounded context during graceful shutdown. The returned
// shutdown is always safe to call, including in the no-op case.
func Setup(ctx context.Context, serviceName, serviceVersion string) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		slog.Info("telemetry disabled: OTEL_EXPORTER_OTLP_ENDPOINT not set")
		return noop, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),      // honor OTEL_RESOURCE_ATTRIBUTES / OTEL_SERVICE_NAME
		resource.WithTelemetrySDK(), // sdk name/version attributes
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return noop, err
	}

	// --- Traces ---
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, err
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	// --- Metrics ---
	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		// Don't leak the already-started trace pipeline on partial failure.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if serr := tracerProvider.Shutdown(shutdownCtx); serr != nil {
			slog.Warn("shutting down tracer provider after metric setup failure", "error", serr)
		}
		return noop, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("telemetry enabled",
		"otlp_endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		"service_name", serviceName,
	)

	return func(ctx context.Context) error {
		return errors.Join(
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		)
	}, nil
}
