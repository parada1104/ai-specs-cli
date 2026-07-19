#!/usr/bin/env bash
# Unit tests for vault-fs-mcp.sh path resolution (no MCP host required).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT/catalog/recipes/vault-canonical-store/templates/vault-fs-mcp.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok - $*"; }

# Stub npx so we never hit the network; record argv.
STUB_BIN="$TMP/bin"
mkdir -p "$STUB_BIN"
cat >"$STUB_BIN/npx" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"${VAULT_FS_MCP_ARGV_LOG:?}"
exit 0
EOF
chmod +x "$STUB_BIN/npx"
export PATH="$STUB_BIN:$PATH"

# --- missing env ---
if CANONICAL_VAULT_PATH= "$SCRIPT" 2>/dev/null; then
  fail "expected failure when CANONICAL_VAULT_PATH empty"
fi
pass "rejects empty CANONICAL_VAULT_PATH"

# --- unresolved nested $ ---
if CANONICAL_VAULT_PATH='$OBSIDIAN_VAULT_PATH/scope' "$SCRIPT" 2>/dev/null; then
  fail "expected failure when path still contains \$"
fi
pass "rejects unresolved nested \$ in path"

# --- missing directory ---
if CANONICAL_VAULT_PATH="$TMP/does-not-exist" "$SCRIPT" 2>/dev/null; then
  fail "expected failure when directory missing"
fi
pass "rejects missing directory"

# --- absolute path alone (no OBSIDIAN_VAULT_PATH / nested composition) ---
SCOPE="$TMP/Mobile Documents/vault scope"
mkdir -p "$SCOPE"
export VAULT_FS_MCP_ARGV_LOG="$TMP/argv.txt"
# Contract: MCP only needs CANONICAL_VAULT_PATH set to an absolute directory.
env -u OBSIDIAN_VAULT_PATH \
  PATH="$PATH" \
  VAULT_FS_MCP_ARGV_LOG="$VAULT_FS_MCP_ARGV_LOG" \
  CANONICAL_VAULT_PATH="$SCOPE" \
  "$SCRIPT"
grep -qxF "$SCOPE" "$VAULT_FS_MCP_ARGV_LOG" || fail "argv log missing absolute path; got: $(cat "$VAULT_FS_MCP_ARGV_LOG")"
grep -q 'server-filesystem@2025.7.1' "$VAULT_FS_MCP_ARGV_LOG" || fail "missing pinned package"
# Must not depend on a second vault env var being present.
pass "standalone absolute CANONICAL_VAULT_PATH (OBSIDIAN unset) works"

# --- tilde expansion ---
HOME_SCOPE="$HOME/.cache/ai-specs-vault-fs-mcp-test-$$"
mkdir -p "$HOME_SCOPE"
export VAULT_FS_MCP_ARGV_LOG="$TMP/argv-tilde.txt"
CANONICAL_VAULT_PATH="~/.cache/ai-specs-vault-fs-mcp-test-$$" "$SCRIPT"
grep -qxF "$HOME_SCOPE" "$VAULT_FS_MCP_ARGV_LOG" || fail "tilde not expanded; got: $(cat "$VAULT_FS_MCP_ARGV_LOG")"
rm -rf "$HOME_SCOPE"
pass "expands leading ~/"

echo "All vault-fs-mcp.sh checks passed."
