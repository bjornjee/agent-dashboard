#!/bin/bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HOME/.local/bin"
ADAPTER="${1:-claude-code}"

# ---------------------------------------------------------------------------
# Adapter install functions
# ---------------------------------------------------------------------------

install_claude_code() {
  local claude_dir="$HOME/.claude"
  local plugins_dir="$claude_dir/plugins"
  local settings="$claude_dir/settings.json"

  if [ ! -d "$claude_dir" ]; then
    echo "  WARNING: $claude_dir not found. Is Claude Code installed?"
    echo "  You may need to add the plugin manually via: /plugin add bjornjee/agent-dashboard"
    return
  fi

  # Ensure plugins directories exist
  mkdir -p "$plugins_dir/marketplaces" "$plugins_dir/cache"

  # --- 1. Clone or update marketplace repo ---
  local mkt_dir="$plugins_dir/marketplaces/agent-dashboard"
  if [ -d "$mkt_dir/.git" ]; then
    echo "  Updating marketplace repo..."
    git -C "$mkt_dir" pull --ff-only --quiet 2>/dev/null || true
  else
    echo "  Cloning marketplace repo..."
    rm -rf "$mkt_dir"
    git clone --quiet https://github.com/bjornjee/agent-dashboard "$mkt_dir"
  fi

  # --- 2. Register in known_marketplaces.json ---
  local known="$plugins_dir/known_marketplaces.json"
  local commit_sha
  commit_sha=$(git -C "$mkt_dir" rev-parse HEAD 2>/dev/null || echo "")
  local now
  now=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)

  node -e "
    const fs = require('fs');
    const p = '$known';
    const k = fs.existsSync(p) ? JSON.parse(fs.readFileSync(p, 'utf8')) : {};
    k['agent-dashboard'] = {
      source: { source: 'github', repo: 'bjornjee/agent-dashboard' },
      installLocation: '$mkt_dir',
      lastUpdated: '$now'
    };
    fs.writeFileSync(p, JSON.stringify(k, null, 2) + '\n');
    console.log('  Updated known_marketplaces.json');
  "

  # --- 3. Install adapter to plugin cache ---
  # Read version from the adapter's plugin.json (authoritative for Claude plugin),
  # falling back to the VERSION file and then 0.0.0.
  local version
  version=$(node -e "console.log(require('$REPO_DIR/adapters/$ADAPTER/.claude-plugin/plugin.json').version)" 2>/dev/null || cat "$REPO_DIR/VERSION" 2>/dev/null || echo "0.0.0")
  local cache_dir="$plugins_dir/cache/agent-dashboard/agent-dashboard/$version"
  mkdir -p "$cache_dir"
  # Copy the adapter contents into the cache
  cp -R "$REPO_DIR/adapters/claude-code/." "$cache_dir/"
  echo "  Installed adapter v$version to plugin cache"

  # --- 4. Register in installed_plugins.json ---
  local installed="$plugins_dir/installed_plugins.json"
  node -e "
    const fs = require('fs');
    const p = '$installed';
    const d = fs.existsSync(p) ? JSON.parse(fs.readFileSync(p, 'utf8')) : { version: 2, plugins: {} };
    d.version = d.version || 2;
    d.plugins = d.plugins || {};

    const key = 'agent-dashboard@agent-dashboard';
    const existing = d.plugins[key] || [];
    // Find or create the user-scope entry
    let entry = existing.find(e => e.scope === 'user');
    const now = '$now';
    if (entry) {
      entry.installPath = '$cache_dir';
      entry.version = '$version';
      entry.lastUpdated = now;
      entry.gitCommitSha = '$commit_sha';
    } else {
      existing.push({
        scope: 'user',
        installPath: '$cache_dir',
        version: '$version',
        installedAt: now,
        lastUpdated: now,
        gitCommitSha: '$commit_sha'
      });
    }
    d.plugins[key] = existing;
    fs.writeFileSync(p, JSON.stringify(d, null, 2) + '\n');
    console.log('  Updated installed_plugins.json');
  "

  # --- 5. Enable in settings.json ---
  if [ -f "$settings" ]; then
    node -e "
      const fs = require('fs');
      const p = '$settings';
      const s = JSON.parse(fs.readFileSync(p, 'utf8'));
      s.enabledPlugins = s.enabledPlugins || {};
      if (!s.enabledPlugins['agent-dashboard@agent-dashboard']) {
        s.enabledPlugins['agent-dashboard@agent-dashboard'] = true;
        fs.writeFileSync(p, JSON.stringify(s, null, 2) + '\n');
        console.log('  Enabled plugin in settings.json');
      } else {
        console.log('  Plugin already enabled in settings.json');
      }
    "
  fi
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
