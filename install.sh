#!/bin/sh
set -eu

# ---------------------------------------------------------------------------
# agent-dashboard installer
#
# Downloads the pre-built binary, verifies its checksum, and installs it.
# Plugin registration is handled separately via Claude Code's /plugin command.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
#   ./install.sh              # download pre-built binary
#   ./install.sh --build      # compile from source (requires Go, run from repo checkout)
# ---------------------------------------------------------------------------

REPO="bjornjee/agent-dashboard"
BIN_DIR="$HOME/.local/bin"
STATE_DIR="${AGENT_DASHBOARD_DIR:-$HOME/.agent-dashboard}"
BUILD_FROM_SOURCE=false
WORK_DIR=""

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --build) BUILD_FROM_SOURCE=true; shift ;;
    *)       shift ;;
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
# Prerequisite checks
# ---------------------------------------------------------------------------

check_prerequisites() {
  missing=""

  if ! check_cmd tmux; then
    missing="$missing  - tmux: https://github.com/tmux/tmux/wiki/Installing\n"
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
# Download, verify, and install binary
# ---------------------------------------------------------------------------

install_binary_download() {
  step "1/2" "Downloading agent-dashboard v$VERSION ($OS/$ARCH)..."

  asset="agent-dashboard_${VERSION}_${OS}_${ARCH}.tar.gz"
  url="https://github.com/$REPO/releases/download/v${VERSION}/$asset"
  checksums_url="https://github.com/$REPO/releases/download/v${VERSION}/checksums.txt"

  if ! curl -fsSL "$checksums_url" -o "$WORK_DIR/checksums.txt"; then
    err "Failed to download checksums from $checksums_url"
    exit 1
  fi

  if ! curl -fsSL "$url" -o "$WORK_DIR/$asset"; then
    err "Failed to download $url"
    if check_cmd go; then
      info "Falling back to building from source..."
      install_binary_build
      return
    fi
    exit 1
  fi

  # Verify checksum
  expected=$(grep "$asset" "$WORK_DIR/checksums.txt" | awk '{print $1}')
  if [ -z "$expected" ]; then
    err "Asset $asset not found in checksums.txt"
    exit 1
  fi

  if check_cmd sha256sum; then
    actual=$(sha256sum "$WORK_DIR/$asset" | awk '{print $1}')
  elif check_cmd shasum; then
    actual=$(shasum -a 256 "$WORK_DIR/$asset" | awk '{print $1}')
  else
    info "WARNING: neither sha256sum nor shasum found, skipping checksum verification."
    actual="$expected"
  fi

  if [ "$actual" != "$expected" ]; then
    err "Checksum verification failed!"
    err "  Expected: $expected"
    err "  Actual:   $actual"
    err "The downloaded file may have been tampered with."
    exit 1
  fi
  info "Checksum verified."

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
  step "1/2" "Building agent-dashboard from source..."
  cd "$REPO_DIR"
  make build
  mkdir -p "$BIN_DIR"
  cp bin/agent-dashboard "$BIN_DIR/agent-dashboard"
  info "Installed to $BIN_DIR/agent-dashboard"
  cd - >/dev/null
}

# ---------------------------------------------------------------------------
# Bootstrap settings
# ---------------------------------------------------------------------------

bootstrap_settings() {
  step "2/2" "Bootstrapping settings..."
  local settings_file="$STATE_DIR/settings.toml"
  if [ ! -f "$settings_file" ]; then
    mkdir -p "$STATE_DIR"

    # Try repo root first (--build mode), then write a minimal default
    local example=""
    if [ -f "$(dirname "$0")/settings.example.toml" ]; then
      example="$(dirname "$0")/settings.example.toml"
    fi

    if [ -n "$example" ]; then
      cp "$example" "$settings_file"
    else
      # Write minimal default settings inline (no external dependency)
      cat > "$settings_file" <<'TOML'
# Agent Dashboard settings
# See https://github.com/bjornjee/agent-dashboard#user-settings for all options.

[banner]
show_mascot   = true
show_quote    = true

[notifications]
enabled       = false
sound         = false
silent_events = false

[debug]
key_log       = false

[experimental]
ascii_pet     = false
TOML
    fi
    info "Created $settings_file"
  else
    info "$settings_file already exists, skipping."
  fi
}

# ---------------------------------------------------------------------------
# PATH check
# ---------------------------------------------------------------------------

check_path() {
  case ":$PATH:" in
    *":$BIN_DIR:"*) return ;;
  esac

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
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

echo "=== agent-dashboard installer ==="
echo ""

check_prerequisites

if [ "$BUILD_FROM_SOURCE" = true ]; then
  install_binary_build
else
  detect_platform
  resolve_version

  WORK_DIR=$(mktemp -d)
  trap cleanup EXIT

  install_binary_download
fi

bootstrap_settings
check_path

echo ""
echo "=== Next steps ==="
echo ""
echo "  1. Add the marketplace (run in any Claude Code session):"
echo ""
echo "     /marketplace add bjornjee/agent-dashboard"
echo ""
echo "  2. Install the plugin:"
echo ""
echo "     /plugin install agent-dashboard@agent-dashboard"
echo ""
echo "  3. Restart Claude Code sessions for hooks and skills to take effect."
echo ""
echo "  4. Run the dashboard in a tmux pane:"
echo ""
echo "     agent-dashboard"
echo ""
