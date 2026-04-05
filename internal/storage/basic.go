package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
)

// basicStorage is a filesystem-based storage implementation.
//
// IMPORTANT: Context parameters are accepted on all methods for interface
// compatibility with PostgresStorage, but are NOT honored for file operations.
// Go's os.ReadFile/WriteFile do not support context cancellation natively.
// Long-running file operations will complete even if context is cancelled.
// For production workloads requiring cancellation, use PostgresStorage.
type basicStorage struct {
	cfg config.Config
}

// NewBasicStorage creates a new filesystem-based storage implementation.
// Sessions are stored in {storage_location}/{uuid}/raw.json with an index at {storage_location}/index.json.
func NewBasicStorage(cfg config.Config) Storage {
	return &basicStorage{cfg: cfg}
}

// LoadSession loads a session from disk by UUID.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) LoadSession(_ context.Context, uuid string) (*session.Session, error) {
	location := s.cfg.GetStorageLocation()
	filePath := filepath.Join(location, uuid, "raw.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: %s", uuid)
		}
		return nil, err
	}

	var sess session.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}

	return &sess, nil
}

// FindSessionByPrefix finds a session by UUID prefix (minimum 8 characters).
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) FindSessionByPrefix(ctx context.Context, prefix string) (*session.Session, error) {
	if len(prefix) < 8 {
		return nil, fmt.Errorf("prefix must be at least 8 characters")
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, summary := range idx.Sessions {
		if strings.HasPrefix(summary.UUID, prefix) {
			matches = append(matches, summary.UUID)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("session not found: %s", prefix)
	}

	if len(matches) > 1 {
		shortMatches := make([]string, len(matches))
		for i, m := range matches {
			if len(m) >= 8 {
				shortMatches[i] = m[:8]
			} else {
				shortMatches[i] = m
			}
		}
		return nil, fmt.Errorf("ambiguous prefix %s matches %d sessions: %v",
			prefix, len(matches), shortMatches)
	}

	return s.LoadSession(ctx, matches[0])
}

// SessionExists checks if a session exists by UUID.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) SessionExists(_ context.Context, uuid string) bool {
	location := s.cfg.GetStorageLocation()
	filePath := filepath.Join(location, uuid, "raw.json")
	_, err := os.Stat(filePath)
	return err == nil
}

// DeleteSession removes a session and all its summaries from storage.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) DeleteSession(ctx context.Context, uuid string) error {
	if !s.SessionExists(ctx, uuid) {
		return fmt.Errorf("session not found: %s", uuid)
	}

	// Delete all summaries first
	summaries, err := s.ListSummaries(ctx, uuid)
	if err == nil {
		for _, sum := range summaries {
			if err := s.DeleteSummary(ctx, uuid, sum.ID); err != nil {
				shortSessionID := uuid
				if len(uuid) >= 8 {
					shortSessionID = uuid[:8]
				}
				shortSummaryID := sum.ID
				if len(sum.ID) >= 8 {
					shortSummaryID = sum.ID[:8]
				}
				slog.Warn("failed to delete summary during session deletion",
					"session_id", shortSessionID,
					"summary_id", shortSummaryID,
					"error", err)
			}
		}
	}

	// Delete session directory
	location := s.cfg.GetStorageLocation()
	sessionDir := filepath.Join(location, uuid)
	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("failed to delete session directory: %w", err)
	}

	// Remove from index
	idx, err := s.loadIndex()
	if err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	// Filter out the deleted session
	newSessions := make([]SessionSummary, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		if s.UUID != uuid {
			newSessions = append(newSessions, s)
		}
	}
	idx.Sessions = newSessions

	// Save updated index
	if err := s.saveIndex(idx); err != nil {
		return fmt.Errorf("failed to update index: %w", err)
	}

	shortID := uuid
	if len(uuid) >= 8 {
		shortID = uuid[:8]
	}
	slog.Info("deleted session", "uuid", shortID)
	return nil
}

