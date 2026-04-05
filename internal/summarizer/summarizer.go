package summarizer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
	"github.com/robert-nemet/sessionmngr/internal/storage"
)

// Summarizer orchestrates conversation summarization.
type Summarizer struct {
	storage       storage.Storage
	client        SummaryClient
	cfg           config.Config
	tokens        metric.Int64Counter
	tokensPerCall metric.Int64Histogram
	userID        string
}

// New creates a new Summarizer with the given storage and configuration.
// promptFile is optional - if empty, the default prompt is used.
// Returns error if the client cannot be created (e.g., missing API key).
func New(store storage.Storage, cfg config.Config, promptFile, userID string) (*Summarizer, error) {
	client, err := NewClient(cfg, promptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create summary client: %w", err)
	}

	meter := otel.Meter("session-manager")
	tokens, _ := meter.Int64Counter("session_manager.summarizer.tokens",
		metric.WithDescription("Tokens consumed by summarizer API calls"))
	tokensPerCall, _ := meter.Int64Histogram("session_manager.summarizer.tokens_per_call",
		metric.WithDescription("Total tokens per summarizer call"),
		metric.WithUnit("{token}"))

	return &Summarizer{
		storage:       store,
		client:        client,
		cfg:           cfg,
		tokens:        tokens,
		tokensPerCall: tokensPerCall,
		userID:        userID,
	}, nil
}

// NewWithClient creates a Summarizer with a specific client (useful for testing).
func NewWithClient(store storage.Storage, client SummaryClient, cfg config.Config) *Summarizer {
	meter := otel.Meter("session-manager")
	tokens, _ := meter.Int64Counter("session_manager.summarizer.tokens",
		metric.WithDescription("Tokens consumed by summarizer API calls"))
	tokensPerCall, _ := meter.Int64Histogram("session_manager.summarizer.tokens_per_call",
		metric.WithDescription("Total tokens per summarizer call"),
		metric.WithUnit("{token}"))

	return &Summarizer{
		storage:       store,
		client:        client,
		cfg:           cfg,
		tokens:        tokens,
		tokensPerCall: tokensPerCall,
	}
}

// SummarizeSession generates a summary for an entire session.
// The context controls cancellation and timeout for the AI API call.
// Returns the created summary or error.
func (s *Summarizer) SummarizeSession(ctx context.Context, sessionID string) (*storage.Summary, error) {
	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	return s.summarizeMessages(ctx, sess, 0, len(sess.Messages), s.client, "")
}

// SummarizeRange generates a summary for a specific message range.
// The context controls cancellation and timeout for the AI API call.
// Start is inclusive, end is exclusive (like Go slices).
func (s *Summarizer) SummarizeRange(ctx context.Context, sessionID string, start, end int) (*storage.Summary, error) {
	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	return s.summarizeMessages(ctx, sess, start, end, s.client, "")
}

// SummarizeSessionWithType generates a summary using a specific prompt type.
// Creates a one-off client with the given prompt type.
// For "resume" type, uses a deterministic UUID so repeated calls upsert the same row.
func (s *Summarizer) SummarizeSessionWithType(ctx context.Context, sessionID, promptType string) (*storage.Summary, error) {
	client, err := NewClient(s.cfg, promptType)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for prompt type %q: %w", promptType, err)
	}

	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Resume uses deterministic ID so concurrent calls upsert the same row
	var fixedID string
	if promptType == "resume" {
		fixedID = ResumeID(sess.UUID)
	}

	return s.summarizeMessages(ctx, sess, 0, len(sess.Messages), client, fixedID)
}

// ResumeID returns a deterministic UUID for a session's resume summary.
// Uses UUID v5 (SHA-1 namespace) so the same session always gets the same resume ID.
func ResumeID(sessionID string) string {
	namespace := uuid.MustParse("b9a1f5d0-7c3e-4a2b-9f8d-1e6c5a3b7d0f")
	return uuid.NewSHA1(namespace, []byte(sessionID)).String()
}

