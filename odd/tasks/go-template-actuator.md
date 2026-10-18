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

### RED suite defect (resolved — fixed in the test file)

Ten test cases originally failed in their own fixture setup, before the actuator was ever invoked: they called `os.WriteFile(dest, ...)` without creating the destination's parent directories (os.WriteFile never creates parents). That fixture-seeding defect was fixed in the Go test file during implementation (parent directories are created before each seeded `os.WriteFile`); no assertion was changed. The suite is green on this branch: `go test ./...` passes the full behavior set, including legacy-placeholder seeding, managed-current backfill, CRLF backfill, and the always-overwrite path.

### Contract deviation to review

- `resolveTemplateDest` runs `git rev-parse --path-format=relative --git-path <remainder>` (retrying without the flag for git < 2.31). The Python reference emits absolute paths, which canonizes symlinks — on macOS the RED worktree test's `t.TempDir()` path (`/var/folders/...`) would never equal git's real-path output (`/private/var/...`). The relative emission keeps the path lexical (anchored at the caller's project root), which is what the test pins and what a caller expects; the resolved file is identical. The Python fallback keeps its historical absolute behavior.
- Exit-2 routing: the ODD doc's "exit 2 → Python fallback" was superseded (per the parent's instruction and the GO-08 native review) by fail-CLOSED on a valid exit-2 error envelope (policy refusal, missing source, execution failure) and fail-open only on infrastructure failures. No RED test pinned the conflicting fallback behavior; the conflict did not materialize in the suite.

### Candidate-1 findings fix (native review, GO-09 template actuator)

Fixed exactly the three WARNING findings; the six SUGGESTION findings
(R1-dest-containment-absent, R2-emit-refuse-dup, R2-render-fallback-asymmetry,
R2-warn-str-dup, R3-2, R3-3) remain deliberately deferred to a later
hardening PR — nothing in this change touches them.

- **R1-symlink-following-dest-write** + **R3-1**: `writeTemplateContent` now
  Lstats the destination first and refuses any symlink (dangling or not) with
  `errDestSymlink`; both write-failure call sites (managed_stale policy-auto
  refresh and the final write) render through `writeTemplateRefusal`, mapping
  the guard to the actionable `templateSymlinkRefusal`. The Python fallback
  authority mirrors the refusal verbatim (`_symlink_refusal`) and guards
  `write_content` with `dest.is_symlink()`, raising `RuntimeError` (the same
  class the bridge fails closed on).
- **R2-exit2-shape-split**: the file-header comment now documents the
  deliberate exit-2 taxonomy (INPUT/INFRASTRUCTURE stderr diagnostics vs
  DECISION error envelopes). Behavior unchanged.