// SaveSession saves the complete session to disk and updates the index.
// If the session has no UUID and title is unique, creates new session with generated UUID.
// If title exists, returns the existing session.
// If session has UUID, updates the existing session.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) SaveSession(ctx context.Context, sess *session.Session) (*session.Session, error) {
	// Case 1: Session has UUID - this is an update
	if sess.UUID != "" {
		return s.updateSession(sess)
	}

	// Case 2: No UUID - check if title exists
	existingSess, err := s.FindSessionByTitle(ctx, sess.Title)
	if err == nil {
		// Title exists - return existing session
		slog.Debug("session with title already exists", "title", sess.Title, "uuid", existingSess.UUID)
		return existingSess, nil
	}

	// Case 3: Title is unique - create new session with generated UUID
	return s.createNewSession(sess)
}

// createNewSession generates a new UUID and creates the session
func (s *basicStorage) createNewSession(sess *session.Session) (*session.Session, error) {
	// Generate UUID
	id := uuid.New()
	sess.UUID = id.String()

	// Set timestamps if not already set
	now := time.Now()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.LastUpdatedAt.IsZero() {
		sess.LastUpdatedAt = now
	}

	return s.updateSession(sess)
}

// updateSession writes session to disk (used for both create and update)
func (s *basicStorage) updateSession(sess *session.Session) (*session.Session, error) {
	location := s.cfg.GetStorageLocation()
	sessionDir := filepath.Join(location, sess.UUID)

	// Ensure directory exists
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, err
	}

	// Marshal session
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return nil, err
	}

	// Write raw.json
	rawPath := filepath.Join(sessionDir, "raw.json")
	if err := os.WriteFile(rawPath, data, 0644); err != nil {
		return nil, err
	}

	// Update index
	if err := s.addToIndex(sess); err != nil {
		slog.Error("failed to update index", "error", err, "uuid", sess.UUID)
		// Try to rebuild index from disk
		slog.Info("attempting to rebuild index from disk sessions")
		if rebuildErr := s.rebuildIndex(); rebuildErr != nil {
			slog.Error("index rebuild failed", "error", rebuildErr)
			// Don't fail the save - session is on disk even if index is broken
		} else {
			slog.Info("index rebuilt successfully")
			// Retry adding to index
			if retryErr := s.addToIndex(sess); retryErr != nil {
				slog.Error("failed to add to rebuilt index", "error", retryErr)
			}
		}
	}

	slog.Debug("saved session", "uuid", sess.UUID, "message_count", len(sess.Messages))
	return sess, nil
}

// FindSessionByTitle searches for a session with the given title
func (s *basicStorage) FindSessionByTitle(ctx context.Context, title string) (*session.Session, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	for _, summary := range idx.Sessions {
		if summary.Title == title {
			return s.LoadSession(ctx, summary.UUID)
		}
	}

	return nil, fmt.Errorf("session with title %s not found", title)
}

// ListSessions returns a paginated list of sessions sorted by LastUpdatedAt descending.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) ListSessions(ctx context.Context, page int, perPage int, tags []string) ([]SessionSummary, int, int, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, 0, 0, err
	}

	// Sort by LastUpdatedAt descending (most recent first)
	sort.Slice(idx.Sessions, func(i, j int) bool {
		return idx.Sessions[i].LastUpdatedAt.After(idx.Sessions[j].LastUpdatedAt)
	})

	sessions := idx.Sessions
	if len(tags) > 0 {
		filtered := sessions[:0]
		for _, sess := range sessions {
			if hasAnyTag(sess.Tags, tags) {
				filtered = append(filtered, sess)
			}
		}
		sessions = filtered
	}

	totalSessions := len(sessions)
	totalPages := (totalSessions + perPage - 1) / perPage

	if totalPages == 0 {
		totalPages = 1
	}

	// Calculate pagination bounds
	start := (page - 1) * perPage
	if start >= totalSessions {
		return []SessionSummary{}, totalPages, totalSessions, nil
	}

	end := start + perPage
	if end > totalSessions {
		end = totalSessions
	}

	result := sessions[start:end]
	for i, sess := range result {
		entries, err := s.ListSummaries(ctx, sess.UUID)
		if err == nil {
			result[i].SummaryCount = len(entries)
		}
	}
	return result, totalPages, totalSessions, nil
}

