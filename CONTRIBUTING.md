# Contributing to agent-dashboard

Thanks for your interest in contributing!

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [tmux](https://github.com/tmux/tmux)
- [Node.js 18+](https://nodejs.org/) (for the Claude Code adapter)

## Development

```bash
# Clone the repo
git clone https://github.com/bjornjee/agent-dashboard
cd agent-dashboard

# Build the binary
make build

# Run tests
make test

# Install locally
make install
```

## Submitting Changes

1. Fork the repo and create a feature branch from `main`
2. Make your changes and ensure `make test` passes
3. Use [conventional commits](https://www.conventionalcommits.org/) for commit messages
4. Open a pull request against `main`

## Project Layout

- `cmd/dashboard/` — Go TUI application (Bubble Tea)
- `adapters/claude-code/` — Claude Code plugin (hooks, skills, agents)
- `schema/` — JSON Schema for agent state

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
