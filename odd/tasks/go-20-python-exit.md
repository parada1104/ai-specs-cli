# Go 20 — Python exit: the binary as the only entry point

Program document (like `go-single-binary-epic.md`): it fixes the END STATE and slices the remaining
work into units. Base `ecfb843` on `epic/go-single-binary`. Worktree
`.worktrees/go-20-python-exit`, branch `change/go-20-python-exit`.

## Specs
- S1 — Session request (verbatim): "decisión 1 me gusta la 4 al final es 'un nuevo inicio' ya con esto
  mas limpio sin tanto python+bash+go podria ser solo go +bash como wrappers" y "quiero que este como
  si se hubiera hecho con go desde 0".
- S2 — Decisions 2 and 3, confirmed by the human as recommended: **B2** — flip the entry point first
  and delete in a following unit, with the caller audit as the gate; **C2** — the parity harness keeps
  an explicit legacy entry/flag so the shell route stays runnable from the live tree.
- S3 — The compiled binary is the ONLY entry point: it is installed as `~/.local/bin/ai-specs` and the
  bash launcher stops being the entry. `AI_SPECS_HOME` stops being "a git checkout you run" and
  becomes the data home (catalog, cache, and `lib/` only while unported verbs still need it).
- S4 — "Como si se hubiera hecho con Go desde 0": zero Python under the product tree; bash exists only
  as wrappers (the shims of unported verbs, the catalog recipe hooks) and never as the orchestrator of
  a verb.
- S5 — No observable behaviour changes without evidence: every unit carries RED→GREEN, its
  differential (parity corpus and/or `tests/validate.sh`) and its native review. The suite counts and
  the parity fixture set are quoted per unit, never asserted.
- S6 — Review budget rules the slicing: one unit = one small PR against the epic with its own review;
  a verb port is sliced further when it does not fit one review.
- S7 — Measured scope at `ecfb843`: **19 bash scripts / 3.039 lines** and **43 Python modules /
  21.346 lines** under `lib/`; of the 19 verbs, 9 are native and 7 are still shims (`hub`, `init`,
  `add-dep`, `skills`, `recipe`, `configure-recipes`, `upgrade`).
- S8 — The program is finished when: `ai-specs` (the binary) runs every verb natively; `lib/` has no
  Python; every surviving bash file is a thin wrapper that calls the binary; the install/upgrade path
  installs and updates a binary install kind; and the parity harness still has a legacy reference.
- S9 — Install source (human decision, verbatim option): "Release verificado + build local de
  respaldo" — `install.sh` downloads the published release asset and verifies it against its
  SHA256SUMS (the same verified-artifact pattern the gate binary already uses), and only builds with
  `go build` when there is no asset for the platform or no network; the native upgrade path uses the
  same channel and the same verification. The binary is installed as the entry point
  (`$INSTALL_BIN/ai-specs`), not symlinked into a checkout, and `AI_SPECS_HOME` remains the data home
  (catalog, cache, and `lib/` only while unported verbs still need it).

## Measured scope

Native verbs today: `refresh-bundled`, `sync`, `sync-agent`, `doctor`, `rules-audit`,
`brief-render-policy`, `baseline-brief`, `version`, `help`.
Shimmed verbs and what they drag in:

