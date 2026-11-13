# GO-20 / U2e T6 — rewrite the parity contract to the post-migration truth

Program: `odd/tasks/go-20-python-exit.md`, item U2e. Feature: `odd/tasks/go-20-u2e-flip.md`, task T6.
Card: #191 (epic umbrella). Branch `change/go-20-u2e-t6-contract`, base `epic/go-single-binary` @ `ae9a4c99`
(GO-20 U2e T5 PR4b, `lib/` deleted, PR #527).

**Documentation-only slice.** Two files: `docs/go-migration-parity-contract.md` and this record. No code, test,
golden or manifest is touched; nothing is staged or committed.

## Specs

- **S1** — The card's T6 line, verbatim (quoted from the parent handoff; no tracker tool was available in this
  session to read #191 directly): "Rewrite the contract (§7, D1, D9, D13, D14, D26) with S4 divergences.
  Includes L26: resolved symlink path accepted as divergence; parity runs with canonical TMPDIR."
- **S2** — L26 of `odd/tasks/go-20-u2e-flip.md:320-322`, verbatim: "Paridad y TMPDIR (finding, not fixed in
  PR-B). Under `TMPDIR` in `/var/folders/...` (macOS `/var` → `/private/var`), `tests/parity/run.py` reported
  `gate-absent failing=28, gate-present failing=28`. Diff: the Go doctor prints `target: /private<TEMP>` while
  the leg it is compared with prints `target: <TEMP>`. Canonicalized scratch (`TMPDIR=/private/tmp/...`):
  `failing=0` on both gate modes, which is what the handoff expected." and "The Go binary resolves symlinked
  paths in its output while bash did not: same class as D-U2e-1 (L7/L13). A real divergence on symlinked
  homes, to decide, not normalize here."
- **S3** — L28's operational rule, verbatim (`go-20-u2e-flip.md:335`): "Cause: the path form, not the content.
  Run parity and validate with a canonical TMPDIR."
- **S4** — The accepted divergences joined to the contract, verbatim from U2 S4 (as recorded in
  `odd/tasks/go-20-u2e-flip.md` S4): "(1) gate binary receipt without a committed digest: Go refuses, Python
  accepts; (2) TOML non-bare keys: Go quotes, Python writes raw; (3) datetimes: Go refuses, Python corrupts to
  strings. The doctor malformed manifest message is the native one (U2c)."
- **S5** — Reconciliation rule (parent request): the contract must describe the FINAL observed state, not the
  migration-era expectation; where the flip doc's expectation was superseded by a measurement, the measurement
  wins and the difference is recorded.
- **S6** — Boundary: a contract claim that is factually wrong in the CODE is reported here, never fixed in code;
  only the named sections plus the summary are edited.

## Tasks

| ID | Work | Status |
|---|---|---|
| T1 | §7 rewritten: four TTY surfaces with code evidence, the `recipe add` write-order correction, and the `configure-recipes` no-TTY divergence record | done |
| T2 | Settled TOLERANT records added in §10: empty config key (S14-Δ D1), D9, D13, D14, D26, resolved symlink paths (L26) | done |
| T3 | §9 rows D1, D9, D13, D14, D26 rewritten to the post-migration state; the §9 preamble states what CLOSED means | done |
| T4 | §12 rewritten: frozen goldens vs live Go, the N-rule status, `lib/` deleted, zero production Python | done |
| T5 | This record: specs, evidence table, reconciliation notes | done |

## Evidence table (contract claim → source)

