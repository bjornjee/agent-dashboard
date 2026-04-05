.PHONY: build build-web fmt vet test test-race install install-web clean seed web help

VERSION := $(shell v=$$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'); [ -n "$$v" ] && echo "$$v" || cat VERSION)
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"
ADAPTER ?= claude-code

build: ## Build the dashboard binary
	go build $(LDFLAGS) -o bin/agent-dashboard ./cmd/dashboard/
	@if [ "$$(uname)" = "Darwin" ]; then codesign -s - bin/agent-dashboard; fi

build-web: ## Build the web server binary
	go build -o bin/agent-dashboard-web ./cmd/web/

fmt: ## Auto-format Go source files
	gofmt -w .

vet: ## Run go vet (checks formatting + vets)
	@unformatted=$$(gofmt -l .); \
	  if [ -n "$$unformatted" ]; then \
	    echo "Unformatted files (run make fmt):"; \
	    echo "$$unformatted"; \
	    exit 1; \
	  fi
	@manifest_ver=$$(sed -n 's/.*"\."[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' .release-please-manifest.json 2>/dev/null); \
	  file_ver=$$(cat VERSION 2>/dev/null | tr -d '[:space:]'); \
	  if [ -n "$$manifest_ver" ] && [ -n "$$file_ver" ] && [ "$$manifest_ver" != "$$file_ver" ]; then \
	    echo "VERSION file ($$file_ver) is out of sync with .release-please-manifest.json ($$manifest_ver)"; \
	    echo "Run: echo $$manifest_ver > VERSION"; \
	    exit 1; \
	  fi
	go vet ./...

test: vet ## Run all tests (vets first)
	CGO_ENABLED=0 go test ./...

test-race: vet ## Run tests with race detector (CI only, requires codesigned binaries)
	go test -race ./...

install: ## Build and install binary + adapter (ADAPTER=claude-code)
	./install.sh $(ADAPTER)

install-web: build-web ## Install web server binary
	cp bin/agent-dashboard-web ~/.local/bin/

web: build-web ## Run web server locally
	./bin/agent-dashboard-web --port 8390

clean: ## Remove build artifacts and state
	rm -rf bin/
	rm -rf ~/.agent-dashboard/agents/

seed: ## Seed fake agent state for testing
	@mkdir -p ~/.agent-dashboard/agents
	@echo '{"target":"skills:0.0","tmux_pane_id":"%0","session":"skills","window":0,"pane":0,"state":"done","cwd":"/tmp/skills","branch":"main","files_changed":["+index.js"],"last_message_preview":"All tests pass.","session_id":"fake-session-1","started_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'","model":"claude-sonnet-4-6","permission_mode":"default","subagent_count":0,"last_hook_event":"Stop","updated_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/.agent-dashboard/agents/fake-session-1.json
	@echo '{"target":"api:1.0","tmux_pane_id":"%1","session":"api","window":1,"pane":0,"state":"input","cwd":"/tmp/api","branch":"feat/auth","files_changed":["~config.ts"],"last_message_preview":"Which auth provider?","session_id":"fake-session-2","started_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'","model":"claude-opus-4-6","permission_mode":"default","subagent_count":0,"last_hook_event":"PermissionRequest","updated_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/.agent-dashboard/agents/fake-session-2.json

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
