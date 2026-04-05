package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
	"github.com/robert-nemet/sessionmngr/internal/storage"
	"github.com/robert-nemet/sessionmngr/internal/summarizer"
	"github.com/robert-nemet/sessionmngr/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	tmpDir := t.TempDir()
	testutil.SetEnvForTest(t, config.EnvStorageLocation, tmpDir)
	testutil.SetEnvForTest(t, config.EnvAnthropicAPIKey, tmpDir)
	cfg := config.NewConfig()
	return NewHandlers(cfg)
}

func TestStartSession(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	args := &StartSessionInput{Title: "Test Session"}
	_, resp, err := h.startSession(ctx, nil, args)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.SessionID)
	assert.Len(t, resp.SessionID, 8)
	assert.Equal(t, "Test Session", resp.Title)
	assert.NotEmpty(t, resp.CreatedAt)
}

func TestStartSessionWithEmptyTitle(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	args := &StartSessionInput{Title: ""}
	_, resp, err := h.startSession(ctx, nil, args)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Title, "Title should be auto-generated when empty")
	assert.NotEmpty(t, resp.SessionID)
}

func TestSyncConversation(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start a session first
	startArgs := &StartSessionInput{Title: "Sync Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	sessionID := startResp.SessionID

	// Sync messages
	syncArgs := &SyncConversationInput{
		SessionID: sessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "How are you?"},
		},
	}

	_, syncResp, err := h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	assert.Equal(t, sessionID, syncResp.SessionID)
	assert.Equal(t, 3, syncResp.SyncedCount)
	assert.NotEmpty(t, syncResp.LastUpdatedAt)
}

func TestSyncConversationNoSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	syncArgs := &SyncConversationInput{
		SessionID: "",
		Messages: []session.Message{
			{Role: "user", Content: "Test"},
		},
	}

	_, _, err := h.syncConversation(ctx, nil, syncArgs)
	assert.Error(t, err, "Expected error when session_id is empty")
}

func TestSyncConversationEmptyMessages(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Try empty messages
	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  []session.Message{},
	}

	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	assert.Error(t, err, "Expected error for empty messages")
}

func TestSyncConversationSystemRole(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// System role should be accepted
	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "system", Content: "System message"},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
		},
	}

	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	assert.NoError(t, err, "System role should be accepted")
}

func TestSyncConversationRejectsMessageCountDecrease(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Regression Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Sync with 22 messages
	initialMessages := make([]session.Message, 22)
	for i := 0; i < 22; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		initialMessages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  initialMessages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Attempt sync with 12 messages (should be rejected)
	reducedMessages := make([]session.Message, 12)
	for i := 0; i < 12; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		reducedMessages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  reducedMessages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sync rejected")
	assert.Contains(t, err.Error(), "12")
	assert.Contains(t, err.Error(), "22")
}

func TestSyncConversationAllowsGrowingMessageCount(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Growing Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Sync with 10 messages
	initialMessages := make([]session.Message, 10)
	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		initialMessages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  initialMessages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Sync with 15 messages (should succeed)
	growingMessages := make([]session.Message, 15)
	for i := 0; i < 15; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		growingMessages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  growingMessages,
	}
	_, syncResp, err := h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)
	assert.Equal(t, 15, syncResp.SyncedCount)
}

func TestSyncConversationAllowsSameMessageCount(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Same Count Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Sync with 10 messages
	messages := make([]session.Message, 10)
	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Re-sync with same 10 messages (should succeed)
	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, syncResp, err := h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)
	assert.Equal(t, 10, syncResp.SyncedCount)
}

func TestSyncConversationAllowsNewSessionWithMessages(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start empty session
	startArgs := &StartSessionInput{Title: "New Session Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Sync with 5 messages (0 -> 5 should succeed)
	messages := make([]session.Message, 5)
	for i := 0; i < 5; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, syncResp, err := h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)
	assert.Equal(t, 5, syncResp.SyncedCount)
}

func TestSyncConversationMultipleSyncsGrowing(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Multiple Syncs Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Sync 1: 2 messages (should succeed)
	messages := []session.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
	}
	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Sync 2: 4 messages (should succeed - growing)
	messages = []session.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
		{Role: "user", Content: "Message 2"},
		{Role: "assistant", Content: "Response 2"},
	}
	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Sync 3: 6 messages (should succeed - growing)
	messages = []session.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
		{Role: "user", Content: "Message 2"},
		{Role: "assistant", Content: "Response 2"},
		{Role: "user", Content: "Message 3"},
		{Role: "assistant", Content: "Response 3"},
	}
	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Sync 4: 3 messages (should fail - shrinking)
	messages = []session.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
		{Role: "user", Content: "Message 2"},
	}
	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sync rejected")
}