| Verb (shim) | bash | Python invoked by those scripts |
| --- | --- | --- |
| `doctor` | `doctor.sh` 46 | `doctor.py` 1560 |
| `rules-audit` | `rules-audit.sh` 44 | `rules-inventory.py` 557 |
| `hub` | `hub.sh` 53 | `hub.py` 689 |
| `init` | `init.sh` 348 | `init_tui.py` 429, `env_scaffold.py` 667, `envrc-scaffold.py` 46, `refresh-bundled.py` 160, `cli_version.py` 302, plus the sync-side renderers already native |
| `add-dep` | `skills-add.sh` 222 | (registration is inline; its sync leg goes through the seam since S19a) |
| `skills` | `skills.sh` 39, `skills-list.sh` 275, `skills-remove.sh` 217 | `project-cache.py` 679, `lock.py` 490 |
| `recipe` | `recipe.sh` 42, `recipe-add.sh` 59, `recipe-init.sh` 59, `recipe-list.sh` 49, `recipe-remove.sh` 203 | `recipe-add.py` 302, `recipe-init.py` 272, `recipe-list.py` 129, `recipe-configure.py` 590, `recipe-read.py` 134 |
| `configure-recipes` | `recipe-config.sh` 49 | `config_wizard.py` 300 |
| `upgrade` | `upgrade.sh` 386 | `changelog.py` 351 (the native engine exists in `internal/upgrade`) |
| (sync route, already native) | `sync.sh` 299, `sync-agent.sh` 551 | `recipe-materialize.py` 4265, `agents-render.py` 881, `hooks-render.py` 481, `mcp-render.py` 315, `target-resolve.py` 207, `flatten-resolved-skills.py` 56, `gitignore-render.py` 64, `gitignore-root-refresh.py` 77, `toml-read.py` 223, `vendor-skills.py` 236, `refresh-bundled.py` 160, `cli_version.py` 302, `env_scaffold.py` 667, `project-cache.py` 679, `lock.py` 490 |
| catalog assets | `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`, `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | the ledger/spec-promotion helpers those hooks call |

Two findings that shape the order:

- The binary ALREADY shims the unported verbs (`internal/cli/shim.go`), so A4 is **stageable**: the flip
  changes where the binary comes from and who is the entry, and the bash behind it stays as an internal
  detail until each verb is ported. Nothing forces a big-bang verb port.
- `lib/sync.sh` stays alive after the flip for one reason only: `lib/upgrade.sh` (still a shim) calls it.
  Under the binary the seam is set, so that call can go native in the same unit the `upgrade` route is
  wired — which is what unlocks deleting `sync.sh`, `sync-agent.sh` and the sync-side Python.

## Tasks
- T1 — **U1 (the flip)**, sliced in three because it is three different risks; U1a is delivered:
  - **U1a — DONE** (`cd37c44`, PR #362): the native `upgrade` route is wired, and the two shell
    post-steps the port omits are recorded and pinned.
  - **U1b-1 — the release channel for the CLI binary**: the workflow that already builds and attaches
    the gate's four platform assets also builds and attaches `ai-specs-<os>-<arch>`, with the
    committed trust root the native engine reads and the same local reproduction/verification scripts
    the gate's digests have. Nothing can be verified before this exists.
  - **U1b-2 — the install flip**: `install.sh` stops symlinking a shell launcher and installs the
    binary as the entry (`$INSTALL_BIN/ai-specs`) — download + verify first, `go build` as the
    fallback — with `AI_SPECS_HOME` as the data home, plus the binary install kind in the upgrade
    engine (wiring `Acquire`/`SwapBinary`, today used by nothing).
  - **U1c — the legacy entry for parity** (C2) and `upgrade.sh`'s sync leg through the seam, which is
    what orphans the shell sync.
- T2 — **U2 (the first deletion, after the audit)**: audit `lib/_internal/*.py` callers with the flip
  applied and delete what no live caller reaches (`doctor.py`, `rules-inventory.py`, the sync-side
  renderers and the substrate only the already-native routes used), keeping the parity evidence green.
- T3 — **U3 `hub`**: the interactive launcher (689 lines of Python + a TUI). Includes the decision of
  what replaces the wizard UX now that the parity contract classifies rich/questionary rendering as
  FREE.
- T4 — **U4 `init`**: the manifest write, the scaffold cluster (`env_scaffold.py` 667,
  `envrc-scaffold.py`, `refresh-bundled.py`, `cli_version.py`) and the TUI seam, on top of the
  renderers that are already native.
- T5 — **U5 `skills` + `add-dep`**: the three `skills-*.sh` scripts plus `project-cache.py` and
  `lock.py` usage (both have native cores in `internal/projectcache` and `internal/lock`).
- T6 — **U6 `recipe` + `configure-recipes`**: seven scripts plus `recipe-*.py` and
  `config_wizard.py`, on top of `internal/schema` and `internal/sync` authorities.
- T7 — **U7 the substrate tail**: `util.py` 864, `recipe_schema.py` 864, `gate_binary.py` 739 (whose
  native twin already exists in `internal/gatebinary`), `toml-read.py`/`toml_write.py`, and the two
  catalog hooks that still call Python.
- T8 — **U8 the close**: `lib/` without Python, bash only as wrappers, the install/upgrade ADR, and the
  verification that the suite and the parity corpus prove the end state.

Each task is sliced into review-sized units when it is opened; the sizes above are the reason T4–T6
cannot each be one PR.

## Log
- L1 — The human's decision (verbatim, 2026-10-15): "decisión 1 me gusta la 4 al final es 'un nuevo
  inicio' ya con esto mas limpio sin tanto python+bash+go podria ser solo go +bash como wrappers",
  "2 2?" and "3 3?" (confirming B2 and C2 as recommended), and "quiero que este como si se hubiera
  hecho con go desde 0".
- L2 — Measured reconnaissance at `ecfb843`, recorded in the tables above: 19 bash scripts / 3.039
  lines, 43 Python modules / 21.346 lines, 9 native verbs and 7 shims, plus the per-script Python
  invocation map and the two catalog hooks that call Python.
- L3 — Grep discipline that avoided a false audit: several `lib/_internal/*.py` mentions inside the
  gate's Go files and the catalog docs are COMMENTS that name the Python module they mirror
  (`util.py`, `recipe_schema.py`, `lock.py`), not callers. The real callers are the bash scripts.
- L4 — The staging insight this program depends on: `internal/cli` already dispatches unported verbs to
  bash, so A4 does not require porting them first, and the flip can land before the ports.
- L5 — Consequence recorded for U1: `lib/upgrade.sh` calling `lib/sync.sh` is the last thing keeping
  the shell sync alive after the flip; the seam (S19a) makes that call native, which is what makes U2's
  deletion of the sync-side Python possible.
- L6 — Open decision handed to the human (it shapes U1): where the binary comes from at install time —
  a verified release asset, a local `go build`, or both in a defined order. The repo already has the
  verified-artifact pattern for the gate binary, and the upgrade engine already distinguishes install
  kinds.
- L7 — Decided (2026-10-15): verified release asset first, local `go build` as the fallback (S9).
  Consequences recorded for U1: it depends on the release assets card (`odd/tasks/go-12-upgrade.md`
  names Go 15's release assets and `<home>/bin/SHA256SUMS`), the verification must reuse the
  `internal/gatebinary` digest discipline rather than inventing a second one, and the upgrade engine
  gains the binary install kind next to the legacy checkout kind.
- L8 — U1 is sliced before it is opened, because it is three different risks in one story:
  **U1a** wire the native `upgrade` route (its core already exists), **U1b** the install/release/data
  home change, **U1c** the explicit legacy entry for parity plus `upgrade.sh`'s sync leg through the
  seam (the change that orphans the shell sync).
- L9 — U1a delivered (`cd37c44`, PR #362). Two facts it recorded rather than fixed: the port compares
  physical paths where the shell compares logical ones, and the two shell post-steps it omits.
- L10 — Reconnaissance for the flip, which re-slices it: the repository HAS releases and a workflow
  that attaches assets (`.github/workflows/release-worktree-gate.yml`), but ONLY the gate's four
  platform assets — there is no `ai-specs-<os>-<arch>` — and the gate's digests live in a committed
  trust root whose discipline is "the network supplies only bytes, the repository supplies the
  expected digest". `internal/upgrade.Acquire` (download, verify before any replacement, atomic swap)
  is ported and tested but called by nothing. So the flip cannot go live before the CLI has a release
  channel, and U1b splits into **U1b-1** (the channel: workflow + committed trust root) and **U1b-2**
  (the install flip itself, whose download path is fixture-testable and fails closed to the local
  build until U1b-1 lands).
- L11 — U1b-1 renamed the release workflow `release-worktree-gate.yml` → `release-binaries.yml`,
  because it now releases BOTH binaries. Two things about that rename are worth recording: L10's
  reference to the old name is the reconnaissance record and stays as history, and the rename broke
  three tests in `tests/test_worktree_gate_release_phase4.py` that pin that path — the THIRD time this
  session an artifact was moved without first grepping every test that names it. The sharpened rule:
  enumerate every artifact the unit MOVES or renames (not only the ones it edits) and grep the test
  tree for each BY NAME before running the evidence chain.
- L12 — U1c is DROPPED as obsolete, and the evidence is what makes it safe to drop: the slice's two
  premises dissolved under a grep. `lib/upgrade.sh` does NOT call `lib/sync.sh` (only a comment at
  `:70`; `grep "^[^#]*sync\.sh"` returns nothing), and the explicit legacy entry the slice was going to
  build already exists (`tests/_go_cli.py:21`, `LEGACY_CLI = ROOT / "bin" / "ai-specs"`, documented at
  the module header and used by the parity legacy leg). Its real residue — six suites that drive
  `bin/ai-specs` plus the `bash -n` sweep in `validate.sh` — is not a unit of its own and folds into
  U2/U8.
- L13 — U2 is re-cut after a live-caller inventory (delegated, read-only): **nothing** in `lib/` is
  deletable today. Every script and every `lib/_internal` module has at least one live caller, and the
  whole set is sustained by `bin/ai-specs` itself (`parity.py:1508`, `_go_cli.py:43`, ~20 suites through
  `_blackbox.py:18`) — precisely because the launcher does NOT delegate the already-native verbs to the
  binary. Parity re-bases at `parity.py:1449`, where it copies `lib/` from the LIVE tree, so a frozen
  snapshot has to exist before anything is removed. U2 is therefore not "delete the shell": it is flip
  the launcher → re-base the parity snapshot → delete what the flip orphaned, which is why the human's
  verbs-first decision puts the ports ahead of it.
- L14 — The release trust root is VERSION-scoped, and that is a rule for every future unit: changing Go
  source does NOT invalidate `bin/SHA256SUMS`, because that file describes the bytes of a RELEASED
  version (v0.24.0), not the working tree. The comparison belongs ON the release tag, where the release
  workflow's checksum job already performs it; off-tag a suite must skip with its own reason rather
  than fail, and a non-canonical local toolchain must skip rather than blame the trust root. U3a
  learned this by failing its first evidence chain on U1b-1's own test (that unit's document, L5).
- L15 — The review binding needs three corrections when the work happens in a worktree of a stacked
  epic, and every one of them was learned by failing (U3a's document, L7, has the details): pass
  `workspaceRoot` on EVERY call or `assess` measures the session cwd and reports a tier for paths the
  unit never touched; pass an explicit `baseRef` + `committedOnly`, because the provider's default
  projection is "since the last release" (`e773514a`) and the unit's candidate is the delta since the
  epic tip; and keep `lensesReason` SHORT, because an over-long one is rejected as `lenses-invalid`
  with no lineage created and no mutation (safe to retry).
- L16 — The review's advisories are an INPUT to verification, not a formality to acknowledge. In U3a
  three lenses converged on eight lines, the differential confirmation turned them into a real
  divergence from the shell contract, and the result was a follow-up unit (**U3a-fix**) instead of a
  silently republished candidate: the reviewed bytes stay the published bytes, and the repair gets its
  own tier, tests and receipt.

- L17 — An idiom that LOOKS equivalent to the primitive it replaces is a hypothesis, not a port. U3a's
  review caught the most expensive instance of this class so far: a character-device stat standing in for
  `isatty(3)`. It looks interchangeable, it passes every test whose streams are buffers or pipes, and it
  silently falsifies the PREMISE of the guard built on top of it — with both streams at `/dev/null` the
  hub launched Python where the shell printed its frozen message and exited 2. The family already had two
  earlier members in this program: Python's `is True` versus Go truthiness (now pinned by
  `TestWorktreeFlowEnabledKeepsPythonIsTrueSemantics`) and logical versus physical path comparison (U1a).
  The rule that follows: for every primitive a port substitutes, name the language-specific semantics it
  must preserve and write the ADVERSARIAL test — a device that is a character device but not a terminal,
  a string that is truthy but not `True`, a path that resolves logically but not physically.
- L18 — A review's advisories are worth more than the receipt they arrive with, but they arrive as IDS and
  LOCATIONS, not as prose. U3a's ten advisories named `internal/cli/cli.go:158-165` three times from three
  lenses, which is what made them worth a differential experiment; the experiment is what turned a
  "readability warning" into a confirmed product defect and then into a unit. Reading the sites the
  review points at, and building the experiment that can refute it, is the whole verification job.
- L19 — Requirement change, verbatim (2026-10-16), after the FIRST decision had already been made: the
  human chose "Prompt layer propio, cero dependencias (recomendada)". What changed the decision was the
  MEASURED cost of the alternative, not a preference: pterm v0.12.82 — the newest version compatible
  with the canonical go1.24.13 toolchain, since v0.12.83 and newer require Go 1.26 — vendored 15
  modules, 549 Go files, 334 049 lines and 15 MB, and introduced a 105-line `go.sum`. So D8's "zero
  third-party dependencies" would have stopped being true, and six records would have needed amending
  (`go.mod:8-9`, `release-binaries.yml:66-69`, `build-gate.sh:4`,
  `test_worktree_gate_release_phase4.py:284-302`, `assets.go:6`,
  `go-migration-parity-contract.md:509-511`). U3b now implements the prompt layer natively on
  `internal/tty`, so no vendor tree, no `go.sum`, no amendment, and D8 and L14 stay intact; those six
  sites become the record of what the program did NOT need. The estimate behind the first decision
  ("~6 modules") was the DIRECT requirement list; the vendored closure was more than twice that. The
  lesson for the next dependency question: measure the closure BEFORE asking, not after.

## Progress

| Unit | State | Evidence |
| --- | --- | --- |
| U1a — the `upgrade` verb native | delivered | `cd37c44` (PR #362); two divergences pinned, not fixed |
| U1b-2a — the engine knows both install kinds | delivered | `5b603fb4` (PR #363) |
| U1b-2b — `install.sh` installs the compiled entry | delivered | `a5bdce71` (PR #364); three follow-ups recorded |
| U1b-1 — the CLI release channel | delivered | `20e653aa` (PR #365); review `review-bb12e5dc1d27de79` approved |
| U1c — legacy entry + sync leg through the seam | **DROPPED** (obsolete) | L12 |
| U3a — the hub's non-interactive contract | **delivered, review approved** | `0ba47aa` (PR #366); 2495 tests green; `review-97428fc2534aa06c` burned |
| U3a-fix — the confirmed advisories | **delivered** | `54197dd`; 2496 tests green; the isatty divergence proven fixed against the shell |
| U3b — the interactive hub, native and dependency-free | **delivered, review approved** | `b572a7d` (PR #368); 2494 tests green; `review-431359c0d529da28` approved, 0 blocking, 14 advisory |
| U3b-2 advisories — the 14 review advisories closed | **delivered** | `55d235e` (PR #369); 10 fixed / 4 declined with a reason, 0 open; doc `go-20-u3b-advisories.md` |
| U4a — the classic `init` path, native | **delivered, judgment approved** | `76d8b83` (PR #371); T4a of `go-20-u4-init.md`; Judgment Day 8/8 verified after a two-round fix, independent verify PASS, 44 init tests + parity 29/29 |
| U4b-1 — the wizard, native | **delivered** | `9a57121` (PR #373); `tui.Text` + `internal/cli/initwizard.go`; the 7 PTY wizard flows now drive the built binary; assess: medium, review not due; verify unavailable (subagent timeout), raw-mode lifecycle covered by reading |
| U4b-2 — per-recipe configuration prompts | pending | `init_tui.py:204-233`, needs `recipe-read.py` + `config_wizard.py` (U6 surface) |
| U4c — the lock `[meta]` stamp, native | **delivered** | `internal/cliversion.StampLockMeta` replaces `cli_version.py stamp-meta` in `sync`; the flatten stays (measured: `refresh-bundled.py` never wrote `[meta]`, its docstring lies); assess: medium, review not due |
| U4d-1 — the harness env step, native | **delivered** | `e13f3a6` (PR #377); `sync.RunEnvScaffold` replaces `env_scaffold.py`'s non-interactive half; the dotenv surface is single-sourced in `internal/sync` and doctor delegates; differential against the Python: byte-identical |
| U4d-2 — the wizard's env offer | pending | prompts, `write_env`, the three legacy migrations, `direnv allow` |
| U2 — flip the launcher + parity snapshot + deletion | pending, after the ports | L13 |
| U5..U7 — skills, recipes, substrate tail | pending | |
| U8 — program closure | pending | |
