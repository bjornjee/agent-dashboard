#!/bin/sh
set -eu

# ---------------------------------------------------------------------------
# agent-dashboard uninstaller
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/uninstall.sh | sh
#   ./uninstall.sh            # interactive (prompts before deleting user data)
#   ./uninstall.sh --yes      # non-interactive (deletes everything)
# ---------------------------------------------------------------------------

BIN_DIR="$HOME/.local/bin"
STATE_DIR="${AGENT_DASHBOARD_DIR:-$HOME/.agent-dashboard}"
CLAUDE_DIR="$HOME/.claude"
SKIP_PROMPT=false

while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) SKIP_PROMPT=true; shift ;;
    *)        shift ;;
  esac
done

info()  { printf '  %s\n' "$@"; }
step()  { printf '[%s] %s\n' "$1" "$2"; }

# ---------------------------------------------------------------------------
# 1. Remove binaries
# ---------------------------------------------------------------------------

step "1/4" "Removing binaries..."
for bin in agent-dashboard agent-dashboard-web; do
  if [ -f "$BIN_DIR/$bin" ]; then
    rm "$BIN_DIR/$bin"
    info "Removed $BIN_DIR/$bin"
  fi
done

# ---------------------------------------------------------------------------
# 2. Remove state directory (prompt unless --yes)
# ---------------------------------------------------------------------------

step "2/4" "Removing state directory..."
if [ -d "$STATE_DIR" ]; then
  # Safety: refuse to delete paths outside $HOME
  case "$STATE_DIR" in
    "$HOME"/*) ;;
    *)
      info "WARNING: STATE_DIR '$STATE_DIR' is not under \$HOME, skipping deletion."
      info "Remove it manually if intended."
      STATE_DIR="" ;;
  esac

  if [ -n "$STATE_DIR" ]; then
    if [ "$SKIP_PROMPT" = true ]; then
      rm -rf "$STATE_DIR"
      info "Removed $STATE_DIR"
    else
      printf '  %s contains settings and usage data. Delete it? [y/N] ' "$STATE_DIR"
      read -r answer </dev/tty 2>/dev/null || answer="n"
      case "$answer" in
        [yY]*) rm -rf "$STATE_DIR"; info "Removed $STATE_DIR" ;;
        *)     info "Kept $STATE_DIR" ;;
      esac
    fi
  fi
else
  info "No state directory found."
fi

# ---------------------------------------------------------------------------
# 3. Remove plugin files
# ---------------------------------------------------------------------------

step "3/4" "Removing plugin files..."
if [ -d "$CLAUDE_DIR/plugins" ]; then
  # Remove marketplace clone (legacy installs)
  if [ -d "$CLAUDE_DIR/plugins/marketplaces/agent-dashboard" ]; then
    rm -rf "$CLAUDE_DIR/plugins/marketplaces/agent-dashboard"
    info "Removed marketplace clone"
  fi

  # Remove plugin cache
  if [ -d "$CLAUDE_DIR/plugins/cache/agent-dashboard" ]; then
    rm -rf "$CLAUDE_DIR/plugins/cache/agent-dashboard"
    info "Removed plugin cache"
  fi
else
  info "No Claude plugins directory found."
fi

# ---------------------------------------------------------------------------
# 4. Deregister from Claude Code JSON files
# ---------------------------------------------------------------------------

step "4/4" "Deregistering plugin..."
if command -v node >/dev/null 2>&1 && [ -d "$CLAUDE_DIR" ]; then

  # --- known_marketplaces.json ---
  known="$CLAUDE_DIR/plugins/known_marketplaces.json"
  if [ -f "$known" ]; then
    AD_FILE_PATH="$known" node -e "
      const fs = require('fs');
      const p = process.env.AD_FILE_PATH;
      const k = JSON.parse(fs.readFileSync(p, 'utf8'));
      if (k['agent-dashboard']) {
        delete k['agent-dashboard'];
        fs.writeFileSync(p, JSON.stringify(k, null, 2) + '\n');
        console.log('  Removed from known_marketplaces.json');
      }
    " 2>/dev/null || true
  fi

  # --- installed_plugins.json ---
  installed="$CLAUDE_DIR/plugins/installed_plugins.json"
  if [ -f "$installed" ]; then
    AD_FILE_PATH="$installed" node -e "
      const fs = require('fs');
      const p = process.env.AD_FILE_PATH;
      const d = JSON.parse(fs.readFileSync(p, 'utf8'));
      if (d.plugins && d.plugins['agent-dashboard@agent-dashboard']) {
        delete d.plugins['agent-dashboard@agent-dashboard'];
        fs.writeFileSync(p, JSON.stringify(d, null, 2) + '\n');
        console.log('  Removed from installed_plugins.json');
      }
    " 2>/dev/null || true
  fi

  # --- settings.json ---
  settings="$CLAUDE_DIR/settings.json"
  if [ -f "$settings" ]; then
    AD_FILE_PATH="$settings" node -e "
      const fs = require('fs');
      const p = process.env.AD_FILE_PATH;
      const s = JSON.parse(fs.readFileSync(p, 'utf8'));
      if (s.enabledPlugins && s.enabledPlugins['agent-dashboard@agent-dashboard']) {
        delete s.enabledPlugins['agent-dashboard@agent-dashboard'];
        fs.writeFileSync(p, JSON.stringify(s, null, 2) + '\n');
        console.log('  Disabled plugin in settings.json');
      }
    " 2>/dev/null || true
  fi

else
  if [ -d "$CLAUDE_DIR" ]; then
    info "node not found — please manually remove agent-dashboard entries from:"
    info "  $CLAUDE_DIR/plugins/known_marketplaces.json"
    info "  $CLAUDE_DIR/plugins/installed_plugins.json"
    info "  $CLAUDE_DIR/settings.json (enabledPlugins)"
  fi
fi

echo ""
echo "Done. Restart Claude Code sessions to complete removal."
