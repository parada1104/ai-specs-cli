# Go 07 S13 PR1 — Bindings and the durable tracker witness

## Objective
Expose the gate's `--resolve-bindings` decision core (capability binding resolution,
capability conflict grading and the durable tracker binding witness) through the
importable shared package, and add the root-side native binding authority that calls it.
Preserve the gate JSON command contract byte for byte.

This is PR1 of slice S13 (`odd/tasks/go-07-sync-plan.md:463`), split by the human into
three chained PRs:

1. **PR1 (this one)** — bindings + durable tracker witness.
2. PR2 — merge-config + resolved-config.
3. PR3 — hooks (`execute_hooks`, `_resolve_provider_markers`) + recipe-mcp emission.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no
  card could be created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report
  and proceed without recording a decision. `tracker.none` is presentation and evidence
  only and is never written by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope
- Base: `36b5405` on `epic/go-single-binary` (S12.2 A/B and S12.3 A/B merged, S12
  follow-up register merged).
- Worktree: `.worktrees/go-07-s13-bindings`. Branch: `change/go-07-s13-bindings`.
- Move the bindings decision core and the witness writer out of package main into
  `catalog/recipes/worktree-flow/gate/shared/`, leaving the main package as a thin
  wrapper with the historical names aliased so the existing gate tests compile unchanged.
- Add the root-side native authority `internal/sync/bindings.go` on top of the shared core.
- Regenerate only the four canonical gate checksum entries that the extraction requires.

## In-scope units (Python seam, `lib/_internal/recipe-materialize.py`)
- bindings: `GO_BINDINGS_BRIDGE_FALLBACK` :2340, `BindingResolutionError` :2355,
  `binding_error_code` :2369, `go_binding_resolution` :2416, `binding_resolution` :2537,
  `resolve_bindings` :2578, fallback `_python_resolve_bindings` :2614.
- witness: `git_common_dir` :2677, `tracking_declared` :2705, `tracker_witness_payload`
  :2721, `write_tracker_witness` :2763.
- Gate core: `gate/bindings.go` (271 L, `runResolveBindings` :224) plus
  `gate/witness_write.go` (7.8 K) and the `RealPath`/`gitCommon` helpers they use.

## Constraints
- Budget: **1548 additions plus deletions, approved by the human for this slice** (documented
  estimate was 1200). The measured churn is structural: counting both sides of the ~425
  moved lines double-counts ~850, and the 491 new test lines are the mandated, non-weakenable
  net. The semantic delta (code that is new rather than relocated) is ~700.
- Never remove or weaken tests to fit the budget. The extraction is a refactor whose
  checks are the existing gate tests plus the new root differential, so no new RED cycle
  is claimed for the extraction itself.
- The gate JSON contract is unchanged: the same stdin fields, the same stdout fields and
  order, resolution errors stay data with exit 0, and process-level failures stay exit 2.
- The witness bytes stay byte-identical to the Python writer
  (`json.dump(indent=2, sort_keys=True)` plus one trailing newline) and the write stays
  best-effort: no repository means no write and no warning.
- No production Python change, no CLI wiring (the root authority is added, not called from
  the pipeline; S14 owns the wiring), no dependency change.
- The root module imports the shared core through the existing
  `replace ai-specs.dev/worktree-gate => ./catalog/recipes/worktree-flow/gate` pin.
- Do not modify `development` and do not publish a release.
- Parent owns the commit, push, PR, merge and cleanup.

## Tasks
- [ ] T1: Extract the bindings decision core into `gate/shared/bindings.go` without
      behavior change, leaving a thin main package wrapper and the historical aliases.
- [ ] T2: Extract the witness writer (`WriteTrackerWitness`, `TrackerWitnessPayload`,
      `MarshalTrackerWitness`, `WitnessWrittenAt`, `TrackingDeclared`) and the
      `RealPath`/`GitCommonDir` resolution rule into the shared package, with the main
      package delegating so every existing call site and test keeps compiling.
- [ ] T3: Add `internal/sync/bindings.go`: the root boundary types plus
      `ResolveBindings`, delegating to the shared core and performing the best-effort
      witness write the Python authority performs.
- [ ] T4: Add the root differential and state-parity tests (`internal/sync/bindings_test.go`
      plus a `testdata` Python reference driver) mirroring the hookactuator pattern.
- [ ] T5: Regenerate the four `SHA256SUMS` digest lines with go1.24.13 and verify them
      with `scripts/verify-gate-sums.sh`.

