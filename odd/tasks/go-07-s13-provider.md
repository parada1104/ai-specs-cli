# Go 07 S13 PR4b-1 — Shared native provider resolver

## Objective
Extract the doctor-local port of `provider_install.resolve_provider` into a shared root package
(`internal/provider`) that is faithful to the Python authority, and switch `internal/doctor` to it.
PR4b-2 (`build_recipe_mcp` plus `_resolve_provider_markers` under `internal/sync/`) consumes this
package next.

PR4b was split after measurement: the combined estimate (~950-1,100 raw lines) exceeds the slice
budget, and this extraction de-risks the trust-check seam that PR4b-2 depends on.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is never written by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope
- Base: `24c4ca1` on `epic/go-single-binary` (S13 PR1, PR2a, PR2b, PR3 and PR4a merged).
- Worktree: `.worktrees/go-07-s13-provider`. Branch: `change/go-07-s13-provider`.
- **No gate change**: doctor-only refactor plus a new root package; `SHA256SUMS` must not change.

## Python authority (`lib/_internal/provider_install.py`)
`resolve_provider` :382-437 and its helpers: `detect_platform` :85-104, `expected_binary_name`
:106-108, `VERSION_RE` :39, `_version_tuple` :311-314, `_version_at_least` :316-320, `_run_version`
:322-338, `cache_root` :340-348 (argument, then `AI_SPECS_HOME`, then `~/.cache/ai-specs`, then
`expanduser().resolve()` / `cache/bin/jinna`), `managed_binary_path` :350-352, `_read_receipt`
:354-361, `_managed_candidates` :363-380, `_sha256` :192-198. Doctor caller:
`lib/_internal/dep_check.py:_check_one` :117-160 (an exception maps to
`detail="provider check failed: <exc>"`, `source="unresolved"`).

## Known gaps of the doctor-local port (`internal/doctor/depcheck.go:282-539`)
1. The result lacks `binary`, `command` (bare binary for `path`, absolute candidate for `managed`),
   `repository` and `release_tag`; PR4b-2 needs `command`.
2. `""` binary is defaulted to `jinna` where Python keeps a set-but-empty value.
3. `cache_root` does not resolve symlinks and has no `AI_SPECS_HOME`/default fallback.
4. The final winner re-runs `_run_version` outside any `try` in Python, so its failure raises; the Go
   port returns `unresolved` silently instead of the error.
5. Error strings differ (timeout and OS-error wording) from Python's `InstallError` messages.
6. Candidate order: Python `listdir` order vs sorted `os.ReadDir` (observable only on version ties).
7. Version tuple overflow maps to 0 in Go (arbitrary precision in Python).

## Tasks
- [x] T1: Create `internal/provider` with a Python-faithful `Resolve` (result struct with every
      field, error return mirroring `InstallError`, `CacheRoot` with the env/default fallback and
      symlink resolution), closing gaps 1-5 and pinning 6-7 as documented divergences.
- [x] T2: Switch `internal/doctor` to the shared package, deleting the local helpers, keeping the
      `depResult` mapping of `dep_check._check_one` (including `provider check failed: <exc>`), and
      moving/keeping the existing doctor tests green.
- [x] T3: Focused tests for the shared package (path source, managed source with verified receipt,
      receipt rejections, min_version, winner re-check failure, cache_root fallbacks), with a Python
      differential driver under `internal/provider/testdata/` importing the real `resolve_provider`.
- [ ] T4: Full check set with real exit codes; native review; PR and squash-merge to the epic.
      Checks done (exit 0 for every set), review approved and its authority burned; the PR and the
      squash-merge to `epic/go-single-binary` are the remaining closing step.

## Acceptance criteria
- `internal/provider.Resolve` matches the Python authority on every covered scenario: result fields,
  error strings, and the managed `command` path (symlinks resolved).
- `internal/doctor` produces the same dep-check output as before for passing cases and now reports
  `provider check failed: <exc>` where Python does.
- `SHA256SUMS`, the gate module, `internal/sync` and production Python are untouched.
- The authored candidate stays within the budget (700 estimated; 1441 measured and accepted by the human).

