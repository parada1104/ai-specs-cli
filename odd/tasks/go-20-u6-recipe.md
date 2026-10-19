# Go 20 / U6 — `recipe` and `configure-recipes`, native

Unit document for T6 of the Python exit program (`odd/tasks/go-20-python-exit.md`). Worktree
`.worktrees/go-20-u6-recipe`, branch `change/go-20-u6-recipe`, based on the epic tip `1b06267`.

## Specs

- S1 — Program request, verbatim (2026-10-15): "quiero que este como si se hubiera hecho con go desde
  0". Program rule (S4): zero Python under the product tree; bash only as wrappers.
- S2 — Session request, verbatim (2026-10-19): "vamos, todo se esta mergeando en la epica de go single
  biney verdd?" — the delivery lane stays `epic/go-single-binary`, never `development`, one PR per unit.
- S3 — No observable behaviour changes without evidence: RED→GREEN per rule, the differential against
  the legacy route (stdout, stderr, exit code and the produced tree on twin projects), and the parity
  corpus where a fixture reaches the route.
- S4 — The argument contracts are the scripts': every subcommand's usage text, its flags, its
  positional slots, the dispatcher's three failure arms (bare → usage on stderr + exit 2, `--help` →
  usage on stdout + exit 0, unknown → the two lines + exit **1**).
- S5 — The manifest stays the only source of truth and every write is atomic through the shared writer
  (`internal/config`), with the valid→invalid TOML guard. Removing a recipe also drops its lock entries;
  adding one never invents a second block for the same id.
- S6 — One grader per behaviour. `internal/schema` already owns recipe.toml parsing and validation,
  `internal/conflicts` owns conflict detection and `internal/config` owns the manifest surgery: a unit
  that needs one of them reuses it, and if a core lives outside `internal/` (the gate's
  `shared/recipeconfigwrite.go`) it is PROMOTED, never copied.
- S7 — U4b-2 is blocked on the wizard core (`run_config_wizard`), not on the whole verb: the slice that
  lands it says so and unblocks that unit in the same delivery.
- S9 — "Retires" means the BINARY stops using it, not that the file leaves the tree. Measured
  (U6a): `lib/_internal/hub.py:36` eagerly loads `recipe-list` as a sibling and `lib/hub.sh` is still what
  `bin/ai-specs hub` execs until U2 flips the launcher, so deleting `recipe-list.py` when `recipe list`
  goes native would break the legacy hub. Every slicing row is read that way, and the physical deletion
  of the shell scripts and their modules is U7/U8 work with the launcher flip.
- S8 — Retirement is honest per module: `recipe-read.py` and `recipe-conflicts.py` are NOT retired by
  this unit because their callers are `env_scaffold.py`, `dep_check.py` and `recipe-materialize.py`,
  which die with U2/U7. Each slice names what it actually kills.

## Slicing

