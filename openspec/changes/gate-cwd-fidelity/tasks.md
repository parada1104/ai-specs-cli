# Tasks: gate-cwd-fidelity

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1000–1300 (cwd.go ~250, cwd_test.go ~450, event.go ~80, event_cwd_test.go ~120, main.go ~70, message.go ~60, decide_test.go ~80, spec delta ~35) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 spec-hygiene delta → PR 2 cwd grammar (`cwd.go` + `cwd_test.go`) → PR 3 event model (`event.go` + `event_cwd_test.go`) → PR 4 degrade routing + messages (`main.go`, `message.go`, `decide_test.go`) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High
```

All paths relative to the worktree root `.worktrees/gate-cwd-fidelity` (branch `change/gate-cwd-fidelity`).

**Frozen — no task may touch:**
- Pi/OMP event cwd semantics (`openspec/specs/workspace-context/spec.md`; `tests/test_hooks_render.py::_assert_process_cwd_event`; `lib/_internal/hooks-render.py`).
- Launcher `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` (and materialized copy) — one shared Go binary covers both hook surfaces.
- `doctor.py::_check_worktree_gate` / `stamped_gate_version`.
- `catalog/recipes/worktree-flow/gate/decide.go` policy, `tokenize.go`, `config.go`, `uri.go`, `extract.go` rules.

**Verification commands (used throughout):**
- `go test ./...` in `catalog/recipes/worktree-flow/gate/`
- `./tests/run.sh` and `./tests/validate.sh` (repo root)

---

## Phase 1 — Infrastructure (PR 1: spec hygiene + groundwork)

- [x] **1.1 Spec delta: add second MODIFIED of *Shell Command Write-Bypass Detection*** to `openspec/changes/gate-cwd-fidelity/specs/worktree-flow/spec.md` (design §9.3 / ADR-6). Keep every existing scenario (redirect, tee, sed, python heredoc, fail-open x2, outside-repo, linked worktree, non-write, read-only heredoc). Replace ONLY the sentence "Each confident candidate path MUST be resolved against the event `cwd` (when relative) and routed through…" with: relative candidates resolve against the recovered command cwd when the command determines one (`git -C <dir>` / `cd <dir> && ...`); otherwise against a usable event cwd; otherwise degrade per *Honest degrade when effective cwd is unrecoverable*; the hook process `$PWD` MUST NOT be used as a blocking fallback. Do NOT rewrite heuristics, URI rules, or hook registration. (Spec hygiene only — no Go.) <!-- sdd-owner: implementation -->
- [x] **1.2 CodeGraph index for this worktree** (skill `codegraph-worktree`): confirm `git rev-parse --show-toplevel` is the `.worktrees/gate-cwd-fidelity` root, run `codegraph init` there before any codegraph explore/query on gate symbols; after `cwd.go` symbols land, re-init; corroborate callers of `ParseEvent`, `Decide`, `BlockMessage`, `splitSegments` with `grep` (index staleness rule). Never commit `.codegraph/`. <!-- sdd-owner: implementation -->
- [x] **1.3 Baseline evidence:** run `go test ./...` in `catalog/recipes/worktree-flow/gate/` and record the pre-change pass/fail baseline (RED evidence starts here). <!-- sdd-owner: implementation -->

## Phase 2 — PR 2: cwd recovery grammar (RED → GREEN → TRIANGULATE → REFACTOR)

- [x] **2.1 RED:** create `catalog/recipes/worktree-flow/gate/cwd_test.go` with table tests for `splitSegmentsWithSep`: separators (`|`, `||`, `&&`, `;`) preserved as `segment{tokens, sep}` so `cd A && foo | tee f` differs from pipeline-local `cd`; empty segment handling; input from `splitPOSIX` tokens (reuse `tokenize.go` only via call). <!-- sdd-owner: implementation -->
- [x] **2.2 GREEN:** implement `splitSegmentsWithSep` in new `catalog/recipes/worktree-flow/gate/cwd.go` (stdlib only). Keep `event.go::splitSegments` as a thin wrapper or update call sites. <!-- sdd-owner: implementation -->
- [x] **2.3 RED:** table tests for `staticDirOperand` + `resolveDir` (§3.4/§3.5 closed lists): reject `$`, backtick, `$(`, leading `~`, operand `-` / `-N`; accept quoted `"/tmp/My WT"` as one operand; reject non-existing directory (`IsExistingDirectory`); relative operand resolves against base only when base set. <!-- sdd-owner: implementation -->
- [x] **2.4 RED:** table tests for `recoverCwdWalk` (§3.3 semantics): sequential `S` updates only across `&&`/`;` after a recovered `cd`; `|`/`||` pipeline siblings inherit pre-pipeline `S`; `git -C <dir>` sets a per-segment git overlay without changing `S`; multiple `-C` chain (`git -C A -C B mv rel dest`); attached `-C<path>` accepted; `git -C A && echo x > rel` resolves `rel` against `S` (event cwd), NOT `A`; nested `cd A && cd B && echo x > rel` → last `cd` wins; `cd A && git -C B mv a dest` → `dest` against `B`; unrecoverable forms: `cd -`, `cd "$WT"`, `cd $(pwd)`, subshell `(cd A)`, non-existing operand — affected overlay and later sequential `S` become `none` without rewriting prior segments. <!-- sdd-owner: implementation -->
- [x] **2.5 GREEN:** implement `staticDirOperand`, `resolveDir`, and `recoverCwdWalk` in `cwd.go`. <!-- sdd-owner: implementation -->
- [x] **2.6 TRIANGULATE/REFACTOR:** add quoted-space fixture `git -C "/tmp/My WT" mv a b` (one operand, existing dir, created via `t.TempDir()` or `gitFixture`); confirm `cd` with no operand / extra operands is unrecoverable. Run `go test ./...` in the gate dir. <!-- sdd-owner: implementation -->

## Phase 3 — PR 3: event model (WriteCandidate, CwdTrusted, Source)

- [x] **3.1 RED:** extend `catalog/recipes/worktree-flow/gate/event_cwd_test.go`: SHELL event produces `[]WriteCandidate{Path, Base, Source}` with `Source=command` when `recoverCwdWalk` yields an overlay for the candidate's segment; `Source=event` only when `CwdTrusted`; `Source=none` otherwise. PATH event: no command walk; `Source=event` when `CwdTrusted`, else `none`; absolute PATH candidates classify regardless. Missing/unusable JSON cwd → `CwdTrusted=false` while `Cwd` still records the process fallback for diagnostics (do NOT treat fallback as trusted). Missing JSON cwd with SHELL command `git -C <wt> …` → still `Source=command`. <!-- sdd-owner: implementation -->
- [x] **3.2 RED:** test that `extractPass2` runs ONCE per event (not once per segment) and receives the final sequential `S` as its base (§3.6); existing `extract_pass2_test.go` must stay green. <!-- sdd-owner: implementation -->
- [x] **3.3 GREEN:** modify `catalog/recipes/worktree-flow/gate/event.go`: add `WriteCandidate` and `Event.CwdTrusted`; rewire `ParseEvent` SHELL path to `splitSegmentsWithSep` + `recoverCwdWalk` + per-segment `extractPass1`, single `extractPass2`; assign `Source` per mode. `extract.go` rules unchanged. <!-- sdd-owner: implementation -->
- [x] **3.4 REFACTOR:** keep `splitSegments` wrapper (or remove dead code), preserve `dedupeStrings` behavior; `go test ./...` green in gate dir. <!-- sdd-owner: implementation -->

## Phase 4 — PR 4: degrade routing + message parity (R2/R3)

- [x] **4.1 RED:** in `catalog/recipes/worktree-flow/gate/decide_test.go` (and/or a new `message` test block): verbatim tests for `BlockMessage(shell, toolName, candidate, branch, commandCwd, createWorktree)` — `createWorktree=true` keeps the legacy `/worktree-new` sentence plus the named cwd; `createWorktree=false` names the absolute command cwd and MUST NOT contain `/worktree-new`; shell variants still name the bash-bypass risk. `AskMessage` same extra args, three-destination guidance, MUST NOT contain `WORKTREE_GATE_MODE=off`. New `DegradeMessage(mode)`: no `/worktree-new`, no `WORKTREE_GATE_MODE=off`, no "to bypass"; `always` vs `ask` wording may differ, exit 0 for that candidate. Update existing `TestBlockMessageVerbatim` / `TestAskMessagePresentsThreeDestinationsAndNoSelfBypass` call sites. <!-- sdd-owner: implementation -->
- [x] **4.2 GREEN:** modify `catalog/recipes/worktree-flow/gate/message.go` with the new signatures and `DegradeMessage`. <!-- sdd-owner: implementation -->
- [x] **4.3 RED:** tests for `effectiveBase(c)` in `main_test.go`: absolute path → no base, no degrade; `Source=command|event` → base, no degrade; `Source=none` + relative → degrade. Then `run()` integration fixtures (design §7 matrix): `cd - && echo x > rel` → exit 0 in `always`, no `protected-branch`; omitted event cwd + relative + process cwd = protected primary → exit 0 (NOT a `$PWD` block); `cd "$WT" && …` → degrade; `echo x > rel` with trusted event cwd = primary → still exit 2; `cd <protected-primary> && echo x > f` → exit 2 with `/worktree-new`; absolute path inside primary → exit 2 unchanged; `gate_mode=ask` + degrade → exit 0, stderr is `DegradeMessage`; `gate_mode=off` → exit 0 before recovery. <!-- sdd-owner: implementation -->
- [x] **4.4 GREEN:** modify `catalog/recipes/worktree-flow/gate/main.go`: loop uses `effectiveBase`; degrade branch prints `DegradeMessage` and continues (no `Decide` block for that candidate, later honest block still exits 2); pass command cwd + `createWorktree` into `BlockMessage`/`AskMessage`; `IsClaudeException` joins against `effectiveBase`, not raw `event.Cwd`. `ResolveGateMode` untouched. <!-- sdd-owner: implementation -->
- [x] **4.5 RED/GREEN `explainRun` parity:** `--explain` uses the same effective base; add `cwd_source` (and `command_cwd`) to `explainOutput`; fixture `git -C <wt> mv rel-a rel-b` with event cwd = protected primary → `decision=allow`, `cwd_source=command`. `Cwd` stays the diagnostic event cwd. <!-- sdd-owner: implementation -->
- [x] **4.6 REFACTOR:** R3 edge — blocked non-session primary (subrepo/standalone extra primary fixture): message names that cwd, no `/worktree-new`; if the topology fixture is too expensive, unit-test `BlockMessage(..., createWorktree=false)` plus one `Decide` block on a standalone extra primary (design §12). Resolve the `Decision.RepoRoot` vs call-site helper question (§12) with the cheaper option. <!-- sdd-owner: implementation -->

## Phase 5 — Testing & validation

- [x] **5.1 Full §7 fixture matrix** (one `run()` test covering both hook ids via shared binary is enough): verify every row of the design §7 table passes, including `git -C A && echo x > rel` overlay trap and `gate_mode=off` short-circuit. <!-- sdd-owner: implementation -->
- [x] **5.2 Gate regression:** `go test ./...` in `catalog/recipes/worktree-flow/gate/` — all existing tests (`decide_test.go`, `event_cwd_test.go`, `extract_*`, `tokenize_*`, `uri_test.go`, `config_test.go`, `topology_test.go`) stay green. <!-- sdd-owner: implementation -->
- [x] **5.3 Repo validation:** `./tests/run.sh` and `./tests/validate.sh` at repo root — green (launcher, renderer, doctor untouched). <!-- sdd-owner: implementation -->
- [x] **5.4 Frozen-contract check:** `grep` confirms no edit under `lib/_internal/hooks-render.py`, `worktree-gate.sh`, `doctor.py`, `decide.go`, `tokenize.go`, `config.go`, `uri.go`, `extract.go`; `tests/test_hooks_render.py::_assert_process_cwd_event` still passes. <!-- sdd-owner: implementation -->
- [x] **5.5 Record RED/GREEN evidence** per strict TDD (config.yaml `strict_tdd: true`) in the apply progress notes: baseline (1.3), per-phase RED failures, final GREEN output. <!-- sdd-owner: implementation -->

## Phase 6 — Review gate (parent)

- [x] Start or reuse bounded review. N/A/complete because RDD is globally disabled by explicit user decision; no receipt or review approval invented. <!-- sdd-owner: parent -->

---

## Rollback boundaries

- PR 1: spec-delta-only revert; no code impact.
- PR 2: `cwd.go` + `cwd_test.go` are purely additive (new file + wrapper); revert restores baseline.
- PR 3: event model change is the first behavior-affecting slice; revert restores old `Event` shape. Old tests in `event_cwd_test.go` are updated in-slice.
- PR 4: degrade routing + messages; revert restores pre-change block-on-guess behavior (known false positives).
