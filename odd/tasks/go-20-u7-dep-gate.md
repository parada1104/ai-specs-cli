# Go 20 / U7-1 — the CLI-dep gate, native

Unit document for **U7 item 1** of the program (`odd/tasks/go-20-python-exit.md`). Base
`epic/go-single-binary` at `494b028`; worktree `.worktrees/go-20-u7-dep-gate`, branch
`change/go-20-u7-dep-gate`.

## Specs

- S1 — Scope (verbatim, program doc `## Next`, item 1): "**The native CLI-dep gate** — `_dep_gate` +
  `dep_install` are the last interactive surface with no Go side. Porting it retires **S37** (the
  `runLegacyInitDepGate` seam in `init`'s wizard) and the matching seam in `configure-recipes`, which
  is the only reason `runShim` still exists."
- S2 — It is sliced before it is opened (program S6, verbatim): "one unit = one small PR against the
  epic with its own review; a verb port is sliced further when it does not fit one review." Measured:
  `_dep_gate` is ~40 lines standing on `dep_install.py` (240: plans, consent prompt, brew/apt
  subprocesses) and on `provider_install.py`'s install half (597: network, checksums, extraction),
  while its CHECK half exists in Go only as `internal/doctor`'s unexported probe. Slices: **T1** the
  check, the plans and the panel; **T2** the provider install half; **T3** the gate itself and the
  seam's retirement; **T4** the dead code and the program row.
- S3 — The plan half reproduces `dep_install.resolve_install_plan`'s DISPLAY byte for byte:
  brew is `brew install <formula>`; apt is `sudo apt-get install -y <pkg>`; guidance is the declared
  `install_url`, or `install '<binary>' manually` when there is none; a provider plan is
  `GitHub Release <repository> (<release_policy>) for <goos>/<goarch>`.
- S4 — The panel reproduces `config_wizard._render_dep_panel`'s line byte for byte (format string
  verbatim from the legacy source): `f"{status:4s}  {r.binary:12s}  {r.purpose}{detail}{hint}"`, with
  `status` = `OK` when the dep is ok, `WARN` when it is required and not ok, `INFO` otherwise;
  `detail` = `f" ({r.detail})"` only when the detail is non-empty; `hint` = `f" → {r.install_url}"`
  only when a URL is declared AND the dep is not ok.
- S5 — One authoritative grader per behaviour (project rule): the probe moves OUT of `internal/doctor`
  into `internal/deps` and doctor delegates to it; nothing is copied.
- S6 — T1 changes no user-visible behaviour: `doctor`'s output is identical and the gate keeps running
  through S37's seam on all three of its callers until T3.
- S7 — The provider plan's validation errors are `provider_install.build_release_plan`'s, verbatim,
  because the gate surfaces them inside `install offer skipped: <exc>`: "provider dependency is not the
  allowlisted GitHub Release installer", "provider release policy must be latest-stable", and the
  platform message "provider has no artifact for this platform; supported targets: <list>.<guidance>".
- S8 — Out of scope for T1: no subprocess install, no download, no wiring change, no deletion.
- S9 — Evidence per slice: RED observed before GREEN; the plan differential drives the REAL
  `dep_install.resolve_install_plan`/`describe_install_plan` against `deps.ResolvePlan`/`Describe`
  across the matrix (brew, apt, both, neither, Darwin, Linux, guidance-only, unknown binary, and the
  provider plan with its rejection paths); then `go test ./cmd/... ./internal/...`, the parity corpus,
  `gofmt -l`, and the manifest diff.
- S10 — T2's scope, measured (verbatim, `odd/tasks/go-20-u6-recipe.md` L10): "`provider_install.py`'s
  install half (597 — network download, checksums, extraction; only its READ-ONLY resolution was
  ported, into `internal/provider`)". T2 ports exactly that half: `_fetch_bytes`, `parse_release`,
  `parse_release_metadata`, `parse_sha256sums`, `_safe_member_name`/`_expected_members`/
  `extract_release_archive`, `install_github_release`.
- S11 — The install's rejection strings are part of the observable contract: `_dep_gate` renders them
  inside its `install offer skipped: <exc>` line and `offer_and_install` prints
  `  ! install <binary> failed: <exc>`. Every message is reproduced verbatim from the Python.
