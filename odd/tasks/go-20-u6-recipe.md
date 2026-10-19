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
| **U6b** | `recipe add`: the `[recipes.<id>]` append with its config placeholder, the dup/internal guards, the non-TTY path. On a TTY the verb still delegates whole to `lib/recipe.sh` (the init-wizard seam pattern), because `recipe add` is one of the four TTY surfaces the parity contract freezes | the BINARY's non-TTY use of `recipe-add.sh`/`recipe-add.py`; the TTY leg flips in U6d+U6e | no |
| **U6c** | `recipe remove`: route the already-ported `config.RemoveRecipeSegments` and port the lock-entry cleanup | `recipe-remove.sh` | no |
| **U6d** | recipe config write (`internal/config.UpdateRecipeConfig`, promoted from the gate's shared file) + `recipe configure --inspect/--set/--dry-run` + the wizard core `run_config_wizard` on `internal/tui` | `recipe-config-write.py`, the write half of `config_wizard.py` | **yes** |
| **U6e** | `configure-recipes`: the whole-project loop, the TTY gate (exit 3), the dep gate and the env offer; the dispatcher's usage/unknown arms; **and the flip of `recipe add`'s TTY seam onto the native wizard + env offer, which is what finally kills that seam**; the shim map goes empty | `recipe.sh`, `recipe-config.sh`, `config_wizard.py`, and the U6b seam | no (already unblocked) |

## Tasks

- T1 (U6a, S4/S6) — the catalog resolver and `recipe list`, with its differential.
- T2 (U6b, S4/S5) — `recipe add` and its differential, including the dup and internal-test refusals.
- T3 (U6c, S5) — `recipe remove` over the existing segment remover, plus the lock cleanup and its
  differential.
- T3b (U6d+U6e, S7) — the U6b seam's flip, owned explicitly: U6d wires the "Configure now?" leg to the
  native wizard core and U6e wires the env offer, and only then does the TTY delegation die. U6b's own
  record (L4) flagged that no row owned this; this task is that owner.
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
- L4 — Evidence (U6b, T2), `recipe add` native: the wrapper's argument contract
  and add_recipe's non-TTY path went native through TWO new cores —
  `config.AppendRecipeSegments` beside `RemoveRecipeSegments` (the ONE
  appender, S6: read → literal append → atomic write → parse → on failure
  RESTORE the normalized original with recipe-add.py's frozen guard message —
  parity contract §3's parse-validate-and-restore, pinned in
  `internal/config/writes_test.go` including the restore-on-invalid path over
  an already-invalid base, a valid base broken by a space id, and the CRLF
  normalization the restore leaves behind; this guard is UNCONDITIONAL, unlike
  the remove heredocs' valid→invalid-only `guardValidToInvalid`, so the shared
  guard is deliberately not reused — one grader per behaviour) and
  `runRecipeAdd`/`runRecipeAddVerb` in `internal/cli/recipe.go` (usage /
  missing-id / unknown-flag arms, the cd/pwd resolution, add_recipe's guard
  order manifest → internal-test → catalog → dup, the placeholder conversion,
  the materialize listing and the non-TTY Next steps). Shared cores reused,
  none re-implemented: `cdSkillsTarget`/`cdReason`, `recipe.CatalogHome`/
  `CatalogDir`, `schema.LoadRecipeToml`, `config.LoadManifest`/`Truthy`,
  `toml.TOMLValue` (toml_write.py's port), `atomicWrite`/`readManifestText`,
  `runShim`, `tty.IsTTY`; the tests reuse skillsListEnv/Home, skillsWriteFile,
  skillsTreeSnapshot, runOne, smokeSetup and skillsOracle.
- L4 differential: `bash lib/recipe-add.sh <args>` vs the built binary on
  byte-identical TWIN projects under one shared env — isolated AI_SPECS_HOME
  (lib symlinked so the wrapper execs recipe-add.py; the catalogue seeded as
  a REAL directory: `alpha`, the rich `cfg`, `test-fixture`, `bad id`),
  LC_ALL=C, PYTHONDONTWRITEBYTECODE=1, the LOUD tomllib probe
  (skillsOracle; run with PATH=/tmp/pybin:$PATH), stdout/stderr/exit byte for
  byte after normalizing the twin project root, PLUS the resulting trees (the
  appended manifest bytes are the contract) and a home snapshot proving the
  verb writes NOTHING in the CLI home (no cache, no vendor, no bytecode).
  Sixteen cases, all byte-identical: a catalogue recipe added to a manifest
  without it; the rich `cfg` recipe (six config fields → the placeholder
  bytes with Python's sorted() order and required-beats-default, five
  provides primitives → the listing, BOTH Next steps lines); the
  already-enabled duplicate; an id absent from the catalogue; the internal
  `test-fixture` refusal; an id whose `../` must NOT be cleaned
  (`nope/../alpha` — Path joins without normalizing, so lookup, message and
  control flow carry the id as joined); an uninitialized project; the missing
  id (two stderr lines + usage + exit 2); `--help`; an unknown flag; a second
  positional; a positional after `--` (success) and a third after `--`
  (exit 2); the cwd default; and TWO guard rows — an already-invalid
  manifest (`flag = True`, test_recipe_add's rollback fixture) and a valid
  manifest broken by the space-id append — both printing the frozen
  `Error: adding '…' would produce an invalid TOML manifest (…).
  ai-specs.toml was not modified.` with the original bytes restored. The
  routing proof (`recipe add` with PATH emptied) pins that no shim, no bash
  and no Python run on the non-TTY verb, and the route test pins `init`,
  `configure`, `remove`, the bare `recipe`, `--help`/`-h`/`help` and the
  unknown arms on `lib/recipe.sh`.
- L4 TTY DECISION — option (a): the shell's own TTY test (`tty.IsTTY(stdin,
  stdout)` — `[[ -t 0 && -t 1 ]]`, Python's `sys.stdin.isatty() and
  sys.stdout.isatty()`) gates the route in Run: on a terminal the WHOLE verb
  delegates back to `lib/recipe.sh` through the documented seam (runRecipeAdd
  → runShim — the init wizard's AI_SPECS_INIT_TUI_PY pattern), so
  add_recipe's two TTY branches (the ensure_deps/questionary PRE-write gate
  at recipe-add.py:155-174 and the "Configure now?" wizard +
  env_scaffold.offer_harness_env at :262-275) stay byte-for-byte legacy and
  nothing interactive is half-ported. Reason for rejecting (b): dropping the
  branch would silently remove an interactive surface no other slice
  re-adds, AND would falsify docs/go-migration-parity-contract.md §7's
  frozen TTY table (`recipe add` is one of its four TTY surfaces) — a
  document outside this slice's edit surfaces; (a) keeps both true and keeps
  S3 intact on BOTH routes (the legacy branch computes exactly what it
  computes today). Pinned by TestRecipeAddTTYSeamStaysOnLegacyScript: TTY →
  the legacy script runs with recipe.sh's `add` shift and appends nothing;
  non-TTY → the seam never fires and the port appends. Consequence for the
  slicing table's `retires recipe-add.sh, recipe-add.py` row, read through
  S9: the BINARY's use is retired on the route this slice ports (everything
  non-TTY — automation, CI, both differential legs), while the TTY route
  still execs those two files through the seam; flipping the seam to U6d's
  native wizard + the existing sync.OfferHarnessEnv is the follow-up, and
  physical deletion stays U7/U8. Recorded rather than claimed — the row
  overclaims until that flip, which is a spec gap this entry amends.
- L4 deviations: (1) a `cd` failure keeps exit 1 and the `cd: <path>:
  <reason>` shape but drops bash's own `lib/recipe-add.sh: line N: ` prefix
  (U5a's S12 class, pinned by TestRecipeAddMissingPathDeviation); (2) the
  guard message's embedded parse-error detail is diagnostic (ADR 0002) —
  the differential normalizes ONLY that parenthesized detail
  (recipeAddGuardDetailRE) and byte-compares the id, the framing and the
  restored manifest around it; (3) the catalogue resolver's second candidate
  keeps L3's deviation 3 verbatim (anchored on the running binary, not lib/'s
  physical tree) — every differential home carries a catalogue, so it is not
  byte-exercised here either.
- L5 — Evidence (U6c, T3), `recipe remove` native: the wrapper's argument
  contract and BOTH of the script's write steps went native without a second
  remover. The manifest surgery is ROUTED, not re-written:
  `config.RemoveRecipeSegments` — already ported and differential-tested
  against `testdata/recipe_remove_ref.py` in
  `internal/config/writes_differential_test.go` (segmenter, valid→invalid
  guard, CRLF normalization, not-found message, repo-manifest sub-tables) —
  is called from `runRecipeRemove` in `internal/cli/recipe.go`, which owns
  only the shell contract around it: the `usage()` heredoc bytes, the
  argument loop (`<id>` then optional `[path]`, `--` semantics, unknown flags,
  `ERROR: missing recipe id` + usage on stderr + exit 2), the cd/pwd target
  resolution, the `ERROR: <toml_path> not found.` guard, and the print sides
  of the helper's frozen messages (`✓ removed N section(s) …` on stdout, the
  `✗` and the guard error on stderr, exit 1). The lock cleanup
  (recipe-remove.sh:167-203) is the second step through the EXISTING
  `internal/lock` API — `LoadLock` (its missing/non-regular-file shape is the
  script's `[[ -f "$LOCK_PATH" ]]`: nothing removed, nothing written),
  `RemoveRecipeLockEntries` (the 1:1 port of
  `remove_recipe_lock_entries`, already covered by lock_test.go's "Remove
  helpers return presence") and `WriteLock` — so **`internal/lock needed NO
  change this slice**: `mutation.go` is untouched and `mutation_test.go` was
  deliberately not created (no new behaviour to grade — one grader per
  behaviour, S6, and a surface is not filled for its own sake). `Run`
  dispatches `routeRecipeRemove` with `args[2:]` (recipe.sh's own subcommand
  shift). There is NO TTY seam, unlike U6b: recipe-remove.sh has no
  interactive branch, so the whole verb is native end to end — which is what
  retires the BINARY's use of recipe-remove.sh under S9 (the file itself
  stays for U7/U8; no Python file was deleted).
- L5 differential: `bash lib/recipe-remove.sh <args>` vs the built binary on
  byte-identical TWIN projects under one shared env — isolated AI_SPECS_HOME
  (skillsListHome, `lib` symlinked so the wrapper runs), LC_ALL=C,
  PYTHONDONTWRITEBYTECODE=1, `WORKTREE_GATE_BIN` pinned to a freshly built
  worktree-gate (skillsRemoveGateBinary + skillsRemoveEnv, U5b's pattern:
  without the pin lock.py's bridge fails open with a
  `GO_LOCK_WRITE_BRIDGE_FALLBACK` stderr line the native leg never prints and
  the byte comparison would measure the bridge, not the verb), the LOUD
  tomllib probe (skillsOracle; run with `PATH=/tmp/pybin:$PATH`),
  stdout/stderr/exit byte for byte after normalizing each leg's own project
  root, PLUS the resulting project trees — manifest bytes AND lock bytes —
  and a home snapshot around the whole table (the verb writes nothing in the
  CLI home). Thirteen cases, all byte-identical: (1) a recipe that exists
  with its config sub-table (both segments dropped; the sibling multi-line
  string with blank lines survives byte-for-byte, D2's class); (2) a recipe
  with lock entries (entries dropped, meta/managed/agents preserved — D1's
  subject —, the lock rewritten to byte-identical bytes across legs and
  `  ✓ cleaned lock entries …` printed by both); (3) a lock carrying ONLY
  another recipe's entries (NOT rewritten: the surviving comment line plus a
  before/after byte assertion per leg pin "rewrite only when something was
  removed"); (4) a recipe absent from the manifest (`  ✗ … not found`, exit 1,
  project untouched); (5) a manifest with no `[recipes.*]` at all (same);
  (6) a deletion that would break a valid TOML (frozen `ERROR: removing
  recipe '…' would produce invalid TOML: …`, exit 1, bytes untouched — only
  the embedded parse detail normalized, recipeRemoveGuardDetailRE, the same
  ADR 0002 move as L4); (7) the missing id (two stderr lines + usage + exit
  2); (8) `--help`; (9) an unknown flag; (10) a third positional; (11) a
  positional after `--` (success); (12) a missing manifest (`ERROR: <path>
  not found.`, exit 1); (13) the cwd default. The routing proof (`recipe
  remove` with PATH emptied) pins that no shim, no bash and no Python run on
  the verb, and the route test pins `init`, `configure`, the bare `recipe`,
  `--help`/`-h`/`help` and the unknown arms on `lib/recipe.sh`.
- L5 deviations: (1) a `cd` failure keeps exit 1 and the `cd: <path>:
  <reason>` shape but drops bash's own `lib/recipe-remove.sh: line N: ` prefix
  (U5a's S12 class, pinned by TestRecipeRemoveMissingPathDeviation); (2) the
  guard message's embedded parse-error detail is diagnostic (ADR 0002),
  normalized only inside that one message with the id, the framing and the
  untouched manifest byte-compared around it; (3) a corrupt lock file exits 1
  like the script's traceback but prints Go's load error instead of Python's
  traceback text (the class U5b already recorded), not differentially
  exercised here; (4) a READ-ONLY manifest would hit the shared
  `config.atomicWrite` R4 refusal where the script's mkstemp+os.replace
  completes — the same divergence TestSkillsRemoveReadOnlyDeviation pins for
  `skills remove`, inherited through the shared writer (inside
  internal/config, outside this slice's edit surfaces) and NOT differentially
  exercised by this table.
- L5 spec findings (reported, not silently absorbed): (a)
  docs/go-migration-parity-contract.md §9 rows D1, D13 and D14 describe a
  recipe-remove.sh that no longer exists — the current script writes
  atomically (mkstemp + os.replace), validates before writing, and delegates
  the lock to lock.py, which preserves `[managed.*]`, `[deps]` and
  `[agents.*]`; D1 now holds only for the `[recipes]`/`[skills]` sections the
  lock writer emits on NEITHER side (recorded-defect in internal/lock), and
  D14's "no TOML validation" is exactly the guard this port reuses. Those rows
  live outside this slice's edit surfaces, so they are reported for the
  program document's own pass (T6), not edited here. (b) The slicing table's
  U6c `retires recipe-remove.sh` row needs no L4-style amendment: with no TTY
  branch, the binary's use of the script is fully retired by this route, so
  the row is honest already when read through S9 (physical deletion stays
  U7/U8).
