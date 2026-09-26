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

Worker: Lane C1 bridge (`feat/adv-bridge`, this worktree). Surfaces touched: `lib/_internal/recipe-materialize.py`, `tests/test_template_actuator_bridge.py`, `tests/test_copy_apply_bridge.py`, this section. Go sources unchanged → no rebuild, SHA256SUMS untouched (rule 5 N/A; a local dist binary was built for WORKTREE_GATE_BIN runs only, into `tmp/`, never committed).

Baseline on arrival (base `888a425`): focused suites green — `python3 -m unittest tests.test_hook_gate_bridge tests.test_copy_apply_bridge tests.test_template_actuator_bridge` → 44 OK (5 loud skips) without a binary; 68 OK with `WORKTREE_GATE_BIN` (dist binary built via `scripts/build-gate.sh`, canonical go1.24.13, no toolchain warning).

Verification context: the review fix batches 904add9 + 84a88bc are byte-identical to this lane's surfaces on the current tree (`git diff --stat 84a88bc HEAD -- <4 surfaces>` empty; the PR squashes absorbed them), so every recorded pre-correction location was re-located by symbol, not by line.

### Disposition table

| Advisory (lens) | Disposition | Evidence / reasoning |
|---|---|---|
| R3-fallback-warning-order (WARNING, recorded :1839-1858) | **stale** | Region = `_fallback_materialize_and_reconcile`, rewritten by the final fix 84a88bc (in-tree content): the two unconditional `_warn_hook_bridge_fallback` emits became the `announce`/`emit()` split — `announce=False` on the `output is None` path (the token warning was already emitted by `go_materialize_hook`) and `announce=True` on envelope-shaped failures (which had not warned yet). A degraded run carries exactly one token warning; pinned by `assert_reconciled_repair` (`err.count(GO_HOOK_GATE_BRIDGE_FALLBACK) == 1`) and the repair tests (malformed stdout / crash exit 70 / timeout after Go write). Re-verified on the current tree: no call site double-warns. |
| R4-001 (WARNING, recorded :1846-1852) | **stale** | Region = the fallback's baseline-recording block, rewritten by 84a88bc: the baseline is recorded only for ON-DISK bytes whose digest equals the CLI-rendered content (`disk_sha` arm), with the dispatcher snapshotting `pre_dest_bytes` and routing every unusable envelope (None / wrote-null / mismatched record) through the reconcile. Pinned by `test_wrote_true_null_record_with_written_dest_reconciles_the_lock`, `test_record_matching_disk_is_applied_as_before`, `test_record_sha_mismatching_disk_preserves_user_bytes_and_lock`. |
| R4-002 (WARNING, recorded :1856-1860) | **stale** | Region = the fallback's else-arm warning that claimed "no bytes were changed" even when a degraded Go run had left changed, unproven bytes. Rewritten by 84a88bc: changed-but-unproven bytes now FAIL CLOSED (accurate warning + `RuntimeError`, actionable `rm` hint); unchanged bytes keep the preserve/no-record behavior. Pinned by `test_infra_failure_leaving_unproven_changed_bytes_fails_closed` and `test_infra_failure_with_preexisting_user_gate_preserves_bytes_and_baseline`. |
| R4-unowned-gate-baseline (WARNING, gate-baseline handling; locate) | **fixed** (9b622c7) | Audited every lock-ownership record site in the module. Hook `set_gate_baseline` sites are all verified-CLI-rendered (dispatcher disk check; fallback digest-compare; body/refresh record just-written bytes). The template `set_managed_override` dispatcher had NO disk check: a plan-matching record was applied to the lock even when the destination on disk was absent or held different bytes — an unverified/unowned lock record (the exact class the hook bridge hardened in 904add9 as R1-lock-baseline-unverified-disk). Fix: `materialize_template` now hashes the resolved destination (`resolve_template_dest`) and applies the record only when the digest matches; otherwise one `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK` warning names "the returned record sha256 does not match the destination on disk" and the Python body rewrites/records itself. RED→GREEN below. |
| R3-reconcile-check-then-act (SUGGESTION) | **deferred** | Located: `_fallback_materialize_and_reconcile` hashes the destination, then `write_lock`s — a residual hash-then-act window between the disk check and the lock write (and the mirror-image window in the Go-success path between Go's write and Python's lock write). Closing it requires the file-write authority and the lock authority to act in one process (Go owning the lock write), which is the already-planned migration — outside this lane's architecture and surfaces. No single-writer sync hazard exists (sync is the only lock writer in a run). Reported to the parent. |
| R3-untested-missing-dest-arm (SUGGESTION, test) | **fixed** (2c6f16f) | Located: `CopyApplyGoAuthorityTests` covered dest-exists (overwrite warn) and dest-identical (no warn) but never the fresh command copy with dest ABSENT on the Go path. Added `test_command_go_path_missing_dest_copies_without_warn` (no fallback, empty stderr, source bytes at dest). Coverage-only: the behavior was already correct, so no RED is achievable; evidence is before/after (test count 68→69 with binary, suite green). |
| R1-symlink-preflight-toctou-window (SUGGESTION) | **not-a-defect** | The Python pre-flight `dest.is_symlink()` refusal is defense-in-depth, and the residual check-then-act window cannot produce a write through a link: the fallback write re-verifies at open (`os.open` with `O_NOFOLLOW`, `ELOOP` → `_symlink_refusal`), and the Go authority Lstats + opens with `syscall.O_NOFOLLOW` (`templateactuator.go:282-288`, mirrored for hooks). The window can only end in a refusal (fail closed), never in a symlink write. |
| R2-001 (SUGGESTION, location unrecorded) | **deferred** | Id carries no location/claim to re-locate. Re-inspected the bridge module for the review-2a8449d06a0c1d5b frozen classes (reconcile order, unowned baseline, dest-arm coverage): no additional verifiable defect beyond the ones dispositioned above. The one real R2-class candidate — the duplicated subprocess/parse/refuse scaffolding across the four `go_*` bridge runners — is the documented temporary fail-open pattern with per-bridge pinned envelopes and contracts; consolidating it is a drive-by refactor outside advisory hardening scope. Reported to the parent. |
| R2-002 (SUGGESTION, location unrecorded) | **deferred** | Same basis as R2-001. |
| GO-08 candidate-6 copy-bridge SUGGESTIONs | **stale / already-covered** (per parent steer) | The results-count-mismatch guard in `go_apply_copy` (`len(stdout["results"]) != len(items)`) is pinned by `CopyApplyResultsCountMismatchTests.test_empty_results_for_one_item_is_an_envelope_mismatch` (tests/test_copy_apply_bridge.py:608+) — verified on the current tree; not duplicated. The candidate-6 control-char refusal class belongs to `lib/_internal/lock.py` and is already implemented there (`_has_control_char` / `_refuse_control_chars` + `FallbackControlCharRefusalTests` in tests/test_lock.py) — outside this lane's surfaces, confirmed covered, reported to the parent. |
| Python-side template-bridge advisories (C3's list, proactively re-inspected; no C3 report received yet) | **absorbed where verifiable** | R1-toctou-residual-symlink-race → not-a-defect on the Python side (same two-layer guard as R1-symlink-preflight-toctou-window above). R1-dest-containment-absent / R1-ancestor-symlink-traversal-deferred → the Go side is C3's to fix; any Go semantic change must be mirrored in `_python_materialize_template` afterwards — NOT fixed here blind to avoid diverging from C3's chosen semantics; reported to the parent for sequencing. R2-refusal-string-dual-authority / R2-warn-str-dup / R2-emit-refuse-dup → dual Go/Python message authority is the pinned byte-identical parity contract (`_symlink_refusal` is the single Python source; parity pinned by tests); not-a-defect. R2-render-fallback-asymmetry / R3-2 / R3-3 → no verifiable Python-side defect located; await C3's report. |

### RED→GREEN evidence for the behavior fix (9b622c7)

- RED: `env -u WORKTREE_GATE_BIN python3 -m unittest tests.test_template_actuator_bridge.TemplateActuatorFallbackTests.test_record_sha_mismatching_disk_falls_back_without_lock_write` → FAILED: `AssertionError: 0 != 1` on the fallback-token count — the stub's `wrote: true` envelope with a `b*64` record over user bytes was trusted and pushed into the lock via the lock-write bridge (whose own envelope mismatch then warned with the wrong token). The old dispatcher had no destination-on-disk check.
- GREEN: same test → OK. The stub now writes nothing the envelope claims; user bytes stay untouched, the lock gains no `TARGET` entry, and exactly one `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK` warning names the disk mismatch.
- Existing assertion adjusted (strengthened, not weakened): `test_envelope_contract_one_call_per_template` pinned `lock["managed"][TARGET]["sha256"] == "a"*64` with a stub that never wrote dest — exactly the unverified-record behavior the fix removes. The stub now writes the rendered body bytes and carries the truthful rendered digest (`hashlib.sha256(b"#!/bin/sh\necho hi\n")` = `299001868fb8…cbba`); the pin's intent (the Go envelope's record lands in the lock) is preserved and the `✓ template` output assertion proves the Go-record path still runs.

