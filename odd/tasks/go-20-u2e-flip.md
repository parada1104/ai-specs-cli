# Go 20 / U2e — flip, parity corpus snapshot, Python deletion (feature)

Program: `odd/tasks/go-20-python-exit.md`, item U2e. Plan: `odd/tasks/go-20-u2-launcher-flip.md` (U2e section).
Branch `change/go-20-u2e-flip-1`, base `epic/go-single-binary` @ `5e98caf` (U2d closed, PR #431).
Slice 1 = documentation and measured inventory only. No source or test change in this slice.

## Specs

- S1 — Program rule, verbatim from the program doc: "zero Python under the product tree; bash only as
  wrappers."
- S2 — U2e scope, verbatim from `odd/tasks/go-20-u2-launcher-flip.md` (U2e): "the flip and the deletion:
  `bin/ai-specs` execs the compiled binary for every verb; the parity corpus is snapshotted and extended per
  S7; `lib/_internal` and the five differentials that run Python (`config`, `deps`, `gatebinary`, `schema`,
  `target`) are deleted; the contract (§7, D1, D9, D13, D14, D26) is rewritten with the accepted divergences
  of S4. `validate.sh` runs once on the result."
- S3 — Parity gap rule, verbatim from U2 S7: "U2e must either extend it to those verbs or record each gap
  explicitly. Silent gaps are not acceptable." Gaps: hub, upgrade, skills, add-dep, configure-recipes,
  standalone refresh-bundled, recipe remove, recipe configure.
- S4 — Accepted divergences, verbatim from U2 S4 ("aceptar ambas como contrato"), joined to the contract in
  U2e: (1) gate binary receipt without a committed digest: Go refuses, Python accepts; (2) TOML non-bare keys:
  Go quotes, Python writes raw; (3) datetimes: Go refuses, Python corrupts to strings. The doctor malformed
  manifest message is the native one (U2c).
- S5 — Trello card for U2e: none exists on board `69ec097f13e2d38ecd89a557` (lists Backlog, Ready, In Progress,
  Review, Testing interno, Done checked with filters "U2" and "Go 20"). Human decision, verbatim: "Seguir sin
  card". No card is created here; Trello comments and readback are pending until the human creates or links it.
- S6 — Escalation rule, verbatim from the parent request: "Escalame si una regeneración de sums tocara el trust
  root de release." Applies to the gate digest regeneration owned by D-U2d-9, not to this slice.
- S7 — Out of scope for this slice: the flip itself, corpus extension, deletions, contract rewrite. D-U2d-9
  stays registered in `odd/tasks/go-20-u2d-tests.md` and Engram 245, outside this feature.

## Tasks

- **T1** — Measure the U2e inventory (S2, S3). Route: inline (read-only measurement). Commit: 7428268 → 182f96b (PR #432).
- **T2** — Flip: `bin/ai-specs` execs the native binary for every verb (13 `bash "$LIB_DIR/…"` dispatch lines,
  `bin/ai-specs:38-50`). Blocked until T3 and the named test rows are native. Commit: pending.
- **T3** — Retarget the U2e test rows to the native binary, no assertion removed. Split into named units (L6):
  - **T3a** `test_upgrade_output` (oracle seed `lib/_internal/changelog.py`). Slice 2. Commit: e4bccc7 (PR #433).
  - **T3b** `AgentsSubmenuManifestTests` (`lib/_internal` loader, no CLI surface). Go coverage in
    `internal/hub/interactive_test.go` (D12 cases) and `internal/config/agents_test.go`. Route: map each Python case to a Go test, then retire the Python class with the equivalence recorded (L12). Slice 3. Commit: 4b82b37 → e5f40a4 (PR #436).
  - **T3c** `_materialize_process` (`lib/_internal/recipe-materialize.py`). No native `materialize` verb; the native
    chain is `baseline-brief` (`internal/cli/brief.go`). Route: rewrite against the real surface. Slice 4, branch
    `change/go-20-u2e-flip-4` (base c2edbbf). Scope: L15; hooks part closed in Go: L16. Commit: efa7b63 (PR #438).
  - **T3d** `test_init_brief_route` (LEGACY_CLI; bash `lib/init.sh` seam via `AI_SPECS_GO_BIN`). Route: golden fixtures
    for the legacy comparison (L6). Commit: pending.
  - **T3e** `test_init_native` (legacy vs native oracle). Route: golden fixtures frozen from current legacy output (L6). Commit: pending.
- **T4** — Extend the parity corpus to the S3 gaps or record each gap (`tests/parity/parity.py`). Commit: pending.
- **T5** — Replace the five Python differentials (`config`, `deps`, `gatebinary`, `schema`, `target`) with native
  checks that keep their assertions; then delete `lib/_internal`. Commit: pending.
- **T6** — Rewrite the contract (§7, D1, D9, D13, D14, D26) with S4. Commit: pending.
- **T7** — Decide the generated-hook `python3` (see L4). Human decision: the generated Cursor wrapper must not invoke
  python3 (L10). Slice T7 (branch `change/go-20-t7-hook-no-python`, base e4bccc7): deny escaping by POSIX awk in
  `internal/sync/hooksrender.go`, 3 behavior tests, and the differential reference updated with an intentional
  divergence marker (L10). Commit: a958124 (PR #434).
- **T9** — `binaryEntry` under a symlinked home (`internal/upgrade`, same class as D-U2e-1, L14). The post-flip binary
  install is compared as `EvalSymlinks(LocalBin)` against an unresolved `BinaryPath`, so it reports "not the standard
  global installation". Named unit, approved by the human; NOT implemented yet. Runs after T3c–T3e close, in plan order.
  Commit: pending.
- **T8** — `validate.sh` once on the result, native assess per candidate, PR with auto-merge. Commit: pending.

## Log

- L1 — Parent request, verbatim (2026-10-24): "Continúa la migración Python→Go: U2d quedó cerrada (PR #431 →
  5e98caf, card #189 Done, Engram 245, doc L18-L20 + deuda D-U2d-9). Recupera de Engram 241/245 y
  odd/tasks/go-20-u2d-tests.md + el programa de U2 el alcance de U2e, y ejecuta su primer slice con el flujo
  habitual: worktree propio, TDD, TMPDIR/scratch/Python fijados, assess por candidato, validate.sh una vez, PR
  al épico con auto-merge autorizado, nunca development. Límites: no crees nuevas cards en Trello — la decisión
  de Robert sigue pendiente (mi pregunta expiró sin respuesta, no la tomes ni por aprobada ni por rechazada);
  D-U2d-9 sigue registrada en doc y Engram, fuera de este slice. Si U2e carece de card existente para
  comentarios/readback, repórtamelo en cuanto lo compruebes en vez de crearla. Escalame si una regeneración de
  sums tocara el trust root de release. Comienza por recuperar estado y confirmarme el alcance y la card de U2e
  antes de implementar."
- L2 — Human answers in the same session (verbatim): slice = "Doc + inventario (Recomendado)"; card = "Seguir
  sin card".
- L3 — Measured inventory (epic 5e98caf, worktree `change/go-20-u2e-flip-1`):
  - 49 test modules touch `lib/_internal` (list in scratch `u2e/internal-users.txt`).
  - 51 test modules touch the launcher or `lib/*.sh` (scratch `u2e/launcher-users.txt`).
  - `tests/parity/parity.py` defines 29 fixtures. Verbs covered: init, sync, sync-agent, doctor, rules-audit,
    help, version, recipe list/init/add, `bogus-verb`. Not covered (S3 gap): hub, upgrade, skills, add-dep,
    configure-recipes, refresh-bundled standalone, recipe remove, recipe configure.
- L4 — Python execution on the product path, measured. No direct `exec` of `python3` remains in active non-test
  Go code. Findings:
  - `internal/sync/hooksrender.go:771` emits `python3 -c` inside the generated Cursor hook script. This is product
    output, a runtime dependency on the user's `python3`, not a Go-side exec. Decision needed (T7).
  - `internal/cli/recipe.go:2205` (`configureFrontDoor`) and `internal/hub/interactive.go:205` (hub `delegate`)
    run `<home>/bin/ai-specs`. After the flip the launcher execs the binary, so these hops stay valid but add
    one process each. Recorded as T2 dependencies.
  - `internal/cli/shim.go:36` runs `bash`. Dead code since U2b (L7 in `go-20-u2b-init.md`); deletion is U2e.
  - `internal/sync/sync.go:132-137`: `stepMode` defaults to `"python"` and its comment says the default "still
    execs the remaining modules". Not yet verified whether any step still reaches Python. Open for T2/T5.
  - `internal/target/resolve.go:35` keeps `programName = "lib/_internal/target-resolve.py"` only for the usage
    string on stderr. Deleting `lib/_internal` requires changing it (parity row).
- L5 — Trello: no card for U2e (S5). Engram 245 open item 1 (no card for D-U2d-9) also stands.
- L6 — Slice 2 scope, human answers (verbatim): scope = "Solo las que encajan" (only T3a this slice; T3b-T3e
  named units, not touched here); oracle for comparative rows = "Fixtures golden" (frozen from current legacy
  output when their turn comes). T1 entry completed in this PR's doc commit: 7428268 → 182f96b.
- L7 — Evidence, product defect (proposed debt D-U2e-1, not registered in Engram yet): on macOS, `TMPDIR` lives
  under the `/var` → `/private/var` symlink. Baseline `test_upgrade_output` failed 9 of 11 before any change with
  "~/.local/bin/ai-specs resolves outside ~/.ai-specs": `internal/upgrade/upgrade.go:253` compares the
  `EvalSymlinks` target against the unresolved `ExpectedHome`. Not fixed here (product code, out of slice). The
  fixture now canonicalizes its HOME with `realpath`. Robert to decide registration.
- L8 — Human decision for `test_upgrade_notices.test_missing_parser_does_not_break_the_upgrade`, which used the
  seed as its mutation target ("Mutar sin semilla"): the lambda is `unlink(missing_ok=True)`. The mutation is no
  longer observable; the test keeps checking that the upgrade survives a missing parser. Its module also needs
  `PYTHONPATH=tests` (pre-existing import by bare name, same on the epic base).
- L9 — Human correction to L8 ("Semilla local al test"): L8's "Mutar sin semilla" was reverted because with the
  seed absent `unlink(missing_ok=True)` is a no-op and silently drops the missing-parser coverage. The shared fixture
  stays unseeded; only `test_missing_parser_does_not_break_the_upgrade` seeds `lib/_internal/changelog.py` into the
  base install and origin, and its mutation is strict `unlink()` again. Shared-helper consumers of `setup_install`
  (inventoried before any edit): `test_docs_drift_fixes.py`, `test_upgrade_notices.py`, `test_upgrade_output.py`;
  none changed. The `validate.sh` run started on the L8 state was stopped and is re-run once on the final state.
- L10 — Human decisions, T7 (verbatim answers): (1) the generated Cursor hook wrapper must NOT invoke python3; the
  deny message is escaped with POSIX awk (`LC_ALL=C`), with non-ASCII UTF-8 emitted raw (option "UTF-8 crudo";
  json.dumps would emit \uXXXX for non-ASCII, a deliberate divergence only for non-ASCII messages). (2) When the
  differential reference blocked the slice, the decision was "Actualizar la referencia": `internal/sync/testdata/
  hooksrender_ref.py` gets `_apply_t7_cursor_escape`, which rewrites exactly the lib renderer's python3 deny line in
  generated `.cursor/hooks/*.sh` files to the Go line. This is an INTENTIONAL divergence from
  `lib/_internal/hooks-render.py`, valid until `lib/_internal` is deleted (T5). `lib/_internal` is not touched; no
  differential rows are retired; the rest of the Cursor differential stays byte-for-byte. Evidence: differential
  97 subtests PASS (29 cursor), 0 FAIL; the 3 new tests PASS. Scratch artifacts live under the session TMPDIR.
- L11 — T7 validate failure and decision (verbatim answers: "Normalizar en paridad", with strict conditions). The
  legacy-vs-go parity corpus (cache-layout and hooks-five-runtimes, both gate modes) compared the Cursor wrapper
  `.cursor/hooks/worktree-flow-worktree-gate-shell.sh`, which now carries the Go awk line while the legacy leg still
  emits python3. Authorized: a single exact-string tree normalization `N10-t7-cursor-deny-line` in
  `tests/parity/parity.py`, canonicalizing the two exact deny lines on both legs to one marker. Nothing else in
  parity.py changed (diff: +32 lines, 0 deletions). BOUNDARY: intentional divergence, Robert's T7 decision, valid
  only while lib/_internal exists. EXPIRY: T5, when lib/_internal is deleted; then delete N10, `_T7_*` constants and
  the reference rewrite together. No parity rows retired; T7 not delayed.
- L12 — Slice 3 (T3b), equivalence map for `AgentsSubmenuManifestTests` (4 cases, `tests/test_docs_drift_fixes.py`,
  retired; the Python loader `lib/_internal/hub.py` is deleted at T5):
  - "agents table without enabled is completed, not duplicated" (`[agents]` bare, last) →
    `internal/config` `TestSetEnabledAgentsCompletesEmptyTableAtEOF` (new in this slice; mutation of the EOF branch
    fails it and `TestSetEnabledAgentsKeepsAgentsAsLastTable`, then restored).
  - "agents table last with other keys gets enabled" → `TestSetEnabledAgentsKeepsAgentsAsLastTable`.
  - "existing enabled line is replaced" → `TestSetEnabledAgentsReplacesExistingEnabledLine`; hub end-to-end
    `TestAgentsReplacesExistingEnabledLine`.
  - "missing agents table is appended once" → `TestSetEnabledAgentsAppendsOneAgentsTable`; hub
    `TestAgentsAppendsOneTableWhenAbsent`.
  The hub-level picker, messages and pause are covered by `internal/hub/interactive_test.go` (D12 rows).
  Evidence: `go test ./internal/config ./internal/hub` PASS; `python3 -m unittest tests.test_docs_drift_fixes`
  27 → 23 tests (the 4 retired), 1 failure `test_dirty_tree_abort_renders_real_newline` ALSO on the epic base
  without this slice (same run, `failures=1`): `resolves outside ~/.ai-specs`, the D-U2e-1 defect (L7), not
  caused here and not fixed here. Scope vs plan: unchanged (retire + map, as T3b states); one Go case added
  because the Go side lacked the EOF-empty-table row.
- L13 — D-U2e-1 resolved before this slice lands: fix PR #435 merged to the epic as 112d91f (container-only
  canonicalization of `~/.ai-specs` in `internal/upgrade`; regression test for a symlinked home, and a guard test
  that keeps a `~/.ai-specs` pointing outside refused). This slice rebased onto 112d91f with no product change.
  L12's "not fixed here" note is superseded. Follow-up not in scope here: `binaryEntry` (post-flip binary install)
  has the same unresolved comparison under a symlinked home; needs its own decision.
  Gate: T3b does not merge with a red suite. validate.sh is re-run once on the integrated tip (see PR body).
- L14 — Human decisions after slice 3 (verbatim intent): (1) PR #436 merged manually by squash with delete-branch,
  only after the head SHA matched the validated commit (4b82b37, validated on 112d91f), base = epic, diff = tests
  and doc only, assess reviewDue false. Repo auto-merge setting is administrative and not part of this feature.
  (2) T3b commit hash recorded here in the next doc PR (as T1). (3) `binaryEntry` under a symlinked home approved as
  named unit T9, same class as D-U2e-1 (L7/L13), not in any merged PR, not implemented; to run after T3c–T3e close,
  in plan order. Engram records the same class.
- L15 — Slice 4 (T3c), equivalence map for `_materialize_process`. Native observable: `baseline-brief <root>
  --resolved-config-out <path>` (`internal/cli/brief.go`, `internal/sync/baselinebrief.go`) writes the resolved-config
  document through the same spine. Rewritten to `_baseline_brief` (6 of 8 call sites; 229 assertions before and after,
  none removed): `_resolved_context` (x2 tests), `_resolved_bindings`, the reconcile carry, `_resolved` (x4 tests),
  `test_resolved_config_only_also_carries_context`, `test_build_resolved_config_only_attaches_brief_fragments`.
  Equivalence: a scratch oracle (not committed) ran both legs on every rewritten call site: 11 resolved-config
  comparisons, rc 0 on both legs, documents equal, including the two `--resolved-config-only` ones. The -only mode
  itself is no longer invoked by the suite; it is pinned natively by `internal/sync/resolvedconfigonly_test.go`.
  Mutation (real, compiling): `internal/sync/resolvedcatalog.go` writes `brief_fragments` as `resolvedFragmentsJSON(nil)`
  makes 6 tests fail, including the rewritten brief-fragments test; reverted. A first mutation (deleting the line)
  did not compile and was discarded; its failures were not evidence.
  STAYS ON THE LEGACY PROCESS: `test_resolved_hooks_out_shape` and `test_resolved_hooks_env_carries_config`. Their
  contract is the `--resolved-hooks-out` document, which has no native verb: `MaterializeRecipes` writes it only to
  the in-process `ResolvedHooksOut` (`internal/sync/materialize.go`, `sync.go:344`), and its only oracle is the Python
  differential (`materialize_test.go`). This keeps `lib/_internal/recipe-materialize.py` needed until T5 has a native
  replacement. Decision needed before T5: a Go test on `MaterializeRecipes` for the hooks document, or a native flag
  (a product change, not requested here).
  Pre-existing, not touched: 14 type-checker notices on `_cap_recipe(caps=[...])` (list passed to a `tuple` hint,
  test lines 478-512); the unittest suite passes. SUPERSEDED by L16 for the hooks part (see below).
- L16 — Decision on L15's open hooks item (option a, Robert's answer). Hooks document, `--resolved-hooks-out`:
  a Go test on `MaterializeRecipes` replaces the two retired Python process legs. Files: `internal/sync/materialize_hooks_test.go`
  (`TestResolvedHooksDocumentShapeMatchesLegacy`, `TestResolvedHooksEnvCarriesExportedConfigOnly`) and golden files
  `internal/sync/testdata/materialize-hooks/{shape,env}.golden.json`, captured from the legacy `recipe-materialize.py`
  process on the same fixtures (deterministic, two runs byte-identical). The test compares the spine document and the written
  file byte for byte with the golden, then keeps the retired assertions as explicit field checks: `enabled_agents` = [claude],
  one hook, `recipe`, `id`, `event`, `matcher`, `blocking` = true, `script_path`; and for the env case `env` exports
  `WORKTREE_GATE_PROTECTED` = "main development" and not `worktrees_dir`. Nothing was loosened to fit the native output: it
  matched the legacy bytes on the first run. Sensitivity (real mutations, reverted): `blocking` forced to false and env emptied
  each fail both tests.
  Python side: `tests/test_recipe_materialize.py` no longer runs any Python process. The two `--resolved-hooks-out` legs are retired
  with this mapping; their end-to-end `sync` assertions on `.claude/settings.json` (managed identity, matcher, command, env) stay.
  `_materialize_process` is deleted (no callers left). The module docstring now states that no test runs Python.
  Also: `_cap_recipe` signature accepts `list[str] | tuple[str, ...]`; the 14 type-checker notices from L15 are resolved (type hint only,
  no behavior change; 73 tests still pass).
  Assertion count (measured with `grep -c self.assert`): the Python module went 229 at c2edbbf → 217 now. The two retired tests held
  19 `self.assert` lines at a4063a1; 7 remain (their end-to-end `sync` checks). The 12 removed are the document checks above: shape
  returncode, `enabled_agents`, hook count, `recipe`, `id`, `event`, `matcher`, `blocking`, `script_path` (9, moved to
  `TestResolvedHooksDocumentShapeMatchesLegacy`, where the returncode becomes the `MaterializeRecipes` error check); env returncode,
  `WORKTREE_GATE_PROTECTED`, `worktrees_dir` (3, moved to `TestResolvedHooksEnvCarriesExportedConfigOnly`).
  Effect on T5: `tests/test_recipe_materialize.py` no longer depends on `lib/_internal`. The remaining `lib/_internal` users are
  other suites, unaffected by this slice.
  Evidence: `PYTHONPATH=tests python3 -m unittest tests.test_recipe_materialize` 73 OK; `go test ./internal/sync -count=1` OK; the
  golden-hooks tests PASS; LSP diagnostics clean on the test file.
  Process note: a background `validate.sh` started on a4063a1 was stopped before it finished. It ran on a worktree I was editing, so
  its result is not evidence. One `validate.sh` run is made on the final head.
