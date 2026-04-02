#!/bin/bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
ADAPTER_DIR="$REPO_DIR/adapters/claude-code"
CLAUDE_PLUGINS="$HOME/.claude/plugins/marketplaces"
SETTINGS="$HOME/.claude/settings.json"
BIN_DIR="$HOME/.local/bin"

echo "=== agent-dashboard installer ==="

# 1. Build the Go binary
echo "[1/3] Building agent-dashboard binary..."
cd "$REPO_DIR"
make build
mkdir -p "$BIN_DIR"
cp bin/agent-dashboard "$BIN_DIR/agent-dashboard"
echo "  Installed to $BIN_DIR/agent-dashboard"

# 2. Symlink the Claude Code adapter as a plugin
echo "[2/3] Installing Claude Code adapter..."
PLUGIN_DIR="$CLAUDE_PLUGINS/agent-dashboard"
if [ -L "$PLUGIN_DIR" ]; then
  echo "  Symlink already exists, updating..."
  rm "$PLUGIN_DIR"
fi
if [ -d "$PLUGIN_DIR" ]; then
  echo "  WARNING: $PLUGIN_DIR exists as a directory, skipping symlink."
  echo "  Remove it manually if you want the symlink: rm -rf $PLUGIN_DIR"
else
  ln -s "$ADAPTER_DIR" "$PLUGIN_DIR"
  echo "  Linked $ADAPTER_DIR -> $PLUGIN_DIR"
fi

# 3. Register the plugin in Claude Code settings
echo "[3/3] Registering plugin in Claude Code settings..."
if [ ! -f "$SETTINGS" ]; then
  echo "  WARNING: $SETTINGS not found. Is Claude Code installed?"
  echo "  You may need to add the plugin manually."
else
  if grep -q '"agent-dashboard@agent-dashboard"' "$SETTINGS" 2>/dev/null; then
    echo "  Plugin already registered."
  else
    node -e "
      const fs = require('fs');
      const p = '$SETTINGS';
      const s = JSON.parse(fs.readFileSync(p, 'utf8'));
      s.enabledPlugins = s.enabledPlugins || {};
      s.enabledPlugins['agent-dashboard@agent-dashboard'] = true;
      fs.writeFileSync(p, JSON.stringify(s, null, 2) + '\n');
    "
    echo "  Enabled agent-dashboard plugin."
  fi
fi

echo ""
echo "Done. Restart Claude Code sessions to activate hooks."
echo "Run 'agent-dashboard' in a tmux pane to start the dashboard."
