# Apply Progress: tracker-ledger-foundation

Delivery: `size:exception` single PR (maintainer-authorized in `tasks.md`). Unit 1 is
the **first ordered commit slice**; this executor did not commit, push, or merge.

## Unit 1 — Phase 1: identity + witness (Go)

Tasks 1.1–1.5 complete. Persisted checkboxes updated in
`openspec/changes/tracker-ledger-foundation/tasks.md` (`- [x]` for 1.1, 1.2, 1.3, 1.4,
1.5).

### Files changed

| Path | Change |
|---|---|
| `catalog/recipes/worktree-flow/gate/ledger/identity.go` | New: work identity (A2, A11), `Facts` seam, archive-aware `ResolveChangeDir`, active-slug enumeration |
| `catalog/recipes/worktree-flow/gate/ledger/identity_test.go` | New: table-driven identity + change-dir tests, `t.TempDir()` git repos |
| `catalog/recipes/worktree-flow/gate/ledger/witness.go` | New: read-only witness decode → `Binding`, `WitnessPath` |
| `catalog/recipes/worktree-flow/gate/ledger/witness_test.go` | New: four states + missing/unreadable/unknown-`v` dormancy |
| `openspec/changes/tracker-ledger-foundation/tasks.md` | Checkboxes 1.1–1.5 → `- [x]` |
| `openspec/changes/tracker-ledger-foundation/apply-progress.md` | This file |

834 added lines across 4 Go files (385 + 233 + 126 + 90). No existing file modified
except the two `openspec/**` files above — the module, `main.go`, `gitfacts.go`, and the
worktree gate path are untouched.

### Focused test command

```bash
go -C catalog/recipes/worktree-flow/gate test ./ledger/...
# ok  ai-specs.dev/worktree-gate/ledger  2.0s   (39 PASS cases, 0 skip)
```

