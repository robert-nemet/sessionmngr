package mcp

import "github.com/robert-nemet/sessionmngr/internal/session"

// StartSessionInput is the input for the start-session tool.
type StartSessionInput struct {
	Title string `json:"title,omitempty" jsonschema:"optional title for session"`
}

// StartSessionOutput is the output for the start-session tool.
type StartSessionOutput struct {
	SessionID  string `json:"session_id" jsonschema:"short session ID (first 8 chars of UUID)"`
	Title      string `json:"title" jsonschema:"session title"`
	CreatedAt  string `json:"created_at" jsonschema:"session creation timestamp"`
	IsExisting bool   `json:"is_existing" jsonschema:"true if continuing existing session with same title"`
}

// SwitchSessionInput is the input for the switch-session tool.
type SwitchSessionInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
}

// SwitchSessionOutput is the output for the switch-session tool.
type SwitchSessionOutput struct {
	SessionID      string            `json:"session_id" jsonschema:"short session ID"`
	Title          string            `json:"title" jsonschema:"session title"`
	Resume         string            `json:"resume,omitempty" jsonschema:"AI-generated session resume for context loading"`
	RecentMessages []session.Message `json:"recent_messages" jsonschema:"last N messages from the session"`
	MessageCount   int               `json:"message_count" jsonschema:"total number of messages"`
	LastUpdatedAt  string            `json:"last_updated_at" jsonschema:"last update timestamp"`
}

// LoadCompleteSessionInput is the input for the load-complete-session tool.
type LoadCompleteSessionInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
}

// LoadCompleteSessionOutput is the output for the load-complete-session tool.
type LoadCompleteSessionOutput struct {
	SessionID     string            `json:"session_id" jsonschema:"short session ID"`
	Title         string            `json:"title" jsonschema:"session title"`
	Messages      []session.Message `json:"messages" jsonschema:"all messages from the session"`
	MessageCount  int               `json:"message_count" jsonschema:"total number of messages"`
	LastUpdatedAt string            `json:"last_updated_at" jsonschema:"last update timestamp"`
}

// SyncConversationInput is the input for the sync-conversation tool.
type SyncConversationInput struct {
	SessionID string            `json:"session_id" jsonschema:"session ID (short or full UUID)"`
	Messages  []session.Message `json:"messages" jsonschema:"COMPLETE conversation history - must include ALL messages from the session not just recent ones"`
}

// SyncConversationOutput is the output for the sync-conversation tool.
type SyncConversationOutput struct {
	SessionID     string `json:"session_id" jsonschema:"short session ID"`
	SyncedCount   int    `json:"synced_count" jsonschema:"number of messages synced"`
	LastUpdatedAt string `json:"last_updated_at" jsonschema:"last update timestamp"`
}

// ListSessionsInput is the input for the list-sessions tool.
type ListSessionsInput struct {
	Page int      `json:"page,omitempty" jsonschema:"page number (default: 1)"`
	Tags []string `json:"tags,omitempty" jsonschema:"filter by tags (returns sessions matching any tag)"`
}

// SessionSummaryOutput represents a single session in list-sessions output.
type SessionSummaryOutput struct {
	SessionID     string   `json:"session_id" jsonschema:"short session ID"`
	Title         string   `json:"title" jsonschema:"session title"`
	MessageCount  int      `json:"message_count" jsonschema:"number of messages"`
	SummaryCount  int      `json:"summary_count" jsonschema:"number of summaries"`
	Tags          []string `json:"tags,omitempty" jsonschema:"session tags"`
	LastUpdatedAt string   `json:"last_updated_at" jsonschema:"last update timestamp"`
}

// ListSessionsOutput is the output for the list-sessions tool.
type ListSessionsOutput struct {
	Sessions      []SessionSummaryOutput `json:"sessions" jsonschema:"list of sessions"`
	Page          int                    `json:"page" jsonschema:"current page number"`
	TotalPages    int                    `json:"total_pages" jsonschema:"total number of pages"`
	TotalSessions int                    `json:"total_sessions" jsonschema:"total number of sessions"`
}

// VersionOutput is the output for the session-manager-version tool.
type VersionOutput struct {
	Version        string `json:"version" jsonschema:"build version"`
	BuildTimestamp string `json:"build_timestamp" jsonschema:"build timestamp in ISO 8601 format"`
}

// ============================================================================
// Summarizer MCP Tools
// ============================================================================

// SummarizeSessionInput is the input for the summarize-session tool.
type SummarizeSessionInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
	Type      string `json:"type,omitempty" jsonschema:"prompt type (resume, operational, business, troubleshooting, evaluation). If empty uses default prompt. When set, start_msg/end_msg are ignored."`
	StartMsg  int    `json:"start_msg,omitempty" jsonschema:"optional range start (0-based, inclusive). Ignored when type is set."`
	EndMsg    int    `json:"end_msg,omitempty" jsonschema:"optional range end (exclusive). Ignored when type is set."`
}

