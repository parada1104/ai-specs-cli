#!/usr/bin/env bash
set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# A missing VERSION degrades to 'unknown' (same contract as hub.py and
# cli_version.py) instead of a raw cat error with exit 1.
if [[ -f "$REPO_ROOT/VERSION" ]]; then
    cat "$REPO_ROOT/VERSION"
else
    echo "unknown"
fi
