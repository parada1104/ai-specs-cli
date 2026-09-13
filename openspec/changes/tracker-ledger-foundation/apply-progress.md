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

Phase 4–7 (`4.1`–`7.7`) remain unchecked in
`openspec/changes/tracker-ledger-foundation/tasks.md`; exact lines are listed in the
Unit 1 tail above from `- [ ] 4.1` onward (the `2.x` and `3.x` rows were removed from
that list as they are now `- [x]`).

### Structured status consumed / produced (Unit 2)

Consumed: same `openspec` artifact store and repo-local `actionContext` as Unit 1, with
`allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli]`; this executor wrote only
inside `.worktrees/tracker-ledger-foundation` on `change/tracker-ledger-foundation`
(`catalog/recipes/worktree-flow/gate/ledger/**`,
`openspec/changes/tracker-ledger-foundation/**`). No `actionContext` warning fired and no
edit left the allowed roots. Produced: this progress artifact plus the persisted `- [x]`
marks for 2.1–2.4.

## Unit 3 — Phase 3: verdict + CLI

Tasks 3.1–3.7 complete. Persisted checkboxes updated in
`openspec/changes/tracker-ledger-foundation/tasks.md` (`- [x]` for 3.1–3.7; the Phase 3
block now has zero unchecked rows). Re-read after the edit to confirm.

### Files changed

| Path | Change |
|---|---|
| `catalog/recipes/worktree-flow/gate/ledger/verdict.go` | New: pure checkpoint predicate — 5 checkpoints × 3 modes, evidence model, conflict snapshot, prompt, doctor finding |
| `catalog/recipes/worktree-flow/gate/ledger/verdict_test.go` | New: posture matrix (5×3×8 scenarios), ask/warn/always pins, opt-out/adjudication, enum surface, exit codes |
| `catalog/recipes/worktree-flow/gate/ledger/decide.go` | New: `DecisionRequest` validation + locked `PersistDecision`/`PersistConflict`; conflict clearing |
| `catalog/recipes/worktree-flow/gate/ledger/decide_test.go` | New: four-side conflict predicate, adjudication round trip, fail-closed persist paths, checkpoint-scoped opt-out |
| `catalog/recipes/worktree-flow/gate/ledger_cmd.go` | New (`package main`): `--ledger` dispatcher, exact JSON contract, evidence loader, decide persist, `ledgerSelftest` |
| `catalog/recipes/worktree-flow/gate/ledger_cmd_test.go` | New: flag surface, exact JSON keys, exit-code contract, dormant/unevaluable/block, decide round trip, fail-open |
| `catalog/recipes/worktree-flow/gate/main.go` | `--ledger` flag set + switch (cleanup precedent); `selftest` calls `ledgerSelftest` |
| `openspec/changes/tracker-ledger-foundation/tasks.md` | Checkboxes 3.1–3.7 → `- [x]` |
| `openspec/changes/tracker-ledger-foundation/apply-progress.md` | This section (merged with Units 1–2) |

**1,925 authored lines** across 6 new Go files + 24 added `main.go` lines (338 + 429 +
123 + 223 + 275 + 513 = 1,901 new; `main.go` +24). The worktree gate path is otherwise
untouched: `Decide`, `Event`, cleanup, tokenize and the `--explain` path have no ledger
reference (grep evidence below).

### What Unit 3 implements

- **One predicate, five checkpoints (3.4).** `ledger.Grade(Input) Verdict` is pure (no
  IO): it short-circuits dormant → `unevaluable` (corrupt store) → `identity_unavailable`
  → item selection, then applies the mode posture. `Checkpoints` is exactly
  `work-start, apply-start, pr-review, pre-merge, archive-close`; `LedgerModes` is exactly
  `always, ask, warn`; `NormalizeMode` defaults an unset/unknown mode to `warn` (A9/D18
  warn-first adoption). The worktree `gate-mode` is never read (D4).
- **Decisions.** `allow | block | ask | dormant | unevaluable`; `Verdict.ExitCode()` is
  `2` only for `block`. `ask` carries a `Prompt` (reason, four sides, legal choices) and
  exits `0`; `warn` never blocks; `always` blocks missing/conflicted state with
  `needs-item`/`conflict`, and blocks `identity_unavailable` at every checkpoint.
- **Evidence and conflict (3.2/A6).** Four sides (`local` = ledger item snapshot unless
  overridden, `remote`/`code`/`git` = host `--evidence` file). An empty side is
  unavailable and never a winner; any available side disagreeing with the snapshot is a
  conflict. A conflict grade records the snapshot on the item; a persisted
  `adjudicate`/`opt-out` decision for that checkpoint resolves it and `PersistDecision`
  clears the snapshot, so doctor stops warning.
- **CLI (3.3/3.4).** `--ledger --checkpoint --ledger-mode --project-root --witness
  --store --evidence --decide --explain` on the existing binary (one module, one asset,
  one trust root; A1). Stdout is always one JSON object with the design keys
  (`capability, active, checkpoint, mode, decision, reason, identity{common_dir,branch,
  change,key}, item, conflict, prompt, doctor{severity,name,message}`); `--explain` is an
  alias. Evidence/flag-parse/IO problems fail open (exit `0`); a failed `--decide`
  persist fails closed (exit `2`). Default paths are `<git-common-dir>/ai-specs/ledger/
  {witness,state}.json`; `--witness`/`--store` override them.
- **Selftest (3.5).** `--selftest` still compiles every gate regexp, checks git, and then
  runs `ledgerSelftest()` in-process (enum surface, warn/ask/always posture, dormant,
  conflict predicate) before printing `ok`. No network, no store IO.
