# GO-20 U2e T5 PR4a S4 — hooksrunner regex pins + helper retirement

Base: epic/go-single-binary 0889c54. Branch: change/go-20-u2e-t5-pr4a-s4-hooksrunner.
Goal: the last S4 slice. `internal/sync/hooksrunner_test.go` stops exec'ing python3 while pinning the
RE2/Python regex gap, and the now-callerless `syncPythonAuthority` / `runSyncPythonOracle` helpers leave
`internal/sync/stamps_test.go`. Same discipline as PR #496 (doctor), #497 (rulesaudit), #498 (skills),
#499 (conflicts), #501 (briefpolicy), #502 (checksync), #503 (envscaffold), #504 (refreshbundled),
#505 (vendorskills), #506 (conflictscore), #507 (copyapply), #508 (hookactuator), #509 (templatedocs).

## Specs

- S1: `TestRecipeHooksRegexEngineDivergence` and `TestRecipeHooksRegexPythonOnlyConstructs` keep their names,
  their subtest names and every native assertion (each `hooksFullMatch` compile error, each RE2 non-match and
  each fail-open RE2 match), but the Python side comes from frozen standard-library `re.fullmatch` verdicts in
  `internal/sync/testdata/hooksrunner_re_golden.json` instead of a live interpreter. No python3 exec, no `lib/`
  access, no skip: a missing golden fails.
- S2: The frozen bundle carries `oracle` metadata — `python: 3.14.7`, `module: Python standard-library re
  (no lib/ script)`, the `driver` (one `re.fullmatch(pattern, input)` per corpus case, `Y` = compiled and
  matched, `N` = compiled and did not match, `ERR` = `re.error`) and a `note` — and the loader refuses a bundle
  missing any of those keys.
