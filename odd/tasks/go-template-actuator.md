# ODD Feature: go-template-actuator

**Card**: GO-09 (Trello `6ab6a1720c9604d9728e979d`, https://trello.com/c/KNjBWFHA)
**Worktree**: `.worktrees/go-template-actuator` — branch `feat/go-template-actuator` from development `6d4890e`
**Pipeline role**: PARALLEL PREPARATION lane (slice 6). This session prepares ONLY — no gate binary changes, no SHA256SUMS, no native review, no PR. Implementation starts after slice 5 (GO-08) merges, with rebase. Delivery is SERIALIZED through the main session.

## Goal

Strangler slice 6 prep: contract map + RED test suite for migrating the template actuator (recipe-materialize.py 829-948/989-1010: template rendering, git-path dests, not_exists seeding) to Go.

## Tasks

- [x] Scout: exact contract of the template actuator path (inputs, rendering, dest handling, seeding, refusals, exact strings, callers). **Status: done — see ## Scout below; commit 001ef58**
- [x] RED: failing Go parity test suite for the future Go core (template actuator), following the recipeconfigwrite_test.go pattern. **Status: done — commit 96477f3; `go vet` fails exactly on the three undefined core symbols (runMaterializeTemplate first at templateactuator_test.go:142); gofmt clean; 17 tests (pure rendering x3, dest resolution x4, end-to-end x10)**
- [x] ODD doc evidence updated; hand-off note for the implementation session. **Status: done — this section**

## Rules for this lane

- Allowed surfaces: odd/tasks/go-template-actuator.md, catalog/recipes/worktree-flow/gate/recipeconfigwrite_test.go-style NEW test file (e.g. templateactuator_test.go), tests/ Python RED tests if the bridge contract needs them.
- Do NOT touch main.go, SHA256SUMS, dist/, or any production .go/.py file — those belong to the implementation WU.
- No commits to the delivery lane; local commits on feat/go-template-actuator are fine after each task.

## Out of scope

- Implementation, native review, PR, merge. hook/gate actuator (shares classification port with this slice — port classify once when implementing).

## Scout: template actuator contract (read-only, slice 6)

All line refs against `lib/_internal/recipe-materialize.py` @ `6d4890e` (3157 lines).

### Callers (exactly one)

- `materialize_template(recipe_dir, tpl, project_root, merged_cfg, recipe_id=rid)` — called once per enabled recipe at `recipe-materialize.py:2939`, inside the per-recipe sync loop (`for rid, cfg in enabled.items()`), after skills and commands, before docs and hooks. `merged_cfg` there is `util.project_owned_recipe_config(project_root, None, rid, merge_config(recipe, manifest_config, home=cli_home))`.
- `resolve_template_dest` (:829) and `render_template_bytes` (:989) have no other callers. `doctor.py:1195` reads `tpl.condition` only (consumer of the schema, not of the actuator).

### Inputs

- `tpl` (recipe schema, `recipe_schema.py:380-410`): `source` (recipe-relative), `target` (project-relative or `.git/...`), `condition` (default `"not_exists"`), `update_policy` (default `"auto"`; schema validates `auto | confirm | never-force`, and the actuator RE-validates at :889 — the actuator refusal is independent of schema parsing).
- `merged_cfg` keys consumed: `repo_topology` (via `util.render_override_bytes`, default `"auto"`), `worktrees_dir` (default `".worktrees"`), `integration_branch` (default `"main"`). Empty/None values fall to the default (`str(cfg.get(key) or default)`).
- Lock: `project_root/ai-specs/.ai-specs.lock` via `lock.load_lock`; entry `(lock["managed"] or {}).get(target)` where `target = Path(tpl.target).as_posix()`. `lock.set_managed_override(lock, target, sha, recipe=recipe_id, source=tpl.source, kind="template", policy=policy)` then `lock.write_lock` — the LOCK WRITE STAYS IN PYTHON in the bridge design (record envelope handed back).
- Source mode: `dest` is `os.chmod(dest, src.stat().st_mode)` — full source permission bits copied.

### Behavior sequence (materialize_template, :866-948)

1. `dest = resolve_template_dest(project_root, tpl.target)` — `.git/...` targets via `git -C project_root rev-parse --git-path <remainder>`; relative output anchored at project_root; ANY failure (OSError, nonzero exit, empty stdout, non-`.git/` target) → literal `project_root / target`. Rationale: linked worktrees have `.git` as a gitfile; hooks land in the shared hooks dir.
2. Source check: `recipe_dir / tpl.source` must be a regular file, else `RuntimeError("template source not found: {src}")` (src is the absolute joined path).
3. `content = render_template_bytes(src, merged_cfg)`: `util.render_override_bytes` replaces `__WORKTREE_REPO_TOPOLOGY__` (with `cfg.repo_topology` or `auto` when cfg is None), then — only when cfg is not None — `__WORKTREE_WORKTREES_DIR__` → `worktrees_dir` or `.worktrees`, `__WORKTREE_INTEGRATION_BRANCH__` → `integration_branch` or `main`. Tokens absent → untouched. With cfg None the cleanup tokens stay literal (pin this edge).
4. Load lock; `target = Path(tpl.target).as_posix()`; validate policy against `util.OVERRIDE_POLICIES = ("auto", "confirm", "never-force")`, else `RuntimeError("invalid update policy '{policy}' for template '{target}'; expected auto | confirm | never-force")`.
5. `record(sha-of-bytes)`: upsert managed entry with sha256 (CRLF-NORMALIZED, `util.sha256_bytes`) of the bytes actually written (default: rendered content; seeding: the DEST's actual bytes).
6. `write_content()`: mkdir -p dest parent, write rendered bytes, chmod source mode.
7. `not_exists` + dest exists branch (`classify` = shared Go port, see below):
   - `untracked` and disk sha ∈ {rendered sha, source-catalog sha} → seed: `record(dest.read_bytes())`, NO rewrite (pre-render placeholder projects reconcile on next sync).
   - `untracked` otherwise → warn (exact string below), no write, NO record. Then falls through to the skip line.
   - `managed_stale` + policy `auto` → write+record+info("refreshed managed template {target}"), then falls through to the skip line.
   - `managed_stale` (confirm/never-force) or `user_modified` → warn (exact string below), no write, no record, falls through to skip line.
   - `managed_current` → backfill `record()` only (provenance), no rewrite, falls through to skip line.
8. Fall-through end: `print(f"    · template skipped (exists) {tpl.target}")` and return. Non-`not_exists` condition (or missing dest): always `write_content(); record(); print(f"    ✓ template {tpl.target}")`.

### Exact strings (warnings / errors / lines)

- `override metadata missing for {target}; preserving existing file without assigning ownership. To preserve this local file, leave it unchanged. To replace it with the current recipe version, remove it and run sync again:\n  rm {target} && ai-specs sync`
- `override user-modified: {target} was not refreshed. Refresh with:\n  rm {target} && ai-specs sync`
- `override managed-stale ({policy}-required): {target} was not refreshed. Refresh with:\n  rm {target} && ai-specs sync`
- `invalid update policy '{policy}' for template '{target}'; expected auto | confirm | never-force`
- `template source not found: {abs src}`
- lines: `✓ template {target}`, `· template skipped (exists) {target}`, info: `refreshed managed template {target}`

### Classification: shared port (do NOT re-port in slice 6)

The ownership decision (`util.classify_managed_override`, util.py:651 bridge / :808 fallback) is ALREADY ported once as the pure core `classifyManagedOverride` in `catalog/recipes/worktree-flow/gate/classify.go` behind `worktree-gate --plan-classify` (`GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK` in util.py). The slice 6 actuator core must REUSE `classifyManagedOverride` (same package) with the dest bytes + lock entry + rendered would-write bytes. `classify_managed_override` itself is NOT re-written here; its other consumers are slice 7's (recipe-init states, `recipe-init.py:192`) — single port, shared.

### Bridge pattern (fail-open, matching GO-05..GO-08)

- Reserved fallback token: `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK` (matches `GO_RECIPE_CONFIG_BRIDGE_FALLBACK` et al.). Implementation adds it to `recipe-materialize.py` as a module constant + single warning helper; exactly ONE warning line per degraded run, naming the reason; never raises.
- Go command: `worktree-gate --materialize-template` in a NEW `catalog/recipes/worktree-flow/gate/templateactuator.go`; handler `runMaterializeTemplate(stdin, stdout, stderr) int`, one JSON envelope in, one JSON envelope out, exit 0/2 — same shape as `--write-recipe-config`.
- Exit 2 (envelope error) or any bridge failure (no binary, bad JSON) → Python warns `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK: {reason}; ...` and runs the historical Python path above. The Go side OWNS: dest resolution, rendering, classification (via shared core), file write + chmod, and the record payload. Python owns: lock load/write, printing (indent + `print_step_output` compact filtering), recipe iteration.
- Envelope contract is pinned byte-for-byte by `templateactuator_test.go` (input keys: `project_root, recipe_dir, recipe_id, source, target, condition, update_policy, config{repo_topology,worktrees_dir,integration_branch}, managed_entry{sha256}`; output keys: `dest, wrote, record{target,sha256,recipe,source,kind,policy}|null, message, info, warnings[], error|null`). `record.target` is the posix form of `tpl.target`; `record.sha256` is the normalized sha of the bytes actually on disk after the actuation.

### RED suite note

`templateactuator_test.go` references `runMaterializeTemplate`, `resolveTemplateDest`, `renderTemplateBytes` — none exist yet, so the gate package INTENTIONALLY fails to compile on this branch until the implementation WU lands `templateactuator.go`. That is the contract: the tests are written first and define acceptance for slice 6.

## Hand-off note (implementation session, after GO-08 merges)

1. **Rebase this branch** over the post-GO-08 `development` and resolve the gate package compile break by ADDING the core, never by weakening the tests. The suite is the acceptance bar; exact strings are contractual.
2. **New file** `catalog/recipes/worktree-flow/gate/templateactuator.go` (package main) with: `renderTemplateBytes(src []byte, config map[string]any) []byte` (pure), `resolveTemplateDest(projectRoot, target string) string` (subprocess `git -C <root> rev-parse --git-path <remainder>`, fail-open to the literal join), and `runMaterializeTemplate(stdin io.Reader, stdout, stderr io.Writer) int` (envelope contract as pinned in ## Scout + the test structs).
3. **Reuse** `classifyManagedOverride` (classify.go) and `sha256Bytes` — no new classification port; slice 7 consumes the same port.
4. **main.go wiring** (one flag + one case, mirroring `writeRecipeConfigCmd`): `--materialize-template`. This and SHA256SUMS/dist are OUTSIDE this lane — implementation WU only.
5. **Python bridge** in `recipe-materialize.py`: module constant `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK`, one warning per degraded run, exit 2 / bad JSON / no binary → run the historical Python path; lock record comes back in `output.record` and Python owns `load_lock`/`set_managed_override`/`write_lock` and all printing (indent + `print_step_output`).
6. **Validation**: `go test ./...` in the gate dir (suite must go green), `./tests/run.sh`, and an end-to-end `sync` smoke with gate_impl=go for the post-merge hook target in a linked worktree.

### Local commits on feat/go-template-actuator (prep lane)

- `001ef58` docs(go-09): scout template actuator contract in ODD doc
- `96477f3` test(go-09): RED parity suite for the Go template actuator (slice 6 prep)
- this commit (ODD evidence + hand-off)

## Implementation evidence (slice 6, implementation session)

Commits: the delivery lane owns them (working-tree hand-off, nothing committed by this session).

### What landed

- `catalog/recipes/worktree-flow/gate/templateactuator.go` (NEW): `renderTemplateBytes` (pure; topology token with the `str(cfg.get(key, "auto"))` semantics, cleanup tokens ONLY when config is non-nil, CRLF byte surgery only), `resolveTemplateDest` (fail-open git-path resolution), `runMaterializeTemplate` (one JSON envelope in/out, exit 0/2), reusing `classifyManagedOverride` and `sha256Bytes` from classify.go — no second classification port. The actuator types are `templateActuatorRequest/Record/Output` because the RED suite owns the `templateInput/...` names in its own file.
- `catalog/recipes/worktree-flow/gate/main.go`: one flag (`--materialize-template`) + one case, mirroring `writeRecipeConfigCmd`.
- `lib/_internal/recipe-materialize.py`: `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK` + `go_materialize_template` bridge (exit-2-with-error-envelope FAILS CLOSED per the GO-08 findings fix; infra failures warn exactly once and fall back), `materialize_template` dispatcher (Python keeps lock load/write, all printing), historical body preserved as `_python_materialize_template`.
- `tests/test_template_actuator_bridge.py` (NEW): 16 tests — Go-path parity (fresh write, seed, stale auto refresh, user-modified), envelope contract, refusal fail-closed, and the fallback matrix (missing binary, garbage stdout, exit 2 without error envelope, envelope mismatch, exact RuntimeError strings).
- `catalog/recipes/worktree-flow/bin/SHA256SUMS`: regenerated via `scripts/build-gate.sh` with the canonical go1.24.13 toolchain (no toolchain warning emitted), 4/4 digests.

### RED/GREEN evidence (observed)

- RED (before implementation): `cd catalog/recipes/worktree-flow/gate && go vet ./...` failed on the undefined symbols (`undefined: runMaterializeTemplate` first at templateactuator_test.go:142) — the suite did not compile.
- GREEN: `cd catalog/recipes/worktree-flow/gate && gofmt -l .` → empty; `go vet ./...` → clean; `go test ./...` → 22 of the suite's behaviors pass; 10 fixture-seeded cases fail INSIDE THE TEST'S OWN SETUP (see defect below).
- `env -u WORKTREE_GATE_BIN python3 -m unittest tests.test_template_actuator_bridge tests.test_materialize_bridge tests.test_merge_config_bridge` → Ran 85 tests, OK.
- `WORKTREE_GATE_BIN=$PWD/dist/worktree-gate-current python3 -m unittest tests.test_template_actuator_bridge tests.test_materialize_bridge tests.test_merge_config_bridge` → Ran 85 tests, OK.
- `python3 -m unittest discover -s tests -p 'test_*materialize*.py'` → Ran 97 tests, OK.
- Extra focused parity: `python3 -m unittest tests.test_override_ownership` OK both without the binary (Python fallback) and with it (Go path) — the retained authority suite is behavior-identical through the bridge.

### RED suite defect (reported, NOT fixed — suite treated as read-only)

Ten test cases fail in their own fixture setup, before the actuator is ever invoked: they call `os.WriteFile(dest, ...)` without creating the destination's parent directories (os.WriteFile never creates parents), so no implementation can make them pass. Exact sites: TestTemplateActuatorNotExistsSeedsRenderedCopy (:381), TestTemplateActuatorNotExistsSeedsLegacyPlaceholder (:407), TestTemplateActuatorNotExistsPreservesUntracked (:435), TestTemplateActuatorManagedStaleAutoRefresh (:468), TestTemplateActuatorStaleRefusalPolicies/confirm + /never-force (:502), TestTemplateActuatorUserModifiedRefusal (:532), TestTemplateActuatorManagedCurrentBackfill (:557), TestTemplateActuatorCRLFShaParity/backfill_normalizes (:585), TestTemplateActuatorAlwaysConditionOverwrites (:625). Minimal fix (delivery lane): add `os.MkdirAll(filepath.Dir(dest), 0o755)` before each seeded `os.WriteFile`. The behaviors they pin are implemented and verified: legacy-placeholder seeding, managed-current backfill, CRLF backfill, and the always-overwrite path were smoke-verified through the real binary / the Python bridge tests.

### Contract deviation to review

- `resolveTemplateDest` runs `git rev-parse --path-format=relative --git-path <remainder>` (retrying without the flag for git < 2.31). The Python reference emits absolute paths, which canonizes symlinks — on macOS the RED worktree test's `t.TempDir()` path (`/var/folders/...`) would never equal git's real-path output (`/private/var/...`). The relative emission keeps the path lexical (anchored at the caller's project root), which is what the test pins and what a caller expects; the resolved file is identical. The Python fallback keeps its historical absolute behavior.
- Exit-2 routing: the ODD doc's "exit 2 → Python fallback" was superseded (per the parent's instruction and the GO-08 native review) by fail-CLOSED on a valid exit-2 error envelope (policy refusal, missing source, execution failure) and fail-open only on infrastructure failures. No RED test pinned the conflicting fallback behavior; the conflict did not materialize in the suite.
