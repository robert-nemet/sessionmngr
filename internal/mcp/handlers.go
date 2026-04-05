// Package mcp implements the Model Context Protocol server with session management handlers.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
	"github.com/robert-nemet/sessionmngr/internal/storage"
	"github.com/robert-nemet/sessionmngr/internal/summarizer"
	"github.com/robert-nemet/sessionmngr/internal/version"
)

var validTag = regexp.MustCompile(`^[a-z0-9_-]+$`)

// sanitizeTag lowercases and trims a tag, then validates it.
func sanitizeTag(t string) (string, error) {
	t = strings.ToLower(strings.TrimSpace(t))
	if len(t) < 4 {
		return "", fmt.Errorf("tag %q must be at least 4 characters", t)
	}
	if !validTag.MatchString(t) {
		return "", fmt.Errorf("tag %q must contain only a-z, 0-9, -, _", t)
	}
	return t, nil
}

// Handlers manages MCP tool handlers with storage backend.
type Handlers struct {
	cfg        config.Config
	storage    storage.Storage
	pool       *pgxpool.Pool          // nil for file storage
	summarizer *summarizer.Summarizer // nil if API keys not configured
	userID     string                 // for metrics attribution
}

// NewHandlers creates a new Handlers instance with appropriate storage backend.
// Uses PostgresStorage if STORAGE_TYPE=postgres, otherwise BasicStorage.
func NewHandlers(cfg config.Config) *Handlers {
	if cfg.IsPostgresStorage() {
		return newPostgresHandlers(cfg)
	}

	store := storage.NewBasicStorage(cfg)

	// Initialize summarizer (optional - may be nil if API keys not configured)
	sum, err := summarizer.New(store, cfg, "", "local")
	if err != nil {
		slog.Warn("summarizer not available - summary tools will return errors",
			"error", err,
			"hint", "set ANTHROPIC_API_KEY or OPENAI_API_KEY to enable summarization")
		sum = nil
	} else {
		slog.Info("summarizer initialized",
			"provider", cfg.GetSummarizerProvider(),
			"model", cfg.GetSummarizerModel())
	}

	return &Handlers{
		cfg:        cfg,
		storage:    store,
		summarizer: sum,
		userID:     "local",
	}
}

// newPostgresHandlers creates handlers with PostgresStorage backend.
func newPostgresHandlers(cfg config.Config) *Handlers {
	dbURL := cfg.GetDatabaseURL()
	if dbURL == "" {
		slog.Error("DATABASE_URL is required when STORAGE_TYPE=postgres")
		panic("DATABASE_URL is required when STORAGE_TYPE=postgres")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		panic("failed to connect to database: " + err.Error())
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("failed to ping database", "error", err)
		panic("failed to ping database: " + err.Error())
	}

	userID := cfg.GetDefaultUserID()
	if userID == "" {
		slog.Error("DEFAULT_USER_ID is required when STORAGE_TYPE=postgres")
		panic("DEFAULT_USER_ID is required when STORAGE_TYPE=postgres")
	}

	slog.Info("using PostgreSQL storage", "user_id", userID)

	store := storage.NewPostgresStorage(pool, userID)

	// Initialize summarizer (optional - may be nil if API keys not configured)
	sum, err := summarizer.New(store, cfg, "", userID)
	if err != nil {
		slog.Warn("summarizer not available - summary tools will return errors",
			"error", err,
			"hint", "set ANTHROPIC_API_KEY or OPENAI_API_KEY to enable summarization")
		sum = nil
	} else {
		slog.Info("summarizer initialized",
			"provider", cfg.GetSummarizerProvider(),
			"model", cfg.GetSummarizerModel())
	}

	return &Handlers{
		cfg:        cfg,
		storage:    store,
		pool:       pool,
		summarizer: sum,
		userID:     userID,
	}
}

