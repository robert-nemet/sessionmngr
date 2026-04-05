package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
	"github.com/robert-nemet/sessionmngr/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestStorageWithSession creates a test storage and a sample session
func setupTestStorageWithSession(t *testing.T) (Storage, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	ctx := context.Background()
	testutil.SetEnvForTest(t, config.EnvStorageLocation, tmpDir)
	cfg := config.NewConfig()
	store := NewBasicStorage(cfg)

	// Create a test session
	sess := &session.Session{
		Title: "Test Session for Summaries",
		Messages: []session.Message{
			{Role: "user", Content: "How do I optimize my database?"},
			{Role: "assistant", Content: "You can add indexes to frequently queried columns."},
			{Role: "user", Content: "What kind of index?"},
			{Role: "assistant", Content: "B-tree indexes are most common for equality queries."},
		},
	}

	savedSess, err := store.SaveSession(ctx, sess)
	require.NoError(t, err)

	return store, tmpDir, savedSess.UUID
}

func TestSaveSummary(t *testing.T) {
	store, tmpDir, sessionID := setupTestStorageWithSession(t)

	sum := &Summary{
		// ID not set - storage will generate UUID
		SessionID: sessionID,
		Model:     "claude-3-5-sonnet-20241022",
		CreatedAt: time.Now(),
		MessageRange: MessageRange{
			Start: 0,
			End:   4,
		},
		Content: Content{
			Title:    "Database Optimization",
			Markdown: "# Database Optimization\n\n## Introduction\nNeed to optimize database queries.\n\n## Solution\nAdd B-tree indexes.",
		},
	}

	ctx := context.Background()
	err := store.SaveSummary(ctx, sessionID, sum)
	require.NoError(t, err)

	// Verify storage generated a UUID
	require.NotEmpty(t, sum.ID, "Expected storage to generate ID")
	assert.Len(t, sum.ID, 36, "Expected UUID format (36 chars)")

	// Verify summaries directory was created
	summariesDir := filepath.Join(tmpDir, sessionID, "summaries")
	info, err := os.Stat(summariesDir)
	require.NoError(t, err, "Summaries directory not created")
	assert.True(t, info.IsDir())

	// Verify metadata.json was created
	metadataPath := filepath.Join(summariesDir, "metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	require.NoError(t, err, "metadata.json not created")

	var metadata SummaryMetadata
	require.NoError(t, json.Unmarshal(metadataData, &metadata))

	assert.Equal(t, sessionID, metadata.SessionID)
	require.Len(t, metadata.Summaries, 1)

	// Verify summary file was created with correct naming (UUID-based)
	entry := metadata.Summaries[0]
	assert.Equal(t, sum.ID, entry.ID)

	summaryPath := filepath.Join(summariesDir, fmt.Sprintf("%s.json", sum.ID))
	summaryData, err := os.ReadFile(summaryPath)
	require.NoError(t, err, "Summary file not created")

	var loadedSummary Summary
	require.NoError(t, json.Unmarshal(summaryData, &loadedSummary))

	assert.Equal(t, "Database Optimization", loadedSummary.Content.Title)
	assert.Equal(t, sum.ID, loadedSummary.ID)
}

func TestSaveSummaryValidation(t *testing.T) {
	store, _, sessionID := setupTestStorageWithSession(t)

	tests := []struct {
		name    string
		summary *Summary
		wantErr string
	}{
		{
			name: "missing CreatedAt",
			summary: &Summary{
				SessionID:    sessionID,
				MessageRange: MessageRange{Start: 0, End: 1},
				Content:      Content{Title: "Test"},
			},
			wantErr: "CreatedAt is required",
		},
		{
			name: "missing Title",
			summary: &Summary{
				SessionID:    sessionID,
				CreatedAt:    time.Now(),
				MessageRange: MessageRange{Start: 0, End: 1},
				Content:      Content{},
			},
			wantErr: "Content.Title is required",
		},
		{
			name: "SessionID mismatch",
			summary: &Summary{
				SessionID:    "wrong-session-id",
				CreatedAt:    time.Now(),
				MessageRange: MessageRange{Start: 0, End: 1},
				Content:      Content{Title: "Test"},
			},
			wantErr: fmt.Sprintf("SessionID (wrong-session-id) does not match provided sessionID (%s)", sessionID),
		},
		{
			name: "MessageRange Start negative",
			summary: &Summary{
				SessionID:    sessionID,
				CreatedAt:    time.Now(),
				MessageRange: MessageRange{Start: -1, End: 1},
				Content:      Content{Title: "Test"},
			},
			wantErr: "MessageRange.Start must be >= 0, got: -1",
		},
		{
			name: "MessageRange End <= Start",
			summary: &Summary{
				SessionID:    sessionID,
				CreatedAt:    time.Now(),
				MessageRange: MessageRange{Start: 5, End: 5},
				Content:      Content{Title: "Test"},
			},
			wantErr: "MessageRange.End (5) must be > Start (5)",
		},
		{
			name: "invalid ID format - not UUID",
			summary: &Summary{
				ID:           "not-a-uuid",
				SessionID:    sessionID,
				CreatedAt:    time.Now(),
				MessageRange: MessageRange{Start: 0, End: 1},
				Content:      Content{Title: "Test"},
			},
			wantErr: "ID must be a valid UUID format, got: not-a-uuid",
		},
		{
			name: "invalid ID format - path traversal attempt",
			summary: &Summary{
				ID:           "../../etc/passwd",
				SessionID:    sessionID,
				CreatedAt:    time.Now(),
				MessageRange: MessageRange{Start: 0, End: 1},
				Content:      Content{Title: "Test"},
			},
			wantErr: "ID must be a valid UUID format, got: ../../etc/passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.SaveSummary(context.Background(), sessionID, tt.summary)
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}
}

func TestListSummaries(t *testing.T) {
	store, _, sessionID := setupTestStorageWithSession(t)

	// Create multiple summaries (IDs will be generated by storage)
	summaries := []*Summary{
		{
			SessionID:    sessionID,
			Model:        "claude-3-5-sonnet-20241022",
			CreatedAt:    time.Now(),
			MessageRange: MessageRange{Start: 0, End: 2},
			Content: Content{
				Title:    "First Summary",
				Markdown: "# First Summary\n\n## Problem\nProblem 1\n\n## Solution\nSolution 1",
			},
		},
		{
			SessionID:    sessionID,
			Model:        "gpt-4",
			CreatedAt:    time.Now().Add(time.Second),
			MessageRange: MessageRange{Start: 2, End: 4},
			Content: Content{
				Title:    "Second Summary",
				Markdown: "# Second Summary\n\n## Problem\nProblem 2\n\n## Solution\nSolution 2",
			},
		},
	}

	for _, sum := range summaries {
		require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum))
	}

	// List all summaries
	entries, err := store.ListSummaries(context.Background(), sessionID)
	require.NoError(t, err)

	require.Len(t, entries, 2)

	// Verify SummaryEntry contains expected metadata
	assert.Equal(t, "First Summary", entries[0].Title)
	assert.Equal(t, "claude-3-5-sonnet-20241022", entries[0].Model)
	assert.Equal(t, sessionID, entries[0].SessionID)
}

