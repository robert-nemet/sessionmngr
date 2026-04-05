package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robert-nemet/sessionmngr/internal/session"
)

// PostgresStorage implements Storage interface using PostgreSQL.
type PostgresStorage struct {
	pool   *pgxpool.Pool
	userID string
}

// NewPostgresStorage creates a new PostgreSQL-based storage implementation.
// userID scopes all operations to a specific user.
func NewPostgresStorage(pool *pgxpool.Pool, userID string) Storage {
	return &PostgresStorage{
		pool:   pool,
		userID: userID,
	}
}

// SaveSession persists a session to PostgreSQL.
// If sess.UUID is empty, creates new session with generated UUID.
// If sess.UUID is empty and title exists, returns existing session (deduplication).
// If sess.UUID is set, updates existing session.
func (s *PostgresStorage) SaveSession(ctx context.Context, sess *session.Session) (*session.Session, error) {
	// Case 1: Session has UUID - this is an update
	if sess.UUID != "" {
		return s.updateSession(ctx, sess)
	}

	// Case 2: No UUID - check if title exists for this user
	existingSess, err := s.FindSessionByTitle(ctx, sess.Title)
	if err == nil {
		slog.Debug("session with title already exists", "title", sess.Title, "uuid", existingSess.UUID)
		return existingSess, nil
	}

	// Case 3: Title is unique - create new session with generated UUID
	return s.createNewSession(ctx, sess)
}

// createNewSession generates a new UUID and creates the session
func (s *PostgresStorage) createNewSession(ctx context.Context, sess *session.Session) (*session.Session, error) {
	id := uuid.New()
	sess.UUID = id.String()

	now := time.Now()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.LastUpdatedAt.IsZero() {
		sess.LastUpdatedAt = now
	}

	return s.updateSession(ctx, sess)
}

