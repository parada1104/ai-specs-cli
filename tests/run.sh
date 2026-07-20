#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# Offline vendor for kepano/obsidian-skills when vault-canonical-store is enabled in tests.
export AI_SPECS_VENDOR_FIXTURE_ROOT="${AI_SPECS_VENDOR_FIXTURE_ROOT:-$ROOT/tests/fixtures/kepano-obsidian-skills}"
bash "$ROOT/tests/test_vault_fs_mcp.sh"
python3 -m unittest discover -s tests -p 'test_*.py'
