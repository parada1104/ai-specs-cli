# GO-20 U2e T5 PR4a S3 — changelog live oracle

Base: epic/go-single-binary d4bb71a. Branch: change/go-20-u2e-t5-pr4a-s3-changelog.
Goal: `internal/changelog/changelog_test.go` stops exec'ing python3 and reading `lib/_internal/changelog.py`
without silent coverage loss. Same discipline as PR #496 (doctor), PR #497 (rulesaudit), PR #498 (skills),
PR #499 (conflicts) — this is the last slice of PR4a S3.

## Specs

- S1: `TestDifferentialCLI` reads a frozen golden (`internal/changelog/testdata/changelog_golden.json`) captured
  once from `lib/_internal/changelog.py main` over the same 14-entry corpus and the same nine `(current, new)` pairs
  doubled over `--notices` (252 invocations); no python3, no `lib/` access, no skip. Missing golden fails.
- S2: Every native (non-Python) assertion of the file survives unchanged: the parser, range, notice, bullet,
  `pySplitLines` and `RunCLI` usage/nothing-to-report tests are untouched; only the live Python leg changed.
- S3: The frozen test asserts a hard-coded independent inventory of the fourteen case names (count, uniqueness,
  order) against both the in-test corpus and the golden before comparing anything, so deleting, duplicating or
  reordering a golden case fails as an inventory error. Each case also pins its run count and, per run, its
  `current`/`new`/`notices` arguments, so a drifted pair grid fails as argument drift.
- S4: Frozen inputs equal the in-test corpus: the corpus is pinned by a sorted `sha256  <case>.md` manifest, asserted
  before any comparison, so input drift fails as input drift, not as an opaque payload mismatch.
- S5: No absolute host path leaks into the golden (the payload is versions, dates from the fixture text and prose):
  `grep -rn /Users/|/private/|/var/folders internal/changelog/testdata` is 0 matches.
- S6: Negative controls: mutating one frozen stdout, deleting a case, duplicating a case, drifting the corpus
  manifest, and drifting one run's arguments each fail rc=1.
- S7: `gofmt -l internal/changelog` empty, `go vet ./internal/changelog/` rc=0, the test free of live `python3` /
  `exec.Command` / `lib/_internal` (comments only), `go test ./internal/changelog/` rc=0 (also under `umask 077`).
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestDifferentialCLI` (`python3 lib/_internal/changelog.py <path> <current> <new> [--notices]`) | FREEZE | the payload is host-independent: exit code plus stdout, no clock (nothing prints `today`), no absolute path, no hashed directory name. Nothing needed tokenizing. The `real` corpus entry was the only moving input (the repository `CHANGELOG.md`), so its text was frozen once into `testdata/real_changelog.md`; the corpus manifest still pins exactly that text, so the golden cannot silently follow later changelog edits. Nothing is retired. |

Cases: 14 (`sample`, `empty`, `noheadings`, `malformed`, `nodate`, `endash`, `dupes`, `fenced`,
`noticefence`, `markup`, `longbody`, `unreleasedonly`, `crlf`, `real`). Pairs: 9
(`0.20.0→0.22.0`, `0.21.0→0.22.0`, `0.22.0→0.22.0`, `0.22.0→0.21.0`, `not-a-version→0.22.0`,
`0.20.1→0.21.0`, `0.19.0→0.22.0`, `1.0.0→2.0.0`, `0.0.0→9.9.9`), each with and without `--notices`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6 | inline | |
| T2 | S7, S8 | inline | |

## Log

- L1 (Robert): "si, avancemos con las entregas" — authorizing continuing the PR4a S3 deliveries with this
  changelog slice (the last of S3).
- L2: Baseline at d4bb71a: the live python differential is green, 252 invocations compared
  (`logs/00-baseline-live-leg.log`, rc=0, 6.62s).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s3-changelog (Robert's standing
  scratch convention); every go command ran with `TMPDIR` set there and its rc persisted in `logs/`.
- L4: RED before the golden existed: `read frozen oracle testdata/changelog_golden.json: no such file or
  directory`, rc=1 (`logs/03-red-missing-golden.log`).
- L5: Capture harness was a temporary `internal/changelog/zz_scratch_capture_test.go` (python3 exec) that
  cross-checked every invocation against the live Go port before writing, so the golden is legacy output, not port
  output; the cross-check found no disagreement over all 252 invocations. The file was moved out of the worktree
  before the commit (preserved in the scratch dir as `zz_scratch_capture_test.go.retired`).
- L6: Captured twice into two fresh scratch dirs and `cmp` byte-identical (rc=0, `logs/04-capture-twice.log`);
  oracle metadata `python: 3.14.7`, `entry: main`. Golden size: 55464 B.
- L7: `real` corpus entry: the legacy leg read the repository `CHANGELOG.md`, which every release edits, so keeping
  that read would turn each changelog edit into an unrelated test failure while the rest of the golden stayed put.
  The text was frozen once into `internal/changelog/testdata/real_changelog.md` (identical to `CHANGELOG.md` at
  d4bb71a, `cmp` rc=0) and the corpus manifest pins its sha256, so the coverage is identical and drift is still
  detected as input drift.
- L8: No date, timestamp, mode or mtime reaches the snapshot: the payload is text only, so no clock injection and no
  `syscall.Umask`/`copystat` pinning is required (unlike the rulesaudit and flatten oracles). Proven by a green run
  under `umask 077` (`logs/12-green-umask077.log`, rc=0).
- L9: Negative controls (each rc=1; golden restored from a pristine copy and `cmp`-verified after each): mutate one
  frozen stdout (`logs/07-nc1-mutate-result.log`), delete a case → `frozen oracle has 13 cases, inventory 14`
  (`08-nc2-delete-case.log`), duplicate a case → `15 cases, inventory 14` (`09-nc3-duplicate-case.log`), drift the
  corpus manifest → `frozen inputs differ from the corpus` (`10-nc4-input-drift.log`), drift one run's `new`
  argument → `sample: frozen run 0 is [0.20.0 0.23.0] ... corpus [0.20.0 0.22.0]` (`11-nc5-run-argument-drift.log`).
- L10: `gofmt -l internal/changelog` empty, `go vet ./internal/changelog/` rc=0, the live-leg grep over the test
  matches comment lines only, and the host-path grep over `internal/changelog/testdata` has 0 matches
  (`logs/13-14-checks.log`). Package tests green: `go test ./internal/changelog/` rc=0, 252 invocations compared
  (`logs/05-green-package-tests.log`).
- L11: S8 evidence: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/17-build.log`), `./tests/validate.sh` rc=0
  (`logs/18-validate.log`): `Ran 2345 tests in 589.381s`, `OK (skipped=165, expected failures=3)`, parity summary
  `gate-absent failing=0, gate-present failing=0 — PASS`.
