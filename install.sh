#!/bin/sh
set -eu

# ---------------------------------------------------------------------------
# agent-dashboard installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
#   ./install.sh              # download pre-built binary + adapter
#   ./install.sh --build      # compile from source (requires Go, run from repo checkout)
# ---------------------------------------------------------------------------

REPO="bjornjee/agent-dashboard"
BIN_DIR="$HOME/.local/bin"
STATE_DIR="${AGENT_DASHBOARD_DIR:-$HOME/.agent-dashboard}"
ADAPTER="claude-code"
BUILD_FROM_SOURCE=false
WORK_DIR=""

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --build)   BUILD_FROM_SOURCE=true; shift ;;
    --adapter) ADAPTER="$2"; shift 2 ;;
    *)         shift ;;
  esac
done

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

info()  { printf '  %s\n' "$@"; }
err()   { printf 'ERROR: %s\n' "$@" >&2; }
step()  { printf '[%s] %s\n' "$1" "$2"; }

check_cmd() {
  command -v "$1" >/dev/null 2>&1
}

cleanup() {
  if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
    rm -rf "$WORK_DIR"
  fi
}

# Returns 0 if version $1 >= $2 (major.minor comparison)
version_ge() {
  major1=$(echo "$1" | cut -d. -f1)
  minor1=$(echo "$1" | cut -d. -f2)
  major2=$(echo "$2" | cut -d. -f1)
  minor2=$(echo "$2" | cut -d. -f2)
  [ "$major1" -gt "$major2" ] 2>/dev/null && return 0
  [ "$major1" -eq "$major2" ] 2>/dev/null && [ "$minor1" -ge "$minor2" ] 2>/dev/null && return 0
  return 1
}

# ---------------------------------------------------------------------------
# Validate adapter name (allowlist)
# ---------------------------------------------------------------------------

validate_adapter() {
  case "$ADAPTER" in
    claude-code) ;;
    *) err "Unknown adapter: $ADAPTER (available: claude-code)"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------

check_prerequisites() {
  missing=""

  if ! check_cmd tmux; then
    missing="$missing  - tmux: https://github.com/tmux/tmux/wiki/Installing\n"
  fi

  if ! check_cmd node; then
    missing="$missing  - node (18+): https://nodejs.org/\n"
  else
    node_ver=$(node -v 2>/dev/null | sed 's/^v//')
    if ! version_ge "$node_ver" "18.0"; then
      missing="$missing  - node 18+ (found $node_ver): https://nodejs.org/\n"
    fi
  fi

  if [ "$BUILD_FROM_SOURCE" = true ]; then
    if ! check_cmd git; then
      missing="$missing  - git: https://git-scm.com/\n"
    fi
    if ! check_cmd go; then
      missing="$missing  - go (1.26+): https://go.dev/dl/\n"
    else
      go_ver=$(go version 2>/dev/null | sed 's/.*go\([0-9][0-9.]*\).*/\1/')
      if ! version_ge "$go_ver" "1.26"; then
        missing="$missing  - go 1.26+ (found $go_ver): https://go.dev/dl/\n"
      fi
    fi
  else
    if ! check_cmd curl; then
      missing="$missing  - curl: required for downloading release assets\n"
    fi
  fi

  if [ -n "$missing" ]; then
    err "Missing prerequisites:"
    printf '%b' "$missing"
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Detect OS and architecture
# ---------------------------------------------------------------------------

detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)      err "Unsupported OS: $OS"; exit 1 ;;
  esac

  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)       ARCH="amd64" ;;
    amd64)        ARCH="amd64" ;;
    arm64)        ARCH="arm64" ;;
    aarch64)      ARCH="arm64" ;;
    *)            err "Unsupported architecture: $ARCH"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Resolve latest release version
# ---------------------------------------------------------------------------

resolve_version() {
  # Try gh CLI first (handles auth/rate limits), fall back to curl
  if check_cmd gh; then
    VERSION=$(gh api "repos/$REPO/releases/latest" --jq '.tag_name' 2>/dev/null | sed 's/^v//') || true
  fi

  if [ -z "${VERSION:-}" ]; then
    VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
      | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p') || true
  fi

  if [ -z "${VERSION:-}" ]; then
    err "Could not determine latest release version."
    err "Check your internet connection or install from source with --build."
    exit 1
  fi

  # Validate version format
  case "$VERSION" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) err "Unexpected version format: $VERSION"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Download and install binary
# ---------------------------------------------------------------------------