// newHandlersFromPool creates handlers using an existing DB pool scoped to a user ID.
// Used by HTTP transport where pool is created once and user ID comes from each request.
func newHandlersFromPool(cfg config.Config, pool *pgxpool.Pool, userID string) *Handlers {
	store := storage.NewPostgresStorage(pool, userID)

	sum, err := summarizer.New(store, cfg, "", userID)
	if err != nil {
		sum = nil
	}

	return &Handlers{
		cfg:        cfg,
		storage:    store,
		pool:       nil, // pool is owned by the HTTP server, not these handlers
		summarizer: sum,
		userID:     userID,
	}
}

// Close releases resources (database connections).
func (h *Handlers) Close() {
	if h.pool != nil {
		h.pool.Close()
	}
}

func (h *Handlers) startSession(ctx context.Context, _ *mcp.CallToolRequest, args *StartSessionInput) (result *mcp.CallToolResult, out *StartSessionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "start-session", h.userID, start, err) }()
	slog.Debug("startSession called", "title", args.Title)

	// Validate title length
	const maxTitleLength = 1000
	if len(args.Title) > maxTitleLength {
		return nil, nil, fmt.Errorf("title length %d exceeds maximum of %d", len(args.Title), maxTitleLength)
	}

	// Check for existing session with same title (dedup)
	existingSess, _ := h.storage.FindSessionByTitle(ctx, args.Title)
	if existingSess != nil {
		slog.Info("continuing existing session", "uuid", existingSess.UUID, "short_id", existingSess.ShortID(), "title", existingSess.Title)
		return nil, &StartSessionOutput{
			SessionID:  existingSess.ShortID(),
			Title:      existingSess.Title,
			CreatedAt:  existingSess.CreatedAt.Format(time.RFC3339),
			IsExisting: true,
		}, nil
	}

	// Check session limit before creating
	maxSessions := h.cfg.GetMaxSessionsPerUser()
	_, _, totalSessions, countErr := h.storage.ListSessions(ctx, 1, 1, nil)
	if countErr != nil {
		return nil, nil, fmt.Errorf("failed to check session count: %w", countErr)
	}
	if totalSessions >= maxSessions {
		return nil, nil, fmt.Errorf("session limit reached (%d/%d). Delete unused sessions to create new ones", totalSessions, maxSessions)
	}

	// Create new session
	sess := session.NewSession(args.Title)
	savedSess, err := h.storage.SaveSession(ctx, sess)
	if err != nil {
		slog.Error("failed to save new session", "error", err)
		return nil, nil, fmt.Errorf("failed to save session: %w", err)
	}

	slog.Info("session created", "uuid", savedSess.UUID, "short_id", savedSess.ShortID(), "title", savedSess.Title)

	return nil, &StartSessionOutput{
		SessionID:  savedSess.ShortID(),
		Title:      savedSess.Title,
		CreatedAt:  savedSess.CreatedAt.Format(time.RFC3339),
		IsExisting: false,
	}, nil
}

const recentMessageCount = 20

func (h *Handlers) switchSession(ctx context.Context, _ *mcp.CallToolRequest, args *SwitchSessionInput) (result *mcp.CallToolResult, out *SwitchSessionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "switch-session", h.userID, start, err) }()
	if args.SessionID == "" {
		return nil, nil, fmt.Errorf("session_id is required")
	}

	// Find session by prefix
	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, err
	}

	// Get recent messages (last N or all if fewer)
	recentMessages := sess.Messages
	if len(recentMessages) > recentMessageCount {
		recentMessages = recentMessages[len(recentMessages)-recentMessageCount:]
	}

	// Find existing resume summary
	var resume string
	if resumeSummary := h.findResumeSummary(ctx, sess.UUID); resumeSummary != nil {
		resume = resumeSummary.Content.Markdown
	}

	slog.Info("switched to session", "uuid", sess.UUID, "title", sess.Title,
		"message_count", len(sess.Messages), "has_resume", resume != "")

	return nil, &SwitchSessionOutput{
		SessionID:      sess.ShortID(),
		Title:          sess.Title,
		Resume:         resume,
		RecentMessages: recentMessages,
		MessageCount:   len(sess.Messages),
		LastUpdatedAt:  sess.LastUpdatedAt.Format(time.RFC3339),
	}, nil
}

