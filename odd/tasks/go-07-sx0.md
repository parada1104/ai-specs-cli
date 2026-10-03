# [Go 07.SX0] Shared gate package extraction — feature document

Card: `6ac06c7a0d9b0e444869419b` — https://trello.com/c/aIusguGP (In Progress)
Parent card: `6a84e7acca1baf394d9b482b` — https://trello.com/c/ITeLc5xL (Done; planning card, not reopened)
Epic card: [Go single-binary migration](https://trello.com/c/qwlHQ7Xa)
Branch: `change/go-07-sx0` · Worktree: `.worktrees/go-07-sx0` · Base: epic tip `0db6c2f`
Worktree verified 2026-10-03: `git rev-parse --show-toplevel` → `.worktrees/go-07-sx0`, branch `change/go-07-sx0`, tree clean.

## Tracker

- card_id: `6ac06c7a0d9b0e444869419b`
- url: https://trello.com/c/aIusguGP
- list: In Progress
- parent: `6a84e7acca1baf394d9b482b` (https://trello.com/c/ITeLc5xL)

## Scope (parent brief, binding)

Q2 decision (human, supersedes plan-doc Q2(b)): **one authoritative importable Go
package inside `ai-specs.dev/worktree-gate`, sibling of `ledger`; in-process
invocation from the root binary; `gate_binary.py` NOT ported**; gate keeps a thin
`main` with existing flags, dispatch and byte behavior. Placement resolved by the
parent: root imports via `require ai-specs.dev/worktree-gate v0.0.0` +
`replace => ./catalog/recipes/worktree-flow/gate`.

Constraints:

- Extract only decision/actuator logic required by S10–S13. No unrelated gate domains.
- Gate stays standalone: zero external dependencies, go1.22 floor.
- No speculative wrappers/interfaces; check callers; preserve contracts.
- Python remains oracle through S16. Gate-present parity already implemented; not repaired here.
- Checksum regeneration is local, not release: `scripts/build-gate.sh` with canonical
  Go1.24.13, regenerate committed `SHA256SUMS`, `scripts/verify-gate-sums.sh`. No tag/publish.
- Delivery (commit, push, PR, merge) belongs to the parent. This worker produces the
  reviewed change and stops; the human-authorized delivery boundary overrides the
  default per-task work-unit commit rule of this harness.
- Allowed edit surfaces: `odd/tasks/go-07-sx0.md`, `odd/tasks/go-07-sync-plan.md`
  (Q2 + SX0 dependency reconciliation only), `catalog/recipes/worktree-flow/gate/*.go`
  (S10–S13 extraction + required dispatch callers/tests),
  `catalog/recipes/worktree-flow/gate/shared/**`, `go.mod`,
  `catalog/recipes/worktree-flow/bin/SHA256SUMS`.
- Surface extension (parent authorization, 2026-10-03):
  `internal/sync/gateshared_import_test.go` — one small shared-package
  import/contract test. No runtime wiring, no S15 changes, no other root files.
- Forbidden: `gate_binary.py`, `recipe-materialize.py`, parity corpus changes,
  release workflows, generated dogfood harness files.

## Staged extraction (human decision 2026-10-03)

SX0 is split into reviewable candidates. Each unit gets focused checks, the
required full checks, the local checksum cycle, and an approved review/verifier
before parent delivery. Each downstream slice may start once its required
shared API is integrated — not after all of SX0.

| Candidate | Units | Shared API delivered | Unblocks |
| --- | --- | --- | --- |
| **SX0a (current)** | U1 orphans | RunPlanOrphans / RunApplyOrphans + plan types | S10 (orphans plan/apply seam) |
| SX0b (later) | U2 lock write | RunWriteLock | S10 (lock prune) |
| SX0c (later) | U6 copy-apply | RunApplyCopy | S12 (copy-apply) |
| SX0d+ (later) | U3, U4, U5, U7, U8, U9 | conflicts/stamps/classify, template, hook, bindings, merge-config, resolved-config | S11, S12, S13 |

Helper moves for `git()` / `RealPath` / `orderedMap` are NOT approved in SX0a.
U2 and U6 are separate later candidates; they are not implemented until the
human authorizes them.

## Goal

Make the gate's decision/actuator authority importable in-process from the root
binary without changing the gate binary's observable behavior, so S10–S13 can
implement their Go sides against one authoritative implementation instead of
shelling out to the gate executable.

## Tasks

- [x] T1 — Read-only scope map: inventory gate main-package files; identify the
      S10–S13 decision/actuator domains (orphans plan/apply, lock write, tag +
      primitive conflicts, reconcile stamps, managed-override classify, template
      actuator, hook gate, copy-apply, bindings, merge-config, resolved-config);
      record per-domain files, call sites in main dispatch, contracts to preserve.
- [x] T2 — Reconcile plan doc `odd/tasks/go-07-sync-plan.md`: Q2(b) → approved
      shared-package decision; SX0 dependency note for S10–S13 (docs-only edit).
- [ ] T3 — SX0a (current candidate): move U1 orphans (orphans_plan.go,
      orphans_apply.go) into `gate/shared`; thin main dispatch references;
      root require/replace; small import test. Later candidates SX0b (U2),
      SX0c (U6), SX0d+ (remaining units) stay untouched until authorized.
      UNBLOCKED: Option 1 approved for sortedUnique (see SX0a finding).
- [ ] T4 — Wire root `go.mod`: `require ai-specs.dev/worktree-gate v0.0.0` +
      `replace` to the nested module; keep go1.22 floor; no external deps.
- [x] T5 — Tests: in-process import evidence (root test importing the shared
      package at `internal/sync/gateshared_import_test.go` — parent-extended
      surface, small import/contract test only), gate focused tests green, root
      tests green. Test-first where the change is behavioral; mechanical moves
      get structural + differential evidence with the reason recorded.
- [ ] T6 — Checksums: `scripts/build-gate.sh` (Go1.24.13), regenerate committed
      `catalog/recipes/worktree-flow/bin/SHA256SUMS`, `scripts/verify-gate-sums.sh`.
- [x] T7 — Full checks: focused gate tests, root tests, `./tests/validate.sh`,
      both parity modes (gate-absent + gate-present). CONCLUSIVE on attempt 3
      (verifier-supervised detached run): `VALIDATE_EXIT:0` captured;
      parity PASS both modes; unittest `Ran 2482 tests` → OK. Details in
      Evidence.
- [ ] T8 — RDD native review to approved verdict (or approved independent
      verifier per effective policy); consent lifecycle respected; no fabricated
      approval. **BLOCKED at first START (2026-10-03): verified 18-path
      candidate (base_tree == HEAD tree = 0db6c2f epic tip, no drift, exact
      SX0a surface); START refused consent-binding-stale for a binding bound
      to the old 13-path selectorless target — the 18-path target was never
      invoked, no lineage created. Store inventory: 11 entries (8 escalated,
      2 approved, 1 correction_required). CORRECTED (parent): no S10–S13
      workers have been launched; the entries CANNOT be attributed to them,
      and inventory existence alone does not prove the cause of the stale
      binding — cause unproven. Reported to parent; option A selected
      (committed-range review after parent commit-normalization). HOLD:
      no source/doc mutations and no native START until the parent supplies
      the commit SHA; all current changes preserved.**
- [ ] T9 — Report to parent: task count, tracker identity, owned files, check
      evidence, review result, downstream import API.

## Risks / traps

- R10: any gate-module change normally demands SHA256SUMS regen; brief rules this
  is local regeneration, not release. Execute exactly the documented procedure.
- Byte behavior of thin `main` must not drift: same flags, same dispatch, same
  output. Differential check via existing gate tests + parity both modes.
- Root import must not create an import cycle or pull gate `main` symbols.
- Zero new dependencies: shared package imports stdlib + ledger only.

## SX0a finding (U1 not self-contained as mapped)

The explorer's map marked U1 stdlib-only, which holds for imports but not for
same-package symbols: `absentFrom` (orphans_plan.go:65) calls `sortedUnique`,
defined in `bindings.go:273` (U7, out of scope). Callers of `sortedUnique`:
bindings.go:145,163 (U7), tagconflicts.go:103 (U3), primitiveconflicts.go:93
(U3), resolved_config.go:209 (U9), resolved_config_test.go:224 (U9 test).
Reported to the human before any edit, per instruction.

**RESOLVED (human approval, Option 1): `sortedUnique` moves into shared as
`SortedUnique`, preserving exact behavior. One authority; no temporary
duplicate.** SX0a surface extended ONLY for the necessary imports and the six
reference updates: bindings.go (2 call sites + definition removal + `sort`
import), tagconflicts.go (1), primitiveconflicts.go (1), resolved_config.go
(1), resolved_config_test.go (1). Those domains are NOT extracted and their
behavior must not change. `git()` / `RealPath` / `orderedMap` remain excluded
from SX0a.

Test handling in both options: both orphans test files STAY in package main
(dispatch-contract tests need main's `run()`); identifiers rename to the shared
exported API. Zero test lines move; all 490 lines keep their coverage.

## Scope map (T1, explorer-verified, path:line evidence in session report)

Gate root package: 32 non-test files; `ledger/` already importable (7 files);
go.mod `ai-specs.dev/worktree-gate`, go 1.22.

Extraction units (production lines measured by `wc -l`):

| Unit | Files | Lines | Entanglement |
| --- | --- | --- | --- |
| U1 orphans | orphans_plan.go (93), orphans_apply.go (237) | 330 | self-contained, stdlib only |
| U2 lock write | lockwrite.go (391) | 391 | self-contained |
| U3 conflicts+classify+stamps | classify.go (159), tagconflicts.go (133), primitiveconflicts.go (126), reconcilestamps.go (115), recipe_toml.go (412) | 945 | classify is the shared core of U4/U5; recipe_toml loaders feed U3+U7 |
| U4 template actuator | templateactuator.go (566) | 566 | needs classify + writeTemplateContent + readRegularFile + sha256Bytes |
| U5 hook actuator | hookgateactuator.go (479) | 479 | needs classify + writeTemplateContent; carries test seam var `hookGateRefreshWrite` |
| U6 copy-apply | copyapply.go (302) | 302 | self-contained |
| U7 bindings | bindings.go (286), witness_write.go (189) | 475 | witness imports `ledger` (transitive dep OK) |
| U8 merge-config | mergeconfig.go (572) | 572 | `orderedMap` also used by out-of-scope recipeconfigwrite.go — moves to shared, recipeconfigwrite.go gets a required one-line reference change |
| U9 resolved-config | resolved_config.go (379) | 379 | needs `git()` (gitfacts.go:29) + `RealPath` (pathutil.go:15); both helpers also serve main-only files (cleanup, topology, decide, main, ledger_cmd) → move to shared + update main call sites |
| Total | | **4,469** prod lines + helpers moved (gitfacts.go 106, pathutil.go 81) | |

Test files moving with their domains: 14 `*_test.go` files, **5,818 lines**.
Raw diff estimate: **~10.5k lines, overwhelmingly mechanical** (package decl,
export renames of run/options/option-structs, cross-references); git rename
detection should show most as moves. main.go keeps flags/dispatch and byte
behavior; handlers become thin `shared.RunX(...)` calls; single `os.Exit` site
(main.go:35) untouched; no init() in moving files (extract.go:26 stays in main).

## Evidence

- T2 (2026-10-03): `odd/tasks/go-07-sync-plan.md` §(g) Q2 — recommendation
  replaced with the resolved human decision (option (a) pulled forward as SX0,
  card 6ac06c7a0d9b0e444869419b, placement require/replace recorded); §(b)
  item 8 — package-main blocker annotated with SX0 as the resolution. Two edit
  blocks total; no other lines touched in that file.
- T3/T5/T6 (2026-10-03, pre-crash, real evidence): shared package created
  (`shared/{sorted,orphans_plan,orphans_apply}.go`); main dispatch calls
  `shared.RunPlanOrphans`/`shared.RunApplyOrphans`; six reference updates
  applied (bindings ×2 + definition removal + `sort` import dropped,
  tagconflicts, primitiveconflicts, resolved_config, resolved_config_test);
  both orphans test files stay in package main against the shared API. Gate:
  gofmt clean, `go vet ./...` OK, `go test ./...` ok (main 51.068s, ledger
  3.322s). Root: gofmt clean, `go build ./...` OK, `go test ./...` all packages
  ok including `internal/sync` 12.945s with the 4/4 PASS of
  `gateshared_import_test.go` (RED first observed: import unresolved before
  go.mod wiring). Checksum cycle: `build-gate.sh` with canonical go1.24.13 (no
  toolchain warning), 4 digests regenerated, committed SHA256SUMS updated with
  an SX0a regeneration note, `verify-gate-sums.sh` ok (4 entries match), smoke:
  `worktree-gate-current --version` 0.24.0 and `--plan-orphans` emits the
  contract JSON.
- T7 (2026-10-03, INTERRUPTED ×2): attempt 1 (`/tmp/sx0a-validate.log`) was
  killed by the Herdr crash: parity phase PASSED both modes, phase 5 killed at
  ~44 tests, no exit code. Attempt 2 was delegated to the verifier
  (`/tmp/sx0a-validate-rerun.log`): the 30-minute subagent stall window hit
  while the suite ran legitimately; the orphaned unittest process (pid 7249)
  kept phase 5 alive and its parity phase again PASSED (`parity summary:
  gate-absent failing=0, gate-present failing=0 — PASS`, 38 zero-delta
  fixture-mode results), then pid 7249 vanished at 01:31:59 WITHOUT writing
  `Ran N tests` / `OK` — killed externally, no summary, no exit code. Both
  attempts recorded as interrupted/unknown. Resume pattern: verifier launches
  the suite detached with an exit-capture marker in the log and supervises by
  short polling calls (never a single long bash), so neither the stall window
  nor a wrapper kill can lose the evidence again.
- T7 (2026-10-03, CONCLUSIVE attempt 3): verifier-supervised detached run with
  exit marker (`/tmp/sx0a-validate-final.log`, 573 lines): `VALIDATE_EXIT:0`;
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS` (19
  fixtures per mode, all zero deltas); `Ran 2482 tests in 2015.887s` → `OK`,
  zero unittest FAIL/ERROR lines, 36m12s wall. Remaining `!` lines are
  expected fixture output (bridge fail-open warnings; one digest-mismatch
  rejection fixture asserting the trust-root guard). The verification changed
  no files and made no commits.