// hasAnyTag returns true if sessionTags contains any tag from filter.
func hasAnyTag(sessionTags, filter []string) bool {
	for _, f := range filter {
		for _, t := range sessionTags {
			if t == f {
				return true
			}
		}
	}
	return false
}

// loadIndex is a private helper that loads the index.json file
func (s *basicStorage) loadIndex() (*SessionIndex, error) {
	location := s.cfg.GetStorageLocation()
	indexPath := filepath.Join(location, "index.json")

	// Return empty index if doesn't exist
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return &SessionIndex{Sessions: []SessionSummary{}}, nil
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var idx SessionIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	return &idx, nil
}

// saveIndex is a private helper that saves the index.json file
func (s *basicStorage) saveIndex(idx *SessionIndex) error {
	location := s.cfg.GetStorageLocation()
	indexPath := filepath.Join(location, "index.json")

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0644)
}

// addToIndex is a private helper that adds or updates a session in the index
func (s *basicStorage) addToIndex(sess *session.Session) error {
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}

	// Remove existing entry if present (update case)
	for i, summary := range idx.Sessions {
		if summary.UUID == sess.UUID {
			idx.Sessions = append(idx.Sessions[:i], idx.Sessions[i+1:]...)
			break
		}
	}

	// Add new entry
	idx.Sessions = append(idx.Sessions, SessionSummary{
		UUID:          sess.UUID,
		Title:         sess.Title,
		MessageCount:  len(sess.Messages),
		Tags:          sess.Tags,
		LastUpdatedAt: sess.LastUpdatedAt,
	})

	return s.saveIndex(idx)
}

// rebuildIndex scans all session directories and reconstructs index.json from raw.json files
func (s *basicStorage) rebuildIndex() error {
	location := s.cfg.GetStorageLocation()

	entries, err := os.ReadDir(location)
	if err != nil {
		return fmt.Errorf("failed to read storage directory: %w", err)
	}

	newIndex := &SessionIndex{Sessions: []SessionSummary{}}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Try to load session from this directory
		// Use background context since this is recovery code
		sess, err := s.LoadSession(context.Background(), entry.Name())
		if err != nil {
			slog.Warn("skipping invalid session directory", "dir", entry.Name(), "error", err)
			continue
		}

		// Add to index
		newIndex.Sessions = append(newIndex.Sessions, SessionSummary{
			UUID:          sess.UUID,
			Title:         sess.Title,
			MessageCount:  len(sess.Messages),
			Tags:          sess.Tags,
			LastUpdatedAt: sess.LastUpdatedAt,
		})
	}

	// Save rebuilt index
	return s.saveIndex(newIndex)
}

