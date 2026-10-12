#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="$HOME/.gemini/config/skills"
mkdir -p "$TARGET_DIR"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

ln -sf "$SCRIPT_DIR/plugins/spec/skills/spec" "$TARGET_DIR/spec"
ln -sf "$SCRIPT_DIR/plugins/seal/skills/seal" "$TARGET_DIR/seal"

echo "✓ Jaekit skills (spec, seal) successfully registered to $TARGET_DIR"
echo "✓ Verify in agy cli by running an instruction referencing spec or seal."