- **Repository reality check (3.6).** `scripts/build-gate.sh` built the four targets and
  `dist/worktree-gate-current`; `--selftest` prints `ok`; `--ledger --checkpoint
  work-start --ledger-mode warn` on this repo printed
  `decision=dormant reason=witness-missing active=false` (Unit 4 has not written a
  witness yet) with exit `0`.

### Focused test commands

```bash
go -C catalog/recipes/worktree-flow/gate test -count=1 ./ledger/...
# ok  ai-specs.dev/worktree-gate/ledger  2.6s   (13 + 7 new tests + 5×3×8 matrix cells)
go -C catalog/recipes/worktree-flow/gate test -count=1 .
# ok  ai-specs.dev/worktree-gate  14.5s          (15 new ledger_cmd tests + existing gate suite)
go -C catalog/recipes/worktree-flow/gate test -count=1 ./...
# ok  both packages
gofmt -l catalog/recipes/worktree-flow/gate   # no output
go -C catalog/recipes/worktree-flow/gate vet ./...   # clean
./scripts/build-gate.sh && ./dist/worktree-gate-current --selftest   # ok
python3 -m unittest tests.test_worktree_gate_parity   # 8 tests OK (corpus unchanged)
```

Behavior parity for the untouched modes was checked against a pre-change build
(`main.go` stashed, binary built to `/tmp/gate-old`): `--explain` output is byte-identical
for empty stdin, a path event, a shell event and `--gate-mode off`; `--tokenize` output is
identical. The worktree-gate Python corpus (`tests/test_worktree_gate_parity.py`, 8
tests) stays green.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 3.1 verdict | `ledger/verdict_test.go` | Unit | ✅ `go test ./...` gate ok + ledger 27/27 before any edit | ✅ `undefined: Evidence, Verdict, Checkpoints, Grade, …` (build failed) | ✅ posture matrix + 6 focused tests pass | ✅ 5 checkpoints × 3 modes × 8 scenarios = 120 cells, plus dormant-beats-corrupt ordering, ambiguous/declared severities, unknown checkpoint, exit-code map | ✅ `rfc3339Stamp` extracted; `okayDoctor`/`bindingDoctor`/`verdictDoctor` split |
| 3.2 decide | `ledger/decide_test.go` | Unit | ✅ same baseline | ✅ same RED build (`undefined: DecisionChoices, DecisionRequest, PersistDecision, PersistConflict`) | ✅ conflict predicate, adjudication round trip, fail-closed paths pass | ✅ 7 conflict shapes; opt-out checkpoint scoping; snapshot clear; collision + corrupt + no-primary + bad kind/choice + bad path all fail closed | ✅ `Normalize`/`Validate` split; typed `ErrInvalidDecision`/`ErrUnknownChoice`; `clearConflict`/`setConflict` helpers |
| 3.3 CLI | `ledger_cmd_test.go` | Unit (in-process `run`) | ✅ same baseline | ✅ JSON-not-emitted + wrong exit codes because `--ledger` was unknown; ledger package build failed | ✅ 15 CLI tests pass | ✅ exact top-level/identity/doctor key sets; exit 0 for allow/ask/dormant/unevaluable, 2 for block; decide round trip; invalid JSON; flag-parse fail-open; detached HEAD per mode; two-open collision; item populated; conflict persisted then cleared; explain alias; worktree explain unchanged | ✅ token names `newLedgerOut`/`loadLedgerEvidence`/`persistLedgerDecision`/`storedLedgerSlug`; `emptyLedgerStorePath`/`recordedConflict` test helpers |
| 3.4 GREEN impl | (same files) | Unit | ✅ as above | ✅ as above | ✅ all suites green after `go build` | ✅ full `go test ./...` + `-count=1` | ✅ coupling grep + `go list -deps ./ledger` (only stdlib + ledger) |
| 3.5 selftest | `ledger_cmd_test.go` (`TestLedgerJSONContractKeys`, existing `TestSelftestOK`) + script run | Unit + binary | ✅ existing `TestSelftestOK` | ✅ mutation B (ask→block) made `TestSelftestOK` FAIL, proving `ledgerSelftest` runs | ✅ `--selftest` prints `ok`, exit 0 | ✅ binary `--selftest` after `build-gate.sh`; gate regexp compile + git checks still precede it | ✅ ledger invariant block isolated in `ledgerSelftest()` in `ledger_cmd.go` |
| 3.6 triangulate | build + binary + Python corpus | Integration | ✅ parity test 8/8 pre-change (baseline green) | n/a (verification task) | ✅ binary emits dormant JSON exit 0 on this repo | ✅ old vs new `--explain` byte-identical (4 inputs) + `--tokenize` identical; parity corpus 8/8 OK | ✅ `--explain` worktree path untouched |
| 3.7 refactor | `ledger/verdict_test.go`, `ledger/decide_test.go` | Unit | ✅ 35/35 new tests before refactor | n/a (refactor) | ✅ green after dedupe | ✅ two mutation checks below | ✅ `rfc3339Stamp` dedupe; enum helper names verified; no worktree coupling |

**Mutation checks (proving the assertions are not vacuous):**
1. `Evidence.Conflict` with the `side != e.Local` comparison removed → `TestEvidenceConflictFourSidesNoDefaultWinner` and `TestGradePostureMatrixFiveCheckpointsThreeModes` FAILed.
2. `outcome` mapping `ModeAsk` to `block` → 6 ledger tests, 4 `package main` tests and `TestSelftestOK` FAILed (including `TestGradeAskPromptAndExitZero` and `TestLedgerExitCodeContract`).

Both mutations were reverted; `gofmt -l` clean and `go test -count=1 ./...` green again.