func TestListSummariesEmpty(t *testing.T) {
	store, _, sessionID := setupTestStorageWithSession(t)

	entries, err := store.ListSummaries(context.Background(), sessionID)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLoadSummary(t *testing.T) {
	store, _, sessionID := setupTestStorageWithSession(t)

	// Create a summary (ID will be generated by storage)
	sum := &Summary{
		SessionID:    sessionID,
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 4},
		Content: Content{
			Title:    "Test Summary",
			Markdown: "# Test Summary\n\n## Problem\nTest Problem\n\n## Solution\nTest Solution",
		},
	}

	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum))
	require.NotEmpty(t, sum.ID, "Expected ID to be generated")

	// Load the summary
	loaded, err := store.LoadSummary(context.Background(), sessionID, sum.ID)
	require.NoError(t, err)

	assert.Equal(t, sum.ID, loaded.ID)
	assert.Equal(t, "claude-3-5-sonnet-20241022", loaded.Model)
	assert.Equal(t, "Test Summary", loaded.Content.Title)
}

func TestSaveSummaryCreatesDirectory(t *testing.T) {
	store, tmpDir, sessionID := setupTestStorageWithSession(t)

	sum := &Summary{
		SessionID:    sessionID,
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 2},
		Content:      Content{Title: "Auto-create directory test"},
	}

	// Verify directory doesn't exist yet
	summariesDir := filepath.Join(tmpDir, sessionID, "summaries")
	_, err := os.Stat(summariesDir)
	require.True(t, os.IsNotExist(err), "Summaries directory should not exist yet")

	// Save summary should create directory
	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum))

	// Verify directory now exists
	info, err := os.Stat(summariesDir)
	require.NoError(t, err, "Summaries directory was not created")
	assert.True(t, info.IsDir())
}