install_binary_download() {
  step "1/4" "Downloading agent-dashboard v$VERSION ($OS/$ARCH)..."

  asset="agent-dashboard_${VERSION}_${OS}_${ARCH}.tar.gz"
  url="https://github.com/$REPO/releases/download/v${VERSION}/$asset"

  if ! curl -fsSL "$url" -o "$WORK_DIR/$asset"; then
    err "Failed to download $url"
    if check_cmd go; then
      info "Falling back to building from source..."
      install_binary_build
      return
    fi
    exit 1
  fi

  mkdir -p "$BIN_DIR"
  tar -xzf "$WORK_DIR/$asset" -C "$WORK_DIR"
  cp "$WORK_DIR/agent-dashboard" "$BIN_DIR/agent-dashboard"

  # Ad-hoc codesign on macOS to prevent AMFI/Gatekeeper issues
  if [ "$OS" = "darwin" ]; then
    codesign -f -s - "$BIN_DIR/agent-dashboard" 2>/dev/null || true
    xattr -d com.apple.quarantine "$BIN_DIR/agent-dashboard" 2>/dev/null || true
  fi

  info "Installed to $BIN_DIR/agent-dashboard"
}

install_binary_build() {
  REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
  step "1/4" "Building agent-dashboard from source..."
  cd "$REPO_DIR"
  make build
  mkdir -p "$BIN_DIR"
  cp bin/agent-dashboard "$BIN_DIR/agent-dashboard"
  info "Installed to $BIN_DIR/agent-dashboard"
  cd - >/dev/null
}

# ---------------------------------------------------------------------------
# Download and install adapter
# ---------------------------------------------------------------------------

install_adapter_download() {
  local claude_dir="$HOME/.claude"
  local plugins_dir="$claude_dir/plugins"

  if [ ! -d "$claude_dir" ]; then
    info "WARNING: $claude_dir not found. Is Claude Code installed?"
    info "You may need to add the plugin manually via: /plugin add $REPO"
    return
  fi

  step "2/4" "Downloading adapter..."

  asset="agent-dashboard-adapter-${ADAPTER}.tar.gz"
  url="https://github.com/$REPO/releases/download/v${VERSION}/$asset"

  if ! curl -fsSL "$url" -o "$WORK_DIR/$asset"; then
    err "Failed to download adapter: $url"
    err "You can install the adapter manually via: /plugin add $REPO"
    return
  fi

  # Extract adapter to plugin cache
  local cache_dir="$plugins_dir/cache/agent-dashboard/agent-dashboard/$VERSION"
  mkdir -p "$cache_dir"
  tar -xzf "$WORK_DIR/$asset" -C "$cache_dir"
  info "Installed adapter v$VERSION to plugin cache"

  register_plugin "$VERSION" "$cache_dir"
}

install_adapter_build() {
  REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
  local claude_dir="$HOME/.claude"
  local plugins_dir="$claude_dir/plugins"

  if [ ! -d "$claude_dir" ]; then
    info "WARNING: $claude_dir not found. Is Claude Code installed?"
    info "You may need to add the plugin manually via: /plugin add $REPO"
    return
  fi

  # Read version from the adapter's plugin.json, falling back to git tag, then VERSION file
  local version
  version=$(node -e "console.log(require('$REPO_DIR/adapters/$ADAPTER/.claude-plugin/plugin.json').version)" 2>/dev/null \
    || (cd "$REPO_DIR" && v=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'); [ -z "$v" ] && { git fetch --tags --quiet 2>/dev/null; v=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'); }; [ -n "$v" ] && echo "$v") \
    || sed 's/ *#.*//' "$REPO_DIR/VERSION" 2>/dev/null \
    || echo "0.0.0")

  step "2/4" "Installing adapter from source..."
  local cache_dir="$plugins_dir/cache/agent-dashboard/agent-dashboard/$version"
  mkdir -p "$cache_dir"
  cp -R "$REPO_DIR/adapters/$ADAPTER/." "$cache_dir/"
  info "Installed adapter v$version to plugin cache"

  register_plugin "$version" "$cache_dir"
}

# ---------------------------------------------------------------------------
# Register plugin in Claude Code
# ---------------------------------------------------------------------------