**Coupling evidence (D4/A1):**
- `go list -deps ./ledger` lists only `ai-specs.dev/worktree-gate/ledger` (stdlib otherwise) — no gate `Decide`/`Event`/cleanup type is reachable.
- `grep -rn -E "Decide|Event|cleanup|gate-mode|GateMode|WORKTREE_GATE|PROTECTED|tokenize" ledger/*.go` matches only the package doc comment in `identity.go`; no code reference.
- `ledger_cmd.go` reads no worktree gate flag or env var; it only injects `gitMemo`/`gitCommon`/`RealPath`/`processCwd` as the shared Git-facts seam.

### Deviations from design (Unit 3 only)

1. **Conflict reason is `conflict`, not `needs-item`.** Task 3.1's parenthetical
   "(needs-item)" is pinned to the *missing item* case (design test-table row
   "bound + empty store + always + apply-start | block `needs-item`"). A disagreement or a
   two-open collision returns `reason=conflict` so the JSON `conflict` object and the
   doctor WARN have a matching reason. Both are `block`/exit `2` in `always`, ask/`0` in
   `ask`, allow/`0` in `warn`.
2. **Conflict resolution is checkpoint-scoped.** A persisted `adjudicate` (or `opt-out`)
   resolves only the checkpoint it names, mirroring D19 for opt-outs. The snapshot is
   cleared by `--decide`, so "current conflict" means unresolved at the current
   checkpoint. This makes the next checkpoint re-present the evidence rather than inherit
   a stale "resolved" bit.
3. **Local evidence side is the item snapshot.** The `--evidence` file supplies
   `remote`/`code`/`git`; `local` defaults to `item.item_id` (the design evidence table).
   The file may still override `local`, which keeps the predicate testable for
   "missing local vs present remote".
4. **Stored-slug recovery in the dispatcher.** `identity.go` cannot be edited in this
   slice's allowed surface, so the CLI re-derives the identity with `StoredSlug` taken
   from the single open item for `common_dir+branch`, preserving A11 when a change folder
   is archived mid-item. Two open items with different slugs are left to the collision
   path.
5. **Unknown checkpoint/mode are fail-open, not parse errors.** An unknown
   `--checkpoint` value returns `unevaluable`/`unknown-checkpoint`/exit `0` (doctor
   ERROR) instead of aborting; an unknown/empty `--ledger-mode` normalizes to `warn`.
   Actual flag-parse errors keep the existing launch fail-open (`warning … failing open`,
   exit `0`).