// SummarizeSessionOutput is the output for the summarize-session tool.
type SummarizeSessionOutput struct {
	SummaryID string `json:"summary_id" jsonschema:"short summary ID (first 8 chars)"`
	Title     string `json:"title" jsonschema:"generated summary title"`
	CreatedAt string `json:"created_at" jsonschema:"summary creation timestamp"`
	Model     string `json:"model" jsonschema:"AI model used for summarization"`
	Markdown  string `json:"markdown" jsonschema:"full summary content as markdown"`
}

// ListSummariesInput is the input for the list-summaries tool.
type ListSummariesInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
}

// SummaryEntryOutput represents a single summary in list-summaries output.
type SummaryEntryOutput struct {
	SummaryID string `json:"summary_id" jsonschema:"short summary ID"`
	Title     string `json:"title" jsonschema:"summary title"`
	CreatedAt string `json:"created_at" jsonschema:"creation timestamp"`
	Model     string `json:"model" jsonschema:"AI model used"`
}

// ListSummariesOutput is the output for the list-summaries tool.
type ListSummariesOutput struct {
	Summaries []SummaryEntryOutput `json:"summaries" jsonschema:"list of summaries"`
	Count     int                  `json:"count" jsonschema:"total number of summaries"`
}

// GetSummaryInput is the input for the get-summary tool.
type GetSummaryInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
	SummaryID string `json:"summary_id" jsonschema:"summary ID (8+ chars or full UUID)"`
}

// GetSummaryOutput is the output for the get-summary tool.
type GetSummaryOutput struct {
	SummaryID string `json:"summary_id" jsonschema:"short summary ID"`
	Title     string `json:"title" jsonschema:"summary title"`
	Markdown  string `json:"markdown" jsonschema:"full summary content as markdown"`
	CreatedAt string `json:"created_at" jsonschema:"creation timestamp"`
	Model     string `json:"model" jsonschema:"AI model used"`
}

// ExportConversationInput is the input for the export-conversation tool.
type ExportConversationInput struct {
	SessionID        string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
	IncludeSummaries *bool  `json:"include_summaries,omitempty" jsonschema:"include summaries in export (default: true)"`
}

// ExportConversationOutput is the output for the export-conversation tool.
type ExportConversationOutput struct {
	Export string `json:"export" jsonschema:"JSON string containing session and summaries"`
}

// DeleteSessionInput is the input for the delete-session tool.
type DeleteSessionInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
}

// DeleteSessionOutput is the output for the delete-session tool.
type DeleteSessionOutput struct {
	SessionID        string `json:"session_id" jsonschema:"deleted session ID"`
	Title            string `json:"title" jsonschema:"deleted session title"`
	SummariesDeleted int    `json:"summaries_deleted" jsonschema:"number of summaries deleted"`
}

// ============================================================================
// Incremental Sync Tools (Phase 4.3)
// ============================================================================

// SessionStatusInput is the input for the session-status tool.
type SessionStatusInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
}

// SessionStatusOutput is the output for the session-status tool.
type SessionStatusOutput struct {
	SessionID       string   `json:"session_id" jsonschema:"short session ID"`
	MessageCount    int      `json:"message_count" jsonschema:"number of messages stored"`
	LastMessageHash string   `json:"last_message_hash" jsonschema:"SHA-256 prefix of last message content (16 hex chars)"`
	LastMessageRole string   `json:"last_message_role" jsonschema:"role of the last message"`
	LastSyncedAt    string   `json:"last_synced_at" jsonschema:"last sync timestamp"`
	Tags            []string `json:"tags,omitempty" jsonschema:"session tags"`
}

// TagSessionInput is the input for the tag-session tool.
type TagSessionInput struct {
	SessionID string   `json:"session_id" jsonschema:"session ID (8+ chars or full UUID)"`
	Add       []string `json:"add,omitempty" jsonschema:"tags to add"`
	Remove    []string `json:"remove,omitempty" jsonschema:"tags to remove"`
}

// TagSessionOutput is the output for the tag-session tool.
type TagSessionOutput struct {
	SessionID string   `json:"session_id" jsonschema:"short session ID"`
	Tags      []string `json:"tags" jsonschema:"current tags after update"`
}

// AppendMessagesInput is the input for the append-messages tool.
type AppendMessagesInput struct {
	SessionID  string            `json:"session_id" jsonschema:"session ID (short or full UUID)"`
	Messages   []session.Message `json:"messages" jsonschema:"new messages to append"`
	AfterCount *int              `json:"after_count,omitempty" jsonschema:"expected current message count for validated append (omit for lossy append)"`
}

// AppendMessagesOutput is the output for the append-messages tool.
type AppendMessagesOutput struct {
	SessionID     string `json:"session_id" jsonschema:"short session ID"`
	AppendedCount int    `json:"appended_count" jsonschema:"number of messages appended"`
	TotalCount    int    `json:"total_count" jsonschema:"total messages after append"`
	LastUpdatedAt string `json:"last_updated_at" jsonschema:"last update timestamp"`
	HasGap        bool   `json:"has_gap" jsonschema:"true if a gap marker was inserted (lossy append)"`
}