// findResumeSummary returns the resume summary for a session, or nil if none exists.
func (h *Handlers) findResumeSummary(ctx context.Context, sessionID string) *storage.Summary {
	summaries, err := h.storage.LoadAllSummaries(ctx, sessionID)
	if err != nil {
		slog.Warn("failed to load summaries for resume", "error", err, "session_id", sessionID)
		return nil
	}
	for i := range summaries {
		if summaries[i].PromptSource == "type:resume" {
			return &summaries[i]
		}
	}
	return nil
}

const (
	resumeStaleThreshold = 150
	resumeTimeout        = 2 * time.Minute
	resumeMaxRetries     = 3
)

// resumeInFlight tracks sessions with active background resume generation.
// Prevents concurrent compaction for the same session.
var resumeInFlight sync.Map

// maybeGenerateResume checks if a resume summary needs regeneration and triggers it in the background.
// A resume is stale if there are 150+ new messages since the last resume (or no resume exists and session has 150+ messages).
// Per-session concurrency guard prevents duplicate work. Retries with exponential backoff on failure.
func (h *Handlers) maybeGenerateResume(sess *session.Session) {
	if h.summarizer == nil {
		return
	}

	messageCount := len(sess.Messages)
	if messageCount < resumeStaleThreshold {
		return
	}

	// Per-session concurrency guard
	if _, loaded := resumeInFlight.LoadOrStore(sess.UUID, true); loaded {
		slog.Debug("resume generation already in progress", "session_id", safeShortID(sess.UUID))
		return
	}

	go func() {
		defer resumeInFlight.Delete(sess.UUID)

		ctx, cancel := context.WithTimeout(context.Background(), resumeTimeout)
		defer cancel()

		resume := h.findResumeSummary(ctx, sess.UUID)

		if resume != nil && (messageCount-resume.MessageRange.End) < resumeStaleThreshold {
			return // resume is fresh enough
		}

		slog.Info("background resume generation",
			"session_id", safeShortID(sess.UUID),
			"message_count", messageCount,
			"incremental", resume != nil)

		backoff := time.Second
		for attempt := 1; attempt <= resumeMaxRetries; attempt++ {
			_, err := h.summarizer.CompactResume(ctx, sess.UUID)
			if err == nil {
				slog.Info("background resume generated",
					"session_id", safeShortID(sess.UUID),
					"message_count", messageCount,
					"attempt", attempt)
				return
			}

			slog.Warn("background resume generation failed",
				"error", err,
				"session_id", safeShortID(sess.UUID),
				"attempt", attempt,
				"max_retries", resumeMaxRetries)

			if attempt < resumeMaxRetries {
				select {
				case <-ctx.Done():
					slog.Warn("background resume generation timed out",
						"session_id", safeShortID(sess.UUID))
					return
				case <-time.After(backoff):
					backoff *= 2
				}
			}
		}
	}()
}

func (h *Handlers) loadCompleteSession(ctx context.Context, _ *mcp.CallToolRequest, args *LoadCompleteSessionInput) (result *mcp.CallToolResult, out *LoadCompleteSessionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "load-complete-session", h.userID, start, err) }()
	if args.SessionID == "" {
		return nil, nil, fmt.Errorf("session_id is required")
	}

	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, err
	}

	slog.Info("loaded complete session", "uuid", sess.UUID, "title", sess.Title,
		"message_count", len(sess.Messages))

	return nil, &LoadCompleteSessionOutput{
		SessionID:     sess.ShortID(),
		Title:         sess.Title,
		Messages:      sess.Messages,
		MessageCount:  len(sess.Messages),
		LastUpdatedAt: sess.LastUpdatedAt.Format(time.RFC3339),
	}, nil
}