### Observed suites (final, both commits)

- `WORKTREE_GATE_BIN=tmp/dist-gate/worktree-gate-current python3 -m unittest tests.test_hook_gate_bridge tests.test_copy_apply_bridge tests.test_template_actuator_bridge` → `Ran 70 tests ... OK`.
- Same without `WORKTREE_GATE_BIN` → `OK (skipped=5)` (loud skips are the binary-dependent Go-authority classes; fallback contracts fully run).
- New test in isolation without binary: `Ran 0 tests ... OK (skipped=1)` (class-level loud skip, existing pattern).
- Go: sources unchanged → no rebuild, no SHA256SUMS regeneration (rule 5 N/A).

Commits (this lane, `feat/adv-bridge`): `9b622c7` fix(bridge): verify the template record against the destination on disk; `2c6f16f` test(copy-bridge): pin the Go-path missing-dest command arm; plus this evidence doc. No push, no PR.

Reported to parent (outside C1 surfaces): (1) R3-reconcile-check-then-act residual window — architectural, closes when Go owns the lock write; (2) template dest-containment/ancestor-symlink classes must be mirrored into `_python_materialize_template` after C3 lands its Go semantics; (3) opaque R2-001/R2-002 located to no further verifiable defect.


## Lane C2 evidence

Method: the recorded locations are pre-correction candidate (a236fa2) lines. Each advisory was re-located by symbol on the current tree (base 888a425 already carries the R4-001 CRITICAL rewrite of the :341-373 region: `hookBackupComplete` + `writeHookBackupSnapshot`). The native store preserves id/lens/location/severity only, so defect classes were re-verified by inspection + reproduction at the anchor; opaque ids (R3-00x/R4-00x) are dispositioned strictly by what reproduces (or not) at their recorded anchor region.

