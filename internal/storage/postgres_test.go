package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robert-nemet/sessionmngr/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDatabaseURL = "postgres://sessionmgr:sessionmgr@localhost:5432/session_manager"

func setupTestDB(t *testing.T) (*pgxpool.Pool, string) {
	if os.Getenv("TEST_POSTGRES") == "" {
		t.Skip("Skipping Postgres tests. Set TEST_POSTGRES=1 to run.")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDatabaseURL)
	require.NoError(t, err)

	// Create a test user
	userID := uuid.New().String()
	_, err = pool.Exec(ctx, `INSERT INTO users (id, name, email) VALUES ($1, $2, $3)`,
		userID, "Test User", "test-"+userID[:8]+"@example.com")
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clean up test data
		_, _ = pool.Exec(ctx, `DELETE FROM user_usage WHERE user_id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM summaries WHERE user_id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		pool.Close()
	})

	return pool, userID
}

func TestPostgresSaveSession(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	now := time.Now()
	sess := &session.Session{
		UUID:          "",
		Title:         "Test Postgres Session",
		CreatedAt:     now,
		LastUpdatedAt: now,
		Tags:          []string{"test", "postgres"},
		Messages: []session.Message{
			{Role: "user", Content: "Test question"},
			{Role: "assistant", Content: "Test answer"},
		},
	}

	savedSess, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)
	require.NotEmpty(t, savedSess.UUID, "UUID should be generated")

	// Load and verify
	loaded, err := store.LoadSession(context.Background(), savedSess.UUID)
	require.NoError(t, err)

	assert.Equal(t, savedSess.UUID, loaded.UUID)
	assert.Equal(t, "Test Postgres Session", loaded.Title)
	assert.Len(t, loaded.Messages, 2)
	assert.Equal(t, []string{"test", "postgres"}, loaded.Tags)
}

func TestPostgresTitleDeduplication(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	now := time.Now()

	// Create first session
	sess1 := &session.Session{
		Title:         "Duplicate Title",
		CreatedAt:     now,
		LastUpdatedAt: now,
		Messages:      []session.Message{},
	}

	saved1, err := store.SaveSession(context.Background(), sess1)
	require.NoError(t, err)

	// Try to create second session with same title
	sess2 := &session.Session{
		Title:         "Duplicate Title",
		CreatedAt:     now,
		LastUpdatedAt: now,
		Messages:      []session.Message{},
	}

	saved2, err := store.SaveSession(context.Background(), sess2)
	require.NoError(t, err)

	// Should return the same session
	assert.Equal(t, saved1.UUID, saved2.UUID)
}

func TestPostgresUpdateSession(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	now := time.Now()
	sess := &session.Session{
		Title:         "Update Test",
		CreatedAt:     now,
		LastUpdatedAt: now,
		Messages:      []session.Message{},
	}

	saved, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)

	// Update session
	saved.Messages = append(saved.Messages, session.Message{Role: "user", Content: "New message"})
	saved.LastUpdatedAt = time.Now()

	updated, err := store.SaveSession(context.Background(), saved)
	require.NoError(t, err)

	assert.Len(t, updated.Messages, 1)

	// Verify update persisted
	loaded, err := store.LoadSession(context.Background(), saved.UUID)
	require.NoError(t, err)
	assert.Len(t, loaded.Messages, 1)
}

func TestPostgresFindSessionByPrefix(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	// Create a session with known UUID
	sess := &session.Session{
		Title:         "Prefix Test",
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
		Messages:      []session.Message{},
	}

	saved, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)

	// Find by 8-char prefix
	prefix := saved.UUID[:8]
	found, err := store.FindSessionByPrefix(context.Background(), prefix)
	require.NoError(t, err)
	assert.Equal(t, saved.UUID, found.UUID)

	// Test prefix too short
	_, err = store.FindSessionByPrefix(context.Background(), "abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")

	// Test not found
	_, err = store.FindSessionByPrefix(context.Background(), "deadbeef")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresListSessions(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	// Create multiple sessions
	for i := 0; i < 15; i++ {
		sess := &session.Session{
			Title:         "List Test " + string(rune('A'+i)),
			CreatedAt:     time.Now().Add(time.Duration(-i) * time.Hour),
			LastUpdatedAt: time.Now().Add(time.Duration(-i) * time.Hour),
			Messages:      []session.Message{},
		}
		_, err := store.SaveSession(context.Background(), sess)
		require.NoError(t, err)
	}

	// Test page 1
	sessions, totalPages, totalSessions, err := store.ListSessions(context.Background(), 1, 10, nil)
	require.NoError(t, err)

	assert.Len(t, sessions, 10)
	assert.Equal(t, 2, totalPages)
	assert.Equal(t, 15, totalSessions)

	// Verify sorted by updated_at DESC
	assert.Equal(t, "List Test A", sessions[0].Title)

	// Test page 2
	sessions2, _, _, err := store.ListSessions(context.Background(), 2, 10, nil)
	require.NoError(t, err)
	assert.Len(t, sessions2, 5)
}

func TestPostgresSessionExists(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	sess := &session.Session{
		Title:         "Exists Test",
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
		Messages:      []session.Message{},
	}

	saved, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)

	assert.True(t, store.SessionExists(context.Background(), saved.UUID))
	assert.False(t, store.SessionExists(context.Background(), "nonexistent-uuid"))
}

func TestPostgresSaveSummary(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	// Create a session first
	sess := &session.Session{
		Title:         "Summary Test Session",
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
		Messages: []session.Message{
			{Role: "user", Content: "Message 1"},
			{Role: "assistant", Content: "Response 1"},
		},
	}
	saved, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)

	// Create summary
	summary := &Summary{
		SessionID:    saved.UUID,
		Model:        "claude-sonnet-4-20250514",
		PromptSource: "default",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 2},
		Content: Content{
			Title:    "Test Summary",
			Markdown: "# Test Summary\n\nThis is a test summary.",
		},
	}

	err = store.SaveSummary(context.Background(), saved.UUID, summary)
	require.NoError(t, err)
	require.NotEmpty(t, summary.ID)

	// List summaries
	entries, err := store.ListSummaries(context.Background(), saved.UUID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, summary.ID, entries[0].ID)

	// Load summary
	loaded, err := store.LoadSummary(context.Background(), saved.UUID, summary.ID)
	require.NoError(t, err)
	assert.Equal(t, summary.ID, loaded.ID)
	assert.Contains(t, loaded.Content.Markdown, "Test Summary")
}

func TestPostgresDeleteSummary(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	// Create session and summary
	sess := &session.Session{
		Title:         "Delete Summary Test",
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
		Messages:      []session.Message{{Role: "user", Content: "Test"}},
	}
	saved, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)

	summary := &Summary{
		SessionID:    saved.UUID,
		Model:        "test-model",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 1},
		Content:      Content{Title: "Delete Me", Markdown: "# Delete Me"},
	}
	err = store.SaveSummary(context.Background(), saved.UUID, summary)
	require.NoError(t, err)

	// Delete summary
	err = store.DeleteSummary(context.Background(), saved.UUID, summary.ID)
	require.NoError(t, err)

	// Verify deleted
	entries, err := store.ListSummaries(context.Background(), saved.UUID)
	require.NoError(t, err)
	assert.Len(t, entries, 0)

	// Delete again should error
	err = store.DeleteSummary(context.Background(), saved.UUID, summary.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresSummaryValidation(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	sess := &session.Session{
		Title:         "Validation Test",
		CreatedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
		Messages:      []session.Message{{Role: "user", Content: "Test"}},
	}
	saved, err := store.SaveSession(context.Background(), sess)
	require.NoError(t, err)

	// Missing CreatedAt
	err = store.SaveSummary(context.Background(), saved.UUID, &Summary{
		Content: Content{Title: "Test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CreatedAt")

	// Missing Title
	err = store.SaveSummary(context.Background(), saved.UUID, &Summary{
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 1},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Title")

	// Invalid MessageRange
	err = store.SaveSummary(context.Background(), saved.UUID, &Summary{
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 5, End: 3},
		Content:      Content{Title: "Test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MessageRange")
}

func TestPostgresCountSummariesToday_NoRows(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)

	count, err := store.CountSummariesToday(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestPostgresTrackAndCountSummaries(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)
	ctx := context.Background()

	// Track 3 summaries
	for i := 0; i < 3; i++ {
		require.NoError(t, store.TrackSummary(ctx))
	}

	count, err := store.CountSummariesToday(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Track one more — verify increment
	require.NoError(t, store.TrackSummary(ctx))
	count, err = store.CountSummariesToday(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}

func TestPostgresUpdateTags(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)
	ctx := context.Background()

	// Create a session with messages and initial tags
	now := time.Now()
	sess := &session.Session{
		Title:         "Postgres Tag Test",
		CreatedAt:     now,
		LastUpdatedAt: now,
		Tags:          []string{"initial"},
		Messages: []session.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}
	saved, err := store.SaveSession(ctx, sess)
	require.NoError(t, err)

	// Update tags only
	err = store.UpdateTags(ctx, saved.UUID, []string{"initial", "new-tag", "another"})
	require.NoError(t, err)

	// Reload and verify tags updated
	loaded, err := store.LoadSession(ctx, saved.UUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"initial", "new-tag", "another"}, loaded.Tags)

	// Verify messages were NOT modified
	assert.Len(t, loaded.Messages, 2)
	assert.Equal(t, "hello", loaded.Messages[0].Content)
}

func TestPostgresUpdateTagsNotFound(t *testing.T) {
	pool, userID := setupTestDB(t)
	store := NewPostgresStorage(pool, userID)
	ctx := context.Background()

	err := store.UpdateTags(ctx, uuid.New().String(), []string{"tag"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
