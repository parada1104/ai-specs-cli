# Go 20 / U2a — native sync helpers

Unit U2a of `odd/tasks/go-20-u2-launcher-flip.md`. Branch `change/go-20-u2a-sync-helpers` off
`epic/go-single-binary` @ `7daf7eb`. Each Python helper keeps its script under `lib/_internal/` until
U2e, so every port runs a differential against the Python while it still exists.

## Specs

- S1 — Parent unit, verbatim from the U2 plan: "U2a — native sync helpers: check-sync, gitignore render ×2,
  refresh-bundled call, vendor-skills, brief-render-policy, agents-render, target-resolve (S5)."
- S2 — Inventory, verified on `7daf7eb` (`internal/sync/sync.go`):
  - `check-sync`: `sync.go:211` runs `cli_version.py check-sync`. The policy core is native
    (`internal/cliversion/cliversion.go:1-10`); only the process shim is Python.
  - `gitignore-render`: `sync.go:263`. Native twin in `internal/sync/gitignore.go`; the differential at
    `internal/sync/gitignore_test.go:121-125,343-350` runs the real Python.
  - `gitignore-root-refresh`: `sync.go:271`. Same native twin, same differential.
  - `refresh-bundled`: `sync.go:276`. No native sync path. The top-level verb is a compatibility no-op
    (`internal/cli/refresh_bundled.go:29-32`), so this port is real work.
  - `vendor-skills`: `sync.go:281`, plus the recipe-dep delegation at `materialize_loop.go:615-673`. Only
    part of skill rendering is native (`internal/skills/vendored.go:16-20`). Real work.
  - `brief-render-policy`: `sync.go:391`. Native twin `internal/sync/briefpolicy.go`.
  - `agents-render`: `sync.go:393`. Native twin `internal/sync/agentsrender.go:1332-1414`.
  - `target-resolve`: **not a Python call.** `sync.go:443-460` calls `target.Run` natively. The U2 plan's
    S5 listed it in error; the correction is logged there (L4).
- S3 — Two Python executions outside the S2 list, both must be classified in this unit:
  - `materialize_loop.go:670-673` (`materializeRunPython`): one inline `python3 -c` for recipe-dep delegation.
  - `hooksrender.go:771`: the GENERATED hook script shells out to `python3 -c` to JSON-encode its output.
    This ships to user machines. It is product output, not a Go→Python call, and it is a decision (D4).
- S4 — Ordering, which the port must keep: gitignore before cache and deps; refresh before vendor; brief
  policy and render after recipe materialization (`sync.go:259-281,346-398`). Lock writes are serial:
  refresh, vendor, renderer, then the final version stamp (`sync.go:433-434`). Sync is not transactional:
  earlier writes stand when a later step fails (`sync.go:425-426`). The port keeps all of it.
- S5 — Divergences this unit must decide, not silently keep or silently change:
  - D1 — file mode. Native gitignore writes fixed `0644` (`internal/sync/gitignore.go:71-75,135-152`);
    Python's `write_text` is umask-dependent. The parity harness measures modes (`tests/parity/parity.py:7-16`).
  - D2 — unmatched begin marker in `gitignore-root-refresh`. Native prints one line; Python raises with a
    traceback (`internal/sync/gitignore.go:92-93,122-124`). Both fail; the text differs.
  - D3 — `brief-render-policy` exit is ignored at `sync.go:391`; the caller gates on stdout `true`. A
    missing output can turn into a skipped brief while stderr reports an error. Preserve, or fix?
  - D4 — the generated hook's `python3` dependency (S3). Out of U2's scope to remove silently; it is a
    product decision about what the installed hook may assume.
- S6 — Failure of a refresh leaves a partial cache: destinations are deleted before the copy
  (`refresh-bundled.py:65-75,87-98`). The port keeps the behaviour unless the human decides otherwise.
- S7 — Gates per task: `go build ./...`, `gofmt -l`, `go vet` on touched packages, focused tests in
  `internal/sync/`, the parity runner for the full-sync fixtures (`tests/parity/parity.py:1033-1036`), and
  `./tests/validate.sh` once by the parent. Python legs under `PATH=/tmp/pybin:$PATH`.

## Tasks

- **T1** — `check-sync`: route sync to the native check. Differential against `cli_version.py check-sync`,
  exit codes 0 / 1 / warning on stderr (`sync.go:215-216` collapses nonzero to 1). Tests: the existing
  sync-gate cases in `tests/test_sync_pipeline.py:3436-3438` stay green.