func (h *Handlers) syncConversation(ctx context.Context, _ *mcp.CallToolRequest, args *SyncConversationInput) (result *mcp.CallToolResult, out *SyncConversationOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "sync-conversation", h.userID, start, err) }()
	slog.Debug("syncConversation called", "session_id", args.SessionID, "message_count", len(args.Messages))

	// Debug: Log payload metadata (no user content for privacy)
	if len(args.Messages) > 0 {
		lastMsg := args.Messages[len(args.Messages)-1]
		slog.Debug("last message metadata",
			"role", lastMsg.Role,
			"content_length", len(lastMsg.Content))
	}

	// Validate input
	if args.SessionID == "" {
		return nil, nil, fmt.Errorf("session_id is required")
	}

	if len(args.Messages) == 0 {
		return nil, nil, fmt.Errorf("cannot sync empty message array")
	}

	// Validate message limits (prevent DoS and stdio buffer issues)
	maxMessages := h.cfg.GetMaxMessages()
	maxContentLength := h.cfg.GetMaxContentLength()

	if len(args.Messages) > maxMessages {
		return nil, nil, fmt.Errorf("message count %d exceeds maximum of %d", len(args.Messages), maxMessages)
	}

	// Validate messages
	for i, msg := range args.Messages {
		if strings.TrimSpace(msg.Content) == "" {
			return nil, nil, fmt.Errorf("message %d has empty content", i)
		}
		if len(msg.Content) > maxContentLength {
			return nil, nil, fmt.Errorf("message %d content length %d exceeds maximum of %d", i, len(msg.Content), maxContentLength)
		}
	}

	// Load session from disk
	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find session: %w", err)
	}

	// Validate message count - reject if shrinking (data loss prevention)
	currentCount := len(sess.Messages)
	incomingCount := len(args.Messages)

	if incomingCount < currentCount {
		return nil, nil, fmt.Errorf(
			"sync rejected: incoming message count (%d) is less than current count (%d). "+
				"Please resend ALL messages from your conversation context",
			incomingCount, currentCount)
	}

	slog.Debug("message count validation passed",
		"uuid", sess.UUID,
		"current_count", currentCount,
		"incoming_count", incomingCount)

	sess.Messages = args.Messages
	sess.LastUpdatedAt = time.Now()

	// Save immediately to disk (session has UUID, so this is an update)
	savedSess, err := h.storage.SaveSession(ctx, sess)
	if err != nil {
		slog.Error("failed to save session", "error", err, "uuid", sess.UUID)
		return nil, nil, fmt.Errorf("failed to save session: %w", err)
	}

	slog.Info("synced conversation",
		"uuid", savedSess.UUID,
		"message_count", len(savedSess.Messages),
		"previous_count", currentCount,
		"delta", len(savedSess.Messages)-currentCount)

	// Trigger background resume generation if stale
	h.maybeGenerateResume(savedSess)

	return nil, &SyncConversationOutput{
		SessionID:     savedSess.ShortID(),
		SyncedCount:   len(savedSess.Messages),
		LastUpdatedAt: savedSess.LastUpdatedAt.Format(time.RFC3339),
	}, nil
}

func (h *Handlers) listSessions(ctx context.Context, _ *mcp.CallToolRequest, args *ListSessionsInput) (result *mcp.CallToolResult, out *ListSessionsOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "list-sessions", h.userID, start, err) }()
	page := 1
	if args != nil && args.Page > 0 {
		page = args.Page
	}

	perPage := 10
	var filterTags []string
	if args != nil {
		filterTags = args.Tags
	}
	sessions, totalPages, totalSessions, err := h.storage.ListSessions(ctx, page, perPage, filterTags)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Convert to output format with short IDs
	summaries := make([]SessionSummaryOutput, len(sessions))
	for i, sess := range sessions {
		summaries[i] = SessionSummaryOutput{
			SessionID:     safeShortID(sess.UUID),
			Title:         sess.Title,
			MessageCount:  sess.MessageCount,
			SummaryCount:  sess.SummaryCount,
			Tags:          sess.Tags,
			LastUpdatedAt: sess.LastUpdatedAt.Format(time.RFC3339),
		}
	}

	return nil, &ListSessionsOutput{
		Sessions:      summaries,
		Page:          page,
		TotalPages:    totalPages,
		TotalSessions: totalSessions,
	}, nil
}