// summarizeMessages is the internal implementation that works with an already-loaded session.
// If fixedID is non-empty, it is used as the summary ID (enabling upsert for resume summaries).
func (s *Summarizer) summarizeMessages(ctx context.Context, sess *session.Session, start, end int, client SummaryClient, fixedID string) (*storage.Summary, error) {
	// Validate range
	if start < 0 {
		return nil, fmt.Errorf("start index cannot be negative: %d", start)
	}
	if end > len(sess.Messages) {
		return nil, fmt.Errorf("end index %d exceeds message count %d", end, len(sess.Messages))
	}
	if start >= end {
		return nil, fmt.Errorf("invalid range: start (%d) must be less than end (%d)", start, end)
	}

	messageCount := end - start
	if messageCount < s.cfg.GetSummarizerMinMessages() {
		return nil, fmt.Errorf("range has %d messages, minimum required is %d",
			messageCount, s.cfg.GetSummarizerMinMessages())
	}

	messages := sess.Messages[start:end]

	// Generate summary via AI
	result, err := client.GenerateSummary(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		model := client.GetModelName()
		s.tokens.Add(ctx, result.Usage.InputTokens,
			metric.WithAttributes(attribute.String("model", model), attribute.String("type", "input"), attribute.String("user_id", s.userID)))
		s.tokens.Add(ctx, result.Usage.OutputTokens,
			metric.WithAttributes(attribute.String("model", model), attribute.String("type", "output"), attribute.String("user_id", s.userID)))
		s.tokensPerCall.Record(ctx, result.Usage.InputTokens+result.Usage.OutputTokens,
			metric.WithAttributes(attribute.String("type", "full"), attribute.String("model", model), attribute.String("user_id", s.userID)))
	}

	s.autoTag(ctx, sess, result.Tags)

	// Create summary object
	sum := &storage.Summary{
		ID:           fixedID, // empty = storage generates new UUID; non-empty = upsert
		SessionID:    sess.UUID,
		Model:        client.GetModelName(),
		PromptSource: client.GetPromptSource(),
		CreatedAt:    time.Now(),
		MessageRange: storage.MessageRange{
			Start: start,
			End:   end,
		},
		Content: storage.Content{
			Title:    result.Title,
			Markdown: result.Markdown,
		},
	}

	// Save to storage
	if err := s.storage.SaveSummary(ctx, sess.UUID, sum); err != nil {
		return nil, fmt.Errorf("failed to save summary: %w", err)
	}

	return sum, nil
}

// CompactResume generates or updates the resume summary for a session.
// First call (no prior resume): full regen via SummarizeSessionWithType.
// Subsequent calls: incremental — prior resume + delta messages only.
func (s *Summarizer) CompactResume(ctx context.Context, sessionID string) (*storage.Summary, error) {
	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Find existing resume
	var existingResume *storage.Summary
	summaries, err := s.storage.LoadAllSummaries(ctx, sess.UUID)
	if err == nil {
		for i := range summaries {
			if summaries[i].PromptSource == "type:resume" {
				existingResume = &summaries[i]
				break
			}
		}
	}

	// No prior resume: full regen
	if existingResume == nil {
		return s.SummarizeSessionWithType(ctx, sess.UUID, "resume")
	}

	// Prior resume exists: incremental update
	// Bounds check: if resume covers more messages than currently exist, fall back to full regen
	if existingResume.MessageRange.End > len(sess.Messages) {
		return s.SummarizeSessionWithType(ctx, sess.UUID, "resume")
	}
	delta := sess.Messages[existingResume.MessageRange.End:]
	if len(delta) == 0 {
		return existingResume, nil
	}

	client, err := NewClient(s.cfg, "resume")
	if err != nil {
		return nil, fmt.Errorf("failed to create resume client: %w", err)
	}

	result, err := client.GenerateIncrementalResume(ctx, existingResume.Content.Markdown, delta)
	if err != nil {
		return nil, fmt.Errorf("failed to generate incremental resume: %w", err)
	}

	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		model := client.GetModelName()
		s.tokens.Add(ctx, result.Usage.InputTokens,
			metric.WithAttributes(attribute.String("model", model), attribute.String("type", "input"), attribute.String("user_id", s.userID)))
		s.tokens.Add(ctx, result.Usage.OutputTokens,
			metric.WithAttributes(attribute.String("model", model), attribute.String("type", "output"), attribute.String("user_id", s.userID)))
		s.tokensPerCall.Record(ctx, result.Usage.InputTokens+result.Usage.OutputTokens,
			metric.WithAttributes(attribute.String("type", "incremental"), attribute.String("model", model), attribute.String("user_id", s.userID)))
	}

	s.autoTag(ctx, sess, result.Tags)

	messageCount := len(sess.Messages)
	sum := &storage.Summary{
		ID:           ResumeID(sess.UUID),
		SessionID:    sess.UUID,
		Model:        client.GetModelName(),
		PromptSource: "type:resume",
		CreatedAt:    time.Now(),
		MessageRange: storage.MessageRange{
			Start: 0,
			End:   messageCount,
		},
		Content: storage.Content{
			Title:    result.Title,
			Markdown: result.Markdown,
		},
	}

	if err := s.storage.SaveSummary(ctx, sess.UUID, sum); err != nil {
		return nil, fmt.Errorf("failed to save incremental resume: %w", err)
	}

	return sum, nil
}

