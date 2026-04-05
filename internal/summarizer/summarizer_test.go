package summarizer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
	"github.com/robert-nemet/sessionmngr/internal/storage"
	"github.com/robert-nemet/sessionmngr/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockStorage implements storage.Storage for testing
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) SaveSession(ctx context.Context, sess *session.Session) (*session.Session, error) {
	args := m.Called(ctx, sess)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}

func (m *MockStorage) LoadSession(ctx context.Context, uuid string) (*session.Session, error) {
	args := m.Called(ctx, uuid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}

func (m *MockStorage) FindSessionByPrefix(ctx context.Context, prefix string) (*session.Session, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}

func (m *MockStorage) SessionExists(ctx context.Context, uuid string) bool {
	args := m.Called(ctx, uuid)
	return args.Bool(0)
}

func (m *MockStorage) DeleteSession(ctx context.Context, uuid string) error {
	args := m.Called(ctx, uuid)
	return args.Error(0)
}

func (m *MockStorage) ListSessions(ctx context.Context, page int, perPage int, tags []string) ([]storage.SessionSummary, int, int, error) {
	args := m.Called(ctx, page, perPage, tags)
	return args.Get(0).([]storage.SessionSummary), args.Int(1), args.Int(2), args.Error(3)
}

func (m *MockStorage) SaveSummary(ctx context.Context, sessionID string, sum *storage.Summary) error {
	args := m.Called(ctx, sessionID, sum)
	return args.Error(0)
}

func (m *MockStorage) ListSummaries(ctx context.Context, sessionID string) ([]storage.SummaryEntry, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).([]storage.SummaryEntry), args.Error(1)
}

func (m *MockStorage) LoadAllSummaries(ctx context.Context, sessionID string) ([]storage.Summary, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).([]storage.Summary), args.Error(1)
}

func (m *MockStorage) LoadSummary(ctx context.Context, sessionID, summaryID string) (*storage.Summary, error) {
	args := m.Called(ctx, sessionID, summaryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Summary), args.Error(1)
}

func (m *MockStorage) DeleteSummary(ctx context.Context, sessionID, summaryID string) error {
	args := m.Called(ctx, sessionID, summaryID)
	return args.Error(0)
}

func (m *MockStorage) ValidateAPIKey(ctx context.Context, keyHash, userID string) (bool, error) {
	args := m.Called(ctx, keyHash, userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) FindSessionByTitle(_ context.Context, _ string) (*session.Session, error) {
	return nil, fmt.Errorf("not found")
}

func (m *MockStorage) CountSummariesToday(_ context.Context) (int, error) { return 0, nil }

func (m *MockStorage) TrackSummary(_ context.Context) error { return nil }

func (m *MockStorage) UpdateTags(ctx context.Context, sessionID string, tags []string) error {
	args := m.Called(ctx, sessionID, tags)
	return args.Error(0)
}

// MockSummaryClient implements SummaryClient for testing
type MockSummaryClient struct {
	mock.Mock
}

func (m *MockSummaryClient) GenerateSummary(ctx context.Context, messages []session.Message) (*SummaryResult, error) {
	args := m.Called(ctx, messages)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SummaryResult), args.Error(1)
}

func (m *MockSummaryClient) GenerateIncrementalResume(ctx context.Context, priorResume string, delta []session.Message) (*SummaryResult, error) {
	args := m.Called(ctx, priorResume, delta)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SummaryResult), args.Error(1)
}

func (m *MockSummaryClient) GetModelName() string    { return "test-model" }
func (m *MockSummaryClient) GetPromptSource() string { return "type:resume" }

func newTestConfig(t *testing.T) config.Config {
	t.Helper()
	testutil.SetEnvForTest(t, "SUMMARIZER_MIN_MESSAGES", "1")
	return config.NewConfig()
}