func (h *Handlers) getVersion(ctx context.Context, _ *mcp.CallToolRequest, _ any) (result *mcp.CallToolResult, out *VersionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "session-manager-version", h.userID, start, err) }()
	slog.Info("version requested", "version", version.Version, "build_timestamp", version.BuildTimestamp)
	return nil, &VersionOutput{
		Version:        version.Version,
		BuildTimestamp: version.BuildTimestamp,
	}, nil
}

// ============================================================================
// Summarizer Tool Handlers
// ============================================================================

// Validation constants
const (
	minSessionIDLength = 8
	minSummaryIDLength = 8
)

// safeShortID returns the first 8 characters of an ID, or the full ID if shorter.
// Prevents panic from unsafe string slicing.
func safeShortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// validateSessionID validates that a session ID meets minimum length requirements.
func validateSessionID(sessionID string) error {
	if len(sessionID) < minSessionIDLength {
		return fmt.Errorf("session_id must be at least %d characters", minSessionIDLength)
	}
	return nil
}

// validateSummaryID validates that a summary ID meets minimum length requirements.
func validateSummaryID(summaryID string) error {
	if len(summaryID) < minSummaryIDLength {
		return fmt.Errorf("summary_id must be at least %d characters", minSummaryIDLength)
	}
	return nil
}

// validateMessageRange validates start_msg and end_msg parameters.
// Both values must be non-negative, and if both are provided, start must be less than end.
func validateMessageRange(startMsg, endMsg int) error {
	if startMsg < 0 {
		return fmt.Errorf("start_msg must be non-negative, got %d", startMsg)
	}
	if endMsg < 0 {
		return fmt.Errorf("end_msg must be non-negative, got %d", endMsg)
	}
	if startMsg > 0 && endMsg > 0 && startMsg >= endMsg {
		return fmt.Errorf("start_msg (%d) must be less than end_msg (%d)", startMsg, endMsg)
	}
	return nil
}

func (h *Handlers) summarizeSession(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *SummarizeSessionInput,
) (result *mcp.CallToolResult, out *SummarizeSessionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "summarize-session", h.userID, start, err) }()
	// Check summarizer available
	if h.summarizer == nil {
		return nil, nil, fmt.Errorf("summarizer not configured - set ANTHROPIC_API_KEY or OPENAI_API_KEY")
	}

	// Validate session ID
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}

	// Validate message range (fail fast on obviously invalid ranges)
	if err := validateMessageRange(args.StartMsg, args.EndMsg); err != nil {
		return nil, nil, err
	}

	// Check daily summary limit
	maxSummaries := h.cfg.GetMaxSummariesPerDay()
	summariesToday, err := h.storage.CountSummariesToday(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check summary limit: %w", err)
	}
	if summariesToday >= maxSummaries {
		return nil, nil, fmt.Errorf("daily summary limit reached (%d/%d). Resets at midnight UTC", summariesToday, maxSummaries)
	}

	// Load session to get message count for progress estimation
	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session not found: %w", err)
	}

	// Determine range and log progress info
	var startMsg, endMsg int
	if args.StartMsg > 0 || args.EndMsg > 0 {
		startMsg = args.StartMsg
		endMsg = args.EndMsg
		if endMsg == 0 {
			endMsg = len(sess.Messages)
		}
	} else {
		startMsg = 0
		endMsg = len(sess.Messages)
	}

	messageCount := endMsg - startMsg
	estimatedTokens := messageCount * 400 // Rough estimate: 400 tokens per message

	slog.Info("starting summarization",
		"session_id", args.SessionID,
		"message_count", messageCount,
		"estimated_input_tokens", estimatedTokens,
		"model", h.cfg.GetSummarizerModel(),
		"note", "this may take 30-60 seconds for large sessions")

	// Call summarizer
	// When type is set, range is ignored — typed summaries always use the full session.
	var summary *storage.Summary
	if args.Type != "" {
		slog.Info("calling AI API with prompt type", "type", args.Type)
		summary, err = h.summarizer.SummarizeSessionWithType(ctx, args.SessionID, args.Type)
	} else if args.StartMsg > 0 || args.EndMsg > 0 {
		slog.Info("calling AI API for message range",
			"start", args.StartMsg,
			"end", args.EndMsg)
		summary, err = h.summarizer.SummarizeRange(ctx, args.SessionID, args.StartMsg, args.EndMsg)
	} else {
		slog.Info("calling AI API for full session")
		summary, err = h.summarizer.SummarizeSession(ctx, args.SessionID)
	}

	if err != nil {
		slog.Error("summarization failed", "error", err, "session_id", args.SessionID)
		return nil, nil, fmt.Errorf("summarization failed: %w", err)
	}

	slog.Info("summary generated successfully",
		"session_id", safeShortID(summary.SessionID),
		"summary_id", safeShortID(summary.ID),
		"model", summary.Model,
		"title", summary.Content.Title)

	// Track usage (best-effort — don't fail the request if tracking fails)
	if trackErr := h.storage.TrackSummary(ctx); trackErr != nil {
		slog.Warn("failed to track summary usage", "error", trackErr)
	}

	// Return summary
	output := &SummarizeSessionOutput{
		SummaryID: safeShortID(summary.ID),
		Title:     summary.Content.Title,
		CreatedAt: summary.CreatedAt.Format(time.RFC3339),
		Model:     summary.Model,
		Markdown:  summary.Content.Markdown,
	}

	return &mcp.CallToolResult{}, output, nil
}