| Advisory | Lens | Anchor on current tree | Disposition | Evidence |
|---|---|---|---|---|
| R2-refresh-backup-contract | readability | pre-corr :355-370 → rewritten into `hookBackupComplete` / `writeHookBackupSnapshot` | **stale** | Region rewritten by R4-001: the backup contract is now two named helpers (completeness-vs-content-hash-key; atomic temp+Sync+rename, temp removed on every failure path) with pinning tests `TestHookGateActuatorRefreshRepairsPartialBackup` + `TestHookGateActuatorRefreshKeepsCompleteBackup` (inode identity). Nothing of the old inline block remains. |
| R3-001 | reliability | pre-corr :355-371 → same rewritten region | **stale** | The reliability class (a truncated/partial snapshot masquerading as the immutable snapshot) is exactly what the completeness check + atomic rename close; covered by the two pinning tests above. |
| R3-002 | reliability | :341-373 → IS the rewritten region | **stale** | Recorded range coincides with the R4-001 rewrite. |
| R4-002 | resilience | refresh rollback/restore block (`runHookGateRefresh`, current :414-423 region) | **fixed** (be62502) | Live: the restore used bare `os.WriteFile`, which follows symlinks — a dest swapped to a link between the entry guard and the write gets the bystander overwritten during rollback. RED (behavioral, old restore substituted in a throwaway tree): `go test -run TestHookGateActuatorRefreshRollbackNeverWritesThroughSymlink` → `bystander bytes = "#!/bin/sh\n# user customization\n", want untouched`. Fix: restore goes through `writeTemplateContent` (Lstat + `O_NOFOLLOW`, ELOOP → `errDestSymlink`), the same guarded writer as the primary path; the refresh write step is a package var (`hookGateRefreshWrite`) so the race is injectable in-process. GREEN: bystander untouched, link preserved, exit 2 + symlink refusal; suite green. Considered retaining the backup when the restore fails; rejected — the merged contract test `TestHookGateActuatorRefreshRollback` pins backup removal after rollback, and retention/removal is safety-equivalent given content-hash immutability. |
| R4-003 | resilience | :308-314 (same refresh path; resilience-critical section = the backup write) | **stale** | The anchor's resilience-critical section is precisely what R4-001 rewrote (atomic snapshot + completeness check). Post-correction the refresh flow is guarded end-to-end: entry Lstat guard, guarded primary write (`O_NOFOLLOW`), atomic snapshot, and (after be62502) guarded restore. No distinct live resilience defect reproducible at the anchor; no change. |
| R1-hook-rel-path-unsanitized | risk | :84-86 `hookScriptRelPath` | **fixed** (be62502) | Live + demonstrated with the PRE-fix binary: envelope `recipe_id: "../escape"` → rel `ai-specs/recipes/../escape/hooks/gate.sh` → wrote `proj/ai-specs/escape/hooks/gate.sh`, exit 0, and returned a lock record keyed on the traversal rel (Python would have written a gate baseline under that key). Fix: `hookRelPathEscapes` guard in `runMaterializeHook` refuses separators/dot-segments in recipe_id and dot-segment script basenames with a decision envelope before any path join. GREEN: exit 2, error envelope, zero files written (re-probed with the post-fix binary). RED→GREEN tests: `TestHookGateActuatorRefusesEscapingRelPath` (2 subtests), `TestHookRelPathEscapes`. |
| R3-003 | reliability | :84-86 (same anchor) | **fixed** (be62502) | Reliability reading of the same unsanitized rel computation — traversal-shaped inputs producing destinations outside the managed tree or onto parent directories — closed by the same guard and tests as R1. |
| R3-004 | reliability | :216-222 → `hookActuatorRecord` declaration | **not-a-defect** | Anchor region is the record payload declaration: every field is populated by the single `hookRecord` constructor; nil record is the documented "write nothing to the lock" signal; refusal paths never attach a record. No reliability hazard reproducible; no change. |
| R2-dup-emit-refuse | suggestion | `emit`/`refuse` closures in `runMaterializeHook` | **fixed** (be62502) | `refuse` re-implemented emit's marshal+print; now delegates to `emit` and overrides the exit code only. Behavior-identical (refusal envelope bytes unchanged, pinned by `TestHookGateActuatorSourceMissing` et al.). Suite green before/after. Same duplication exists in `templateactuator.go` — Lane C3 surface, REPORTED to parent. |
| R2-dup-write-branches | suggestion | `classifyMissing` / `classifyManagedStale` arms | **fixed** (3628d01) | Arms duplicated write/refuse/record/message; merged with a state-selected message verb. Exact messages unchanged (pinned by existing tests). Suite green. |
| R2-refresh-signature | suggestion | `runHookGateRefresh` (8 params) | **fixed** (be62502 + 3628d01) | Parameters flattened into the `hookRefreshCall` value (struct landed with the rollback fix, call-site cleanup in the refactor unit). Suite green. |
| R2-fixture-gatemode-context | suggestion | `hookgateactuator_test.go` `writeHookSource` config | **fixed** (3628d01) | Fixture no longer restates `gate_mode`; every end-to-end test now exercises the default arm of `hookConfigGet` (default "always" produces the same bytes `hookFixtureRendered` expects). No assertion touched; 22 hook tests green (19 baseline + 3 new). |

