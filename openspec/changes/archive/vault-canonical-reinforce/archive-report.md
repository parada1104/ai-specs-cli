# Archive: vault-canonical-reinforce

Pre-merge archive on `feat/vault-canonical-reinforce` before squash-merge into `development` (PR #137).

## Shipped

- Recipe `vault-canonical-store` 1.2.0: kepano Obsidian skills (dep), `vault-fs-mcp.sh` wrapper, env-owned `CANONICAL_VAULT_PATH`.
- OpenCode MCP args render as `{env:VAR}` (not bare `$VAR`).
- Dry + live evals (`ac_mcp_live_scope`); smoke against real hermes-vault ai-specs path.

## Verification

- `./tests/validate.sh` / unit suite green on branch.
- Live: Claude + cursor-agent `ac_mcp_live_scope`; real-path smoke + live read of decision note.
