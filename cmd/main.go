// Session Manager MCP Server main entry point.
// Initializes configuration, logging, and starts the MCP server with stdio transport.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/logging"
	"github.com/robert-nemet/sessionmngr/internal/mcp"
	"github.com/robert-nemet/sessionmngr/internal/telemetry"
	"github.com/robert-nemet/sessionmngr/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.NewConfig()

	if err := logging.Init(cfg); err != nil {
		panic("failed to initialize logging: " + err.Error())
	}

	shutdown, err := telemetry.Init(ctx, "session-manager", version.Version)
	if err != nil {
		slog.Warn("telemetry init failed, continuing without OTel", "error", err)
		shutdown = func(context.Context) {}
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdown(shutdownCtx)
	}()

	mcp.InitMetrics()

	slog.Debug("session-manager starting", "version", version.Version, "storage", cfg.GetStorageLocation())

	if cfg.IsHTTPTransport() {
		mcp.StartHTTPServer(ctx, cfg)
	} else {
		mcp.StartServer(ctx, cfg)
	}
}