Commands and observed results:

- RED (compile, suite convention for new symbols): `go test -run 'TestHookRelPathEscapes|TestHookGateActuatorRefusesEscapingRelPath|TestHookGateActuatorRefreshRollbackNeverWritesThroughSymlink' .` → `undefined: hookRelPathEscapes / hookGateRefreshWrite`, FAIL.
- RED (behavioral, traversal): pre-fix binary + `recipe_id "../escape"` envelope → `{"rel":"ai-specs/recipes/../escape/hooks/gate.sh",...,"wrote":true,...}` exit 0, file at `proj/ai-specs/escape/hooks/gate.sh`.
- RED (behavioral, rollback): old `os.WriteFile` restore substituted in a throwaway tree → `bystander bytes = "#!/bin/sh\n# user customization\n", want untouched` FAIL.
- GREEN (traversal): post-fix binary + same envelope → `{"error":"hook rel path would escape ai-specs/recipes/: recipe_id \"../escape\", script \"hooks/gate.sh\"",...}` exit 2, `find` → 0 files.
- GREEN (full): `go test -count=1 ./...` in `catalog/recipes/worktree-flow/gate` → ok/ok (ledger included); hook tests 22/22 PASS (19 baseline + 3 new, no existing assertion modified).
- Assets: `scripts/build-gate.sh` (go1.24.13, no toolchain warning) → `cd dist && shasum -a 256 worktree-gate-*` → regenerated `bin/SHA256SUMS` → `scripts/verify-gate-sums.sh <generated> <committed>` → `ok — 4 digest entries match the committed trust root`; `gofmt -l .` empty; `go vet ./...` clean.

