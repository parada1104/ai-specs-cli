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
| **U6d-1** | recipe config write: `internal/config.UpdateRecipeConfig` PROMOTED out of `catalog/recipes/worktree-flow/gate/shared/recipeconfigwrite.go` (one grader, S6) + the whole non-interactive `recipe configure` surface of `recipe-configure.py` (`--inspect`, `--set`, `--dry-run`, `--json`, `--sync`, `--ignore-cli-version`) | `recipe-config-write.py` for this route | no |
| **U6d-2** | the wizard core `run_config_wizard` on `internal/tui` (enum→select, bool→confirm, regex validation, blank→omit) + the flip of `recipe add`'s "Configure now?" arm onto it (T3b, first half) | the interactive half of `config_wizard.py` | **yes** |
| **U6e** | `configure-recipes`: the whole-project loop, the TTY gate (exit 3), the dep gate and the env offer; the dispatcher's usage/unknown arms; **and the flip of `recipe add`'s TTY seam onto the native wizard + env offer, which is what finally kills that seam**; the shim map goes empty | `recipe.sh`, `recipe-config.sh`, `config_wizard.py`, and the U6b seam | no (already unblocked) |

## Tasks

- T1 (U6a, S4/S6) — the catalog resolver and `recipe list`, with its differential.
- T2 (U6b, S4/S5) — `recipe add` and its differential, including the dup and internal-test refusals.
- T3 (U6c, S5) — `recipe remove` over the existing segment remover, plus the lock cleanup and its
  differential.
- T3b (U6d+U6e, S7) — the U6b seam's flip, owned explicitly: U6d wires the "Configure now?" leg to the
  native wizard core and U6e wires the env offer, and only then does the TTY delegation die. U6b's own
  record (L4) flagged that no row owned this; this task is that owner.
- T4 (U6d-1, S5/S6) — promote the config writer into `internal/config` and port `recipe configure`'s
  non-interactive surface: the argument contract of `recipe-configure.py` (`--inspect`, `--set`,
  `--dry-run`, `--json`, `--sync`, `--ignore-cli-version`), the report wording, and the
  `PROJECT_OWNED_KEYS` routing — with its differential.
