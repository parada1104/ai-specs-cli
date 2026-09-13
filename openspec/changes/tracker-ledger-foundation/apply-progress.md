# Apply Progress: tracker-ledger-foundation

Delivery: `size:exception` single PR (maintainer-authorized in `tasks.md`). Units 1 and 2
are the **first two ordered commit slices**; this executor did not commit, push, or merge.

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

Units 3–7 remain untouched; stop point is exactly after Unit 2 (Phase 2) as instructed.

### Workload / PR boundary

- Assigned slices: Units 1–2. Authored lines: **~834 (Unit 1) + 934 (Unit 2) = 1,768**
  across 6 new Go files, over the 400-line review budget — expected: `tasks.md` declares
  `400-line budget risk: High` with an accepted `size:exception` for the whole change (the
  budget constrains how work is sliced, never the code; no comments, tests, or cases were
  dropped to fit).
- PR boundary: single PR for the whole change; Unit 1 is commit slice 1 and Unit 2 is
  commit slice 2 (`ledger` package only — no importer, no `main.go` change, worktree gate
  path untouched). `Review Workload Forecast` gate: `Decision needed before apply: No`,
  `Chained PRs recommended: No`, `Chain strategy: size-exception` — delivery decision was
  already resolved by the maintainer, so implementation proceeded.

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

## Unit 2 — Phase 2: store (Go)

Tasks 2.1–2.4 complete. Persisted checkboxes updated in
`openspec/changes/tracker-ledger-foundation/tasks.md` (`- [x]` for 2.1, 2.2, 2.3, 2.4; the
Phase 2 block now has zero unchecked rows). Re-read after the edit to confirm.

### Files changed

| Path | Change |
|---|---|
| `catalog/recipes/worktree-flow/gate/ledger/store.go` | New: atomic `state.json` store (A3/A5), item/decision model, `Primary`/`OpenItems`, `HasOptOut`, `OverCeiling`, locked `AppendDecisionToPrimary` |
| `catalog/recipes/worktree-flow/gate/ledger/store_test.go` | New: path/corruption/atomic-write/identity-key/item-rule/opt-out/ceiling/concurrency tests |
| `openspec/changes/tracker-ledger-foundation/tasks.md` | Checkboxes 2.1–2.4 → `- [x]` |
| `openspec/changes/tracker-ledger-foundation/apply-progress.md` | This section (merged with Unit 1) |

934 added lines across 2 new Go files (340 + 594). No existing file modified except the
two `openspec/**` files above — `main.go`, `gitfacts.go`, the module, and the worktree gate
path are untouched. `go list -deps ./ledger` lists only stdlib plus the `ledger` package:
no worktree `Decide`/`Event`/cleanup type is reachable (A1).

### What the store implements

- **Path (A3).** `StorePath(common) = <git-common-dir>/ai-specs/ledger/state.json`; not
  `openspec/**`, not committed.
- **Read (A5).** Missing file = empty items (`Store{V: StoreVersion}`). Invalid JSON, a
  non-object, an empty file, or an unknown `v` = `*StoreCorruptError` carrying zero items:
  unevaluable (`ReasonStoreCorrupt = "store-corrupt"`), and the file bytes are left exactly
  as found (never repaired or synthesized).
- **Write (A5).** `SaveStore` creates dirs, writes `state.json.tmp.*` in the destination
  directory, fsyncs, then `os.Rename` over the target; a later save replaces rather than
  appends. The empty store marshals `"items": []`, never `null`.
- **Identity key (A5).** `ItemIdentity.Key()` = `IdentityKey(common, branch[, change])`,
  i.e. `common_dir + "\x1f" + branch [ + "\x1f" + change]`. A change-enriched item is not
  matched by the branch-only or another-change key.
- **Item rules (A5/D17).** `Primary(key)` selects only `status == "open"` rows: 0 →
  `ErrNoPrimary`, 1 → the item, 2+ → `ErrMultipleOpen` with **no item returned** (a
  collision is for a human, never a silent pick). `OpenItem` always appends a NEW row and
  `CloseItem` appends a `close` decision + flips status; there is no reopen path, so a
  closed row stays closed and a reused branch opens a new item.
- **Provider neutrality (A7).** Core fields are exactly `id, identity, status, item_id,
  provider_id, native_type, url, state, provider, exemption, conflict, decisions`. Provider
  vocabulary lives only in the opaque `provider` `json.RawMessage`, which the predicates
  never read and which round-trips unchanged.
- **Decisions (A6/D19).** `decisions[]` is append-only over the closed kind set
  `adjudicate|opt-out|open|close|link`; an unknown kind returns `ErrUnknownDecisionKind`
  without mutating the item. `HasOptOut(key, checkpoint)` matches `kind=opt-out` **and** the
  checkpoint, so an `apply-start` opt-out does not cover `pre-merge`.
- **Ceiling (A5).** `OverCeiling()` is advisory only: `> 32` items or `> 64 KiB` marshalled;
  saving never drops rows (no compaction).
- **Concurrency.** `AppendDecisionToPrimary(path, key, d)` is the locked read-modify-write
  append. See deviation 1.

### Focused test command

```bash
go -C catalog/recipes/worktree-flow/gate test ./ledger/...
# ok  ai-specs.dev/worktree-gate/ledger  2.2s
```

Triangulation runs for the concurrency and collision rules:

```bash
go -C catalog/recipes/worktree-flow/gate test -race -count=3 \
  -run 'TestConcurrentAppend|TestTwoOpen|TestReusedBranch|TestClosedItem' ./ledger/...
# ok  ai-specs.dev/worktree-gate/ledger  1.4s   (race detector clean)
```

