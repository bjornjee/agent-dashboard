#!/bin/sh
set -eu

# ---------------------------------------------------------------------------
# agent-dashboard uninstaller
#
# Removes the binary and state directory. Plugin deregistration is handled
# separately via Claude Code's /plugin command.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/uninstall.sh | sh
#   ./uninstall.sh            # interactive (prompts before deleting user data)
#   ./uninstall.sh --yes      # non-interactive (deletes everything)
# ---------------------------------------------------------------------------

BIN_DIR="$HOME/.local/bin"
STATE_DIR="${AGENT_DASHBOARD_DIR:-$HOME/.agent-dashboard}"
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

step "1/2" "Removing binaries..."
for bin in agent-dashboard agent-dashboard-web; do
  if [ -f "$BIN_DIR/$bin" ]; then
    rm "$BIN_DIR/$bin"
    info "Removed $BIN_DIR/$bin"
  fi
done

# ---------------------------------------------------------------------------
# 2. Remove state directory (prompt unless --yes)
# ---------------------------------------------------------------------------

step "2/2" "Removing state directory..."
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

echo ""
echo "=== Next steps ==="
echo ""
echo "  Remove the plugin from Claude Code (run in any Claude Code session):"
echo ""
echo "     /plugin uninstall agent-dashboard@agent-dashboard"
echo ""
echo "  Optionally remove the marketplace:"
echo ""
echo "     /marketplace remove agent-dashboard"
echo ""
echo "  Then restart Claude Code sessions to complete removal."
echo ""
