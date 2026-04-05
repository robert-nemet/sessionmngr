// Package config provides configuration management for the session-manager MCP server.
// Configuration is loaded from environment variables with sensible defaults.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvStorageLocation is the environment variable for session storage directory.
	EnvStorageLocation = "SESSIONS_MCP_STORAGE"
	// EnvLogLevel is the environment variable for log level (debug, info, warn, error).
	EnvLogLevel = "SESSIONS_MCP_LOG_LEVEL"
	// EnvLogLocation is the environment variable for log file location (or "stdio" for stdout).
	EnvLogLocation = "SESSIONS_MCP_LOG_LOCATION"
	// EnvMaxMessages is the environment variable for maximum messages per sync.
	EnvMaxMessages = "SESSIONS_MCP_MAX_MESSAGES"
	// EnvMaxContentLength is the environment variable for maximum content length per message.
	EnvMaxContentLength = "SESSIONS_MCP_MAX_CONTENT_LENGTH"

	// Storage backend configuration

	// EnvStorageType selects storage backend: "file" (default) or "postgres".
	EnvStorageType = "STORAGE_TYPE"
	// EnvDatabaseURL is the PostgreSQL connection string.
	EnvDatabaseURL = "DATABASE_URL"
	// EnvDefaultUserID is the default user ID for single-user mode (Phase 4.1).
	EnvDefaultUserID = "DEFAULT_USER_ID"

	// Transport configuration

	// EnvTransportType selects transport: "stdio" (default) or "http".
	EnvTransportType = "TRANSPORT_TYPE"
	// EnvHTTPPort is the port for HTTP transport (default: 8080).
	EnvHTTPPort = "HTTP_PORT"

	// Summarizer configuration

	// EnvAnthropicAPIKey is the API key for Anthropic.
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY"
	// EnvOpenAIAPIKey is the API key for OpenAI.
	EnvOpenAIAPIKey = "OPENAI_API_KEY"
	// EnvSummarizerModel is the model to use for summarization.
	EnvSummarizerModel = "SUMMARIZER_MODEL"
	// EnvSummarizerProvider is the AI provider (anthropic or openai).
	EnvSummarizerProvider = "SUMMARIZER_PROVIDER"
	// EnvSummarizerMinMessages is the minimum messages required for summarization.
	EnvSummarizerMinMessages = "SUMMARIZER_MIN_MESSAGES"

	// Limits configuration

	// EnvMaxSessionsPerUser is the maximum number of sessions allowed per user.
	EnvMaxSessionsPerUser = "MAX_SESSIONS_PER_USER"
	// EnvMaxSummariesPerDay is the maximum number of summaries a user can create per day.
	EnvMaxSummariesPerDay = "MAX_SUMMARIES_PER_DAY"

	defaultStorageDir         = "mcp-sessions"
	defaultStorageType        = "file"
	defaultLogLevel           = "info"
	defaultTransportType      = "stdio"
	defaultHTTPPort           = "8080"
	defaultMaxMessages        = 1000
	defaultMaxContentLength   = 100000 // 100KB
	defaultSummarizerModel    = "claude-sonnet-4-20250514"
	defaultSummarizerProvider = "anthropic"
	defaultSummarizerMinMsgs  = 10

	defaultMaxSessionsPerUser = 100
	defaultMaxSummariesPerDay = 10
)

// Config holds the server configuration loaded from environment variables.
type Config struct {
	storageLocation  string
	logLevel         slog.Level
	logLocation      string
	maxMessages      int
	maxContentLength int

	// Storage backend configuration
	storageType   string
	databaseURL   string
	defaultUserID string

	// Transport configuration
	transportType string
	httpPort      string

	// Summarizer configuration
	anthropicAPIKey    string
	openaiAPIKey       string
	summarizerModel    string
	summarizerProvider string
	summarizerMinMsgs  int

	// Limits configuration
	maxSessionsPerUser int
	maxSummariesPerDay int
}

