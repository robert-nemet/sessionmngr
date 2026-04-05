# Session Manager MCP

An MCP server for recording and resuming Claude conversations across sessions.

Stop re-explaining your infrastructure, codebase, or problem context every time you open a new chat. Session Manager saves your full conversation history and lets you pick up exactly where you left off — across clients, across days.

## How It Works

- **start-session** — create or resume a named session
- **switch-session** — load a past session and resume with full context
- **sync-conversation** — save full conversation to disk
- **append-messages** — incremental sync, preferred over full sync
- **session-status** — check sync state
- **load-complete-session** — load full message history for a session
- **list-sessions** — browse sessions, filter by tags
- **tag-session** — tag sessions for easy retrieval
- **summarize-session** — generate an AI summary of any session
- **list-summaries** — list summaries for a session
- **get-summary** — retrieve a specific summary
- **export-conversation** — export as markdown or JSON
- **delete-session** — remove a session and its summaries
- **session-manager-version** — return build version

Sessions are stored locally as JSON files. No external service required.

## Installation

### From Binary (Recommended)

Download the latest release for your platform from [Releases](https://github.com/robert-nemet/sessionmngr/releases), extract, and move to your PATH:

```bash
tar -xzf session-manager-mcp_*.tar.gz
sudo mv session-manager-mcp /usr/local/bin/
```

### From Source

```bash
git clone https://github.com/robert-nemet/sessionmngr.git
cd sessionmngr
make build
# Binary: build/session-manager-mcp
```

## Setup

Session Manager uses the MCP stdio transport by default. Configure it in your MCP client by pointing to the binary:

```json
{
  "mcpServers": {
    "session-manager": {
      "command": "/usr/local/bin/session-manager-mcp",
      "env": {
        "SESSIONS_MCP_LOG_LEVEL": "info"
      }
    }
  }
}
```

Replace `/usr/local/bin/session-manager-mcp` with the actual path to the binary. Refer to your MCP client's documentation for where to add this configuration.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `SESSIONS_MCP_STORAGE` | `~/mcp-sessions` | Storage directory |
| `SESSIONS_MCP_LOG_LEVEL` | `info` | debug, info, warn, error |
| `SESSIONS_MCP_LOG_LOCATION` | `<storage>/logs` | Log file path or `stdio` |
| `SESSIONS_MCP_MAX_MESSAGES` | `1000` | Max messages per sync |
| `SESSIONS_MCP_MAX_CONTENT_LENGTH` | `100000` | Max bytes per message |
| `ANTHROPIC_API_KEY` | — | Required for AI summarization (Anthropic) |
| `OPENAI_API_KEY` | — | Required for AI summarization (OpenAI) |
| `SUMMARIZER_MODEL` | provider default | Model to use for summarization |

## Storage

Sessions are stored in the directory configured by `SESSIONS_MCP_STORAGE` (defaults to `~/mcp-sessions/`):

```
~/mcp-sessions/
├── {uuid}/
│   ├── raw.json              # Full conversation + metadata
│   └── summaries/
│       ├── {uuid}.json       # Individual summary
│       └── metadata.json     # Summary index
└── index.json                # Session index
```


## Session Summarizer

`session-summarizer` is a CLI for generating AI summaries of sessions.

```bash
export ANTHROPIC_API_KEY=sk-ant-...

# List sessions
session-summarizer sessions

# Summarize a session
session-summarizer summarize --session abc12345

# List summaries
session-summarizer list --session abc12345

# Export as markdown
session-summarizer export --session abc12345 --summary def45678 --format markdown
```

Supports Anthropic and OpenAI. Provider is auto-detected from the API key set.

## Building

```bash
make build       # Build all binaries
make test        # Run tests
make lint        # Run linter
make verify      # Test + lint + build
make release VERSION=v1.0.0  # Tag and push release
```

## License

MIT
