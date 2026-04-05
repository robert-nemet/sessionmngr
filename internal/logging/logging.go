// Package logging provides structured logging setup for the session-manager MCP server.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/robert-nemet/sessionmngr/internal/config"
)

// Init initializes the global logger with configuration from cfg.
// Sets up file-based or stdout logging depending on configuration.
// Returns error if log file cannot be created.
// Note: Log files are left open and closed by the OS on process exit.
// This is acceptable for MCP servers running as subprocesses.
func Init(cfg config.Config) error {
	logWriter, err := setupLogWriter(cfg)
	if err != nil {
		return err
	}

	handler := slog.NewJSONHandler(logWriter, &slog.HandlerOptions{
		Level: cfg.GetLogLevel(),
	})
	slog.SetDefault(slog.New(handler))

	return nil
}

func setupLogWriter(cfg config.Config) (io.Writer, error) {
	if cfg.IsStdioLogging() {
		return os.Stderr, nil
	}

	logFile, err := setupLogFile(cfg.GetLogLocation())
	if err != nil {
		return nil, err
	}

	// Log file intentionally left open; OS will close on process exit
	return logFile, nil
}

func setupLogFile(logLocation string) (*os.File, error) {
	if err := os.MkdirAll(logLocation, 0755); err != nil {
		return nil, err
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFilePath := filepath.Join(logLocation, fmt.Sprintf("session-manager-%s.log", timestamp))

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return logFile, nil
}