// NewConfig creates a new Config instance by reading environment variables.
// Falls back to sensible defaults if environment variables are not set.
// Panics if home directory cannot be determined.
func NewConfig() Config {
	storageLocation := os.Getenv(EnvStorageLocation)
	if storageLocation == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			panic("could not determine home directory: " + err.Error())
		}
		storageLocation = filepath.Join(homeDir, defaultStorageDir)
	}

	logLevel := parseLogLevel(os.Getenv(EnvLogLevel))

	logLocation := os.Getenv(EnvLogLocation)
	if logLocation == "" {
		logLocation = filepath.Join(storageLocation, "logs")
	}

	maxMessages := parseIntEnv(EnvMaxMessages, defaultMaxMessages)
	maxContentLength := parseIntEnv(EnvMaxContentLength, defaultMaxContentLength)

	// Storage backend configuration
	storageType := os.Getenv(EnvStorageType)
	if storageType == "" {
		storageType = defaultStorageType
	}
	databaseURL := os.Getenv(EnvDatabaseURL)
	defaultUserID := os.Getenv(EnvDefaultUserID)

	// Transport configuration
	transportType := os.Getenv(EnvTransportType)
	if transportType == "" {
		transportType = defaultTransportType
	}
	httpPort := os.Getenv(EnvHTTPPort)
	if httpPort == "" {
		httpPort = defaultHTTPPort
	}

	// Summarizer configuration
	anthropicAPIKey := os.Getenv(EnvAnthropicAPIKey)
	openaiAPIKey := os.Getenv(EnvOpenAIAPIKey)

	summarizerProvider := os.Getenv(EnvSummarizerProvider)
	if summarizerProvider == "" {
		// Auto-detect based on which API key is set
		if anthropicAPIKey != "" {
			summarizerProvider = "anthropic"
		} else if openaiAPIKey != "" {
			summarizerProvider = "openai"
		} else {
			summarizerProvider = defaultSummarizerProvider
		}
	}

	summarizerModel := os.Getenv(EnvSummarizerModel)
	if summarizerModel == "" {
		// Default model based on provider
		if summarizerProvider == "openai" {
			summarizerModel = "gpt-4o"
		} else {
			summarizerModel = defaultSummarizerModel
		}
	}

	summarizerMinMsgs := parseIntEnv(EnvSummarizerMinMessages, defaultSummarizerMinMsgs)

	maxSessionsPerUser := parseIntEnv(EnvMaxSessionsPerUser, defaultMaxSessionsPerUser)
	maxSummariesPerDay := parseIntEnv(EnvMaxSummariesPerDay, defaultMaxSummariesPerDay)

	return Config{
		storageLocation:    storageLocation,
		logLevel:           logLevel,
		logLocation:        logLocation,
		maxMessages:        maxMessages,
		maxContentLength:   maxContentLength,
		storageType:        storageType,
		databaseURL:        databaseURL,
		defaultUserID:      defaultUserID,
		transportType:      transportType,
		httpPort:           httpPort,
		anthropicAPIKey:    anthropicAPIKey,
		openaiAPIKey:       openaiAPIKey,
		summarizerModel:    summarizerModel,
		summarizerProvider: summarizerProvider,
		summarizerMinMsgs:  summarizerMinMsgs,
		maxSessionsPerUser: maxSessionsPerUser,
		maxSummariesPerDay: maxSummariesPerDay,
	}
}

// GetStorageLocation returns the configured session storage directory path.
func (c Config) GetStorageLocation() string {
	return c.storageLocation
}

// GetLogLevel returns the log verbosity level parsed from SESSIONS_MCP_LOG_LEVEL.
// Defaults to Info if unset or invalid.
func (c Config) GetLogLevel() slog.Level {
	return c.logLevel
}

// GetLogLocation returns the configured log file location.
// Returns "stdio" if logging to stdout is configured.
func (c Config) GetLogLocation() string {
	return c.logLocation
}

// IsStdioLogging returns true if logging is configured to stdout instead of file.
func (c Config) IsStdioLogging() bool {
	return strings.ToLower(c.logLocation) == "stdio"
}

// GetMaxMessages returns the maximum number of messages allowed per sync.
func (c Config) GetMaxMessages() int {
	return c.maxMessages
}

// GetMaxContentLength returns the maximum content length per message.
func (c Config) GetMaxContentLength() int {
	return c.maxContentLength
}

// GetStorageType returns the storage backend type ("file" or "postgres").
func (c Config) GetStorageType() string {
	return c.storageType
}

// IsPostgresStorage returns true if Postgres storage is configured.
func (c Config) IsPostgresStorage() bool {
	return strings.ToLower(c.storageType) == "postgres"
}

// GetDatabaseURL returns the PostgreSQL connection string.
func (c Config) GetDatabaseURL() string {
	return c.databaseURL
}

// GetDefaultUserID returns the default user ID for single-user mode.
func (c Config) GetDefaultUserID() string {
	return c.defaultUserID
}

// IsHTTPTransport returns true if HTTP transport is configured.
func (c Config) IsHTTPTransport() bool {
	return strings.ToLower(c.transportType) == "http"
}

// GetHTTPPort returns the port for HTTP transport.
func (c Config) GetHTTPPort() string {
	return c.httpPort
}

// GetAnthropicAPIKey returns the Anthropic API key.
func (c Config) GetAnthropicAPIKey() string {
	return c.anthropicAPIKey
}

// GetOpenAIAPIKey returns the OpenAI API key.
func (c Config) GetOpenAIAPIKey() string {
	return c.openaiAPIKey
}

// GetSummarizerModel returns the model to use for summarization.
func (c Config) GetSummarizerModel() string {
	return c.summarizerModel
}

// GetSummarizerProvider returns the AI provider name (anthropic or openai).
func (c Config) GetSummarizerProvider() string {
	return c.summarizerProvider
}

// GetSummarizerMinMessages returns the minimum messages required for summarization.
func (c Config) GetSummarizerMinMessages() int {
	return c.summarizerMinMsgs
}

// GetMaxSessionsPerUser returns the maximum number of sessions allowed per user.
func (c Config) GetMaxSessionsPerUser() int {
	return c.maxSessionsPerUser
}

// GetMaxSummariesPerDay returns the maximum number of summaries a user can create per day.
func (c Config) GetMaxSummariesPerDay() int {
	return c.maxSummariesPerDay
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseIntEnv(envVar string, defaultVal int) int {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}

	var parsed int
	if _, err := fmt.Sscanf(val, "%d", &parsed); err != nil {
		return defaultVal
	}
	return parsed
}
