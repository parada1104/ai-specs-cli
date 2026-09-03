# Apply progress: gate-cwd-fidelity

Merged from first apply batch (no prior `apply-progress` artifact).

**Delivery:** single PR + `size:exception` (explicit). No chained PRs.
**Structured status consumed:** `applyState: ready`, `artifactStore: openspec`, `actionContext.mode: repo-local`, `allowedEditRoots` = worktree root. Authoritative.

## Completed tasks (persisted checkboxes)

All 24 implementation-owned rows in `openspec/changes/gate-cwd-fidelity/tasks.md` are `- [x]`.

Parent-owned (deferred, unchanged):

- [ ] Start or reuse bounded review. <!-- sdd-owner: parent -->

## Files changed

New:

- `catalog/recipes/worktree-flow/gate/cwd.go`
- `catalog/recipes/worktree-flow/gate/cwd_test.go`

Modified:

- `catalog/recipes/worktree-flow/gate/event.go` — `WriteCandidate`, `CwdTrusted`, ParseEvent walk
- `catalog/recipes/worktree-flow/gate/event_cwd_test.go`
- `catalog/recipes/worktree-flow/gate/main.go` — `effectiveBase`, degrade, explain fields
- `catalog/recipes/worktree-flow/gate/main_test.go` — §7 `run()` matrix
- `catalog/recipes/worktree-flow/gate/message.go` — extra args + `DegradeMessage`
- `catalog/recipes/worktree-flow/gate/decide_test.go` — verbatim message tests
- `catalog/recipes/worktree-flow/bin/SHA256SUMS` — canonical go1.24.13 digests for the new binary
- `openspec/changes/gate-cwd-fidelity/specs/worktree-flow/spec.md` — MODIFIED *Shell Command Write-Bypass Detection*
- `openspec/changes/gate-cwd-fidelity/tasks.md` — implementation checkboxes

Frozen (confirmed unmodified): `lib/_internal/hooks-render.py`, `worktree-gate.sh`, `doctor.py`, `decide.go`, `tokenize.go`, `config.go`, `uri.go`, `extract.go`.

`.codegraph/` is local index only — not for commit.

## Test commands run