- T4b (U6d-2, S6/S7) — port the wizard core onto `internal/tui`, flip `recipe add`'s "Configure now?"
  arm onto it (T3b, first half), and record that U4b-2 is unblocked by THIS task.
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
- L6 — Evidence (U6d-1, T4), `recipe configure` native + the promoted writer. THE INHERITED CORE:
  `internal/config/recipeconfig.go` (the 1:1 port of the gate's `shared/recipeconfigwrite.go`, itself
  ported from `recipe-config-write.py`, plus `_parse_assignment`, `_validate_values`, `_recipe_state`,
  the one dotted-key reader, the Python `==` and `_write_project_key`) was judged KEEP with one fix:
  `UpdateRecipeConfig` dereferenced a nil `*toml.Table` whenever the recipe block (or its config
  table) did NOT exist yet — Python's `_existing_config` yields `{}` there and the append branch is
  exactly the new-recipe case, so the inherited code PANICKED on `configure fresh --set k=v` (RED
  captured by `TestUpdateRecipeConfig/a_missing_recipe_block_is_appended…`: SIGSEGV at
  recipeconfig.go:1182). Fix: the empty-table guard at the read site (Python's `{}`), everything else
  in the core stood as written — the unit tests for assignment parsing (incl. the str fallback, the
  non-str no-detail refusal, the dotted/table TOML parses), validation (type/enum/secret/regex in
  reference order, and the shared grader's exact `[config.reconcile]…` messages), the write with its
  comments/multiline/conflict/valid-TOML guards and byte-untouched refusals, and the project-key
  write (replace-with-comment / insert-before-next-header / append-fresh, verified against the
  Python function's actual bytes) all pass as inherited.
- L6 module graph / one grader (S6): the gate's writer lives in the separate module
  `ai-specs.dev/worktree-gate` (replace ⇒ `./catalog/recipes/worktree-flow/gate`). The promotion
  direction is FORCED: the gate module can never import `ai-specs.dev/ai-specs/internal/config`
  (the go tool rejects another module's `internal/` tree, replace or not) while a reverse require
  would close a root↔gate require cycle — so the authoritative copy for the native CLI is
  `internal/config.UpdateRecipeConfig`, the gate keeps its own copy behind `--write-recipe-config`
  as the Python route's bridge (both remain live: legacy recipe-configure.py reaches the gate,
  the native verb reaches internal/config), and the two are pinned TOGETHER by
  `TestUpdateRecipeConfigMatchesGateCopy` — five shared fixtures (inline write, dotted write into
  an inline table and into a header table, the whole-table+dotted refusal, the new-recipe append)
  run through BOTH implementations with identical bytes, changed flags and refusal strings (the
  gate copy parses through its bounded embedded-Python seam, so the test probes tomllib LOUDLY and
  runs under `PATH=/tmp/pybin:$PATH`). Structured-value validation routes to
  `shared.ValidateStructured` — the shipped shape grader, fed by `schema.ConfigTable.ShapeJSON` —
  with NO third checker added: `internal/schema`'s private validator grades recipe.toml LOAD, the
  shared one grades the write route's envelope; both mirror `recipe_schema._check_shape_value`
  byte-for-byte and existed before this slice, so unifying them is recorded as future work outside
  this slice's edit surfaces. The promoted core's deliberate deltas from the gate copy are the ones
  its header records: internal/toml instead of the embedded-Python tomllib seam for
  reads (parse-error DETAIL text differs — ADR 0002; frozen framings byte-identical) and no
  rewrite-on-refusal (a byte-level no-op for LF manifests); plus the WRITE SINK, undocumented until
  this correction round (doc correction (b)): `internal/config.atomicWrite` keeps the manifest's
  mode (0o7777 mask), RESOLVES a symlinked manifest to its target before the rename and REFUSES a
  read-only manifest, where the gate copy's `atomicWriteFile` renames over the path as given
  (replacing a symlink with a regular file) and forces 0o644 — both copies stay live (legacy bridge
  vs native verb), so each sink is graded where it is used (`TestSkillsRemoveReadOnlyDeviation`
  covers this package's read-only refusal); and the one value-domain divergence (B-003): a TOML
  datetime inside a re-serialized inline table is silently CORRUPTED by the gate copy into a quoted
  string (its bounded embedded-Python seam lowers tomllib's datetime through
  `json.dumps(default=str)`), while the promoted copy refuses `cannot serialize datetime to TOML` —
  the Python reference's own TypeError text from toml_write.py — with the bytes untouched. That
  divergence is pinned as an explicit case, not as byte identity, by
  `TestUpdateRecipeConfigRefusesADatetimeTheGateCopyCorrupts` (it runs BOTH copies and asserts the
  promoted refusal plus the gate's corrupted bytes), and the five shared fixtures of
  `TestUpdateRecipeConfigMatchesGateCopy` keep byte-identity over the domain where it holds.
- L6 the verb: `runRecipeConfigure` in `internal/cli/recipe.go` ports `recipe-configure.py` main()
  end to end. The SCRIPT is the contract: argparse's surface is EMULATED (help/usage through
  argparse's width algorithm — COLUMNS → stdout TIOCGWINSZ → 80, minus 2 —, registration-order
  prefix abbreviation, the ambiguous/expected-one-argument/ignored-explicit/unrecognized/required
  messages and their precedence). The reference the port FOLLOWS is Python 3.14 — the interpreter
  this machine's differential oracle runs (`PATH=/tmp/pybin/python3` is 3.14.7), so S3 measures the
  port against it — and 3.11 was probed side by side: help/usage (default AND COLUMNS-pinned) and
  the unknown/ambiguous/expected-one-argument/required arms are byte-identical, but ONE form
  diverges: the interleaved trailing positional `recipe configure cfg --inspect /path` (and
  `… --set k=v /path`) is `unrecognized arguments` + exit 2 on 3.11 and ACCEPTED on 3.14 (doc
  correction (a) — the earlier claim of byte-identity between both interpreters was false). The
  port accepts the 3.14 form; the differential case `interleaved positional fills the path slot`
  pins it, and against a ≤3.11 oracle that case fails, which is how the divergence stays visible;
  `_schema_for`'s `unknown recipe '<id>': …` wrapping (with Path-join semantics: an
  absolute id replaces the catalogue root, interior `..` kept uncleaned); `manifest not found:` /
  `invalid manifest:`; the two parser.error() arms; `PROJECT_OWNED_KEYS = {repo_topology}` routed to
  `[project]` through `config.WriteProjectKey` BEFORE the recipe block write (unchanged-value
  project keys skipped, exactly like the Python loop); report wording as data — a new ordered
  `rcJSON` tree in this file emits `json.dumps(document, ensure_ascii=False, indent=2)` or the
  compact `(",", ":")` separators under `--json` (the THIRD json.dumps variant in the tree is
  unavoidable: internal/config's encoder is ensure_ascii=True with no indent mode and its `Obj`
  constructor is unexported — so the report EMITTER lives with the verb that owns the output, while
  every number still renders through the shared `config.FormatPythonJSONFloat`, one float
  formatter). `--inspect`'s schema document re-reads recipe.toml for the [config] key ORDER
  (fields first in declaration order, then tables — schema exposes maps; the same re-read pattern
  buildrecipemcp.go uses for provides.mcp), the shape renders as nested document DATA at the
  report's indent (the differential caught a pre-rendered compact fragment), and preflight.cli_version
  is the FULL installed/pin/policy_ok/lock block (the differential caught the misread of
  recipe-configure.py's misleadingly-named `lock_meta` local — both were found by RED runs before
  GREEN, not absorbed as deviations). Path resolution is `Path.resolve()` (realpath with a
  tolerated missing tail) so `manifest not found:` prints the same /private/var spelling on macOS.
  Routing: cli.go intercepts `configure` in recipe.sh's own case order; `init`, the bare `recipe`,
  `--help|-h|help` and unknown subcommands keep the shim (pinned in cli_test), no TTY seam (the
  script's arm is a plain python3 exec), and no cd/pwd target resolution (neither recipe.sh nor the
  script resolves one).
- L6 the --sync leg, decided and ported IN FULL: apply_project's tail runs
  `<home>/bin/ai-specs sync <root> [--ignore-cli-version]` and `<home>/bin/ai-specs doctor <root>`
  as subprocesses and parses their CAPTURED output — the last `syncing …` line (failed_step) and
  the last `Summary: N OK, N INFO, N WARN, N ERROR` line (verify, with doctor_exit). The native
  leg spawns the SAME front-door command with the same cwd/env and the same regexes — never
  Python directly (S1) — and both parse targets are printed identically by the native ports
  (`internal/sync/step.go`, `internal/doctor/doctor.go`), so nothing is half-ported and nothing
  stays on a legacy seam; what sits behind bin/ai-specs (bash → lib/*.sh today, the Go binary
  after U2) is the sync/doctor slices' business, exactly as the script treats it. It is NOT in the
  main differential (running the real spine there would measure sync's writes, not this verb's
  report), so `TestRecipeConfigureSyncLegDifferential` pins it byte-for-byte against the Python
  reference over a STUB front door: sync failure → `partial`, `failed_step: "recipes"`, exit 1 and
  the frozen reason; sync ok + doctor ≠ 0 → `failed` + `doctor verification failed`, exit 1 with
  the verify block still populated; both ok → the parsed counts, status `ok`, exit 0; a doctor
  with no Summary → `parsed: false` with null counts, still `ok`. The stub ALSO asserts the FULL
  spawn contract (B-004/A-002): invoked binary path (`$0`), cwd = the resolved project root
  (`pwd -P`), the argv root positional, and the `--ignore-cli-version` presence — every violation
  is appended to that leg's `$STUB_CONTRACT_LOG`, which the test reads back, and the stub exits 90,
  so a wrong spawn fails on the log AND on the pinned exit codes even when BOTH legs would violate
  identically (a shared wrong answer would otherwise still byte-match); a combined
  `--sync --ignore-cli-version` case pins the flag as the sync argv's third argument (previously
  unpinned). A front door that cannot be
  spawned exits 1 with Go's exec error and NO report — Python's uncaught FileNotFoundError does
  the same (no report), only the wording differs (ADR 0002).
- L6 differential: `bash lib/recipe.sh configure <args>` vs the built binary on byte-identical
  TWIN projects under one shared env — isolated AI_SPECS_HOME (fixture catalogue as a REAL
  directory, lib/VERSION/bin symlinked), WORKTREE_GATE_BIN pinned to a freshly built gate
  (skillsRemoveEnv/skillsRemoveGateBinary, U5b's pattern: without the pin the Python leg's write
  bridge fails open with `GO_RECIPE_CONFIG_BRIDGE_FALLBACK` on stderr and the bytes would measure
  the fail-open bridge), LC_ALL=C, PYTHONDONTWRITEBYTECODE=1, the LOUD tomllib probe (skillsOracle;
  run with `PATH=/tmp/pybin:$PATH`), stdout/stderr/exit byte for byte after normalizing each leg's
  own project root (both spellings — Python resolve() prints the realpath), PLUS the resulting
  project trees and a home snapshot around the whole table. THIRTY cases, all
  byte-identical: `--inspect` with a config block, without one, on a recipe absent from the
  manifest, `--json`-compact, and with the cwd default; `--inspect` on a SECOND fixture recipe
  (`mcporder`) whose two `[[provides.mcp]]` presets are declared OUT of sorted order (trello
  before engram) with both ids in the manifest's `[mcp]` — `grounding.mcp.present` is the script's
  `sorted(name for name in required_mcp if name in manifest_mcp)` and B-001 caught the port
  emitting declaration order (RED was the two arrays flipped); `--inspect` on the interleaved
  trailing positional `cfg --inspect <path>` (doc correction (a)); `--set` of an inline scalar, the str
  raw-value fallback, a nested dotted key, a whole structured table, a `repo_topology` landing in
  `[project]`, and a `--json` report; the unchanged-value no-op; an invalid enum and an unknown key
  (both exit 3, project untouched); `--dry-run` (no write); the `[tool]` pin blocking with exit 4
  and `--ignore-cli-version` bypassing it (write happens); a missing recipe; a missing manifest
  (the resolved-path spelling); `--help` at the default width AND at COLUMNS=60 (both legs wrap
  identically); an unknown flag; a missing recipe_id; the --inspect/--set conflict; neither flag;
  an ambiguous abbreviation; `--set` without a value; and a positional after `--`. Plus
  `TestRecipeConfigureNativeNeedsNoBashOrPython` (PATH emptied, home with only a catalogue: the
  inspect document is built with no shim/bash/Python — S1) and, probed outside the table and
  byte-identical, `--set=`/abbreviation/cwd/`--help=x` corners and COLUMNS=100.
  Four refusals are recorded rather than hidden: (1) parse-error DETAIL inside `invalid manifest:`,
  `invalid TOML value for …` and the write guard comes from internal/toml, not tomllib — the
  framings and everything around the detail are byte-compared (ADR 0002, same move as L4/L5);
  (2) an unspawnable front door prints Go's exec error where Python prints a traceback (same
  exit 1, no report either way); (3) a TOML datetime in the recipe config dies in the REPORT
  BUILDER on both routes but at different MOMENTS (corrected this round — the earlier text claimed
  `both exit 1 uncaught`, which is wrong on both counts): the NATIVE verb refuses BEFORE any write —
  `--set` over a config value that is a datetime prints Go's serialization error to stderr, emits NO
  report, exits 1 and leaves the manifest bytes untouched (the failure happens while building
  `applied.changed`), and `--inspect` reports exit 3 through `rejected(...)` with the serialization
  error as the report's reason — while the PYTHON reference builds the report dict fine, WRITES the
  manifest inside apply_project, and only then crashes in json.dumps (uncaught TypeError traceback,
  exit 1, file already modified), and its `--inspect` dies the same way with exit 1 and NO report.
  Verified against lib/_internal/recipe-configure.py directly (probe: `--set` → exit 1, empty
  stdout, `board_id = "new"` on disk; `--inspect` → exit 1, empty stdout) and pinned native by
  `TestRecipeConfigureDatetimePathsPinTheRecordedDeviation`; the write-TIME sibling (a datetime the
  writer must RE-SERIALIZE) is B-003's divergence in the module-graph entry above; (4) argparse's help width is queried through
  COLUMNS → TIOCGWINSZ → 80 like shutil.get_terminal_size, so on a real TTY the port matches the
  legacy wrap; mixed leftover ordering (an unknown option AND a third positional together) follows
  argument order here and is not pinned by any case (unexercised corner). The differential also
  caught two of my own port bugs pre-GREEN (the compact shape fragment and the inspect preflight
  block), which is the RED evidence for the report-emitter work; the inherited core's panic is the
  RED evidence for the one core fix above.
- L6 spec note: the U6d-1 slicing row and T4 listed only `--inspect/--set/--dry-run/--json`; the
  slice ships the script's WHOLE non-interactive surface (`--sync`, `--ignore-cli-version`
  included — the flags decide the report's preflight/ignore fields and the sync leg, so leaving
  them out would leave them unowned by any row). The row and T4 were amended in place by this
  entry; the `retires recipe-config-write.py for this route` claim reads through S9 unchanged —
  the BINARY no longer reaches that file (or the gate bridge) on this route, while the legacy
  python route still does, and physical deletion stays U7/U8.

- L7 (U6d-1, post-review) — the native review's **risk** slot never admitted: the reviewer returned two
  findings and gave the second (a positive observation) the `R2-` prefix while the risk lens requires
  `R1-`, so admission refused the whole payload and the relay kept resubmitting the preserved result
  (288 ms, no model run) instead of producing a new one. The refusal is a reviewer formatting defect,
  not a code defect — but the payload is preserved under
  `.git/gentle-ai/rejected-results/review-6b43808bb7fae376/`, so its findings were read and acted on:
  - **`R1-path-traversal-parity` (WARNING), acted.** `configureRecipeDir` joined the user's `recipe_id`
    verbatim, so `..` or an ABSOLUTE id resolved the recipe read outside the catalog root and
    `--inspect` reported an arbitrary directory's `recipe.toml`. The reviewer itself judged the impact
    limited (local CLI, `recipe.toml`-shaped files only) and the behaviour parity with
    `recipe-configure.py`. The port now refuses an invalid id with the guard `skills remove` already
    uses, before any read — a DELIBERATE fail-closed divergence, pinned by
    `TestRecipeConfigureRefusesATraversingRecipeID`.
  - **`R2-secret-literal-guard-ported` (SUGGESTION), no action.** It is an observation that the
    secret-shaped-literal refusal was ported correctly, not a defect.
  The candidate changed with the guard, so this lineage cannot close on it; the outside view for the
  committed candidate is Judgment Day, recorded in its own section.

- L8 (U6d-1, dual-blind review — ONE bounded correction round) — the completed two-judge review of
  the committed candidate, findings resolved in one pass. No finding was judged wrong: all six were
  reproduced before fixing (two as RED tests, one as a passing divergence assertion, three by probe
  or source). B-003 and B-004/A-002 were PIN defects, not code defects — the behavior was already
  correct and the tests did not say so:
  - **B-001 (MAJOR) `grounding.mcp.present` order**, acted. The port appended present ids in the
    recipe's DECLARATION order; `recipe-configure.py:225` emits `sorted(...)`, so a recipe
    declaring `trello` then `engram` (both in the manifest's `[mcp]`) reported
    `["trello","engram"]` natively and `["engram","trello"]` from the script. The 28-case table
    could not catch it because `recipeConfigureFixtureRecipe` declares no `[[provides.mcp]]`.
    Fix: `configureGrounding` sorts the filtered ids; pinned by the differential's new `mcporder`
    fixture + case whose recipe declares the presets unsorted (RED: the two arrays flipped,
    byte-compared stdout).
  - **A-001/B-005 the validation regex was gated on `kind == "str"`**, acted.
    `recipeconfig.go` gated `validation.regex` on the field kind where
    `_validate_values` gates on `isinstance(value, str)`, so a field whose declared type maps to
    neither str/int/bool (no `type`, a custom name like `float`, a typo) accepted a non-matching
    string and wrote it where the script refuses exit 3 with `does not match its validation
    pattern`. Fix: gate on the value — Python's exact predicate; pinned by the unit cases over
    the `floaty` (custom name) and `untyped` (no type) fields (RED: both accepted the
    non-matching strings) plus the isinstance-gate preservation case (a non-string skips the
    pattern).
  - **B-002 CRLF manifests got mixed endings**, acted. `UpdateRecipeConfig` read the manifest
    RAW, so untouched lines kept CRLF while the rewritten line got LF; the reference's
    `read_text` (universal newlines) normalizes in memory and writes the whole file LF. Fix: read
    through the package's existing `readManifestText` — the one universal-newline reader the
    package already shares (`_write_project_key`, the remove/add writers), not a second
    implementation; pinned by `TestUpdateRecipeConfigNormalizesCRLFToLF` (a write returns the
    whole file LF byte-for-byte; a no-op leaves the CRLF bytes untouched, like the reference's
    early return) — RED: the mixed-endings bytes observed.
  - **B-004/A-002 the `--sync` stub pinned nothing**, acted. The stub matched only `$1` — no
    project-root, cwd, binary-path or flag assertion, and no case combined `--sync` with
    `--ignore-cli-version`. The stub now asserts `$0`, `pwd -P`, the argv root positional and the
    flag presence, appending every violation to that leg's `$STUB_CONTRACT_LOG` (read back by the
    test) and exiting 90 — so even two identically wrong legs fail on their logs and the pinned
    exit codes; a combined `--sync --ignore-cli-version` case pins the flag as the sync argv's
    third argument. Both legs satisfied the strengthened contract on the first run: no code change
    was needed, the defect was the missing pin.
  - **B-003 the "one grader" pin did not hold over the real domain**, acted. Over the five shared
    fixtures the gate copy and the promoted copy still agree byte for byte; with an inline table
    holding a TOML datetime the gate copy CORRUPTS it into a quoted string (its bounded
    embedded-Python seam lowers datetimes through `json.dumps(default=str)`), while the promoted
    copy refuses `cannot serialize datetime to TOML` with the bytes untouched — the Python
    reference's own answer (toml_write.py's TypeError). The pin is now honest: byte identity kept
    over its domain, plus `TestUpdateRecipeConfigRefusesADatetimeTheGateCopyCorrupts` running BOTH
    copies (native: refusal + untouched bytes; gate leg: the corrupted bytes asserted) with a
    comment recording which copy corrupts. The promoted copy was NOT repaired to imitate the
    gate; the divergence is documented in L6's module-graph entry and in recipeconfig.go's header.
  - **L6/L7 doc corrections**, acted, each verified: (a) the argparse byte-identity claim was
    false for interleaved positionals — `recipe configure cfg --inspect /path` (and
    `--set k=v /path`) is `unrecognized arguments` + exit 2 on Python 3.11 and accepted on 3.14;
    L6 now says the port follows 3.14 (the interpreter `PATH=/tmp/pybin` — this machine's
    differential oracle — runs) and why, with every other probed surface byte-identical between
    the two, and the accepted form is pinned by the new differential case (it fails against a
    ≤3.11 oracle, keeping the divergence visible). (b) The write-sink divergence is now
    documented in L6 and in recipeconfig.go's header: `internal/config.atomicWrite` keeps the
    mode, resolves a symlinked manifest and refuses a read-only manifest, where the gate copy's
    `atomicWriteFile` renames over (replacing a symlink) and forces 0o644. (c) The datetime
    deviation described the write and the exit codes wrongly in both directions: the native verb
    refuses BEFORE any write (exit 1, stderr, no report, bytes untouched) while the Python
    reference WRITES the manifest and then crashes in json.dumps (exit 1, traceback, file
    modified), and the `--inspect` path reports exit 3 through `rejected(...)` on the native side
    where Python dies uncaught at exit 1 — corrected in L6 with the probe evidence and pinned
    native by `TestRecipeConfigureDatetimePathsPinTheRecordedDeviation`.
  Consequence for the differential: 28 → THIRTY cases (the `mcporder` fixture and the interleaved
  positional), all byte-identical under `PATH=/tmp/pybin:$PATH` (3.14.7); the sync-leg table grew
  the combined flag case. Candidate changed with these fixes, so this lineage cannot close on the
  reviewed bytes — the same handoff rule L7 recorded.
