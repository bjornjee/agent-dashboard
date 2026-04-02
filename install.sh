#!/bin/bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HOME/.local/bin"
ADAPTER="${1:-claude-code}"

echo "=== agent-dashboard installer ==="
echo ""

# Validate adapter
if [ ! -d "$REPO_DIR/adapters/$ADAPTER" ]; then
  echo "ERROR: Unknown adapter '$ADAPTER'"
  echo ""
  echo "Available adapters:"
  for d in "$REPO_DIR"/adapters/*/; do
    [ -d "$d" ] && echo "  - $(basename "$d")"
  done
  exit 1
fi

# 1. Build the Go binary
echo "[1/2] Building agent-dashboard binary..."
cd "$REPO_DIR"
make build
mkdir -p "$BIN_DIR"
cp bin/agent-dashboard "$BIN_DIR/agent-dashboard"
echo "  Installed to $BIN_DIR/agent-dashboard"

# 2. Install adapter
echo "[2/2] Installing '$ADAPTER' adapter..."
case "$ADAPTER" in
  claude-code)
    SETTINGS="$HOME/.claude/settings.json"
    if [ ! -f "$SETTINGS" ]; then
      echo "  WARNING: $SETTINGS not found. Is Claude Code installed?"
      echo "  You may need to add the plugin manually via: /plugin add bjornjee/agent-dashboard"
    else
      node -e "
        const fs = require('fs');
        const p = '$SETTINGS';
        const s = JSON.parse(fs.readFileSync(p, 'utf8'));

        // Register marketplace source
        s.extraKnownMarketplaces = s.extraKnownMarketplaces || {};
        if (!s.extraKnownMarketplaces['agent-dashboard']) {
          s.extraKnownMarketplaces['agent-dashboard'] = {
            source: { source: 'github', repo: 'bjornjee/agent-dashboard' }
          };
          console.log('  Registered agent-dashboard marketplace.');
        } else {
          console.log('  Marketplace already registered.');
        }

        // Enable the plugin
        s.enabledPlugins = s.enabledPlugins || {};
        if (!s.enabledPlugins['agent-dashboard@agent-dashboard']) {
          s.enabledPlugins['agent-dashboard@agent-dashboard'] = true;
          console.log('  Enabled agent-dashboard plugin.');
        } else {
          console.log('  Plugin already enabled.');
        }

        fs.writeFileSync(p, JSON.stringify(s, null, 2) + '\n');
      "
    fi
    ;;
  *)
    echo "  Adapter '$ADAPTER' has no automatic registration step."
    echo "  See adapters/$ADAPTER/README.md for setup instructions."
    ;;
esac

echo ""
echo "Done. Restart Claude Code sessions to activate hooks."
echo "Run 'agent-dashboard' in a tmux pane to start the dashboard."
echo ""
echo "Alternative: install the plugin via Claude Code directly:"
echo "  /plugin add bjornjee/agent-dashboard"
