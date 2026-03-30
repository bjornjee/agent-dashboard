.PHONY: build test install clean seed help

build: ## Build the dashboard binary
	go build -o bin/agent-dashboard ./cmd/dashboard/

test: ## Run all tests
	go test ./cmd/dashboard/...

install: build ## Install to ~/.local/bin
	@mkdir -p ~/.local/bin
	cp bin/agent-dashboard ~/.local/bin/agent-dashboard

clean: ## Remove build artifacts and state
	rm -rf bin/
	rm -f ~/.claude/agent-dashboard/state.json

seed: ## Seed fake agent state for testing
	@mkdir -p ~/.claude/agent-dashboard
	@echo '{"agents":{"skills:0.0":{"target":"skills:0.0","session":"skills","window":0,"pane":0,"state":"done","cwd":"/tmp/skills","branch":"main","files_changed":["+index.js"],"last_message_preview":"All tests pass.","updated_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"},"api:1.0":{"target":"api:1.0","session":"api","window":1,"pane":0,"state":"input","cwd":"/tmp/api","branch":"feat/auth","files_changed":["~config.ts"],"last_message_preview":"Which auth provider?","updated_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}}}' > ~/.claude/agent-dashboard/state.json

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