func (h *Handlers) listSummaries(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *ListSummariesInput,
) (result *mcp.CallToolResult, out *ListSummariesOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "list-summaries", h.userID, start, err) }()
	// Validate session ID
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}

	// Find session by prefix
	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session not found: %w", err)
	}

	// List summaries
	summaries, err := h.storage.ListSummaries(ctx, sess.UUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list summaries: %w", err)
	}

	// Convert to output format
	entries := make([]SummaryEntryOutput, len(summaries))
	for i, sum := range summaries {
		entries[i] = SummaryEntryOutput{
			SummaryID: safeShortID(sum.ID),
			Title:     sum.Title,
			CreatedAt: sum.CreatedAt.Format(time.RFC3339),
			Model:     sum.Model,
		}
	}

	output := &ListSummariesOutput{
		Summaries: entries,
		Count:     len(entries),
	}

	slog.Info("listed summaries",
		"session_id", safeShortID(sess.UUID),
		"count", len(summaries))

	return &mcp.CallToolResult{}, output, nil
}

func (h *Handlers) getSummary(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *GetSummaryInput,
) (result *mcp.CallToolResult, out *GetSummaryOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "get-summary", h.userID, start, err) }()
	// Validate IDs
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}
	if err := validateSummaryID(args.SummaryID); err != nil {
		return nil, nil, err
	}

	// Check summarizer available
	if h.summarizer == nil {
		return nil, nil, fmt.Errorf("summarizer not configured - set ANTHROPIC_API_KEY or OPENAI_API_KEY")
	}

	// Use summarizer's LoadSummary which handles prefix resolution
	summary, err := h.summarizer.LoadSummary(ctx, args.SessionID, args.SummaryID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load summary: %w", err)
	}

	output := &GetSummaryOutput{
		SummaryID: safeShortID(summary.ID),
		Title:     summary.Content.Title,
		Markdown:  summary.Content.Markdown,
		CreatedAt: summary.CreatedAt.Format(time.RFC3339),
		Model:     summary.Model,
	}

	slog.Info("retrieved summary",
		"session_id", safeShortID(summary.SessionID),
		"summary_id", safeShortID(summary.ID))

	return &mcp.CallToolResult{}, output, nil
}