// SaveSummary persists a summary to storage and updates metadata.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) SaveSummary(ctx context.Context, sessionID string, sum *Summary) error {
	if !s.SessionExists(ctx, sessionID) {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Validate required fields
	if sum.CreatedAt.IsZero() {
		return fmt.Errorf("CreatedAt is required")
	}
	if sum.Content.Title == "" {
		return fmt.Errorf("Content.Title is required")
	}

	// Validate SessionID matches parameter
	if sum.SessionID != "" && sum.SessionID != sessionID {
		return fmt.Errorf("SessionID (%s) does not match provided sessionID (%s)", sum.SessionID, sessionID)
	}

	// Validate MessageRange bounds
	if sum.MessageRange.Start < 0 {
		return fmt.Errorf("MessageRange.Start must be >= 0, got: %d", sum.MessageRange.Start)
	}
	if sum.MessageRange.End <= sum.MessageRange.Start {
		return fmt.Errorf("MessageRange.End (%d) must be > Start (%d)", sum.MessageRange.End, sum.MessageRange.Start)
	}

	// Generate UUID if not provided (storage layer owns ID generation)
	if sum.ID == "" {
		sum.ID = uuid.New().String()
	} else {
		// Validate and normalize provided ID to prevent path traversal and ensure consistency
		parsed, err := uuid.Parse(sum.ID)
		if err != nil {
			return fmt.Errorf("ID must be a valid UUID format, got: %s", sum.ID)
		}
		// Normalize to lowercase for consistency (uuid.New().String() always returns lowercase)
		sum.ID = parsed.String()
	}
	// Set SessionID if not provided
	if sum.SessionID == "" {
		sum.SessionID = sessionID
	}

	// Warn if Model is not set (not required, but expected)
	if sum.Model == "" {
		slog.Warn("summary.Model is empty", "session", sessionID)
	}

	location := s.cfg.GetStorageLocation()
	summariesDir := filepath.Join(location, sessionID, "summaries")

	// Create summaries directory if needed
	if err := os.MkdirAll(summariesDir, 0755); err != nil {
		return err
	}

	// Marshal and save summary
	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}

	summaryPath := filepath.Join(summariesDir, summaryFilename(sum.ID))
	if err := os.WriteFile(summaryPath, data, 0644); err != nil {
		return err
	}

	// Update metadata
	err = s.updateSummaryMetadata(sessionID, SummaryEntry{
		ID:           sum.ID,
		SessionID:    sessionID,
		Title:        sum.Content.Title,
		Model:        sum.Model,
		MessageRange: sum.MessageRange,
		CreatedAt:    sum.CreatedAt,
	})
	if err != nil {
		// Rollback: delete the summary file we just wrote
		if removeErr := os.Remove(summaryPath); removeErr != nil {
			slog.Warn("failed to cleanup summary file after metadata update failure",
				"session", sessionID, "id", sum.ID, "error", removeErr)
		}
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// ListSummaries returns metadata for all summaries in a session.
// Returns SummaryEntry (not full Summary) for memory efficiency.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) ListSummaries(ctx context.Context, sessionID string) ([]SummaryEntry, error) {
	if !s.SessionExists(ctx, sessionID) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	location := s.cfg.GetStorageLocation()
	summariesDir := filepath.Join(location, sessionID, "summaries")

	// Return empty list if no summaries directory
	if _, err := os.Stat(summariesDir); os.IsNotExist(err) {
		return []SummaryEntry{}, nil
	}

	metadata, err := s.loadSummaryMetadata(sessionID)
	if err != nil {
		// If metadata doesn't exist but directory does, return empty list
		if os.IsNotExist(err) {
			return []SummaryEntry{}, nil
		}
		return nil, err
	}

	return metadata.Summaries, nil
}

// LoadSummary retrieves a specific summary by session ID and summary ID.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) LoadSummary(ctx context.Context, sessionID, summaryID string) (*Summary, error) {
	if !s.SessionExists(ctx, sessionID) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	location := s.cfg.GetStorageLocation()
	summaryPath := filepath.Join(location, sessionID, "summaries", summaryFilename(summaryID))

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("summary not found: %s", summaryID)
		}
		return nil, err
	}

	var sum Summary
	if err := json.Unmarshal(data, &sum); err != nil {
		return nil, err
	}

	return &sum, nil
}

// LoadAllSummaries retrieves all summaries for a session in a single operation.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) LoadAllSummaries(ctx context.Context, sessionID string) ([]Summary, error) {
	if !s.SessionExists(ctx, sessionID) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	entries, err := s.ListSummaries(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return []Summary{}, nil
	}

	location := s.cfg.GetStorageLocation()
	summaries := make([]Summary, 0, len(entries))

	for _, entry := range entries {
		summaryPath := filepath.Join(location, sessionID, "summaries", summaryFilename(entry.ID))
		data, err := os.ReadFile(summaryPath)
		if err != nil {
			slog.Warn("failed to load summary", "summary_id", entry.ID, "error", err)
			continue
		}

		var sum Summary
		if err := json.Unmarshal(data, &sum); err != nil {
			slog.Warn("failed to parse summary", "summary_id", entry.ID, "error", err)
			continue
		}
		summaries = append(summaries, sum)
	}

	return summaries, nil
}