- S12 — The safety invariants are the point of the slice, not decoration: allowlisted HTTPS hosts only
  (`api.github.com`, `github.com`), checksum verified BEFORE extraction, work done in a sibling temp
  directory and published with one atomic rename, an existing managed target never touched, a required
  member set with no symlinks/hardlinks/traversal/duplicates, and size caps on the response, the
  metadata and every member.

## Tasks

| ID | Task | Specs | Commit |
| --- | --- | --- | --- |
| T1 | `internal/provider.BuildReleasePlan` + `internal/deps` (the probe promoted, plans resolving, panel rendering); doctor delegates | S3-S7, S9 | `e72a3a4`, `f4a734e` |
| T2 | `internal/provider`'s install half: download, SHA256SUMS verification, extraction, managed receipt | S10-S12 | `143722f` |
| T3 | `deps.Gate` (panel, per-plan offer, install, recheck, `Configure anyway?`) wired into init's wizard, `configure-recipes` and `recipe add`; S37's seam and driver deleted | S1, S4, S6 | `internal/deps/gate.go` + `internal/cli/depgate.go` |
| T4 | Verify `runShim`/`routeShim`/`shims` are unreachable and delete them; update the program doc's Progress row | S1 | see the T4 section: the map went, the exec path stayed (two named consumers) |

## T1 — verification