func TestSummarizeMessages_AutoTagsCallsUpdateTags(t *testing.T) {
	mockStore := new(MockStorage)
	mockClient := new(MockSummaryClient)
	cfg := newTestConfig(t)

	sess := &session.Session{
		UUID:  "test-session-uuid-full",
		Title: "Test Session",
		Tags:  []string{"existing"},
		Messages: []session.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
			{Role: "user", Content: "msg3"},
			{Role: "assistant", Content: "msg4"},
			{Role: "user", Content: "msg5"},
			{Role: "assistant", Content: "msg6"},
			{Role: "user", Content: "msg7"},
			{Role: "assistant", Content: "msg8"},
			{Role: "user", Content: "msg9"},
			{Role: "assistant", Content: "msg10"},
		},
	}

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).Return(&SummaryResult{
		Title:    "Test Summary",
		Markdown: "# Test Summary\n\nContent here.",
		Tags:     []string{"golang", "debugging", "existing"},
		Usage:    TokenUsage{InputTokens: 100, OutputTokens: 50},
	}, nil)

	// UpdateTags should be called with merged tags (existing + new, no duplicates)
	mockStore.On("UpdateTags", mock.Anything, "test-session-uuid-full", []string{"existing", "golang", "debugging"}).Return(nil)
	mockStore.On("SaveSummary", mock.Anything, "test-session-uuid-full", mock.Anything).Return(nil)

	s := NewWithClient(mockStore, mockClient, cfg)

	sum, err := s.summarizeMessages(context.Background(), sess, 0, 10, mockClient, "")
	require.NoError(t, err)
	assert.Equal(t, "Test Summary", sum.Content.Title)

	// Verify UpdateTags was called (not SaveSession)
	mockStore.AssertCalled(t, "UpdateTags", mock.Anything, "test-session-uuid-full", []string{"existing", "golang", "debugging"})
	mockStore.AssertNotCalled(t, "SaveSession", mock.Anything, mock.Anything)
}

func TestSummarizeMessages_NoTagsSkipsUpdateTags(t *testing.T) {
	mockStore := new(MockStorage)
	mockClient := new(MockSummaryClient)
	cfg := newTestConfig(t)

	sess := &session.Session{
		UUID:  "test-session-uuid-full",
		Title: "Test Session",
		Messages: []session.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
			{Role: "user", Content: "msg3"},
			{Role: "assistant", Content: "msg4"},
			{Role: "user", Content: "msg5"},
			{Role: "assistant", Content: "msg6"},
			{Role: "user", Content: "msg7"},
			{Role: "assistant", Content: "msg8"},
			{Role: "user", Content: "msg9"},
			{Role: "assistant", Content: "msg10"},
		},
	}

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).Return(&SummaryResult{
		Title:    "Test Summary",
		Markdown: "# Test Summary\n\nNo tags here.",
		Usage:    TokenUsage{InputTokens: 100, OutputTokens: 50},
	}, nil)

	mockStore.On("SaveSummary", mock.Anything, "test-session-uuid-full", mock.Anything).Return(nil)

	s := NewWithClient(mockStore, mockClient, cfg)

	_, err := s.summarizeMessages(context.Background(), sess, 0, 10, mockClient, "")
	require.NoError(t, err)

	// UpdateTags should NOT be called when no tags extracted
	mockStore.AssertNotCalled(t, "UpdateTags", mock.Anything, mock.Anything, mock.Anything)
}

func TestSummarizeMessages_AllTagsExistSkipsUpdateTags(t *testing.T) {
	mockStore := new(MockStorage)
	mockClient := new(MockSummaryClient)
	cfg := newTestConfig(t)

	sess := &session.Session{
		UUID:  "test-session-uuid-full",
		Title: "Test Session",
		Tags:  []string{"golang", "debugging"},
		Messages: []session.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
			{Role: "user", Content: "msg3"},
			{Role: "assistant", Content: "msg4"},
			{Role: "user", Content: "msg5"},
			{Role: "assistant", Content: "msg6"},
			{Role: "user", Content: "msg7"},
			{Role: "assistant", Content: "msg8"},
			{Role: "user", Content: "msg9"},
			{Role: "assistant", Content: "msg10"},
		},
	}

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).Return(&SummaryResult{
		Title:    "Test Summary",
		Markdown: "# Test Summary\n\nContent.",
		Tags:     []string{"golang", "debugging"},
		Usage:    TokenUsage{InputTokens: 100, OutputTokens: 50},
	}, nil)

	mockStore.On("SaveSummary", mock.Anything, "test-session-uuid-full", mock.Anything).Return(nil)

	s := NewWithClient(mockStore, mockClient, cfg)

	_, err := s.summarizeMessages(context.Background(), sess, 0, 10, mockClient, "")
	require.NoError(t, err)

	// UpdateTags should NOT be called when all tags already exist
	mockStore.AssertNotCalled(t, "UpdateTags", mock.Anything, mock.Anything, mock.Anything)
}

