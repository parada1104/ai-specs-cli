# ODD Feature: go-hook-gate

**Card**: GO-10 (Trello `6ab6a6aee7b690c45763669d`, https://trello.com/c/g3DWSd86)
**Worktree**: `.worktrees/go-hook-gate` — branch `feat/go-hook-gate` from development `6d4890e`
**Pipeline role**: PARALLEL PREPARATION lane (slice 7). This session prepares ONLY — no gate binary changes, no SHA256SUMS, no native review, no PR. Implementation starts after slice 6 (GO-09) merges, with rebase. Delivery is SERIALIZED through the main session.

## Goal

Strangler slice 7 prep: contract map + RED test suite for migrating the hook/gate actuator (recipe-materialize.py 959-1245: 8 hook placeholders, state machine, refresh rollback) to Go. Last slice of the rank-3 write-actuator chain.

## Tasks

- [ ] Scout: exact contract of the hook/gate actuator path (placeholders, state machine, refresh rollback, exact strings, callers, bridge pattern fit). **Status: done — see ## Scout below; commit <sha>**
- [ ] RED: failing Go parity test suite for the future Go core (hook/gate actuator), following the recipeconfigwrite_test.go pattern. **Status: pending**
- [ ] ODD doc evidence updated; hand-off note for the implementation session. **Status: pending**

## Rules for this lane

- Allowed surfaces: odd/tasks/go-hook-gate.md, NEW Go test file (e.g. hookgateactuator_test.go), tests/ Python RED tests if the bridge contract needs them.
- Do NOT touch main.go, SHA256SUMS, dist/, or any production .go/.py file.
- The classify_managed_override port (util.py:651/808) is written ONCE at implementation, shared with slice 6 (GO-09) — document the sharing point in the contract, do not implement it.
- Local commits on feat/go-hook-gate after each task are fine; no push.

## Out of scope

- Implementation, native review, PR, merge. MUST NOT be combined with template actuator (~1000-line review budget).

## Scout: hook/gate actuator contract (read-only, slice 7)

All line refs against `lib/_internal/recipe-materialize.py` @ `6d4890e` (3157 lines).

### Callers (exactly one)

- `materialize_hook_script(recipe_dir, hook, project_root, recipe_id, merged_cfg, cli_home, refresh)` — called once per runtime hook at `recipe-materialize.py:2952`, inside the per-recipe sync loop (`for rhook in recipe.runtime_hooks`), after docs and `execute_hooks`. `refresh` is `refresh_gates` (the `--refresh-gates` flag, NEVER set by ordinary sync). The returned `script_path` feeds the `resolved_hooks` entries handed to hooks-render.py.
- `hook_script_rel_path` (:959) has no other callers. Test callers: `tests/test_tracker_card_gate_hook.py:560,569` (refresh + idempotence), `tests/test_trello_mcp_workflow_recipe.py:273,290,300`.
- `hook` is the runtime-hook schema (`recipe_schema.py:104`): `script` (recipe-relative, validated non-absolute/inside-recipe/no-`..`), plus `id`/`event`/`description` (actuator only reads `script`).

### The 8 hook placeholders (constants :962-980)

Rendered in this exact order, each only when the token is present in the source bytes:

1. `__WORKTREE_GATE_MODE__` → default `"always"`; `str(merged_cfg.get("gate_mode", default))` — NOTE: unlike the validated tokens below this has NO `or default` fallback, so a present-but-empty value stays empty (pathological; config validation prevents it in practice).
2. `__TRACKER_CARD_GATE_MODE__` → default `"warn"` (same `get(key, default)` shape; both live in `GATE_MODE_PLACEHOLDERS`).
3. `__WORKTREE_GATE_SCOPE__` → `merged_cfg.get("gate_scope") or "auto"`; validated against `("auto", "superrepo", "subrepo")`, else `RuntimeError("invalid gate_scope '{scope}'; allowed: auto | superrepo | subrepo")`.
4. `__WORKTREE_REPO_TOPOLOGY__` → `or "auto"`; validated against `("auto", "standalone", "monorepo-apps", "monorepo-submodules")`, else `RuntimeError("invalid repo_topology '{t}'; allowed: auto | standalone | monorepo-apps | monorepo-submodules")`.
5. `__WORKTREE_GATE_IMPL__` → `or "auto"`; validated against `("auto", "go")`, else `RuntimeError("invalid gate_impl '{impl}'; bash has been removed; allowed: auto | go")`.
6. `__WORKTREE_GATE_VERSION__` → `"dev"` default; otherwise `_load_cli_version().read_installed_version(cli_home)`, ANY exception → `"dev"`. Version resolution stays in the Python adaptation layer (Python owns the version module); the bridge envelope carries the resolved `gate_version` string.
7. `__TRACKER_CLI_HOME__` → `str(Path(cli_home).resolve())` when cli_home is not None, else `""` (empty → host skips evidence acquisition, fail-open).
8. `__TRACKER_LIB_INTERNAL__` → `str((Path(cli_home) / "lib" / "_internal").resolve())` or `""` (the ledger_bridge.py evidence dir).

(`__WORKTREE_WORKTREES_DIR__` / `__WORKTREE_INTEGRATION_BRANCH__` are TEMPLATE-only — rendered by `render_template_bytes` :989, not by the hook path.)

### rel path + lock

- `rel = hook_script_rel_path(recipe_id, hook)` = `ai-specs/recipes/{recipe_id}/hooks/{Path(hook.script).name}` (:959-960) — BASENAME only, subdirs flattened.
- `dest = project_root / rel`; parent mkdir -p BEFORE rendering errors can happen? No — mkdir happens before rendering (:1108-1112: mkdir then read+render; a render error leaves an empty hook dir behind, acceptable).
- Source check FIRST: `recipe_dir / hook.script` must be a regular file, else `RuntimeError(f"hook script not found: {src}")` (absolute joined path).
- Lock: `project_root/ai-specs/.ai-specs.lock` via `lock.load_lock`; `entry = (lock["managed"] or {}).get(rel)`.
- `record(written)`: `lock.set_gate_baseline(lock, rel, util.sha256_bytes(written), recipe=recipe_id, source=hook.script)` then `lock.write_lock` (atomic). `set_gate_baseline` = `set_managed_override(..., kind="gate", policy="auto")` (lock.py:159-181). sha256 is CRLF-normalized (`util.normalized_bytes`). **The lock write stays in PYTHON in the bridge design** — Go returns the record payload in the envelope, Python calls set_gate_baseline + write_lock (same as the slice-6 template contract).

### State machine (non-refresh, :1176-1222)

`state = util.classify_managed_override(dest, entry, would_write=content)` — **the SHARED classification port (util.py:651/808, `worktree-gate --plan-classify`)**. It is ALREADY ported in Go (`classifyManagedOverride` in classify.go, written once for slice 6). Slice 7's implementation MUST REUSE it and must NOT re-port or fork the decision; only the argument adaptation differs: the hook path passes the exact rendered content as `would_write`.

- `missing` → `dest.write_text(content)`, `chmod 0o755`, `record(content)`, print `    ✓ hook script {rel}`, return rel.
- `managed_current` → `record(content)` only (no write), print `    · hook skipped (current) {rel}`.
- `managed_stale` (baseline matches disk bytes but rendered content drifted) → write + chmod 0755 + record, print `    ✓ hook refreshed (baseline matched) {rel}` — ordinary sync MAY force-update an unmodified gate.
- `user_modified` → NO write, NO record, warn exact string (below), print `    · hook skipped (user-modified) {rel}`.
- `untracked` (dest exists, no baseline — unknown provenance) → NO write, NO record, NEVER seed (unlike the template actuator's not_exists seeding — hooks have no seed path), warn exact string (below), print `    · hook skipped (no provenance) {rel}`.

Gate files are always written with mode 0o755 (source mode is NOT copied — unlike templates).

### Refresh path (:1043-1081 `_refresh_gate`, all-or-nothing)

Entered only with `refresh=True`, unconditionally (classification is skipped — a refresh replaces even a user-modified gate after backup):

1. `prior = dest.read_bytes()` if dest exists else None.
2. If prior: backup via `_write_gate_backup` (:1017-1032) → `project_cache.gate_backup_path(project_root, rel, sha256_bytes(prior), cli_home)` = `<cache_root>/backups/<sha256(str(rel)) hex>/<content_sha>.sh` (project-cache.py:119-142; content-hash filename makes it immutable — never overwritten, only written if absent). **Bridge design: Python PRE-COMPUTES the backup path (it owns project-cache) and passes it in the envelope; Go writes it if absent and reports it back.**
3. `dest.write_text(content)`, chmod 0o755, `set_gate_baseline(sha256_bytes(content))`, `write_lock`.
4. On ANY exception: restore prior bytes + chmod 0755 (best effort), unlink the created backup if it now exists, re-raise. The lock is never partially updated. In the bridge: Go performs 2-3 and, on any Go-side failure before success, exits 2 with prior bytes unchanged and the backup removed; the Python-side lock write happening after means Python must treat an envelope error as "gate unchanged, no record".
5. Print `    ✓ hook refreshed {rel}`, return rel.

### Exact strings

Warnings (warn = `print(f"  ! {msg}", file=sys.stderr)`, :165-166; the envelope carries the inner string, Python adds the prefix):
- user-modified: `hook {rel} is user-modified; preserving existing bytes. Refresh with:\n  rm {rel} && ai-specs sync  (or: ai-specs sync --refresh-gates)`
- no provenance: `hook {rel} has no recorded provenance; preserving existing bytes. A baseline is recorded only when the CLI renders the gate. Refresh with:\n  rm {rel} && ai-specs sync  (or: ai-specs sync --refresh-gates)`

Messages (without the 4-space print indentation, matching the slice-6 envelope convention):
- `✓ hook script {rel}` / `· hook skipped (current) {rel}` / `✓ hook refreshed (baseline matched) {rel}` / `· hook skipped (user-modified) {rel}` / `· hook skipped (no provenance) {rel}` / `✓ hook refreshed {rel}`

Errors (exit 2 in the bridge): `hook script not found: {src}`, and the three validation messages above.

### Bridge pattern fit

Mirrors the merged slices (GO_RECIPE_CONFIG_BRIDGE_FALLBACK, GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK): new token `GO_HOOK_GATE_BRIDGE_FALLBACK`, one greppable warning per degraded run, TEMPORARY Python path deleted once proven. Contract: one JSON envelope on stdin, one on stdout, exit 0/2 — identical shape to slice 6's `--materialize-template`. Reuses the ported `classifyManagedOverride` core (shared port with slice 6, written ONCE — do not touch classify.go's decision). Go owns: rendering (8 placeholders + validation), actuation (write/chmod), refresh backup/rollback. Python keeps: lock I/O, record application, warning/message printing, backup-path precomputation, gate-version resolution, bridge warning.

## Hand-off note (for the implementation session)

- RED suite: `catalog/recipes/worktree-flow/gate/hookgateactuator_test.go` — does not compile until `hookgateactuator.go` defines `runMaterializeHook`, `hookScriptRelPath`, `renderHookGateContent`. Those tests are the acceptance bar; implement byte-for-byte/string-for-string.
- Reuse, never re-port: `classifyManagedOverride` + `sha256Bytes` (classify.go). The classification port belongs to slice 6/7 jointly (util.py:651/808).
- Implement AFTER slice 6 (GO-09) merges, on a rebase of this branch; delivery is serialized through the main session. Do NOT combine with the template actuator in one review (~1000-line budget).