Gates, run from `.worktrees/go-20-u7-dep-gate` with `PATH=/tmp/pybin:$PATH` (the interpreter decides
this suite's green: `/usr/bin/python3` is 3.9 and resurrects an unrelated Unicode-version failure in
`internal/provider`'s own tests):

| gate | command | result |
| --- | --- | --- |
| Go suite (through the real entry) | `./tests/validate.sh` → `go test ./cmd/... ./internal/...` + the gate binary's packages | 31 packages `ok`, 0 `FAIL` |
| differential parity harness | `tests/parity/run.py` | `gate-absent failing=0, gate-present failing=0 — PASS` |
| Python suites | `python3 -m unittest discover -s tests -p 'test_*.py'` | `Ran 2405 tests … OK (skipped=165)` — this run's own count, quoted not asserted (program S5) |
| formatting | `gofmt -l internal/ catalog/recipes/worktree-flow/gate` | empty |
| manifest untouched | `git diff --quiet -- ai-specs/ai-specs.toml` | exit 0 |
| plan differential | `go test ./internal/deps/ -run TestResolvePlanDifferential` | 14 cases, decoded projections value-for-value equal |

- RED, observed twice. First the new APIs did not exist (`undefined: Result`, `PlanRequest`, `Plan`,
  `BuildReleasePlan`) — the seam's shape was not there. Second, a real behavioural pin: with the probe
  returning the id the `recipe.toml` DECLARES, `TestCheckProjectDepsUsesTheManifestKey` failed
  (`RecipeID = "declared-id", want the manifest key dir-name`), because `check_project_deps`
  overwrites the id with the manifest key AFTER probing. Removing the overwrite reproduced the failure
  and restoring it went green.
- What this closes: **nothing user-visible**. T1 is the read-only half of the gate — the probe became a
  shared authority, and the plans and the panel exist but nothing calls them. The gate still runs
  through S37's seam on all three callers (S6).
- What U7-1 still owes: **T2** the provider install half (`install_github_release`: download,
  `SHA256SUMS`, extraction, managed receipt), **T3** the gate itself (panel, per-plan offer, install,
  recheck, `Configure anyway?`) wired into init's wizard, `configure-recipes` and `recipe add`, with
  S37's seam and driver deleted, and **T4** the dead-code check (`runShim`/`routeShim`/`shims`) plus the
  program doc's Progress row.

## T4 — the dead-code check, and the correction the check forced

The row said: "Verify `runShim`/`routeShim`/`shims` are unreachable and delete them." Two of the
three are NOT unreachable, and saying otherwise would have been the lie S8 warns about:

| name | verdict | consumer |
| --- | --- | --- |
| `shims` (the verb→script map) | **unreachable, deleted** | nothing: U6e left it empty, so `Route`'s lookup could never succeed |
| `routeShim` (the route kind) | **alive, kept** | `recipe init` (`Route`'s own switch) and init.go's `AI_SPECS_INIT_TUI_PY` hatch |
| `runShim` (the exec path) | **alive, kept** | the same two: `recipe init` → `lib/recipe-init.sh`, and the hatch → `lib/init.sh` |

So T4 retired the map and its lookup, and documented the exec path instead of deleting it: it leaves
when `recipe-init.py` is ported (program item 2) and when U2's launcher flip removes the hatch. What
T4 does change is the reason `runShim` exists — from "legacy verbs might still need it" (a map) to
two named, owned calls.

## T3/T4 — verification

Gates, run from `.worktrees/go-20-u7-t3-gate` with `PATH=/tmp/pybin:$PATH` (the interpreter decides
this suite's green).

| gate | command | result |
| --- | --- | --- |
| build | `go build ./...` | exit 0 |
| Go suite (through the real entry) | `./tests/validate.sh` → `go test ./cmd/... ./internal/...` + the gate binary's packages | 31 packages `ok`, 0 `FAIL` |
| formatting | `gofmt -l internal/` | empty |
| vet | `go vet ./internal/cli/ ./internal/deps/` | exit 0 |
| manifest untouched | `git diff --quiet -- ai-specs/ai-specs.toml` | exit 0 |
| gate contract | `go test ./internal/deps/ -run TestGate` | unit matrix + 13 differential cases, value for value |
| Python suites | `python3 -m unittest discover -s tests -p 'test_*.py'` | `Ran 2405 tests … OK (skipped=165)` — this run's own count, quoted not asserted (program S5) |
| differential parity harness | `tests/parity/run.py` | `gate-absent failing=0, gate-present failing=0 — PASS` |
| dead code | `grep -rn "\bshims\b" --include=*.go .` | no reference outside the deleted declaration |

RED, in the two forms this unit allows: the compile RED (`undefined: Gate`, `undefined:
GateRequest`, `undefined: runPackageCommand`) against tests written first, and the mutation RED that
makes the byte assertions mean something (L14). T3 changes user-visible output in one place only —
the two frozen confirm messages and every panel/offer line now come from Go — so the parity corpus
(`tests/parity/run.py`) is part of the gate, not an extra.

## Log

- L1 — Request (verbatim, 2026-10-19): "si, continuemos", answering the hand-off plan's first item
  ("The native CLI-dep gate … is the only reason `runShim` still exists"), after the program closed
  U6e with `var shims = map[string]route{}` and left U7's scope written in
  `odd/tasks/go-20-python-exit.md`.
- L2 — Slicing measured before opening (S2): the gate is not one PR. `dep_install.offer_and_install`
  runs `brew`/`apt-get` through `subprocess.run(timeout=300)` and delegates the GitHub-release kind to
  `provider_install.install_github_release` (download, `SHA256SUMS`, extraction, managed receipt) —
  delivery-adjacent surface (program risk: a mistake here installs bytes on a user's machine), so it
  gets its own slice and its own review. T1 stops at the read-only half: what to offer, and how to
  display it.
- L3 — T1 finding: the probe had NO shared authority. `internal/doctor/depcheck.go` held it
  unexported and package-private (`checkOne`, `checkProjectDeps`, `checkRecipeCLIDeps`), and the gate
  needs the per-recipe form (`dep_check.check_cli_deps`), which doctor's port did not expose (its
  `checkProjectDeps` walks the manifest). Promoting it into `internal/deps` is what S5 requires;
  doctor now calls `deps.CheckProject` and keeps only its check mapping.
- L4 — T1 finding, recorded not fixed (parity contract debt, U2's): `docs/go-migration-parity-contract.md`
  §8 "Network and external process surface" does NOT list the gate's installs (`brew install`,
  `sudo apt-get install -y`, the provider download), although they are the tree's most consequential
  external calls. T2/T3 own the row once the installs are native.
- L5 — Slice boundary, recorded because the commit tool measures against `origin/main` and this branch
  sits on a stacked epic: the boundary is the EPIC TIP (`494b028`), never `origin/main`. Against the
  epic, T1 is two work units — `e72a3a4` (the probe move: +781/−452, mostly relocation) and `f4a734e`
  (plans, panel and the differential: ~+880) — and each one compiles and passes its packages alone
  (verified in a scratch worktree at `e72a3a4`).
- L6 — Native review `review-0accf2e7e86e68a2` (tier high, 3 lenses: risk, readability,
  reliability): **risk and readability admitted** (103 083 B and 103 129 B prompts, `completed`).
  **`review-reliability` hit the relay bound twice**: 988 499 ms and then 988 506 ms against the same
  988 499 ms ceiling, on a 103 108-byte prompt — the two runs land within 7 ms of the wall, so this is
  not the transient case the program already met once; the lens's configured model cannot finish this
  candidate inside the bound. Both failures were withdrawn with the exact
  `capture-unachievable --withdraw=true` binding, so the slot is re-offered and the lineage stays
  resumable with the candidate tree untouched (`54b6ba2`). Levers, in the program's own order: the
  human's `~/.gentle-shell/config/gentle-ai/models.json` row for `review-reliability`
  (`anthropic/claude-sonnet-5-5` or `opencode-go/glm-5.3`), raising
  `GENTLE_PI_REVIEW_RELAY_PI_TIMEOUT_MS` above the reviewer's real wall time (ceiling 7 200 000 ms), or
  a smaller candidate — which is a NEW transaction and needs this lineage's `ABANDON` approved first.
- L7 — Push and the PR wait for L6's decision: the branch stays local so the review binding and the
  published bytes cannot diverge (`e72a3a4`, `f4a734e`, `63f0884` are the reviewed range).
- L8 — Third attempt after the human's roster change ("lo cambié al deepseek de opencode-go"): **the same
  wall to the millisecond** — 988 499 ms, the third identical number. Diagnosis, and it is not the
  reviewer being slow: `~/.gentle-shell/config/gentle-ai/models.json` (mtime 00:17, i.e. after the
  change) STILL routes `review-reliability` to `opencode-go/mimo-v2.6-pro` with `thinking: "off"`.
  The edit did not land on that row, so the run repeated the configuration that had already failed
  twice. The catalog (`~/.gentle-shell/agent/models-store.json`) has no bare `deepseek` under
  `opencode-go`: the ids are `deepseek-v4-pro` and `deepseek-v4.1-flash`. Slot withdrawn again, so the
  lineage stays resumable and the next attempt is one capture away.
  RULE this costs the next session: three identical ceilings mean the ROSTER is the problem; before
  spending another 16 minutes, read the roster row back and confirm the model id it names exists in the
  catalog. `opencode-go/deepseek-v4.1-flash` (the corrected row) finished the same 103 108-byte prompt
  that three `mimo-v2.6-pro` runs could not.
- L9 — The refuter's one BLOCKER was **factually wrong**: `R3-COMPILE-UNUSED-TOML` claimed the `toml`
  import was left behind in `internal/doctor/depcheck.go` and that "the candidate cannot build". The
  frozen candidate used `toml.Table` in `checkRecipeCLIDeps` and built and passed every gate. The
  correction the finding asked for was still cheap and worth taking, so it was taken instead of argued:
  the guard moved to `manifest.go:hasRecipeTables()` (next to the cache it reads) and `depcheck.go` no
  longer imports `toml` at all. Behaviour unchanged, doctor's and deps' suites green. Recorded because
  a deterministic claim from a reviewer is still a claim: the compiler decides, and here it had
  already decided.
- L11 — T2 delivered as `143722f` (one commit, ~1724 added lines — the size exception is argued in the commit
  message and the PR: a brand-new package whose fixture suite shares archive builders reads better whole
  than hand-carved across two commits). The porting worker skipped RED by writing the implementation
  first, so RED was reconstructed by MUTATION and it bit: flipping the checksum comparison failed
  `TestInstallReleaseHappyPath`, `TestInstallReleaseChecksumMismatch`, `TestInstallReleaseVersionMismatch`
  and `TestInstallReleaseBelowMinVersion`, and disabling the missing-member check failed
  `TestExtractReleaseArchiveMissingMember` and `TestExtractReleaseArchiveMissingBinary` — each with its
  exact message — and restoring both went green. `go test ./internal/provider/`, `go vet` and `gofmt -l`
  clean under `PATH=/tmp/pybin:$PATH`; `./tests/validate.sh` on the final tree: **exit 0**, `go test
  ./cmd/... ./internal/...` green, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`,
  `Ran 2405 tests … OK (skipped=165)`.
  METHOD note for the next delegation: say "write the tests first" AND "if you write the implementation
  first, reconstruct RED by mutation and report both mutations" — the writer reported the gap honestly,
  which is the only reason it was cheap to close.
- L12 — T3 design, decided before the code. The gate is `internal/deps.Gate` (`gate.go`), the
  legacy function arm by arm: probe → panel → (TTY only) one plan per missing required dep → offer
  each plan → recheck → panel → `N required CLI tool(s) missing. Configure anyway?`. Two things are
  deliberately NOT in it: nothing re-reads the recipe (the caller already holds the parsed
  `*schema.Recipe`; the Python re-read existed only because the seam was a separate process) and
  nothing bootstraps a vendor tree (the port has no Python packages to install). Every outside
  world is an injected seam — `Probe`, `Prompt`, `Run`, `InstallProvider`, `Suspend`/`Resume` — which
  is what makes the sequencing and the bytes pinnable in-process. The CLI half is
  `internal/cli/depgate.go`: two thin adapters, because the three callers differ only in who owns
  the raw-mode window (`init`'s wizard holds it and the gate suspends it around every install child;
  `recipe add` and `configure-recipes` open one window for the gate's own questions and close it on
  the way out).
- L13 — T3 retires the seam's whole vocabulary, not just its driver. `wizardDepGate` now takes the
  recipe the caller already loaded (`func(*schema.Recipe)`), and `wizardDepGateOutcome` is
  `{Proceed, Err}`: **rc 3 and rc 11 are gone with the driver that produced them** — 3 was Python's
  `_ensure_deps` refusing to run without rich/questionary, 11 was the driver's own `read_recipe`
  failure, and the native gate has neither a vendor tree to bootstrap nor a second read to fail.
  The three call sites lose their dead `Rc == 3` arms and their frozen lines
  (`  ! CLI dependency check skipped: interactive dependencies are unavailable` in `recipe add`,
  the loop's silent exit 3, the wizard's bare rc-3 return); each keeps its own `Err` wording
  verbatim, and the two surviving answers are Proceed (configure) and !Proceed (the declined
  confirm, which every caller already worded). The one arm with no Python analogue is
  `Resume` failing after a child: a terminal that cannot go back to raw mode is an error, because
  nothing after it can prompt.
- L14 — T3's RED, and the mutation that makes it honest. The first RED is the compile one
  (`undefined: Gate`, `undefined: GateRequest`, `undefined: runPackageCommand` against the tests
  written first). Byte-level RED cannot come from writing the port *after* its tests in one pass, so
  it was reconstructed by mutation and it bit in all three directions: the guidance arrow turned
  into a dash failed `01-two-missing-declined`; the panel's status precedence inverted failed the
  same case; skipping the recheck after the offer failed that case and two unit tests
  (`TestGateInstallsPackagePlan`, `TestGateProviderPlanDescribesThenInstalls`) that read the second
  panel. Method note for the next unit: same lesson as T2's, one level up — "write the tests first"
  buys the compile RED, and the mutations buy the rest.
- L15 — T3's contract test is a differential that drives the REAL `_dep_gate`:
  `internal/deps/testdata/gate_driver.py` fakes only the outside world, at the seams the frozen
  module itself uses — `config_wizard._dep_check.check_cli_deps` (the probe, scripted per case),
  `questionary` in `sys.modules` (every `confirm(...).ask()` records its message and answers from the
  script), `dep_install`'s `platform.system`/`shutil.which`/`subprocess.run` and its
  `_load_provider_install` (which keeps the REAL `provider_install` module, because
  `resolve_install_plan` calls `build_release_plan` on it — only the network install is scripted) —
  and `sys.stdin/stdout.isatty` for the TTY guard. 13 cases compare the verdict, both streams, the
  confirm messages in order and the commands run, value for value. Two fixture rules worth
  recording: the driver sets `COLUMNS=1000` because rich wraps the console at the terminal width
  (the port emits one unwrapped line per result — recorded as a deviation below, not silently), and
  the Python side had to be given a `StringIO` subclass whose `isatty()` returns the scripted answer,
  because `redirect_stdout` REPLACES `sys.stdout` and `_dep_gate` asks the replacement.
- L16 — T3 documented deviations (the ADR 0002 class, pinned where reachable): rich wraps a long
  panel line at the terminal width and the port does not (one line per result); the package arm's
  `OSError` wording is Go's (`exec` error text) where Python narrated its own, while
  `TimeoutExpired`'s message IS reproduced because `_dep_gate` renders it inside the frozen
  `install offer skipped:`/`failed:` lines; and a prompt that cannot be answered (EOF, interrupt)
  is a decline, following the port's existing questionary-`None` precedent rather than inventing an
  error.
- L19 — Slice boundary and size exceptions (L5's rule restated, because the commit tool measures
  against `origin/main` and this branch sits on a stacked epic): the boundary is the EPIC TIP
  `ff5e81b`, never `origin/main`. Against it this unit is 15 files, +1766/−349, in five commits:
  `gate.go`+unit matrix (862, a new package plus the matrix that grades it — splitting the tests out
  would leave the behaviour ungraded in its own commit), the differential+driver (519, a pure
  contract-test commit), the CLI wiring (570 across 6 files, of which 319 are deletions — one
  retirement, not several), T4's dead code (49), and the docs (115). T2's precedent applies: a
  brand-new package whose fixture suite shares builders reads better whole than hand-carved.
- L18 — The full gate on the final tree: `./tests/validate.sh` **exit 0** — 31 Go packages `ok`,
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, `Ran 2405 tests in
  1577.699s … OK (skipped=165)`, `gofmt -l` empty, manifest diff empty. Time note for the next
  session: the Python suite alone needs ~26 minutes on this machine, so a shorter timeout is not a
  hang — two 8- and 25-minute timeouts were spent learning that before the run was given a full hour.
- L17 — T4's premise was half wrong, recorded rather than quietly narrowed. `runShim` and
  `routeShim` are LIVE: `recipe init` routes to `lib/recipe-init.sh` through them, and init.go's
  `AI_SPECS_INIT_TUI_PY` hatch hands the whole verb to `lib/init.sh` the same way. Only the
  verb→script map was dead (U6e left it empty, so `Route`'s lookup could never fire), and only it
  was deleted, along with the test assertion that read it. The two survivors now carry a comment
  naming their consumers and the unit that retires each: `recipe-init.py` is U7's second item, the
  hatch leaves with U2's launcher flip. Recorded because the difference between "the mechanism is
  dead" and "the mechanism is down to two callers" is exactly what a program forgets.
- L10 — The review on the corrected candidate (`review-aa5e96c9b4183ea7`) ended in state **`escalated`**,
  and the escalation is the benign kind: the provider recorded `R3-001` as a **non-candidate-causal,
  downgraded** finding — `downgraded_causal_findings: [{finding_id: R3-001, reason: unverified_location}]`
  with `candidate_causal_finding_ids: []`. The claim ("the doctor test file no longer imports `os` but
  `writeCatalogRecipe` calls `os.MkdirAll`/`os.WriteFile`, so the test package cannot compile") is false:
  the helper calls doctor's own `writeFile`, and `go test ./internal/doctor/ -run TestNothingMatches`
  compiles and runs the package. `escalated` is informational for the maintainer, so nothing blocks and
  nothing is approved either. Recorded because it is a pattern worth naming: both review cycles on this
  unit produced a FALSE deterministic BLOCKER from the same class of misread (R3-COMPILE-UNUSED-TOML,
  then R3-001), and the compiler was the tie-breaker both times. Rule: when a reviewer's claim is
  "this does not compile", run the compiler before believing or correcting anything.
- L20 — Native review `review-172f8ed806b45d1b` (3 lenses: risk, resilience, reliability) reached the
  provider refuter and came back **`correction_required`** with one candidate-caused deterministic
  BLOCKER, and this one is real — unlike L10's homonym. `R3-001` @ `internal/cli/recipe.go:474`:
  `gateOutcome` reports a failure as `{Err: err}` with `Proceed` still false, so the `!outcome.Proceed`
  arm authored first shadowed the `outcome.Err != nil` arm, leaving it unreachable. Consequence: a gate
  error (terminal could not be opened, unusable stdin) was worded as a decline — `Skipped <id> (missing
  CLI deps)` on stderr with config fields, the guidance block without — and the command carried on as if
  the gate had answered. The `configure-recipes` call site (:2917) always had the correct order, which is
  why only one line was named: the defect was per-call-site, not per-seam. RED first (both legs, exact
  bytes: the decline's `Skipped` line and the decline's guidance block), then the arm swap, then GREEN.
  Two lessons. (1) A shared outcome type that encodes failure as "false plus an error" makes arm order
  load-bearing; the failing test has to exercise the CALL SITE, not the seam — `depgate_test.go` pinned
  `recipeDepGate` in isolation and stayed green through the bug. (2) The review's own finding ids repeat
  across cycles (`R3-001` twice on this unit, opposite verdicts), so the id is not evidence; the
  compiler/test is.
