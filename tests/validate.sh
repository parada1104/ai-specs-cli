#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# lib/ is deleted by PR4b; the gate degrades to a note instead of failing then.
if [ -d lib ]; then
    python3 -m py_compile lib/_internal/*.py tests/*.py
    bash -n lib/*.sh bin/ai-specs tests/*.sh
else
    echo "validate.sh: lib/ absent — skipping lib/_internal/*.py and lib/*.sh syntax checks"
    python3 -m py_compile tests/*.py
    bash -n bin/ai-specs tests/*.sh
fi
if command -v go >/dev/null 2>&1; then
    echo "validate.sh: go found — checking Go formatting (gofmt -l)"
    gofmt -l catalog/recipes/worktree-flow/gate
else
    echo "validate.sh: WARNING: 'go' not found on PATH — skipping gofmt check on catalog/recipes/worktree-flow/gate" >&2
fi
./tests/run.sh