- **T2** — `gitignore-render` and `gitignore-root-refresh`: call the native functions. Re-run the existing
  differential. D1 and D2 are decided here; the decision is written in the unit log before the commit.
- **T3** — `brief-render-policy` and `agents-render`: call the native functions. D3 is decided here. The
  existing differentials (`internal/sync/agentsrender_test.go:714-717`) run against the Python first, then
  against the native call.
- **T4** — `refresh-bundled`: native cache rebuild, legacy cleanup and lock normalization. Differential
  against `refresh-bundled.py`, including the partial-cache behaviour of S6.
- **T5** — `vendor-skills`: native root `[[deps]]` path and the recipe-dep delegation. Differential against
  `vendor-skills.py` with `AI_SPECS_VENDOR_FIXTURE_ROOT`, the same fixture the Python uses.
- **T6** — `materializeRunPython` (S3): replace the inline `python3 -c` with the native path it wraps.
  D4 is decided before the hook generator is touched.
- **T7** — remove the Go→Python executions in `internal/sync/sync.go` once T1–T6 are green. Acceptance:
  `grep -n python3 internal/sync/*.go` returns only tests and generated hook content (the latter per D4).

## Decisions this unit needs from the human

Written here so they are not discovered mid-task. Each one is taken before its task starts.

- D1 — gitignore file mode: keep `0644` (native) or match the Python's umask-dependent mode.
- D2 — unmatched begin marker: accept the native one-line wording as contract, or match the traceback.
- D3 — ignored `brief-render-policy` exit: preserve the current skip-on-missing-output, or fail loudly.
- D4 — the generated hook's `python3` dependency: keep it (installed hooks may assume python3), or
  replace the JSON encoding with a shell-only form.

Decided by the human on 2026-10-22, taking the recommendation in each case:

- D1 → **match the umask.** The native gitignore writes with the umask-dependent mode, like Python.
  The fixed `0644` is an accident, not a hardening. T2 changes `internal/sync/gitignore.go:71-75,135-152`.
- D2 → **accept the native wording** as contract. Both paths fail with a nonzero exit; only the text
  differs. Recorded as an accepted divergence in U2e.
- D3 → **preserve** in U2a. A port is not a behaviour change. The silent skip is a separate unit
  to be scheduled after U2.
- D4 → **keep the hook's python3** for now. Installed hooks may depend on it, and changing a shipped
  artifact is a separate decision. T6 leaves the generated hook untouched.

## Log

- L1 — Parent request, verbatim: "ok" (2026-10-22), approving the merge of #407 and the drafting of this
  unit document, with no code in this step.
- L2 — The inventory in S2 was measured by read-only exploration on `7daf7eb` and checked by grep before
  this document was written.
- L3 — S3 surfaced two Python executions the U2 plan had not listed. They are added here, not hidden.
- L4 — Correction to `go-20-u2-launcher-flip.md` S5 (the U2 plan): `target-resolve` is not executed by
  Python from sync. Measured: `sync.go:443-460` calls `target.Run`. The U2 plan's list is corrected here, and
  the U2 plan's own text is left as written for its history.

- L5 — The four decisions D1–D4 were taken by the human on 2026-10-22, each with the recommended
  option. They are recorded in the Decisions section above, before any task starts.

- L6 — T1 (`check-sync`) done on `change/go-20-u2a-code`, 2026-10-22. `internal/sync/checksync.go` adds
  `CheckSync(root, home string, ignoreCliVersion bool) (int, string)`, mirroring
  `lib/_internal/cli_version.py:238-277` and delegating the policy core to `internal/cliversion`
  unchanged. `internal/sync/sync.go:209-217` now calls it instead of exec'ing the module; the caller's
  collapse of any nonzero to 1 is untouched, and the returned stderr is written verbatim (the ignore
  warning included). Exit contract, quoted from the Python: `2` for missing argv (not reachable from sync),
  `1` + `ERROR: <manifest> not found.` for a missing manifest, `1` + `ERROR: <message>` for an invalid
  `[tool]`, `0` for no policy or a passing policy, `0` + one `⚠ ai-specs: ignoring CLI version policy
  (<reason>)` line when a failing policy is ignored, and `1` + three lines (`ERROR: <reason>.`, the upgrade
  hint, the `--ignore-cli-version` hint) for a failing policy. `checksync_test.go`
  (`TestCheckSyncDifferential`) runs the real Python module and the native function over nine fixtures
  (no pin, matching pin, matching min, mismatching pin, below minimum, mismatching pin + `--ignore-cli-version`,
  unknown installed version, missing manifest, invalid `[tool]`) comparing exit code and stderr; RED was the
  compile failure `undefined: CheckSync`, GREEN is all nine subtests passing. Non-vacuity: forcing the
  policy-failure branch to exit 0 made three subtests fail (`native = 0, python = 1`) before reverting.
  Known divergence, recorded not fixed: malformed TOML raises an uncaught traceback in Python, while the port
  exits 1 with a single `ERROR: cannot parse ...` line; both fail, only the text differs (same class as D2).

