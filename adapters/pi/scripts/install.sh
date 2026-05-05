#!/usr/bin/env sh
# Symlink the pi extension into pi's auto-discovery directory.
# Re-runnable: removes any existing symlink/file at the target before linking.
set -eu

ADAPTER_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ADAPTER_DIR/extensions/agent-dashboard.js"
DEST_DIR="${HOME}/.pi/agent/extensions"
DEST="$DEST_DIR/agent-dashboard.js"

if [ ! -f "$SRC" ]; then
  echo "error: source extension missing: $SRC" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"

if [ -e "$DEST" ] || [ -L "$DEST" ]; then
  rm -f "$DEST"
fi

ln -s "$SRC" "$DEST"
echo "installed: $DEST -> $SRC"
