package mcp

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type mcpMetrics struct {
	calls   metric.Int64Counter
	errors  metric.Int64Counter
	latency metric.Float64Histogram
}

var toolMetrics *mcpMetrics

// InitMetrics initializes MCP tool metrics against the global MeterProvider.
// Must be called after telemetry.Init so instruments are registered with the real provider.
func InitMetrics() {
	slog.Debug("initializing mcp metrics")
	meter := otel.Meter("session-manager")
	calls, _ := meter.Int64Counter("session_manager.tool.calls",
		metric.WithDescription("Number of MCP tool calls"))
	errors, _ := meter.Int64Counter("session_manager.tool.errors",
		metric.WithDescription("Number of MCP tool call errors"))
	latency, _ := meter.Float64Histogram("session_manager.tool.latency",
		metric.WithDescription("MCP tool call latency"),
		metric.WithUnit("ms"))
	toolMetrics = &mcpMetrics{calls: calls, errors: errors, latency: latency}
	slog.Debug("mcp metrics initialized")
}

func recordCall(ctx context.Context, tool, userID string, start time.Time, err error) {
	if toolMetrics == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("tool", tool),
		attribute.String("user_id", userID),
	)
	toolMetrics.calls.Add(ctx, 1, attrs)
	toolMetrics.latency.Record(ctx, float64(time.Since(start).Milliseconds()), attrs)
	if err != nil {
		toolMetrics.errors.Add(ctx, 1, attrs)
	}
}
