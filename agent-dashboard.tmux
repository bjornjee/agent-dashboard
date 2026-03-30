#!/usr/bin/env bash

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASHBOARD_BIN="$PLUGIN_DIR/bin/agent-dashboard"
SESSION_NAME="dashboard"

# Add plugin bin to PATH so the dashboard binary is available
export PATH="$PLUGIN_DIR/bin:$PATH"

# Bind prefix + D to switch to (or create) a dedicated dashboard session
tmux bind-key D run-shell "
  if tmux has-session -t $SESSION_NAME 2>/dev/null; then
    tmux switch-client -t $SESSION_NAME
  else
    tmux new-session -d -s $SESSION_NAME '$DASHBOARD_BIN'
    tmux switch-client -t $SESSION_NAME
  fi
"
