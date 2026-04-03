#!/bin/bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HOME/.local/bin"
ADAPTER="${1:-claude-code}"

# ---------------------------------------------------------------------------
# Adapter install functions
# ---------------------------------------------------------------------------

install_claude_code() {
  local settings="$HOME/.claude/settings.json"
  if [ ! -f "$settings" ]; then
    echo "  WARNING: $settings not found. Is Claude Code installed?"
    echo "  You may need to add the plugin manually via: /plugin add bjornjee/agent-dashboard"
    return
  fi

  node -e "
    const fs = require('fs');
    const p = '$settings';
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
}

install_generic() {
  echo "  Adapter '$1' has no automatic registration step."
  echo "  See adapters/$1/README.md for setup instructions."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

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
echo "[1/3] Building agent-dashboard binary..."
cd "$REPO_DIR"
make build
mkdir -p "$BIN_DIR"
cp bin/agent-dashboard "$BIN_DIR/agent-dashboard"
echo "  Installed to $BIN_DIR/agent-dashboard"

# 2. Bootstrap default settings
STATE_DIR="${AGENT_DASHBOARD_DIR:-$HOME/.agent-dashboard}"
SETTINGS_FILE="$STATE_DIR/settings.toml"
echo "[2/3] Bootstrapping settings..."
if [ ! -f "$SETTINGS_FILE" ]; then
  mkdir -p "$STATE_DIR"
  cat > "$SETTINGS_FILE" <<'TOML'
# Agent Dashboard settings
# See https://github.com/bjornjee/agent-dashboard for documentation.

[banner]
show_mascot = true   # show the axolotl pixel art
show_quote  = true   # show the daily quote

[notifications]
enabled       = false  # enable desktop notifications
sound         = false  # play alert sound on attention events
silent_events = false  # show notification for non-alerting stops
TOML
  echo "  Created $SETTINGS_FILE"
else
  echo "  $SETTINGS_FILE already exists, skipping."
fi

# 3. Install adapter
echo "[3/3] Installing '$ADAPTER' adapter..."
case "$ADAPTER" in
  claude-code) install_claude_code ;;
  *)           install_generic "$ADAPTER" ;;
esac

echo ""
echo "Done. Restart Claude Code sessions to activate hooks."
echo "Run 'agent-dashboard' in a tmux pane to start the dashboard."
echo ""
echo "Alternative: install the plugin via Claude Code directly:"
echo "  /plugin add bjornjee/agent-dashboard"
