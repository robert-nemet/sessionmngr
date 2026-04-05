# Session Manager MCP Server

A stateless Go-based MCP server for recording Claude conversations across multiple sessions.

## Status

**Current Phase:** Phase 3F - Prompt Tuning & Summary Generation (2 weeks)

### Phase 1: Foundation ✅ COMPLETE
- ✅ Stateless multi-session architecture
- ✅ 5 MCP tools (start, switch, sync, list, version)
- ✅ Immediate disk writes
- ✅ Configurable input validation
- ✅ Index auto-recovery
- ✅ 17 passing unit tests
- ✅ Tagged v0.2.0-alpha

### Phase 2: Bug Fixes & CI/CD ✅ COMPLETE
- ✅ Fix switch-session to return full conversation context to LLM
- ✅ GitHub Actions CI/CD (build, test, lint)
- ✅ Multi-platform releases (Linux, macOS - amd64/arm64)
- ✅ GoReleaser configuration

### Phase 3: Summarization (File-Based) ✅ COMPLETE
- ✅ Storage extensions for summaries
- ✅ Summarizer core (Anthropic + OpenAI support)
- ✅ CLI tool (`session-summarizer`)
- ✅ 5 MCP summarizer tools (summarize, list, get, export, delete)
- ✅ 4 specialized prompt types (operational, business, troubleshooting, evaluation)
- See [PLANv3.md](docs/PLANv3.md) for complete plan

### Phase 3F: Prompt Tuning 🔄 CURRENT (Jan 21 - Feb 4)
- Generate 20+ summaries across all prompt types
- Validate prompt quality with real sessions
- Tune prompts based on output evaluation
- Current: 25 sessions, 16 summaries generated

### Phase 4.1: Postgres Storage ✅ COMPLETE
- ✅ Docker Compose + Liquibase migrations
- ✅ PostgresStorage implementation
- ✅ Migration tool (`session-manager-migrate`)
- ✅ 25 sessions and 16 summaries migrated

### Phase 4.1b: Minimal HTTP Transport ✅ COMPLETE
- ✅ HTTP/SSE transport using MCP SDK
- ✅ API key authentication (Bearer token)
- ✅ Per-request user scoping via `X-User-ID` header
- ✅ Docker Compose deployment with Caddy
- ✅ Ready for multi-user testing

### Phase 5: Multi-User Service 🔮 FUTURE
- Scale to 100 beta users
- Per-user API keys with database auth
- Usage limits enforcement
- Daemon mode for async summarization

## Build

```bash
make build
# Binaries:
#   build/session-manager-mcp    - MCP server
#   build/session-summarizer     - CLI summarization tool
#   build/session-manager-migrate - File to Postgres migration tool
```

## Installation

### From Binary Releases (Recommended)

