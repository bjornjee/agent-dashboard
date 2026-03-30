#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

echo "Installing agent-dashboard..."

# Build the Go binary
if command -v go &>/dev/null; then
  make build
  echo "Done! Binary at bin/agent-dashboard"
  echo ""
  echo "Add to .tmux.conf:"
  echo "  run-shell $(pwd)/agent-dashboard.tmux"
  echo ""
  echo "Then reload: tmux source-file ~/.tmux.conf"
  echo "Launch with: prefix + D"
else
  echo "Error: Go is required to build the dashboard."
  echo "Install Go: https://go.dev/dl/"
  exit 1
fi