Regression sweep for the same commit (`tests/run.sh`'s Go half):

```bash
go -C catalog/recipes/worktree-flow/gate test ./...   # gate ok 11.3s + ledger ok 2.1s
gofmt -l catalog/recipes/worktree-flow/gate            # no output = clean
go -C catalog/recipes/worktree-flow/gate vet ./...     # clean
```

### TDD Cycle Evidence

| Task | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| 1.1 identity | `go test ./ledger/...` → `build failed` (`undefined: DeriveIdentity`, `IdentityOptions`, `ReasonIdentityUnavailable`, …) | identity cases pass | Split into A2 table (named branch / detached / unborn / non-repo), injected-facts case, slug enrichment (0 / 1 / several / archive-excluded / separate planning root), stored slug after archive, rename = new identity, two worktrees = one identity | `realPath` resolves an as-yet-missing `.git` through its parent so worktree and checkout agree; comment added for why `rev-parse --verify HEAD` is required |
| 1.2 witness | same RED run (`undefined: ReadBinding`, `WitnessBound`, …) | `TestReadBindingStates` (bound/ambiguous/unbound/declared-not-bound) + `TestReadBindingMissingIsDormant` (8 unusable shapes) pass | Added unusable-path cases beyond the task text: invalid JSON, missing `v`, wrong capability, unknown state, bound-without-recipe-id (never guess a provider) | `dormant()` helper collapses the unusable outcomes into one vocabulary-legal reason |
| 1.4 archive-aware slug | n/a (test-only task) | `TestResolveChangeDirArchiveOrder` asserts active → latest dated → legacy → fallback with pinned paths | **Mutation check**: changed `dated[len(dated)-1]` → `dated[0]`; `TestResolveChangeDirArchiveOrder` and `TestResolveChangeDirMatchesPythonHelper/latest_dated_wins` both FAILed (`go=…2026-01-01-slug python=…2026-03-03-slug`), proving the Python parity test is not vacuous; mutation reverted and both pass | `TestResolveChangeDirMatchesPythonHelper` skips when `python3`/helper is absent so release CI without Python cannot fail on it; malformed-date guard extracted to `isISODate` |

Python parity reference (read-only, not modified):
`tests/_change_paths.py::change_dir` — 6 fixture trees (`active wins`, `latest dated
wins`, `malformed date ignored`, `legacy undated`, `fallback`, `suffix not a slug
match`) produce byte-identical resolution in Go and Python.

### Deviations from design (Unit 1 only)

1. **Git-facts seam.** `gitfacts.go` is `package main`, which is not importable by
   `package ledger`, so `identity.go` cannot call `gitCommonWith`/`gitMemo` directly.
   Design A1's "shared Git facts stay in `gitfacts.go`" is honored by injection instead
   of duplication: the ledger exposes `type Facts func(dir string, args ...string)
   string` (identical signature to `gitMemo`), injectable via
   `IdentityOptions.Facts`; Unit 3's `ledger_cmd.go` passes the gate's memoized reader.
   A small built-in `localFacts` fallback keeps the package self-contained for tests.
   The `ledger` package imports nothing from the module
   (`go list -deps ./ledger` lists only `ai-specs.dev/worktree-gate/ledger`), so no
   worktree `Decide`/`Event`/cleanup type is reachable (1.5 grep evidence: no code
   references; prose comments only).
2. **Unborn branch detection.** `symbolic-ref --quiet --short HEAD` still names a branch
   in a freshly `git init`-ed repository, so A2's "unborn branch" case additionally
   requires `rev-parse --verify --quiet HEAD` to fail. Without that, an empty repository
   would produce an identity with no commits to tie an item to.
3. **One reason for every unusable witness.** Missing, unreadable, invalid JSON,
   unknown/missing `v`, non-`tracker` capability, unknown state, and a `bound` state
   without `recipe_id` all normalize to `unbound` + `witness-missing`. The design
   vocabulary has no other dormant reason, and D6 forbids guessing a provider, so no
   new reason string or provider inference was introduced.
4. **Stored slug precedence.** A stored slug (mid-item archive, A11) is kept even when
   several active folders exist, because the item already knows its change. Tested
   explicitly; a fresh derivation still reports `change-ambiguous`.

### Remaining tasks (unchecked lines in the persisted tasks artifact)

```text
- [ ] 2.1 RED: `catalog/recipes/worktree-flow/gate/ledger/store_test.go`: missing file = empty items; corrupt JSON = `unevaluable`/`store-corrupt`, never synthesized; atomic `state.json.tmp.*` + rename leaves no temp residue; identity key `common_dir+"\x1f"+branch[+"\x1f"+change]`. <!-- sdd-owner: implementation -->
- [ ] 2.2 RED: item rules — closed never reopened, reused branch opens a NEW item (D17), two `open` rows = conflict not pick, provider-neutral fields only (opaque `provider` unread), append-only `decisions[]` kinds `adjudicate|opt-out|open|close|link`, `opt-out` scoped to one checkpoint (D19), advisory ceiling 32 items / 64 KiB (no compaction). <!-- sdd-owner: implementation -->
- [ ] 2.3 GREEN: implement `catalog/recipes/worktree-flow/gate/ledger/store.go` at `<git-common-dir>/ai-specs/ledger/state.json` (A3). <!-- sdd-owner: implementation -->
- [ ] 2.4 TRIANGULATE/REFACTOR: concurrent-append test (two writers, one replace wins, no lost decision); commit. <!-- sdd-owner: implementation -->
- [ ] 3.1 RED: `catalog/recipes/worktree-flow/gate/ledger/verdict_test.go` — 5 checkpoints × 3 modes table for the design posture matrix incl. `always` blocking missing/conflicted (`needs-item`) and pre-merge `identity_unavailable`, `warn` never blocking, `ask` → `decision=ask` exit 0. <!-- sdd-owner: implementation -->
- [ ] 3.2 RED: `catalog/recipes/worktree-flow/gate/ledger/decide_test.go` — four-side evidence disagreement records conflict with no default winner; `--decide` persists then re-grade allows; failed persist exits 2. <!-- sdd-owner: implementation -->
- [ ] 3.3 RED: `catalog/recipes/worktree-flow/gate/ledger_cmd_test.go` — flag surface (`--ledger --checkpoint --ledger-mode --project-root --witness --store --evidence --decide --explain`), exact JSON keys from design, exit 0 for `allow|ask|dormant|unevaluable` and 2 only for `block`, flag-parse fail-open on verdict calls. <!-- sdd-owner: implementation -->
- [ ] 3.4 GREEN: implement `catalog/recipes/worktree-flow/gate/ledger/verdict.go`, `catalog/recipes/worktree-flow/gate/ledger/decide.go`, `catalog/recipes/worktree-flow/gate/ledger_cmd.go`, and the `--ledger` switch in `catalog/recipes/worktree-flow/gate/main.go` (cleanup-mode precedent). <!-- sdd-owner: implementation -->
- [ ] 3.5 GREEN: extend `selftest` in `catalog/recipes/worktree-flow/gate/main.go` to assert ledger identity/verdict invariants in-process, still printing `ok` and keeping gate regexp compiles. <!-- sdd-owner: implementation -->
- [ ] 3.6 TRIANGULATE: `go build` + `dist/worktree-gate-current --ledger --checkpoint work-start --ledger-mode warn` on this repo; confirm existing `--explain`/worktree corpus output unchanged. <!-- sdd-owner: implementation -->
- [ ] 3.7 REFACTOR + commit: mode/enum helper names, no coupling to worktree gate semantics (D4/A1). <!-- sdd-owner: implementation -->
- [ ] 4.1 RED: `tests/test_tracker_ledger_witness.py` — after sync, `<git-common-dir>/ai-specs/ledger/witness.json` exists with the four states, candidate ids for `ambiguous`, no provider guessed, and survives the `RESOLVED_CONFIG_TEMP` EXIT trap. <!-- sdd-owner: implementation -->
- [ ] 4.2 RED: add synthetic provider `tests/fixtures/recipes/test-tracker-ledger/recipe.toml` declaring `tracker` (gated by `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES`) + `tests/fixtures/recipes/test-tracker-ledger-conflict/recipe.toml` for the ambiguous case. <!-- sdd-owner: implementation -->
- [ ] 4.3 GREEN: add the atomic `mkstemp`+`os.replace` witness writer to `lib/_internal/recipe-materialize.py` right after `resolve_bindings`/`check_capability_conflicts`; keep it the only binding producer. <!-- sdd-owner: implementation -->
- [ ] 4.4 GREEN: confirm `lib/sync.sh` deletes only `RESOLVED_CONFIG_TEMP` (no witness trap, no second temp root). <!-- sdd-owner: implementation -->
- [ ] 4.5 TRIANGULATE: worktree case — witness written from the main checkout is read by a linked worktree (common-dir, not realpath cache). <!-- sdd-owner: implementation -->
- [ ] 4.6 REFACTOR + commit; `python3 -m py_compile lib/_internal/recipe-materialize.py`. <!-- sdd-owner: implementation -->
- [ ] 5.1 RED: `tests/test_ledger_mode_config.py` for A9 mapping (`ledger_mode` wins; `gate_mode` off→skip, warn→warn, always→always; worktree `gate_mode` never read) against `catalog/recipes/trello-mcp-workflow/recipe.toml`. <!-- sdd-owner: implementation -->
- [ ] 5.2 GREEN: add `[config.ledger_mode]` enum `always|ask|warn` default `warn` to `catalog/recipes/trello-mcp-workflow/recipe.toml` and its README config table in the same commit. <!-- sdd-owner: implementation -->
- [ ] 5.3 GREEN+RED: `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` resolves the verified binary and grades `work-start` (no change folder required; `openspec/**` never blocked). <!-- sdd-owner: implementation -->
- [ ] 5.4 GREEN+RED: `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` grades `apply-start` (`kind=path`) and `pr-review` (`kind=shell` `pr_create`), delete the heredoc `## Tracker` grader, prompt on `ask`, persist the answer via `--decide`; update `tests/test_tracker_card_gate_hook.py`. <!-- sdd-owner: implementation -->
- [ ] 5.5 GREEN+RED: `lib/_internal/premerge_guardian.py` invokes `pre-merge` and `archive-close` at both stages, tolerates cold `AI_SPECS_HOME`, leaves tier/verify-evidence math untouched; update `tests/test_premerge_guardian.py`. <!-- sdd-owner: implementation -->
- [ ] 5.6 TRIANGULATE: all five hosts return identical verdict/exit for one pinned fixture input (spec "All five checkpoints reach one predicate"). <!-- sdd-owner: implementation -->
- [ ] 5.7 REFACTOR: hosts are acquisition/JSON bridges only — grep-assert no `is_valid_link`-style predicate added in shell or Python; point `catalog/recipes/git-pr-flow/commands/pr-create.md` at the verdict; commit. <!-- sdd-owner: implementation -->
- [ ] 6.1 GREEN+RED: `lib/_internal/doctor.py` renders the JSON `doctor` finding under check `tracker-ledger` with A10 severities (INFO unbound, WARN ambiguous/declared-not-bound/missing-witness/conflict, ERROR infra) and stops grading; drop `_check_tracker_card_link` as a grader; update `tests/test_doctor_tracker_card.py`; assert no runtime-brief/`AGENTS.md` dormancy line (D15). <!-- sdd-owner: implementation -->
- [ ] 6.2 GREEN: `lib/_internal/trello_link.py` stays a parser — comments + no new predicate; existing consumers delegate to the Go verdict. <!-- sdd-owner: implementation -->
- [ ] 6.3 RED: pinned `tests/fixtures/tracker-ledger-corpus/*.json` covering every design test-table row (dormant, empty store × modes, consistent evidence, four-side conflict, persisted `--decide`, checkpoint-scoped opt-out, branch reuse → new item, two open items, detached HEAD per mode, `openspec/**` never blocked, missing binary → exit 0). <!-- sdd-owner: implementation -->
- [ ] 6.4 GREEN: `tests/test_tracker_ledger_parity.py` drives `dist/worktree-gate-current --ledger` per fixture (mirror `tests/test_worktree_gate_parity.py` (read-only) shape) and fails on any host/Go divergence. <!-- sdd-owner: implementation -->
- [ ] 6.5 TRIANGULATE: run corpus twice; byte-identical verdicts and no store residue; confirm existing worktree-gate corpus unchanged. <!-- sdd-owner: implementation -->
- [ ] 6.6 REFACTOR + commit. <!-- sdd-owner: implementation -->
- [ ] 7.1 GREEN: update `docs/capabilities.md`, `docs/runtime-hooks.md`, `README.md`, `catalog/recipes/trello-mcp-workflow/README.md`, and `CHANGELOG.md` with the witness/store paths, five checkpoints, modes, dormancy-in-doctor, and "no provider writes in this slice". <!-- sdd-owner: implementation -->
- [ ] 7.2 VERIFY: `scripts/build-gate.sh` then `scripts/verify-gate-sums.sh` — same four assets, one trust root; regenerate `catalog/recipes/worktree-flow/bin/SHA256SUMS` only with canonical `go1.24.13`, else record the pending-release note. <!-- sdd-owner: implementation -->
- [ ] 7.3 VERIFY: `./tests/validate.sh` (py_compile, `bash -n`, gofmt, Go tests incl. `ledger`, unittest discovery) fully green; no runner path edits were needed because the module did not move. <!-- sdd-owner: implementation -->
- [ ] 7.4 GREEN: write `openspec/changes/tracker-ledger-foundation/verify-report.md` with one `Criterion N: PASS` row per the 11 proposal Success Criteria, plus RED/GREEN evidence per unit. <!-- sdd-owner: implementation -->
- [ ] 7.5 GREEN: append the Judgment Day input set to `openspec/changes/tracker-ledger-foundation/verify-report.md` (spec scenario → corpus fixture → test name) for up to 3 authorized rounds; record each round's fix re-run. <!-- sdd-owner: implementation -->
- [ ] 7.6 VERIFY: `python3 lib/_internal/premerge_guardian.py --root . --stage pre-archive` then `--stage pre-merge` pass; archive this change folder to `openspec/changes/archive/2026-09-13-tracker-ledger-foundation/` on the review branch and re-run the guardian after the move. <!-- sdd-owner: implementation -->
- [ ] 7.7 VERIFY: open ONE PR for the whole change with `gh` (base `development`, accepted `size:exception`), keeping the 7 work units as separate commits in unit order, no merge, and record the PR URL + total changed-line count + per-commit line counts in `openspec/changes/tracker-ledger-foundation/verify-report.md`. <!-- sdd-owner: implementation -->
```

Units 2–7 are untouched; stop point is exactly after Unit 1 (Phase 1) as instructed.

### Workload / PR boundary

- Assigned slice: Unit 1 only. Authored lines: **~834** (4 new Go files), over the
  400-line review budget — expected: `tasks.md` declares `400-line budget risk: High`
  with an accepted `size:exception` for the whole change (review budget constrains the
  slice, not the code; no comments, tests, or cases were dropped to fit).
- PR boundary: single PR for the whole change; Unit 1 is commit slice 1
  (`ledger` package only — no importer, deletable, worktree gate path untouched).
  `Review Workload Forecast` gate: `Decision needed before apply: No`,
  `Chained PRs recommended: No`, `Chain strategy: size-exception` — delivery decision
  was already resolved by the maintainer, so implementation proceeded.

### Structured status consumed / produced

Consumed (parent prompt + native status engine): `artifactStore: openspec`,
`changeRoot: openspec/changes/tracker-ledger-foundation`, `actionContext.mode:
repo-local`, `allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli]`,
worktree used: `.worktrees/tracker-ledger-foundation` on
`change/tracker-ledger-foundation`. The native status JSON in the parent prompt listed
`sdd/*` artifacts as `missing` with `nextRecommended: "No active SDD changes found."`;
per the artifact-store rules for `openspec` the authoritative inputs were read from the
change folder on disk (proposal/spec/design/tasks all present) rather than treated as
missing. `actionContext.warnings`: none — no target file left the allowed roots
(`catalog/recipes/worktree-flow/gate/ledger/**`, `openspec/changes/tracker-ledger-foundation/**`).
