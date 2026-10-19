# Go 20 / U4 — the `init` verb, native

Unit document for the fourth port of the Python exit program (`odd/tasks/go-20-python-exit.md`, T4).
Worktree `.worktrees/go-20-u4a-init-core`, branch `change/go-20-u4a-init-core`, based on the epic tip.

## Specs

- S1 — Program request, verbatim (2026-10-15): "quiero que este como si se hubiera hecho con go desde
  0". Program rule (S4): zero Python under the product tree; bash exists only as wrappers (the shims
  of unported verbs) and never as the orchestrator of a verb.
- S2 — Session request, verbatim (2026-10-17): "continuemos donde nos quedamos respecto a la migración
  a full go". The unit continues the program at T4; nothing in this document re-opens a closed unit.
- S3 — No observable behaviour changes without evidence: RED→GREEN per rule, the differential (parity
  corpus and/or `tests/validate.sh`), and the recorded exit codes, stdout and stderr of every rule the
  port touches. The legacy `lib/init.sh` output is the contract, line for line, except where a
  divergence is written down below with its reason.
- S4 — `init` is idempotent and never overwrites the manifest. `ai-specs/ai-specs.toml` is user-owned:
  `--force` does not touch it, only the managed agent block in the root `.gitignore`.
- S5 — The classic (non-interactive) path is the slice's product: no prompt, template manifest,
  baseline `AGENTS.md`, both `.gitignore` files, the lock stamp and the next-steps text. Exit codes:
  `0` success, `1` unusable target / cancel / failure, `2` usage error.
- S6 — The interactive wizard stays in the legacy script for this slice. `init` decides the TUI gate
  with the legacy predicate — `--tui`, or (auto) stdin and stdout are TTYs, no `--name`, no `--force`
  and no manifest yet — and delegates the WHOLE verb to `lib/init.sh` with the original argv when it
  fires, so the wizard's behaviour, messages and exit codes are unchanged. The port owns the verb
  again once the native wizard lands (T4d).
- S7 — Asset sources are the embedded trees (`assets.FS("templates")`), not `${AI_SPECS_HOME}` on
  disk: the binary carries `ai-specs.toml.tmpl` and `gitignore-root.tmpl` at its own revision.
- S8 — Divergence, recorded not accidental: the legacy manifest render is
  `sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g"`, which mangles a project name containing `&`, `/` or a
  backslash (sed replacement syntax) and silently emits the wrong `name = "..."`. The port substitutes
  literally, so such a name lands verbatim. Pinned by a test; the legacy text stays the reference for
  every name the sed path already rendered correctly.
- S9 — The two steps with no native authority yet keep their existing seam, and each is named in the
  code: `refresh-bundled` (the flattened cache, whose native verb is a compatibility no-op because the
  assets are embedded) and the lock `[meta]` stamp. They run through the same legacy entry the shell
  used, exactly once, so a fresh `init` still produces the lock the rest of the system reads. T4c
  retires them.
  - **Corrected during T4c (measured, not assumed):** `refresh-bundled --init` does NOT stamp
    `[meta]`. Its docstring says it does, but `refresh()` only normalizes the lock
    (`lib/_internal/refresh-bundled.py:100-131`), and a legacy `init` leaves the lock with no `[meta]`
    at all — verified by running both routes against twin projects: the legacy lock has `[managed]`
    only. So `init` has nothing to stamp, the seam step is pure flatten plus its three status lines,
    and the native stamp belongs to `sync`, the route that really writes it.
- S10 — Divergence, recorded not accidental: the legacy route renders the baseline brief through
  Python, whose thin bridges announce themselves on stderr (`GO_*_BRIDGE_FALLBACK: no verified
  worktree-gate binary; using the temporary Python <authority>`). The native route renders in process,
  has no Python fallback to announce, and therefore prints none of those lines. Everything else on
  stderr is the contract.
- S11 — `ai-specs/.gitignore` is silent in the wrapper sense but not in `init`: the shell calls
  `gitignore-render.py` directly, so its `  ✓ wrote <abs path> (N dep(s))` status line IS part of
  init's stdout. The native step prints it through the same wrapper.

- S12 — The judgment round (below) found that the shell's own failure messages are not a contract: `shift 2`
  out of range on a trailing `--name`, `mkdir`'s diagnostic, and `gitignore-render.py`'s traceback are
  accidents of bash and Python. The port keeps their EXIT CODES and streams, and replaces their text
  with one line that names the flag, the directory or the manifest. Each is pinned, and none of them
  changes a successful run's bytes.

### T4b specs (the wizard, native)

- S13 — The wizard is `internal/tui`'s, on the same prompt primitives the hub already uses
  (`Select`, `MultiSelect`, `Confirm`, `Pause`) plus ONE new one: a text prompt with a default
  (`lib/_internal/init_tui.py:261` is a `questionary.text`). No third-party dependency, no PTY-only
  code path beyond what `internal/tui` already owns.
- S14 — Flow, in this order (`init_tui.py:234-400`): banner panel and `target:` line; project name
  (default = `--name` if given, else the target's basename; empty answer keeps the default; the value is
  `strip()`ed); repo topology select (`auto → <detected> (detected)`, `standalone`,
  `monorepo-apps`, `monorepo-submodules`, default `auto`); agents multi-select in `SUPPORTED_AGENTS`
  order with `DEFAULT_AGENTS` pre-checked; recipes multi-select over the catalog in sorted order with
  `DEFAULT_RECIPES` pre-checked (skipped entirely when the catalog is empty); preview panel with
  `name`, `agents`, `recipes`, `config`, `mcp`; confirm (`default: true`).