| slice | scope | retires | unblocks U4b-2 |
| --- | --- | --- | --- |
| **U6a** | `recipe list` + the catalog reader the verbs share (home-first resolution, the internal-test-recipe filter) | the BINARY's use of `recipe-list.sh` (see S9) | no — it establishes the reader the rest uses |
| **U6b** | `recipe add`: the `[recipes.<id>]` append with its config placeholder, the dup/internal guards, the non-TTY path; the interactive legs stay recorded as such | `recipe-add.sh`, `recipe-add.py` | no |
| **U6c** | `recipe remove`: route the already-ported `config.RemoveRecipeSegments` and port the lock-entry cleanup | `recipe-remove.sh` | no |
| **U6d** | recipe config write (`internal/config.UpdateRecipeConfig`, promoted from the gate's shared file) + `recipe configure --inspect/--set/--dry-run` + the wizard core `run_config_wizard` on `internal/tui` | `recipe-config-write.py`, the write half of `config_wizard.py` | **yes** |
| **U6e** | `configure-recipes`: the whole-project loop, the TTY gate (exit 3), the dep gate and the env offer; the dispatcher's usage/unknown arms; the shim map goes empty | `recipe.sh`, `recipe-config.sh`, `config_wizard.py` | no (already unblocked) |

## Tasks

- T1 (U6a, S4/S6) — the catalog resolver and `recipe list`, with its differential.
- T2 (U6b, S4/S5) — `recipe add` and its differential, including the dup and internal-test refusals.
- T3 (U6c, S5) — `recipe remove` over the existing segment remover, plus the lock cleanup and its
  differential.
- T4 (U6d, S5/S6/S7) — promote the config writer into `internal/config`, port `recipe configure`, port
  the wizard core, and record that U4b-2 is unblocked.
- T5 (U6e, S4) — `configure-recipes`, the dispatcher arms, and the shim map emptied.
- T6 (all, S8) — update the program document per slice with what each one actually retired.

## Log

- L1 — Human, verbatim (2026-10-19): "vamos, todo se esta mergeando en la epica de go single biney
  verdd?"
- L2 — Evidence (2026-10-19), delegated read-only mapping of the whole surface (7 293 raw lines, of
  which `recipe-materialize.py` 4 265 is the SYNC spine, not the verb). What it settled:
  - Go already owns `internal/schema` (recipe.toml parse/validate), `internal/conflicts`,
    `internal/config.readRecipes`/`RemoveRecipeSegments`, and the whole materialize spine. The verbs are
    wiring around cores that exist — the U5 lesson again.
  - The configure feature is ONE thing split three ways: `recipe-config-write.py` is the write authority
    (line surgery + valid-TOML guard + atomic write, with a Go bridge to the gate binary),
    `config_wizard.py` is the interactive TUI AND the `configure-recipes` verb, and
    `recipe-configure.py` is the non-interactive single-recipe surface with a JSON report.
  - `configure-recipes` = all enabled recipes + TTY required (exit 3) + the CLI-dep gate + the env
    offer, which is `sync.OfferHarnessEnv` since U4d-2.
  - `recipe-config-write.py`'s core already exists in Go but OUTSIDE `internal/`:
    `catalog/recipes/worktree-flow/gate/shared/recipeconfigwrite.go`, reachable only through
    `worktree-gate --write-recipe-config`. Promoting it into `internal/config` is S6's work and it is
    the "one grader" the gate already claims.
  - `recipe-read.py` and `recipe-conflicts.py` keep living after this unit (their callers are
    `env_scaffold.py`, `dep_check.py`, `recipe-materialize.py`), so S8 keeps the retirement claims
    honest.
  - Tests today drive `bin/ai-specs` for six recipe suites; only the sync contract suites and the TUI
    suites use the built binary via `tests/_go_cli.py`. Repointing them is each slice's own job where
    the route becomes native.
- L3 — Evidence (U6a), the differential and its environment: `recipe list` runs `bash
  lib/recipe-list.sh` and the built binary on the SAME project under one shared env — isolated
  `AI_SPECS_HOME` built like `tests/_blackbox.py:isolated_home` (install entries symlinked in,
  `cache/` local to the temp dir), `LC_ALL=C`, `PYTHONDONTWRITEBYTECODE=1`, and the LOUD tomllib
  probe that skips naming the probe and the fix (this machine's plain `python3` is 3.9 without
  `tomllib`, so the suite runs with `PATH=/tmp/pybin:$PATH`) — comparing stdout, stderr and the exit
  code byte for byte, PLUS the homes and projects snapshotted around the whole table to prove the
  verb writes nothing (no cache, no manifest write, no `ai-specs/` tree). Ten cases, all
  byte-identical: a project enabling two recipes (`worktree-flow` in the repo catalogue →
  `installed`, `ghost-recipe` with no catalogue entry → no row at all, `tdd-flow` disabled →
  `disabled`, every other real catalogue recipe `available`), a home with an EMPTY
  `catalog/recipes` (`isolated_home(catalog=False)`'s shape) → `No recipes available.`, a fixture
  catalogue carrying `test-fixture` and `.hidden` that must NOT reach the listing
  (`util.is_internal_test_recipe` and the dot filter), a project with no manifest
  (`Project not initialized. Run: ai-specs init`, exit 1), `--help` (usage on stdout, exit 0), an
  unknown flag (two stderr lines, exit 2), a second positional (exit 2), a positional after `--`, a
  second positional after `--`, and the cwd default. The routing proof (`recipe list` with PATH
  emptied) pins that no shim, no bash and no Python run on the verb, and the route test pins that
  `add`, `init`, `configure`, `remove`, the bare `recipe`, `--help`/`-h`/`help`, `bogus`, `listx`,
  `-list` and `List` still resolve to `lib/recipe.sh`. Three deviations are recorded rather than
  hidden: (1) a `cd` failure keeps exit 1 and the `cd: <path>: <reason>` shape but drops bash's own
  `lib/recipe-list.sh: line 47: ` prefix (U5a's S12 class, pinned by
  TestRecipeListMissingPathDeviation); (2) a catalogue that exists but cannot be read words Go's own
  error where Python printed a traceback — same exit 1, ADR 0002 wording class, not differentially
  exercised; (3) the resolver's SECOND candidate (recipe-list.py:30-41's module-path fallback):
  Python anchors on `Path(recipe-list.py).resolve()` — the physical tree that holds `lib/` — while
  the port anchors on the RUNNING BINARY's home (`home.ResolveHome` of `selfPath()`, the launcher
  loop bin/ai-specs itself uses); the anchors coincide in every production layout (`bin/ai-specs`
  and `lib/` side by side) and in this repository, and diverge only when the binary is built outside
  `<root>/bin` while `lib/` physically lives in the root (the test harness's smoke binary), where
  the port lists nothing where Python would reach the repo catalogue through `lib/`'s symlinks — so
  the differential's homes ALL carry a catalogue and never byte-measure the anchors; the branch is
  pinned in-process instead by `TestCatalogHomeFallsBackToInstallRoot` (the home without a
  catalogue lists the install root's recipes) and `TestRecipeListFallsBackToBinaryInstallRoot` (the
  home still wins when it has a catalogue; an outdoor binary lists nothing). Shared-reader note
  (S6): `internal/recipe/list.go` already owned the scan, the `test-`/dot filter, the statuses and
  the `internal/config` manifest read, so U6a ADDS one function beside `CatalogDir` — `CatalogHome`,
  the home-first choice — with no signature change (hub and initwizard keep their existing calls,
  unrippled), and fixes `CatalogDir`'s comment that claimed both Python candidates "reduce to this
  one" without saying what picks the home between them. `internal/recipe/verbs.go` was deliberately
  NOT created: everything the verb needed beyond the resolver already lived in list.go (the manifest
  read) or belongs with the verb's own output (the row format of recipe-list.py:120, printed in
  `internal/cli/recipe.go` like every other verb prints its own rows), so there is one home for the
  resolver, the scan and the manifest read: `internal/recipe/list.go`.