// autoTag merges new tags from a summary result into the session's existing tags.
func (s *Summarizer) autoTag(ctx context.Context, sess *session.Session, newTags []string) {
	if len(newTags) == 0 {
		return
	}
	existing := make(map[string]bool, len(sess.Tags))
	for _, t := range sess.Tags {
		existing[t] = true
	}
	merged := make([]string, len(sess.Tags))
	copy(merged, sess.Tags)
	var added []string
	for _, t := range newTags {
		if !existing[t] {
			merged = append(merged, t)
			added = append(added, t)
		}
	}
	if len(added) > 0 {
		if err := s.storage.UpdateTags(ctx, sess.UUID, merged); err != nil {
			slog.Warn("failed to auto-tag session", "session_id", sess.UUID[:8], "error", err)
		} else {
			slog.Info("auto-tagged session", "session_id", sess.UUID[:8], "tags", added)
		}
	}
}

// ListSummaries returns all summaries for a session.
// Uses background context since this is typically called outside of request context.
func (s *Summarizer) ListSummaries(ctx context.Context, sessionID string) ([]storage.SummaryEntry, error) {
	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}

	return s.storage.ListSummaries(ctx, sess.UUID)
}

// SelectSummaryOptions specifies how to select a summary for export.
type SelectSummaryOptions struct {
	SummaryID string // Explicit summary ID (takes precedence)
	Latest    bool   // Select most recent summary
	Index     int    // Select by 1-based index (1=most recent)
}

// SelectSummary chooses a summary based on the given options.
// Returns the summary ID to use, or error if selection fails.
// Selection priority: explicit ID > --latest > -n index > auto-select (if single) > error
func (s *Summarizer) SelectSummary(ctx context.Context, sessionID string, opts SelectSummaryOptions) (string, error) {
	// Explicit summary ID takes precedence
	if opts.SummaryID != "" {
		return opts.SummaryID, nil
	}

	// Get and sort summaries
	entries, err := s.ListSummaries(ctx, sessionID)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no summaries found for session %s", sessionID)
	}

	// Sort by date (most recent first)
	storage.SortSummariesByDate(entries)

	// --latest: pick most recent
	if opts.Latest {
		return entries[0].ID, nil
	}

	// -n index: pick by 1-based index
	if opts.Index > 0 {
		if opts.Index > len(entries) {
			return "", fmt.Errorf("index %d out of range (have %d summaries)", opts.Index, len(entries))
		}
		return entries[opts.Index-1].ID, nil
	}

	// Auto-select if only one summary
	if len(entries) == 1 {
		return entries[0].ID, nil
	}

	// Multiple summaries, no selection method
	return "", fmt.Errorf("session has %d summaries - use --latest, -n <index>, or --summary <id> (run 'list -s %s' to see them)", len(entries), sessionID)
}

// LoadSummary retrieves a specific summary by ID or prefix.
// summaryID can be a full UUID or a prefix (minimum 8 characters).
func (s *Summarizer) LoadSummary(ctx context.Context, sessionID, summaryID string) (*storage.Summary, error) {
	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}

	// Try to resolve summary ID prefix to full ID
	resolvedID, err := s.resolveSummaryPrefix(ctx, sess.UUID, summaryID)
	if err != nil {
		return nil, err
	}

	return s.storage.LoadSummary(ctx, sess.UUID, resolvedID)
}

// UUIDLength is the length of a UUID string with dashes (e.g., "550e8400-e29b-41d4-a716-446655440000")
const UUIDLength = 36

// resolveSummaryPrefix finds a summary by prefix match.
// Returns the full summary ID or error if no match or multiple matches.
func (s *Summarizer) resolveSummaryPrefix(ctx context.Context, sessionID, prefix string) (string, error) {
	// If prefix looks like a full UUID, use as-is
	if len(prefix) == UUIDLength {
		return prefix, nil
	}

	// List all summaries and find matches
	entries, err := s.storage.ListSummaries(ctx, sessionID)
	if err != nil {
		return "", err
	}

	var matches []string
	for _, e := range entries {
		if len(e.ID) >= len(prefix) && e.ID[:len(prefix)] == prefix {
			matches = append(matches, e.ID)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("summary not found: %s", prefix)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous summary prefix '%s' matches %d summaries", prefix, len(matches))
	}

	return matches[0], nil
}

// DeleteSummary removes a summary from storage.
// summaryID can be a full UUID or a prefix (minimum 8 characters).
func (s *Summarizer) DeleteSummary(ctx context.Context, sessionID, summaryID string) error {
	sess, err := s.storage.FindSessionByPrefix(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Try to resolve summary ID prefix to full ID
	resolvedID, err := s.resolveSummaryPrefix(ctx, sess.UUID, summaryID)
	if err != nil {
		return err
	}

	return s.storage.DeleteSummary(ctx, sess.UUID, resolvedID)
}