- S3: **Decision: FREEZE a new bundle, not RETIRE.** The PR2-B hooksrunner golden (`hooksrunner_golden.json`,
  #489) freezes `execute_hooks` outcomes for its 11 differential scenarios. Only 4 of the 22 regex verdicts
  appear there, and only indirectly, as the raised error string of the `board_id` / `slug` validation cases
  (`[0-9a-f]{24}` matched / mismatched, `[a-z]+` matched / mismatched); it never freezes a verdict for the 18
  divergence pairs (`\1`, `(?P=x)`, the four lookarounds, `\d \w \s`, `\W \S \D \B`, `\p{L}`, `[[:alpha:]]`,
  `\u0041`, `\Z`, `a*+`, `(?>a)`, `(?#x)`, the conditional and `a$\n`). Retiring would drop those 18. The
  PR2-B golden therefore stays byte-identical and the overlap is mapped below.
- S4: The corpus (22 cases in 5 groups, capture order) is the hard-coded, independent inventory of case names:
  `validateGoldenIdentity` checks count, order and uniqueness, and a separate per-verdict count (Y=10, N=11,
  ERR=1) pins the shape, so a flattened bundle fails before any native assertion runs.
- S5: Frozen inputs equal the in-test corpus: each record's `pattern` and `input` are compared before its
  verdict is used, so fixture drift fails as `frozen case "<name>" input diverges`, not as an opaque mismatch.
- S6: The golden freezes no host path: the loader rejects any payload containing `/Users/`, `/private/` or
  `/var/folders`, and the captured corpus is the in-test literals.
- S7: Negative controls: mutating one verdict, deleting a case, duplicating a case and drifting one frozen
  input each fail rc=1, with the golden restored byte-identically (`cmp`) afterwards.
- S8: Helper retirement: after S1 `syncPythonAuthority` and `runSyncPythonOracle` have zero callers in the
  package (grep before deleting), so both are deleted from `stamps_test.go` together with their now-unused
  `bytes` and `os/exec` imports. `syncRecipeHeader`, `writeSyncCatalogRecipe` and every native test stay; no
  `internal/sync` test may reference `lib/_internal` except in provenance comments.
- S9: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, focused tests rc=0,
  `go test ./internal/sync/ -count=1` rc=0, `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and
  `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `pythonFullMatchStatus` inside `TestRecipeHooksRegexEngineDivergence` (python3 `re.fullmatch` over 15 pairs: 6 Python-valid/RE2-uncompilable constructs, 3 Unicode-vs-ASCII `\d \w \s` matches, 4 `\W \S \D \B` fail-open cases, 2 RE2-only constructs) | FREEZE | the payload is three string verdicts per pair with no host-dependent value, so it freezes verbatim. RETIRE was rejected on evidence: the PR2-B `hooksrunner_golden.json` freezes `execute_hooks` error/stdout/stderr/marker bytes for 11 scenarios, and its regex bearing is limited to the 4 `board_id` / `slug` validation cases' error strings — it carries no verdict for any of these 15 pairs. RETARGET has no candidate. The live leg ran exactly these 15 pairs, so the freeze is 1:1. |
| `pythonFullMatchStatus` inside `TestRecipeHooksRegexPythonOnlyConstructs` (python3 `re.fullmatch` over 6 Python-only constructs plus the `a$` / trailing-newline asymmetry) | FREEZE | same verdict-only payload, same 1:1 matrix (7 pairs); the `$` asymmetry is a match/no-match fact, so it freezes as a verdict rather than a live run. No other frozen oracle covers these constructs. |

## Case-by-case mapping to the PR2-B golden (#489, unchanged)

| this corpus case | PR2-B `hooksrunner_golden.json` record | overlap |
|---|---|---|
| (all 18 divergence pairs) | — | none: the PR2-B bundle holds no verdict for them |
| pattern `[0-9a-f]{24}` on `69ec0a2099ea20956e371d62` | `board_id 24-hex regex matches` (error `""`) | indirect: the frozen error string implies Python matched |
| pattern `[0-9a-f]{24}` on `ZZZZZZZZZZZZZZZZZZZZZZZZ` | `board_id regex mismatch` (error `does not match required pattern`) | indirect |
| pattern `[a-z]+` on `goodslug` | `slug regex matches` (error `""`) | indirect |
| pattern `[a-z]+` on `Bad Slug` | `slug regex mismatch` (error `does not match required pattern`) | indirect |

The four overlapping pairs are deliberately *not* re-added to the frozen corpus here: they are already covered
end to end by the PR2-B differential, and this bundle exists only for the pairs that golden cannot reach.

## Corpus (22 cases, capture order)

`compile/` — `backreference`, `named backreference`, `lookahead`, `negative lookahead`, `lookbehind`,
`negative lookbehind`; `class/` — `digit class`, `word class`, `space class`; `failopen/` — `non-word class`,
`non-space class`, `non-digit class`, `non-boundary`, `unicode property`, `posix class`; then
`unicode escape`, `end-of-text escape`, `possessive quantifier`, `atomic group`, `comment group`,
`conditional`, `dollar before trailing newline`. Frozen verdicts: 10 `Y`, 11 `N`, 1 `ERR`
(`failopen/unicode property`, the only `re.error`).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S7 | inline | |
| T2 | S8–S9 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then the materialize-helper consumers; one worker at a time, merge after validate.
- L2: Scratch/TMPDIR: `/Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-hooksrunner` (Robert's
  standing scratch convention); every `go test` / `go build` / `validate.sh` ran with `TMPDIR` pointing at its
  `tmp/`, and each command's rc is appended to its log in `logs/`.
- L3: Baseline of both live legs before any edit: rc=0 in 0.85s, Python 3.14.7
  (`logs/00-baseline-live-legs.log`).
- L4: RED after the restructure with no golden: `read frozen oracle: open
  testdata/hooksrunner_re_golden.json: no such file or directory` at `hooksrunner_test.go:578` and `:631`,
  rc=1 (`logs/01-red-missing-golden.log`).
- L5: The capture harness was temporary code (`internal/sync/zz_hooksrecapture_test.go`, gated by
  `HOOKSRUNNER_RE_CAPTURE`) and was removed before the handoff, so the golden's shape is exactly the
  `hooksREGolden` struct the loader decodes and the corpus is exactly `hooksRECases()` — legacy Python output,
  not port output.
- L6: Captured twice into fresh scratch files and `cmp` byte-identical (`logs/02-capture1.log`,
  `logs/02-capture2.log`, `logs/03-capture-cmp.log` rc=0): 3196 B, 22 cases, `python: 3.14.7`. The grep over
  the committed golden for `/Users/`, `/private/`, `/var/folders` is 0.
- L7: Negative controls, each rc=1, golden restored from the pristine capture copy and `cmp`-verified
  byte-identical after each: mutating one verdict → `11 Y verdicts, want 10` (`logs/07-nc-verdict.log`),
  deleting `failopen/non-space class` → `21 records, want 22` (`logs/07-nc-delete.log`), duplicating the first
  case → `23 records, want 22` (`logs/07-nc-duplicate.log`), drifting one pattern →
  `frozen case "compile/negative lookahead" input diverges` (`logs/07-nc-inputdrift.log`).
- L8: Zero-caller grep before deletion (`logs/04-zero-callers.log`): the only hits for
  `syncPythonAuthority|runSyncPythonOracle` were their own definitions at `stamps_test.go:36,40,57,60`; a
  repo-wide Go grep confirms no reference outside the deleted bodies. Both helpers and the `bytes` / `os/exec`
  imports were removed; `syncRecipeHeader`, `writeSyncCatalogRecipe` and every native stamps test stay.
- L9: GREEN: focused run rc=0 with 41 subtests + 9 top-level tests (`logs/05-green-focused.log`);
  `gofmt -l internal/sync` empty (`logs/11-gofmt.log`), `go vet ./internal/sync/` rc=0 (`logs/12-vet.log`),
  `go test ./internal/sync/ -count=1` rc=0 in 12.19s (`logs/08-green-package-tests.log`).
- L10: Remaining python/`lib/_internal` inventory over `internal/sync/*_test.go` (`logs/11-greps.log`):
  `spine_test.go:811`, `vendorskills_test.go:472,516`, `syncagent_materialize_test.go:36` are native no-python
  guards (PATH-shadow probes, never an oracle); 13 files match `lib/_internal` in provenance comments only;
  `syncagent_test.go:232` reads `lib/_internal/platform.sh` as a live fixture oracle with a skip — out of this
  slice's edit surfaces, reported as a follow-up, and not a python3 oracle.
- L11: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/09-build.log`) and `./tests/validate.sh` rc=0 with
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS` (`logs/10-validate.log`). Worktree
  carries only the four intended paths (`hooksrunner_test.go`, `stamps_test.go`, the new golden and this doc);
  no scratch or capture artifact is inside it, and the change is left unstaged for the orchestrator.
