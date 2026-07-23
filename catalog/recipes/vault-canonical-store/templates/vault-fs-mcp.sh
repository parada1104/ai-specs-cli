#!/usr/bin/env bash
# Launch @modelcontextprotocol/server-filesystem with CANONICAL_VAULT_PATH.
#
# Why a wrapper: many agent hosts expand ${VAR} in mcp.env / process env, but
# leave a bare "${CANONICAL_VAULT_PATH}" argv literal (or empty) when the var is
# missing from the host environment at parse time. Putting the path only in
# args is fragile. This script reads the env at exec time and passes one argv.
set -euo pipefail

ROOT="${CANONICAL_VAULT_PATH:-}"
if [[ -z "$ROOT" ]]; then
  echo "vault-fs-mcp: CANONICAL_VAULT_PATH is unset or empty." >&2
  echo "Set it in .envrc to an absolute vault scope path, then restart the agent." >&2
  exit 1
fi

# Expand a leading ~/ (users sometimes store a home-relative value).
if [[ "$ROOT" == ~* ]]; then
  ROOT="${ROOT/#\~/$HOME}"
fi

# Reject unresolved nested shell refs left over from naive .env assignment.
if [[ "$ROOT" == *'$'* ]]; then
  echo "vault-fs-mcp: CANONICAL_VAULT_PATH still contains '\$' ($ROOT)." >&2
  echo "Use an absolute path, or set it in .envrc so the shell expands nested vars." >&2
  exit 1
fi

if [[ ! -d "$ROOT" ]]; then
  echo "vault-fs-mcp: not a directory: $ROOT" >&2
  exit 1
fi

exec npx -y @modelcontextprotocol/server-filesystem@2025.7.1 "$ROOT"
