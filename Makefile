.PHONY: build test install clean seed help release-patch release-minor release-major

VERSION := $(shell cat VERSION)
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

build: ## Build the dashboard binary
	go build $(LDFLAGS) -o bin/agent-dashboard ./cmd/dashboard/

test: ## Run all tests
	go test -race ./cmd/dashboard/...

install: build ## Install to ~/.local/bin
	@mkdir -p ~/.local/bin
	cp bin/agent-dashboard ~/.local/bin/agent-dashboard

clean: ## Remove build artifacts and state
	rm -rf bin/
	rm -rf ~/.agent-dashboard/agents/

seed: ## Seed fake agent state for testing
	@mkdir -p ~/.agent-dashboard/agents
	@echo '{"target":"skills:0.0","tmux_pane_id":"%0","session":"skills","window":0,"pane":0,"state":"done","cwd":"/tmp/skills","branch":"main","files_changed":["+index.js"],"last_message_preview":"All tests pass.","session_id":"fake-session-1","started_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'","model":"claude-sonnet-4-6","permission_mode":"default","subagent_count":0,"last_hook_event":"Stop","updated_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/.agent-dashboard/agents/fake-session-1.json
	@echo '{"target":"api:1.0","tmux_pane_id":"%1","session":"api","window":1,"pane":0,"state":"input","cwd":"/tmp/api","branch":"feat/auth","files_changed":["~config.ts"],"last_message_preview":"Which auth provider?","session_id":"fake-session-2","started_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'","model":"claude-opus-4-6","permission_mode":"default","subagent_count":0,"last_hook_event":"PermissionRequest","updated_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/.agent-dashboard/agents/fake-session-2.json

define bump_version
	@CURRENT=$$(cat VERSION); \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1); \
	MINOR=$$(echo $$CURRENT | cut -d. -f2); \
	PATCH=$$(echo $$CURRENT | cut -d. -f3); \
	case "$(1)" in \
		patch) PATCH=$$((PATCH + 1));; \
		minor) MINOR=$$((MINOR + 1)); PATCH=0;; \
		major) MAJOR=$$((MAJOR + 1)); MINOR=0; PATCH=0;; \
	esac; \
	NEW="$$MAJOR.$$MINOR.$$PATCH"; \
	echo "$$NEW" > VERSION; \
	git add VERSION; \
	git commit -m "release: v$$NEW"; \
	git tag "v$$NEW"; \
	echo "Bumped $$CURRENT -> $$NEW, tagged v$$NEW"; \
	echo "Run 'git push origin main v$$NEW' to trigger release"
endef

release-patch: test ## Bump patch version, commit, and tag (0.2.0 -> 0.2.1)
	$(call bump_version,patch)

release-minor: test ## Bump minor version, commit, and tag (0.2.0 -> 0.3.0)
	$(call bump_version,minor)

release-major: test ## Bump major version, commit, and tag (0.2.0 -> 1.0.0)
	$(call bump_version,major)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