Reported outside surfaces: `emit`/`refuse` duplication also exists in `templateactuator.go` (`runMaterializeTemplate`) — Lane C3.

Commits: `be62502` fix(hook-gate) — behavior unit; `3628d01` refactor(hook-gate) — readability unit + sums. Suite verified green at each commit (be62502 checked out in a detached worktree and tested).


## Lane C3 evidence

Worker: Lane C3 (template) · branch `feat/adv-template` · base `development` `888a425` · commits `0bb9d61` (guards), `211b2bd` (dedup + test pins). Surfaces touched: `catalog/recipes/worktree-flow/gate/templateactuator.go`, `.../templateactuator_test.go`, `.../bin/SHA256SUMS`, this section.

### Dispositions

| Advisory id | Disposition | Reason / evidence |
|---|---|---|
| R1-dest-containment-absent | **fixed** (`0bb9d61`) | Re-located: literal join `filepath.Join(projectRoot, target)` in `resolveTemplateDest` + write in `runMaterializeTemplate`; no containment existed (verified on current tree). Fix: `templatePathContained` lexical check on literal (non-`.git/`) targets → exit-2 refusal `templateEscapingTargetRefusal`. Git-resolved dests are deliberately not contained (linked-worktree shared hooks dir legitimately lives in the main repo). |
| R1-ancestor-symlink-traversal-deferred | **fixed** (`0bb9d61`) | Re-located: `writeTemplateContent` did `MkdirAll`+`OpenFile` following directory symlinks; a planted symlinked ancestor below the project root redirected the write outside. Fix: `firstSymlinkedAncestor` walk (root→dest, `Lstat` per component) via `ensureTemplateAncestorsReal`, run once per invocation before both write paths → refusal `templateAncestorSymlinkRefusal` (`errAncestorSymlink`). Residual documented in-code: walk→open window narrowed, not eliminated (full fix needs an openat chain). Ancestors at/above root and git-resolved dests outside root are out of scope (resolved project_root; git-trusted emission). |
| R1-toctou-residual-symlink-race | **fixed** (`0bb9d61`) | Re-located: post-write `os.Chmod(dest, mode)` traversed the path — a link swapped in after the write would be chmod'ed through. Fix: chmod moved onto the open handle (`file.Chmod(mode)`), the exact inode just written. The Lstat→open window is already covered by `O_NOFOLLOW`→`ELOOP`→`errDestSymlink` (earlier batch). RED is not deterministically expressible for a race; before/after code evidence: `grep -n "os.Chmod" templateactuator.go` 1 hit → 0 hits, `file.Chmod` present; full suite green. |
| R2-emit-refuse-dup | **fixed** (`211b2bd`) | `emit`/`refuse` closures duplicated marshal+diagnose+print; unified into one `emitEnvelope(out, exit)` helper. Behavior-identical (same envelope bytes, same exit taxonomy incl. marshal-failure → 2); full suite green before/after. |
| R2-render-fallback-asymmetry | **not-a-defect** (Go side) | Compared `renderTemplateBytes` vs Python `render_template_bytes` + `util.render_override_bytes` semantics line by line: topology token default `auto` (nil config or missing key), cleanup stamps only when config non-nil with `str(get(key) or default)` falsiness, nil config leaves cleanup tokens literal, CRLF untouched by rendering. Equivalent. New-guard refusal strings being Go-authority-only is deliberate fail-closed asymmetry — routed to C1 below. |
| R2-warn-str-dup | **deferred → C1** | The warn strings are Go/Python dual-authority text; single-sourcing lives in the Python bridge (`recipe-materialize.py`), not this lane's surface. Go copies are parity-pinned verbatim by tests (must not be "deduplicated" into divergence). |
| R2-refusal-string-dual-authority | **deferred → C1** | Verified today `templateSymlinkRefusal` (Go) ≡ `_symlink_refusal` (Python) verbatim, char for char. Keeping them single-sourced is a Python-side maintenance fix. |
| R3-2 | **deferred → C1** | Location unrecorded for the template lineage (native store keeps id/lens/severity only); no verifiable Go-side claim found. In Lane C4's inventory the same classes landed in Python bridge surfaces. |
| R3-3 | **deferred → C1** | Same as R3-2. |
| Unrecorded bridge-tests SUGGESTIONs (review-faee79d90d3f3863, Go surface) | **fixed** (`211b2bd`) | Re-inspection found three untested arms; pinned additively: directory-as-source refusal (`IsRegular` arm), `pyConfigString` str() parity over bools/None/number literals, `templateConfigOr` numeric-zero falsiness fallback. No existing assertion weakened. |