## Acceptance criteria
- Gate stdin/stdout envelope fields, JSON order and exit codes remain unchanged.
- Resolution error strings, conflict severities and the warning string stay byte-identical.
- The witness payload, its key order, its indentation and its atomic temp+rename write are
  unchanged, and the write stays best-effort and dormant outside a repository.
- Existing gate tests pass against the shared implementation without edits.
- The root differential matches the Python fallback body (`_python_resolve_bindings`) and
  the Python witness bytes for every scenario the existing bridge tests cover.
- The four trust-root digests match a rebuild through `scripts/build-gate.sh` under go1.24.13.
- The complete authored candidate stays within the 1200-line budget.

## Checks
- Gate: `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` and `go vet ./...` there.
- Bridge: `python3 -m unittest tests.test_binding_bridge` and any witness test.
- Root: `go test ./cmd/... ./internal/...`, `gofmt -l`, `git diff --check`.
- Trust root: `scripts/build-gate.sh` plus `scripts/verify-gate-sums.sh`.
- Full: `./tests/validate.sh` with durable logs and captured exit evidence.
- Environment: the Go tests must run with the real toolchain and the Python 3.14.7 oracle
  prefixed on PATH (`~/.local/share/mise/installs/{go/1.24.13,python/3.14.7}/bin`), never
  through the mise shims.
- Native review uses the exact committed slice base `36b5405`, not the whole epic.

## Progress
- Slice prepared: worktree `change/go-07-s13-bindings` created at base `36b5405`; the epic
  worktree is clean and in sync with `origin/epic/go-single-binary`.
- Toolchain verified before any edit: `go1.24.13`, Python `3.14.7`, bash `5.3.20`.
- Split decided by the human: three chained PRs; this document covers PR1.
- T1-T5 implemented by a delegated writer, measured 1548 additions plus deletions
  (tracked 467: 73 added and 394 deleted across four slimmed wrappers plus the checksum file;
  untracked 1081: shared/bindings.go 214, shared/witness.go 190, shared/gitcommon.go 45,
  shared/realpath.go 39, internal/sync/bindings.go 102, internal/sync/bindings_test.go 417,
  testdata/bindings_ref.py 74). No gate test file was modified.
- Writer checks: gate suite `ok` (42.1s) and `go vet` clean; root module `ok`; `gofmt -l` and
  `git diff --check` clean; `tests.test_binding_bridge` 28/28 OK; `tests.test_tracker_ledger_witness`
  14/14 OK; four digests regenerated with go1.24.13 and `scripts/verify-gate-sums.sh` reported
  `ok — 4 digest entries match the committed trust root`.
- Design deviation recorded by the writer: the root acquisition swallows a recipe that the loader
  rejects, because the Python oracle `_python_resolve_bindings` and the gate's
  `loadRecipeCapabilities` both swallow (only `conflicts_core` propagates).
- Budget overage accepted by the human: 1548 against the 1200 estimate, see Constraints.
- Independent verification: **PASS** on all seven falsifiable claims (extraction fidelity,
  gate contract, root parity, witness bytes/best-effort, trust root, hygiene, budget). The
  verifier re-derived every moved body against `git show 36b5405:<path>` (identical apart
  from package/visibility renames), ran the gate and root suites itself, rebuilt the gate
  from a fresh directory with go1.24.13 and reproduced all four digests, and constructed
  three scenarios the suite does not cover (explicit binding to an unreadable recipe, two
  declarers with one unreadable, explicit binding to a loader-rejected recipe) comparing the
  root authority with the real Python oracle — no divergence on bindings or errors. Gate
  `func Test` count unchanged at 352; no gate test file modified; `internal/sync/sync.go:329`
  still execs Python, so the new authority is intentionally unwired (S14).
- Full validation: `./tests/validate.sh` reported exit 0 with `Ran 2482 tests in 1609.092s`
  then `OK`, and zero `FAIL:`/`ERROR:` lines. Durable evidence:
  `~/.cache/s13-pr1-validate.log` and `~/.cache/s13-pr1-validate.exit`.
- Work unit commit: `3792313` — `feat(sync): add the native binding authority over the shared
  gate core` (13 files, 1319 insertions and 394 deletions; the slice's authored churn without
  this document is 1548). Native review candidate for this slice; base `36b5405`.
