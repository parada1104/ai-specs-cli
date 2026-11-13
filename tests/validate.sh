#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# lib/ was deleted in GO-20 U2e T5 (PR4b); the Python and shell surface that
# remains is the test suite and the launcher itself.
python3 -m py_compile tests/*.py
bash -n bin/ai-specs tests/*.sh
if command -v go >/dev/null 2>&1; then
    echo "validate.sh: go found — checking Go formatting (gofmt -l)"
    gofmt -l catalog/recipes/worktree-flow/gate
else
    echo "validate.sh: WARNING: 'go' not found on PATH — skipping gofmt check on catalog/recipes/worktree-flow/gate" >&2
fi
./tests/run.sh