1. Download the latest release for your platform from [Releases](https://github.com/robert-nemet/session-manager-mcp/releases)

2. Extract the archive:
   ```bash
   tar -xzf session-manager-mcp_v*_*.tar.gz
   ```

3. Move the binary to a permanent location:
   ```bash
   # System-wide (requires sudo)
   sudo mv session-manager-mcp /usr/local/bin/
   
   # Or user-local
   mkdir -p ~/bin
   mv session-manager-mcp ~/bin/
   ```

4. Verify installation:
   ```bash
   session-manager-mcp --version 2>&1 || echo "Binary ready"
   ```

### From Source

```bash
git clone https://github.com/robert-nemet/session-manager-mcp.git
cd session-manager-mcp
make build VERSION=v0.2.0
# Binary: build/session-manager-mcp
```

### Claude Desktop (HTTP — Remote Server)

Connect to the hosted server at `mcp.rnemet.dev`. You need a user UUID and API key — request these from the server admin.

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "session-manager": {
      "transport": "http",
      "url": "https://mcp.rnemet.dev/session-manager/mcp",
      "headers": {
        "Authorization": "Bearer <your-api-key>",
        "X-User-ID": "<your-user-uuid>"
      }
    }
  }
}
```

Restart Claude Desktop after editing.

### Claude Desktop (Local Binary)

```json
{
  "mcpServers": {
    "session-manager": {
      "command": "/path/to/session-manager-mcp",
      "env": {
        "SESSIONS_MCP_STORAGE": "~/claude-sessions",
        "SESSIONS_MCP_LOG_LEVEL": "info"
      }
    }
  }
}
```

**Note**: Replace `/path/to/session-manager-mcp` with the full path to your binary:
- If installed to `/usr/local/bin`: use `/usr/local/bin/session-manager-mcp`
- If installed to `~/bin`: use full path like `/Users/yourname/bin/session-manager-mcp`
- If built from source: use `build/session-manager-mcp` or full path to build directory

Restart Claude Desktop.

## Configuration

### MCP Server

Environment variables:

- `SESSIONS_MCP_STORAGE` - Storage directory (default: ~/claude-sessions)
- `SESSIONS_MCP_LOG_LEVEL` - debug, info, warn, error (default: info)
- `SESSIONS_MCP_LOG_LOCATION` - stdio or path (default: <storage>/logs)
- `SESSIONS_MCP_MAX_MESSAGES` - Max messages per sync (default: 1000)
- `SESSIONS_MCP_MAX_CONTENT_LENGTH` - Max bytes per message (default: 100000)

### Postgres Storage

To use PostgreSQL instead of file-based storage:

- `STORAGE_TYPE` - Set to `postgres` to use PostgreSQL (default: `file`)
- `DATABASE_URL` - PostgreSQL connection string (required when `STORAGE_TYPE=postgres`)
- `DEFAULT_USER_ID` - UUID of the user for multi-user support (required for Postgres)

Example Claude Desktop config with Postgres:

```json
{
  "mcpServers": {
    "session-manager": {
      "command": "/path/to/session-manager-mcp",
      "env": {
        "STORAGE_TYPE": "postgres",
        "DATABASE_URL": "postgres://user:pass@host:5432/sessions?sslmode=require",
        "DEFAULT_USER_ID": "your-user-uuid"
      }
    }
  }
}
```

### HTTP Transport (Remote Server)

To run the MCP server as an HTTP service (e.g., deployed on a remote server):

- `TRANSPORT_TYPE` - Set to `http` to use HTTP/SSE transport (default: `stdio`)
- `HTTP_PORT` - Port for HTTP server (default: `8080`)
- `STORAGE_TYPE` - Must be `postgres` (file storage not supported in HTTP mode)
- `DATABASE_URL` - PostgreSQL connection string

**Authentication:** Each user has their own API key(s) stored in the database. The server validates the API key + user ID on every request. See [API Key Management](docs/API_KEY_MANAGEMENT.md) for details.

Example Claude Desktop config with HTTP transport:

```json
{
  "mcpServers": {
    "session-manager": {
      "transport": "http",
      "url": "https://sessions.yourdomain.com/mcp",
      "headers": {
        "Authorization": "Bearer sk_user_specific_key_abc123...",
        "X-User-ID": "550e8400-e29b-41d4-a716-446655440000"
      }
    }
  }
}
```

**Server-side configuration:**

```bash
# Set environment variables
export TRANSPORT_TYPE=http
export HTTP_PORT=8080
export STORAGE_TYPE=postgres
export DATABASE_URL=postgres://user:pass@localhost:5432/session_manager

# Run server
./session-manager-mcp
```

**Creating users and API keys:**

See [docs/API_KEY_MANAGEMENT.md](docs/API_KEY_MANAGEMENT.md) for complete instructions on:
- Creating users
- Generating and hashing API keys
- Managing multiple keys per user
- Revoking compromised keys

See "Remote Deployment (HTTP Transport)" section below for full deployment instructions.

### Migration Tool

Migrate file-based sessions to Postgres:

```bash
# Dry run (preview what would be migrated)
session-manager-migrate --database-url "postgres://..." --user-id "uuid" --dry-run