// updateSession writes session to database (used for both create and update)
func (s *PostgresStorage) updateSession(ctx context.Context, sess *session.Session) (*session.Session, error) {
	messagesJSON, err := json.Marshal(sess.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal messages: %w", err)
	}

	query := `
		INSERT INTO sessions (id, user_id, title, messages, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			messages = EXCLUDED.messages,
			tags = EXCLUDED.tags,
			updated_at = EXCLUDED.updated_at
	`

	_, err = s.pool.Exec(ctx, query,
		sess.UUID,
		s.userID,
		sess.Title,
		messagesJSON,
		sess.Tags,
		sess.CreatedAt,
		sess.LastUpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	slog.Debug("saved session", "uuid", sess.UUID, "message_count", len(sess.Messages))
	return sess, nil
}

// FindSessionByTitle searches for a session with the given title for this user
func (s *PostgresStorage) FindSessionByTitle(ctx context.Context, title string) (*session.Session, error) {
	query := `
		SELECT id, title, messages, tags, created_at, updated_at
		FROM sessions
		WHERE user_id = $1 AND title = $2
	`

	var sess session.Session
	var messagesJSON []byte

	err := s.pool.QueryRow(ctx, query, s.userID, title).Scan(
		&sess.UUID,
		&sess.Title,
		&messagesJSON,
		&sess.Tags,
		&sess.CreatedAt,
		&sess.LastUpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("session with title %s not found", title)
		}
		return nil, err
	}

	if err := json.Unmarshal(messagesJSON, &sess.Messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	return &sess, nil
}

// LoadSession retrieves a session by full UUID.
func (s *PostgresStorage) LoadSession(ctx context.Context, uuid string) (*session.Session, error) {
	query := `
		SELECT id, title, messages, tags, created_at, updated_at
		FROM sessions
		WHERE user_id = $1 AND id = $2
	`

	var sess session.Session
	var messagesJSON []byte

	err := s.pool.QueryRow(ctx, query, s.userID, uuid).Scan(
		&sess.UUID,
		&sess.Title,
		&messagesJSON,
		&sess.Tags,
		&sess.CreatedAt,
		&sess.LastUpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", uuid)
		}
		return nil, err
	}

	if err := json.Unmarshal(messagesJSON, &sess.Messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	return &sess, nil
}

// FindSessionByPrefix finds a session by UUID prefix (minimum 8 characters).
func (s *PostgresStorage) FindSessionByPrefix(ctx context.Context, prefix string) (*session.Session, error) {
	if len(prefix) < 8 {
		return nil, fmt.Errorf("prefix must be at least 8 characters")
	}

	query := `
		SELECT id, title, messages, tags, created_at, updated_at
		FROM sessions
		WHERE user_id = $1 AND id::text LIKE $2
	`

	rows, err := s.pool.Query(ctx, query, s.userID, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []session.Session
	for rows.Next() {
		var sess session.Session
		var messagesJSON []byte

		err := rows.Scan(
			&sess.UUID,
			&sess.Title,
			&messagesJSON,
			&sess.Tags,
			&sess.CreatedAt,
			&sess.LastUpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(messagesJSON, &sess.Messages); err != nil {
			return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
		}

		sessions = append(sessions, sess)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("session not found: %s", prefix)
	}

	if len(sessions) > 1 {
		shortMatches := make([]string, len(sessions))
		for i, sess := range sessions {
			if len(sess.UUID) >= 8 {
				shortMatches[i] = sess.UUID[:8]
			} else {
				shortMatches[i] = sess.UUID
			}
		}
		return nil, fmt.Errorf("ambiguous prefix %s matches %d sessions: %v",
			prefix, len(sessions), shortMatches)
	}

	return &sessions[0], nil
}

// SessionExists checks if a session with given UUID exists for this user.
func (s *PostgresStorage) SessionExists(ctx context.Context, uuid string) bool {
	query := `SELECT EXISTS(SELECT 1 FROM sessions WHERE user_id = $1 AND id = $2)`

	var exists bool
	err := s.pool.QueryRow(ctx, query, s.userID, uuid).Scan(&exists)
	if err != nil {
		return false
	}

	return exists
}

// DeleteSession removes a session and all its summaries from storage.
func (s *PostgresStorage) DeleteSession(ctx context.Context, uuid string) error {
	if !s.SessionExists(ctx, uuid) {
		return fmt.Errorf("session not found: %s", uuid)
	}

	// Delete summaries first (cascading deletes should handle this, but be explicit)
	deleteSummariesQuery := `DELETE FROM summaries WHERE session_id = $1`
	_, err := s.pool.Exec(ctx, deleteSummariesQuery, uuid)
	if err != nil {
		return fmt.Errorf("failed to delete summaries: %w", err)
	}

	// Delete session
	deleteSessionQuery := `DELETE FROM sessions WHERE id = $1 AND user_id = $2`
	result, err := s.pool.Exec(ctx, deleteSessionQuery, uuid, s.userID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("session not found or access denied: %s", uuid)
	}

	shortID := uuid
	if len(uuid) >= 8 {
		shortID = uuid[:8]
	}
	slog.Info("deleted session from postgres", "uuid", shortID, "user_id", s.userID)
	return nil
}

// ListSessions returns paginated session summaries sorted by LastUpdatedAt descending.
func (s *PostgresStorage) ListSessions(ctx context.Context, page int, perPage int, tags []string) ([]SessionSummary, int, int, error) {
	var (
		totalSessions int
		rows          pgx.Rows
		err           error
	)
	offset := (page - 1) * perPage

	if len(tags) > 0 {
		err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND tags && $2`, s.userID, tags).Scan(&totalSessions)
		if err != nil {
			return nil, 0, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT id, title, jsonb_array_length(messages), tags, updated_at,
			       (SELECT COUNT(*) FROM summaries WHERE session_id = sessions.id)
			FROM sessions WHERE user_id = $1 AND tags && $2
			ORDER BY updated_at DESC LIMIT $3 OFFSET $4`,
			s.userID, tags, perPage, offset)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = $1`, s.userID).Scan(&totalSessions)
		if err != nil {
			return nil, 0, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT id, title, jsonb_array_length(messages), tags, updated_at,
			       (SELECT COUNT(*) FROM summaries WHERE session_id = sessions.id)
			FROM sessions WHERE user_id = $1
			ORDER BY updated_at DESC LIMIT $2 OFFSET $3`,
			s.userID, perPage, offset)
	}
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	totalPages := (totalSessions + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}

	var summaries []SessionSummary
	for rows.Next() {
		var summary SessionSummary
		if err := rows.Scan(&summary.UUID, &summary.Title, &summary.MessageCount, &summary.Tags, &summary.LastUpdatedAt, &summary.SummaryCount); err != nil {
			return nil, 0, 0, err
		}
		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	if summaries == nil {
		summaries = []SessionSummary{}
	}

	return summaries, totalPages, totalSessions, nil
}

// SaveSummary persists a summary to storage.
func (s *PostgresStorage) SaveSummary(ctx context.Context, sessionID string, sum *Summary) error {
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

	// Generate UUID if not provided
	if sum.ID == "" {
		sum.ID = uuid.New().String()
	} else {
		// Validate and normalize provided ID
		parsed, err := uuid.Parse(sum.ID)
		if err != nil {
			return fmt.Errorf("ID must be a valid UUID format, got: %s", sum.ID)
		}
		sum.ID = parsed.String()
	}

	// Set SessionID if not provided
	if sum.SessionID == "" {
		sum.SessionID = sessionID
	}

	if sum.Model == "" {
		slog.Warn("summary.Model is empty", "session", sessionID)
	}

	// Marshal message_range and content to JSON
	messageRangeJSON, err := json.Marshal(sum.MessageRange)
	if err != nil {
		return fmt.Errorf("failed to marshal message_range: %w", err)
	}

	// Store full markdown content as text
	content := sum.Content.Markdown
	if content == "" {
		content = sum.Content.Title
	}

	query := `
		INSERT INTO summaries (id, session_id, user_id, prompt_type, content, message_range, model, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			prompt_type = EXCLUDED.prompt_type,
			content = EXCLUDED.content,
			message_range = EXCLUDED.message_range,
			model = EXCLUDED.model
	`

	_, err = s.pool.Exec(ctx, query,
		sum.ID,
		sessionID,
		s.userID,
		sum.PromptSource,
		content,
		messageRangeJSON,
		sum.Model,
		sum.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save summary: %w", err)
	}

	return nil
}

// ListSummaries returns metadata for all summaries in a session.
func (s *PostgresStorage) ListSummaries(ctx context.Context, sessionID string) ([]SummaryEntry, error) {
	if !s.SessionExists(ctx, sessionID) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	query := `
		SELECT id, session_id,
			   COALESCE(SUBSTRING(content FROM 1 FOR 100), '') as title,
			   model, message_range, created_at
		FROM summaries
		WHERE session_id = $1 AND user_id = $2
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, sessionID, s.userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []SummaryEntry
	for rows.Next() {
		var entry SummaryEntry
		var messageRangeJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.SessionID,
			&entry.Title,
			&entry.Model,
			&messageRangeJSON,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(messageRangeJSON, &entry.MessageRange); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message_range: %w", err)
		}

		// Extract title from first line if needed
		if strings.Contains(entry.Title, "\n") {
			entry.Title = strings.Split(entry.Title, "\n")[0]
		}
		entry.Title = strings.TrimPrefix(entry.Title, "# ")

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if entries == nil {
		entries = []SummaryEntry{}
	}

	return entries, nil
}

// LoadSummary retrieves a specific summary by session ID and summary ID.
func (s *PostgresStorage) LoadSummary(ctx context.Context, sessionID, summaryID string) (*Summary, error) {
	if !s.SessionExists(ctx, sessionID) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	query := `
		SELECT id, session_id, prompt_type, content, message_range, model, created_at
		FROM summaries
		WHERE id = $1 AND session_id = $2 AND user_id = $3
	`

	var sum Summary
	var messageRangeJSON []byte
	var content string
	var promptType *string

	err := s.pool.QueryRow(ctx, query, summaryID, sessionID, s.userID).Scan(
		&sum.ID,
		&sum.SessionID,
		&promptType,
		&content,
		&messageRangeJSON,
		&sum.Model,
		&sum.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("summary not found: %s", summaryID)
		}
		return nil, err
	}

	if promptType != nil {
		sum.PromptSource = *promptType
	}

	if err := json.Unmarshal(messageRangeJSON, &sum.MessageRange); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message_range: %w", err)
	}

	// Extract title from content
	sum.Content.Markdown = content
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		sum.Content.Title = strings.TrimPrefix(lines[0], "# ")
	}

	return &sum, nil
}

// LoadAllSummaries retrieves all summaries for a session in a single query.
func (s *PostgresStorage) LoadAllSummaries(ctx context.Context, sessionID string) ([]Summary, error) {
	if !s.SessionExists(ctx, sessionID) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	query := `
		SELECT id, session_id, prompt_type, content, message_range, model, created_at
		FROM summaries
		WHERE session_id = $1 AND user_id = $2
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, sessionID, s.userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []Summary
	for rows.Next() {
		var sum Summary
		var messageRangeJSON []byte
		var content string
		var promptType *string

		err := rows.Scan(
			&sum.ID,
			&sum.SessionID,
			&promptType,
			&content,
			&messageRangeJSON,
			&sum.Model,
			&sum.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if promptType != nil {
			sum.PromptSource = *promptType
		}

		if err := json.Unmarshal(messageRangeJSON, &sum.MessageRange); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message_range: %w", err)
		}

		sum.Content.Markdown = content
		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			sum.Content.Title = strings.TrimPrefix(lines[0], "# ")
		}

		summaries = append(summaries, sum)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if summaries == nil {
		return []Summary{}, nil
	}

	return summaries, nil
}

// DeleteSummary removes a summary from storage.
func (s *PostgresStorage) DeleteSummary(ctx context.Context, sessionID, summaryID string) error {
	if !s.SessionExists(ctx, sessionID) {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	query := `DELETE FROM summaries WHERE id = $1 AND session_id = $2 AND user_id = $3`

	result, err := s.pool.Exec(ctx, query, summaryID, sessionID, s.userID)
	if err != nil {
		return fmt.Errorf("failed to delete summary: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("summary not found: %s", summaryID)
	}

	return nil
}

// ValidateAPIKey checks if an API key hash is valid for a given user ID.
// Returns true if the key exists, belongs to the user, and is active.
// Updates last_used_at timestamp asynchronously if valid.
func (s *PostgresStorage) ValidateAPIKey(ctx context.Context, keyHash, userID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(
		SELECT 1 FROM api_keys
		WHERE key_hash = $1
		  AND user_id = $2
		  AND is_active = true
	)`

	err := s.pool.QueryRow(ctx, query, keyHash, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to validate API key: %w", err)
	}

	if exists {
		go func() {
			updateCtx := context.Background()
			updateQuery := `UPDATE api_keys SET last_used_at = NOW() WHERE key_hash = $1`
			if _, err := s.pool.Exec(updateCtx, updateQuery, keyHash); err != nil {
				slog.Warn("failed to update last_used_at for API key", "error", err)
			}
		}()
	}

	return exists, nil
}

// UpdateTags replaces the tags on a session without touching other fields.
func (s *PostgresStorage) UpdateTags(ctx context.Context, sessionID string, tags []string) error {
	query := `UPDATE sessions SET tags = $1 WHERE id = $2 AND user_id = $3`
	result, err := s.pool.Exec(ctx, query, tags, sessionID, s.userID)
	if err != nil {
		return fmt.Errorf("failed to update tags: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

// CountSummariesToday returns the number of summaries created today by the current user.
func (s *PostgresStorage) CountSummariesToday(ctx context.Context) (int, error) {
	var count int
	query := `SELECT summaries_count FROM user_usage WHERE user_id = $1 AND date = CURRENT_DATE`
	err := s.pool.QueryRow(ctx, query, s.userID).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to count summaries today: %w", err)
	}
	return count, nil
}

// TrackSummary increments the daily summary count for the current user.
func (s *PostgresStorage) TrackSummary(ctx context.Context) error {
	query := `
		INSERT INTO user_usage (user_id, date, summaries_count)
		VALUES ($1, CURRENT_DATE, 1)
		ON CONFLICT (user_id, date) DO UPDATE
		SET summaries_count = user_usage.summaries_count + 1
	`
	if _, err := s.pool.Exec(ctx, query, s.userID); err != nil {
		return fmt.Errorf("failed to track summary: %w", err)
	}
	return nil
}