| Contract claim (as now written) | Source |
|---|---|
| `lib/` is gone; the launcher only resolves and execs the native binary | `bin/ai-specs:1-33`; `git ls-files lib` is empty; `ae9a4c99` |
| No production Go path spawns an interpreter | `internal/upgrade/upgrade.go:68`, `internal/target/topology.go:91`, `internal/doctor/projectcache.go:140`, `internal/doctor/checks_tracker.go:255`, `internal/gatebinary/gatebinary.go:563,596,609`, `internal/deps/deps.go:185` (grep `exec.Command\|exec.LookPath`, non-test, non-testdata) |
| `init` TUI gate | `internal/cli/init.go:303-318` (`shouldInitTUI`) |
| `hub` TTY matrix, and uninitialized + non-TTY exits 2 with nothing to launch | `internal/hub/hub.go:198-211`, `:198-203` |
| `configure-recipes` requires a TTY, exit 3, message | `internal/cli/recipe.go:2890-2897` |
| `recipe add`'s TTY branch is the config tail and the manifest write already happened | `internal/cli/recipe.go:383-391` (append), `:437-470` (tail) |
| Legacy `configure-recipes` order: `ensure_deps` before the TTY gate | `lib/_internal/config_wizard.py:282-287` @ `ae9a4c99^`; `lib/_internal/util.py:57-76` |
| Legacy silent-exit-3 registration (T4, 2026-10-24) | `tests/parity/parity.py:1851-1863` (the `configure-recipes` "recorded" text) |
| Legacy message shape (rc 3 + exact stderr) | `internal/cli/testdata/smoke_legacy.golden.json`, key `configure-recipes-nontty` |
| D1 (lock data) closed: removal delegates to the lock writer | `internal/lock/mutation.go:81`; `internal/cli/recipe.go:565,697`; `ea61d132` (PR #301, 2026-09-27) |
| D9: `--dry-run` writes nothing | `internal/upgrade/upgrade.go:9-11`, `:276-281`; `TestModeOnlyDirtDryRunThenReal`, `internal/upgrade/upgrade_test.go:347-381`; `a68440af` (2026-09-29) |
| D13: every manifest write is atomic; read-only manifests are refused | `internal/config/writes.go:133-183`, `:156-158`; call sites `writes.go:238,286,333,386,392`, `agents.go:115`, `recipeconfig.go:1358,1445`; `TestSkillsRemoveReadOnlyDeviation` `internal/cli/skills_test.go:862-934`; `TestSkillsAddReadOnlyDeviation` `:1622-1686`; `TestSetEnabledAgentsRefusesReadOnlyManifest` `internal/config/agents_safety_test.go:68-88` |
| D14: the removal paths validate; `skills add` does not (mirrored) | `internal/config/writes.go:101-114`, `:282`, `:329`, `:370-397`, `:185-189` |
| D26: standalone `refresh-bundled` is a no-op; the help text now says so | `internal/cli/refresh_bundled.go:29-81`; `internal/cli/help.txt:8-9`; `6340c27b` (PR #446, 2026-10-24); decision "ah si si, basta con sync" (`odd/tasks/go-20-u2b-init.md` S2/L2, 2026-10-22) |
| `sync`'s refresh step, its title and flatten lines | `internal/sync/sync.go:268-270`; `internal/sync/refreshbundled.go:73-76`; `init`'s header `internal/cli/init.go:662` |
| Empty config key (S14-Δ D1) settled; the inline-config residual is pinned | `catalog/recipes/worktree-flow/gate/shared/mergeconfig.go:295-298`, `:346`; `odd/tasks/go-07-s14-divergences.md`; `08f1174` (PR #349, 2026-10-13); `TestNativeResolvedConfigInlineConfigBeforeFlatKeyDivergence`, `internal/sync/resolvedconfig_test.go:535-587` |
| Resolved symlink paths: `doctor` prints the resolved root | `internal/doctor/doctor.go:130`; `internal/projectcache/projectcache.go:65-74`; `odd/tasks/go-20-u2e-flip.md` L26/L28 |
| Corpus is frozen goldens vs live Go; 70 files, 62 at `6340c27`, 8 from Go | `tests/parity/goldens/**` (`legacy_launcher` field of every file, read programmatically); `tests/parity/capture_goldens.py:1-50` |
| N10/N11 active pending recapture; N12 permanent | `tests/parity/parity.py:196-241` (comments), `:365-437` (rule records), `odd/tasks/go-20-u2e-t5-pr4a-s7b-parity.md` F3 |
| `git clone --depth 1` now pins on `ref`/`rev` | `internal/sync/vendorskills.go:48-57`, `:253-268` |
| `▸ recipe <id>` / `RECIPE_MCP_TEMP:` are plain output lines now | `internal/sync` (return values, GO-07 S16); the greps lived in `lib/*.sh`, deleted |

## Reconciliation notes

- **N-rules (S7b wins over the flip doc's T5 expectation).** The flip doc's T5 line expected N10 and N11 to expire
  with `lib/`. Measured in S7b and unchanged today: dropping them fails three fixtures in both gate modes
  (`cache-layout`, `hooks-five-runtimes` on the `.cursor/hooks/*.sh` tree hash; `vendor-add-clone-failure` on
  stderr), because the frozen bytes carry the canonicalized shapes. They therefore stay ACTIVE pending a corpus
  recapture decision. `N12-gate-cache-size` is PERMANENT: it normalizes the cached gate binary's byte total, which
  moves with every legitimate gate change. §12 states exactly this; it does not copy the stale expectation.
- **The `configure-recipes` divergence has TWO legacy shapes, and both are on record.** The T4 registration
  (2026-10-24) measured the EMPTY-stderr shape; the frozen pre-flip launcher record measures the MESSAGE shape. The
  cause is `ensure_deps`, which returns 3 silently when the vendored UI packages are absent and lets the TTY gate
  print when they are present. §7 records both shapes and the precondition instead of picking one. Not a product
  defect: exit 3 on both sides.
- **§7's `recipe add` claim was wrong, not just stale.** The old text said the dependency gate runs before the
  manifest write. It does not: `AppendRecipeSegments` writes first and the interactive tail (wizard, confirm,
  dependency gate) runs after (`internal/cli/recipe.go:383-391` vs `:437-470`), so a declined confirm leaves the
  recipe registered. This is the "recipe add's frozen gate claim" the program doc already listed as staleness
  (`odd/tasks/go-20-python-exit.md:296-299`).
- **Two registers both called `D1`.** §9's `D1` (lock data loss) and S14-Δ's `D1` (empty config key) are unrelated.
  §9's row now says so and points at §10; the S14-Δ closure is its own TOLERANT record. The card's T6 list ("§7, D1,
  D9, D13, D14, D26") and the parent's "D1 = empty config key" are both satisfied by covering both.
- **Expected-failure inventory (L28), question (a).** NOT reactivated. After PR4b, two rows remain:
  `tests/test_init_brief_route.py:123` (`test_usable_seam_runs_the_native_verbs_and_no_brief_python`) and `:145`
  (`test_native_chain_failure_writes_the_placeholder`), both `@unittest.expectedFailure`, both still waiting on the
  bash init seam that died with `lib/` (L63 recorded them as pending PR4b; PR4b deleted the seam and did not touch
  them). They still fail for an environment reason, so the suite stays green and the rows are inert, i.e. L28's exit
  criterion ("every row must be reactivated and green when its owner lands") is unmet for these two. The other L28
  rows are closed: `test_unknown_recipe_fails` is reactivated and green with D-U2d-9 closed
  (`tests/test_recipe_materialize.py:338-346`), `TestDoctorMissingPathDeviation` lost its `t.Skip` in PR3, and the
  Go `t.Skip` count for that class is 0.
- **Four Go-output goldens, question (b).** Still open as a decision, and measurable: 8 of the 70 goldens carry a
  Go-era `legacy_launcher` (`bb4a87f5` ×4, `a8bc8de3` ×2, `c3e85d33` ×2 — the fixtures `surface-verbs`,
  `all-recipes-enabled`, `skills-surface`, `nested-fanout-suppression` in both gate modes) while the other 62 carry
  `6340c27b`. Each regeneration had its own approval (L32, L35, L36) but there is no standing decision that the
  corpus mixes two oracle kinds, and `tests/parity/parity.py:1-13` still calls the whole corpus the legacy side.
  §12 states the split honestly; deciding whether those four fixtures become Go-only fixtures or get a recapture is
  still the human's.
- **`capture_goldens.py` labeling, question (c).** Fixed by S7b and verified today: the module docstring no longer
  claims a legacy capture, and `main()` refuses with exit 2 instead of freezing native output over the corpus
  (`tests/parity/capture_goldens.py:1-50`).
- **Found factually wrong in the CODE/docs but NOT fixed here** (outside the named sections, reported per S6):
  1. §1's command table still names `lib/*.sh` entry points for all 14 verbs; the entries are now `internal/cli`.
  2. §3's "whole-file `\n{3,}` collapse — FROZEN as observed" and §9's `D2` row describe heredocs that no longer
     exist: the collapse was removed by `ea61d132` and the Go `joinSegments` does not collapse
     (`internal/config/writes.go:91-99`). §12 now says the paragraph describes frozen legacy bytes only.
  3. §6's `doctor` row still says doctor writes `__pycache__` into `lib/_internal/`; there is no Python and no
     `lib/` (`internal/doctor/doctor.go:11`). What is still true is the EXECUTION of the gate binary's
     `--selftest` and every dep `version_check` through `sh -c`.
  4. §8's "`skills add` has no revision pinning" and §9's `D16` row are contradicted by the code: `ref`/`rev` are
     read, a sha-shaped ref is fetched and checked out, anything else becomes `--branch`
     (`internal/sync/vendorskills.go:48-57`, `:253-268`). §12 states the corrected fact.
  5. §11's `AI_SPECS_INIT_TUI_PY` and `PYTHONDONTWRITEBYTECODE` rows describe a tree that no longer exists.
  6. `tests/parity/parity.py:1867` ("contract line 258") is a stale line pointer into the contract; the D26 text
     moved in this slice. Not edited (outside the allowed surfaces).
- **Not verified in this session, deliberately:** no build, no `go test`, no `./tests/validate.sh`, no parity run —
  the slice is documentation-only and the parent's verification contract asks for evidence citation and repo
  cross-checks instead. Every changed claim was checked against the code or the frozen data by reading/grepping.

## Log

- L1 (Robert): "ok avancemos" — proceed with T6, the contract rewrite.