## Checks
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l`, `go vet ./internal/...`, `git diff --check`.
- Regression: `python3 -m unittest tests.test_jinna_provider_recipe tests.test_dep_check` (if present).
- Full: `./tests/validate.sh` with durable logs (`~/.cache/s13-pr4b1-validate.{log,exit}`), gate binary
  built first with `bash scripts/build-gate.sh`.
- Native review over committed base `24c4ca1`.

## Progress
- Slice prepared: worktree and branch created at base `24c4ca1`; scope mapped by a read-only scout.
- Implemented by a delegated writer (T1-T3): `internal/provider/provider.go` (501),
  `internal/provider/provider_test.go` (592), `internal/provider/testdata/provider_ref.py` (78);
  `internal/doctor/depcheck.go` +24/-225, `internal/doctor/depcheck_test.go` -21. Measured 1441 authored
  lines against the 700 estimate; the human accepted the overage (in line with PR1/PR2a).
- Gaps 1-5 closed; divergences pinned with direction: candidate order (sorted `os.ReadDir` vs
  `listdir`), version-tuple overflow (saturates at MaxInt64), Unicode `\d` digits (Python accepts Nd).
- TDD RED not captured (port and tests authored together); expected outputs are pinned to the Python
  authority by the differential driver instead.
- Writer checks: provider/doctor tests, `go test ./cmd/... ./internal/...`, `gofmt`, `go vet`,
  `git diff --check`, `tests.test_jinna_provider_recipe` (66 OK), `tests.test_dep_check` (12 OK): all exit 0.
- Independent verification: PASS. Nine scenarios outside the suite run against the real Python
  `resolve_provider` and the port (symlinked `AI_SPECS_HOME`, three managed versions with `min_version`,
  wrong receipt sha, PATH below minimum with managed fallback, non-executable candidate, PATH version
  failure with fallback, env fallback, missing receipt): all match; the tie-break probe diverges exactly
  as pinned. Four `DepResult` scenarios match `dep_check._check_one` on all eleven fields.
- Full validation with the gate binary present: `./tests/validate.sh` exit 0, `Ran 2482 tests in
  1617.570s` then `OK`, zero `FAIL:`/`ERROR:` lines. Evidence: `~/.cache/s13-pr4b1-validate.{log,exit}`.
  `SHA256SUMS`, the gate module, `internal/sync`, `lib/`, `scripts/` and `go.mod` are untouched.

- Work unit commit: `112dd7c` — `refactor(doctor): extract a Python-faithful shared provider resolver
  (S13 PR4b-1)`. Native review candidate; base `24c4ca1`.
- Native review (approved, authority burned): `inspect` with `baseRef=24c4ca1` plus `committedOnly`
  bound the correct six-file target `06e07da…`. The first `start` returned `consent-binding-stale`
  (the host consent binding had expired on its 10-minute TTL without an answer and no lineage was
  created); a second `start` minted a fresh envelope, which the host resolved, and created lineage
  `review-2f6c9c0f71d806e2` (tier high, 4 lenses, 1541 changed lines, correction budget 200). The final
  admitted lens captured as `approved` (`store_revision: 5394ce0b…`) and the exact acknowledgement
  burned the authority (`gentle-ai.review-acknowledged/v1`). Seven advisory findings, all non-blocking,
  recorded under Follow-ups.
- Incident of the earlier session (no lineage created): an `inspect` without `baseRef` bound the
  whole-epic target (`247ba91…`, base `e773514`), and three `start` calls with `baseRef=24c4ca1` replayed
  that target and returned `consent-binding-expired` (`native_invocation_attempted: false`). The fix is to
  pass `baseRef` to `inspect` from the first call.
- Reviewer transport incident: the `review-resilience` slot failed six times with
  `reviewer-empty-output` (`stopReason: length`) on `nan/glm5.3-flash` and then on `nan/qwen3.8-flash`
  (131 000-token output ceiling); both models exhausted their output budget reasoning and emitted no
  text. The group capture aborts wholesale when one lens fails, so the four lenses were captured one
  slot at a time and that lens was routed to `anthropic/claude-sonnet-5-5`, which emitted on the first
  run. The host relay reads `~/.pi/gentle-ai/models.json` (`gentlePiConfigHome()`), never the subagent
  `model_profiles` in `subagents.json`.

## Follow-ups
- `resolveProviderDep` (`internal/doctor/depcheck.go:304-333`) has no direct test of its `DepResult`
  mapping and detail strings; only the shared package is tested (verifier finding, non-blocking). The
  native review raised the same gap as `R3-doctor-mapping-untested` (`internal/doctor/depcheck.go:312`).
- `resolveProviderDep` passes a non-nil home, so an empty home resolves to the cwd where Python `None`
  falls back to `AI_SPECS_HOME`; both real callers pass a resolved home, so no path observes it.
- PR4b-2: native `build_recipe_mcp` plus `_resolve_provider_markers` over `internal/provider`.
- Advisory findings of the native review, all informational and non-blocking (no correction was opened):
  - `R2-1` readability WARNING, `internal/provider/provider.go:61-72`.
  - `R2-2` readability WARNING, `internal/provider/provider.go:309-313`.
  - `R2-3` readability SUGGESTION, `internal/doctor/depcheck.go:280-283`.
  - `R2-4` readability SUGGESTION, `internal/provider/provider.go:175-177`.
  - `R3-doctor-mapping-untested` reliability SUGGESTION, `internal/doctor/depcheck.go:312`.
  - `R3-runversion-grandchild-pipe-hang` reliability WARNING, `internal/provider/provider.go:220-245`.
  - `R4-runversion-waitdelay` resilience WARNING, `internal/provider/provider.go:216`.

## Rollback
Revert this delivery as one commit; doctor returns to its local helpers.
