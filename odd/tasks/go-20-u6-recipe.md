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

## U6e — the native review, and what its advisories became

The review (`review-ea5e3ea49a8c6e1a`, tier high, 2049 lines) **closed approved with four lenses and zero
blocking findings**, and its authority is burned. Getting there is worth recording, because the first two
attempts failed in different ways:

1. The **group** capture ran all four reviewers concurrently and died at the relay bound (1 009 529 ms
   against 1 009 502 for a 127 578-byte prompt) with `prepared_reviewers: 0` — **nothing** was consumed and
   nothing admitted. Group is all-or-nothing: with four ~128 KB prompts, one slow reviewer discards the
   three healthy runs. This is U3b-2's lesson re-measured, and it is why the remaining captures went one
   at a time.
2. **Sequentially**, `risk` (2 844 B), `resilience` (2 191 B) and `readability` (3 707 B) were admitted; then
   `reliability` hit the same 16.8-minute bound and the provider declared the slot **unachievable** — a
   mutation, not a reoffered failure. Its own `capture-unachievable --withdraw=true` continuation, run with
   the native binary, returned `withdrawn: true` and the slot was reoffered; the **retry finished** and the
   review closed. The wall is time, not content, so a second attempt was worth its 17 minutes.

The eleven advisory findings, with dispositions — none blocks, and none is silently dropped:

