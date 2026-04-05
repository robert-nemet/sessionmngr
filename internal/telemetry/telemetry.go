// Package telemetry sets up OpenTelemetry metrics with Prometheus export.
// Metrics are exposed via /metrics endpoint, scraped by Grafana Alloy.
package telemetry

import (
	"context"
	"fmt"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init sets up OTel metrics with Prometheus exporter.
// Metrics are always initialized — the Prometheus exporter is harmless even if nobody scrapes.
// Call the returned shutdown function on process exit.
func Init(ctx context.Context, serviceName, serviceVersion string) (func(context.Context), error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTel resource: %w", err)
	}

	promExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExporter),
	)
	otel.SetMeterProvider(mp)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Printf("OTel error: %v", err)
	}))

	return func(ctx context.Context) {
		_ = mp.Shutdown(ctx)
	}, nil
}
