#!/usr/bin/env bash
# vendor-deps.sh — refresh pure-Python rich + questionary into lib/_vendor.
# Idempotent maintenance target. Pin range mirrors util.DEPS_SPEC.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
VENDOR_DIR="$AI_SPECS_HOME/lib/_vendor"
mkdir -p "$VENDOR_DIR"
python3 -m pip install --upgrade --target "$VENDOR_DIR" \
  "rich>=13.0.0,<15" \
  "questionary>=2.0.0,<2.1"
echo "Vendored into $VENDOR_DIR"
