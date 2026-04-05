package storage

import "sort"

// SortSummariesByDate sorts summary entries by CreatedAt descending (most recent first).
// Modifies the slice in place.
func SortSummariesByDate(entries []SummaryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
}