func (h *Handlers) exportConversation(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *ExportConversationInput,
) (result *mcp.CallToolResult, out *ExportConversationOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "export-conversation", h.userID, start, err) }()
	// Validate session ID
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}

	// Find session by prefix
	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session not found: %w", err)
	}

	// Build export data with versioning
	exportData := map[string]any{
		"version":     "1.0",
		"session":     sess,
		"exported_at": time.Now().Format(time.RFC3339),
	}

	// Include summaries (default: true if not specified)
	includeSummaries := true
	if args.IncludeSummaries != nil {
		includeSummaries = *args.IncludeSummaries
	}

	if includeSummaries {
		summaries, err := h.storage.LoadAllSummaries(ctx, sess.UUID)
		if err != nil {
			slog.Warn("failed to load summaries for export", "error", err)
			// Continue without summaries
		} else {
			exportData["summaries"] = summaries
			slog.Info("included summaries in export", "count", len(summaries))
		}
	}

	// Serialize to JSON
	jsonBytes, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize export: %w", err)
	}

	output := &ExportConversationOutput{
		Export: string(jsonBytes),
	}

	slog.Info("exported conversation",
		"session_id", safeShortID(sess.UUID),
		"message_count", len(sess.Messages),
		"include_summaries", includeSummaries)

	return &mcp.CallToolResult{}, output, nil
}

func (h *Handlers) deleteSession(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *DeleteSessionInput,
) (result *mcp.CallToolResult, out *DeleteSessionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "delete-session", h.userID, start, err) }()
	// Validate session ID
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}

	// Find session by prefix
	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session not found: %w", err)
	}

	// List summaries before deletion (for count)
	summaries, err := h.storage.ListSummaries(ctx, sess.UUID)
	summariesDeleted := 0
	if err == nil {
		summariesDeleted = len(summaries)
	}

	// Delete the session (which also deletes all summaries)
	if err := h.storage.DeleteSession(ctx, sess.UUID); err != nil {
		return nil, nil, fmt.Errorf("failed to delete session: %w", err)
	}

	output := &DeleteSessionOutput{
		SessionID:        safeShortID(sess.UUID),
		Title:            sess.Title,
		SummariesDeleted: summariesDeleted,
	}

	slog.Info("deleted session",
		"session_id", safeShortID(sess.UUID),
		"title", sess.Title,
		"summaries_deleted", summariesDeleted)

	return &mcp.CallToolResult{}, output, nil
}

// ============================================================================
// Incremental Sync Handlers (Phase 4.3)
// ============================================================================

// messageHash returns the first 16 hex characters of SHA-256(content).
func messageHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

func (h *Handlers) sessionStatus(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *SessionStatusInput,
) (result *mcp.CallToolResult, out *SessionStatusOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "session-status", h.userID, start, err) }()
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}

	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session not found: %w", err)
	}

	output := &SessionStatusOutput{
		SessionID:    safeShortID(sess.UUID),
		MessageCount: len(sess.Messages),
		LastSyncedAt: sess.LastUpdatedAt.Format(time.RFC3339),
		Tags:         sess.Tags,
	}

	if len(sess.Messages) > 0 {
		lastMsg := sess.Messages[len(sess.Messages)-1]
		output.LastMessageHash = messageHash(lastMsg.Content)
		output.LastMessageRole = lastMsg.Role
	}

	slog.Info("session status",
		"session_id", safeShortID(sess.UUID),
		"message_count", len(sess.Messages))

	return nil, output, nil
}

