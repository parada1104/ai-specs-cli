# GO-20 U2e T5 PR4a S4 — checksync live oracle

Base: epic/go-single-binary 375a7ac. Branch: change/go-20-u2e-t5-pr4a-s4-checksync.
Goal: `internal/sync/checksync_test.go` stops exec'ing python3 and reading `lib/_internal/cli_version.py`
without silent coverage loss. Same discipline as PR #496 (doctor), PR #497 (rulesaudit), PR #498 (skills),
PR #499 (conflicts), PR #501 (briefpolicy).

## Specs

- S1: `TestCheckSyncDifferential` reads a frozen golden (`internal/sync/testdata/checksync_golden.json`)
  captured once from `lib/_internal/cli_version.py check-sync` over the same nine fixtures, including the
  `--ignore-cli-version` mode; no python3, no `lib/` access, no skip. A missing golden fails.
- S2: Every native assertion of the live test survives: the native leg still asserts the exit code and the
  stderr text byte for byte (three-line policy error, ignore warning, `not found.`, invalid `[tool]`), still
  asserts the legacy shim wrote no stdout, and still asserts the fixture files are not mutated.
- S3: The frozen test asserts a hard-coded independent inventory of the nine case names (count, uniqueness,
  order) against both the in-test corpus and the golden before comparing anything, so deleting, duplicating or
  reordering a golden case fails as an inventory error.
- S4: Frozen inputs equal the in-test corpus: each golden case pins its `manifest` text and `manifest_absent`
  flag, its `version` text and `version_absent` flag, and its `ignore` flag; the test compares them before any
  comparison, so input drift fails as input drift, not as an opaque payload mismatch.
- S5: No absolute host path leaks into the golden: the only host-dependent value in the payload is the
  resolved project root in the missing-manifest stderr, tokenized as `<DIR>`;
  `grep -c "/Users/\|/private/\|/var/folders"` over `internal/sync/testdata/checksync_golden.json` is 0. The
  native stderr is tokenized the same way (raw and symlink-resolved form of both temp dirs) before the byte
  comparison.
- S6: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a frozen
  input each fail rc=1.
- S7: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the test free of live `python3` /
  `exec.Command` / `lib/_internal`, `go test ./internal/sync/ -run CheckSync -count=1` and
  `go test ./internal/sync/ -count=1` rc=0.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestCheckSyncDifferential` (`cli_version.py check-sync`, nine fixtures + `--ignore-cli-version`) | FREEZE | the payload is host-independent except for the missing-manifest stderr, which carries the resolved project root; tokenizing it as `<DIR>` keeps the whole payload — rc, empty stdout and the recorded legacy stderr text for all nine fixtures. The check-sync shim itself (root/home resolution, the `not found.` line, the three-line policy error, the ignore warning, the two exit codes) has no frozen coverage anywhere else, so RETIRE would lose it; the policy core it delegates to is frozen at `internal/cliversion/testdata/version_golden.json` (85 cases, `internal/cliversion/cliversion_test.go:229`) and the doctor evaluator at `internal/doctor/testdata/cliversion_golden.json` (28 cases, `internal/doctor/legacy_differential_test.go:21`), but neither pins the shim's exit codes or stdout/stderr, so RETARGET would leave this file asserting nothing of its own. |

Cases (9): `no pin`, `matching pin`, `matching min`, `mismatching pin policy error`,
`below minimum policy error`, `mismatching pin with policy warn`, `unknown installed version`,
`missing manifest`, `invalid manifest` — 9 frozen records.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6 | inline | |
| T2 | S7, S8 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then spine+syncagent, then the materialize-helper consumers; one worker at a time, merge after
  validate.
- L2: Baseline at 375a7ac: the live python differential is green for all nine fixtures
  (`logs/01-baseline-live-leg.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-checksync (Robert's standing
  scratch convention).
- L4: RED before the golden existed: `read frozen oracle: open testdata/checksync_golden.json: no such file or
  directory`, rc=1 (`logs/03-red-missing-golden.log`).
- L5: Capture harness was a standalone python script in the scratch dir (`capture.py`, never in the worktree)
  that ran the real `lib/_internal/cli_version.py check-sync` under a fresh temp root/home pair per fixture,
  asserted the temporary paths never survived tokenization, and froze the fixture inputs alongside the legacy
  output — so the golden is legacy output, not port output.
- L6: Captured twice into two fresh scratch dirs and `cmp` byte-identical (rc=0); oracle metadata
  `python: 3.14.7`. Golden size: 3579 B. No wall-clock value is frozen.
- L7: Negative controls (each rc=1; golden restored and `cmp`-verified after each): mutate one frozen result →
  only `no pin` fails on stderr (`logs/07-nc1-mutate-result.log`), delete a case → `checksync: 8 records, want
  9` (`08-nc2-delete-case.log`), duplicate a case → `10 records, want 9` (`09-nc3-duplicate-case.log`), drift a
  frozen manifest → `frozen inputs differ from the corpus for "matching pin"` (`10-nc4-input-drift.log`).
- L8: `gofmt -l internal/sync` empty (`logs/14-gofmt.log`), `go vet ./internal/sync/` rc=0 (`15-vet.log`),
  `grep -n "python3\|exec\.Command\|lib/_internal"` over the test matches one comment line only
  (`13-grep-live-legs.log`), host-path grep over the golden is 0 (`13-grep-live-legs.log`),
  `go test ./internal/sync/ -run CheckSync -count=1` rc=0 (`06-green-focused.log`) and
  `go test ./internal/sync/ -count=1` rc=0 (`16-green-package-tests.log`).
- L9: No file modes or mtimes are frozen: the snapshot is rc/stdout/stderr only, so no `umask`/mtime pinning is
  needed (and `internal/sync` has a `t.Parallel()` test at `envscaffold_test.go:1466`, so a process-wide umask
  would be unsafe here).
- L10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`17-build.log`), `./tests/validate.sh` rc=0
  (`18-validate.log`): `Ran 2345 tests in 562.664s`, `OK (skipped=165, expected failures=3)`, parity summary
  `gate-absent failing=0, gate-present failing=0 — PASS`.
- L11: The worktree carries only the three intended paths (test, golden, this doc); the capture harness and both
  captures live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
