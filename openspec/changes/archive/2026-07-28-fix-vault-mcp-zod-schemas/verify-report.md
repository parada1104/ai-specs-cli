# Verify Report — fix-vault-mcp-zod-schemas

**Tier**: tasks-only (single-template fix + docs; no new capability, no delta spec)
**Branch**: `fix/vault-mcp-zod-schemas`
**Verified at**: `e09eada`, rebased onto `development` @ `7e9ac67`

## Outcome

**PASS**

## Task coverage

All tasks in `tasks.md` are checked and each maps to a shipped artifact:

| Phase | Tasks | Evidence |
|-------|-------|----------|
| 1 — Tests | T1.x | `tests/test_vault_canonical_store_recipe.py`, `tests/test_vault_fs_mcp.sh` assert the `zod@3` pin; confirmed RED before the template change |
| 2 — Fix | T2.x | `catalog/recipes/vault-canonical-store/templates/vault-fs-mcp.sh` names `zod@3` as a second `-p` package |
| 3 — Docs | T3.1–T3.3 | recipe README, `tests/evals/README.md`, `CHANGELOG.md` `[Unreleased]` → `Fixed` |
| 4 — Validation | T4.1–T4.2 | see below |

## Evidence

- `./tests/validate.sh` — exit 0, **1094 tests OK** (re-run post-rebase; the pre-rebase
  figure in the PR body was 1052 against the older base).
- `python3 tests/smoke_vault_mcp_fs.py` — exit 0, `SMOKE_OK`. Allowed directory resolves
  to `CANONICAL_VAULT_PATH` alone; out-of-scope paths stay denied.
- Strict TDD respected: assertions landed first and were confirmed RED
  (`missing zod@3 pin`) before the template edit.

## Rebase note

`development` moved under this branch when #158 merged. Rebased onto `7e9ac67`; the only
conflict was a duplicated `### Fixed` heading in `CHANGELOG.md` (both changes appended
under the same section). Resolved by merging both entries under one heading — no content
dropped from either side. `catalog/recipes/vault-canonical-store/README.md` auto-merged;
the `zod@3` rationale section and #158's env-layout edits are disjoint and both present.
Full suite re-run after the rebase, not carried over from the pre-rebase run.

## Not run

**Judgment Day was not run for this change.** Verification here is task-coverage plus the
test/smoke evidence above. Recorded explicitly so the gap is visible rather than assumed.

## Residual notes

- No recipe `version` bump, following the #151 (`f4c5ecf`) precedent for template-target
  changes.
- `tests/evals/lib/vault_mcp_live.py` keeps its `2025.11.25` default: it registers the
  package directly, bypassing the wrapper, so it does not inherit the `zod@3` pin.
  `2025.11.25` and `2026.1.14` were confirmed to emit valid schemas unaided; only the
  stale rationale in the docstrings was corrected.
- The package pin stays at `2025.7.1` deliberately: `2025.7.29+` replaces argv directories
  with MCP client roots with no opt-out, which either denies a vault outside the workspace
  or widens scope to cwd + vault. Neither expresses this recipe's contract.
