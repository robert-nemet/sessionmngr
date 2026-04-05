package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/version"
)

func getServer(cfg config.Config) *mcp.Server {
	return buildServer(NewHandlers(cfg))
}

func getServerWithHandlers(h *Handlers) *mcp.Server {
	return buildServer(h)
}

func buildServer(handlers *Handlers) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "session-manager", Version: version.Version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "start-session",
		Description: `Create a new session and return its short ID.

		IMPORTANT: Sessions with the same title are deduplicated - if a session with the given title
		already exists, that session will be returned instead of creating a new one. This prevents
		accidentally creating duplicate sessions with the same name.`,
	}, handlers.startSession)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "switch-session",
		Description: "Switch to an existing session by ID (short or full UUID)",
	}, handlers.switchSession)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "load-complete-session",
		Description: "Load a complete session with ALL messages. Use when you need the full conversation history. Warning: response can be very large for sessions with many messages.",
	}, handlers.loadCompleteSession)

	mcp.AddTool(server, &mcp.Tool{
		Name: "sync-conversation",
		Description: `Sync full conversation state to specified session (writes immediately to disk).

		IMPORTANT: You MUST send ALL messages from the conversation, not just recent ones.
		The server will reject syncs where the incoming message count is less than what's
		already stored (to prevent data loss). Always include the complete conversation history.

		Prefer using append-messages for incremental sync. Use the sync-session prompt for
		step-by-step guidance on the correct sync workflow.`,
	}, handlers.syncConversation)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list-sessions",
		Description: "List sessions with pagination (sorted by most recent first). Optionally filter by tags.",
	}, handlers.listSessions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tag-session",
		Description: "Add or remove tags on a session. Provide add and/or remove lists.",
	}, handlers.tagSession)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "session-manager-version",
		Description: "Get build version and timestamp information",
	}, handlers.getVersion)

	// Summarizer tools
	mcp.AddTool(server, &mcp.Tool{
		Name: "summarize-session",
		Description: `Generate an AI-powered summary for a session or message range.

		The summary is generated using the configured AI model (Anthropic Claude or OpenAI) and
		automatically saved for future reference. You can summarize the entire session or specify
		a message range. Requires ANTHROPIC_API_KEY or OPENAI_API_KEY to be configured.`,
	}, handlers.summarizeSession)

	mcp.AddTool(server, &mcp.Tool{
		Name: "list-summaries",
		Description: `List all AI-generated summaries for a session.

		Returns metadata for each summary including title, creation date, and model used.
		Use get-summary to retrieve the full summary content.`,
	}, handlers.listSummaries)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get-summary",
		Description: `Retrieve and display a specific summary by ID.

		Returns the full summary content as markdown, which you can read and reference
		in the current conversation.`,
	}, handlers.getSummary)

	mcp.AddTool(server, &mcp.Tool{
		Name: "export-conversation",
		Description: `Export a session with all messages and summaries as JSON.

		The export includes the complete conversation history and optionally all associated
		summaries. Useful for backup, sharing, or migrating sessions between environments.
		Format is compatible with the session storage structure.`,
	}, handlers.exportConversation)

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete-session",
		Description: `Delete a session and all its associated summaries permanently.

		This operation is irreversible. The session and all its summaries will be
		removed from storage. Use export-conversation first if you need a backup.`,
	}, handlers.deleteSession)

	// Incremental sync tools (Phase 4.3)
	mcp.AddTool(server, &mcp.Tool{
		Name: "session-status",
		Description: `Get current session state including message count and last message hash.

		Use this to check the server's state before syncing. Returns message count,
		last message hash (for matching), and last sync timestamp.`,
	}, handlers.sessionStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name: "append-messages",
		Description: `Append new messages to a session (incremental sync).

		Preferred over sync-conversation for ongoing conversations. Two modes:
		- With after_count: validated append, rejects if server message count doesn't match (continuity enforced)
		- Without after_count: lossy append, inserts a gap marker and appends unconditionally

		Use the sync-session prompt for step-by-step guidance on the correct sync workflow.`,
	}, handlers.appendMessages)

	// Resource: session status
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "session-manager://sessions/{session_id}/status",
			Name:        "session-status",
			Description: "Session state including message count and last message hash",
			MIMEType:    "application/json",
		},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			// Extract session_id from URI: session-manager://sessions/{session_id}/status
			uri := req.Params.URI
			parts := strings.Split(uri, "/")
			// URI: session-manager://sessions/{session_id}/status
			// parts: ["session-manager:", "", "sessions", "{session_id}", "status"]
			if len(parts) < 5 {
				return nil, fmt.Errorf("invalid resource URI: %s", uri)
			}
			sessionID := parts[3]

			if err := validateSessionID(sessionID); err != nil {
				return nil, err
			}

			_, output, err := handlers.sessionStatus(ctx, nil, &SessionStatusInput{SessionID: sessionID})
			if err != nil {
				return nil, err
			}

			jsonBytes, err := json.Marshal(output)
			if err != nil {
				return nil, fmt.Errorf("failed to serialize session status: %w", err)
			}

			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						URI:      uri,
						MIMEType: "application/json",
						Text:     string(jsonBytes),
					},
				},
			}, nil
		},
	)

	// Prompt: sync-session
	server.AddPrompt(
		&mcp.Prompt{
			Name:        "sync-session",
			Description: "Step-by-step guide for syncing conversation to session-manager",
			Arguments: []*mcp.PromptArgument{
				{
					Name:        "session_id",
					Description: "The session ID to sync to",
					Required:    true,
				},
			},
		},
		func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			sessionID := req.Params.Arguments["session_id"]
			if sessionID == "" {
				return nil, fmt.Errorf("session_id argument is required")
			}

			promptText := fmt.Sprintf(`Sync your conversation to session %s using these steps:

1. Call session-status with session_id="%s" to get the server's current state.

2. If the server has 0 messages:
   → Call sync-conversation with ALL your messages.

3. If the server has messages:
   → Find the last known message in your context (match by hash from session-status).
   → Call append-messages with only the NEW messages and after_count set to the server's message_count.

4. If append-messages fails (count mismatch):
   → Call sync-conversation with everything you have in context.

5. If sync-conversation also fails (context was compressed, you can't send all messages):
   → Call append-messages WITHOUT after_count (lossy append — a gap marker will be inserted, data loss accepted).`, sessionID, sessionID)

			return &mcp.GetPromptResult{
				Description: "Sync conversation workflow",
				Messages: []*mcp.PromptMessage{
					{
						Role:    mcp.Role("user"),
						Content: &mcp.TextContent{Text: promptText},
					},
				},
			}, nil
		},
	)

	return server
}

// StartServer initializes and runs the MCP server with stdio transport.
// Blocks until the server exits or context is cancelled.
func StartServer(ctx context.Context, cfg config.Config) {
	slog.Debug("initializing MCP server")
	server := getServer(cfg)
	slog.Debug("MCP server initialized, starting stdio transport")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		slog.Error("MCP server error", "error", err)
		log.Fatal(err)
	}
	slog.Debug("MCP server stopped")
}
