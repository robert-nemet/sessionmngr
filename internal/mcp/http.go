package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/storage"
)

// StartHTTPServer initializes and runs the MCP server with HTTP/SSE transport.
// Validates API keys against the database per-request.
func StartHTTPServer(ctx context.Context, cfg config.Config) {
	pool, err := pgxpool.New(ctx, cfg.GetDatabaseURL())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("failed to ping database", "error", err)
		return
	}

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		userID := r.Header.Get("X-User-ID")
		return getServerWithHandlers(newHandlersFromPool(cfg, pool, userID))
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", authMiddleware(pool, mcpHandler))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/metrics", promhttp.Handler())

	addr := ":" + cfg.GetHTTPPort()
	srv := &http.Server{Addr: addr, Handler: mux}

	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		slog.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
		close(shutdownDone)
	}()

	slog.Info("starting HTTP transport", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server error", "error", err)
		return
	}
	<-shutdownDone
	slog.Info("HTTP server stopped")
}

func authMiddleware(pool *pgxpool.Pool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		userID := r.Header.Get("X-User-ID")

		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		if userID == "" {
			http.Error(w, "X-User-ID header required", http.StatusUnauthorized)
			return
		}

		apiKey := strings.TrimPrefix(auth, "Bearer ")
		keyHash := hashAPIKey(apiKey)

		store := storage.NewPostgresStorage(pool, userID)
		valid, err := store.ValidateAPIKey(r.Context(), keyHash, userID)

		if err != nil || !valid {
			slog.Warn("invalid API key attempt", "user_id", userID, "error", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
