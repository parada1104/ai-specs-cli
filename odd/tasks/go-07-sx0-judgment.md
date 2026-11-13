# [Go 07.SX0] Judgment ledger — commit 8e6c8af (frozen)

Parent instruction 2026-10-03: persist the judgment outcome for the immutable
SX0a commit. This document is the worker-recorded ledger. It changes nothing in
code, go.mod or checksum behavior.

## Subject

- Commit: `8e6c8af971cef88fbb0adecdfc7a87650bd01c54` — `refactor(gate): extract shared orphan authority`
- Parent-committed exact SX0a surface: 18 path endpoints, 499 insertions /
  135 deletions with rename detection (16 stat entries; 2 renames:
  `orphans_plan.go → shared/orphans_plan.go`, `orphans_apply.go → shared/orphans_apply.go`).
- Base boundary: `0db6c2f13564cf5049f1e5b86cca36fe3ad335b9` (epic tip, S9 merge).

## Verdict

**JUDGMENT APPROVED.** Round 1. No fixes. No re-judgment.

| Count | Value |
| --- | --- |
| Confirmed severe | 0 |
| Suspect severe | 0 |
| Contradictions (severe) | 0 |
| Informational | 3 |

## Participants (parent-side, on the immutable commit)

- Functional verifier `musfz00w-2-2tvq` — PASS.
- Blind JD judge A `musfzig1-4-t7ln`.
- Blind JD judge B `musfziam-3-jkjl`.

## Informational rows and disposition

1. **Outdated T3/T4/T6 checkboxes** (both judges). The boxes were unchecked
   while the work was factually complete with evidence on record. Resolved by
   this reconciliation: `odd/tasks/go-07-sx0.md` marks T3/T4/T6 `[x]` and adds
   the judgment cross-reference. Content unchanged otherwise.
2. **go.mod comment prematurely says the runtime invokes shared** (judge A).
   The comment describes the end-state import before S10 wires the call sites.
   NOT fixed by worker decision: parent ruled no go.mod or checksum-behavior
   change for suggestions. Tracked here for a later parent-owned commit.
3. **Trust-root published-assets mismatch warning** (judge B). The committed
   `SHA256SUMS` describes published release assets; the release sequencing
   check is the parent's separate workstream. NOT fixed; no release is
   authorized. Tracked here.

## Verifier evidence record

- Focused checks (worker-recorded, pre-commit, unchanged tree content):
  gate `gofmt -l` clean, `go vet ./...` clean, `go build ./...` ok,
  `go test ./...` ok (main 51.068s, ledger 3.322s); root `go test ./...` all
  packages ok; `internal/sync/gateshared_import_test.go` 4/4 PASS with RED
  observed before the go.mod wiring.
- Full validation: `/tmp/sx0a-validate-final.log`
  sha256 `c007717ace86f65221818dfd396168f8f4069a4e63e18fcdf93b4535be5a9a4a`;
  `VALIDATE_EXIT:0`; `parity summary: gate-absent failing=0, gate-present
  failing=0 — PASS` (19 fixtures per mode, zero deltas); `Ran 2482 tests in
  2015.887s` → OK; wall 36m12s.
- Checksum cycle: `scripts/build-gate.sh` with canonical go1.24.13 (no
  toolchain warning); `scripts/verify-gate-sums.sh` ok (4 entries). Committed
  asset digests: darwin-amd64 `107c466c…1330c`, darwin-arm64
  `83109bc2…8764`, linux-amd64 `23eca62f…0607`, linux-arm64
  `3668aa8c…cf8d7` (full 64-hex values in
  `catalog/recipes/worktree-flow/bin/SHA256SUMS`).
- **Limitation**: the full validation log predates the commit and carries no
  tree hash. Content equivalence rests on the parent verifier's corroboration
  of the worktree state at commit time (clean tree, exact surface).

## RDD record

- Native review: **UNAVAILABLE** for this candidate. Two failed STARTs;
  `native_invocation_attempted:false`, `lineage_created:false` on every
  attempt; the second envelope exposed sticky wide-transaction drift
  (bootstrap base `e773514`, ~250 paths) ignoring the explicit committed-range
  input. Unchanged retries stopped by parent instruction.
- **Zero lineages created by this session.** No store reset; no abandons; the
  11 pre-existing store entries are unrelated (attribution unproven).
- ASSESS (explicit `nativeReviewOutcome:"unavailable"`, outcome_source
  explicit): risk medium (executable_change: SHA256SUMS), 18 paths / 1178
  lines, candidate `consumed:false`, reviewDue `slice_budget_reached`,
  writerProfile large (runtime). Returned plan: writerSelfVerification true,
  independentVerifier false.
- **No native approval exists. None is inferred.** The approval recorded here
  is the parent's JUDGMENT APPROVED over one functional verifier and two blind
  JD judges.

## Outstanding delivery tasks (parent-owned; not worker checkboxes)

- Commit of the held documentation delta (`odd/tasks/go-07-sx0.md` T8/T9/Judgment
  records + this ledger).
- Push / PR / merge decisions on the epic integration branch.
- Release sequencing check (INFO-3). **No release authorized.**

## Change control

No code edits. go.mod and checksum behavior unchanged for the informational
suggestions. This ledger plus the reconciled task document are the only worker
outputs of this instruction.