- S15 — Cancelling at ANY prompt — Ctrl-C, EOF, or a `false`-equivalent answer at the confirm — returns
  exit 1 and writes NOTHING (no staged manifest, no sidecar, no project files). A non-TTY stdin or
  stdout returns exit 3 before any prompt, so the classic path can take over.
- S16 — Outputs, byte-for-byte the Python's shapes: the staged manifest at `--out`
  (`init_tui.py:160-203`, header, `[project]` with `name`/`subrepos`/`repo_topology`, `[agents]`,
  one `[recipes.<id>]` with `enabled = true` per selection, the `[recipes.<id>.config]` table when a
  recipe carries values, and the trailing `# [[deps]]` / `# [mcp.*]` comment pair), the JSON sidecar at
  `<out>.json` (`name`, `agents`, `recipes`, `configured`), and the manifest ALSO placed at
  `<target>/ai-specs/ai-specs.toml` when that file does not exist yet — never overwriting one.
- S17 — Recipe ids and config keys are emitted bare when they match `^[A-Za-z0-9_-]+$` and quoted as a
  JSON-style basic string otherwise (`init_tui.py:130-142`), so `foo.bar` renders as
  `[recipes."foo.bar"]`.
- S18 — `repo_topology` is project-owned: a recipe-side answer is promoted to `[project].repo_topology`
  and never written under `[recipes.<id>.config]`.
- S19 — Deferred, with the reason in this document: the per-recipe configuration prompts
  (`init_tui.py:204-233` → `recipe-read.py` + `config_wizard.py`, the U6 surface) are T4b-2; until then
  the native wizard prints `  · configure <id> later: ai-specs recipe configure <id>` for a selected
  recipe that declares config fields. The harness-env prompts (`env_scaffold.collect_env_vars` /
  `offer_harness_env`) stay on their existing seam until T4d.
  - **Closed by T4b-2 / U4b-2 (S35-S40):** the prompt clause and its deferral line are gone — the
    step is native behind the documented CLI-dep gate seam (S37); the harness-env clause was already
    closed by T4d-2 (S28-S33, recorded there). Nothing of S19 remains open.
- S20 — The PTY suite is the contract: `tests/test_init_tui.py` wizard flows move to the native binary
  for every case that does not configure a recipe, and the four recipe-configuration cases stay on the
  legacy launcher until T4b-2, each named in the test file.
  - **Resolved by T4b-2 / U4b-2 (S40):** every configuration case drives the built binary; the
    dep-gate case's scripted leg runs INSIDE that binary through S37's seam. See S40 for what (honestly)
    still exercises the legacy module directly.

### T4b-2 / U4b-2 specs (the per-recipe configuration step)

Landed after U6d-2 — L9 of `odd/tasks/go-20-u6-recipe.md` names the pieces this slice consumes and
records S7's unblock (that unit's wizard core, prompt wiring and schema). S19's deferral is retired
here; the new numbers start at S35 because S21/S22 are a pre-existing gap in this document.

