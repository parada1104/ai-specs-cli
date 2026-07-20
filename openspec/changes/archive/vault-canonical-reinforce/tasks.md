# Tasks: vault-canonical-reinforce

Depth: **standard**

Source specs:
- `openspec/changes/vault-canonical-reinforce/specs/vault-canonical-store/spec.md`
- `openspec/changes/vault-canonical-reinforce/specs/mcp-env-rendering/spec.md`
- `openspec/changes/vault-canonical-reinforce/specs/recipe-evals/spec.md`

Execution mode: **strict TDD**. Phase 1 MUST show RED before Phase 2 recipe/render changes land.

Legend: `[P]` = can run in parallel with sibling `[P]` tasks in the same phase.
Unmarked tasks are sequential within the phase.

Tracker: Trello https://trello.com/c/6CxB4kAs (card 45)

## Defaults (locked unless user overrides at auth)

1. Ship **all five** kepano skills as recipe `source = "dep"` (not project `[[deps]]`, not vendored into catalog).
2. Keep filesystem MCP pin `@modelcontextprotocol/server-filesystem@2025.7.1`.
3. Path story stays env-owned (`CANONICAL_VAULT_PATH`); harden rendering + docs for spaces / iCloud; do **not** invent a new literal-path config key in v1.
4. Dry unit + eval smoke first; live vault eval module + `run-live-vault.sh` as part of this change.
5. Bump recipe version `1.1.0` → `1.2.0`.

---

## Phase 1 — Tests scaffolding (RED)

- [x] **T1.1** — Extend `tests/test_vault_canonical_store_recipe.py`: assert recipe declares `vault-context` + five kepano dep skill ids with `url`/`path`; assert MCP pin remains `2025.7.1` and arg `${CANONICAL_VAULT_PATH}`.
  **Done when:** tests collect and fail for missing dep declarations.

- [x] **T1.2** `[P]` — RED: materialize with network fixture or mocked vendor proves kepano dep skill ids are expected after enable (follow `test_external_dirs.py` dep pattern).
  **Req:** kepano skills present after enable/sync (dry).

- [x] **T1.3** `[P]` — RED in `tests/test_sync_pipeline.py` (or vault-specific): sync with agents `[claude, cursor, opencode, pi, omp]` and `CANONICAL_VAULT_PATH` containing spaces; assert each MCP config keeps a **single** path arg element (`$CANONICAL_VAULT_PATH` / `${CANONICAL_VAULT_PATH}` per agent rules), never split on spaces; OpenCode command args use `{env:CANONICAL_VAULT_PATH}` (not bare `$VAR` — OpenCode only expands `{env:VAR}`).
  **Req:** spaced-path MCP args.

- [x] **T1.4** `[P]` — RED: `tests/evals/eval_harness_smoke.py` loads
  `tests/evals/scenarios/vault-canonical-store/*` fixtures and materializes vault recipe.
  **Req:** dry eval fixtures.

- [x] **T1.5** — Confirm Phase 1 RED evidence (failing for missing deps / spaced-path asserts / missing scenarios). Record command + summary.

---

## Phase 2 — Recipe + docs (GREEN)

- [x] **T2.1** — Update `catalog/recipes/vault-canonical-store/recipe.toml`: add five `source = "dep"` skills from `https://github.com/kepano/obsidian-skills.git` with paths `skills/<id>`; bump version to `1.2.0`; keep bundled `vault-context` + MCP pin.
  **Req:** kepano skills ship with recipe.

- [x] **T2.2** — Update `vault-context/SKILL.md`: cross-link Obsidian skills (when to load markdown/bases/canvas/cli/defuddle); reinforce “do not hardcode path; use runtime brief / MCP”; note Obsidian CLI requires Obsidian open.
  **Req:** vault-context guidance.

- [x] **T2.3** — Fix/refresh README + `docs/recipes-catalog.md`: recipe **does** declare MCP preset; document `.envrc` iCloud/spaced path pattern; list kepano skills.
  **Req:** docs match behavior.

- [x] **T2.4** — GREEN: Phase 1 unit tests pass. Record evidence.

---

## Phase 3 — Path hardening (if RED exposed a real bug)

- [x] **T3.1** — Skipped (no renderer bug; spaced-path test green). Was: Only if T1.3 fails for a real renderer bug: fix `mcp-render.py` / merge path so args stay atomic JSON array elements and OpenCode bare-dollar form is preserved for vault preset. Prefer minimal fix.
  **Req:** mcp-env-rendering delta.

- [x] **T3.2** — Skipped (not needed). Was: Optional stretch (only if cheap): envrc-scaffold also scans MCP `args` for `${VAR}` so args-only refs still appear in `.envrc.example`. Skip if it expands scope without failing tests.
  **Done when:** decided explicitly in apply-progress.

---

## Phase 4 — Evals

- [x] **T4.1** — Add scenario dirs under `tests/evals/scenarios/vault-canonical-store/`:
  - `ac_kepano_skills_present/`
  - `ac_mcp_path_with_spaces/` (dry-oriented; may be asserted mostly in unit tests)
  - `ac_vault_context_guidance/`
  Each with `scenario.toml` + `prompt.txt`.
  **Req:** recipe-evals vault client.

- [x] **T4.2** — Add `tests/evals/eval_vault_canonical_live.py` + `tests/evals/run-live-vault.sh` mirroring VCS live runner pattern; dry skips without `EVALS_LIVE=1`.
  **Req:** live opt-in vault evals.

- [x] **T4.3** — Wire dry smoke assertions; document in `tests/evals/README.md`.
  **Req:** discoverable vault eval client.

- [x] **T4.4** — Run `./tests/validate.sh` (or project full suite) green before commit/PR.

- [x] **T4.5** — Live MCP connect/scope: `ac_mcp_live_scope` with
  `EVALS_RUNTIMES=claude,cursor-agent` — agent reads scoped `MARKER.md` via
  vault-canonical MCP; sibling secret must not leak; prefer tool-call evidence.
  **Done when:** live run green (or documented soft-fail with cause).
  **Evidence:** Claude OK (~36s) and cursor-agent OK (~33s) for
  `EVALS_SCENARIOS=ac_mcp_live_scope`. Host notes in `tests/evals/README.md`
  (pin 2025.7.1 tools-fetch fail on Claude; live uses 2025.11.25 + roots/add-dir).

---

## Phase 5 — Close prep

- [x] **T5.1** — Update CHANGELOG under Unreleased.
- [ ] **T5.2** — Commit planning + implementation on `feat/vault-canonical-reinforce`; open PR to `development` only after tier artifacts committed.
- [ ] **T5.3** — Archive-tail on review branch before merge (not after).

---

## Out of scope

- Pinning kepano git SHAs/tags in vendor (shallow default branch only today).
- Migrating venturi_coffee off project `[[deps]]` (follow-up after recipe lands).
- Changing filesystem MCP to versions ≥2025.7.29 (roots override) without a dedicated pi/omp fix.
- New recipe config key for literal absolute vault path (env remains source of truth).
