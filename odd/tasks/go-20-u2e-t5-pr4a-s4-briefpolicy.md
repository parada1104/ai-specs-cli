# GO-20 U2e T5 PR4a S4 — briefpolicy live oracle

Base: epic/go-single-binary 09041c6. Branch: change/go-20-u2e-t5-pr4a-s4-briefpolicy.
Goal: `internal/sync/briefpolicy_test.go` stops exec'ing python3 and reading `lib/_internal/brief-render-policy.py`
without silent coverage loss. Same discipline as PR #496 (doctor), PR #497 (rulesaudit), PR #498 (skills),
PR #499 (conflicts).

## Specs

- S1: `TestBriefRenderPolicyDifferential` reads a frozen golden (`internal/sync/testdata/briefpolicy_golden.json`)
  captured once from `lib/_internal/brief-render-policy.py` over the same six manifests and both modes (default and
  `--validate`); no python3, no `lib/` access, no skip. A missing golden fails.
- S2: Every native (non-Python) assertion of the live test survives unchanged: `TestBriefRenderPolicy` and
  `TestBriefRenderPolicyMissingFile` are untouched, the live leg still asserts rc and stdout byte for byte, the
  stderr contract (empty on success, one `error: ` line on failure) is preserved as the class comparison the leg
  already made, and the manifest-not-mutated assertion still runs for every non-missing case.
- S3: The frozen test asserts a hard-coded independent inventory of the six case names (count, uniqueness, order)
  against both the in-test corpus and the golden before comparing anything, plus the per-case mode order
  (`default`, `validate`), so deleting, duplicating or reordering a golden case fails as an inventory error.
- S4: Frozen inputs equal the in-test corpus: each golden case pins its `manifest` text and `missing` flag and the
  test compares them before any comparison, so input drift fails as input drift, not as an opaque payload mismatch.
- S5: No absolute host path leaks into the golden: the only host-dependent value in the payload is the temporary
  directory in the missing-manifest stderr, tokenized as `<DIR>`; `grep -c /Users/\|/private/\|/var/folders` over
  `internal/sync/testdata/briefpolicy_golden.json` is 0.
- S6: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a frozen
  input each fail rc=1.
- S7: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the test free of live `python3` /
  `exec.Command` / `lib/_internal`, `go test ./internal/sync/ -run BriefRender -count=1` and
  `go test ./internal/sync/ -count=1` rc=0.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestBriefRenderPolicyDifferential` (`brief-render-policy.py` default + `--validate`) | FREEZE | the payload is host-independent except for the missing-manifest stderr, which carries the temporary directory; tokenizing it as `<DIR>` keeps the full payload — rc, stdout and the recorded legacy stderr text for all six manifests in both modes. Nothing hashes the TempDir, so no RETARGET is needed, and no case is retired: the 6 cases x 2 modes are frozen, not reduced. |

Cases (6): `missing brief table`, `render true`, `render false`, `invalid type`, `bad toml`, `missing manifest`.
Modes per case (2): `default`, `validate` — 12 frozen records.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6 | inline | |
| T2 | S7, S8 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then spine+syncagent, then the materialize-helper consumers; one worker at a time, merge after
  validate.
- L2: Baseline at 09041c6: the live python differential is green for all 12 records
  (`logs/00-baseline-live-leg.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-briefpolicy (Robert's standing
  scratch convention).
- L4: RED before the golden existed: `read frozen oracle: open testdata/briefpolicy_golden.json: no such file or
  directory`, rc=1 (`logs/03-red-missing-golden.log`).
- L5: Capture harness was a standalone python script in the scratch dir (`capture.py`, never in the worktree) that
  ran the real `lib/_internal/brief-render-policy.py` under a fresh temp dir per case, asserted the temporary
  directory never survived tokenization, and asserted the policy did not mutate the manifest — so the golden is
  legacy output, not port output. The two error wordings (`[Errno 2] ...` vs Go's `open ...`, tomllib's
  `Expected ']' ...` vs the Go parse diagnostic) stay out of the frozen stdout and are why stderr remains a class
  comparison, as in the live leg (D3).
- L6: Captured twice into two fresh scratch dirs and `cmp` byte-identical (rc=0); oracle metadata
  `python: 3.14.7`. Golden size: 2968 B. No wall-clock value is frozen.
- L7: Negative controls (each rc=1; golden restored and `cmp`-verified after each): mutate one frozen result →
  only `render false/default` fails on stdout (`logs/07-nc1-mutate-result.log`), delete a case → `briefpolicy: 5
  records, want 6` (`08-nc2-delete-case.log`), duplicate a case → `7 records, want 6`
  (`09-nc3-duplicate-case.log`), drift a frozen manifest → `frozen inputs differ from the corpus for "render true"`
  (`10-nc4-input-drift.log`).
- L8: `gofmt -l internal/sync` empty (`logs/14-gofmt.log`), `go vet ./internal/sync/` rc=0 (`15-vet.log`),
  `grep -n "python3\|exec\.Command\|lib/_internal"` over the test matches one comment line only
  (`13-grep-live-legs.log`), host-path grep over the golden is 0 (`13-grep-live-legs.log`),
  `go test ./internal/sync/ -run BriefRender -count=1` rc=0 (`06-green-focused.log`) and
  `go test ./internal/sync/ -count=1` rc=0 (`16-green-package-tests.log`).
- L9: No file modes or mtimes are frozen: the snapshot is rc/stdout/stderr only, so no `umask`/mtime pinning is
  needed (and `internal/sync` has a `t.Parallel()` test, so a process-wide umask would be unsafe here).
- L10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`17-build.log`), `./tests/validate.sh` rc=0
  (`18-validate.log`): `Ran 2345 tests in 586.674s`, `OK (skipped=165, expected failures=3)`, parity summary
  `PASS`.
- L11: The worktree carries only the three intended paths (test, golden, this doc); the capture harness and both
  captures live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