# Actual migration
session-manager-migrate --database-url "postgres://..." --user-id "uuid"
```

### Summarizer

Environment variables for `session-summarizer` CLI:

- `ANTHROPIC_API_KEY` - Anthropic API key (required for anthropic provider)
- `OPENAI_API_KEY` - OpenAI API key (required for openai provider)
- `SUMMARIZER_PROVIDER` - AI provider: `anthropic` or `openai` (auto-detected from API keys)
- `SUMMARIZER_MODEL` - Model to use (defaults: `claude-sonnet-4-20250514` for Anthropic, `gpt-4o` for OpenAI)
- `SUMMARIZER_MIN_MESSAGES` - Minimum messages required for summarization (default: 10)

**Auto-detection:**
- Provider: If only `OPENAI_API_KEY` is set, uses OpenAI. If `ANTHROPIC_API_KEY` is set, uses Anthropic.
- Model: Defaults to appropriate model for the detected provider.

**Implementation notes:**
- Uses official SDKs: `anthropic-sdk-go` and `openai-go`
- API timeout: 120 seconds
- Max conversation size: 300,000 characters (chunking not yet implemented)

## Tools

### start-session
Creates new session or returns existing session with same title.

**Input:** `{"title": "optional"}`  
**Returns:** `{"session_id": "550e8400", "title": "...", "created_at": "...", "is_existing": false}`

**Note:** If title exists, returns that session with `is_existing: true`.

### switch-session
Switches to existing session by ID prefix (8+ chars) and loads full conversation context.

**Input:** `{"session_id": "550e8400"}`  
**Returns:** `{"session_id": "550e8400", "title": "...", "messages": [...], "message_count": 24, "last_updated_at": "..."}`

**Note:** The `messages` array contains the complete conversation history for LLM context.

### sync-conversation
Saves full conversation to session (overwrites previous). Rejects syncs where message count decreases to prevent data loss.

**Input:** `{"session_id": "550e8400", "messages": [{"role": "user", "content": "..."}]}`
**Returns:** `{"session_id": "550e8400", "synced_count": 12, "last_updated_at": "..."}`

### list-sessions
Lists sessions, paginated (10 per page), sorted by most recent.

**Input:** `{"page": 1}`  
**Returns:** `{"sessions": [...], "page": 1, "total_pages": 5, "total_sessions": 42}`

### session-manager-version
Returns build version and timestamp.

**Returns:** `{"version": "dev", "build_timestamp": "2025-12-20T10:00:00Z"}`

## Storage Structure

### File-Based (Default)

```
~/claude-sessions/
├── {uuid}/
│   ├── raw.json           # Full conversation + metadata
│   └── summaries/         # AI-generated summaries
│       ├── metadata.json  # Summary index
│       └── {uuid}.json    # Individual summary files
├── {uuid}/
│   └── raw.json
└── index.json             # Fast session listing
```

### PostgreSQL

When using `STORAGE_TYPE=postgres`, data is stored in two tables:

- `sessions` - Session metadata and full conversation (JSONB)
- `summaries` - AI-generated summaries linked to sessions

## Session Summarizer CLI

The `session-summarizer` CLI generates AI-powered summaries of conversations for technical documentation.

### Usage

```bash
# Set your API key
export ANTHROPIC_API_KEY=sk-ant-...
# Or for OpenAI:
export OPENAI_API_KEY=sk-...

# List all sessions with summary status
session-summarizer sessions
session-summarizer sessions --all        # Show all (no pagination)
session-summarizer sessions --page 2     # Specific page

# Summarize an entire session
session-summarizer summarize --session abc12345

# Summarize specific message range
session-summarizer summarize --session abc12345 --start 10 --end 50

# List all summaries for a session
session-summarizer list --session abc12345

# Export summary as JSON
session-summarizer export --session abc12345 --summary def45678

# Export summary as Markdown
session-summarizer export --session abc12345 --summary def45678 --format markdown

# Show version
session-summarizer version
```

### Session Discovery

The `sessions` command shows all sessions with their summary status:

```
$ session-summarizer sessions
ID          TITLE                                     MSGS  SUMMARIES
----------  ----------------------------------------  -----  ---------
fd61f66a1c  Implement auth feature                      45          2
abc1234567  Debug database queries                      23          0

Page 1/1 (total: 2 sessions)
```

Use the ID (first 10 chars) with `--session` flag for other commands.

### Summary Format

Summaries are generated as markdown documents:

```markdown
# Database Query Optimization

## Introduction
SELECT query taking 5+ seconds on the users table.

## Analysis
Full table scan occurring due to missing index on frequently queried email column.

