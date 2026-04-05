info:
	@echo "Build:"
	@echo "  build             Build all binaries"
	@echo "  build-mcp         Build MCP server binary"
	@echo "  build-summarizer  Build summarizer CLI binary"
	@echo "  clean             Remove build artifacts"
	@echo "  install           Install binaries to GOPATH/bin"
	@echo ""
	@echo "Test & Lint:"
	@echo "  test              Run tests (vet, fmt, test)"
	@echo "  test-ci           Run tests with race detector and coverage"
	@echo "  lint              Run golangci-lint"
	@echo "  verify            Run test-ci + lint + build (pre-commit check)"
	@echo ""
	@echo "Release:"
	@echo "  release           Tag and push release (triggers CI build). Requires VERSION=v*.*.*"
	@echo "  release-snapshot  Build snapshot locally with goreleaser (no publish)"
	@echo ""
	@echo "Database (local):"
	@echo "  db-up             Start local postgres"
	@echo "  db-down           Stop local postgres"
	@echo "  db-reset          Reset local postgres (drops data)"
	@echo "  db-logs           Tail postgres logs"
	@echo "  db-psql           Open psql shell"
	@echo "  db-generate-key   Generate a random API key"
	@echo "  db-create-user    Create user + API key locally. Requires NAME= EMAIL= KEY="
	@echo ""
	@echo "Database (migrations):"
	@echo "  db-migrate        Run Liquibase migrations (local)"
	@echo "  db-migrate-status Show migration status"
	@echo "  db-migrate-rollback Rollback last migration"
	@echo "  db-migrate-validate Validate changelog"

VERSION ?= dev
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/robert-nemet/sessionmngr/internal/version.Version=$(VERSION) -X github.com/robert-nemet/sessionmngr/internal/version.BuildTimestamp=$(BUILD_TIME)

build: clean build-mcp build-summarizer

build-mcp:
	go build -ldflags "$(LDFLAGS)" -o build/session-manager-mcp ./cmd

build-summarizer:
	go build -ldflags "$(LDFLAGS)" -o build/session-summarizer ./cmd/session-summarizer

clean:
	rm -f ./build/*

test:
	go vet ./...
	go fmt ./...
	go test ./... -v

lint:
	@echo "Checking for missing documentation comments..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with:"; \
		echo "  brew install golangci-lint  (macOS)"; \
		echo "  or visit: https://golangci-lint.run/usage/install/"; \
	fi

install:
	go build -ldflags "$(LDFLAGS)" -o $$(go env GOPATH)/bin/session-manager-mcp ./cmd
	go build -ldflags "$(LDFLAGS)" -o $$(go env GOPATH)/bin/session-summarizer ./cmd/session-summarizer
	@echo "Installed to: $$(go env GOPATH)/bin/session-manager-mcp"
	@echo "Installed to: $$(go env GOPATH)/bin/session-summarizer"

# Run tests with race detector and coverage (same as CI)
test-ci:
	go vet ./...
	go fmt ./...
	go test -v -race -coverprofile=coverage.out ./...

# Verify all checks pass (runs same checks as CI: test, lint, build)
verify: test-ci lint
	@echo "Running build verification..."
	@$(MAKE) build VERSION=dev
	@echo "All checks passed! Ready for commit."

# Release snapshot for local testing (same as release workflow builds)
release-snapshot:
	@if command -v goreleaser >/dev/null 2>&1; then \
		goreleaser release --snapshot --clean --skip=publish; \
		echo "Snapshot release created in dist/"; \
		echo "Test binaries:"; \
		ls -lh dist/*/session-manager-mcp | head -5; \
	else \
		echo "goreleaser not installed. Install: brew install goreleaser"; \
		exit 1; \
	fi

# Create and push a new release tag
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "VERSION is required. Usage: make release VERSION=v1.0.0"; \
		exit 1; \
	fi
	@if ! echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$$'; then \
		echo "Invalid version format. Must match v*.*.* (e.g., v1.0.0, v0.2.0-alpha)"; \
		exit 1; \
	fi
	@echo "Creating release $(VERSION)..."
	@echo "Running verification checks..."
	@$(MAKE) verify
	@echo "All checks passed!"
	@echo ""
	@echo "Creating git tag $(VERSION)..."
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@echo "Tag created"
	@echo ""
	@echo "Pushing tag to remote..."
	@git push origin $(VERSION)
	@echo "Tag pushed"

# Database targets
db-up:
	docker compose up -d postgres
	@echo "Waiting for postgres to be ready..."
	@until docker compose exec -T postgres pg_isready -U sessionmgr -d session_manager > /dev/null 2>&1; do \
		sleep 1; \
	done
	@echo "Postgres is ready!"

db-down:
	docker compose down

db-reset:
	docker compose down -v
	$(MAKE) db-up

db-logs:
	docker compose logs -f postgres

db-psql:
	docker compose exec postgres psql -U sessionmgr -d session_manager

# Generate a random API key
db-generate-key:
	@echo "sk_$$(openssl rand -base64 32 | tr -d '=+/' | cut -c1-43)"

# Create a user and API key. Usage: make db-create-user NAME="Alice" EMAIL="alice@example.com" KEY="sk_yourkey"
db-create-user:
	@if [ -z "$(NAME)" ] || [ -z "$(EMAIL)" ] || [ -z "$(KEY)" ]; then \
		echo "Usage: make db-create-user NAME=\"Alice\" EMAIL=\"alice@example.com\" KEY=\"sk_yourkey\""; \
		exit 1; \
	fi
	@HASH=$$(echo -n "$(KEY)" | openssl dgst -sha256 | cut -d' ' -f2); \
	USER_ID=$$(docker compose exec -T postgres psql -U sessionmgr -d session_manager -tAc \
		"INSERT INTO users (name, email) VALUES ('$(NAME)', '$(EMAIL)') RETURNING id;"); \
	docker compose exec -T postgres psql -U sessionmgr -d session_manager -c \
		"INSERT INTO api_keys (user_id, key_hash, name) VALUES ('$$USER_ID', '$$HASH', 'default');"; \
	echo ""; \
	echo "User created:"; \
	echo "  User ID: $$USER_ID"; \
	echo "  API Key: $(KEY)"; \
	echo ""; \
	echo "Claude Desktop / Claude Code config:"; \
	echo "  Authorization: Bearer $(KEY)"; \
	echo "  X-User-ID: $$USER_ID"

# Liquibase targets
db-migrate:
	cd liquibase && liquibase update

db-migrate-status:
	cd liquibase && liquibase status

db-migrate-rollback:
	cd liquibase && liquibase rollback-count 1

db-migrate-validate:
	cd liquibase && liquibase validate

.PHONY: info build build-mcp build-summarizer clean test test-ci lint install verify release release-snapshot db-up db-down db-reset db-logs db-psql db-generate-key db-create-user db-migrate db-migrate-status db-migrate-rollback db-migrate-validate
