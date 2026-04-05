// Package session provides core types and functionality for managing conversation sessions.
package session

import "time"

// Message represents a single message in a conversation between user and assistant.
type Message struct {
	Role    string `json:"role"`    // Role of the message sender (e.g., "user", "assistant", "system")
	Content string `json:"content"` // Content is the message text
}

// Session represents a complete conversation with messages and metadata.
// Sessions are identified by UUID; titles are user-friendly labels that may be reused.
// The storage layer enforces title uniqueness as a deduplication mechanism.
type Session struct {
	UUID          string    `json:"uuid"`            // UUID is the unique identifier (36 characters)
	Title         string    `json:"title"`           // Title is the session name (must be unique)
	Messages      []Message `json:"messages"`        // Messages contains the full conversation transcript
	CreatedAt     time.Time `json:"created_at"`      // CreatedAt is the session creation timestamp
	LastUpdatedAt time.Time `json:"last_updated_at"` // LastUpdatedAt is the last sync timestamp
	Tags          []string  `json:"tags,omitempty"`  // Tags for categorization (optional)
}

// ShortID returns first 8 chars of UUID for human-friendly display.
// This provides a more convenient identifier for users while maintaining uniqueness.
func (s *Session) ShortID() string {
	if len(s.UUID) >= 8 {
		return s.UUID[:8]
	}
	return s.UUID
}