## Solution
Added B-tree index on email column:
\`\`\`sql
CREATE INDEX idx_users_email ON users(email);
\`\`\`

## Conclusion
Query time reduced from 5+ seconds to 50ms.
```

## Development & Deployment

### Development Setup

#### Prerequisites

- Go 1.21+
- Docker & Docker Compose (for Postgres mode)
- Liquibase (for database migrations)
- Make

#### Building from Source

```bash
# Clone repository
git clone https://github.com/robert-nemet/session-manager-mcp.git
cd session-manager-mcp

# Build all binaries
make build

# Build with specific version
make build VERSION=v1.0.0

# Install to $GOPATH/bin
make install
```

**Binaries created:**
- `build/session-manager-mcp` - MCP server
- `build/session-summarizer` - CLI summarization tool
- `build/session-manager-migrate` - Migration tool

#### Development with Postgres

**1. Start PostgreSQL:**
```bash
# Start Postgres container
make db-up

# Verify it's running
docker ps | grep session-manager-db
```

**2. Run Migrations:**
```bash
# Apply all migrations
make db-migrate

# Check migration status
make db-migrate-status

# Validate migrations
make db-migrate-validate
```

**3. Connect to Database:**
```bash
# Open psql shell
make db-psql

# View logs
make db-logs
```

**4. Reset Database:**
```bash
# Drop and recreate (deletes all data)
make db-reset
```

**Database Connection Details:**
- Host: `localhost:5432`
- Database: `session_manager`
- User: `sessionmgr`
- Password: `sessionmgr`
- Connection String: `postgres://sessionmgr:sessionmgr@localhost:5432/session_manager?sslmode=disable`

#### Migrate File-Based Sessions to Postgres

```bash
# Preview migration (dry run)
./build/session-manager-migrate \
  --database-url "postgres://sessionmgr:sessionmgr@localhost:5432/session_manager?sslmode=disable" \
  --user-id "your-user-uuid" \
  --dry-run

# Run actual migration
./build/session-manager-migrate \
  --database-url "postgres://sessionmgr:sessionmgr@localhost:5432/session_manager?sslmode=disable" \
  --user-id "your-user-uuid"
```

**Note:** Create a user first:
```sql
-- In psql (make db-psql)
INSERT INTO users (id, username, email, created_at, updated_at)
VALUES (
  'your-user-uuid',
  'your-username',
  'you@example.com',
  NOW(),
  NOW()
);
```

### Deployment

#### Local Deployment (File-Based Storage)

**1. Install Binary:**
```bash
# From releases
tar -xzf session-manager-mcp_v*_*.tar.gz
sudo mv session-manager-mcp /usr/local/bin/

# Or from source
make install
```

**2. Configure Claude Desktop:**

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "session-manager": {
      "command": "/usr/local/bin/session-manager-mcp",
      "env": {
        "SESSIONS_MCP_STORAGE": "~/claude-sessions",
        "SESSIONS_MCP_LOG_LEVEL": "info"
      }
    }
  }
}
```

**3. Restart Claude Desktop**

#### Production Deployment (Postgres Storage)

**1. Setup PostgreSQL Database:**

```bash
# Using managed Postgres (e.g., Hetzner, AWS RDS, DigitalOcean)
# Create database: session_manager
# Create user with full permissions

# Run migrations
cd liquibase
liquibase \
  --url="jdbc:postgresql://your-host:5432/session_manager" \
  --username=your-user \
  --password=your-password \
  update
```

**2. Create User:**

```sql
INSERT INTO users (id, username, email, created_at, updated_at)
VALUES (
  gen_random_uuid(),
  'production-user',
  'user@example.com',
  NOW(),
  NOW()
);
```

**3. Configure Claude Desktop with Postgres:**

```json
{
  "mcpServers": {
    "session-manager": {
      "command": "/usr/local/bin/session-manager-mcp",
      "env": {
        "STORAGE_TYPE": "postgres",
        "DATABASE_URL": "postgres://user:pass@host:5432/session_manager?sslmode=require",
        "DEFAULT_USER_ID": "your-user-uuid",
        "SESSIONS_MCP_LOG_LEVEL": "info"
      }
    }
  }
}
```

**4. Restart Claude Desktop**

#### Remote Deployment (HTTP Transport)

For deploying to a remote server with HTTP transport:

**1. Setup PostgreSQL Database:**

Same as "Production Deployment (Postgres Storage)" above - create database and run migrations.

**2. Create User(s) and API Keys:**

Follow the instructions in [docs/API_KEY_MANAGEMENT.md](docs/API_KEY_MANAGEMENT.md) to:

```sql
-- Example: Create user and API key
BEGIN;