RED observation (before the fix): `go test ./...` failed with
`undefined: templateSymlinkRefusal` (templateactuator_test.go:749) — the suite
does not compile without the guard; the behavior it pins was a through-link
write (pre-fix run of the overwrite/refresh subtests showed exit 0 + link
target rewritten, and the Python test failed `AssertionError: RuntimeError not
raised`, i.e. the fallback wrote through the link and created the dangling
link's target).

GREEN commands (all observed):
- `cd catalog/recipes/worktree-flow/gate && gofmt -l . && go vet ./... && go test ./...` — gofmt empty, vet clean, `ok ai-specs.dev/worktree-gate` + `ok ai-specs.dev/worktree-gate/ledger` (includes the new TestTemplateActuatorSymlinkDestRefused: overwrite, managed_stale auto refresh, and dangling-link subtests).
- `python3 -m unittest tests.test_template_actuator_bridge tests.test_materialize_bridge tests.test_merge_config_bridge` — Ran 86 tests, OK.
- `WORKTREE_GATE_BIN=$PWD/dist/worktree-gate-current python3 -m unittest tests.test_template_actuator_bridge` — Ran 17 tests, OK.
- `python3 -m unittest discover -s tests -p 'test_*materialize*.py'` — Ran 97 tests, OK.
- `scripts/build-gate.sh` — 4 targets built, no toolchain warning (go1.24.13); `scripts/verify-gate-sums.sh <(cd dist && shasum -a 256 worktree-gate-* | grep -v current) catalog/recipes/worktree-flow/bin/SHA256SUMS` — ok, 4 digest entries match.

### Candidate fix batch 2 (native review, GO-09 template actuator)

Fixed exactly the three WARNING findings; the SUGGESTION findings
(R1-ancestor-symlink-traversal-deferred, R1-toctou-residual-symlink-race,
R2-refusal-string-dual-authority) remain deliberately deferred to the
advisory-hardening PR — nothing in this change touches them.

- **R3-toctou-go-dest-guard**: `writeTemplateContent` keeps the Lstat
  pre-check (early actionable refusal + dangling-link/not_exists path) and
  now opens the destination with `os.OpenFile(... | syscall.O_NOFOLLOW)`,
  mapping `syscall.ELOOP` back to `errDestSymlink`, so a link swapped in
  between the check and the write can no longer be followed.
- **R3-toctou-py-dest-guard**: the Python fallback `write_content` mirrors
  the same two layers — `dest.is_symlink()` refusal kept, write switched to
  `os.open(... | O_NOFOLLOW)` with `errno.ELOOP` mapped to the SAME
  `_symlink_refusal` string (parity contract).
- **R2-shasums-command-pruned**: the SHA256SUMS header "Reproduce any digest
  locally with:" block and the truncated go-binding-witness note now carry
  the complete two-step command (build-gate.sh + the explicit four-arch
  shasum invocation); no note reordered or deleted, digests regenerated.

RED observation (before the fix): the new regression tests pass pre-change
(`TestWriteTemplateContentRefusesSymlinkDest` both subtests OK;
`test_fallback_refuses_symlinked_destination` extended dangling-case
assertEqual OK) because the candidate-1 Lstat pre-check already refuses both
symlink cases. The O_NOFOLLOW layer closes the check-to-open swap window,
which is not observable by a behavior test without a deliberate race harness
or an injection hook — both excluded by the batch's no-abstraction scope
rule — so no pre-implementation failing run was available (justified
exception; the race-layer fix is pinned indirectly by the ELOOP→refusal
mapping sharing the same `errDestSymlink`/refusal-string contract the
existing tests pin).

GREEN commands (all observed):
- `cd catalog/recipes/worktree-flow/gate && gofmt -l . && go vet ./... && go test ./...` — gofmt empty, vet clean, `ok ai-specs.dev/worktree-gate` + `ok ai-specs.dev/worktree-gate/ledger` (includes new `TestWriteTemplateContentRefusesSymlinkDest` regular-file + dangling subtests).
- `python3 -m unittest tests.test_template_actuator_bridge tests.test_materialize_bridge tests.test_merge_config_bridge` — Ran 86 tests, OK.
- `WORKTREE_GATE_BIN=$PWD/dist/worktree-gate-current python3 -m unittest tests.test_template_actuator_bridge` — Ran 17 tests, OK (against the rebuilt binary).
- `python3 -m unittest discover -s tests -p 'test_*materialize*.py'` — Ran 97 tests, OK.
- `scripts/build-gate.sh` — 4 targets built, no toolchain warning (go1.24.13); `bash scripts/verify-gate-sums.sh /tmp/gate-sums-generated catalog/recipes/worktree-flow/bin/SHA256SUMS` — ok, 4 digest entries match (digests spliced from the generated shasum output, never hand-edited).

### Candidate fix batch 3 (native review, GO-09 template actuator bridge)

Fixed exactly the five WARNING findings of lineage review-23e32be9a662e564
(candidate = the bridge module commit); every SUGGESTION from this lineage and
from review-faee79d90d3f3863 / review-53597d8ba46f3a25 remains deferred to the
advisory-hardening PR — nothing in this change touches them.

- **R1-lock-record-trust**: the Go-returned record is now validated against
  the plan Python actually sent (`_template_record_mismatch`): target (posix),
  source, recipe, policy, kind == "template", and sha256 as 64-char lowercase
  hex (same spirit as the GO-08 results-count-mismatch guard). Any mismatch →
  exactly ONE fallback warning naming the mismatch, then the historical Python
  body; the unvalidated record never reaches the lock.
- **R3-record-null-lock-drift**: `wrote: true` with `record: null` is now an
  envelope mismatch — one fallback warning naming "wrote without a record",
  then the Python body (which rewrites the destination and records it itself).
- **R3-bridge-timeout-double-exec**: `subprocess.TimeoutExpired` is caught
  separately; the single fallback warning names the timeout with the value in
  seconds (`timed out after 60s`), and the fallback docstring notes the
  timed-out Go run may already have written the destination, which the Python
  body rewrites idempotently. No retries, timeout value unchanged.
- **R2-docstring-omits-fail-closed**: the `materialize_template` docstring now
  states both failure shapes — fail-CLOSED RuntimeError on a delivered exit-2
  REFUSAL envelope, fail-OPEN one-warning fallback on infrastructure failures
  (no verified binary, crash, timeout, malformed/mismatched envelope).
- **R3-red-parity-suite-broken**: the stale "RED suite defect (reported, NOT
  fixed)" paragraph now states the truth — the fixture-seeding defect was
  fixed in the Go test file (no assertion changed) and the Go suite is green
  on this branch.

RED observation (before the fix): the four new stub-envelope tests failed —
`err.count(GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK)` was 0 for the mismatching
`record.target`/`record.source`/non-hex `sha256` cases (the record was trusted
and written into the lock as-is) and for `wrote: true, record: null` (the lock
update was silently skipped) — `env -u WORKTREE_GATE_BIN python3 -m unittest
tests.test_template_actuator_bridge -k record -k wrote` → Ran 5, FAILED
(failures=4, 1 pre-existing skip). No existing assertion was changed or
weakened.

GREEN commands (all observed):
- `python3 -m unittest tests.test_template_actuator_bridge tests.test_materialize_bridge tests.test_merge_config_bridge` — Ran 90 tests, OK.
- `WORKTREE_GATE_BIN=$PWD/dist/worktree-gate-current python3 -m unittest tests.test_template_actuator_bridge` — Ran 21 tests, OK.
- `python3 -m unittest discover -s tests -p 'test_*materialize*.py'` — Ran 97 tests, OK.
- `python3 -m unittest tests.test_override_ownership` — Ran 18 tests, OK.
- Gate asset unchanged: no Go file touched, no rebuild run.