6. **Two-open collision has no `--decide` pick path.** `PersistConflict`/`PersistDecision`
   fail closed when the identity has two open items, so the collision stays a conflict
   for a human; this slice ships no silent or scripted pick (spec: "never as a silent
   pick"). A future slice can add an explicit adjudication payload.
7. **`--decide` is skipped while dormant.** With no active witness there is no item to
   attach a decision to, so `--decide` is a no-op and the grade stays `dormant`/exit `0`
   instead of failing closed. A bound ledger with no primary item still fails closed
   (exit `2`).

### Slice workload / PR boundary (Unit 3)

- Authored lines: **1,925** (1,901 new Go + 24 `main.go`), over the 400-line review
  budget. Expected under the maintainer-accepted `size:exception`; no comments, tests, or
  edge cases were dropped, and no restyling was done.
- Boundary: commit slice 3 of the single PR, and the first slice that changes the shipped
  binary. It is still inert without a witness (Unit 4): every checkpoint on a repo without
  `witness.json` grades `dormant`. Reverting `ledger_cmd.go` + the `main.go` hunk (and
  `verdict.go`/`decide.go`) restores the worktree gate exactly — proven by the
  old-vs-new `--explain` byte comparison.
- **Commit step not executed.** Task 3.7's `REFACTOR + commit` is complete except for the
  commit itself: the parent prompt explicitly forbids committing in this phase, so the
  slice boundary is left staged-but-uncommitted for the parent/PR owner. No commit, push,
  or merge was performed.

### Remaining tasks

Phase 4–7 (`4.1`–`7.7`) remain unchecked in
`openspec/changes/tracker-ledger-foundation/tasks.md`; the exact lines are listed in the
Unit 1 tail above from `- [ ] 4.1` onward. Python hosts were deliberately **not** wired
(Unit 5) and no witness producer was added (Unit 4), per the assigned boundary.

### Structured status consumed / produced (Unit 3)

Consumed: `artifactStore: openspec`, `actionContext.mode: repo-local`,
`allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli]`, worktree
`.worktrees/tracker-ledger-foundation` on `change/tracker-ledger-foundation`. The parent
supplied the Unit 3 assignment and the accepted `size:exception`; the `Review Workload
Forecast` gate is `Decision needed before apply: No`, `Chained PRs recommended: No`,
`400-line budget risk: High`, so implementation proceeded without a delivery pause.

**Status-engine discrepancy (warning, not a blocker):** the native status JSON was
computed at `planningHome.root = /Users/robert/proyectos/nnodes/ai-specs-cli` (the main
checkout), where `openspec/changes/` contains only `archive/` — the change folder lives
on the `change/tracker-ledger-foundation` branch inside this worktree. The status therefore
reported `changeRoot: null`, every artifact `missing`, and
`applyState/blockedReasons: "No active SDD changes found."`. For the `openspec` store the
authoritative inputs were read from
`.worktrees/tracker-ledger-foundation/openspec/changes/tracker-ledger-foundation/` on
disk (proposal/spec/design/tasks/apply-progress all present), which the parent prompt also
named explicitly. No edit left the allowed roots; no other `actionContext` warning fired.

## Unit 4 — Phase 4: durable witness (Python bridge)

Tasks 4.1–4.6 complete. Persisted checkboxes updated in
`openspec/changes/tracker-ledger-foundation/tasks.md` (`- [x]` for 4.1–4.6; the Phase 4
block now has zero unchecked rows, re-read after the edit). Units 5–7 were deliberately
**not** started: no checkpoint host reads the witness yet, and no old grader was retired.

### Files changed

| Path | Change |
|---|---|
| `lib/_internal/recipe-materialize.py` | +159: `git_common_dir`, `tracking_declared`, `tracker_witness_payload`, `write_tracker_witness` (A4) + two call sites |
| `tests/test_tracker_ledger_witness.py` | New (328): unit + integration tests for the four states, trap survival, atomicity, no-guess, linked worktree |
| `tests/fixtures/recipes/test-tracker-ledger/recipe.toml` | New (8): synthetic provider declaring `tracker` |
| `tests/fixtures/recipes/test-tracker-ledger-conflict/recipe.toml` | New (8): second `tracker` declarer for the ambiguous case |
| `openspec/changes/tracker-ledger-foundation/tasks.md` | Checkboxes 4.1–4.6 → `- [x]` |
| `openspec/changes/tracker-ledger-foundation/apply-progress.md` | This section (merged with Units 1–3) |

**503 authored lines** (159 + 328 + 16) plus 6 changed `tasks.md` lines. Over the 400-line
review budget by design: `tasks.md` declares `400-line budget risk: High` with a
maintainer-accepted `size:exception`; no comments, tests, or cases were dropped to fit.

### What Unit 4 implements

- **Path (A4/A3).** `git_common_dir(project_root)` mirrors the Go reader
  (`gitfacts.go::gitCommonWith`): `rev-parse --path-format=absolute --git-common-dir`,
  relative fallback, then `Path.resolve()` so a linked worktree and its main checkout agree.
  Witness target is `<git-common-dir>/ai-specs/ledger/witness.json`.
- **Producer stays single.** `resolve_bindings` is still the only binding producer; the new
  code only *persists* its map. `tracker_witness_payload` contains no verdict logic — the
  state is a lookup (`bound` when `tracker` is in the map), a candidate count, and a
  declaration flag.
- **Four states.** `bound` (recipe id recorded), `ambiguous` (two or more enabled declarers,
  `candidates` recorded, `recipe_id` empty — never a guess), `unbound` (no declarer, no
  declaration), `declared-not-bound` (`openspec/config.yaml` top-level `tracking:` exists but
  the capability is not bound).
- **Atomic write (A4).** `mkstemp` (`witness.json.tmp.*`) in the destination directory →
  `json.dump` + `flush` + `fsync` → `os.replace`; the temp file is unlinked on any failure.
- **Outside the trap.** The witness is written to the Git common dir, never to
  `RESOLVED_CONFIG_TEMP`; `lib/sync.sh` was not modified because it already deletes only its
  five named temps (task 4.4, asserted by a test).
- **Best-effort, fail-dormant.** Outside a repository there is no common dir and the write is
  skipped; an `OSError` warns and leaves the ledger dormant rather than aborting sync. No
  provider is inferred in any state.
- **Deactivation is a state, not a deletion.** A sync whose enabled set shrinks (or empties)
  overwrites a previously bound witness with the new outcome, so disabling the tracker recipe
  cannot leave a stale `bound` activation behind.

### Focused test command

```bash
python3 -m unittest tests.test_tracker_ledger_witness -v
# Ran 14 tests in 2.9s
# OK
python3 -m py_compile lib/_internal/recipe-materialize.py   # exit 0
```

Surrounding existing suites (all green, no new failures):

```bash
python3 -m unittest tests.test_recipe_materialize tests.test_recipe_conflicts \
  tests.test_sync_recipe_capture                       # Ran 106 tests  OK
python3 -m unittest tests.test_sync_pipeline tests.test_worktree_flow_recipe \
  tests.test_agents_render_brief_fragments             # Ran 171 tests  OK
python3 -m unittest tests.test_trello_mcp_workflow_recipe \
  tests.test_plan_build_flow_recipe tests.test_brief_render_policy   # Ran 56 tests  OK
```

Full `./tests/run.sh` (Go + unittest discovery): `Ran 1929 tests ... FAILED (failures=11,
skipped=2)`. **All 11 failures pre-exist at HEAD (Units 1–3) and none is caused by Unit 4** —
they are the known gate-asset digest/build state that task 7.2 owns (Unit 3 changed the Go
source without regenerating `catalog/recipes/worktree-flow/bin/SHA256SUMS`). Verified by
running the same modules in a detached worktree at `HEAD`: `Ran 17 tests ... FAILED
(failures=10)` in the two gate-asset modules plus `test_release_materialization`'s
digest-driven ERROR, i.e. the identical 11 before any Unit 4 edit. The temporary verification
worktree was removed afterwards.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 4.1 witness states | `tests/test_tracker_ledger_witness.py` | Unit | ✅ `unittest tests.test_recipe_materialize tests.test_recipe_conflicts tests.test_sync_recipe_capture` → 92/92 before any edit | ✅ `AttributeError: module 'recipe_materialize_witness' has no attribute 'git_common_dir'` + missing-file errors (9 errors, 2 failures of 12) | ✅ bound / ambiguous / unbound / declared-not-bound / no-provider-vocabulary / trap-survival / no-temp-residue / non-repo cases pass | ✅ explicit `[[bindings]]` beats two declarers → `bound`; disabling the bound recipe overwrites the witness → `unbound`; ambiguous candidates are both fixture ids with `recipe_id == ""` | ✅ `tracker_witness_payload` split from `write_tracker_witness`; shared `LEDGER_*`/`WITNESS_*` constants; `_witness_path`/`_witness` test helpers |
| 4.2 synthetic fixtures | fixtures + same test file | Unit | ✅ as above | ✅ fixtures absent before this change (catalog had no `test-tracker-ledger*`) | ✅ `test-tracker-ledger` → `bound`; `test-tracker-ledger` + `-conflict` → `ambiguous` | ✅ fixtures declare no skills/commands/templates, so they exercise binding only, not materialization | ✅ fixtures live under `tests/fixtures/recipes/` and are picked up by `populate_catalog`; `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES` gating unchanged |
| 4.3 atomic writer | same test file | Unit | ✅ as above | ✅ same RED run (`git_common_dir` undefined) | ✅ witness written at the common-dir path with `v=1`, `capability=tracker`, ISO-Z `written_at` | ✅ `witness.json.tmp.*` glob is empty after sync; a first call site accidentally placed inside the `cap_conflicts` loop was caught immediately by the bound test, and the call was moved to module scope before GREEN | ✅ single `write_tracker_witness` seam; both call sites pass the same already-resolved map |
| 4.4 sync.sh trap | same test file | Unit (static) + Integration | ✅ `lib/sync.sh` untouched | n/a (confirmation task) | ✅ `test_sync_sh_trap_never_names_the_witness` passes: trap line has `rm -f`, no `witness` token, `RESOLVED_CONFIG_TEMP=` appears exactly once | ✅ end-to-end `ai-specs init` + `ai-specs sync` with `trello-mcp-workflow` enabled leaves `bound`/`trello-mcp-workflow` at the common dir **after the real EXIT trap ran** | ✅ no `sync.sh` edit was needed; the design's "no witness trap, no second temp root" holds as written |
| 4.5 linked worktree | same test file | Integration | ✅ as above | ✅ same RED run | ✅ `git_common_dir(linked) == git_common_dir(main)`; the witness written from the main checkout is read back through the linked worktree's `.git` file | ✅ built `dist/worktree-gate-current --ledger --checkpoint apply-start --ledger-mode warn --project-root <linked>` returns `active=true`, `decision=allow`, `identity.branch=linked-branch` — the Go reader consumes Python's witness | ✅ assertion also pins that the witness is NOT inside the linked worktree's own `.git` file |
| 4.6 refactor + compile | — | — | ✅ 14/14 before refactor | n/a | ✅ 14/14 after | ✅ mutation check below | ✅ `python3 -m py_compile lib/_internal/recipe-materialize.py` exit 0 |

**Mutation check (proving the assertions are not vacuous):** changing the ambiguous branch
`state = WITNESS_AMBIGUOUS` → `WITNESS_UNBOUND` made
`test_ambiguous_witness_records_candidates_without_guessing` FAIL (and only it). The mutation
was reverted; all 14 tests are green again. A first, weaker mutation (`len(candidates) > 1` →
`candidates`) was discarded as equivalent under current inputs rather than reported as
evidence.

### Deviations from design (Unit 4 only)

1. **The witness is re-written on the zero-enabled-recipes path.** The design does not name
   this branch, but `materialize_recipes` returns early when no `[recipes.*]` is enabled. Not
   writing there would leave a previously `bound` witness activating a provider whose recipe
   was just disabled. The outcome is still a pure lookup of `resolve_bindings` ({} → `unbound`
   or `declared-not-bound`), so "resolve_bindings is the only producer" is preserved.
2. **Write failures warn instead of aborting sync.** The design enumerates missing witness as
   dormant and corrupt *store* as `unevaluable`; it does not fix a policy for witness IO
   failure. Best-effort + `warn` keeps sync resilient, and the Go reader already treats an
   unreadable witness as `witness-missing`.
3. **`declared-not-bound` reads `openspec/config.yaml` with a top-level `tracking:` line
   scan.** No YAML parser is vendored in this repo, and the design's "(or equivalent
   declaration)" allows the lighter check. Only the presence of the block is read; no field
   inside it is interpreted, so no provider vocabulary enters the core.
