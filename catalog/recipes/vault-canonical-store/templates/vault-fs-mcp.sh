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

# Why zod is pinned separately: 2025.7.1 does not depend on zod directly — it
# inherits it from @modelcontextprotocol/sdk, which now resolves to zod 4.x. Its
# own zod-to-json-schema@^3 only understands zod 3 internals, so against zod 4 it
# emits an empty '{"$schema": ...}' for every tool: no "type", no "properties".
# Hosts that validate tool schemas then reject the whole tools/list. Naming zod@3
# as a second -p package hoists it above the SDK copy, so the server converts its
# schemas with the zod its converter expects.
#
# Do NOT "fix" this by bumping the package. 2025.7.29+ replaces argv directories
# with MCP client roots whenever the client advertises that capability, with no
# opt-out. A host always advertises its cwd as a root, so the vault would either
# fall out of scope entirely or be widened to cwd + vault — and this recipe's
# contract is that CANONICAL_VAULT_PATH alone decides the scope.
exec npx -y \
  -p "@modelcontextprotocol/server-filesystem@2025.7.1" \
  -p "zod@3" \
  mcp-server-filesystem "$ROOT"