// DeleteSummary removes a summary from storage.
// Context is accepted for interface compatibility but not used for file I/O.
func (s *basicStorage) DeleteSummary(ctx context.Context, sessionID, summaryID string) error {
	if !s.SessionExists(ctx, sessionID) {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	location := s.cfg.GetStorageLocation()
	summariesDir := filepath.Join(location, sessionID, "summaries")

	metadata, err := s.loadSummaryMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("summary not found: %s", summaryID)
	}

	// Find and remove summary entry
	found := false
	for i, entry := range metadata.Summaries {
		if entry.ID == summaryID {
			found = true
			metadata.Summaries = append(metadata.Summaries[:i], metadata.Summaries[i+1:]...)
			break
		}
	}

	if !found {
		return fmt.Errorf("summary not found: %s", summaryID)
	}

	// Save metadata first (atomicity: orphan file is better than dangling reference)
	if len(metadata.Summaries) == 0 {
		metadataPath := filepath.Join(summariesDir, "metadata.json")
		if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete metadata file: %w", err)
		}
	} else {
		if err := s.saveSummaryMetadata(sessionID, metadata); err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
	}

	// Then delete summary file
	summaryPath := filepath.Join(summariesDir, summaryFilename(summaryID))
	if err := os.Remove(summaryPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to delete summary file after metadata update",
			"session", sessionID, "id", summaryID, "error", err)
	}

	return nil
}

// loadSummaryMetadata is a private helper that loads the metadata.json file
func (s *basicStorage) loadSummaryMetadata(sessionID string) (*SummaryMetadata, error) {
	location := s.cfg.GetStorageLocation()
	metadataPath := filepath.Join(location, sessionID, "summaries", "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata SummaryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// saveSummaryMetadata is a private helper that saves the metadata.json file
func (s *basicStorage) saveSummaryMetadata(sessionID string, metadata *SummaryMetadata) error {
	location := s.cfg.GetStorageLocation()
	metadataPath := filepath.Join(location, sessionID, "summaries", "metadata.json")

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0644)
}

// updateSummaryMetadata is a private helper that adds or updates a summary entry in metadata.
func (s *basicStorage) updateSummaryMetadata(sessionID string, entry SummaryEntry) error {
	metadata, err := s.loadSummaryMetadata(sessionID)
	if err != nil {
		// If metadata doesn't exist, create it
		if os.IsNotExist(err) {
			metadata = &SummaryMetadata{
				SessionID: sessionID,
				Summaries: []SummaryEntry{},
			}
		} else {
			return err
		}
	}

	// Remove existing entry if present (update case)
	for i, e := range metadata.Summaries {
		if e.ID == entry.ID {
			metadata.Summaries = append(metadata.Summaries[:i], metadata.Summaries[i+1:]...)
			break
		}
	}

	// Add new entry
	metadata.Summaries = append(metadata.Summaries, entry)

	return s.saveSummaryMetadata(sessionID, metadata)
}

// ValidateAPIKey is not supported in basicStorage (file-based storage).
// API key authentication only works with PostgreSQL storage.
// This method always returns false for file-based storage.
func (s *basicStorage) ValidateAPIKey(_ context.Context, _, _ string) (bool, error) {
	return false, fmt.Errorf("API key authentication not supported in file-based storage, use STORAGE_TYPE=postgres")
}

// UpdateTags replaces the tags on a session without touching other fields.
// Reloads the session from disk to avoid overwriting concurrent changes to messages.
func (s *basicStorage) UpdateTags(ctx context.Context, sessionID string, tags []string) error {
	sess, err := s.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	sess.Tags = tags
	_, err = s.updateSession(sess)
	return err
}

// Limits tracking is not supported in file-based storage — always returns 0/nil (no limits enforced).
func (s *basicStorage) CountSummariesToday(_ context.Context) (int, error) { return 0, nil }

func (s *basicStorage) TrackSummary(_ context.Context) error { return nil }