Regression sweep for the same commit:

```bash
go -C catalog/recipes/worktree-flow/gate test ./...   # gate ok (cached) + ledger ok
gofmt -l catalog/recipes/worktree-flow/gate            # no output = clean
go -C catalog/recipes/worktree-flow/gate vet ./...     # clean
```

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 2.1 store read/write/key | `ledger/store_test.go` | Unit | ✅ `go test ./ledger/...` 39/39 before any edit | ✅ `build failed` (`undefined: ItemIdentity`, `StorePath`, `LoadStore`, `StoreVersion`, `StoreCorruptError`, …) | ✅ 8 store cases pass | ✅ 4 corrupt shapes + missing file + replace-not-append + corrupt-file-unchanged + 4 non-matching identity keys + stable/unique 16-hex id | ✅ `writeAtomic` extracted; typed `StoreCorruptError` |
| 2.2 item rules | `ledger/store_test.go` | Unit | ✅ as above | ✅ same RED run | ✅ closed/collision/provider-neutral/append-only/opt-out/ceiling cases pass | ✅ D17 same-change and new-change reuse; provider key-set guard + opaque round-trip; unknown-kind rejection; per-checkpoint opt-out + non-opt-out decision; both ceiling branches + 40-row no-compaction round trip | ✅ shared `decisionKinds` set; `OpenItems` backs `Primary` |
| 2.3 GREEN store path | `ledger/store.go` | Unit | ✅ as above | ✅ (same RED build) | ✅ `TestStorePathDesignAuthority` + full ledger suite | ✅ `go list -deps ./ledger` shows no gate types | ✅ comment-pass on A3/A5/A7 intent |
| 2.4 concurrency | `ledger/store_test.go` | Unit | ✅ as above | ✅ same RED run | ✅ `TestConcurrentAppendDoesNotLoseDecisions` | ✅ `-race -count=3` green; both opt-outs persisted (3 decisions); no `state.json.tmp.*` residue; fail-closed paths for no-primary/collision/unknown-kind | ✅ lock extracted to `withStoreLock` |

**Mutation checks (proving the assertions are not vacuous):** reverting three rules in
`store.go` — `Primary`'s collision branch returning `open[0]`, `HasOptOut` dropping the
checkpoint match, and `OpenItems` dropping the `status == open` filter — produced exactly
three failures (`TestTwoOpenItemsAreConflictNotPick`, `TestOptOutIsCheckpointScopedD19`,
`TestClosedItemIsNeverSelectedOrReopened`). Mutations were reverted; suite green again.

### Deviations from design (Unit 2 only)

1. **Cross-process lock for appends.** A4/A5 specify atomic temp+rename, which prevents a
   torn file but not a lost read-modify-write. Task 2.4 requires "two writers … no lost
   decision", so `AppendDecisionToPrimary` takes an exclusive `flock` (stdlib
   `syscall.Flock`, unix-only like the four-target release matrix) on `state.json.lock`
   around load → append → save. The kernel releases the lock on close or crash, so a dead
   writer cannot wedge the ledger. The only residue is a persistent `state.json.lock`; the
   atomic-write test globs `state.json.tmp.*` and stays clean.
2. **Unknown store version is corrupt.** Design pins store `"v": 1`; an unknown version is
   `store-corrupt`/unevaluable rather than reinterpreted.
3. **Opaque `provider` is `json.RawMessage`.** The design example draws `{}`; the field is
   unread, so any JSON payload round-trips compactly and no provider key can be promoted to
   core. `TestItemCoreFieldsAreProviderNeutral` pins the neutral key set and bans provider
   vocabulary at the top level.
4. **Item-id join.** Design says "16-hex sha256-of-identity-key-plus-opened-at"; the two
   inputs are joined with the same `\x1f` unit separator the identity key uses, keeping one
   separator convention in the package.
5. **`CloseItem` sets `kind=close` itself** (it appends via `AppendDecision`) so a caller
   cannot close an item while recording a different kind; `OpenItem` similarly records the
   initial `kind=open` decision.

### Slice workload / PR boundary (Unit 2)

- Authored lines: **934** (340 `store.go` + 594 `store_test.go`), over the 400-line review
  budget. This is expected under the maintainer-accepted `size:exception`; no comments,
  tests, or edge cases were dropped to fit, and no restyling was done.
- Boundary: commit slice 2 of the single PR. Only the `ledger` package is touched; nothing
  imports it and there is no `main.go`/CLI change, so reverting `store.go`/`store_test.go`
  leaves the worktree gate and its trust path untouched. Verdict/CLI (Unit 3) and host
  wiring (Units 5–6) were deliberately not started.

### Remaining tasks

Phase 3–7 (`3.1`–`7.7`) remain unchecked in
`openspec/changes/tracker-ledger-foundation/tasks.md`; exact lines are listed in the
Unit 1 tail above from `- [ ] 3.1` onward (the `2.x` rows were removed from that list as
they are now `- [x]`).

### Structured status consumed / produced (Unit 2)

Consumed: same `openspec` artifact store and repo-local `actionContext` as Unit 1, with
`allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli]`; this executor wrote only
inside `.worktrees/tracker-ledger-foundation` on `change/tracker-ledger-foundation`
(`catalog/recipes/worktree-flow/gate/ledger/**`,
`openspec/changes/tracker-ledger-foundation/**`). No `actionContext` warning fired and no
edit left the allowed roots. Produced: this progress artifact plus the persisted `- [x]`
marks for 2.1–2.4.