-- Create user
INSERT INTO users (id, name, email, created_at, updated_at)
VALUES (
  gen_random_uuid(),  -- Save this UUID
  'Alice Johnson',
  'alice@example.com',
  NOW(),
  NOW()
)
RETURNING id;  -- Example: 550e8400-e29b-41d4-a716-446655440000

-- Create API key (hash the plaintext key first)
INSERT INTO api_keys (user_id, key_hash, name, created_at, is_active)
VALUES (
  '550e8400-e29b-41d4-a716-446655440000',  -- user ID from above
  'a1b2c3d4...hash-of-key...',              -- SHA-256 hash of API key
  'laptop',
  NOW(),
  true
);

COMMIT;
```

See the API Key Management doc for key generation and hashing commands.

**3. Deploy with Docker Compose:**

Use the provided `docker-compose.prod.yml`:

```bash
# Set environment variables
export DB_PASSWORD=your-secure-password
export DOMAIN=sessions.yourdomain.com

# Deploy
docker compose -f docker-compose.prod.yml up -d
```

The `docker-compose.prod.yml` includes:
- PostgreSQL with pgvector
- MCP server with HTTP transport
- Caddy for automatic HTTPS

**4. Provide Credentials to Users:**

Give each user their:
- User ID (UUID from step 2)
- API key (plaintext, shown only during generation)

Claude Desktop config for each user:

```json
{
  "mcpServers": {
    "session-manager": {
      "transport": "http",
      "url": "https://sessions.yourdomain.com/mcp",
      "headers": {
        "Authorization": "Bearer sk_user_plaintext_key_here",
        "X-User-ID": "550e8400-e29b-41d4-a716-446655440000"
      }
    }
  }
}
```

**5. Restart Claude Desktop**

**Security Notes:**
- Each user has their own API key(s) stored as SHA-256 hashes in database
- Users can have multiple keys (laptop, desktop, etc.)
- Revoke individual keys without affecting user's other keys
- API keys validated against user ID on every request
- Users cannot access other users' sessions
- Caddy automatically provisions Let's Encrypt SSL certificates
- See [docs/API_KEY_MANAGEMENT.md](docs/API_KEY_MANAGEMENT.md) for key rotation and revocation

#### Docker Deployment (Development)

For local development with Postgres:

```bash
# Start services
docker compose up -d

# Run migrations
make db-migrate

# Verify
make db-psql
```

**Production Docker Setup:**

```yaml
# docker-compose.prod.yml
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: session_manager
    ports:
      - "5432:5432"
    volumes:
      - /path/to/persistent/storage:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready"]
      interval: 10s
      timeout: 5s
      retries: 5
```

#### Environment Variables

**Transport (choose one):**
- `TRANSPORT_TYPE` - Transport mode: `stdio` (default) or `http`

**Storage (choose one):**
- `STORAGE_TYPE` - Storage backend: `file` (default) or `postgres`

**Required (Postgres storage):**
- `DATABASE_URL` - Full Postgres connection string
- `DEFAULT_USER_ID` - User UUID (stdio mode only, not used in HTTP mode)

**Required (HTTP transport):**
- `STORAGE_TYPE` - Must be `postgres` (file storage not supported in HTTP mode)
- `DATABASE_URL` - Full Postgres connection string

**Optional (HTTP transport):**
- `HTTP_PORT` - Port number (default: 8080)

**Optional:**
- `SESSIONS_MCP_STORAGE` - File storage location (file mode only, default: ~/claude-sessions)
- `SESSIONS_MCP_LOG_LEVEL` - debug, info, warn, error (default: info)
- `SESSIONS_MCP_LOG_LOCATION` - Path or "stdio" (default: <storage>/logs)
- `SESSIONS_MCP_MAX_MESSAGES` - Max messages per sync (default: 1000)
- `SESSIONS_MCP_MAX_CONTENT_LENGTH` - Max bytes per message (default: 100000)

#### Health Checks

**File Mode:**
```bash
# Check if server responds
ls ~/claude-sessions/index.json