| Command | Result |
|---------|--------|
| Baseline `go test ./...` in gate dir (1.3) | pass (`ok ai-specs.dev/worktree-gate`, ~10.8s) |
| RED `go test -run TestSplitSegmentsWithSep` | build fail: undefined `segment` / `splitSegmentsWithSep` |
| GREEN same | pass |
| RED WriteCandidate / pass2-once | build fail: no `CwdTrusted` / `WriteCandidate` |
| GREEN same + extract_pass2 | pass |
| RED Block/Ask/Degrade signatures | build fail: too many arguments / undefined `DegradeMessage` |
| GREEN message tests | pass |
| RED `effectiveBase` + `run()` matrix | build fail: undefined `effectiveBase` / explain fields |
| GREEN `TestRunCwdFidelityMatrix` + explain | pass |
| `go test ./...` gate (5.2, after gofmt) | pass (`ok`, ~10.3s) |
| `WorkspaceContextProcessBoundaryTests` (incl. `_assert_process_cwd_event`) | 16/16 ok |
| `python3 -m py_compile` + `bash -n` (validate.sh prefix) | pass |
| `gofmt -l catalog/recipes/worktree-flow/gate` | clean after `gofmt -w` |
| `scripts/build-gate.sh` + SHA256SUMS regen (go1.24.13) | 4 targets |
| Digest/doctor tests after sums update | 12/12 ok |
| `./tests/run.sh` (full, before sums update) | 1790 tests; gate digest failures (fixed by SHA256SUMS); **1 pre-existing error** `test_legacy_ceremony_mapping_table` missing `openspec/changes/retire-decision-matrix/...` (out of scope) |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | spec.md | Spec | N/A | ➖ hygiene | ➖ sentence replace | ➖ scenarios kept | ➖ |
| 1.2 | n/a | Index | N/A | ➖ | `codegraph init` + `index` | grep callers | never commit `.codegraph/` |
| 1.3 | gate package | Unit | ✅ baseline pass | ➖ | ➖ | ➖ | ➖ |
| 2.1 | `cwd_test.go` | Unit | N/A (new) | ✅ compile fail | ✅ | ✅ `&&` vs `\|` vs empty | ➖ |
| 2.2 | `cwd.go` | Unit | ➖ | ✅ 2.1 | ✅ | ➖ | wrapper `splitSegments` |
| 2.3 | `cwd_test.go` | Unit | ✅ | ✅ closed-list cases | ✅ | quoted-space dir | ➖ |
| 2.4 | `cwd_test.go` | Unit | ✅ | ✅ walk table | ✅ | relative `-C` vs A | ➖ |
| 2.5 | `cwd.go` | Unit | ✅ | ✅ 2.3/2.4 | ✅ | ➖ | ➖ |
| 2.6 | `cwd_test.go` | Unit | ✅ | ✅ quoted-space + cd arity | ✅ `go test ./...` | ✅ | ➖ none needed |
| 3.1 | `event_cwd_test.go` | Unit | ✅ | ✅ missing types | ✅ | PATH/SHELL/missing cwd | ➖ |
| 3.2 | `event_cwd_test.go` | Unit | ✅ pass2 green | ✅ | ✅ once + final S | ➖ | ➖ |
| 3.3 | `event.go` | Unit | ✅ | ✅ 3.1/3.2 | ✅ | ➖ | ➖ |
| 3.4 | `event.go` | Unit | ✅ | ➖ | ✅ suite green | ➖ | keep `splitSegments` wrapper |
| 4.1 | `decide_test.go` | Unit | ✅ | ✅ arity fail | ✅ | createWorktree false | ➖ |
| 4.2 | `message.go` | Unit | ✅ | ✅ 4.1 | ✅ | always vs ask degrade | ➖ |
| 4.3 | `main_test.go` | Unit | ✅ | ✅ undefined helper | ✅ | §7 matrix | ➖ |
| 4.4 | `main.go` | Unit | ✅ | ✅ 4.3 | ✅ | ➖ | `shouldCreateWorktree` helper |
| 4.5 | `main_test.go` | Unit | ✅ | ✅ missing JSON fields | ✅ | git -C explain allow | ➖ |
| 4.6 | `main_test.go` + `decide_test.go` | Unit | ✅ | ➖ | ✅ extra-primary fixture | cheaper call-site helper (no `Decision.RepoRoot`) | ✅ |
| 5.1–5.5 | matrix + suite | Unit | ✅ | per-phase above | ✅ gate suite | ➖ | SHA256SUMS |

### Test Summary

- **Total tests written/extended:** cwd grammar tables, ParseEvent source tests, message verbatim/degrade, `effectiveBase`, §7 `run()` matrix, explain `cwd_source`
- **Gate package:** all passing
- **Layers used:** Unit (Go), process-boundary Python (hooks-render, frozen)
- **Approval tests:** existing `TestParseEventCwdNormalization` still asserts fallback `Cwd` storage
- **Pure functions created:** `splitSegmentsWithSep`, `staticDirOperand`, `resolveDir`, `recoverCwdWalk`, `effectiveBase`, `shouldCreateWorktree`, `DegradeMessage`

## Deviations from design

- **`Decision.RepoRoot`:** not added. `shouldCreateWorktree(commandCwd, event.Cwd)` at the call site (`RealPath` equality). Cheaper option from design §12.
- **`SHA256SUMS`:** regenerated with go1.24.13 so doctor/release digest tests accept the new binary. Not in the original file table; required for 5.3 digest/doctor checks. Launcher/`doctor.py` source untouched.
- **`extractPass2` once:** proven via `cd A && python3 -c 'Path("rel").write_text("x")'` attaching final sequential `S`, not per-segment (dedupe would hide double extract).

## Remaining tasks

Implementation: none unchecked.

Parent lifecycle:

```
- [ ] Start or reuse bounded review. <!-- sdd-owner: parent -->
```

## Workload / PR boundary

Single PR + `size:exception`. No chain.

## Status / actionContext

Consumed native JSON: `nextRecommended: sdd-apply`, `applyState: ready`, warnings `[]`. After this batch: implementation complete → `parent-lifecycle` / verify.