- S35 — Step 3.5 is native and follows `_configure_recipes` (init_tui.py:204-233) order and messages
  exactly: for each selected recipe id (the picker's choice order, which is questionary's own —
  `get_selected_values` iterates the CHOICES, so both routes prompt in catalog order) — read
  `catalog/<id>/recipe.toml` through `schema.LoadRecipeToml` (failure → `skip config for <id>: <err>`
  on stderr and on to the next recipe; the id joins the path verbatim like `Path` does, and the error
  DETAIL is Go's where Python printed its exception text — ADR 0002, same class as U6's deviations);
  the CLI-dep gate when the recipe declares `[[deps.cli]]` (S37) — BEFORE the fields check, as the
  Python has it, even for a recipe with no config at all; the fields check over the RAW schema (a
  recipe whose only field is `repo_topology` still gets the confirm, and then the filtered schema
  prompts nothing — exactly the Python, whose fields check precedes `_wizard_recipe`'s filter); the
  `Configure <id> now?` confirm (default yes; aborting it is the Python's `do_now is None` →
  KeyboardInterrupt → the wizard's cancel, exit 1); `runConfigWizard` over the filtered schema
  (project-owned keys removed, S18) with each field's help line on stdout immediately before its
  prompt, the message built from the PARSED `recipe.id` while values are keyed by the picker id (both
  are the Python's); blank answers omitted; a cancelled field prompt is Python's `return {}` — THAT
  recipe stays unconfigured and the loop CONTINUES. Recorded divergence, decided not accidental:
  legacy Ctrl-D inside a field prompt surfaces EOFError through questionary's `ask()` (which catches
  only KeyboardInterrupt) and init_tui turns it into a whole-wizard cancel, while internal/tui's
  prompt contract cannot tell Ctrl-C from EOF — the port answers the Ctrl-C leg, the one a hand
  reaches for, and this entry carries the EOF leg.
- S36 — The write path, decided against a plausible misreading of the handoff: the collected values
  reach the staged manifest through `_render_manifest`'s configured tables — S16's byte order
  (recipe table → config table → trailing comment pair) — NOT through `config.UpdateRecipeConfig`.
  Legacy `_configure_recipes` never writes: it returns the map. The surgical writer inserts at the
  recipe region's END (`_recipe_region_end` → `insert_at = region_end`), which for the last recipe is
  AFTER the trailing comment pair — measured from both sources — so routing init's staged bytes
  through it would break S16/S3 for no gain; `UpdateRecipeConfig` stays the authority it is on the
  configure/add routes over EXISTING manifests. The differential below proves the choice: legacy and
  native staged manifests are byte-identical on all four config cases.
- S37 — The CLI-dep gate (`recipe.cli_deps` + `config_wizard._dep_gate`) has NO native side, and this
  slice keeps that leg on the legacy script through a documented seam rather than half-porting it or
  silently skipping recipes that have deps. When a selected recipe declares `[[deps.cli]]`, the
  wizard suspends raw mode (`Terminal.Suspend/Resume` — the `internal/hub` delegate pattern this
  terminal already documents), spawns `python3 -c` running init_tui's own `_ensure_deps`, recipe-read
  and `config_wizard._dep_gate` over the SAME TTY streams (AI_SPECS_HOME pinned to the wizard's home
  through the package's existing `childEnv`), resumes raw mode, and maps the driver's exit codes:
  **0** configure; **10** declined → the legacy `Skipped <id> (missing CLI deps)` line on stderr;
  **11** read failed → the driver's own `skip config for …` line, quiet continuation; **3** → the
  wizard's rc 3 with NO message (legacy `_ensure_deps` semantics; the classic path takes over);
  anything else — a Python traceback's own exit included → `ERROR: interactive init failed: …`,
  exit 3, exactly init_tui main's failure arm. The panel, the `dep_install` TTY offer and the
  `Configure anyway?` confirm are the legacy script's own bytes on the legacy route's own streams.
  OWNER: **U6e** lands the native dep gate (for `configure-recipes`); this seam retires when init's
  wizard adopts that gate. The seam is named in code at `runLegacyInitDepGate` and
  `initDepGateDriver` in `internal/cli/initwizard.go`, and at the loop that calls it. Recorded
  divergence: the vendor bootstrap runs at the FIRST gated recipe instead of at wizard start where
  the Python runs it (the native wizard needs no Python deps otherwise, so nothing offered to install
  before then), and a decline there exits 3 exactly as the legacy decline does.
- S38 — Preview and sidecar carry the configuration: the preview's `config` row is
  `", ".join(sorted(configured))`, the deferred `(defaults / later)` when nothing was configured; the
  JSON sidecar's `configured` field is `sorted(configured)` — T4b-1 had hard-coded `[]` because T4b-1
  configured nothing.
- S39 — S19's deferral line (`  · configure <id> later: ai-specs recipe configure <id>`) is gone: the
  step it deferred is here. The `AI_SPECS_INIT_TUI_PY` whole-verb hatch stays — now the route back to
  the reference wizard, not any step's seam (no STEP needs it after this slice).
- S40 — PTY coverage, stated honestly: the configuration cases in `tests/test_init_tui.py` drive the
  BUILT BINARY end to end (per-recipe confirm, field prompts, staged config block, the decline, the
  dep-gate skip, the topology routing) — five cases, all repointed from the legacy launcher with this
  slice. The dep-gate case does so THROUGH the seam: its legacy leg (panel, install offer,
  `Configure anyway?`) is spawned by that binary on the same PTY. The in-process Go tests script the
  step and every gate outcome the PTY cases cannot reach (read failure, rc abort, gate-before-fields
  order, no-cli_deps non-invocation, the seam's non-TTY refusal). The `runpy` drivers left on the
  legacy module are the TRIAGE data-level contracts of init_tui.py itself (mocked
  `_configure_recipes`/`_wizard_recipe`), not launcher cases — they are what still exercises the
  legacy module directly, by design.

## T4b-2 / U4b-2 — verification

The per-recipe configuration step is native (`_configure_recipes` runs inside
`internal/cli/initwizard.go`), its values reach the staged manifest through the Python's own render
(S36), and the CLI-dep gate keeps its legacy leg behind the documented seam (S37). Gates, run from
`.worktrees/go-20-u4b2-config` with `PATH=/tmp/pybin:$PATH`:

| gate | command | result |
| --- | --- | --- |
| touched packages | `PATH=/tmp/pybin:$PATH go test ./internal/cli/ ./internal/tui/` | both `ok` |
| Go suite | `PATH=/tmp/pybin:$PATH go test ./cmd/... ./internal/...` | exit 0 — 22 packages `ok`, 0 failing |
| init contract + wizard PTY | `PATH=/tmp/pybin:$PATH python3 -m unittest tests.test_init_tui tests.test_init_native tests.test_init_brief_route` | **47 tests, OK** — including all five configuration cases on the NATIVE binary |
| formatting | `gofmt -l internal/` | empty |
| manifest untouched | `git diff --quiet -- ai-specs/ai-specs.toml` | exit 0 |
| differential (S3) | twin PTY runs, legacy launcher vs built binary, byte-comparing `ai-specs/ai-specs.toml` and the exit codes across four config cases (block written, configure declined, topology+config yes, dep-gate declined) | all four manifests byte-identical, rc 0 on both routes |

- RED, observed: the new scripted tests did not compile against the base (`undefined: wizardDepGate`,
  `unknown field DepGate in struct literal`, `undefined: wizardDepGateOutcome`) — the seam's API did
  not exist yet. GREEN: one implementation pass later every new test passed (six subtests of the
  seam pin included), then the packages, the full Go suite and the three Python suites.
- **What this closes:** all of S19 (prompt clause here, harness-env clause already closed by T4d-2)
  and S20's "stay on the legacy launcher until T4b-2". Of S27, nothing was open for this slice:
  U4d-2's section records its prompt/write/migration clauses closed; the only S27-class item still
  deferred anywhere near init's code is S33's direnv-INSTALL offer, named in
  `internal/sync/envscaffold.go` and retired with the deps surface, not here.
- **What remains, named in code and here:** (1) the CLI-dep gate seam —
  `runLegacyInitDepGate`/`initDepGateDriver` in `internal/cli/initwizard.go`, retired by **U6e**
  (the dep gate for `configure-recipes`) when init adopts that gate; (2) the `refresh-bundled`
  flatten seam (S9, moves to U7 with its readers); (3) the `AI_SPECS_INIT_TUI_PY` whole-verb hatch
  (reference route until U7 — no step needs it any more); (4) S33's direnv-install offer. Physical
  deletion of the legacy files stays U7/U8 through U6's S9.
- **What U4b-2 leaves as the last border of `init`:** the verb now owns its whole flow natively
  except those four named seams — of which the dep gate (S37) is the only one a normal interactive
  run still REACHES (the others fire only with an env override, a missing cache consumer, or a
  missing direnv). The wizard itself — every prompt, the staged bytes, the sidecar, the env offer —
  is native end to end.
- **The dep-gate decision: option (a)**, the documented legacy seam. Why not (b): the gate is an
  interactive surface (panel + install offer + confirm + skip line) that (b) would silently DELETE
  from the native route for every `[[deps.cli]]` recipe — no panel, no `Configure anyway?`, never the
  skip — a live divergence removable only by U6e, with the PTY case stranded on the legacy launcher
  meanwhile; the program rejected exactly that trade in go-20-u6-recipe.md L4 (dropping an
  interactive surface no slice re-adds). Why (a) is affordable: the suspend/resume delegation of a
  legacy Python child on the same TTY is production-proven in this very binary (`internal/hub`'s
  `delegate`), and `init` already execs python3 every run (`refreshBundledAtInit`), so the seam adds
  no dependency class — and it keeps every message byte-legacy through U6e.
- **Spec findings (reported, not absorbed):** (a) The slice handoff's goal line says the values flow
  `runConfigWizard → the same write path U6d-1 uses` (`config.UpdateRecipeConfig`). That conflicts
  with S16/S3 and with the legacy source (S36 above): the surgical writer's region-end insert would
  put the config table after the trailing comments. Specs are authoritative over the handoff
  summary, so the render path shipped and the differential proves byte parity; if `UpdateRecipeConfig`
  for init's staged manifest was really the intent, S16/S3 must be amended explicitly first.
  (b) go-20-u6-recipe.md L9 claims "init (U4b-2) will exercise the existing-wins step for real". It
  cannot: legacy passes `{}`, and the native wizard only ever runs when NO manifest exists (the
  shell's skip branch), so `existing_config` is always empty on this route — recorded as a comment at
  the call rather than a test that could only assert emptiness; configure-recipes (U6e) remains the
  route that exercises it. (c) This document skips S21/S22 (T4b ends at S20, T4d starts at S23) — a
  pre-existing numbering gap, which is why the new block starts at S35 instead of backfilling
  numbers other documents may already cite.
  (d) The hatch's comment in
  `tests/test_init_native.py::test_init_tui_py_env_seam_keeps_legacy_wizard_delegation` said the hatch
  "keeps the recipe-configuration PTY cases … observable" — stale the moment this slice landed, and
  that file is outside the worker's surfaces. The PARENT corrected it in the same slice (it now says
  "Reference/rollback hatch, not a step's seam"), so this clause is closed, not deferred. The review's
  readability lens flagged this clause as stale WITHOUT re-reading the file — a false positive worth
  recording: the sentence describing a defect can outlive the defect. (e) S20 said "the four recipe-configuration cases"; the file actually held FIVE
  tests with a `Configure …` stage (the topology suite has two) and the fifth never carried the
  legacy comment — all five now drive the built binary.

### T4d / U4d specs (the harness env)

The step `sync` runs today is `python3 lib/_internal/env_scaffold.py <root>`, which is the
NON-interactive half of the module: it never prompts. The prompts (`prompt_env_vars`,
`offer_harness_env`) belong to `init`'s wizard path. So U4d splits again.

- S23 — The env step `sync` runs is a port of `env_scaffold.py`'s `main()`: refuse when
  `ai-specs/ai-specs.toml` is missing (`Project not initialized: missing ai-specs/ai-specs.toml under
  <root>` on stderr, exit 1), write `<root>/ai-specs.env.example`, ensure the managed block in
  `<root>/.envrc`, warn once per required variable with no value, and print `Wrote <path>`.
- S24 — `ai-specs.env.example` is generated from the `[[provides.mcp]]` env references of the ENABLED
  recipes (`collect_env_vars`): one `VAR=default  # required by <preset> (<recipe>)` line per variable,
  sorted, with the `ENV_VAR_HELP` sentence appended when the variable has one and the
  `ENV_EXAMPLE_DEFAULTS` value pre-filled (never written to `ai-specs.env`). The header is
  `_ENV_EXAMPLE_HEADER` verbatim, the no-variable case is the literal
  `# (no env vars required by enabled recipes)`, and a changed file keeps the previous bytes in
  `<root>/ai-specs.env.example.bak` while an unchanged one is not rewritten at all.
- S25 — The root `.envrc` carries ONE managed block (`MANAGED_START`, the two `dotenv_if_exists` lines,
  `MANAGED_END`): created with the two-line header when the file is absent, replaced in place when the
  block is present, and appended after a blank line when it is not. Lines outside the block are the
  user's and survive byte for byte.
- S26 — `direnv allow` is NOT run by this step (the Python does not either); it stays in the wizard's
  offer path (U4d-2), where the human is present.
- S27 — Deferred to U4d-2, named in code: `prompt_env_vars`, `offer_harness_env`, `write_env` (the
  merge that never wipes a non-empty secret with a blank answer), the three legacy migrations
  (`migrate_nested_harness_env`, `migrate_legacy_envrc`, `migrate_legacy_harness_env`) and
  `direnv allow`. Until it lands, `init`'s offer keeps its `env_scaffold.py` seam.

## T4d-1 — verification

The env step `sync` runs is native and `sync` no longer execs Python for it. Ported by a bounded
writer with closed surfaces, then reviewed here.

| gate | command | result |
| --- | --- | --- |
| the two touched packages | `go test -count=1 ./internal/doctor/ ./internal/sync/` | `ok 3.538s` / `ok 45.631s` |
| Go suite | `go test ./cmd/... ./internal/...` | 0 failing packages |
| init contract | `python3 -m unittest tests.test_init_native tests.test_init_brief_route` | 16 tests, OK |
| differential parity | `python3 tests/parity/run.py` | 29 fixtures, failing 0 |
| formatting | `gofmt -l internal/` | empty |

- **Differential against the Python, hand-run on twins** (identical manifest and catalog, one driven by
  `python3 lib/_internal/env_scaffold.py <root>`, the other by the built binary's `sync`):
  `ai-specs.env.example` and `.envrc` are **byte-identical**, with the same four sorted warnings and the
  same trailing `Wrote …` line. Zero divergence, so nothing had to be recorded.
- **One authority for the dotenv surface.** The port needed the parsers doctor already had. `sync`
  cannot import `doctor` (the arrow already points the other way), so the shared home is
  `internal/sync` and doctor's private helpers became one-line delegations — the same move this repo
  already made for `ProjectOwnedRecipeConfig`. Every doctor caller and every doctor test is unchanged;
  the literal oracles live on both sides. `LoadHarnessEnv` kept doctor's exact semantics (a non-regular
  file is never opened, invalid UTF-8 reads as empty).
- **A finding worth keeping:** `tests/test_env_scaffold.py` and `tests/test_sync_env_scaffold.py` drive
  `bin/ai-specs sync`, i.e. `lib/sync.sh`, which still execs the Python for this step. They are green
  and they do **not** cover the native step; the Go differential is the only thing that can catch its
  regressions. Repointing them belongs to the launcher flip (U2), not here.
- Deliberately out (S27, named in the ported file): the prompts, `write_env`, the three legacy
  migrations and `direnv allow` — U4d-2, still on the Python seam.

### T4d-2 / U4d-2 specs (the wizard's env offer)

U4d-1 ported the non-interactive half. This slice closes the wizard's tail.

- S28 — Gap being fixed, measured: `init_tui.py:390-391` calls
  `env.collect_env_vars(target)` and, when it answers anything, `env.offer_harness_env(target)`. The
  native wizard (U4b-1) does **not**, so a `--tui` run through the Go binary silently skips the env
  offer the Python one makes. That is a behaviour gap, not a seam.
- S29 — The offer, in order (`env_scaffold.py:572-641`), all soft-failing with the Python's
  `… skipped` / `Could not …` lines instead of aborting the verb: migrate the legacy files, collect the
  variables, prompt, write `ai-specs.env`, regenerate the example and the managed `.envrc` block, run
  `direnv allow`.
- S30 — `prompt_env_vars` (`:496-547`): a `Required environment variables` banner with each variable's
  purpose and its `ENV_VAR_HELP` sentence, then `Configure these values now?` (default yes). Declining
  that confirm is a CANCEL of the whole offer: `prompt_env_vars` answers `None` for a decline and for a
  cancel alike, and `offer_harness_env` returns on `None`, so nothing is written — no `ai-specs.env`, no
  example, no `.envrc` work, no `direnv allow`. Per variable, sorted: a select over the recipe's declared
  `env_allowed` values when it has them — with `_select_default`'s three-step default (matching existing
  value, else matching example default, else the first choice) — a HIDDEN prompt when the name looks
  like a secret (`API_KEY`/`TOKEN`/`SECRET`/`PASSWORD`/`APIKEY`), and a text prompt otherwise.
  Cancelling any prompt cancels the offer without writing `ai-specs.env`.
  - The select PRESELECTS that default: questionary passes it as `initial_choice`, so Enter answers the
    default without moving the cursor. The port needs a Select with an initial cursor for exactly this.
  - The text prompt for a value returns the RAW buffer, not the stripped one: `questionary.text` does not
    trim, and S14's `strip()` belongs to the project-name step only.
- S31 — `write_env` (`:277-299`) merges and never destroys, with two DIFFERENT rules that this line
  originally collapsed into one:
  - a blank/whitespace answer is skipped, so re-prompting cannot wipe a secret;
  - a NON-blank answer REPLACES the previous value — that is how a secret gets rotated — and every other
    key in the file is kept.
  The "only fill keys that are missing or empty" rule belongs to `_merge_into_harness_env`, which the
  three migrations use, not to `write_env`. Corrected here against
  `tests/test_env_scaffold.py::test_write_env_merges_preserves_extras` (an existing key takes the new
  value and extra keys survive); the earlier wording would have forbidden re-prompting a secret at all.
  The file keeps the Python's header and sorted `KEY=value` lines with `_quote_dotenv`'s quoting.
- S32 — The three migrations rename, they do not delete: `ai-specs/.env` and `ai-specs/.envrc` are
  parsed (dotenv, and export lines for the nested file), merged under the same never-wipe rule, and only
  then renamed to `.bak` — and only when at least one key was parsed (JD-6). The nested dotenv runs
  first so the export migration fills gaps only.
- S33 — Deferred and named in code: when `direnv` is absent, the Python offers to INSTALL it through
  the deps layer (`dep_install.py`); that belongs to the deps surface, not to init. The port prints the
  Python's three-line "not installed" message and stops there.

## T4d-2 — verification

The wizard's env offer is native, and the gap S28 described is closed: a `--tui` run through the binary
now offers the variables the enabled recipes need, exactly where the Python did.

| gate | command | result |
| --- | --- | --- |
| the three touched packages | `go test ./internal/tui/ ./internal/sync/ ./internal/cli/` | `ok 0.430s` / `ok 30.790s` / `ok 3.377s` |
| Go suite | `go test ./cmd/... ./internal/...` | 0 failing packages |
| wizard PTY, end to end | `python3 -m unittest tests.test_init_tui` | **31 tests, OK** — including the harness-env case, now on the NATIVE binary: all seven PTY stages, the hidden prompt among them |
| init contract | `python3 -m unittest tests.test_init_native tests.test_init_brief_route` | 16 tests, OK |
| differential parity | `python3 tests/parity/run.py` | 29 fixtures, failing 0 |
| formatting | `gofmt -l internal/` | empty |

- **Two spec corrections, both against my own prose and in favour of the reference:**
  - S30 said a decline of `Configure these values now?` continues without values. It does not:
    `prompt_env_vars` answers `None` for a decline AND for a cancel, and the offer returns on `None`, so
    the whole offer stops (no `ai-specs.env`, no example, no `.envrc` work, no `direnv allow`).
  - S31 collapsed two different merge rules. `write_env` SKIPS a blank answer and REPLACES with any
    non-blank one — that is how a secret is rotated — while "only fill keys that are missing or empty"
    is `_merge_into_harness_env`'s rule, the one the three migrations use. The pinning test
    (`test_write_env_merges_preserves_extras`) settles it.
- **Preserved on purpose**, with new pins because shared code moved: `Text` still strips and keeps its
  default (S14) while the new `TextRaw` returns the raw buffer the env values need, and `Select` renders
  byte-identically while `SelectDefault` adds questionary's preselected `initial_choice`.
- **Deferred, named in code (S33):** when `direnv` is missing the Python offers to install it through the
  deps layer; the port prints the three-line message and stops.
- **The PTY suite now reads honestly:** the harness-env case lost its "LEGACY launcher on purpose"
  comment because the offer it exercises is native. The four per-recipe-configuration cases keep theirs,
  with U4b-2 named as the slice that removes them.
- S34 — Divergences found by the judgment round, each decided rather than fixed:
  - `ai-specs.env` is created **0600**, not the 0644 Python's `write_text` leaves under the usual
    umask (JD-ENV-A-002). The file holds the API keys and tokens the offer just collected, so the port
    is deliberately stricter; nothing else about the file changes, an existing file keeps its own mode,
    and a test pins it.
  - The `writeTextFile` comment cost two rounds to get right, and both failures were mine: the first
    rewrite said it wrote `ai-specs.env.example` (the example has its own writer inside
    `generateEnvExample`), and the second said its only caller was "below" when that caller sits above the
    function. The final text names the single caller without a direction word and explains why the two
    neighbours are not routed through it; every sentence was then checked mechanically — call sites,
    definition order, both file modes — instead of from memory. Both judges' re-judgment evidence named
    the same defect class at the same place; judge B's landed on the superseded revision (it quotes the
    `6578314` wording, which no longer exists at `4dbcd12`), which is the one caveat worth recording: a
    resolution is bound to the revision it read, and the text it asked for is the text that shipped.
  - An EXISTING `ai-specs.env` keeps whatever mode it already had: `os.WriteFile` does not chmod, so a
    file created by an older build (or by the Python) stays 0644 while a fresh one is 0600
    (JD-R1-existing-env-file-keeps-weak-mode, advisory). Recorded rather than "fixed": silently
    narrowing a user's file on every sync is a bigger surprise than the mode they already chose.
  - A Ctrl-D on an empty hidden/raw prompt cancels the offer **silently**, where the Python lets
    `prompt_toolkit` raise `EOFError` and prints `Could not configure environment variables: …`. Nothing
    is written either way; the only difference is that one soft-fail line. Recorded, not fixed: plumbing
    "cancelled by EOF" through the prompt interface would add a case to every prompt for a line no user
    needs. If it ever matters, the fix is an EOF-specific error next to `ErrEnvOfferCancelled`.

## Slicing

| slice | scope | retires |
| --- | --- | --- |
| **T4a** | classic non-interactive `init` native: args, target resolution, directories, template manifest, `AGENTS.md` baseline brief, root `.gitignore` block, `ai-specs/.gitignore`, lock stamp, next steps; TUI gate delegates | `init.sh:35-120,195-233,235-282,284-325,327-348` shell orchestration |
| T4b-1 | the wizard, native: text prompt, name, topology, agents, recipes, preview, confirm, manifest render, JSON sidecar, staging | `init_tui.py:234-400` minus the recipe-config step |
| T4b-2 | per-recipe configuration prompts, native (S35-S40): read → dep-gate seam → fields check → `Configure <id> now?` → `runConfigWizard` → the render; the CLI-dep gate itself stays on its documented legacy seam (S37), owned by U6e | `init_tui.py:204-233` MINUS `_dep_gate` — the gate leg survives on the seam until U6e lands the native gate |
| T4c | the lock `[meta]` writer, native, wired into `sync` | `cli_version.py stamp-meta` (the `sync` call site) |
| T4c-note | the `refresh-bundled` flatten is NOT retired here: the Python readers that consume `{cache}/.bundled/` outlive this unit, and `init`'s step stamps nothing (see S9's correction) | moves to U7 with those readers |
| T4d-1 / U4d-1 | the env step `sync` runs, native: `ai-specs.env.example`, the managed `.envrc` block, the missing-value warnings | `env_scaffold.py`'s non-interactive half |
| T4d-2 / U4d-2 | the wizard's env offer: prompts (`select`/hidden/text), `ai-specs.env` merge, the three legacy migrations, `direnv allow`, wired after the wizard's staging | `env_scaffold.py`'s interactive half, `envrc-scaffold.py` |

## Tasks

- T1 (S3, S5) — the native `runInit` route with the legacy argument contract (`--name`, `--name=`,
  `--force`, `--adopt-brief`, `--tui`, `--no-tui`, `--`, `-h`, unknown flag and extra positional
  errors with their exact messages and exit codes) and the `--help` text.
- T2 (S5) — target resolution (cwd default, not-a-directory error and exit 1, absolute path, name from
  basename) and the banner with its four rows.
- T3 (S4, S7) — directories, template manifest write (never overwrite), and the recorded sed
  divergence (S8).
- T4 (S5) — the baseline `AGENTS.md` chain in process: policy gate, render, placeholder fallback, and
  the `brief.render = false` branch, all with the legacy messages.
- T5 (S5) — root `.gitignore` agent block (append / keep / refresh on `--force`, marker-idempotent)
  and `ai-specs/.gitignore` from the manifest, both in process.
- T6 (S6) — the TUI gate and the delegation to the legacy script.
- T7 (S9) — the lock stamp and the bundled step, through the named seam.
- T8 (S5) — the next-steps text, byte-identical to the heredoc.
- T9 (S3) — tests: Go unit tests for the argument contract and each rule, plus a shell test that runs
  the native route and the legacy route in two throwaway projects and diffs stdout, stderr, exit code
  and the produced tree.

## Log

- L1 — Human, verbatim (2026-10-17): "continuemos donde nos quedamos respecto a la migración a full
  go".
- L2 — Evidence (2026-10-17), delegated read-only mapping of the legacy surface (`init.sh` 348 lines,
  `init_tui.py` 429, `env_scaffold.py` 667, `envrc-scaffold.py` 46, `refresh-bundled.py` 160,
  `cli_version.py` 302): the phases above, what is already native (`internal/target`, `internal/sync`
  brief/gitignore, `internal/cliversion` read side, `internal/lock` serialization) and what is not
  (manifest template write, wizard, env write path, `[meta]` writer). The slicing table is the
  consequence of that measurement.

## T4a — judgment and fix round

Three independent reads of the frozen candidate (`266684e0` on base `f2cb80a1`) ran after the unit was
written, because the native review of this candidate could not close (two of four lenses' models could
not finish; recorded in the program's memory, not in this document):

- **Judgment Day, judge A** — 8 rows: 4 WARNING, 4 SUGGESTION, 0 CRITICAL.
- **Judgment Day, judge B** — 6 rows: 1 HIGH, 3 WARNING, 2 SUGGESTION. Its HIGH is the same defect
  judge A filed as a WARNING at the same line, reproduced independently.
- **Independent verifier** — `PASS`, S1-S11 met, S8/S10 real and singular, one advisory (stderr text
  on the manifest-as-a-directory path), plus two regression tests to hand back.

The fix batch, in one commit:

| id | finding | fix |
| --- | --- | --- |
| C1 | a trailing `--name` bootstrapped the project where the shell aborts at `shift 2` with exit 1 and no writes | the parser refuses it before any write, exit 1, `--name requires a value` (S12) |
| C2 | bare `-` was read as a target path; the shell's `-*` matches it as an unknown flag | it is an unknown flag again, exit 2 with the help hint |
| C3 | the name was substituted into `name = "..."` unescaped, so a backslash or a quote produced a manifest that cannot parse — permanently, because init never overwrites it | `tomlBasicStringBody` escapes the TOML basic string; S8 grows this half and a test pins `a\q`, `a"b` and `a\nb` |
| C4 | the `ai-specs/.gitignore` step failed silently where the shell printed a traceback naming the manifest | the step prints one line naming both paths (S12) |
| C5 | `appendGitignoreBlock` truncated the user-owned root `.gitignore` before writing; the shell appended and renamed | the same bytes are written beside the target and renamed into place |
| C6 | one test asserted `filepath.Base` on its own literal, and the parser cases sat exactly where the drift was | the basename is asserted through `runInit`; `--name`, `--name=` and `-` are pinned; the next-steps text is compared whole |
| C7 | the new route constant broke `gofmt` alignment | `gofmt -w` on the touched files |
| C8 | the silent writer's comment claimed `init` uses it, contradicting S11 | the comment names the printing wrapper as init's call |
| C9 | `mkdir` failure was reported on stdout with a partial `✓` line already printed | both directories are created first, the diagnostic goes to stderr, and no `✓` is printed on failure |
| C10 | a signal-killed `refresh-bundled` child returned `-1`, which `os.Exit` turns into 255; the shell reports 128+signal | the 128+signal rule `runShim` already implements |
| C11 | the verifier's probe had no regression test | `test_manifest_directory_is_never_destroyed_and_exits_1` and `TestInitManifestDirectoryIsNotDestroyed` |

Evidence after the batch (same interpreter, `/tmp/pybin/python3`):

| gate | result |
| --- | --- |
| `go test ./internal/cli/ ./internal/sync/` | both packages `ok` |
| `python3 -m unittest tests.test_init_native tests.test_init_brief_route tests.test_init_tui` | 44 tests, OK |
| `python3 tests/parity/run.py` | 29 fixtures, failing 0 |

One incident is recorded rather than hidden: writing the C11 regression test, a first version passed the
project path to `--name`, which made the legacy route read its own cwd as the project and run a real
`init` in the worktree. The only write it made was a re-stamp of `ai-specs/.ai-specs.lock`, restored
with `git checkout`. The test now runs with `cwd` inside the throwaway project and its docstring says
why.

### Round two — the fix of the fix

The scoped re-judgment returned 7 `verified` and one `regression`: **JD-B-003**. C5's atomic write
renamed over the path, so a SYMLINKED root `.gitignore` became a regular file and the user's real file
— the one `>>` would have written through — kept its old contents. Same trap the manifest writer hit in
the previous unit. C5b resolves the link (and, for a dangling one, the target `>>` would have created)
and renames over the target; `TestInitRootGitignoreKeepsASymlink` and
`TestInitRootGitignoreCreatesADanglingLinksTarget` pin both, and the differential against `lib/init.sh`
on a symlinked `.gitignore` now produces identical target bytes with the link intact.

### Terminal judgment

| round | judge A | judge B |
| --- | --- | --- |
| discovery | 8 rows, 0 CRITICAL | 6 rows, 1 HIGH |
| scoped re-judgment | 7 `verified`, 1 `regression` (JD-B-003) | 8 `verified` |
| round two (JD-B-003 only) | `verified` | — |

**JUDGMENT: APPROVED.** No severe row survives, the final verification passes (the post-fix gate table
above), and the observed divergences from the legacy route are the recorded ones — S8 (literal, escaped
name substitution) and S10 (no Python bridge notices), plus S12's message texts that keep the shell's
exit codes and streams. Judgment Day is an outside view only: it authorizes nothing about delivery,
which stays ordinary repository policy.

## T4a — verification of the first cut (superseded by the round above)

Evidence gathered before the judgment round, kept because it records the RED and the manual
differential that motivated the committed black-box test. The numbers below are pre-fix; the
post-fix gate is the table in the section above.

Run from `.worktrees/go-20-u4a-init-core` with
`PATH=/tmp/pybin:$PATH` (the canonical 3.14.7 interpreter; the machine's `/usr/bin/python3` is 3.9
and fails unrelated suites — recorded in the program's memory notes):

| gate | command | result |
| --- | --- | --- |
| Go suite | `go test ./cmd/... ./internal/...` | 0 failing packages |
| init contract, classic + wizard seam | `python3 -m unittest tests.test_init_native tests.test_init_brief_route tests.test_init_tui` | 42 tests, OK |
| differential parity | `python3 tests/parity/run.py` | 29 fixtures, failing 0 (both modes) |

- RED, observed: the S8 test (`test_name_with_ampersand_lands_literally_on_the_native_route`) fails
  against the base commit's binary, where `init` still routes to the shim and the manifest carries
  `name = "R{{PROJECT_NAME}}D"`; it passes with this slice.
- The manual differential before the committed test existed: two twin projects (same basename, own
  isolated homes), stdout identical after dropping the random `RECIPE_MCP_TEMP:` names, same produced
  file set, and the only stderr difference is S10's absent bridge notices.
- Two divergences are written down with their reason rather than hidden: S8 (literal name
  substitution) and S10 (no Python bridge notices). S11 records that the `ai-specs/.gitignore` status
  line is part of init's stdout, because the shell calls the renderer directly.
- Not a divergence, verified: the `keep` and `--force` branches of the root `.gitignore` block match
  the legacy bytes exactly, including the single separating newline the shell's `append_block` writes
  (which is NOT `gitignore-root-refresh.py`'s two-newline rule — the two algorithms differ and init
  uses its own).