4. **`--resolved-config-only` (standalone `sync-agent`) does not write the witness.** That
   mode documents "no side effects", and the full sync path owns materialization. The design's
   flow names sync, not the standalone helper.
5. **`candidates` is populated only for `ambiguous`.** The spec requires candidate ids for
   that state; other states carry `[]`, keeping "no provider is guessed" visible in the file.
6. **Commit not executed.** Task 4.6's `REFACTOR + commit` is complete except the commit
   itself: the parent prompt forbids committing in this phase, so this slice is commit slice 4
   left uncommitted for the parent/PR owner. No commit, push, or merge was performed.

### Remaining tasks (exact unchecked `- [ ]` lines)

Phases 5–7 remain unchecked; the Phase 4 rows are now `- [x]`. The corresponding lines appear
in the raw `tasks.md`; this list records the exact 20 still-open rows.

```text
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

Checkpoint hosts (Unit 5), grader collapse (Unit 6), and docs/trust (Unit 7) were not
started, per the assigned boundary. No old grader was retired.

### Slice workload / PR boundary (Unit 4)

- Authored lines: **503** (159 `recipe-materialize.py` + 328 test + 16 fixtures), plus 6
  changed `tasks.md` lines — over the 400-line review budget, expected under the
  maintainer-accepted `size:exception`. Nothing was compressed or restyled to fit.
- Boundary: commit slice 4 of the single PR, and the first slice that writes production
  state at sync time. It is still behaviourally inert for checkpoints: no host reads the
  witness yet (Units 5–6), so the ledger stays dormant everywhere regardless of the new file.
  Reverting this slice restores the previous sync exactly — the only shipped-side addition is
  the writer, which nothing consumes yet.
- The Go side was not touched in this unit.

### Structured status consumed / produced (Unit 4)

Consumed (parent prompt + native status engine): `artifactStore: openspec`,
`actionContext.mode: repo-local`, `allowedEditRoots:
[/Users/robert/proyectos/nnodes/ai-specs-cli]`, worktree
`.worktrees/tracker-ledger-foundation` on `change/tracker-ledger-foundation`, accepted
`size:exception`. Every write stayed inside the five allowed surfaces:
`lib/_internal/recipe-materialize.py`, `tests/test_tracker_ledger_witness.py`, the two
`tests/fixtures/recipes/test-tracker-ledger*/recipe.toml` files, and the two
`openspec/changes/tracker-ledger-foundation/**` artifacts. No `actionContext` warning fired
and no edit left the allowed roots.

**Status-engine discrepancy (unchanged, warning only):** the native status JSON was computed
against the main checkout's `planningHome.root`, where the change folder exists only on this
branch inside the worktree, so it again reported every artifact `missing` and
`applyState: blocked` with reason "No active SDD changes found.". For the `openspec` store the
authoritative inputs were read from
`.worktrees/tracker-ledger-foundation/openspec/changes/tracker-ledger-foundation/` on disk
(proposal/spec/design/tasks/apply-progress all present). No real blocker was reported.

Produced: this progress artifact and the persisted `- [x]` marks for 4.1–4.6.

## Unit 5 — Phase 5: checkpoint hosts

Tasks 5.1–5.7 complete. Persisted checkboxes updated in
`openspec/changes/tracker-ledger-foundation/tasks.md` (`- [x]` for 5.1–5.7; the Phase 5
block now has zero unchecked rows, re-read after the edit with `grep -n "^- \[.\] 5\."`).
Units 6–7 were deliberately **not** started: no legacy grader was retired (doctor /
`trello_link`), and no parity corpus or docs/trust work was done.

### Files changed

| Path | Change |
|---|---|
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | `[config.ledger_mode]` (enum `always|ask|warn`, default `warn`); hook descriptions + `gate_mode` help text updated |
| `catalog/recipes/trello-mcp-workflow/README.md` | Config table row, gate-mode table rewrite, `## Tracker`-as-presentation note |
| `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | Heredoc `## Tracker` grader deleted; path→`apply-start`, shell `pr_create`→`pr-review` through the verified binary; A9 mode; ask prompt + `--decide` |
| `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` | `work-start` checkpoint before the artifact gate; A9 mode; same binary bridge |
| `catalog/recipes/git-pr-flow/commands/pr-create.md` | `pr-review` precondition + step note |
| `lib/_internal/premerge_guardian.py` | `resolve_ledger_mode`, `_ledger_binary`, `ledger_blockers`, `_ledger_ask`; `main` invokes `archive-close`/`pre-merge` and merges blockers |
| `tests/test_ledger_mode_config.py` | New: A9 mapping, recipe schema, five-host parity, no-predicate grep-assert |
| `tests/test_tracker_card_gate_hook.py` | Rewritten: stub-binary bridge, `gh pr create` tokenizer matrix, archive commands not gated, bash 3.2 |
| `tests/test_plan_build_gate_hook.py` | +5 work-start bridge tests (no change folder required, mode plumbing, fail-open) |
| `tests/test_premerge_guardian.py` | +5 guardian bridge tests (A9, both stages, cold home, tier math unchanged) |
| `openspec/changes/tracker-ledger-foundation/tasks.md` | Checkboxes 5.1–5.7 → `- [x]` |
| `openspec/changes/tracker-ledger-foundation/apply-progress.md` | This section (merged with Units 1–4) |

**~1,362 authored lines** (`git diff --stat`: 1,027 additions across nine files + 335 new
test lines), 784 deletions. Over the 400-line review budget by design: `tasks.md` declares
`400-line budget risk: High` with a maintainer-accepted `size:exception`; nothing was
compressed or restyled to fit.

### What Unit 5 implements

- **One predicate, five hosts.** `plan-build-gate.sh` grades `work-start`;
  `tracker-card-gate.sh` grades `apply-start` (path) and `pr-review` (shell `pr_create`);
  `premerge_guardian.py` grades `archive-close` (`--stage pre-archive`) and `pre-merge`.
  Every host runs the same verified `worktree-gate --ledger` binary and maps the JSON
  verdict; the guardian renders blockers through its existing `GuardianResult`.
- **A9 mode (5.1/5.2).** `[config.ledger_mode]` (`always|ask|warn`, default `warn`) wins;
  otherwise the tracker `gate_mode` maps `off`→skip, `warn`→`warn`, `always`→`always`. The
  worktree `gate_mode` is never read. `TRACKER_LEDGER_MODE` is the one-shot override.
  Because Unit 5's allowed surfaces exclude `recipe-materialize.py`, the mode is resolved at
  runtime from `ai-specs/ai-specs.toml` (a config read, not a predicate) rather than stamped.
- **Heredoc grader removed (5.4).** `_eval_deficient`, `is_valid_link`, `marker_present`,
  `_emit_and_exit`, and the `## Tracker`/`tracker.none` validity copy are gone from the
  tracker hook. The tokenizer stays (shell acquisition); its archive/`mv` branches were
  deleted because `archive-close` now belongs to the guardian.
- **Ask + `--decide` (5.3/5.4/5.5).** `decision=ask` prints the four evidence sides and the
  legal choices; a `[y/N]` opt-out read from `/dev/tty` is persisted with
  `--decide {"checkpoint":…,"kind":"opt-out","choice":"continue"}`, which also re-grades.
  With no usable terminal the host proceeds (fail open) and records nothing.
- **Fail-open acquisition.** A missing/unverified binary, unparseable JSON, or IO error maps
  to exit `0` and silence; the binary's own flag/parse errors fail open. `openspec/**` never
  reaches the ledger in either path host. The guardian's missing-binary path prints one
  stderr line and adds no blocker.
- **Guardian math untouched (5.5).** The ledger invoke is merged in `main` after
  `check_prearchive`/`check_premerge`; tier minima and verify-evidence checks are unchanged
  (`test_tier_math_unchanged_when_ledger_allows` plus the full existing suite).

### Focused test commands

```bash
python3 -m unittest tests.test_ledger_mode_config tests.test_tracker_card_gate_hook \
  tests.test_plan_build_gate_hook tests.test_premerge_guardian
# Ran 115 tests ... OK
python3 -m unittest tests.test_trello_mcp_workflow_recipe tests.test_plan_build_flow_recipe \
  tests.test_git_pr_flow_recipe tests.test_hooks_render tests.test_recipe_schema \
  tests.test_tracker_ledger_witness tests.test_eval_hook_wiring
# Ran 263 tests ... OK
bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh \
  && bash -n catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh   # exit 0
python3 -m py_compile lib/_internal/premerge_guardian.py tests/test_ledger_mode_config.py
```

Full `./tests/run.sh`: `Ran 1934 tests ... FAILED (failures=11, skipped=2)`. The 11
failures are the same pre-existing gate-asset digest/build failures recorded in Unit 4
(`test_worktree_gate_release_phase4` ×9, `test_release_materialization` ×1,
`test_worktree_root_propagation` ×1); task 7.2 owns regenerating
`catalog/recipes/worktree-flow/bin/SHA256SUMS`. Unit 5 touches no Go source and none of the
11 files involved. The Go half of `run.sh` is green (`ok ai-specs.dev/worktree-gate`,
`ok ai-specs.dev/worktree-gate/ledger`).

### TDD Cycle Evidence

Strict TDD is active (`openspec/config.yaml` `strict_tdd: true`, runner `unittest`), so
every task ran RED → GREEN → TRIANGULATE → REFACTOR. Because Unit 5 wires hosts that did not
call the ledger at all, the RED runs are genuine behaviour failures, not missing symbols.

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 5.1 mode mapping | `test_ledger_mode_config.py` | Unit + host | `unittest test_plan_build_gate_hook test_premerge_guardian test_tracker_card_gate_hook` → 110/110 before any edit | 15 failures: `ledger_mode` field absent; hosts never spawned the binary (`_logged_checkpoints() == []`) | `test_recipe_declares_ledger_mode_enum_and_default` + 6 mapping tests pass | ledger-wins vs gate off/warn/always, env override, worktree-mode-ignored, tracker-host mapping | Runtime manifest read adopted instead of a new stamp (see deviations) |
| 5.2 config | `test_ledger_mode_config.py`, `test_trello_mcp_workflow_recipe` | Unit | same 110/110 | same RED run | recipe + README + help text in place; schema test green | enum/default pinned; README table row pinned via recipe schema | `gate_mode` help text demoted to legacy vocabulary |
| 5.3 work-start | `test_plan_build_gate_hook.py` | Host (subprocess) | 31 existing plan-build tests green before edit | `test_work_start_grades_production_write` failed (no checkpoint logged) | 5 new tests pass | allow/block stub, mode plumbing (`--ledger-mode always`), `openspec/**` never graded, no change folder required, cold home fail-open | Ledger call moved **before** the artifact gate so a missing change folder cannot skip the checkpoint |
| 5.4 tracker host | `test_tracker_card_gate_hook.py` | Host (subprocess) | old 36-test file green at HEAD; rewrite keeps tokenizer matrix | rewritten suite against the old hook: path/shell never graded, grader tokens still present | 19 tests pass | `apply-start`/`pr-review` records, allow/block/warn/ask, marker-free activation, mode off, missing binary, archive commands not gated, 62-case `gh pr create` tokenizer matrix, bash 3.2 | Grader + archive branches deleted; `/dev/tty` open checked before prompting |
| 5.5 guardian | `test_premerge_guardian.py` | CLI (subprocess) + unit | 43 existing guardian tests green before edit | `test_prearchive_grades_archive_close` / `test_premerge_grades_pre_merge` failed (no ledger invoke) | 5 new tests pass | A9 resolver table, both stages, cold `AI_SPECS_HOME` fail-open, tier math unchanged | Ledger merged in `main`, not inside `check_*`, so existing direct-call tests stay pure |
| 5.6 triangulate | `test_ledger_mode_config.py` | Integration | 115 focused tests green | n/a (verification task) | all five hosts block on a `block` verdict and allow on `allow` | one stub binary, five hosts, five distinct checkpoints logged once each: `apply-start, archive-close, pr-review, pre-merge, work-start` | Guardian exits `1` (its existing convention) where the hooks exit `2` — block/allow semantics identical |
| 5.7 refactor | `test_ledger_mode_config.py` | Static | — | `test_hosts_add_no_tracker_predicate` failed on the old tracker hook (`is_valid_link`, `RECOGNIZED`, `card_id`, `## Tracker`) | grep-assert green across both hooks and the guardian | `pr-create.md` points at the `pr-review` verdict | no shared bridge file exists in the allowed surfaces; the four bridge helpers are duplicated per host with a comment (see deviations) |

**Mutation checks (proving the assertions are not vacuous):**

1. In `tracker-card-gate.sh`, flipping the `apply-start` argument to `pr-review` made
   `test_prod_write_grades_apply_start` and the 5.6 checkpoint-set assertion fail.
2. In `premerge_guardian.py`, returning `[]` from `ledger_blockers` unconditionally made
   `test_prearchive_grades_archive_close`, `test_premerge_grades_pre_merge`, and both 5.6
   block subtests fail.
3. In `plan-build-gate.sh`, moving `_ledger_work_start` back after the artifact gate made
   `test_work_start_grades_without_a_change_folder` fail.

All three mutations were reverted; the focused suite is green again.

### Deviations from design (Unit 5 only)

1. **Mode is resolved from the project manifest at runtime, not stamped.** Design A9 places
   `ledger_mode` in recipe config, and `recipe-materialize.py` is outside this unit's allowed
   edit surface, so no new placeholder could be stamped. The hosts read
   `[recipes.trello-mcp-workflow.config]` from `ai-specs/ai-specs.toml` (with the tracker
   `gate_mode` as the A9 fallback) and honor `TRACKER_LEDGER_MODE`. This is still a config
   read plus an enum choice, not a ledger predicate. A later unit may stamp it for hot-path
   speed.
2. **Archive shell actions are no longer gated by the tracker hook.** The design assigns
   `archive-close` to `premerge_guardian.py --stage pre-archive`, and task 5.4 names only
   `apply-start` and `pr-review`, so the tokenizer's `openspec archive` / `ai-specs archive` /
   `mv` / `git mv` branches were deleted rather than left dead. The shell parser is otherwise
   unchanged, so the `gh pr create` false-positive matrix still guards heredocs and comments.
3. **`WORKTREE_GATE_BIN` override.** To keep the hosts hermetic and debuggable — and to let
   the bridge be tested without a released asset — all three hosts accept an explicit
   executable override before the version-keyed cache. It mirrors `worktree-gate.sh`.
4. **Cache binaries need the `.verified` receipt; the override does not.** Hosts never execute
   an unverified cache candidate (mirrors `worktree-cleanup.sh`). The explicit override is a
   debugging/test pin, like `WORKTREE_GATE_BIN` in the worktree launcher.
5. **`ask` reads `/dev/tty`; no tty means fail open.** A hook's stdin is the event JSON, so
   the prompt uses the controlling terminal. When it cannot be opened the host prints the
   evidence and proceeds without recording a decision (consistent with the fail-open blast
   radius); a human declining the opt-out blocks.
6. **The bridge helpers are duplicated per host.** The allowed surfaces contain no shared
   shell/Python file, so `_ledger_mode`/`_ledger_binary`/`_ledger_field`/`_ledger_ask`,
   and the guardian's equivalents, are repeated with a comment. Only the config read and the
   JSON/exit mapping are duplicated; the verdict predicate stays exclusively in Go.
7. **`unevaluable`/`dormant` do not block the guardian.** Design says infra unevaluable fails
   open with a doctor ERROR, so the guardian prints a warning and adds no blocker.
8. **Commit not executed.** Task 5.7's `REFACTOR + commit` is complete except the commit:
   the parent prompt forbids committing, so this slice is commit slice 5 left uncommitted for
   the parent/PR owner. No commit, push, or merge was performed.

### Remaining tasks (exact unchecked `- [ ]` lines)

Phases 6–7 remain unchecked; the Phase 5 rows are now `- [x]`. The 17 still-open rows:

```text
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

Unit 6 (one grader: doctor/`trello_link` retirement + parity corpus) and Unit 7 (docs, trust,
close-out) were not started, per the assigned boundary. No old grader was retired.

### Slice workload / PR boundary (Unit 5)

- Authored lines: **~1,362** (1,027 additions across the four host/docs files and the three
  edited test files + 335 new `test_ledger_mode_config.py`), 784 deletions, over the 400-line
  review budget. Expected under the maintainer-accepted `size:exception`; no comments, tests,
  or edge cases were dropped, and no restyling was done.
- Boundary: commit slice 5 of the single PR, and the slice that makes checkpoints
  behaviourally live. It depends on Units 3–4 (verdict + witness) and is the first slice any
  user-facing hook runs. Reverting the four host files restores the pre-ledger hosts; the
  heredoc grader returns only with the revert (which is the intended rollback).
- The Go side was not touched in this unit.

### Structured status consumed / produced (Unit 5)

Consumed: `artifactStore: openspec`, `actionContext.mode: repo-local`,
`allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli]`, worktree
`.worktrees/tracker-ledger-foundation` on `change/tracker-ledger-foundation`, accepted
`size:exception`. Every write stayed inside the ten allowed surfaces (two recipe files, two
hooks, `pr-create.md`, the guardian, the four test files, and the two
`openspec/changes/tracker-ledger-foundation/**` artifacts). No `actionContext` warning fired
and no edit left the allowed roots.

**Tooling note (deviation, resolved):** a first attempt at rewriting `tracker-card-gate.sh`
with a `bash` heredoc ran a Python splice and was correctly refused by the worktree gate
("refusing shell command that writes … on protected branch 'development'"). The split was
completed with the structured `edit`/`write` tools only; the fallback heredoc was never used.

**Status-engine discrepancy (unchanged, warning only):** the native status JSON was computed
against the main checkout's `planningHome.root`, where this change folder exists only on this
branch inside the worktree, so it again reported every artifact `missing` and
`applyState: blocked` with reason "No active SDD changes found.". For the `openspec` store the
authoritative inputs were read from
`.worktrees/tracker-ledger-foundation/openspec/changes/tracker-ledger-foundation/` on disk.
No real blocker was reported; the forecast gate is `Decision needed before apply: No`,
`Chained PRs recommended: No`, `400-line budget risk: High` with the recorded
`size:exception`, so implementation proceeded without a delivery pause.

Produced: this progress artifact and the persisted `- [x]` marks for 5.1–5.7.