- L7 — T2 (`gitignore-render` + `gitignore-root-refresh`) done on `change/go-20-u2a-code`, 2026-10-22.
  `sync.go:259-273` calls `RenderAiSpecsGitignore` and `RefreshRootGitignore` natively, in the same order,
  inside the same `runStep` wrappers, with the same error propagation and the same two success lines
  (`  ✓ wrote <path> (<n> dep(s))`, `  ✓ refreshed|appended root .gitignore (agent block)`). The two
  `r.exec` invocations are gone; `GO_SYNC_STEP_GITIGNORE` / `GO_SYNC_STEP_GITIGNORE_ROOT` no longer select a
  leg (validated, inert) and are validated so a bogus value is still refused. The Python modules stay under
  `lib/_internal/` as the oracle until U2e.

  D1 implemented: `internal/sync/gitignore.go` writes with `gitignoreFileMode = 0o666` at all three write
  sites, which is exactly Python's `Path.write_text()` rule — a new file gets `0666 &^ umask`, an existing
  file keeps its mode (truncate, never chmod). Measured, not assumed, by the differential: under umask `022`
  a new file is **0644** in both legs; under umask `002` Python writes **0664** while the old fixed `0644`
  wrote `0644`, which is the RED case.

  D2 accepted divergence, recorded here as decided: an unmatched begin marker fails on both sides with a
  non-zero exit (1), empty stdout and an unchanged file; the Python authority raises an uncaught
  `ValueError` (traceback on stderr) while the port prints one `ERROR: <path>: found begin marker without
  matching end marker` line. `TestGitignoreDifferentialUnmatchedBeginMarker` freezes the exit code, empty
  stdout and the untouched file tree, never the traceback text. The old deviation note in `gitignore.go` is
  unchanged and now backed by that test.

  Test evidence: `internal/sync/gitignore_test.go` gains umask cases (new file under 022 → 0644; new file
  under 002 → 0664, the RED case), an existing-file-keeps-its-mode case (0600), a root-refresh append case
  under umask 002, and a shared mode comparison for both legs. RED before the fix:
  `artifact mode: go=0644 ref=0664` on both umask-002 cases, `FAIL TestGitignoreDifferential`. GREEN after:
  `ok ai-specs.dev/ai-specs/internal/sync` (`-run Gitignore` and the whole package). Non-vacuity: forcing
  `gitignoreFileMode` back to `0o644` reproduced the same two failures, then reverted.
  Known divergence, recorded not fixed, per D3: malformed TOML handling of the render is untouched (T1's
  L6 note covers the check-sync case).

  Open, needs the parent: two Python suites stubbed the now-native first two steps through PATH and are red
  for that reason only — `tests/test_sync_run_step_errexit.py` (8/9 fail: `STEP_ONE`/`STEP_TWO` stubs are
  never reached, e.g. `AssertionError: 0 != 7`) and `tests/test_sync_output_verbosity.py` (7/22 fail, same
  seam). Both files are outside this unit's allowed edit surfaces, so they were left untouched; the fix is
  to re-point their stub target to a still-Python step (`refresh-bundled.py` / `vendor-skills.py`) the way
  S15/S17 re-pointed tests whose seam was removed.