func TestSyncConversationErrorMessageFormat(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Error Message Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Sync with 20 messages
	initialMessages := make([]session.Message, 20)
	for i := 0; i < 20; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		initialMessages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  initialMessages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Attempt sync with 10 messages
	reducedMessages := make([]session.Message, 10)
	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		reducedMessages[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("Message %d", i+1),
		}
	}

	syncArgs = &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  reducedMessages,
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "10")
	assert.Contains(t, err.Error(), "20")
}

func TestSwitchSession(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Create a session
	startArgs := &StartSessionInput{Title: "Switch Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	sessionID := startResp.SessionID

	// Add some messages
	syncArgs := &SyncConversationInput{
		SessionID: sessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Test"},
			{Role: "assistant", Content: "Response"},
		},
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Switch to the session
	switchArgs := &SwitchSessionInput{SessionID: sessionID}
	_, switchResp, err := h.switchSession(ctx, nil, switchArgs)
	require.NoError(t, err)

	assert.Equal(t, sessionID, switchResp.SessionID)
	assert.Equal(t, "Switch Test", switchResp.Title)
	assert.Equal(t, 2, switchResp.MessageCount)

	// Verify recent messages are returned
	require.Len(t, switchResp.RecentMessages, 2)
	assert.Equal(t, "user", switchResp.RecentMessages[0].Role)
	assert.Equal(t, "Test", switchResp.RecentMessages[0].Content)
	assert.Equal(t, "assistant", switchResp.RecentMessages[1].Role)
	assert.Equal(t, "Response", switchResp.RecentMessages[1].Content)
	assert.Empty(t, switchResp.Resume, "no resume expected without summarizer")
}

func TestSwitchSessionNotFound(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	switchArgs := &SwitchSessionInput{SessionID: "deadbeef"}
	_, _, err := h.switchSession(ctx, nil, switchArgs)
	assert.Error(t, err, "Expected error for non-existent session")
}

func TestListSessions(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Create multiple sessions
	for i := 0; i < 5; i++ {
		args := &StartSessionInput{Title: "Session " + string(rune('A'+i))}
		_, _, err := h.startSession(ctx, nil, args)
		require.NoError(t, err)
	}

	// List sessions
	listArgs := &ListSessionsInput{Page: 1}
	_, listResp, err := h.listSessions(ctx, nil, listArgs)
	require.NoError(t, err)

	assert.Len(t, listResp.Sessions, 5)
	assert.Equal(t, 1, listResp.Page)
	assert.Equal(t, 1, listResp.TotalPages)
	assert.Equal(t, 5, listResp.TotalSessions)

	// Verify short IDs
	for _, sess := range listResp.Sessions {
		assert.Len(t, sess.SessionID, 8)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	listArgs := &ListSessionsInput{Page: 1}
	_, listResp, err := h.listSessions(ctx, nil, listArgs)
	require.NoError(t, err)

	assert.Empty(t, listResp.Sessions)
	assert.Equal(t, 1, listResp.TotalPages)
}

func TestGetVersion(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, resp, err := h.getVersion(ctx, nil, nil)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Version)
	assert.NotEmpty(t, resp.BuildTimestamp)
}

func TestEndToEndWorkflow(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// 1. Start session
	startArgs := &StartSessionInput{Title: "E2E Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)
	sessionID := startResp.SessionID

	// 2. Sync conversation multiple times
	for i := 0; i < 3; i++ {
		syncArgs := &SyncConversationInput{
			SessionID: sessionID,
			Messages: []session.Message{
				{Role: "user", Content: "Message " + string(rune('1'+i))},
				{Role: "assistant", Content: "Response " + string(rune('1'+i))},
			},
		}
		_, _, err := h.syncConversation(ctx, nil, syncArgs)
		require.NoError(t, err)
	}

	// 3. List sessions
	listArgs := &ListSessionsInput{Page: 1}
	_, listResp, err := h.listSessions(ctx, nil, listArgs)
	require.NoError(t, err)
	assert.Len(t, listResp.Sessions, 1)

	// 4. Switch to session
	switchArgs := &SwitchSessionInput{SessionID: sessionID}
	_, switchResp, err := h.switchSession(ctx, nil, switchArgs)
	require.NoError(t, err)
	assert.Equal(t, 2, switchResp.MessageCount)

	// 5. Verify session persisted
	_, err = h.storage.LoadSession(ctx, startResp.SessionID+"00000000")
	assert.Error(t, err, "Expected error loading with wrong full UUID")
}

// ============================================================================
// Summarizer Tool Tests
// ============================================================================

func TestSummarizeSession_NoMessages(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Skip if no summarizer
	if h.summarizer == nil {
		t.Skip("summarizer not available - set ANTHROPIC_API_KEY or OPENAI_API_KEY to run")
	}

	// Start session with no messages
	startArgs := &StartSessionInput{Title: "Test Session"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Try to summarize empty session (should fail - no messages)
	sumArgs := &SummarizeSessionInput{SessionID: startResp.SessionID}
	_, _, err = h.summarizeSession(ctx, nil, sumArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid range")
}

func TestSummarizeSession_InvalidSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	sumArgs := &SummarizeSessionInput{SessionID: "short"}
	_, _, err := h.summarizeSession(ctx, nil, sumArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestListSummaries_NoSummaries(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Test Session"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// List summaries (should return empty)
	listArgs := &ListSummariesInput{SessionID: startResp.SessionID}
	_, listResp, err := h.listSummaries(ctx, nil, listArgs)
	require.NoError(t, err)
	assert.Equal(t, 0, listResp.Count)
	assert.Empty(t, listResp.Summaries)
}

func TestListSummaries_WithSummaries(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session with messages
	startArgs := &StartSessionInput{Title: "Test Session"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Get full session UUID
	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Create test summaries directly via storage
	sum1 := &storage.Summary{
		SessionID: sess.UUID,
		Model:     "test-model",
		CreatedAt: time.Now(),
		MessageRange: storage.MessageRange{
			Start: 0,
			End:   10,
		},
		Content: storage.Content{
			Title:    "Test Summary 1",
			Markdown: "# Summary 1\nContent here",
		},
	}
	err = h.storage.SaveSummary(ctx, sess.UUID, sum1)
	require.NoError(t, err)

	sum2 := &storage.Summary{
		SessionID: sess.UUID,
		Model:     "test-model-2",
		CreatedAt: time.Now(),
		MessageRange: storage.MessageRange{
			Start: 10,
			End:   20,
		},
		Content: storage.Content{
			Title:    "Test Summary 2",
			Markdown: "# Summary 2\nMore content",
		},
	}
	err = h.storage.SaveSummary(ctx, sess.UUID, sum2)
	require.NoError(t, err)

	// List summaries
	listArgs := &ListSummariesInput{SessionID: startResp.SessionID}
	_, listResp, err := h.listSummaries(ctx, nil, listArgs)
	require.NoError(t, err)

	assert.Equal(t, 2, listResp.Count)
	assert.Len(t, listResp.Summaries, 2)
	assert.Equal(t, "Test Summary 1", listResp.Summaries[0].Title)
	assert.Equal(t, "test-model", listResp.Summaries[0].Model)
	assert.Len(t, listResp.Summaries[0].SummaryID, 8, "SummaryID should be 8 chars")
}

func TestListSummaries_InvalidSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	listArgs := &ListSummariesInput{SessionID: "short"}
	_, _, err := h.listSummaries(ctx, nil, listArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestGetSummary_NotFound(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Skip if no summarizer (would fail at different point)
	if h.summarizer == nil {
		t.Skip("summarizer not available - set ANTHROPIC_API_KEY or OPENAI_API_KEY to run")
	}

	// Start session
	startArgs := &StartSessionInput{Title: "Test Session"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Try to get non-existent summary
	getArgs := &GetSummaryInput{
		SessionID: startResp.SessionID,
		SummaryID: "12345678",
	}
	_, _, err = h.getSummary(ctx, nil, getArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetSummary_InvalidSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	getArgs := &GetSummaryInput{
		SessionID: "short",
		SummaryID: "12345678",
	}
	_, _, err := h.getSummary(ctx, nil, getArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestGetSummary_InvalidSummaryID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Test Session"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	getArgs := &GetSummaryInput{
		SessionID: startResp.SessionID,
		SummaryID: "short",
	}
	_, _, err = h.getSummary(ctx, nil, getArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestExportConversation_Success(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Export Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Add messages
	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Export
	exportArgs := &ExportConversationInput{SessionID: startResp.SessionID}
	_, exportResp, err := h.exportConversation(ctx, nil, exportArgs)
	require.NoError(t, err)

	// Verify JSON is valid
	assert.NotEmpty(t, exportResp.Export)
	var exportData map[string]any
	err = json.Unmarshal([]byte(exportResp.Export), &exportData)
	require.NoError(t, err)

	// Check structure
	assert.Equal(t, "1.0", exportData["version"])
	assert.Contains(t, exportData, "session")
	assert.Contains(t, exportData, "exported_at")
}

func TestExportConversation_WithSummaries(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Export With Summaries"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Get full session UUID
	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Add a summary
	sum := &storage.Summary{
		SessionID: sess.UUID,
		Model:     "test-model",
		CreatedAt: time.Now(),
		MessageRange: storage.MessageRange{
			Start: 0,
			End:   10,
		},
		Content: storage.Content{
			Title:    "Test Summary",
			Markdown: "# Summary\nContent here",
		},
	}
	err = h.storage.SaveSummary(ctx, sess.UUID, sum)
	require.NoError(t, err)

	// Export with summaries (default)
	includeSummaries := true
	exportArgs := &ExportConversationInput{
		SessionID:        startResp.SessionID,
		IncludeSummaries: &includeSummaries,
	}
	_, exportResp, err := h.exportConversation(ctx, nil, exportArgs)
	require.NoError(t, err)

	// Verify summaries included
	var exportData map[string]any
	err = json.Unmarshal([]byte(exportResp.Export), &exportData)
	require.NoError(t, err)

	assert.Contains(t, exportData, "summaries")
	summaries := exportData["summaries"].([]any)
	assert.Len(t, summaries, 1)
}

func TestExportConversation_ExcludeSummaries(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Export Without Summaries"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Get full session UUID
	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Add a summary
	sum := &storage.Summary{
		SessionID: sess.UUID,
		Model:     "test-model",
		CreatedAt: time.Now(),
		MessageRange: storage.MessageRange{
			Start: 0,
			End:   10,
		},
		Content: storage.Content{
			Title:    "Test Summary",
			Markdown: "# Summary\nContent here",
		},
	}
	err = h.storage.SaveSummary(ctx, sess.UUID, sum)
	require.NoError(t, err)

	// Export without summaries
	excludeSummaries := false
	exportArgs := &ExportConversationInput{
		SessionID:        startResp.SessionID,
		IncludeSummaries: &excludeSummaries,
	}
	_, exportResp, err := h.exportConversation(ctx, nil, exportArgs)
	require.NoError(t, err)

	// Verify summaries NOT included
	var exportData map[string]any
	err = json.Unmarshal([]byte(exportResp.Export), &exportData)
	require.NoError(t, err)

	assert.NotContains(t, exportData, "summaries")
}

func TestExportConversation_InvalidSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	exportArgs := &ExportConversationInput{SessionID: "short"}
	_, _, err := h.exportConversation(ctx, nil, exportArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestDeleteSession_Success(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Delete Test"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Add messages
	syncArgs := &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Test"},
			{Role: "assistant", Content: "Response"},
		},
	}
	_, _, err = h.syncConversation(ctx, nil, syncArgs)
	require.NoError(t, err)

	// Delete session
	deleteArgs := &DeleteSessionInput{SessionID: startResp.SessionID}
	_, deleteResp, err := h.deleteSession(ctx, nil, deleteArgs)
	require.NoError(t, err)

	assert.Equal(t, startResp.SessionID, deleteResp.SessionID)
	assert.Equal(t, "Delete Test", deleteResp.Title)
	assert.Equal(t, 0, deleteResp.SummariesDeleted)

	// Verify session is gone
	_, err = h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	assert.Error(t, err)
}

func TestDeleteSession_WithSummaries(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session
	startArgs := &StartSessionInput{Title: "Delete With Summaries"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	// Get full session UUID
	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Add summaries
	for i := 0; i < 3; i++ {
		sum := &storage.Summary{
			SessionID: sess.UUID,
			Model:     "test-model",
			CreatedAt: time.Now(),
			MessageRange: storage.MessageRange{
				Start: i * 10,
				End:   (i + 1) * 10,
			},
			Content: storage.Content{
				Title:    fmt.Sprintf("Summary %d", i+1),
				Markdown: fmt.Sprintf("# Summary %d\nContent", i+1),
			},
		}
		err = h.storage.SaveSummary(ctx, sess.UUID, sum)
		require.NoError(t, err)
	}

	// Delete session
	deleteArgs := &DeleteSessionInput{SessionID: startResp.SessionID}
	_, deleteResp, err := h.deleteSession(ctx, nil, deleteArgs)
	require.NoError(t, err)

	assert.Equal(t, 3, deleteResp.SummariesDeleted)

	// Verify session is gone
	_, err = h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	assert.Error(t, err)

	// Verify summaries are gone (session doesn't exist anymore, so this should error)
	_, err = h.storage.ListSummaries(ctx, sess.UUID)
	assert.Error(t, err)
}

func TestDeleteSession_InvalidSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	deleteArgs := &DeleteSessionInput{SessionID: "short"}
	_, _, err := h.deleteSession(ctx, nil, deleteArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestDeleteSession_NotFound(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	deleteArgs := &DeleteSessionInput{SessionID: "deadbeef"}
	_, _, err := h.deleteSession(ctx, nil, deleteArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSafeShortID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal UUID",
			input:    "550e8400-e29b-41d4-a716-446655440000",
			expected: "550e8400",
		},
		{
			name:     "short string",
			input:    "abc",
			expected: "abc",
		},
		{
			name:     "exactly 8 chars",
			input:    "12345678",
			expected: "12345678",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeShortID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateMessageRange(t *testing.T) {
	tests := []struct {
		name      string
		startMsg  int
		endMsg    int
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid zero values (full session)",
			startMsg:  0,
			endMsg:    0,
			wantError: false,
		},
		{
			name:      "valid range",
			startMsg:  10,
			endMsg:    50,
			wantError: false,
		},
		{
			name:      "valid start only",
			startMsg:  10,
			endMsg:    0,
			wantError: false,
		},
		{
			name:      "valid end only",
			startMsg:  0,
			endMsg:    50,
			wantError: false,
		},
		{
			name:      "negative start",
			startMsg:  -5,
			endMsg:    10,
			wantError: true,
			errMsg:    "start_msg must be non-negative",
		},
		{
			name:      "negative end",
			startMsg:  0,
			endMsg:    -10,
			wantError: true,
			errMsg:    "end_msg must be non-negative",
		},
		{
			name:      "start equals end",
			startMsg:  10,
			endMsg:    10,
			wantError: true,
			errMsg:    "start_msg (10) must be less than end_msg (10)",
		},
		{
			name:      "start greater than end",
			startMsg:  50,
			endMsg:    10,
			wantError: true,
			errMsg:    "start_msg (50) must be less than end_msg (10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageRange(tt.startMsg, tt.endMsg)
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// Session Status Tests
// ============================================================================

func TestSessionStatus_WithMessages(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session and sync messages
	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Status Test"})
	require.NoError(t, err)

	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "How are you?"},
		},
	})
	require.NoError(t, err)

	// Get status
	_, resp, err := h.sessionStatus(ctx, nil, &SessionStatusInput{SessionID: startResp.SessionID})
	require.NoError(t, err)

	assert.Equal(t, startResp.SessionID, resp.SessionID)
	assert.Equal(t, 3, resp.MessageCount)
	assert.Equal(t, "user", resp.LastMessageRole)
	assert.NotEmpty(t, resp.LastMessageHash)
	assert.Len(t, resp.LastMessageHash, 16)
	assert.NotEmpty(t, resp.LastSyncedAt)
}

func TestSessionStatus_EmptySession(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Empty Status"})
	require.NoError(t, err)

	_, resp, err := h.sessionStatus(ctx, nil, &SessionStatusInput{SessionID: startResp.SessionID})
	require.NoError(t, err)

	assert.Equal(t, 0, resp.MessageCount)
	assert.Empty(t, resp.LastMessageHash)
	assert.Empty(t, resp.LastMessageRole)
}

func TestSessionStatus_InvalidSessionID(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, _, err := h.sessionStatus(ctx, nil, &SessionStatusInput{SessionID: "short"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestSessionStatus_NotFound(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, _, err := h.sessionStatus(ctx, nil, &SessionStatusInput{SessionID: "deadbeef"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ============================================================================
// Append Messages Tests
// ============================================================================

func TestAppendMessages_ValidatedAppend(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Start session and sync initial messages
	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Append Test"})
	require.NoError(t, err)

	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
	})
	require.NoError(t, err)

	// Validated append with correct after_count
	afterCount := 2
	_, resp, err := h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID:  startResp.SessionID,
		Messages:   []session.Message{{Role: "user", Content: "New message"}},
		AfterCount: &afterCount,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, resp.AppendedCount)
	assert.Equal(t, 3, resp.TotalCount)
	assert.False(t, resp.HasGap)
}

func TestAppendMessages_ValidatedAppendCountMismatch(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Mismatch Test"})
	require.NoError(t, err)

	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
	})
	require.NoError(t, err)

	// Wrong after_count
	afterCount := 5
	_, _, err = h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID:  startResp.SessionID,
		Messages:   []session.Message{{Role: "user", Content: "New message"}},
		AfterCount: &afterCount,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "append rejected")
	assert.Contains(t, err.Error(), "5")
	assert.Contains(t, err.Error(), "2")
}

func TestAppendMessages_LossyAppend(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Lossy Test"})
	require.NoError(t, err)

	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages: []session.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
	})
	require.NoError(t, err)

	// Lossy append (no after_count)
	_, resp, err := h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID: startResp.SessionID,
		Messages:  []session.Message{{Role: "user", Content: "After gap"}},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, resp.AppendedCount)
	assert.Equal(t, 4, resp.TotalCount) // 2 original + 1 gap marker + 1 new
	assert.True(t, resp.HasGap)

	// Verify gap marker is in the session
	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)
	assert.Equal(t, "system", sess.Messages[2].Role)
	assert.Contains(t, sess.Messages[2].Content, "[gap:")
}

func TestAppendMessages_LossyAppendEmptySession(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Lossy Empty"})
	require.NoError(t, err)

	// Lossy append to empty session (no gap marker needed)
	_, resp, err := h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID: startResp.SessionID,
		Messages:  []session.Message{{Role: "user", Content: "First message"}},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, resp.AppendedCount)
	assert.Equal(t, 1, resp.TotalCount)
	assert.False(t, resp.HasGap)
}

func TestAppendMessages_EmptyMessages(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Empty Append"})
	require.NoError(t, err)

	_, _, err = h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID: startResp.SessionID,
		Messages:  []session.Message{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestAppendMessages_EmptyContent(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Empty Content"})
	require.NoError(t, err)

	_, _, err = h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID: startResp.SessionID,
		Messages:  []session.Message{{Role: "user", Content: "  "}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty content")
}

func TestAppendMessages_ValidatedAppendToEmptySession(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Validated Empty"})
	require.NoError(t, err)

	afterCount := 0
	_, resp, err := h.appendMessages(ctx, nil, &AppendMessagesInput{
		SessionID:  startResp.SessionID,
		Messages:   []session.Message{{Role: "user", Content: "First"}},
		AfterCount: &afterCount,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, resp.AppendedCount)
	assert.Equal(t, 1, resp.TotalCount)
	assert.False(t, resp.HasGap)
}

// ============================================================================
// Message Hash Tests
// ============================================================================

func TestMessageHash(t *testing.T) {
	hash := messageHash("How are you?")
	assert.Len(t, hash, 16)

	// Same content produces same hash
	assert.Equal(t, hash, messageHash("How are you?"))

	// Different content produces different hash
	assert.NotEqual(t, hash, messageHash("Different content"))
}

// ============================================================================
// Resume Summary Tests
// ============================================================================

func TestFindResumeSummary_Found(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Resume Find"})
	require.NoError(t, err)

	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Create a resume summary directly in storage
	resumeSum := &storage.Summary{
		ID:           summarizer.ResumeID(sess.UUID),
		SessionID:    sess.UUID,
		Model:        "test-model",
		PromptSource: "type:resume",
		CreatedAt:    time.Now(),
		MessageRange: storage.MessageRange{Start: 0, End: 20},
		Content: storage.Content{
			Title:    "Session Resume",
			Markdown: "## Goal\nTest the resume feature",
		},
	}
	require.NoError(t, h.storage.SaveSummary(ctx, sess.UUID, resumeSum))

	found := h.findResumeSummary(ctx, sess.UUID)
	require.NotNil(t, found)
	assert.Equal(t, "type:resume", found.PromptSource)
	assert.Contains(t, found.Content.Markdown, "Test the resume feature")
}

func TestFindResumeSummary_NotFound(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "No Resume"})
	require.NoError(t, err)

	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	found := h.findResumeSummary(ctx, sess.UUID)
	assert.Nil(t, found)
}

func TestFindResumeSummary_IgnoresNonResume(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Non Resume"})
	require.NoError(t, err)

	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Create a non-resume summary
	sum := &storage.Summary{
		SessionID:    sess.UUID,
		Model:        "test-model",
		PromptSource: "type:operational",
		CreatedAt:    time.Now(),
		MessageRange: storage.MessageRange{Start: 0, End: 10},
		Content: storage.Content{
			Title:    "Operational Summary",
			Markdown: "# Ops\nSome content",
		},
	}
	require.NoError(t, h.storage.SaveSummary(ctx, sess.UUID, sum))

	found := h.findResumeSummary(ctx, sess.UUID)
	assert.Nil(t, found, "should not return non-resume summary")
}

func TestSwitchSession_WithResume(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Switch Resume"})
	require.NoError(t, err)

	sess, err := h.storage.FindSessionByPrefix(ctx, startResp.SessionID)
	require.NoError(t, err)

	// Add messages
	messages := make([]session.Message, 5)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = session.Message{Role: role, Content: fmt.Sprintf("msg %d", i)}
	}
	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	})
	require.NoError(t, err)

	// Create resume summary
	resumeSum := &storage.Summary{
		ID:           summarizer.ResumeID(sess.UUID),
		SessionID:    sess.UUID,
		Model:        "test-model",
		PromptSource: "type:resume",
		CreatedAt:    time.Now(),
		MessageRange: storage.MessageRange{Start: 0, End: 5},
		Content: storage.Content{
			Title:    "Resume",
			Markdown: "## Goal\nBuilding session manager",
		},
	}
	require.NoError(t, h.storage.SaveSummary(ctx, sess.UUID, resumeSum))

	// Switch should return resume
	_, switchResp, err := h.switchSession(ctx, nil, &SwitchSessionInput{SessionID: startResp.SessionID})
	require.NoError(t, err)

	assert.Equal(t, "## Goal\nBuilding session manager", switchResp.Resume)
	assert.Equal(t, 5, switchResp.MessageCount)
	assert.Len(t, switchResp.RecentMessages, 5)
}

func TestSwitchSession_TruncatesRecentMessages(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Truncate Test"})
	require.NoError(t, err)

	// Add 30 messages
	messages := make([]session.Message, 30)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = session.Message{Role: role, Content: fmt.Sprintf("msg %d", i)}
	}
	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  messages,
	})
	require.NoError(t, err)

	_, switchResp, err := h.switchSession(ctx, nil, &SwitchSessionInput{SessionID: startResp.SessionID})
	require.NoError(t, err)

	assert.Equal(t, 30, switchResp.MessageCount)
	assert.Len(t, switchResp.RecentMessages, recentMessageCount, "should return only last %d messages", recentMessageCount)
	// First returned message should be msg 10 (index 30-20=10)
	assert.Equal(t, "msg 10", switchResp.RecentMessages[0].Content)
	assert.Equal(t, "msg 29", switchResp.RecentMessages[len(switchResp.RecentMessages)-1].Content)
}

func TestResumeID_Deterministic(t *testing.T) {
	sessionID := "550e8400-e29b-41d4-a716-446655440000"

	id1 := summarizer.ResumeID(sessionID)
	id2 := summarizer.ResumeID(sessionID)
	assert.Equal(t, id1, id2, "same session should produce same resume ID")

	other := summarizer.ResumeID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	assert.NotEqual(t, id1, other, "different sessions should produce different resume IDs")
}

func TestSummarizeSession_InvalidRange(t *testing.T) {
	h := setupTestHandlers(t)
	ctx := context.Background()

	// Create a session first
	startArgs := &StartSessionInput{Title: "Range Test Session"}
	_, startResp, err := h.startSession(ctx, nil, startArgs)
	require.NoError(t, err)

	tests := []struct {
		name     string
		startMsg int
		endMsg   int
		errMsg   string
	}{
		{
			name:     "negative start_msg",
			startMsg: -5,
			endMsg:   0,
			errMsg:   "start_msg must be non-negative",
		},
		{
			name:     "negative end_msg",
			startMsg: 0,
			endMsg:   -10,
			errMsg:   "end_msg must be non-negative",
		},
		{
			name:     "start greater than end",
			startMsg: 100,
			endMsg:   50,
			errMsg:   "start_msg (100) must be less than end_msg (50)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := &SummarizeSessionInput{
				SessionID: startResp.SessionID,
				StartMsg:  tt.startMsg,
				EndMsg:    tt.endMsg,
			}
			_, _, err := h.summarizeSession(ctx, nil, args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestStartSession_SessionLimitReached(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetEnvForTest(t, config.EnvStorageLocation, tmpDir)
	testutil.SetEnvForTest(t, config.EnvAnthropicAPIKey, "test-key")
	testutil.SetEnvForTest(t, config.EnvMaxSessionsPerUser, "3")
	cfg := config.NewConfig()
	h := NewHandlers(cfg)
	ctx := context.Background()

	// Create 3 sessions (at limit)
	for i := 0; i < 3; i++ {
		_, _, err := h.startSession(ctx, nil, &StartSessionInput{Title: fmt.Sprintf("Session %d", i)})
		require.NoError(t, err)
	}

	// 4th should be rejected
	_, _, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Session 3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session limit reached")

	// Verify no session was created (still 3, not 4)
	_, listResp, err := h.listSessions(ctx, nil, &ListSessionsInput{Page: 1})
	require.NoError(t, err)
	assert.Equal(t, 3, listResp.TotalSessions)
}

func TestStartSession_SessionLimitAllowsDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetEnvForTest(t, config.EnvStorageLocation, tmpDir)
	testutil.SetEnvForTest(t, config.EnvAnthropicAPIKey, "test-key")
	testutil.SetEnvForTest(t, config.EnvMaxSessionsPerUser, "3")
	cfg := config.NewConfig()
	h := NewHandlers(cfg)
	ctx := context.Background()

	// Create 3 sessions (at limit)
	for i := 0; i < 3; i++ {
		_, _, err := h.startSession(ctx, nil, &StartSessionInput{Title: fmt.Sprintf("Session %d", i)})
		require.NoError(t, err)
	}

	// Same title should still work (deduplication, not a new session) — even with 0 messages
	_, resp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Session 0"})
	require.NoError(t, err)
	assert.True(t, resp.IsExisting)
}

func TestStartSession_SessionLimitDoesNotLeakOnReject(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetEnvForTest(t, config.EnvStorageLocation, tmpDir)
	testutil.SetEnvForTest(t, config.EnvAnthropicAPIKey, "test-key")
	testutil.SetEnvForTest(t, config.EnvMaxSessionsPerUser, "2")
	cfg := config.NewConfig()
	h := NewHandlers(cfg)
	ctx := context.Background()

	// Create 2 sessions (at limit)
	for i := 0; i < 2; i++ {
		_, _, err := h.startSession(ctx, nil, &StartSessionInput{Title: fmt.Sprintf("Session %d", i)})
		require.NoError(t, err)
	}

	// Reject 3 attempts — none should leak
	for i := 0; i < 3; i++ {
		_, _, err := h.startSession(ctx, nil, &StartSessionInput{Title: fmt.Sprintf("New %d", i)})
		require.Error(t, err)
	}

	// Still exactly 2 sessions
	_, listResp, err := h.listSessions(ctx, nil, &ListSessionsInput{Page: 1})
	require.NoError(t, err)
	assert.Equal(t, 2, listResp.TotalSessions)
}

func TestSummarizeSession_DailyLimitReached(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetEnvForTest(t, config.EnvStorageLocation, tmpDir)
	testutil.SetEnvForTest(t, config.EnvAnthropicAPIKey, "test-key")
	testutil.SetEnvForTest(t, config.EnvMaxSummariesPerDay, "0") // zero = always at limit
	cfg := config.NewConfig()
	h := NewHandlers(cfg)
	ctx := context.Background()

	// Create a session with messages
	_, startResp, err := h.startSession(ctx, nil, &StartSessionInput{Title: "Limit Test"})
	require.NoError(t, err)

	msgs := make([]session.Message, 20)
	for i := range msgs {
		msgs[i] = session.Message{Role: "user", Content: fmt.Sprintf("msg %d", i)}
	}
	_, _, err = h.syncConversation(ctx, nil, &SyncConversationInput{
		SessionID: startResp.SessionID,
		Messages:  msgs,
	})
	require.NoError(t, err)

	// Summarize should be rejected (limit is 0)
	_, _, err = h.summarizeSession(ctx, nil, &SummarizeSessionInput{
		SessionID: startResp.SessionID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daily summary limit reached")
}