| id | severity | disposition |
| --- | --- | --- |
| `R2-stale-initwizard-finding` | WARNING | **FIXED here**: the document claimed `initwizard.go:92` still said "U6e lands the native dep gate"; the parent corrected that comment in the same slice, so the stale thing was the sentence denouncing it — the same shape as U4b-2's `R2-stale-spec-finding-d` |
| `R2-fixture-comment-name-mismatch` | SUGGESTION | **FIXED here**: the fixture comment named a case it does not build |
| `R3-Rc3PartialWrite` | WARNING | **RECORDED**: the dep gate's rc 3 returns mid-loop, so recipes configured before it keep their writes. The Python has the same partial-write shape (its `ensure_deps` sits at a different point), and "fix" would mean rolling back writes the user asked for; recorded as a known limitation instead |
| `R3-EnvOfferScope` | WARNING | **RECORDED, owner U7**: `recipe add` offers the project-wide env set where `recipe-add.py` scoped collection to the added id; the scoping lives in `internal/sync.OfferHarnessEnv`, outside this slice's edges |
| `R3-LoopOrderUnproved`, `R3-AptGuidanceUnproved`, `R3-TailSeamClose` | WARNING/SUGGESTION | **RECORDED, owner U7**: the loop order, the dep-guidance bytes and the tail-seam close are pinned in-process, not differentially; a PTY differential is where they belong and the table has none (L4's constraint) |
| `R1-1`, `R1-2`, `R1-3` | WARNING | **RECORDED**: the risk lens's three notes on the write path are informational here — the pre-loop `existing_by_id` snapshot, the one-writer call and the catalogue join are each the Python's behaviour, quoted in the code at the cited lines |
| `R2-dead-empty-shim-map` | SUGGESTION | **DECLINED**: the empty map is deliberate and its comment says why — `runShim` is still exercised by `recipe init`'s direct route and by the `AI_SPECS_INIT_TUI_PY` hatch, so the mechanism is not dead even though no verb is shimmed |

## Slicing

| slice | scope | retires | unblocks U4b-2 |
| --- | --- | --- | --- |
| **U6a** | `recipe list` + the catalog reader the verbs share (home-first resolution, the internal-test-recipe filter) | the BINARY's use of `recipe-list.sh` (see S9) | no — it establishes the reader the rest uses |
| **U6b** | `recipe add`: the `[recipes.<id>]` append with its config placeholder, the dup/internal guards, the non-TTY path. On a TTY the verb still delegates whole to `lib/recipe.sh` (the init-wizard seam pattern), because `recipe add` is one of the four TTY surfaces the parity contract freezes | the BINARY's non-TTY use of `recipe-add.sh`/`recipe-add.py`; the TTY leg flips in U6d+U6e | no |
| **U6c** | `recipe remove`: route the already-ported `config.RemoveRecipeSegments` and port the lock-entry cleanup | `recipe-remove.sh` | no |
| **U6d-1** | recipe config write: `internal/config.UpdateRecipeConfig` PROMOTED out of `catalog/recipes/worktree-flow/gate/shared/recipeconfigwrite.go` (one grader, S6) + the whole non-interactive `recipe configure` surface of `recipe-configure.py` (`--inspect`, `--set`, `--dry-run`, `--json`, `--sync`, `--ignore-cli-version`) | `recipe-config-write.py` for this route | no |
| **U6d-2** | the wizard core `run_config_wizard` on `internal/tui` (enum→select, bool→confirm, regex validation, blank→omit) + the flip of `recipe add`'s "Configure now?" arm onto it (T3b, first half) | the interactive half of `config_wizard.py` | **yes** |
| **U6e** | `configure-recipes`: the whole-project loop, the TTY gate (exit 3), the dep gate and the env offer; the dispatcher's usage/unknown arms; **and the flip of `recipe add`'s TTY seam onto the native wizard + env offer, which is what finally kills that seam**; the shim map goes empty. Plus the two decisions L10 records: `recipe init`'s DIRECT route to recipe-init.sh (found gap — no slicing row owns recipe-init.py), and the dep gate kept on the S37 seam, U7-owned (decision (b)) | `recipe.sh`, `recipe-config.sh`, `config_wizard.py`, and the U6b seam | no (already unblocked) |

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

- L9 — Evidence (U6d-2, T4b + T3b first half), the wizard core and the NARROWED `recipe add` seam.
  THE WIZARD CORE (goals 1/2): `internal/tui` gained ONE primitive, `TextValidated` — TextRaw's
  verbatim-buffer contract plus questionary's `validate=`: every Enter first runs the validator
  over the line AS TYPED (the raw buffer, before any strip/default substitution — the value
  questionary hands the handler), and on failure the settled line is printed, the validator's
  message under it, and the SAME buffer redrawn for another attempt; aborts keep the shared
  ("", false). `Text`/`TextRaw` did not move a byte — pinned by the literals-golden
  TestTextBytesAreUnchangedForExistingCallers, which was captured and GREEN BEFORE this slice's
  implementation (the PRESERVE baseline) and still green after. The core is `runConfigWizard` in
  `internal/cli/recipewizard.go`, a line-for-line port of `config_wizard.run_config_wizard` over
  `internal/schema`'s ConfigSchema behind the injected `wizardPrompts` seam (production
  `tuiWizardPrompts` → `tui.SelectDefault`/`Confirm`/`TextValidated` wired by
  `openRecipeWizardPrompts` — isTTY + the raw-mode terminal, refusal rc 3, the
  runInit/openInitWizardKeys pattern; tests script one ordered transcript): sorted(fields) only
  (never .extra, never the structured tables), the field's help_text printed immediately before
  its prompt, the `<recipe_id>.<key> (<required|optional>, <kind>)` message with kind = declared
  type → else `enum` → else `string` (Python's `field.type or ("enum" if field.enum else
  "string")` means a declared type WINS the message even on an enum field while `if field.enum:`
  still picks the SELECT branch — both pinned; the slice handoff's paraphrase read kind as
  "the field type, or enum when it carries one", which is true only when no type is declared),
  `_field_default`'s four steps verbatim (existing_config → field.default → False-for-bool → ""),
  Python's `str()` for the text prefill (bool → True/False, floats repr-shaped with the fixed
  boundary at 1e16, list/table defaults through their repr: `['alpha', 'beta']`) and Python's
  `bool()` for the confirm (bool("no") is True — pinned), the three validator shapes with the
  re-prompt shown and the NEXT answer accepted (required-only `This field is required.`; regex-
  only with empty ALWAYS passing and `Must match <pattern>` on a failed re.match — Go anchors
  with `^(?:<pattern>)`, the grouped wrap keeping a leading `|` inside the anchor; composed
  checking required FIRST, so whitespace and empty answer with the required message and never
  `Must match`), blank → omit (the key stays out so write-back keeps the existing/default), and
  cancel on ANY prompt → nothing (Python's `{}`, caller skips the write). The core NEVER writes:
  an unparseable `validation.regex` surfaces as an error (Python's re.error escaping to the
  `except Exception`) and the caller words recipe-add.py:266's exact
  `  ! could not open the configuration wizard` line; answers are written by the verb through
  `config.UpdateRecipeConfig` with `sortedWizardConfigValues` — the sorted key order IS Python's
  dict insertion order, so new-key appends stay byte-stable.
  THE FLIP (T3b, first half): the seam moved from the TOP of `runRecipeAdd` INTO the verb, at the
  script's own position — after the guards, BEFORE the write — and `runRecipeAddFlow` injects the
  prompt opener so tests drive the leg without a PTY. The narrowed truth, pinned both ways by
  TestRecipeAddTTYSeamStaysOnLegacyScript (the pin keeps its name, which cli.go's and cli_test's
  routing comments reference): off the TTY the port owns the whole verb (unchanged); on a TTY the
  verb is native end to end when the run would NOT reach a tail without a native side — nothing to
  configure (Python's else branch), or a config-only recipe, where the leg runs
  blank line → `Configure now?` (default yes; decline/abort/cancel wording the frozen
  `You can configure later with: ai-specs configure-recipes`, exit 0) → the wizard → the
  write-back (declined leg leaves the placeholders untouched), and the configure leg REPLACES the
  `Next steps:` guidance exactly as recipe-add.py:247 does. THE SEAM STILL CARRIES, and why:
  `recipeAddTailNeedsLegacy = has_mcp_env || (has_config && recipe.cli_deps)` — the env offer
  (env_scaffold.offer_harness_env) and the CLI-dep gate (recipe-add.py:258's `_run_cli_dep_gate`
  and the same gate inside `configure_selected_recipes` — `config_wizard._dep_gate` with its
  dep_install offer) have no native side yet and BOTH are U6e's declared scope, so any TTY run
  that could reach either delegates the WHOLE verb to lib/recipe.sh before the write (the
  manifest then untouched); with them that route still runs the legacy ensure_deps pre-gate and
  the Python wizard, and physical deletion stays U7/U8 through S9. Native-leg deltas recorded in
  the L4-deviation class: (1) recipe-add.py:155-181's ensure_deps/questionary PRE-write gate has
  no native counterpart and needs none — the native wizard's only TUI dependency is internal/tui,
  which cannot be missing from the running binary, so no run is refused (or pip-offered) for
  Python packages; (2) a TTY whose raw-mode terminal cannot open answers the confirm like a
  decline (legacy questionary would traceback uncaught post-write) — never exercised on a real
  terminal, pinned by the unopenable-TUI subtest; (3) the TTY legs remain NOT differentially
  byte-measured (runOne captures streams; no PTY in the table — L4's constraint unchanged):
  the whole TTY surface is pinned in-process by the scripted seam/core/wiring tests instead.
  **U4b-2 IS UNBLOCKED BY THIS TASK (S7)** — the pieces init's recipe-configuration step will
  consume are named exactly: (a) `runConfigWizard(recipeID, *schema.ConfigSchema, existing,
  wizardPrompts)` — the Go run_config_wizard, package `internal/cli`, callable from
  initwizard.go directly — with its `wizardPrompts` interface so init can script or wire its own
  prompts, and (b) `internal/schema`'s ConfigSchema/ConfigField (recipe.toml's parsed metadata out
  of `schema.LoadRecipeToml`), plus the two caller-side helpers the recipe-add leg already proves
  end to end: `sortedWizardConfigValues` feeding `config.UpdateRecipeConfig` (write-back) and
  `tuiWizardPrompts`/`openRecipeWizardPrompts` (the internal/tui wiring, reusable as-is).
  Nothing of recipe add's verb shell is needed. Test evidence: RED-1 (the core suite against the
  absent file: `undefined: runConfigWizard` et al.), GREEN-1 (core + wiring green with the old
  seam untouched), RED-2 (the rewritten pin on pre-flip code: the four native-leg subtests saw
  the legacy marker and no wizard, the three kept-behaviour subtests passed), GREEN-2 (all seven
  subtests; then `PATH=/tmp/pybin:$PATH go test ./internal/tui/ ./internal/cli/` and
  `PATH=/tmp/pybin:$PATH go test ./cmd/... ./internal/...` green, `gofmt -l internal/` empty,
  `git diff --quiet -- ai-specs/ai-specs.toml` exit 0).
  Spec findings (reported, not absorbed): (a) the kind-paraphrase correction recorded in the
  message entry above; (b) the wizard writes TEXT answers over typed defaults on BOTH routes:
  `update_recipe_config` performs no type coercion, so accepting the untouched `retries = 3`
  prefill rewrites it as the STRING `retries = "3"` — Python behaves identically, so the port
  matches it rather than "fixing" it; anyone expecting typed round-trips is reading the wrong
  contract; (c) cli.go:136-140 and cli_test.go:143-145 still describe the OLD whole-verb TTY
  seam ("on a terminal the verb delegates back to that shim") — both live OUTSIDE this slice's
  edit surfaces, so the narrowed truth is recorded here for T6's pass instead of edited there;
  (d) docs/go-migration-parity-contract.md §7's frozen TTY table now has `recipe add` split
  (native configure leg, script-retained env/dep legs) — same outside-surfaces situation L4
  flagged, advanced one step for T6; (e) on the `recipe add` route the manifest read for
  existing_config is behaviourally indistinguishable from the schema defaults (the placeholder
  append mirrors `_field_default` exactly, and the dup refusal blocks any other config block),
  so `wizardExistingConfig` is pinned by shape, not by a divergent-value case — configure-recipes
  (U6e) and init (U4b-2) will exercise the existing-wins step for real.

- L10 — Evidence (U6e, T5 + T3b second half), the last `recipe` slice: `configure-recipes`
  native, the dispatcher's remainder native, `recipe add`'s tail flipped, the shim map EMPTY.
  WHAT IT RETIRES (S9 — the binary stops using; every file stays for the launcher, deletion is
  U7/U8): `recipe.sh` — every arm it had is native or (for `init`) routed STRAIGHT past it, so the
  dispatcher never execs it; `recipe-config.sh` — wrapper contract and verb both native;
  `config_wizard.py` as the `configure-recipes` verb (main, `configure_selected_recipes`,
  `_enabled_recipe_ids`, `_offer_envrc`, `run_config_wizard` — the last already U6d-2's core); and
  the U6b seam itself — `recipeAddTailNeedsLegacy` and its `runShim` call are DELETED, not narrowed,
  so `recipe add` runs native end to end off AND on a TTY. The `shims` map is empty (one test
  asserts `len(shims)==0`); `routeShim`/`runShim` stay with a comment saying why they live:
  `recipe init`'s new direct route, init.go's `AI_SPECS_INIT_TUI_PY` whole-verb hatch (S39), and
  the fact that the dep-gate seam execs its python3 driver directly — no verb resolves through the
  map anymore, but the strangler's exec path is still in use.
  THE DEP-GATE DECISION (goal 4): **(b), keep the same named seam** — `runLegacyInitDepGate` +
  `initDepGateDriver` (S37), now with THREE callers (init's wizard, the `configure-recipes` loop,
  `recipe add`'s tail) and now **owned by U7**, which must either port `_dep_gate`'s install offer
  or retire the gate with the scripts. Size judgment (the reason, read before deciding):
  `_dep_gate` itself is ~40 lines, but it stands on `dep_install.py` (240 — the package/guidance
  plans, the consent prompt, the brew/apt SUBPROCESS installs) and on `provider_install.py`'s
  install half (597 — network download, checksums, extraction; only its READ-ONLY resolution was
  ported, into `internal/provider`), while its check half exists in Go only as `internal/doctor`'s
  UNEXPORTED probe — promoting it edits a package outside this slice's edit edges, and copying it
  would violate S6. So this slice ports exactly the half that has no install behind it
  (`dep_install.resolve_install_plan`'s GUIDANCE rendering, for `_print_cli_dep_guidance`'s one
  line) and keeps the gate itself on the seam. The gate was NOT removed from any route: the loop
  gates cli-deps recipes BEFORE the fields check (its position, even for a config-less recipe —
  pinned), `recipe add` gates at BOTH script positions (:258 before the guidance block, and
  inside configure_selected_recipes before the wizard — each keeping its own frozen decline line:
  the guidance block vs `Skipped <id> (missing CLI deps)`), and init's S37 path is untouched.
  Gate failure mappings (recorded, pinned): in the loop an unrunnable driver is Python's uncaught
  traceback → exit 1 on our own `ERROR: configure-recipes failed: <detail>` line, and the driver's
  vendor rc 3 answers main's ensure_deps position — SILENT exit 3, before the env offer (S37's
  recorded timing shift); in `recipe add` BOTH degrade to the script's catch framing
  (`  ! CLI dependency check skipped: …`) and the run proceeds, because recipe add never refuses a
  run for Python packages (L9 deviation 1 — there is no pre-write ensure_deps left to answer at)
  and add wraps configure_selected_recipes in its own `except Exception` whose line is
  `  ! could not open the configuration wizard`. `ensure_deps` has no native counterpart in this
  verb either (same L9 argument: nothing before the first gated recipe needs Python — the wizard
  is internal/tui, the offer internal/sync; the driver runs `_ensure_deps` itself at the first
  gated recipe).
  WHAT STAYS ALIVE, AND WHO KILLS IT (S8): `recipe-read.py` — the S37 driver reads the recipe
  through it on every gated run, plus `recipe-configure.py`'s import and the callers S8 names
  (`env_scaffold.py`, `dep_check.py`, `recipe-materialize.py`) → the launcher flip (U2) takes the
  callers, U7 the driver; `recipe-conflicts.py` — only `recipe-materialize.py` and the test suite →
  U2/U7 with the spine's callers; `recipe-init.sh` + `recipe-init.py` — now reached DIRECTLY by
  this dispatcher's `routeRecipeInit` (and still by recipe.sh through bin/ai-specs) → an owner row
  for the port or U7/U8 deletion; `config_wizard._dep_gate` — the driver imports it → U7 (above);
  `lib/recipe.sh` + `lib/recipe-config.sh` + `bin/ai-specs`'s own case arms — the pure-bash
  launcher still routes both verbs → U2/T7's launcher flip (the GO dispatcher is the thing emptied
  here, not the shell launcher); `lib/_vendor` — the driver's and the scripts' Python packages →
  dies with them.
  PTY / DIFFERENTIAL HONESTY: the differential tables are the NON-TTY surface only — 8
  `configure-recipes` cases (usage ×2, unknown flag, second positional, `--` positional, missing
  manifest, the TTY-gate message+exit 3 on an initialized project, the cwd default) and 8 recipe
  dispatcher cases (`--help`/`-h`/`help` on stdout, the bare verb, `bogus`, `-list`, `listx`,
  `init` without its id) — all byte-identical against `bash lib/recipe-config.sh` /
  `bash lib/recipe.sh` under `PATH=/tmp/pybin:$PATH`, plus the PATH-emptied routing proofs (no
  bash, no Python) and the `cd`-failure deviation pin. **The interactive loop is in-process only**
  (scripted seams through `runConfigureRecipesFlow`; no PTY in the Go table — L4's constraint
  unchanged): the gate→wizard→write order with the existing-wins prefill (L9's promised
  divergent-value case: the manifest starts `OLD-GATE`, the transcript shows `def="OLD-GATE"`, the
  write lands `NEW-GATE`), the gate's decline/rc-3/error branches, the fields-check-after-gate
  order, the no-clients non-invocation, wizard cancel, and both soft-fails (`Harness env offer
  skipped: <err>` on stdout with exit 0 — `_offer_envrc`'s outer catch; add's
  `  ! could not configure environment variables` with the append standing). ONE manual twin-PTY
  probe (scratch: `/tmp/u6e_pty_probe.py`, outside the worktree) covered the two lines no pipe can
  reach: `No enabled recipes found in the manifest.` + exit 0 is BYTE-EXACT legacy-vs-native, and
  the `skip <rid>: …` framing + exit 0 match modulo rich's wrapping (below). NOT probed on a PTY:
  the wizard prompt flow and the driver's install offer (child questionary) — that surface stays
  the seam's, for U7.
  Spec findings (reported, not absorbed): (a) **`recipe init` was owned by no U6 row** — recipe-init.py
  (272 lines) appears in no slice, yet U6e claims recipe.sh retires and the shim map empties; this
  slice resolves it by routing `init` STRAIGHT to lib/recipe-init.sh (byte-identical: recipe.sh's
  arm is `bash recipe-init.sh "$@"` after its subcommand shift — pinned by the dispatcher
  differential's `init without its id` case and the route table) and names the gap in cli.go's map
  comment and the amended U6e row; recipe-init.py still needs an owner row (U7 tail or a U6f).
  (b) The task's "non-TTY byte-comparable: no recipes" cannot exist: config_wizard.main checks
  the TTY gate BEFORE `_enabled_recipe_ids`, so off a terminal every enabled-less project measures
  the GATE message (that case is in the table and byte-matches); the `No enabled recipes…` LINE
  itself is TTY-only and was PTY-probed (byte-exact, above). (c) rich Console WRAPS the loop's
  console lines — `skip <rid>: …` and `Skipped …` print through `Console(stderr=True)`, which
  soft-wraps at console width (80 off-terminal) and colours yellow on a TTY; the port prints the
  single unwrapped plain line (the init port's precedent — `skip config for` is plain too).
  Framing and exit codes are equal (the PTY probe compares modulo whitespace); colour and wrapping
  are NOT reproduced, and cannot be differentially measured because the loop is unreachable off a
  terminal. (d) L9 finding (c) is CLOSED here: cli.go's and cli_test's stale whole-verb-seam
  comments landed inside this slice's edit edges and were rewritten, and
  TestRecipeAddTTYSeamStaysOnLegacyScript — whose premise the flip kills — is renamed
  TestRecipeAddTailIsNativeOnATerminal. (e) L9 finding (d) advanced but NOT closed:
  docs/go-migration-parity-contract.md's frozen tables still describe the old world (§7's
  `recipe add` row "the dependency gate runs before the manifest write", and the routing rows
  naming recipe.sh/recipe-config.sh for both verbs) — outside this slice's edit edges, for T6.
  (f) ~~initwizard.go:92 still says "U6e lands the native dep gate"~~ — the PARENT corrected that
  comment in the same slice (it now names U7 as the native dep gate's owner), so this clause is closed;
  the review read it before the correction landed. (g) `recipe add`'s env offer now runs
  through sync.OfferHarnessEnv's AGGREGATE collection where recipe-add.py passed
  `recipe_ids=[id]` (U4d-2's port has no scoping parameter and internal/sync is outside this
  slice's edges); the structural `has_mcp_env` gate still precedes it, so the divergence is the
  prompt SET (and the collect gate) on multi-recipe projects — TTY-only, in-process pinned, not
  differentially measurable. (h) The `_dep_gate` panel stream differs at ONE position:
  recipe-add.py:66 renders add's pre-gate panel on stdout (`Console()`) where the shared S37
  driver prints stderr (`Console(stderr=True)`); on a terminal both reach the same screen, and the
  driver is shared with init and the loop (a stdout variant was judged not worth a second driver).
  Test evidence: RED-1 (Phase A against the shimming binary — the three configure-recipes
  PATH-empty proofs, the cd-deviation pin and the three dispatcher PATH-empty proofs all failed
  with `ai-specs: exec: "bash": executable file not found in $PATH` or bash's prefixed cd error,
  while both new differentials passed through the shim, i.e. legacy-vs-legacy); RED-2 (the new
  seam/route tests against the pre-implementation tree: `undefined: recipeTailSeams`,
  `undefined: routeConfigureRecipes`, `undefined: routeRecipe`, `undefined: routeRecipeInit`,
  `undefined: runConfigureRecipesFlow`); GREEN (both differential tables byte-identical, every
  PATH-empty proof and every scripted loop/tail subtest passing) with
  `PATH=/tmp/pybin:$PATH go test ./internal/cli/ ./internal/config/` ok/ok,
  `PATH=/tmp/pybin:$PATH go test ./cmd/... ./internal/...` all ok (22 packages), `gofmt -l internal/`
  empty, `git diff --quiet -- ai-specs/ai-specs.toml` exit 0, and the twin-PTY probe exit 0.
  One `ai-specs/ai-specs.toml`-independent note: the Python test suites (tests/) were NOT part of
  this slice's authorized commands — they drive bin/ai-specs, whose launcher is untouched, but the
  pre-merge full validation remains the parent's gate (S3).
