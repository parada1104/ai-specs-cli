# ODD Feature: advisory-hardening (campaign)

**Card**: Advisory hardening GO-07..GO-10 — campaign (Trello `6ab7ffd57e187ad0fd9bc1e4`, https://trello.com/c/AIKLqfb6/154-advisory-hardening-go-07go-10-campaign-5-parallel-lanes)
**Base**: development `888a425e726c8d45351f96a6803e21d859b959de`
**Strategy**: same as the strangler rank-3 chain — parallel implementation lanes in isolated worktrees, serialized review/validate/PR/merge through the parent session.

## Tracker

- card_id: 6ab7ffd57e187ad0fd9bc1e4
- url: https://trello.com/c/AIKLqfb6/154-advisory-hardening-go-07go-10-campaign-5-parallel-lanes

## Goal

Close the ~47 advisories frozen by announced stop rules across GO-07/08/09/10 (native-review non-blocking findings), grouped into five disjoint file-surface lanes. Every advisory is dispositioned; none is blindly "fixed".

## Standing rules (all lanes)

1. **Verify-then-fix.** Recorded locations are pre-correction line numbers: re-locate by symbol on the current tree. Confirm the finding still reproduces before fixing. Dispositions per advisory id: `fixed` / `stale` (region already rewritten — cite why) / `not-a-defect` (cite why) / `deferred` (outside surfaces or blocked — report, never edit another lane's file). Never invent claim content: the native store only preserves id/lens/location/severity, and burned receipts are not reopened.
2. **TDD.** RED→GREEN for every behavior-observable fix; for pure-deletion/docs fixes, before/after evidence (grep counts, suites green). Never weaken an existing assertion.
3. **Scoped surfaces.** Each lane edits only its listed files. A finding whose fix belongs to another surface is REPORTED to the parent, not fixed.
4. **Commits.** Commit per work-unit on the lane branch (Conventional Commit). Never push; the parent owns native review, `./tests/validate.sh`, PR, merge.
5. **Go asset changes.** If any `catalog/recipes/worktree-flow/gate/*.go` file changes: rebuild via `scripts/build-gate.sh` (canonical go1.24.13, no toolchain warning), regenerate `bin/SHA256SUMS` with its documented reproduction command, `scripts/verify-gate-sums.sh` → 4/4, `gofmt -l .` empty, `go vet .` clean.
6. **Evidence.** Each lane appends its disposition table + RED/GREEN commands + observed results to its section in this file.
7. **Reply.** When the lane is done (or blocked), reply to the parent session via intercom with a one-paragraph summary; the parent verifies.

## Lane inventory

### Lane C1 — bridge (`feat/adv-bridge`, `.worktrees/adv-bridge`)
Surfaces: `lib/_internal/recipe-materialize.py`, `tests/test_hook_gate_bridge.py`, `tests/test_copy_apply_bridge.py`, `tests/test_template_actuator_bridge.py`, this doc (C1 section only).
Advisories (source lineages: review-a7f5190ce6818b22 fix batch 904add9; review-2a8449d06a0c1d5b final fix 84a88bc):
- WARNING R3-fallback-warning-order — recipe-materialize.py (recorded :1839-1858)
- WARNING R4-001 — recipe-materialize.py (recorded :1846-1852)
- WARNING R4-002 — recipe-materialize.py (recorded :1856-1860)
- WARNING R4-unowned-gate-baseline — recipe-materialize.py, gate-baseline handling (location unrecorded; locate)
- SUGGESTION R3-reconcile-check-then-act
- SUGGESTION R3-untested-missing-dest-arm (test)
- SUGGESTION R1-symlink-preflight-toctou-window
- SUGGESTION R2-001, R2-002 (locations unrecorded; locate in the bridge module)
- Unrecorded SUGGESTIONs from review-2a8449d06a0c1d5b: re-inspect the module for the frozen classes (reconcile order, unowned baseline, dest-arm coverage); fix only what is verifiable.
- GO-08 candidate-6 SUGGESTIONs that land in the copy-bridge sections of recipe-materialize.py: re-inspect; report any that belong to lock.py instead.
- Python-side template-bridge advisories reported by Lane C3 (absorb if in-surface).

### Lane C2 — hookact (`feat/adv-hookact`, `.worktrees/adv-hookact`)
Surfaces: `catalog/recipes/worktree-flow/gate/hookgateactuator.go`, `catalog/recipes/worktree-flow/gate/hookgateactuator_test.go`, this doc (C2 section only).
Advisories (source lineage: review-bc356c28e2236248 C1, candidate tree a236fa2 PRE-correction — the :341-373 region was later rewritten by the R4-001 CRITICAL correction; expect stale findings and re-verify each):
- WARNING R2-refresh-backup-contract (readability, :355-370)
- WARNING R3-001 (reliability, :355-371)
- WARNING R3-002 (reliability, :341-373)
- WARNING R4-002 (resilience, :308-314 recorded as :379-381 pre-correction)
- WARNING R4-003 (resilience, :308-314)
- SUGGESTION R1-hook-rel-path-unsanitized (risk, :84-86 — `hookScriptRelPath`)
- SUGGESTION R3-003 (:84-86)
- SUGGESTION R3-004 (:216-222)
- SUGGESTION R2-dup-emit-refuse (:227-245), R2-dup-write-branches (:290-309), R2-refresh-signature (:332-340)
- SUGGESTION R2-fixture-gatemode-context (hookgateactuator_test.go:143)

### Lane C3 — template (`feat/adv-template`, `.worktrees/adv-template`)
Surfaces: `catalog/recipes/worktree-flow/gate/templateactuator.go`, `catalog/recipes/worktree-flow/gate/templateactuator_test.go`, this doc (C3 section only).
Advisories (source lineages: review-d9a50a413d636b30 symlink fix; review-53597d8ba46f3a25 TOCTOU/sums fix; review-11110035aff82cba core if relevant):
- SUGGESTION R1-dest-containment-absent (writeTemplateContent dest containment)
- SUGGESTION R1-ancestor-symlink-traversal-deferred (ancestor-path symlink walk)
- SUGGESTION R1-toctou-residual-symlink-race (residual Lstat→write race)
- SUGGESTION R2-emit-refuse-dup, R2-render-fallback-asymmetry, R2-warn-str-dup, R2-refusal-string-dual-authority, R3-2, R3-3 — locations unrecorded; those landing in the Go actuator are fixed here; Python-side (recipe-materialize.py / its bridge tests) are REPORTED to the parent for Lane C1.
- Unrecorded bridge-tests SUGGESTIONs from review-faee79d90d3f3863: re-inspect the Go surface only; report the rest.

### Lane C4 — toml (`feat/adv-toml`, `.worktrees/adv-toml`)
Surfaces: `catalog/recipes/worktree-flow/gate/recipeconfigwrite.go`, `catalog/recipes/worktree-flow/gate/recipeconfigwrite_test.go`, `lib/_internal/recipe-config-write.py`, `tests/test_recipe_config_write_bridge.py`, `odd/tasks/go-toml-writer.md` (doc checkbox item only), this doc (C4 section only).
Advisories (source lineages: review-e383bb1f24b02d5c WU1a, review-52660b243e07a157 WU1b, review-75d0a47dfe8e04e2 WU2):
- WU1a: R2-dead-return (:743), R2-enabled-default (:981-987), R2-ponytail-tag (:367-369), R2-rewrite-dup (:1035-1044), R2-root-shadow (:968-973), R3-missing-test-coverage (file-wide — BOUND IT: cover the specific untested branches you can identify, cap the effort, list what remains), R3-recipeRegionEnd-unclosed-prefix (:698)
- WU1b: R3-build-deps (test :42), R3-envelope-quote (test :40), R3-ff-dup (test :101-107), R3-doc-checkbox (odd/tasks/go-toml-writer.md:14)
- WU2: R3-2 (tests/test_recipe_config_write_bridge.py:241-253), R3-3 (lib/_internal/recipe-config-write.py:440-448)
- gate-hang-latency timeout (GO-07 out-of-scope note): verify whether the bridge timeout (GO_RECIPE_CONFIG_WRITE_TIMEOUT_SECONDS=60) leaves a gate-hang class uncovered; fix in-surface only.
- Note: `recipe_toml.go` in the old out-of-scope note is a stale reference; the 14 recorded advisories above are authoritative.

### Lane C5 — lock (`feat/adv-lock`, `.worktrees/adv-lock`)
Surfaces: `lib/_internal/lock.py`, `tests/test_lock.py`, this doc (C5 section only).
Advisories (source: GO-08 batch-2/final-fix review, candidate 6):
- WARNING guard-branch test coverage (tests/test_lock.py) — identify the insufficiently covered guard branch (control-char refusal / results-count guard) and add pinning tests.
- 3 SUGGESTIONs (ids unrecorded): re-inspect lib/_internal/lock.py + tests/test_lock.py for the frozen classes around the control-char refusal batch; fix only what is verifiable.

## Delivery (parent-owned, serialized)

Per lane, in merge-dependency-free order: rebase lane onto current development if needed → regenerate sums if Go changed → focused suites → full `./tests/validate.sh` → native review (protocol skill; single-slot captures if a lens is fragile; baseRef = full SHA of the parent commit, committedOnly true) → burn approval → PR → **merge only with explicit human authorization** → cleanup. After each merge, sync development and re-verify sums for the next lane.

## Progress

- [ ] C1 bridge — implementation lane
- [ ] C2 hookact — implementation lane
- [ ] C3 template — implementation lane
- [ ] C4 toml — implementation lane
- [ ] C5 lock — implementation lane
- [ ] Serialized delivery: review → validate → PR per lane (×5)

## Lane C1 evidence

(appended by Lane C1 worker)

## Lane C2 evidence

(appended by Lane C2 worker)

## Lane C3 evidence

(appended by Lane C3 worker)

## Lane C4 evidence

(appended by Lane C4 worker)

## Lane C5 evidence

Worktree `.worktrees/adv-lock`, branch `feat/adv-lock`, base `development` `888a425`. Surfaces touched: `tests/test_lock.py` only (no defect in `lib/_internal/lock.py` survived verification; see S1–S2).

### WARNING guard-branch test coverage (tests/test_lock.py) — **fixed** (`acc60fd`)

**Identification.** Re-located on the current tree by symbol. The lock writer's guard surface is the control-char refusal batch (`_has_control_char` / `_refuse_control_chars`, `lib/_internal/lock.py`, mirroring `lockwrite.go hasControlChar` + `firstControlCharLocator` + the `lock_path` check). The pre-existing `FallbackControlCharRefusalTests` pinned only 3 of the guard's locator branches (`agents hash`, `managed path`, `meta.synced_at`) plus a clean write. Uncovered guard branches: `lock_path`, `meta.cli_version`, every `managed.<key>` value (sha256/recipe/source/kind/policy), `agents harness`, `agents filename`, the user-facing `write_lock` fallback route, and the emitter skip guards the refusal walk must mirror.

**The "results-count guard" half is not in this surface.** `grep -n "count\|Count" lib/_internal/lock.py` → no match. The GO-08 results-count-mismatch guard lives in the copy bridge: `lib/_internal/recipe-materialize.py:894` (`len(stdout["results"]) != len(items)` inside `go_apply_copy`) and is already covered on that surface by `tests/test_copy_apply_bridge.py` (ResultsCountMismatchTests / `test_empty_results_for_one_item_is_an_envelope_mismatch`, :609-626). Disposition for C5: stale (region belongs to Lane C1's files and is already pinned there); reported to the parent for confirmation — no C1 file was edited.

**RED (mutation checks — the guard already exists, so the tests are proven to pin it):**

1. `_has_control_char` body neutered (`return False  # MUTATION`):
   `python3 -m unittest tests.test_lock` → `Ran 20 tests ... FAILED (failures=13)` — all new refusal-locator tests catch the neutered guard.
2. managed sha256 skip guard dropped from `_write_lock_python` (`not entry.get("sha256")` removed):
   → `Ran 20 tests ... FAILED (failures=1)` — `test_skip_branches_write_without_refusal` catches it.

**GREEN (mutation reverted, tree verified clean vs `888a425` apart from `tests/test_lock.py`):**

- `python3 -m unittest tests.test_lock` → `Ran 20 tests ... OK`
- `python3 -m unittest tests.test_lock tests.test_lock_bridge` (no binary) → `Ran 23 tests ... OK (skipped=1)` (loud skip: no Go authority)
- `WORKTREE_GATE_BIN=<throwaway binary built from this worktree's unchanged Go sources with go1.24.13 into /tmp only; no repo artifacts, no SHA256SUMS touched> python3 -m unittest tests.test_lock tests.test_lock_bridge` → `Ran 27 tests ... OK`
- Note: the main checkout's `dist/worktree-gate-current` is stale (v0.24.0, predates `--write-lock`: `flag provided but not defined`), so a local throwaway binary was used for the with-binary run only.

New pinning tests (all assert the exact Go-parity locator wording and `nothing written on refusal`): `test_control_char_in_lock_path_is_refused`, `test_control_char_in_meta_cli_version_is_refused`, `test_control_char_in_managed_entry_value_is_refused` (subTest per key ×5), `test_control_char_in_agents_harness_is_refused`, `test_control_char_in_agents_filename_is_refused`, `test_write_lock_fallback_route_refuses_control_chars`, `test_skip_branches_write_without_refusal`, `test_non_dict_managed_entry_is_skipped_not_refused`, and `RemoveRecipeLockEntriesTests` (pins the boolean results guard of `remove_recipe_lock_entries`: True exactly when a recipe section existed and was removed).

### Unrecorded candidate-6 SUGGESTIONs (re-inspection of the frozen classes around the refusal batch)

- **S1 — refusal-wording dual authority (Python `_refuse_control_chars` vs Go writer).** Verified branch-by-branch against `lockwrite.go`: `lock_path`, `meta.<key>`, `managed path`, `managed.<key>` (sha256, recipe, source, kind, policy), `agents harness`, `agents filename`, `agents hash` — wording and boundary (0x20 / 0x7F) identical; Python checks `meta` values for exactly the two keys the envelope builder emits (`cli_version`, `synced_at`), so the Go meta-key walk has no reachable Python counterpart. **not-a-defect**: parity holds and is now pinned per branch by exact-message assertions. A cross-authority runtime parity test (Python vs live Go refusal strings) would belong in `tests/test_lock_bridge.py` — outside C5's surfaces; reported to the parent.
- **S2 — check-then-act double walk** (`_refuse_control_chars` validates, then `_write_lock_python` re-walks to emit). **not-a-defect**: single-threaded call on an unchanged dict; the docstring states the contract ("Only the emitted surface is walked — the fallback writes exactly what this function inspects"); verified every `_toml_string` call-site input appears in the refusal walk's checks list, so there is no emit-without-inspect gap. Folding validation into emission would obscure the Go-parity boundary for no shrink in code.
- **S3 — skip-branch / refusal interplay coverage.** **fixed** (`acc60fd`): the emitter skip guards (managed entry without sha256, non-dict entry, empty agents harness) are now pinned to be skipped — never refused, never emitted — and the fallback route through the public `write_lock` is pinned to refuse.

**Suites run:** `python3 -m unittest tests.test_lock tests.test_lock_bridge`, with and without `WORKTREE_GATE_BIN` — all green (see above). `./tests/validate.sh` not run (parent-owned). No push, no PR.