### RED→GREEN evidence (observed)

- RED (guards): new tests added first → `go test -run 'TestTemplateActuatorEscapingTargetRefused\|TestTemplateActuatorSymlinkedAncestorRefused\|TestWriteTemplateContentRefusesSymlinkedAncestor'` → build failed with `undefined: templateEscapingTargetRefusal`, `undefined: templateAncestorSymlinkRefusal`, `undefined: errAncestorSymlink` (symbols are the contract; repo's established RED convention).
- GREEN (guards): after `0bb9d61` — the three tests PASS; in-root `..` twin (`ai-specs/../stays.sh`) still materializes (containment refuses only escapes); real-directory ancestor chain still writes.
- GREEN (dedup/pins): after `211b2bd` — `go test -count=1 ./...` in `catalog/recipes/worktree-flow/gate` → `ok ai-specs.dev/worktree-gate 52.8s`, `ok ai-specs.dev/worktree-gate/ledger 2.9s` (run 3× during the lane, all green).
- Go asset gates: `scripts/build-gate.sh` (canonical `go1.24.13`, no toolchain warning) per work unit; `SHA256SUMS` regenerated with the documented command (`scripts/build-gate.sh` + `cd dist && shasum -a 256 worktree-gate-*`); `scripts/verify-gate-sums.sh <generated> <committed>` → `ok — 4 digest entries match` per commit; `gofmt -l .` empty; `go vet .` clean. Committed digests: `8343f455…` (darwin-amd64), `cdb834ca…` (darwin-arm64), `26bcf44e…` (linux-amd64), `ce4479da…` (linux-arm64).
- `./tests/validate.sh` NOT run (parent-owned).

### Routed to Lane C1 (Python bridge surfaces)

1. Mirror the two new Go-authority refusals in the Python fallback body if parity is wanted: escaping-target and symlinked-ancestor guards do not exist in `_python_materialize_template` / `write_content` (the bridge fails closed on Go's exit-2 refusal, so the asymmetry direction is safe).
2. Python fallback residual of R1-toctou: `write_content` in `_python_materialize_template` ends with `os.chmod(dest, src.stat().st_mode)` — path-based chmod, same residual race fixed on the Go side.
3. R2-warn-str-dup: single-source the warn strings (Go copies are parity-pinned; do not diverge).
4. R2-refusal-string-dual-authority: single-source `_symlink_refusal`/`templateSymlinkRefusal` (currently verified identical).
5. R3-2 / R3-3 (template lineage, locations unrecorded): per the C4 precedent these classes live in the Python bridge/test surfaces.


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