register_plugin() {
  local version="$1"
  local cache_dir="$2"
  local claude_dir="$HOME/.claude"
  local plugins_dir="$claude_dir/plugins"
  local settings="$claude_dir/settings.json"
  local now
  now=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)

  # Ensure plugins directories exist
  mkdir -p "$plugins_dir/marketplaces" "$plugins_dir/cache"

  step "3/4" "Registering plugin..."

  # --- Register in known_marketplaces.json ---
  local known="$plugins_dir/known_marketplaces.json"
  AD_KNOWN_PATH="$known" AD_REPO="$REPO" AD_CACHE_DIR="$cache_dir" AD_NOW="$now" \
    node -e "
      const fs = require('fs');
      const p = process.env.AD_KNOWN_PATH;
      const k = fs.existsSync(p) ? JSON.parse(fs.readFileSync(p, 'utf8')) : {};
      k['agent-dashboard'] = {
        source: { source: 'github', repo: process.env.AD_REPO },
        installLocation: process.env.AD_CACHE_DIR,
        autoUpdate: true,
        lastUpdated: process.env.AD_NOW
      };
      fs.writeFileSync(p, JSON.stringify(k, null, 2) + '\n');
    "
  info "Updated known_marketplaces.json"

  # --- Register in installed_plugins.json ---
  local installed="$plugins_dir/installed_plugins.json"
  AD_INSTALLED_PATH="$installed" AD_CACHE_DIR="$cache_dir" AD_VERSION="$version" AD_NOW="$now" \
    node -e "
      const fs = require('fs');
      const p = process.env.AD_INSTALLED_PATH;
      const d = fs.existsSync(p) ? JSON.parse(fs.readFileSync(p, 'utf8')) : { version: 2, plugins: {} };
      d.version = d.version || 2;
      d.plugins = d.plugins || {};

      const key = 'agent-dashboard@agent-dashboard';
      const existing = d.plugins[key] || [];
      let entry = existing.find(e => e.scope === 'user');
      const now = process.env.AD_NOW;
      if (entry) {
        entry.installPath = process.env.AD_CACHE_DIR;
        entry.version = process.env.AD_VERSION;
        entry.lastUpdated = now;
      } else {
        existing.push({
          scope: 'user',
          installPath: process.env.AD_CACHE_DIR,
          version: process.env.AD_VERSION,
          installedAt: now,
          lastUpdated: now
        });
      }
      d.plugins[key] = existing;
      fs.writeFileSync(p, JSON.stringify(d, null, 2) + '\n');
    "
  info "Updated installed_plugins.json"

  # --- Enable in settings.json ---
  if [ -f "$settings" ]; then
    AD_SETTINGS_PATH="$settings" \
      node -e "
        const fs = require('fs');
        const p = process.env.AD_SETTINGS_PATH;
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

# ---------------------------------------------------------------------------
# Bootstrap settings
# ---------------------------------------------------------------------------

bootstrap_settings() {
  step "4/4" "Bootstrapping settings..."
  local settings_file="$STATE_DIR/settings.toml"
  if [ ! -f "$settings_file" ]; then
    mkdir -p "$STATE_DIR"

    # Try adapter cache first (zero-clone install), then repo root (--build)
    local example=""
    local cache_dir="$HOME/.claude/plugins/cache/agent-dashboard/agent-dashboard"
    if [ -d "$cache_dir" ]; then
      # Find the settings template in the latest cached version
      for d in "$cache_dir"/*/; do
        if [ -f "${d}settings.example.toml" ]; then
          example="${d}settings.example.toml"
        fi
      done
    fi

    # Fall back to repo root (available in --build mode)
    if [ -z "$example" ] && [ -f "$(dirname "$0")/settings.example.toml" ]; then
      example="$(dirname "$0")/settings.example.toml"
    fi

    if [ -n "$example" ]; then
      cp "$example" "$settings_file"
      info "Created $settings_file"
    else
      info "Could not find settings template, skipping."
    fi
  else
    info "$settings_file already exists, skipping."
  fi
}

# ---------------------------------------------------------------------------
# PATH check
# ---------------------------------------------------------------------------

check_path() {
  case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
      echo ""
      info "WARNING: $BIN_DIR is not on your PATH."
      info "Add it to your shell profile:"
      case "${SHELL:-}" in
        */zsh)  info "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc" ;;
        */bash) info "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc" ;;
        *)
          info "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc  # zsh"
          info "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc # bash"
          ;;
      esac
      ;;
  esac
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

echo "=== agent-dashboard installer ==="
echo ""

validate_adapter
check_prerequisites

if [ "$BUILD_FROM_SOURCE" = true ]; then
  install_binary_build
  install_adapter_build
else
  detect_platform
  resolve_version

  WORK_DIR=$(mktemp -d)
  trap cleanup EXIT

  install_binary_download
  install_adapter_download
fi

bootstrap_settings
check_path

echo ""
echo "Done. Restart Claude Code sessions to activate hooks."
echo "Run 'agent-dashboard' in a tmux pane to start the dashboard."