- L8 — T3 (`brief-render-policy` + `agents-render`) done on `change/go-20-u2a-code`, 2026-10-22.
  `sync.go:380-386` now calls `renderAgentsStep(tomlPath, <root>/AGENTS.md, resolvedConfigTemp,
  opts.adoptBrief, out, errW)` — one native call inside the same `runStep` wrapper, in the same position
  (after recipe materialization, before the fan-out) — and the `stepMode("agents-render") == "go"` branch
  plus both `r.exec` invocations (`brief-render-policy.py`, `agents-render.py`) are gone. The resolved-config
  temp is still created, passed and removed exactly as before; the argv it fed is now the native
  `RenderAgentsOptions`. `GO_SYNC_STEP_AGENTS_RENDER` no longer selects a leg (validated, inert) exactly as
  T2 left the two gitignore flags. The Python modules stay under `lib/_internal/` as the oracle until U2e.

  D3 preserved, as decided above: `renderAgentsStep` ignores the policy's failure status and gates on its
  verdict alone, so a manifest the policy cannot read still skips the brief with exit 0 (skip notice on
  stdout, one `error: ` line on stderr). Only the error wording changed, because the two parsers describe
  the same defect differently — the same class as L6's recorded divergence. The silent skip itself is a
  separate unit and was deliberately not fixed here.

  Test evidence. `briefpolicy_test.go` gains `TestBriefRenderPolicyDifferential`: native policy vs the real
  `brief-render-policy.py` over a missing `[brief]` table, `render = true`, `render = false`, an invalid
  type, bad TOML and a missing manifest, each in default and `--validate` mode (12 subtests). Exit code and
  stdout are compared byte for byte; stderr only as a class (`error: ` on both sides), since the interpreters
  word the error differently; the manifest is re-read to prove the policy writes nothing.
  `agentsrender_test.go` gains `TestAgentsRenderStepDifferential` (the composed step vs the real two-module
  sequence, over the 11 governance fixtures including the preserve-if-runtime-brief marker case, comparing
  exit code, both streams, AGENTS.md bytes and lock bytes),
  `TestRenderAgentsStepPolicyFailurePreservesTheSilentSkip` (the D3 invariant above), and
  `TestSyncAgentsStepIsNative`, which drives the whole spine in process through `sync.Run` with a
  PATH-shadowed `python3` that logs every invocation, requires the log to still contain
  `refresh-bundled.py` (so the shadow cannot pass vacuously), and requires it to contain neither
  `brief-render-policy.py` nor `agents-render.py`.

  RED, before the `sync.go` change: `--- FAIL: TestSyncAgentsStepIsNative / sync still execs
  brief-render-policy.py; log: ...` and the same line for `agents-render.py`, with the log showing both
  modules' full argv. GREEN after: `ok ai-specs.dev/ai-specs/internal/sync` with `-run
  'BriefRender|AgentsRender|BriefPolicy|TestSyncAgentsStepIsNative'` and for the whole package. Non-vacuity:
  flipping the native `[brief].render` verdict (`return b` → `return !b`, `briefpolicy.go`) made
  `TestBriefRenderPolicyDifferential` report `stdout: go: "false" / python: "true"` and
  `TestAgentsRenderStepDifferential/brief_render_false_disables_the_policy_gate` report a written AGENTS.md
  where Python skipped; reverted, and `git diff internal/sync/briefpolicy.go` is empty.

  No re-point was needed and none is recorded: L7's open item is stale. Both `tests/test_sync_run_step_errexit.py`
  and `tests/test_sync_output_verbosity.py` already stub `refresh-bundled.py` (a still-Python step) through
  PATH — that re-point landed in 4570be4, the T2 commit, not in T3. Both files were left untouched:
  `python3 -m unittest tests.test_sync_run_step_errexit tests.test_sync_output_verbosity
  tests.test_brief_render_policy tests.test_agents_render_brief_fragments` → `Ran 109 tests ... OK`.

- L9 — T4 (`refresh-bundled`) done on `change/go-20-u2a-code`, 2026-10-22. `internal/sync/sync.go`
  replaced the `r.exec` of `refresh-bundled.py` with one native call, `RefreshBundled(root, r.home, out, errW)`,
  inside the same `runStep("bundled skills + commands")` wrapper and in the same position (after the root
  .gitignore refresh, before `vendor-skills.py`). The `internal(...)` helper stays for the remaining Python
  steps. `internal/sync/refreshbundled.go` is the port; `internal/sync/refreshbundled_test.go` is its
  differential. `lib/_internal/refresh-bundled.py` and `project-cache.py` stay in place as the oracle until
  U2e, and `internal/cli/refresh_bundled.go` was not touched.

  The Python contract, quoted from `lib/_internal/refresh-bundled.py` and used as the differential target:
  - writes `{cache}/.bundled/skills/<id>` (one `shutil.copytree` per directory directly under
    `{home}/bundled-skills/`, sorted by name) and `{cache}/.bundled/commands/<name>.md` (flat `copy2` of
    every `*.md` directly under `{home}/bundled-commands/`, sorted); the cache root is
    `{home}/cache/projects/<cache_key>`.
  - deletes each destination tree (`rmtree`) BEFORE the copy, so a failed copy leaves a partial cache — S6
    is kept, not fixed. The project surface is never written.
  - loads the lock BEFORE the cleanup (`load_lock`) so the legacy `[skills]` hashes are still in memory,
    then calls `remove_bundled_skill_leftovers(project/ai-specs, cli_source, lock.get("skills") or {})`,
    `remove_bundled_command_leftovers(project/ai-specs, cli_source)` and
    `remove_recipe_command_leftovers(project, cli_source)` (the latter two self-read the on-disk legacy
    `[commands]` table). Only copies matching the current bundled source, the legacy lock hash, or the
    recipe provenance are removed; customized copies are kept with a warning.
  - `write_lock` normalizes: `[skills]`, `[commands]` and `[recipes]` are dropped; the emitted lock is the
    header plus `[meta]` (`cli_version`, `synced_at`), `[managed.*]`, `[deps.*.skills.*]` and `[agents.*]`,
    sorted, with a missing `[meta]` simply not emitted.
  - stdout: `  ⇢ flattened <n> bundled skill(s) → cache/.bundled/skills` and
    `  ⇢ flattened <n> bundled command(s) → cache/.bundled/commands` when non-zero, otherwise
    `  = all bundled up-to-date`; plus the leftover-removal and tracked-remediation lines from projectcache.
    stderr: the `  ! keeping customized ...` and `  ! failed to remove ...` warnings.
  - exit: `0` on success; `1` + `ERROR: <project>/ai-specs not found. Run `ai-specs init` first.` on stderr
    when the project surface is missing; `2` for a usage error (not reachable from sync, which always
    supplies both paths). Uncaught filesystem/lock errors exit non-zero in Python with a traceback; the
    port exits 1 with one `ERROR: <message>` line (same class as L6's recorded divergence, not covered by a
    fixture).

  `TestRefreshBundledDifferential` builds twin project/home fixtures, runs the real module on one pair and
  `RefreshBundled` on the other, and compares exit code, stdout, stderr (with the Python lock-bridge's
  `GO_LOCK_WRITE_BRIDGE_FALLBACK` infrastructure warning filtered and the differing temp project root
  normalized), the whole project tree (paths, bytes, permission bits) and the per-project cache tree.
  Fixtures: fresh project, no bundled assets (`all bundled up-to-date`), a stale cache that must be
  rebuilt, a legacy skill copy matching the source plus a legacy command copy matching only the `[commands]`
  hash (with `[skills]`/`[commands]` in the lock, proving the normalization), a lock without `[meta]`, and
  a missing `ai-specs/` directory (nonzero on both). `TestSyncBundledStepIsNative` drives the whole spine
  with a PATH-shadowed `python3` that logs invocations.

  Test evidence. RED, before the `sync.go` change: `TestSyncBundledStepIsNative` failed with
  `refreshbundled_test.go:76: sync still execs refresh-bundled.py; log:` followed by the module's full argv.
  GREEN after: `--- PASS: TestSyncBundledStepIsNative`, `--- PASS: TestRefreshBundledDifferential (0.39s)`
  with all six subtests, and `ok ai-specs.dev/ai-specs/internal/sync` for `-run 'RefreshBundled'`.
  Non-vacuity: replacing `lock.LoadLock(lockPath)` with `lock.NewLock()` (dropping the lock normalization)
  made `legacy_copies_removed_and_lock_normalized` and `lock_without_meta` fail with
  `project: ai-specs/.ai-specs.lock bytes:`; reverted, and `git diff internal/sync/refreshbundled.go` shows
  no break.

  OPEN — three PATH-stub suites lost their injection point because this step is now native. They were not
  deleted or weakened (all three files are outside this unit's allowed edit surfaces), and each needs one
  re-point to a still-Python sync step (`vendor-skills.py`, until T5 ports it):
  1. `internal/sync/agentsrender_test.go:910` `TestSyncAgentsStepIsNative`: its non-vacuity guard
     `if !strings.Contains(logged, "refresh-bundled.py") { t.Fatalf("python3 shadow intercepted nothing...") }`
     (lines 964-967) now fails; the log correctly contains only `vendor-skills.py`. Needs the guard to
     require `vendor-skills.py` instead. This is the only `go test ./internal/sync/` failure after T4.
  2. `tests/test_sync_run_step_errexit.py`: `STEP_ONE = "refresh-bundled.py"` (line 39) plus
     `STEP_ONE_LABEL` (line 41) feed `stubs.intercept(STEP_ONE, ...)`; 7 tests fail, e.g.
     `test_a_failing_step_keeps_the_wrapped_status` `self.assertEqual(proc.returncode, 7, proc.stderr)`
     (line 87) now observes 0 and `test_a_bare_failing_step_keeps_the_wrapped_status` asserts
     `self.assertNotIn(STEP_TWO_LABEL, proc.stdout)` on output the stub no longer injects. Needs
     `STEP_ONE = "vendor-skills.py"` + `STEP_ONE_LABEL = "  syncing vendored skills"` (the contract is
     step-independent by design).
  3. `tests/test_sync_output_verbosity.py`: `StepOutputContractCliTests.FIRST_STEP = "refresh-bundled.py"`
     (line 279) with `FIRST_LABEL` (line 280), and `TemplateSkippedClassificationTests` hard-codes the
     `("refresh-bundled.py", {"rc": 0, "stdout": ...})` intercept tuple (line 865); 7 tests fail, e.g.
     `test_m1_template_skipped_cli_compact_drops_and_verbose_keeps` `self.assertIn("keep-me\n", proc.stdout)`
     (line 869) and `test_t2_4_failing_step_prints_full_output_and_status_both_modes`
     `self.assertEqual(proc.returncode, 7, proc.stderr)` (line 376). Needs the same re-point to
     `vendor-skills.py` / `  syncing vendored skills`.

  Observed after T4: `go build ./...` clean, `gofmt -l internal/sync/` empty, `go vet ./internal/sync/`
  clean; `PATH=/tmp/pybin:$PATH python3 -m unittest tests.test_sync_run_step_errexit
  tests.test_sync_output_verbosity` → `Ran 31 tests ... FAILED (failures=14)`, all 14 the seam loss above.

- L10 — T5 (`vendor-skills`) done on `change/go-20-u2a-code`, 2026-10-22. `internal/sync/vendorskills.go`
  is the port (`SyncVendoredSkills` for the root `[[deps]]` route, `materializeDepSkill` for the
  recipe-dep delegation); `internal/sync/sync.go:276-278` now calls `SyncVendoredSkills(root, r.home, out,
  errW)` in the same `runStep("vendored skills")` position and `internal/sync/materialize_loop.go:88-92`
  calls `materializeDepSkill(req.ProjectRoot, req.AISpecsHome, skill, req.Stdout, req.Stderr)` instead of
  the inline `python3 -c`. The `depSkillDelegationScript` and `delegateDepSkillToPython` are gone;
  `materializeRunPython` itself was NOT touched (T6 owns it). `lib/_internal/vendor-skills.py` and its
  sibling modules stay in place as the differential oracle until U2e. `internal/skills/vendored.go` gained
  `VendoredSkillMarkdownFromMap` (the recipe-dep route has no TOML table), sharing the existing renderer.

  Python contract, quoted from `lib/_internal/vendor-skills.py` and used as the differential target:
  - argv `vendor-skills.py <project_root>`; root `[[deps]]` materialize at
    `{project}/ai-specs/.deps/<id>/skills/<id>`, recipe deps (only `sync_dep_target`, `in_project=False`)
    at `{cache}/.deps/<id>/skills/<id>`, where `{cache}` is `{cli_home}/cache/projects/<cache_key>` and
    `ensure_cache` writes its `meta.toml` sidecar.
  - source resolution: `AI_SPECS_VENDOR_FIXTURE_ROOT` replaces a source containing
    `kepano/obsidian-skills`; a plain local tree (no `.git`) is `copytree`d, otherwise `git clone --depth
    1 --quiet [--branch <ref>]` (a 7-40 hex `ref`/`rev` uses `git init` + `remote add` + `fetch
    --depth 1 origin <sha>` + `checkout FETCH_HEAD`). `path` is `.strip("/")` and selects the `SKILL.md`
    directory; ancillary `assets/`, `references/`, `scripts/` dirs are copied.
  - writes: the target is deleted only AFTER `SKILL.md` is found, then `SKILL.md` is written from
    `render_skill_markdown(from_dep(dep, upstream))` (mode `0666 &^ umask`) and the ancillary dirs copied
    (S6 partial state kept). The root route then hashes every target file (CRLF-normalized) into
    `[deps."<id>".skills."<id>"]` in `ai-specs/.ai-specs.lock` and writes the lock once.
  - stdout: `  ▸ <id>  ←  <source>[  (path: <path>)][  (ref: <ref>)]`, then
    `  ✓ vendored <n> dep(s)`; no deps prints `  (no [[deps]] declared — nothing to vendor)` and writes no
    lock. Recipe delegations print only the `  ▸` line (the caller adds `    ✓ dep skill <id>`).
  - exit: `0` on success; `1` + `  ✗ <msg>` on a missing/invalid dep or a clone/`SKILL.md` failure. Python's
    uncaught filesystem/git errors are a traceback; the port keeps the same exit code with one
    `  ✗ <msg>` line (L6 class). Git's own stderr is forwarded on both legs.

  Tests. `internal/sync/vendorskills_test.go` adds `TestSyncVendoredSkillsDifferential` (twin projects, real
  module vs native: no deps, a local git dep with `ref = 'v1'`, a fixture dep with `path` +
  `references/`, a missing source) comparing exit code, stdout, stderr, lock bytes and the whole project
  tree (bytes/modes); `TestRecipeDepSkillDifferential` compares the native recipe route against the real
  `sync_dep_target(in_project=False)` (stdout + cache `.deps` tree); `TestSyncVendoredSkillsStepIsNative`
  runs the spine with a PATH-shadowed `python3` that logs every invocation. RED before the `sync.go`
  change: `vendorskills_test.go:268: sync still execs vendor-skills.py; log: .../lib/_internal/vendor-skills.py
  .../project`. GREEN after: all three suites pass and `ok ai-specs.dev/ai-specs/internal/sync` for
  `-run 'Vendor|RecipeDep'`; `internal/skills` `-run 'Vendor'` stays green. Non-vacuity: appending `"x"` to
  every lock hash made `dep_with_path_and_ancillary_dirs` fail with `project: ai-specs/.ai-specs.lock
  bytes:` (`"SKILL.md" = "...9ce5x"` vs `"...9ce5"`); reverted, and the line is back to
  `hashes[filepath.ToSlash(rel)] = sum`.

  L10 — PATH-stub seam losses after T5 (all outside this unit's allowed edit surfaces; NOT deleted or
  weakened). No Go→Python sync child remains on the `sync` spine now, so no still-Python step exists to
  re-point them to:
  1. `internal/sync/refreshbundled_test.go:74` `TestSyncBundledStepIsNative`: now fails at
     `read python3 invocation log: ... no such file or directory`; its non-vacuity guard is
     `if !strings.Contains(logged, "vendor-skills.py") { t.Fatalf("python3 shadow intercepted nothing; log:\n%s", logged) }`.
  2. `internal/sync/agentsrender_test.go:961` `TestSyncAgentsStepIsNative`: same read-log failure; guard
     `if !strings.Contains(logged, "refresh-bundled.py") { ... }` (L9 item 1, still open).
  3. `tests/test_sync_run_step_errexit.py`: `STEP_ONE = "refresh-bundled.py"` (line 39) and
     `STEP_TWO = "vendor-skills.py"` (line 40) both name now-native steps; the 7 tests listed in L9
     (e.g. `test_a_failing_step_keeps_the_wrapped_status` asserting `proc.returncode == 7`) can no longer
     intercept a sync step.
  4. `tests/test_sync_output_verbosity.py`: `StepOutputContractCliTests.FIRST_STEP = "refresh-bundled.py"`
     (line 279) and the `("refresh-bundled.py", ...)` tuple (line 865) are both now-native; the 7 tests
     listed in L9 cannot intercept a sync step.
  Parent action: these suites need a deliberate test-only seam (or retirement) in a unit that owns those
  files; T5 did not invent one.