- Native review: lineage `review-4244d619b602f106`, tier **high**, four lenses (risk, resilience,
  readability, reliability); the risk reason was `process_boundary` — `gate/shared/gitcommon.go`
  starts processes. The four-lens group capture failed once on `review-resilience`
  (`reviewer-empty-output`, `stopReason: length`, 604 s, zero submissions, no mutation). Per the
  provider instruction a fresh STATUS reoffered the same four slots; the three healthy lenses were
  captured one slot at a time and the `review-resilience` single-slot retry then emitted, closing the
  capture as **approved** on the last admitted event with **15 non-blocking advisories** (none opened
  a correction and no correction transition was offered). Authority burned with
  `gentle-ai.review-acknowledged/v1`; delivery stays under ordinary repository policy.
  Note for later slices: the `review-resilience` profile is `nan/glm5.3-flash`, which exhausted its
  32768-token output budget on the 94 KB prompt when run inside the four-lens group and only
  emitted on the isolated retry.

## Follow-ups (verifier findings)
These are recorded, not fixed here: each is either pre-existing in the moved code, inert on current
paths, or explicitly deferred to S14/S16. This register is authoritative and is reconciled with the
approved review advisories; the native ids are the authority and the verifier initials map onto them.

- `R3-witness-nonascii-bytes` (SUGGESTION, `catalog/recipes/worktree-flow/gate/shared/witness.go:104-118`)
  — was verifier **V1**. Witness bytes diverge from Python for non-ASCII values: Python
  `json.dump(..., ensure_ascii=True)` escapes as `\uXXXX` while the Go writer emits raw UTF-8
  (demonstrated with `café`). Pre-existing in the moved code, ASCII-only fixtures, no test covers it;
  needs a test before any fix.
- `R3-unmemoized-gitcommon` and `R4-witness-unmemoized-gitfact` (SUGGESTION,
  `catalog/recipes/worktree-flow/gate/shared/witness.go:128-147`) — was verifier **V2**. The witness
  write bypasses the invocation git-fact memo (`gate/gitfacts.go:13-19` states the contract).
  Verdict-neutral and inert on the `--resolve-bindings` path, where no memoized fact precedes the
  write. Fix shape if wanted: forward the fact function as a value, as the S12.3-A refresh seam does.
- `R3-conflict-parity-skipped` (SUGGESTION, `internal/sync/bindings_test.go:138-152`) — was verifier
  **V3**. Conflict-grader parity is asserted only where the Python grader does not raise;
  `_python_check_capability_conflicts` has no try/except on an unreadable recipe. Pre-existing
  gate-vs-Python divergence; the test documents it and claims no silent pass.
- `R3-empty-projectroot-witness` (WARNING, `internal/sync/bindings.go:57-64`) and
  `R4-project-root-unguarded` (SUGGESTION, `internal/sync/bindings.go:35-41`) — was verifier **V4**.
  The root `BindingRequest` does not default `ProjectRoot`; `gate/bindings.go` defaults an empty root
  to `.`, so an S14 wiring mistake that leaves it empty would read `openspec/config.yaml` from the cwd
  and silently skip the witness write. Python parity holds; S14 must set it explicitly.
- `R4-acquisition-skip-unobservable` (SUGGESTION, `internal/sync/bindings.go:86-94`) — a skipped
  unreadable recipe is not observable in the result, so a caller cannot tell a silent skip from a
  recipe that declared nothing.
- `R4-gitfact-failure-collapse` (SUGGESTION,
  `catalog/recipes/worktree-flow/gate/shared/gitcommon.go:38-45`) — a failed git fact collapses to the
  empty string, so "no repository" and "git failed" are indistinguishable.
- `R4-witness-crash-stale-revert` (SUGGESTION,
  `catalog/recipes/worktree-flow/gate/shared/witness.go:185-189`) — a crash between the temp write and
  the rename leaves a stale temp file behind.
- Readability advisories: `R2-001` and `R2-002` (WARNING, `internal/sync/bindings_test.go:158-167`,
  `:175-178`), `R2-005` (SUGGESTION, `internal/sync/bindings_test.go:312-318`), `R2-003`
  (SUGGESTION, `odd/tasks/go-07-s13-bindings.md:152` — a line of this document), `R2-004`
  (SUGGESTION, `catalog/recipes/worktree-flow/gate/gitfacts.go:101`) and `R2-006` (SUGGESTION,
  `catalog/recipes/worktree-flow/gate/shared/witness.go:55-69`).

Minor, pre-existing and not separately tracked: `TrackingDeclared` splits on `\n` where Python uses
`splitlines()`, and Python `read_text()` can raise `UnicodeDecodeError` outside its `except OSError`.

S13 PR2 and PR3 must not fix these opportunistically; they are later work, and the approved review
for this candidate is not reopened by them.

## Rollback
Revert this PR's gate extraction, its four checksum entries, the root authority, the tests
and this document together. PR2 must not integrate before this shared API exists.