func TestSaveSummaryNonexistentSession(t *testing.T) {
	store, _, _ := setupTestStorageWithSession(t)

	sum := &Summary{
		SessionID:    "nonexistent-uuid",
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 1},
		Content:      Content{Title: "Test"},
	}

	err := store.SaveSummary(context.Background(), "nonexistent-uuid", sum)
	assert.Error(t, err, "Expected error when saving summary for nonexistent session")
}

func TestSaveSummaryNormalizesUUID(t *testing.T) {
	store, tmpDir, sessionID := setupTestStorageWithSession(t)

	// Provide uppercase UUID
	uppercaseUUID := "550E8400-E29B-41D4-A716-446655440000"
	expectedLowercase := "550e8400-e29b-41d4-a716-446655440000"

	sum := &Summary{
		ID:           uppercaseUUID,
		SessionID:    sessionID,
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 2},
		Content: Content{
			Title:    "UUID Normalization Test",
			Markdown: "# UUID Normalization Test\n\nTesting uppercase UUID should be normalized to lowercase.",
		},
	}

	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum))

	// Verify ID was normalized to lowercase
	assert.Equal(t, expectedLowercase, sum.ID)

	// Verify file was created with lowercase filename
	summariesDir := filepath.Join(tmpDir, sessionID, "summaries")
	expectedPath := filepath.Join(summariesDir, fmt.Sprintf("%s.json", expectedLowercase))
	_, err := os.Stat(expectedPath)
	require.NoError(t, err, "Expected file with lowercase filename")

	// Verify metadata has the lowercase ID
	metadata, err := os.ReadFile(filepath.Join(summariesDir, "metadata.json"))
	require.NoError(t, err)

	var meta SummaryMetadata
	require.NoError(t, json.Unmarshal(metadata, &meta))

	require.Len(t, meta.Summaries, 1)
	assert.Equal(t, expectedLowercase, meta.Summaries[0].ID)

	// Verify we can load by the normalized (lowercase) ID
	loaded, err := store.LoadSummary(context.Background(), sessionID, expectedLowercase)
	require.NoError(t, err)
	assert.Equal(t, expectedLowercase, loaded.ID)
}

func TestSaveSummaryUpdate(t *testing.T) {
	store, tmpDir, sessionID := setupTestStorageWithSession(t)

	// Create first summary
	sum := &Summary{
		SessionID:    sessionID,
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 2},
		Content: Content{
			Title:    "Original Title",
			Markdown: "# Original Title\n\nOriginal content.",
		},
	}

	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum))

	// Capture the generated ID
	originalID := sum.ID
	require.NotEmpty(t, originalID, "Expected ID to be generated")

	summariesDir := filepath.Join(tmpDir, sessionID, "summaries")

	// Update the summary with same ID but different content
	updatedSum := &Summary{
		ID:           originalID, // Same ID - this is an update
		SessionID:    sessionID,
		Model:        "gpt-4", // Changed model
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 4},
		Content: Content{
			Title:    "Updated Title",
			Markdown: "# Updated Title\n\nUpdated content.",
		},
	}

	require.NoError(t, store.SaveSummary(context.Background(), sessionID, updatedSum))

	// Verify only one file exists (same filename since ID is same)
	entries, err := os.ReadDir(summariesDir)
	require.NoError(t, err)

	jsonFiles := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" && entry.Name() != "metadata.json" {
			jsonFiles++
		}
	}
	assert.Equal(t, 1, jsonFiles, "Expected 1 summary file after update")

	// Load the summary and verify it has updated content
	loaded, err := store.LoadSummary(context.Background(), sessionID, originalID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", loaded.Content.Title)

	// Verify metadata has only one entry for this ID
	metadata, err := os.ReadFile(filepath.Join(summariesDir, "metadata.json"))
	require.NoError(t, err)

	var meta SummaryMetadata
	require.NoError(t, json.Unmarshal(metadata, &meta))

	require.Len(t, meta.Summaries, 1)
	assert.Equal(t, "Updated Title", meta.Summaries[0].Title)
}