# Check logs
tail -f ~/claude-sessions/logs/*.log
```

**Postgres Mode:**
```bash
# Check database connectivity
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM sessions;"

# View recent sessions
psql "$DATABASE_URL" -c "SELECT id, title, updated_at FROM sessions ORDER BY updated_at DESC LIMIT 5;"
```

### Releasing

#### Release Process

Releases are automated via GitHub Actions when version tags are pushed.

**1. Verify Everything Passes:**

```bash
# Run all checks (same as CI)
make verify

# This runs:
# - go test with race detector and coverage
# - golangci-lint
# - build verification
```

**2. Create and Push Tag:**

```bash
# Using make release (recommended - runs verify, creates tag, pushes)
make release VERSION=v1.0.0

# Or manually:
git tag v1.0.0
git push origin v1.0.0
```

The `make release` command:
- Validates version format
- Runs full verification (`make verify`)
- Creates annotated git tag
- Pushes tag to trigger GitHub Actions

**Tag Format:**
- Must match pattern: `v*.*.*` (e.g., `v1.0.0`, `v0.2.0-alpha`)
- Semantic versioning: `vMAJOR.MINOR.PATCH[-PRERELEASE]`
- Examples: `v1.0.0`, `v0.3.1`, `v2.0.0-beta`, `v1.5.2-rc1`

**3. Automated Release Workflow:**

Once the tag is pushed, GitHub Actions automatically:

1. **Validates** tag format
2. **Runs** full test suite with coverage
3. **Builds** binaries for all platforms:
   - `session-manager-mcp` (Linux/macOS, amd64/arm64)
   - `session-summarizer` (Linux/macOS, amd64/arm64)
4. **Creates** GitHub release with:
   - Compiled binaries (tar.gz archives)
   - Checksums (SHA256)
   - Auto-generated changelog
   - Release notes

**4. Verify Release:**

- Check [Releases page](https://github.com/robert-nemet/session-manager-mcp/releases)
- Download and test binaries
- Verify checksums match

#### Local Release Testing

Test the release process locally before pushing tags:

```bash
# Install GoReleaser
brew install goreleaser

# Create snapshot release (no tag required)
make release-snapshot

# Binaries created in dist/
ls -lh dist/*/session-manager-mcp
```

This creates local builds matching production without publishing.

#### What Gets Released

**Included in release archives:**
- `session-manager-mcp` binary
- `session-summarizer` binary
- `LICENSE` file
- `README.md`

**NOT included:**
- `session-manager-migrate` (build locally if needed: `make build`)
- Source code (available via GitHub)
- Test files

#### Release Notes Guidelines

The `make release` command creates a simple annotated tag. For detailed release notes, manually create the tag:

```bash
git tag -a v1.0.0 -m "Release v1.0.0: Brief description

## Features
- Feature 1
- Feature 2

## Bug Fixes
- Fix 1
- Fix 2

## Breaking Changes
- Change 1 (if any)
"
git push origin v1.0.0
```

The GitHub Actions workflow auto-generates changelog from commit messages using conventional commits.

#### Rollback a Release

If a release has issues:

```bash
# Delete local tag
git tag -d v1.0.0

# Delete remote tag
git push origin :refs/tags/v1.0.0

# Delete GitHub release manually from releases page
```

Then fix issues and recreate the tag.

## Testing

```bash
go test ./...
```

## Known Limitations

- File-based storage has race conditions (use single client or switch to Postgres backend)
- No schema migration from old sessions
- See [docs/archive/STORAGE_ISSUES.md](docs/archive/STORAGE_ISSUES.md) for details

## Documentation

- [PLANv3.md](docs/PLANv3.md) - Complete implementation plan with phases
- [API.md](docs/API.md) - Detailed API documentation
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - System architecture
- [API_KEY_MANAGEMENT.md](docs/API_KEY_MANAGEMENT.md) - User and API key management for HTTP transport
- [archive/](docs/archive/) - Historical planning documents