func TestResolveSummaryPrefix(t *testing.T) {
	tests := []struct {
		name      string
		summaries []storage.SummaryEntry
		prefix    string
		wantID    string
		wantErr   string
	}{
		{
			name:   "full UUID returns as-is",
			prefix: "550e8400-e29b-41d4-a716-446655440000",
			wantID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "8-char prefix matches single",
			summaries: []storage.SummaryEntry{
				{ID: "abc12345-1111-2222-3333-444444444444"},
				{ID: "def67890-1111-2222-3333-444444444444"},
			},
			prefix: "abc12345",
			wantID: "abc12345-1111-2222-3333-444444444444",
		},
		{
			name: "prefix matches none",
			summaries: []storage.SummaryEntry{
				{ID: "abc12345-1111-2222-3333-444444444444"},
			},
			prefix:  "xyz",
			wantErr: "summary not found: xyz",
		},
		{
			name: "prefix matches multiple",
			summaries: []storage.SummaryEntry{
				{ID: "abc12345-1111-2222-3333-444444444444"},
				{ID: "abc12345-5555-6666-7777-888888888888"},
			},
			prefix:  "abc12345",
			wantErr: "ambiguous summary prefix 'abc12345' matches 2 summaries",
		},
		{
			name:      "no summaries",
			summaries: []storage.SummaryEntry{},
			prefix:    "abc",
			wantErr:   "summary not found: abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorage)
			mockStore.On("ListSummaries", mock.Anything, "session-123").Return(tt.summaries, nil)

			s := &Summarizer{storage: mockStore}
			gotID, err := s.resolveSummaryPrefix(context.Background(), "session-123", tt.prefix)

			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantID, gotID)
		})
	}
}

func TestSelectSummary(t *testing.T) {
	now := time.Now()
	entries := []storage.SummaryEntry{
		{ID: "old", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "newest", CreatedAt: now},
		{ID: "middle", CreatedAt: now.Add(-1 * time.Hour)},
	}

	tests := []struct {
		name    string
		entries []storage.SummaryEntry
		opts    SelectSummaryOptions
		wantID  string
		wantErr string
	}{
		{
			name:    "explicit summaryID",
			entries: entries,
			opts:    SelectSummaryOptions{SummaryID: "explicit-id"},
			wantID:  "explicit-id",
		},
		{
			name:    "--latest returns most recent",
			entries: entries,
			opts:    SelectSummaryOptions{Latest: true},
			wantID:  "newest",
		},
		{
			name:    "-n 1 returns most recent",
			entries: entries,
			opts:    SelectSummaryOptions{Index: 1},
			wantID:  "newest",
		},
		{
			name:    "-n 2 returns second most recent",
			entries: entries,
			opts:    SelectSummaryOptions{Index: 2},
			wantID:  "middle",
		},
		{
			name:    "-n 3 returns oldest",
			entries: entries,
			opts:    SelectSummaryOptions{Index: 3},
			wantID:  "old",
		},
		{
			name:    "single summary auto-selects",
			entries: []storage.SummaryEntry{{ID: "only-one", CreatedAt: now}},
			opts:    SelectSummaryOptions{},
			wantID:  "only-one",
		},
		{
			name:    "multiple summaries with no flags errors",
			entries: entries,
			opts:    SelectSummaryOptions{},
			wantErr: "session has 3 summaries",
		},
		{
			name:    "no summaries errors",
			entries: []storage.SummaryEntry{},
			opts:    SelectSummaryOptions{},
			wantErr: "no summaries found",
		},
		{
			name:    "index out of range errors",
			entries: entries,
			opts:    SelectSummaryOptions{Index: 10},
			wantErr: "index 10 out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorage)
			mockStore.On("FindSessionByPrefix", mock.Anything, "session-123").Return(&session.Session{UUID: "session-123"}, nil)
			mockStore.On("ListSummaries", mock.Anything, "session-123").Return(tt.entries, nil)

			s := &Summarizer{storage: mockStore}
			gotID, err := s.SelectSummary(context.Background(), "session-123", tt.opts)

			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantID, gotID)
		})
	}
}
