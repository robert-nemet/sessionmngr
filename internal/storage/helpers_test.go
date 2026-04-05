package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSortSummariesByDate(t *testing.T) {
	entries := []SummaryEntry{
		{ID: "old", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "new", CreatedAt: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "mid", CreatedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
	}

	SortSummariesByDate(entries)

	assert.Equal(t, "new", entries[0].ID, "first should be newest")
	assert.Equal(t, "mid", entries[1].ID, "second should be middle")
	assert.Equal(t, "old", entries[2].ID, "third should be oldest")
}

// TestSortConsistency verifies that list and export use the same ordering.
// Both CLI commands (list, export -n) depend on this function for consistent indexing.
func TestSortConsistency(t *testing.T) {
	entries := []SummaryEntry{
		{ID: "first-created", CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{ID: "last-created", CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
		{ID: "middle", CreatedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)},
	}

	SortSummariesByDate(entries)

	// Index 0 (--latest or -n 1) should be most recent
	assert.Equal(t, "last-created", entries[0].ID, "index 0 should be most recent")
	// Index 1 (-n 2) should be second most recent
	assert.Equal(t, "middle", entries[1].ID, "index 1 should be second most recent")
}
