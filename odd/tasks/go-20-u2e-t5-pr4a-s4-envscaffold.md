# GO-20 U2e T5 PR4a S4 — envscaffold live oracle

Base: epic/go-single-binary 3fc070f. Branch: change/go-20-u2e-t5-pr4a-s4-envscaffold.
Goal: `internal/sync/envscaffold_test.go` stops exec'ing python3 and reading
`lib/_internal/env_scaffold.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync).

## Specs

- S1: `TestEnvScaffoldDifferential` (renamed from `TestEnvScaffoldMatchesPythonAuthority`, which no longer
  matches a live python) reads a frozen golden (`internal/sync/testdata/envscaffold_golden.json`) captured once
  from `lib/_internal/env_scaffold.py <root>` over the same fixture the differential built — a manifest that
  enables the presets-bearing `env-fixture` recipe in a fresh catalog home. No python3, no `lib/` access, no
  skip: a missing golden fails.
- S2: Every native assertion of the live test survives: the exit code, the `Wrote <path>` stdout line, the
  byte-exact stderr (`! VAR has no value in ai-specs.env — run ai-specs configure-recipes` per missing
  variable), and the produced `ai-specs.env.example` and `.envrc` bytes.
- S3: The frozen test asserts a hard-coded independent inventory of the case names (count, uniqueness, order)
  against both the in-test corpus and the golden before comparing anything, plus a hard-coded inventory of the
  files the step produces (`".envrc"`, `"ai-specs.env.example"`, sorted). A golden that drops, duplicates or
  reorders a case fails as an inventory error, and a step that stops or starts writing a file fails as a file
  inventory error.
- S4: Frozen inputs equal the in-test corpus: each golden case pins `recipe_id`, `recipe_body`, `enabled` and
  the manifest text the workspace helper writes; the test reads the written manifest back and compares it
  before running the step, so input drift fails as input drift, not as an opaque payload mismatch.
- S5: No absolute host path leaks into the golden: the only host-dependent value in the payload is the resolved
  project root in the `Wrote` line, tokenized as `<ROOT>` on both sides;
  `grep -c "/Users/\|/private/\|/var/folders"` over `internal/sync/testdata/envscaffold_golden.json` is 0, and
  every frozen produced-file path is root-relative (enforced by the loader).
- S6: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a frozen
  input each fail rc=1.
- S7: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the test free of live `python3` /
  `exec.Command` / `lib/_internal` (only provenance comments stay), `go test ./internal/sync/ -run EnvScaffold
  -count=1` (also under `umask 077`) and `go test ./internal/sync/ -count=1` rc=0.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestEnvScaffoldMatchesPythonAuthority` (`python3 lib/_internal/env_scaffold.py <root>` versus the native `RunEnvScaffold`, compared on rc, stdout, stderr and the two produced files) | FREEZE | the payload is host-independent once the resolved project root in the `Wrote` line is tokenized as `<ROOT>`; the rest is rc, the two warning lines, and the bytes of `ai-specs.env.example` and `.envrc`, all of which are deterministic text with no wall-clock and no mode. The live leg ran exactly one fixture (the presets recipe enabled), so the freeze is 1:1 with its coverage — no case is added and none is dropped. RETIRE was rejected: the shim's own surface (the `Wrote` line, the sorted missing-value warnings, the example and `.envrc` bytes it writes) has no frozen coverage anywhere else. RETARGET was rejected for the same reason: the literal in-test oracles below pin other shapes of the same port, not this end-to-end fixture. |

Shapes the live leg never exercised, already pinned by literal in-test oracles that this slice preserves
untouched: the missing-manifest guard (`envscaffold_test.go:117`, S23), the example bytes (`:145`), the
no-variables literal (`:180`), the `.bak`-only-when-changed rule (`:198`), the three `.envrc` shapes
(`:260`, `:279`, `:334`), the sorted warnings (`:368`), and the whole interactive offer (`:812`-`:1860`).

Cases (1): `presets recipe enabled` — 1 frozen record, 2 frozen files.

No file mode or mtime is frozen: the payload carries neither, so nothing here depends on `umask` (worth stating
because this package runs `t.Parallel()` tests such as `TestWriteHarnessEnvIsOwnerOnly` at
`envscaffold_test.go:1575`).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6 | inline | |
| T2 | S7, S8 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then spine+syncagent, then the materialize-helper consumers; one worker at a time, merge after
  validate.
- L2: Baseline at 3fc070f: the live python differential is green
  (`logs/01-baseline-live-leg.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-envscaffold (Robert's
  standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing there.
- L4: RED before the golden existed: `read frozen oracle: open testdata/envscaffold_golden.json: no such file
  or directory`, rc=1 (`logs/03-red-missing-golden.log`).
- L5: Capture harness was a standalone python script in the scratch dir (`capture/capture.py`, never in the
  worktree) that rebuilt the same fixture (manifest + `catalog/recipes/env-fixture/recipe.toml` under
  `AI_SPECS_HOME`), ran the real `lib/_internal/env_scaffold.py <root>` with
  `PYTHONDONTWRITEBYTECODE=1` (no `__pycache__` written into `lib/`), asserted the temporary paths never
  survived tokenization, and froze the fixture inputs alongside the legacy output — so the golden is legacy
  output, not port output. The input manifest (`ai-specs/ai-specs.toml`) is frozen as the case's `manifest`
  field, not as a produced file.
- L6: Captured twice into two fresh scratch subdirs and `cmp` byte-identical (rc=0,
  `logs/04-capture-twice.log`); oracle metadata `python: 3.14.7`. Golden size: 2058 B. No wall-clock value is
  frozen.
- L7: Negative controls (each rc=1; golden restored and `cmp`-verified after each): mutate one frozen result
  → only the stdout comparison fails (`logs/07-nc1-mutate-result.log`), delete a case → `envscaffold: 0
  records, want 1` (`08-nc2-delete-case.log`), duplicate a case → `2 records, want 1`
  (`09-nc3-duplicate-case.log`), drift a frozen `recipe_body` → `frozen inputs differ from the corpus for
  "presets recipe enabled"` (`10-nc4-input-drift.log`).
- L8: `gofmt -l internal/sync` empty (`logs/14-gofmt.log`), `go vet ./internal/sync/` rc=0 (`15-vet.log`),
  `grep -n "exec\.Command\|exec\.LookPath\|os/exec\|python3"` over the test matches one comment line only, the
  remaining `lib/_internal` mentions are provenance comments, host-path grep over the golden is 0
  (`13-grep-live-legs.log`); `-run EnvScaffold` rc=0 (`06-green-focused.log`), the same under `umask 077`
  rc=0 (`11-green-umask077.log`), `go test ./internal/sync/ -count=1` rc=0 (`16-green-package-tests.log`).
- L9: The port helpers the live leg used (`runExitCode`, `dropEnv` in `internal/sync/step.go`) stay in place:
  `step.go:184`, `step.go:196` and `internal/sync/step_test.go:17` still call them, so dropping this leg's use
  left no dead code and no shared test helper had to change.
- L10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`17-build.log`), `./tests/validate.sh` rc=0
  (`18-validate.log`): `Ran 2345 tests in 563.260s`, `OK (skipped=165, expected failures=3)`, parity summary
  `gate-absent failing=0, gate-present failing=0 — PASS`.
- L11: The worktree carries only the three intended paths (test, golden, this doc); the capture harness and
  both captures live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