func TestDeleteSummary(t *testing.T) {
	store, tmpDir, sessionID := setupTestStorageWithSession(t)

	// Create a summary
	sum := &Summary{
		SessionID:    sessionID,
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 4},
		Content: Content{
			Title:    "Summary to Delete",
			Markdown: "# Summary to Delete\n\nTest content.",
		},
	}

	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum))

	summaryID := sum.ID
	summariesDir := filepath.Join(tmpDir, sessionID, "summaries")
	summaryPath := filepath.Join(summariesDir, fmt.Sprintf("%s.json", summaryID))

	// Verify file exists
	_, err := os.Stat(summaryPath)
	require.NoError(t, err, "Summary file not created")

	// Delete the summary
	require.NoError(t, store.DeleteSummary(context.Background(), sessionID, summaryID))

	// Verify file is deleted
	_, err = os.Stat(summaryPath)
	assert.True(t, os.IsNotExist(err), "Summary file should be deleted")

	// Verify metadata.json is deleted (was the only summary)
	metadataPath := filepath.Join(summariesDir, "metadata.json")
	_, err = os.Stat(metadataPath)
	assert.True(t, os.IsNotExist(err), "metadata.json should be deleted when no summaries remain")

	// Verify LoadSummary returns error
	_, err = store.LoadSummary(context.Background(), sessionID, summaryID)
	assert.Error(t, err, "Expected error when loading deleted summary")
}

func TestDeleteSummaryKeepsOthers(t *testing.T) {
	store, tmpDir, sessionID := setupTestStorageWithSession(t)

	// Create two summaries
	sum1 := &Summary{
		SessionID:    sessionID,
		Model:        "claude-3-5-sonnet-20241022",
		CreatedAt:    time.Now(),
		MessageRange: MessageRange{Start: 0, End: 2},
		Content:      Content{Title: "First Summary"},
	}
	sum2 := &Summary{
		SessionID:    sessionID,
		Model:        "gpt-4",
		CreatedAt:    time.Now().Add(time.Second),
		MessageRange: MessageRange{Start: 2, End: 4},
		Content:      Content{Title: "Second Summary"},
	}

	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum1))
	require.NoError(t, store.SaveSummary(context.Background(), sessionID, sum2))

	// Delete first summary
	require.NoError(t, store.DeleteSummary(context.Background(), sessionID, sum1.ID))

	// Verify first is gone, second remains
	_, err := store.LoadSummary(context.Background(), sessionID, sum1.ID)
	assert.Error(t, err, "First summary should be deleted")

	loaded, err := store.LoadSummary(context.Background(), sessionID, sum2.ID)
	require.NoError(t, err, "Second summary should still exist")
	assert.Equal(t, "Second Summary", loaded.Content.Title)

	// Verify metadata still exists with one entry
	summariesDir := filepath.Join(tmpDir, sessionID, "summaries")
	metadataPath := filepath.Join(summariesDir, "metadata.json")
	_, err = os.Stat(metadataPath)
	require.NoError(t, err, "metadata.json should still exist")

	entries, err := store.ListSummaries(context.Background(), sessionID)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestDeleteSummaryNotFound(t *testing.T) {
	store, _, sessionID := setupTestStorageWithSession(t)

	err := store.DeleteSummary(context.Background(), sessionID, "nonexistent-id")
	assert.Error(t, err, "Expected error when deleting nonexistent summary")
}

func TestDeleteSummaryNonexistentSession(t *testing.T) {
	store, _, _ := setupTestStorageWithSession(t)

	err := store.DeleteSummary(context.Background(), "nonexistent-session", "some-id")
	assert.Error(t, err, "Expected error when deleting from nonexistent session")
}
