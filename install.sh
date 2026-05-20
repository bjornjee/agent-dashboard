#!/bin/sh
set -eu

# ---------------------------------------------------------------------------
# agent-dashboard installer
#
# Downloads the pre-built binary, verifies its checksum, and installs it.
# Also installs the Codex global hook bundle used by the dashboard.
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
CODEX_DIR="${CODEX_HOME:-$HOME/.codex}"
CODEX_HOOKS_DIR="$CODEX_DIR/hooks/agent-dashboard"
CODEX_HOOKS_FILE="$CODEX_DIR/hooks.json"
BUILD_FROM_SOURCE=false
WORK_DIR=""
CODEX_HOOKS_SOURCE=""

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

copy_dir_if_missing() {
  src="$1"
  dest="$2"
  if [ -e "$dest" ]; then
    info "$dest already exists, skipping."
    return 1
  fi

  if ! mkdir -p "$(dirname "$dest")"; then
    err "Failed to create $(dirname "$dest")"
    exit 1
  fi
  if ! cp -R "$src" "$dest"; then
    err "Failed to copy $src to $dest"
    exit 1
  fi
}

copy_file_if_missing() {
  src="$1"
  dest="$2"
  if [ -e "$dest" ]; then
    info "$dest already exists, skipping."
    return 1
  fi

  if ! mkdir -p "$(dirname "$dest")"; then
    err "Failed to create $(dirname "$dest")"
    exit 1
  fi
  if ! cp "$src" "$dest"; then
    err "Failed to copy $src to $dest"
    exit 1
  fi
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
# Download, verify, and install binary
# ---------------------------------------------------------------------------

install_binary_download() {
  step "1/3" "Downloading agent-dashboard v$VERSION ($OS/$ARCH)..."

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
  step "1/3" "Building agent-dashboard from source..."
  cd "$REPO_DIR"
  make build
  mkdir -p "$BIN_DIR"
  cp bin/agent-dashboard "$BIN_DIR/agent-dashboard"
  info "Installed to $BIN_DIR/agent-dashboard"
  cd - >/dev/null
}

# ---------------------------------------------------------------------------
# Install Codex hooks
# ---------------------------------------------------------------------------

resolve_codex_hooks_source() {
  repo_dir="$(cd "$(dirname "$0")" && pwd)"
  if [ -f "$repo_dir/adapters/codex/hooks/hooks.json" ]; then
    CODEX_HOOKS_SOURCE="$repo_dir/adapters/codex/hooks"
    return
  fi

  if [ -z "${VERSION:-}" ]; then
    err "Could not locate adapters/codex/hooks in this checkout."
    exit 1
  fi

  source_archive="$WORK_DIR/source.tar.gz"
  source_dir="$WORK_DIR/source"
  source_url="https://github.com/$REPO/archive/refs/tags/v${VERSION}.tar.gz"

  mkdir -p "$source_dir"
  if ! curl -fsSL "$source_url" -o "$source_archive"; then
    err "Failed to download source archive from $source_url"
    exit 1
  fi

  tar -xzf "$source_archive" -C "$source_dir"
  hooks_json=$(find "$source_dir" -path "*/adapters/codex/hooks/hooks.json" -print | head -n 1)
  if [ -z "$hooks_json" ]; then
    err "adapters/codex/hooks/hooks.json not found in source archive."
    exit 1
  fi

  CODEX_HOOKS_SOURCE="$(dirname "$hooks_json")"
}

install_codex_hooks() {
  step "3/3" "Installing Codex dashboard hooks..."
  resolve_codex_hooks_source

  if copy_dir_if_missing "$CODEX_HOOKS_SOURCE" "$CODEX_HOOKS_DIR"; then
    chmod +x "$CODEX_HOOKS_DIR/agent-state-fast.sh" "$CODEX_HOOKS_DIR/agent-state-reporter.sh"
  fi
  copy_file_if_missing "$CODEX_HOOKS_SOURCE/hooks.json" "$CODEX_HOOKS_FILE" || true

  info "Codex hook runtime: $CODEX_HOOKS_DIR"
  info "Codex hook config: $CODEX_HOOKS_FILE"
}

# ---------------------------------------------------------------------------
# Bootstrap settings
# ---------------------------------------------------------------------------

bootstrap_settings() {
  step "2/3" "Bootstrapping settings..."
  settings_file="$STATE_DIR/settings.toml"
  if [ ! -f "$settings_file" ]; then
    example=""
    if [ -f "$(dirname "$0")/settings.example.toml" ]; then
      example="$(dirname "$0")/settings.example.toml"
    fi

    if [ -n "$example" ]; then
      mkdir -p "$STATE_DIR"
      cp "$example" "$settings_file"
      info "Created $settings_file"
    else
      info "settings.example.toml not found, using built-in defaults."
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
    *":$BIN_DIR:"*) return ;;
  esac

  echo ""
  info "WARNING: $BIN_DIR is not on your PATH."
  info "Add $BIN_DIR to PATH in your shell profile."
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
install_codex_hooks
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
echo "  4. Restart Codex sessions and approve the agent-dashboard hooks prompt."
echo ""
echo "     Hooks: $CODEX_HOOKS_FILE"
echo "     Runtime: $CODEX_HOOKS_DIR"
echo ""
echo "  5. Run the dashboard in a tmux pane:"
echo ""
echo "     agent-dashboard"
echo ""