func (h *Handlers) appendMessages(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *AppendMessagesInput,
) (result *mcp.CallToolResult, out *AppendMessagesOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "append-messages", h.userID, start, err) }()
	if args.SessionID == "" {
		return nil, nil, fmt.Errorf("session_id is required")
	}

	if len(args.Messages) == 0 {
		return nil, nil, fmt.Errorf("messages must not be empty")
	}

	// Validate message limits
	maxMessages := h.cfg.GetMaxMessages()
	maxContentLength := h.cfg.GetMaxContentLength()

	for i, msg := range args.Messages {
		if strings.TrimSpace(msg.Content) == "" {
			return nil, nil, fmt.Errorf("message %d has empty content", i)
		}
		if len(msg.Content) > maxContentLength {
			return nil, nil, fmt.Errorf("message %d content length %d exceeds maximum of %d", i, len(msg.Content), maxContentLength)
		}
	}

	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find session: %w", err)
	}

	currentCount := len(sess.Messages)
	hasGap := false

	if args.AfterCount != nil {
		// Validated append: check continuity
		if currentCount != *args.AfterCount {
			return nil, nil, fmt.Errorf(
				"append rejected: expected %d messages on server, but server has %d. "+
					"Use sync-conversation for full replace, or omit after_count for lossy append",
				*args.AfterCount, currentCount)
		}
	} else if currentCount > 0 {
		// Lossy append: insert gap marker
		gapMarker := session.Message{
			Role:    "system",
			Content: "[gap: messages may be missing between this point and the previous message]",
		}
		sess.Messages = append(sess.Messages, gapMarker)
		hasGap = true
	}

	// Check total won't exceed max
	newTotal := len(sess.Messages) + len(args.Messages)
	if newTotal > maxMessages {
		return nil, nil, fmt.Errorf("total message count %d would exceed maximum of %d", newTotal, maxMessages)
	}

	sess.Messages = append(sess.Messages, args.Messages...)
	sess.LastUpdatedAt = time.Now()

	savedSess, err := h.storage.SaveSession(ctx, sess)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to save session: %w", err)
	}

	appendedCount := len(args.Messages)
	totalCount := len(savedSess.Messages)

	slog.Info("appended messages",
		"session_id", safeShortID(savedSess.UUID),
		"appended", appendedCount,
		"total", totalCount,
		"has_gap", hasGap)

	// Trigger background resume generation if stale
	h.maybeGenerateResume(savedSess)

	return nil, &AppendMessagesOutput{
		SessionID:     safeShortID(savedSess.UUID),
		AppendedCount: appendedCount,
		TotalCount:    totalCount,
		LastUpdatedAt: savedSess.LastUpdatedAt.Format(time.RFC3339),
		HasGap:        hasGap,
	}, nil
}

func (h *Handlers) tagSession(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args *TagSessionInput,
) (result *mcp.CallToolResult, out *TagSessionOutput, err error) {
	start := time.Now()
	defer func() { recordCall(ctx, "tag-session", h.userID, start, err) }()
	if err := validateSessionID(args.SessionID); err != nil {
		return nil, nil, err
	}
	if len(args.Add) == 0 && len(args.Remove) == 0 {
		return nil, nil, fmt.Errorf("at least one of add or remove must be provided")
	}
	for i, t := range args.Add {
		sanitized, err := sanitizeTag(t)
		if err != nil {
			return nil, nil, err
		}
		args.Add[i] = sanitized
	}

	sess, err := h.storage.FindSessionByPrefix(ctx, args.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session not found: %w", err)
	}

	// Apply removals
	if len(args.Remove) > 0 {
		removeSet := make(map[string]struct{}, len(args.Remove))
		for _, t := range args.Remove {
			removeSet[t] = struct{}{}
		}
		kept := sess.Tags[:0]
		for _, t := range sess.Tags {
			if _, remove := removeSet[t]; !remove {
				kept = append(kept, t)
			}
		}
		sess.Tags = kept
	}

	// Apply additions (deduplicate)
	if len(args.Add) > 0 {
		existing := make(map[string]struct{}, len(sess.Tags))
		for _, t := range sess.Tags {
			existing[t] = struct{}{}
		}
		for _, t := range args.Add {
			if _, exists := existing[t]; !exists {
				sess.Tags = append(sess.Tags, t)
				existing[t] = struct{}{}
			}
		}
	}

	if _, err := h.storage.SaveSession(ctx, sess); err != nil {
		return nil, nil, fmt.Errorf("failed to save session: %w", err)
	}

	slog.Info("tags updated",
		"session_id", safeShortID(sess.UUID),
		"tags", sess.Tags)

	return nil, &TagSessionOutput{
		SessionID: safeShortID(sess.UUID),
		Tags:      sess.Tags,
	}, nil
}
