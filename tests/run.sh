#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# Offline vendor for kepano/obsidian-skills when vault-canonical-store is enabled in tests.
export AI_SPECS_VENDOR_FIXTURE_ROOT="${AI_SPECS_VENDOR_FIXTURE_ROOT:-$ROOT/tests/fixtures/kepano-obsidian-skills}"
bash "$ROOT/tests/test_vault_fs_mcp.sh"
if command -v go >/dev/null 2>&1; then
    echo "run.sh: go found — running Go gate tests (go test ./catalog/recipes/worktree-flow/gate/...)"
    go -C catalog/recipes/worktree-flow/gate test ./...
else
    echo "run.sh: WARNING: 'go' not found on PATH — skipping Go gate tests (catalog/recipes/worktree-flow/gate/...)" >&2
fi
python3 -m unittest discover -s tests -p 'test_*.py'
