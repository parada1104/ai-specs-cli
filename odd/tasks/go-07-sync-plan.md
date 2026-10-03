# [Go 07] Sync pipeline port — planning package (docs-only)

Card: `6a84e7acca1baf394d9b482b` — https://trello.com/c/ITeLc5xL
Branch: `change/go-07-plan-sync` · Worktree: `.worktrees/go-07-plan-sync`
Base: epic tip `bdf0fe87cff810faac068d4e02aad0080e86e68f` (`epic/go-single-binary`)
Session scope: **planning only — no production Go code** (card text, binding).
Process: ODD feature document. No `openspec/changes/**` folder was created: this
repo's convention for epic card *planning* is `odd/tasks/*.md` (cards 03–06 all
landed that way), and the card forbids inventing new process.

## Goal

Produce a review-ready plan for porting the whole sync pipeline to Go with
byte-identical emitted artifacts, decomposed into chained PRs by module
boundary, with the strangler mechanics, risk register, test strategy and the
human decisions that block implementation made explicit.

Non-goals: no Go production code, no per-slice ODD stubs, no tests/ edits.

## Root verification (required before the first write — boundary is binding)

```
git rev-parse --show-toplevel  → /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/go-07-plan-sync
git branch --show-current      → change/go-07-plan-sync
git log --oneline -1           → bdf0fe8 feat(go): embed catalog, templates and bundled assets via go:embed (epic [Go 06]) (#309)
git worktree list              → 6 worktrees; this one at bdf0fe8, epic worktree at bdf0fe8
git status --short             → (empty)
```

No `ai-specs` harness operation was run against this repo's own `ai-specs/`
(dogfood hazard); the pre-write baseline is clean, so no provisioning output
(`AGENTS.md`, `ai-specs/.ai-specs.lock`) exists to be reverted.

---

## Method and evidence base

Inventory was built by reading the modules and their callers directly in this
worktree (no MCP tools were used, per the card). Every claim below cites
`file:line`. Claims I could not verify are marked `UNVERIFIED:`.

Shorthand used throughout: `…contract.md` = `docs/go-migration-parity-contract.md`
(the authoritative FROZEN/TOLERANT/FREE inventory, card 01), and "the gate" =
the separate `worktree-gate` Go module at `catalog/recipes/worktree-flow/gate`
(own `go.mod`, own release artifact and trust root).

Read as primary sources: `lib/sync.sh`, `lib/sync-agent.sh`,
`lib/_internal/{recipe-materialize,agents-render,brief-render-policy,hooks-render,mcp-render,recipe-conflicts,skill-resolution,skill_contract,project-cache,flatten-resolved-skills,gitignore-render,gitignore-root-refresh,platform.sh,gate_binary}.py|sh`,
`docs/go-migration-parity-contract.md`, `docs/go-migration-orchestration-handoff.md`,
`tests/parity/{parity,run}.py`, `tests/{run,validate}.sh`, `tests/_blackbox.py`,
`internal/*`, `catalog/recipes/worktree-flow/gate/**`, `scripts/*`.

---

## (a) Module inventory

### A0. Pipeline spine

Exactly two Bash entry points orchestrate everything else. Both are already
dispatcher-shimmed in the Go binary (`internal/cli/cli.go`, `shims` map: `sync`,
`sync-agent`:
**verb-level** surfaces the parity harness can compare end to end.

#### `lib/sync.sh` (299 lines)

| Aspect | Value (evidence) |
| --- | --- |
| Usage | `ai-specs sync [path] [--ignore-cli-version] [-v\|--verbose]` (`usage()` `L21-43`) |
| Flags | `--ignore-cli-version`, `--refresh-gates`, `--adopt-brief`, `-v\|--verbose`, `-h\|--help`, `--` (parse loop `L45-76`) |
| Positional | optional `path`, first only; a second positional is exit **2** (`L66-68`) |
| Unknown flag | exit **2** + `Run 'ai-specs sync --help' for usage.` (`L58-62`) |
| Shell contract | `set -euo pipefail` `L16`; `shopt -s inherit_errexit` when available `L90-91` |
| Exit codes | 0 ok; **1** target-resolution failure `L177-180`, missing manifest `L186-190`, `cli_version.py check-sync` failure `L192` (its 2 is collapsed to 1), `recipe-materialize.py` rc passed through verbatim `L262-267`, any fan-out target failure `L290-293`; **2** flag/positional errors `L61`/`L68` |

Ordered steps (order is **FROZEN**, `docs/go-migration-parity-contract.md:341-361`):

| # | Step (line) | Command | Artifacts |
| --- | --- | --- | --- |
| 0 | target resolution `L177` | `target-resolve.py "$TARGET_PATH"` → `PLAN_JSON` (`root`/`planning_root`/`topology`/`targets` extracted `L182-183`) | `root`, `planning_root`, `topology{resolved,via,source}`, `targets[{path,kind,rel}]` |
| 1 | `ai-specs/.gitignore` `L216` | `gitignore-render.py TOML AI_GITIGNORE` | `ai-specs/.gitignore` |
| 2 | root `.gitignore` agent block `L218` | `gitignore-root-refresh.py ROOT templates/gitignore-root.tmpl` | root `.gitignore` managed block |
| 3 | bundled skills + commands `L220` | `refresh-bundled.py ROOT HOME` | cache `.bundled/**`, `ai-specs/.ai-specs.lock` |
| 4 | vendored skills `L222` | `vendor-skills.py ROOT` | `ai-specs/.deps/**`, lock |
| 5 | harness env `L224` | `env_scaffold.py ROOT` | `ai-specs.env.example`, managed `.envrc` block |
| 6 | recipes, capture block `L226-272` | `recipe-materialize.py ROOT HOME --recipe-mcp-out … --resolved-config-out … --resolved-hooks-out … [$REFRESH_GATES]` (`L250`) | recipe/dep skills, commands, templates, docs, gate hooks, lock orphans |
| 7 | `AGENTS.md` `L274-281` | via `sync_agents_render()` gated on `brief-render-policy.py == true` (`L275`) | `ROOT/AGENTS.md` |
| 8 | fan-out, one `run_step` per target `L283-293` | `bash sync-agent.sh --source-root ROOT --target T --all --recipe-mcp … --resolved-config … --resolved-hooks … [--adopt-brief] [--verbose]` (`L286-289`) | per-agent artifacts |
| 9 | `stamp-meta` `L296` | `cli_version.py stamp-meta ROOT HOME` | `[meta]` in manifest |

Output/stream contract (FROZEN, `…contract.md:200-227`):

- `run_step LABEL CMD…` (`L125-175`): prints `  syncing LABEL`, runs the step with
  stdout/stderr captured to two temp files, replays each on its original stream.
- `print_step_output FILE` (`L101-118`): verbose = `cat` the file byte-exact;
  compact = drop blank lines and every line whose first non-space char is one of
  `✓ · ⇢ ▸`. The other glyphs (`!`, `✗`, `ℹ`) must survive compact mode.
- On step failure the **full unfiltered** stdout/stderr is printed before the rc
  is returned (`L162-168`).
- `errexit` stays **off** inside `run_step` until replay and cleanup finish
  (`set +e` `L147`, restore `L171`; fix `a95bb01` =
  `fix(sync): restore errexit after run_step cleanup (#221)`, present in this
  branch's history); restoring it earlier lets a failing `cat` (SIGPIPE /
  ENOSPC) abort the script, leak both temp files and replace the wrapped status.
- `mktemp` failure degrades the step to **unfiltered direct execution** with an
  explicit `  ! cannot create temporary files (check TMPDIR)…` warning and
  returns the command's own rc (`L136-145`).
- Header block `ai-specs sync` + `root/planning/topology/targets/fan-out/derived`
  lines (`L203-214`), footer `✓ ai-specs sync complete` (`L298-299`).

Temp-file discipline (must be reproduced, not improved):

- Five temp paths `L226-228`/`L247`, one `EXIT` trap registered **before** the
  remaining `mktemp` calls (`L246`), with `${VAR:-}` expansion because `set -u`
  makes a trap naming an unset variable die mid-cleanup and replace the exit
  status (`L229-245` comments the two measured reasons).
- `RECIPE_NAMES` is grepped out of materialize's human-readable stdout:
  `grep -oE '▸ recipe [^ ]+' "$RECIPE_OUT_FILE" | sed -E 's/.*recipe //' | paste -sd, -`
  (`L256`). This is the inter-module API hazard recorded in
  `…contract.md:200-212`; the friendly line `  syncing recipes → a, b` (`L258`)
  depends on it.
- `recipe-materialize.py`'s rc is propagated with bare `exit $RECIPE_RC`
  (`L267`) **after** the capture files are printed and removed; the same
  errexit-off-until-cleanup rule applies (`set +e` `L249`, restore `L272`).
- `export AI_SPECS_SYNC_NESTED=1` before the fan-out loop (`L283`), so the child
  `sync-agent` suppresses its own banner/footer (`…contract.md:368`).

#### `lib/sync-agent.sh` (551 lines)

| Aspect | Value (evidence) |
| --- | --- |
| Usage | two forms: `sync-agent [path] [--all\|--<agent>…]` and `sync-agent --source-root R --target T […]` (`L34-62`) |
| Flags | `--source-root`, `--target`, `--recipe-mcp`, `--resolved-config`, `--resolved-hooks`, `--adopt-brief`, `--all`, `--claude\|--cursor\|--opencode\|--codex\|--copilot\|--gemini\|--pi\|--omp`, `-v\|--verbose`, `-h`, `--` (parse loop `L64-106`) |
| Unknown flag / 2nd positional | exit **2** (`L93`, `L100`) |
| Exit codes | 0 ok; **1** standalone resolution/child failure `L115`, fan-out child failure `L165`, missing manifest `L188`, local materialize failure `L199`, same-root missing `AGENTS.md` `L361`, `brief.render=false` with no `AGENTS.md` `L382`; **2** usage. `L170` is `exit 0` after the standalone loop and `L409` is the D24 `exit 0` |
| Env | `AI_SPECS_SYNC_NESTED=1` suppresses banner+footer (`L426-436`, `L548-551`) |
| Reads | `platform.sh` (`L22-23`) — the per-agent path/key matrix: `instructions_path`, `skills_dir`, `mcp_config_path`, `mcp_key`, `native`, `commands_dir`, `runtime_hooks_target` |

Behaviour:

1. **Standalone multi-target mode** (`L112-172`): when neither `--source-root`
   nor `--target` was given and target resolution yields >1 target, it prints a
   `public root fan-out` banner and re-execs itself once per target with
   `--source-root ROOT --target T` and `AI_SPECS_SYNC_NESTED=1`, exit **1** on
   the first child failure (`L165`).
2. **Standalone resolved-config** (`L144-160`): runs
   `recipe-materialize.py ROOT HOME --resolved-config-out TMP --resolved-config-only`
   — deliberately copy-free/hook-free/lock-free; failure only **warns**
   (`WARNING: resolved-config generation failed; subrepo AGENTS.md will be
   rendered without structured fields.`).
3. **Recipe-materialize fallback** (`L191-202`): when `--recipe-mcp` is absent it
   runs materialize and parses `RECIPE_MCP_TEMP:` from its stdout (`L201`,
   `grep '^RECIPE_MCP_TEMP:' | cut -d: -f2-`) after filtering that marker out of
   the failure output (`L198`). This path leaks its capture temp file when
   materialize fails (defect **D23**).
4. **Cache derivation**: `RESOLVED_SKILLS_DIR = project-cache.py ROOT path
   resolved-skills` (`L285`) and `MERGED_COMMANDS_DIR = project-cache.py ROOT
   path root` + `/merged-commands` (`L286`); then `flatten-resolved-skills.py`
   (`L287`) and `project-cache.py merge-commands` (`L288`) wrapped in `run_step`.
5. `ensure_target_workspace` (`L357-386`): same-root requires `AGENTS.md` else
   exit **1**; other targets get `mkdir -p ai-specs/`, the target
   `ai-specs/.gitignore`, `mirror_directory` (`rm -rf` + `cp -R`) of resolved
   skills and merged commands, then `brief-render-policy.py`-gated
   `agents-render.py`, or exit **1** when that policy is false and `AGENTS.md`
   is missing (`L382`).
6. Agent selection (`L389-401`): `[agents].enabled` via `toml-read.py` (`L389`);
   `--all` or explicit selectors override; empty target set triggers the **D24**
   path — `ensure_target_workspace` (`L407`) **then**
   `WARNING: no agents to sync…` and exit **0** (`L403-410`).
7. `MCP_COUNT` is computed inline in Python (manifest `[mcp.*]` + recipe-mcp JSON)
   purely for the banner (`L412-425`).
8. `sync_one_agent` (`L448-542`), one `run_step` per agent (`L544-546`):
   - unknown agent (fails `platform_get … native`) → `  ✗ unknown agent` and
     **return 0** (`L401-404`);
   - enabled-check → `  ! <agent> not in [agents].enabled — syncing anyway`;
   - instructions path (`CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`,
     `.omp/AGENTS.md`) → **relative** symlink to `AGENTS.md`;
   - skills dir → **absolute** symlink to the cache-backed resolved skills; an
     existing **non-symlink** is a hard refusal
     `    ✗ refuse to overwrite non-symlink: <path>` + rc 1; a real *directory*
     is `rm -rf`'d then relinked (`L405-432`);
   - MCP (`mcp-render.py TOML AGENT PATH KEY --recipe-mcp …`, `L486`) or
     `    ℹ mcp skipped (no [mcp.*] in manifest)`;
   - commands: `mkdir -p` (never `rm -rf` — the D3' fix), unlink-through
     symlinks/non-regular files, `cp` each managed `*.md`, then warn
     `    ! preserved non-managed file <dir>/<base> (move it to ai-specs/commands/…)`
     for anything unmanaged, and
     `    ✓ commands     <dir>/ (N file(s))` when N>0;
   - runtime hooks: only when `--resolved-hooks` was passed **and** the file
     exists → `hooks-render.py RESOLVED_HOOKS AGENT TARGET` (`L538`) then
     `    ✓ runtime hooks <target>`. Standalone runs render no hooks
     (defect **D22**).

### A1. `lib/_internal/recipe-materialize.py` (4265 lines) — the module that changed size

**This is the planning session's most important finding. See "Findings".**

| Aspect | Value (evidence) |
| --- | --- |
| Entry | `recipe-materialize.py <project_root> <ai_specs_home> [--recipe-mcp-out P] [--resolved-config-out P] [--resolved-hooks-out P] [--resolved-config-only] [--refresh-gates]` (`main()` `L4206-4265`) |
| Positional validation | exactly 2 non-flag args else usage on stderr + `return 2` (`L4236-4244`) |
| `--resolved-config-only` without `--resolved-config-out` | exit **2** (`L4249-4252`) |
| Exit codes | 0 ok; **1** any exception out of `materialize_recipes` via `fail()`+`return 1` (`L4255-4262`, `fail()` `L169-172`); **2** usage |
| Public entry points | `materialize_recipes()` `L3858-4142`, `build_resolved_config_only()` `L4143-4205`, `build_resolved_config()` `L3835-3844`, `resolve_bindings()` `L2578-2613`, `check_conflicts`/`check_capability_conflicts`/`check_tag_conflicts` `L491-521`/`L503-521`/`L635-647`, `merge_config()` `L2978-3005`, `clean_orphans()` `L3564-3655`, `write_tracker_witness()` `L2763-2829`, `execute_hooks()` `L3069-3130`, `build_recipe_mcp()` `L3179-3223`, `stamp_recipe_reconcile_defaults()` `L443-477` |
| stdout API | `  ▸ recipe <id>` per enabled recipe (`L4014`), `  ✓ wrote recipe MCP temp (N server(s))` (`L4111`), `RECIPE_MCP_TEMP:<path>` (`L4113`), `  ✓ wrote resolved-config (N recipe(s))` (`L4128`), `  ✓ wrote resolved-hooks (N hook(s))` (`L4138`), `  (no [recipes.*] enabled — skipping)` (`L3898`), plus per-artifact lines `    ✓ bundled skill <id>` `L956`, `    ✓ dep skill <id>` `L986`, `    ✓ command <id>` `L1026`, `    ✓ template <target>` `L1548`, `    · template skipped (exists) <target>` `L1543`, `    ✓ doc <target>` `L1632`, `    · doc skipped (exists) <target>` `L1627`, `    ✓ hook script <rel>` `L2298`, `    ✓ hook refreshed <rel>` `L2291`, `    · hook skipped (current) <rel>` `L2302`, `    ✓ hook refreshed (baseline matched) <rel>` `L2309`, `    · hook skipped (user-modified) <rel>` `L2316`, `    · hook skipped (no provenance) <rel>` `L2323`, `  ✓ removed orphaned …` `L3547-3619`, `  ✓ removed stale lock entries for recipe '<id>'` `L3524` |
| Stream helpers | `fail()` → `  ✗ msg` on stderr + `sys.exit(1)` `L169-172`; `warn()` → `  ! msg` on stderr `L174-177`; `info()` → `  ℹ msg` `L178-180` |
| JSON seam | the three `--*-out` files are the real contract, not stdout: `recipe-mcp` (recipe → server dict, consumed by `mcp-render.py --recipe-mcp`), `resolved-config` (`{recipes:…, bindings:…, enabled:…, topology:…}`, consumed by `agents-render.py --resolved-config`), `resolved-hooks` (`{enabled_agents:[…], hooks:[{recipe,id,event,matcher,blocking,script_path,env}]}`, consumed by `hooks-render.py`) |
| Env | `AI_SPECS_GATE_OFFLINE` `L2406`/`L4090`, `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES` `L3872`; `WORKTREE_GATE_BIN` indirectly through `gate_binary.resolve_verified_binary` |
| Side effects | `shutil.copytree` (bundled skill, `L913`), `shutil.copy2` (command/dep/doc, `L925`,`L935`, and doc `_python_doc_copy` `L928-937`), `dest.write_text` + `os.chmod 0o755` for gate hooks (`L1743-1744`, `L1753-1754`), `write_bytes` backup `L1712`, template/rendered bytes written via the actuator or `_python_materialize_template` (`L1430-1550`), witness written atomically `tempfile`+`os.replace` (`L2805`), marker `bootstrap-ready` (`L3115`), `shutil.rmtree` for orphans (`L3546-3558`), lock write through `lock.py` (already Go-owned) |
| Atomicity | mixed: `copytree`/`copy2`/`write_text` are **not** atomic (D13 family); witness uses mkstemp+replace; gate-hook refresh has a documented backup/rollback; there is **no** rollback across the whole step ("previous writes are not rolled back", `sync.sh:291`) |

**Strangler state — already Go-owned.** 22 `GO_*` constants (`L307-3657`) and 38
bridge-client units (~1178 lines) delegate these *decisions* to the separate
`worktree-gate` binary, each with a fail-open warning and a retained Python body:

| Bridge | Gate flag | Python fallback retained |
| --- | --- | --- |
| `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` `L307` | `--plan-reconcile-stamps` `L358-366` | `_stamp_recipe_reconcile_defaults_python` `L407-442` |
| `GO_TAG_CONFLICTS_BRIDGE_FALLBACK` `L543` | `--resolve-tag-conflicts` `L595` | `_python_check_tag_conflicts` `L648-672` |
| `GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK` `L673` | `--resolve-primitive-conflicts` | `_python_check_recipe_conflicts` `L754-765` |
| `GO_COPY_APPLY_BRIDGE_FALLBACK` `L788` | `--apply-copy` | `_python_bundled_skill_copy`/`_python_command_copy`/`_python_doc_copy` `L904-937` |
| `GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK` `L1041` | `--materialize-template` | `_python_materialize_template` `L1430-1550` |
| `GO_HOOK_GATE_BRIDGE_FALLBACK` `L1777` | `--materialize-hook` | `_python_materialize_hook_script` `L2233-2339` + `_fallback_materialize_and_reconcile` `L2001-2084` |
| `GO_BINDINGS_BRIDGE_FALLBACK` `L2340` + `GO_BINDING_ERROR_CODES` `L2348` | `--resolve-bindings` | `_python_resolve_bindings` `L2614-2667` |
| `GO_MERGE_CONFIG_BRIDGE_FALLBACK` `L2830` | `--plan-merge-config` | `_python_merge_config` `L3006-3068` |
| `GO_ORPHANS_BRIDGE_FALLBACK` `L3224` + `GO_ORPHANS_APPLY_FAIL_CLOSED` `L3383` | `--plan-orphans` / `--apply-orphans` | `_python_orphan_plan` `L3336-3382`, `_legacy_clean_orphans` `L3529-3563` |
| `GO_RESOLVED_CONFIG_BRIDGE_FALLBACK` `L3656` | `--plan-resolved-config` | `_python_build_resolved_config` `L3763-3834` |
| `GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK` (in `util.py:671`) | `--plan-classify` | `_python_classify_managed_override` `util.py:842` |
| `GO_LOCK_WRITE_BRIDGE_FALLBACK` (in `lock.py`) | `--write-lock` | lock writer body |
| `GO_BINDINGS_BRIDGE_*`, `--materialize-hook`, `--write-recipe-config`, … | see `…contract.md` §5 and `SHA256SUMS` history | — |

Fail-open postures differ per bridge and are **behaviour**: infrastructure
failure (no verified binary, crash, timeout, malformed/mismatched envelope) →
one warning + Python body; a *delivered refusal envelope* from the hook gate
fails **closed** (`RuntimeError`, no fallback — module docstring `L13-20`).

### A2. Renderers

| Module | Entry | Inputs | Outputs / semantics |
| --- | --- | --- | --- |
| `agents-render.py` (881) | `agents-render.py <toml_path> <output_path> [--preserve-if-runtime-brief] [--resolved-config P] [--adopt-brief]` (`L847-877`) | manifest; optional resolved-config JSON | writes `<output_path>` (`output_path.parent.mkdir(parents=True, exist_ok=True)` + `write_bytes`, `L834-836`). Marker `<!-- ai-specs:runtime-brief -->` `L64`. Write governance: `classify_brief` `L634-667`, `brief_ownership_state` `L668-678`, `brief_effective_state` `L697-712`, `_brief_decision` `L713-781`, `render` `L782-846`; states `missing`/`managed_stale` write, `marker`/`untracked`/`user_modified`/`undetermined` preserve; `--preserve-if-runtime-brief` is **deliberately inert** (design D5, docstring `L6-11`). Section renderers `L202-545`; brief fragments from recipes `L121-170`; config substitution `{{config.KEY}}` `L85-120`; env redaction `_redact_env_value` `L185-201` |
| `brief-render-policy.py` (97) | `brief-render-policy.py <toml_path> [--validate]` `L57-91` | manifest `[brief].render` | prints `true`/`false`, exit 0; non-boolean → default **enabled** (fail-safe); `--validate` exit 1 on invalid type. Read by `sync.sh:275`, `sync-agent.sh:371`, `init.sh:239`, `doctor.py:446`/`:574` |
| `hooks-render.py` (481) | `hooks-render.py <resolved_hooks_json> <agent> <project_root>` (`L465-477`), exit 2 on bad argv | resolved-hooks JSON only (no catalog) | per-agent writers `render_claude` `L136-169`, `render_cursor` `L170-225`, `render_opencode` `L226-304`, `render_pi` `L305-361`, `render_omp` `L362-418`; `EVENT_MAP` `L36-69`; `MANAGED_KEY = "_ai_specs_managed"` `L76`; JSON written `json.dumps(data, indent=2, sort_keys=True) + "\n"` `L111`; generated wrapper/plugin/extension files `write_text` `L199`,`L300`,`L357`,`L414`; emits `  ✓ runtime hooks …` upstream |
| `mcp-render.py` (315) | `mcp-render.py <toml_path> <agent> <target_path> <mcp_key> [--dry-run]` (`L258-311`) | manifest `[mcp.*]` + recipe-mcp JSON | owns the MCP key (replaces it entirely), preserves all other keys; missing file → created with just the key; JSON path `merge_into_json` `L174-201`, TOML path `merge_into_toml` `L202-243` via `_load_toml_write()` `L244-257`; env-var translation regexes `L54-63`, opencode translator `L97-148`, slim-write `L149-161`; `--dry-run` prints `--- <path> (dry-run) ---` + content; exit 1 if manifest missing, 0 with `info: no [mcp.*] entries — skipping <agent>` |

### A3. Skills, cache, contracts, conflicts

| Module | Entry | Notes |
| --- | --- | --- |
| `skill-resolution.py` (288) | `skill-resolution.py <project_root> [--json]` (`L265-284`) | three-source precedence: local `ai-specs/skills/**` (highest) `L52-62`, cache `.recipe/<id>/skills/**` `L63-87`, cache `.deps/<id>/skills/**` `L88-119`, plus a bundled tier `L120-132` (tier order documented `L11-16`); override/config lookup `L150-219`; `collect_skills` `L220-252`, `resolve_skill` `L253-264`. Called by `flatten-resolved-skills.py:21` and `rules-inventory.py:87` |
| `skill_contract.py` (423) | `skill_contract.py sync-metadata <skill_md_path>` (`L403-419`); exit 2 usage, 1 error JSON | shared frontmatter parse/normalize/validate/render: `split_frontmatter` `L30-43`, `parse_frontmatter` `L105-172`, `normalize_local_skill` `L241-298`, `from_local_skill` `L299-304`, `from_dep` `L305-349`, `validate_sync_metadata` `L350-362`, `render_skill_markdown` `L395-402`; `NAME_RE` `L14` (accepts only `[a-z0-9-]`, unlike `skills add` — defect **D19**). Called by `rules-inventory.py:86`, vendoring paths, `tests/test_skill_contract.py` |
| `recipe-conflicts.py` (242) | `recipe-conflicts.py <catalog_dir> <recipe_id>…` (`L219-238`), exit 2 usage, 1 conflicts (JSON on **stderr**) | library `ConflictRegistry` `L115-142`, `Conflict` `L50-57`, `TagConflict` `L58-77`, `check_tag_conflicts` `L78-114`, `check_recipe_conflicts` `L143-161`, `check_capability_conflicts` `L162-218`. Imported by `recipe-materialize.py:481` |
| `project-cache.py` (679) | `project-cache.py <root> path <kind>` `L641-659`, `… ensure-cache` `L637-639`, `… merge-commands <dest>` `L661-672`, exit 2 usage/unknown | **single cache authority** for the whole CLI. Cache root `$AI_SPECS_HOME/cache/projects/<sha256(realpath)[:12]>-<sanitized-basename>/` `L42-53` — FROZEN (`…contract.md:164-176`). Layout `meta.toml`, `.recipe/<id>/skills`, `.deps/<id>/skills`; tiers via `recipe_skills_root` `L87`, `deps_skills_root` `L91`, `bundled_skills_root` `L95`, `bundled_commands_root` `L100`, `inproject_deps_root` `L105`, `commands_dir` `L115`, `backups_root` `L119`, `gate_backup_path` `L129`, `resolved_skills_dir` `L145`. Leftover removal `remove_bundled_skill_leftovers` `L178-232`, `remove_bundled_command_leftovers` `L254-315`, `remove_recipe_command_leftovers` `L316-365`, legacy origin cleanup `remove_legacy_origin` `L481-563`; git-aware remediation printing `_tracked_bundled_leftovers` `L416-437` + `format_tracked_bundled_remediation` `L462-480`; `merge_commands` `L564-616` (`rmtree` dest + `copy2` per file). Callers: `vendor-skills.py:161`, `refresh-bundled.py:48`, `recipe-materialize.py:72`, `skill-resolution.py:38`, `doctor.py:300`,`:1418`, `lib/skills-list.sh:143` |
| `flatten-resolved-skills.py` (57) | `flatten-resolved-skills.py <project_root> <dest_dir>` `L30-52` | `rmtree(dest_dir)` then `copytree` per resolved skill (`L42-49`); prints `  ✓ flattened N skill(s) to <dest>` |
| `gitignore-render.py` (65) | `gitignore-render.py <toml_path> <output_path>` `L45-60` | writes `ai-specs/.gitignore` from `[[deps]]`; `HEADER`/`FOOTER` `L21-29`; prints `  ✓ wrote <path> (N dep(s))`. **Shared with `init.sh:295`** (and `sync-agent.sh:26`) |
| `gitignore-root-refresh.py` (78) | `gitignore-root-refresh.py <project_root> <template_path>` `L59-73` | manages only the block between `MARKER_BEGIN` `L13` and `# --- end ai-specs ---` `L16`; trailing-byte normalization before append (ordering guarantee #6); prints `  ✓ <created\|refreshed> root .gitignore (agent block)`; exit 1 when the template is missing. **Not shared:** `init.sh:262-292` implements the same managed block **inline** (`append_block`/`strip_block` with `awk`, `L274-279`) instead of calling this module — two implementations of one contract, which the port must reconcile (card 09 owns `init`) |

### A4. Adjacent modules invoked by the spine but owned elsewhere

| Module | LOC | Called from | Owning card / Go status |
| --- | --- | --- | --- |
| `target-resolve.py` | 223 | `sync.sh:177`, `sync-agent.sh` standalone block `L112` | **already ported** → `internal/target` |
| `toml-read.py` | 223 | `sync-agent.sh:389` | **already ported** → `internal/toml` |
| `lock.py` | 490 | materialize, `vendor-skills`, `refresh-bundled` | **already ported** → `internal/lock`, and write authority is Go via `--write-lock` |
| `recipe_schema.py` | 864 | materialize, conflicts, recipe verbs | **already ported** → `internal/schema` |
| `cli_version.py` | 302 | `sync.sh:192`,`:296`, `doctor.py:204`, `version.sh:6`, `refresh-bundled.sh:87` | not ported |
| `refresh-bundled.py` | — | `sync.sh:220` | Go **stub** already exists (`internal/cli/refresh_bundled.go`, card 06) |
| `vendor-skills.py` | 236 | `sync.sh:222` | not ported — dep layer (card 11) |
| `env_scaffold.py` | 667 | `sync.sh:224` | not ported — init/env (card 09) |

### A5. Cross-module invocation matrix (caller → callee)

```
bin/ai-specs ──sync.sh──┬─ target-resolve.py        [Go ✔ internal/target]
                        ├─ cli_version.py           [shared: version.sh, refresh-bundled.sh, doctor.py]
                        ├─ gitignore-render.py      [shared: init.sh, sync-agent.sh]
                        ├─ gitignore-root-refresh.py[dup: init.sh:262-292 inline]
                        ├─ refresh-bundled.py       [Go stub ✔]
                        ├─ vendor-skills.py ────────┬─ project-cache.py [shared: doctor.py, skills-list.sh]
                        │                           └─ lock.py          [Go ✔]
                        ├─ env_scaffold.py
                        ├─ recipe-materialize.py ───┬─ project-cache.py
                        │                           ├─ recipe-conflicts.py
                        │                           ├─ recipe_schema.py    [Go ✔]
                        │                           ├─ cli_version.py
                        │                           ├─ gate_binary.py ────► worktree-gate binary (separate module)
                        │                           └─ lock.py / toml_write.py
                        ├─ brief-render-policy.py   [shared: sync-agent.sh:32, init.sh:134, doctor.py:446/574]
                        ├─ agents-render.py         [shared: sync-agent.sh:31, init.sh:133, doctor.py:480]
                        └─ sync-agent.sh ───────────┬─ platform.sh
                                                    ├─ toml-read.py        [Go ✔]
                                                    ├─ project-cache.py
                                                    ├─ flatten-resolved-skills.py ── skill-resolution.py ── project-cache.py
                                                    ├─ recipe-materialize.py (standalone/-only)
                                                    ├─ gitignore-render.py
                                                    ├─ brief-render-policy.py
                                                    ├─ agents-render.py
                                                    ├─ mcp-render.py ──── toml_write.py / toml-read.py
                                                    └─ hooks-render.py
```

---

## (b) Dependency graph against the ported Go packages, and the Go-side gap

### Already available for reuse (do not duplicate)

| Go package | Replaces | Evidence |
| --- | --- | --- |
| `internal/toml` | `toml-read.py`, TOML parsing for manifest/brief/mcp | `internal/toml/toml.go`, `value_parse.go` |
| `internal/toml` + `internal/config` writes | `toml_write.py`, `recipe-config-write` | `internal/config/writes.go`, ADR 0002 |
| `internal/schema` | `recipe_schema.py` (Recipe model, validation error strings FROZEN) | `internal/schema/recipe.go`, `differential_test.go` uses `testdata/driver.py` |
| `internal/target` | `target-resolve.py` (Python `.resolve()`/`realpath` semantics) | `internal/target/resolve.go`, `topology.go`, `repr_test.go` |
| `internal/lock` | `lock.py` read/model; write authority is Go `--write-lock` | `internal/lock/lock.go`, `mutation.go` |
| `internal/config` | global config + repr helpers (`repr_test.go`) | `internal/config/json.go`, `repr_test.go` |
| `internal/home` | `AI_SPECS_HOME` resolution | `internal/home/home.go` |
| root `assets` (go:embed) | catalog/templates/bundled-* reads | `assets.go` (carve-out: the nested gate module is **not** embedded, `assets.go:16-22`; card 11 = user-vendored deps, `assets.go:24-25`) |
| `internal/cli` | verb dispatch + shim (`runShim`) | `internal/cli/cli.go`, `shim.go` |

### Missing in Go (the real work)

1. **No `internal/sync` orchestrator** — ordering, temp capture, verbosity filter,
   errexit discipline, header/footer, fan-out, exit-code collapse.
2. **No Go agent matrix** — `platform.sh`'s agent→paths/keys table has no Go
   equivalent (only `internal/target` knows *targets*, not *agents*).
3. **No Go project cache** — `project-cache.py` (cache key FROZEN) including
   leftover removal, tracked-file remediation text, and `merge_commands`.
4. **No Go renderers** — AGENTS.md (+ brief governance), MCP (JSON+TOML merge),
   hooks (5 agent formats), both `.gitignore` renderers.
5. **No Go skill resolution / skill contract / conflict library.**
6. **No Go `pyrepr`/path-semantics helper package shared by the above.** Repr and
   `realpath` logic exists in `internal/config` and `internal/target`, but
   neither is importable as a general helper without duplication. `UNVERIFIED:`
   whether the epic intends a shared `internal/pyrepr`; today the pattern is
   per-package helpers plus a Python oracle.
7. **No Go gate-binary acquisition** — `gate_binary.py` (trust root
   `catalog/recipes/worktree-flow/bin/SHA256SUMS`, version-keyed cache, atomic
   install, digest-before-execute) is Python-only. Any Go sync that needs gate
   authority must either acquire the binary itself (port) or avoid needing it.
8. **A cross-module blocker: the gate is `package main`.** `worktree-gate`
   (`catalog/recipes/worktree-flow/gate/**`, 22 891 lines incl. tests, own
   `go.mod` `ai-specs.dev/worktree-gate`, go1.22) declares `package main` for
   every file except the importable `gate/ledger` subpackage. The root module
   therefore **cannot import** the gate's decision code (tag/primitive
   conflicts, classify, template actuator, hook gate, copy-apply, lock write,
   resolved config, orphans, merge config, bindings) without refactoring the
   gate into a library package. This is the single largest architectural
   decision in this card — see (c) and (g). **Resolution: slice SX0** removes
   this blocker before S10–S13 by extracting the S10–S13 decision/actuator
   logic into an importable `shared` package inside the gate module (human
   decision 2026-10-03, see the Q2 resolution in section (g)).

---

## (c) Strangler mechanics

### What already exists (the pattern to follow)

Two mechanisms are in production:

1. **Verb-level shim** (`internal/cli`): the Go binary routes an unported verb to
   `bash $AI_SPECS_HOME/lib/<script> "$@"`, pinning `AI_SPECS_HOME` and cleaning
   stale `AI_SPECS_INVOKED_AS`, propagating the child rc verbatim (128+signal on
   signal death) and passing real fds so TTY detection still works
   (`shim.go:20-75`).
2. **Decision-level bridge** (12+ slices): Python keeps orchestration and calls
   the `worktree-gate` Go binary over a JSON envelope for one *decision*, with a
   `GO_*_BRIDGE_FALLBACK` warning and a retained Python body (A1 table).

The differential harness (`tests/parity/parity.py`) compares the **whole CLI**:
both legs run the same fixture in isolated `HOME`/`TMPDIR`/`AI_SPECS_HOME`, and
it diffs per-step exit code, normalized stdout/stderr, and the full project tree
(paths, modes, symlink targets, sha256 of UTF-8 contents) — 8 fixtures
(`fresh-init`, `idempotent-resync`, `all-recipes-enabled`, `multi-agent-fanout`,
`dirty-conflicted-project`, `missing-optional-deps`, `surface-verbs`,
`recipe-surface`), exit 1 on any delta, exit 2 if the Go build is unavailable
(fail-loudly), `--self-test` for legacy-vs-legacy. Normalizations N1–N6 are
documented and justified inline.

### Recommended design: forward strangler in the root binary, per-step opt-in

- `cli.go` gains a native `sync`/`sync-agent` route. `internal/sync` reimplements
  the spine (ordering, capture, verbosity, errexit, header/footer, fan-out) and,
  for every step not yet ported, **execs the current Python module exactly as
  `sync.sh` does today** (same argv, same cwd, same env, same stream capture).
- Each ported step is selected by a temporary env flag
  `GO_SYNC_STEP_<NAME>=go|python`, default `python`, mirroring the existing
  `GO_*_BRIDGE_*` convention. At cutover every flag disappears and the Python
  module is deleted.
- Why forward and not "reverse bridge" (legacy Bash calls Go): the epic's end
  state is one binary owning the pipeline. A reverse bridge would keep
  `sync.sh` as the orchestrator forever and would require the *Python* step's
  fallback to remain, growing the duplicate-grader problem. The forward design
  makes the spine the first thing proven, which is also the biggest risk.
- Why a *flag* rather than "just port it": the flag makes each step
  independently measurable in isolation (parity matrix), and gives a one-line
  rollback while the Python module still exists.

### How each slice is proven (this is the load-bearing part)

Three gates, in increasing cost:

1. **Whole-CLI differential** (`tests/parity/run.py`) — zero deltas on the corpus,
   run twice: with `GO_SYNC_STEP_*=python` (proves the new spine is
   byte-identical to Bash) and with `GO_SYNC_STEP_*=go` for the ported steps
   (proves the Go step is byte-identical to the Python step it replaced).
2. **Module differential in Go** — for every non-verb module (no dispatcher
   surface of its own: `project-cache`, `skill-resolution`, `skill_contract`,
   `recipe-conflicts`, both gitignore renderers), follow the established pattern:
   a Go test that shells out to `python3` and requires byte equality, using
   either a `testdata/<name>_ref.py` reference driver
   (`internal/config/testdata/{recipe_remove_ref,skills_add_ref,skills_remove_ref}.py`,
   `internal/lock/testdata/write_lock_ref.py`) or a canonical JSON projection
   driver (`internal/schema/testdata/driver.py`). This is what makes the
   `_python_*` fallback bodies deletable without a third implementation.
3. **Converted black-box suite** — the existing ~9 300 lines of black-box sync
   tests already drive `bin/ai-specs` through its process boundary and survive
   the port unchanged; they are the deep net, and each slice must run them.

### The parity harness blind spot (must be fixed before it can gate the port)

`tests/_blackbox.py::isolated_home` builds an install root with an **empty
`cache/`** and no gate binary, and `tests/run.sh:9` + `parity.py` `BASE_ENV` set
`AI_SPECS_GATE_OFFLINE=1`. `gate_binary.resolve_verified_binary` therefore
returns `None` in **both** legs (`gate_binary.py:206-262`), so
`all-recipes-enabled` exercises the **Python fallback** of all 22 bridges, never
the Go authority. Consequences:

- the corpus as-is cannot distinguish "Go sync calls gate authority" from
  "Go sync degrades to a fallback";
- once the Go step stops shelling out to Python, the two legs may silently take
  different paths and the zero-delta result proves nothing about the path that
  matters in production.

Fix: make the **mode part of the gate** — run the corpus twice, `gate-absent`
and `gate-present` (pin `WORKTREE_GATE_BIN` to a locally built gate for both
legs, so legacy uses Go authority through its bridges and Go uses its own), and
record both modes in the report. The gate build must not be committed.

---

## (d) Chained-PR slicing

Budget: the card caps a slice at ~700 changed lines including tests. Measured
cost model from the merged cards: Go production ≈ 0.7–1.1× the Python lines it
replaces; Go tests ≈ 0.6× production (differential + table tests); deleted
Python counts toward the diff. That yields **≈200–280 lines of Python per
700-line slice**.

| # | Slice | Python in scope | Port shape | Gate | Est. diff | Risk |
| --- | --- | --- | --- | --- | --- | --- |
| **S1** | `internal/sync` spine + capture/verbosity/errexit helpers + native `sync` route that still execs every Python step; re-point the three script-coupled test files to the CLI | `sync.sh` (299) | forward spine, steps unchanged | parity × 2 modes, all fixtures, `GO_SYNC_STEP_*=python`; re-pointed `test_sync_output_verbosity.py`/`test_sync_run_step_errexit.py`/`test_sync_recipe_capture.py` green against the CLI | ~700 | **high** — the verbosity/errexit/temp contract is FROZEN and easy to get subtly wrong |
| **S2** | `.gitignore` renderers (both) | `gitignore-render.py` (65) + `gitignore-root-refresh.py` (78) | byte port; keep `gitignore-render.py` until card 09/`sync-agent` switch, and reconcile the inline duplicate in `init.sh:262-292` with card 09 (F7) | parity (any fixture writing `.gitignore`) + new `deps-gitignore` fixture; module differential vs Python | ~450 | low |
| **S3** | AGENTS.md + brief policy | `agents-render.py` (881) + `brief-render-policy.py` (97) | byte port of the governance state machine | parity + **new** fixtures: `adopt-brief`, `brief-render-false`, `marker-preserved`, `user-modified-preserved`; existing 1128-line black-box fragments suite | ~700 (tight; split S3a sections / S3b governance if it overruns) | **high** — write governance is the highest-consequence silent-drift area |
| **S4** | MCP rendering | `mcp-render.py` (315) | byte port incl. JSON key order/indent + TOML merge | parity `multi-agent-fanout` extended to all 8 selectors; new `mcp-per-agent` fixture | ~500 | medium |
| **S5** | Runtime hooks rendering | `hooks-render.py` (481) | byte port of 5 agent formats | parity + new `hooks-five-runtimes` fixture; existing 552-line black-box suite | ~700 | medium — generated TS/JSON must be byte-equal |
| **S6** | `internal/projectcache` | `project-cache.py` (679) | byte port; **keep `project-cache.py` alive** for `doctor`/`vendor-skills`/`skills-list` until their cards land | Go module differential vs `python3 project-cache.py …` (kind-by-kind) + new `cache-layout` fixture | ~700 | **high** — cache key is FROZEN; blast radius beyond this card |
| **S7** | Skill resolution + flatten | `skill-resolution.py` (288) + `flatten-resolved-skills.py` (57) | byte port | Go module differential + parity (`multi-agent-fanout`) | ~550 | medium |
| **S8** | Skill contract | `skill_contract.py` (423) | byte port (frontmatter parser is a hand-rolled YAML subset) | Go module differential vs `sync-metadata` CLI + `testdata/driver.py` style projection | ~650 | medium — parser edge cases (block scalars, inline lists) |
| **S9** | Conflict library | `recipe-conflicts.py` (242) | byte port + retire the primitive/tag bridge call sites | Go module differential + parity with a conflicting recipe pair (`tests/fixtures/recipes/test-conflict-{a,b}`) | ~450 | medium |
| **S10** | Materialize: orphans + lock prune | `L3224-3655` (~430) | port the *plan/apply* seam + decide the fallback duplicate | parity (`dirty-conflicted-project`) + new `orphan-cleanup` fixture | ~700 | **high** — deletions, no rollback |
| **S11** | Materialize: conflicts + reconcile stamps | `L307-765` (~460) | retire bridge, Go-native fallback decision | Go differential vs Python fallback bodies + parity `all-recipes-enabled` | ~700 | medium |
| **S12** | Materialize: templates + docs + gate hooks | `L1041-1776` (~700) | retire actuator bridge | parity + new `template-refresh-gates` fixture (also covers `--refresh-gates`) | ~750 (split if needed) | **high** — gate-hook provenance/backup/rollback |
| **S13** | Materialize: bindings + merge-config + resolved-config/hooks + recipe-mcp emission | `L1777-3223` (~1 400) | retire 4 bridges | Go differential + parity | 2–3 PRs | **high** |
| **S14** | Materialize: main orchestration | `L3763-4265` (~500) | orchestration + argument parsing | parity `all-recipes-enabled` in both gate modes | ~700 | **high** |
| **S15** | `sync-agent` fan-out native | `sync-agent.sh` (551) | forward spine for the second verb; retire the `bash sync-agent.sh` exec | parity `multi-agent-fanout` + new `sync-agent-standalone` + nested masking | ~700 | **high** — symlink kinds, D3'/D22/D24 semantics |
| **S16** | Cleanup + docs | delete dead Python + convert coupled tests + amend `…contract.md` for the retired stdout coupling | — | full `./tests/validate.sh`, parity × 2 modes | ~700 | medium |

Count: **16 slices** (S1–S16), i.e. ~11 000–14 000 changed lines. This does not
fit "one card with a few PRs"; see Open Question **Q1**.

Slicing rules that must hold for every slice:

- Never split a caller from the callee contract its evidence depends on: a slice
  that ports a step must also port (or keep) the artifact contract its parity
  fixture observes.
- Shared modules (`project-cache`, `gitignore-render`, `cli_version`,
  `skill_contract`, `skill-resolution`) may not be deleted while a caller owned by
  another card still invokes them; those slices add Go and keep Python, and the
  deletion lands in S16 or in the owning card.
- Every slice's base ref is the previous slice's merge commit (native review
  discipline: last reviewed boundary).
- No slice may change the gate module unless it also regenerates and verifies
  `catalog/recipes/worktree-flow/bin/SHA256SUMS`
  (`scripts/build-gate.sh` + `scripts/verify-gate-sums.sh`).

---

## (e) Risk register

| # | Risk | Concrete mitigation (test or fixture) |
| --- | --- | --- |
| R1 | **Silent artifact drift** in `AGENTS.md`, `ai-specs/.gitignore`, root `.gitignore` block, MCP configs, generated hook files | Whole-tree sha256 comparison per step in the parity harness (`snapshot_tree`) + new per-artifact fixtures (S2–S5) + the 9 300-line black-box suite |
| R2 | **Write-governance regression**: wrong state writes (or fails to write) `AGENTS.md`; `--adopt-brief`/`--refresh-gates` lose meaning | S3 fixtures for all six states + `adopt-brief` + `refresh-gates`; `--preserve-if-runtime-brief` must stay **inert** (docstring `L6-11`); `tests/test_agents_md_render_opt_out.py`, `tests/test_brief_render_policy.py` |
| R3 | **errexit / `run_step` / capture-block semantics** drift (SIGPIPE, ENOSPC, `mktemp` failure, rc replacement, temp-file leaks, `exit $RECIPE_RC`) | S1 re-points `tests/test_sync_run_step_errexit.py` (323 lines) and `tests/test_sync_recipe_capture.py` (225) from `bash lib/sync.sh` to the CLI boundary, and converts `tests/test_sync_output_verbosity.py` (1 277) the same way (see F4); add a fault-injection fixture (unwritable `TMPDIR`, closed stdout) comparing tree + rc + stream classification in both legs |
| R4 | **Five-runtime fan-out** (claude/cursor/opencode/pi/omp; `platform.sh` also serves codex/copilot/gemini) diverges in path, key, symlink kind or hook format | Extend `multi-agent-fanout` to every selector; new `fanout-matrix` fixture asserting instructions symlink is **relative**, skills symlink is **absolute**, non-symlink occupancy is a hard refusal, and `.claude/commands/*` is never `rm -rf`'d |
| R5 | **Idempotency loss** (second sync mutates the tree) | `idempotent-resync` already diffs the tree after each step; extend to three syncs with all recipes + all agents, and to `sync` then `sync-agent --all` |
| R6 | **Inter-module stdout coupling** (`▸ recipe`, `RECIPE_MCP_TEMP:`) breaks while both implementations coexist | Freeze both markers in the corpus; add an explicit assertion fixture for `  syncing recipes → a, b`; the collapse into a return value is a deliberate TOLERANT change that requires a `…contract.md` amendment (S16) |
| R7 | **Parity blind spot**: all bridges take the Python fallback in both legs, so the corpus cannot see the Go-authority path | Two-mode gate (`gate-absent` / `gate-present` with pinned `WORKTREE_GATE_BIN`); make the mode part of the reported result |
| R8 | **Duplicate grader**: porting the 768 lines of `_python_*` fallback bodies into Go creates a second grader beside the gate's | Decide Q2/Q3 first; if the fallback is ported, gate it with a Go differential test whose oracle is the still-present Python body, then delete the Python |
| R9 | **Shared-module blast radius**: `project-cache` (doctor, vendor-skills, skills-list, refresh-bundled), `gitignore-render` (init), `cli_version` (version, refresh-bundled, doctor), `skill_contract`/`skill-resolution` (rules-audit, vendoring) | Ownership table in §A4 and the "never delete while a foreign caller exists" rule in (d); S6/S7/S8 add Go without removing Python |
| R10 | **Gate/trust-root churn**: any gate change requires regenerating `SHA256SUMS` for four architectures with go1.24.13 or the release CI fails | Slices that touch the gate must run `scripts/build-gate.sh` then `scripts/verify-gate-sums.sh`; the toolchain is pinned repo-wide (`SHA256SUMS` header) |
| R11 | **Python path/repr semantics** (`Path.resolve`, `realpath`, `str()`/`repr()`, dict ordering, JSON separators) | Reuse `internal/target` + `internal/config` semantics (ADR 0002, `resolvePy`); every module differential uses Python as the oracle; `internal/*/repr_test.go` already pins the repr rules |
| R12 | **Partial-write behavior** per step (no rollback; "previous writes are not rolled back") | New fault-injection fixtures that fail mid-step (missing dep repo, conflict pair, non-symlink occupancy, unwritable `TMPDIR`) and compare both the tree and the exit code in both legs |
| R13 | **`recorded defect` temptation**: D3, D4, D22, D23, D24 and the D13 non-atomic writes sit directly on the sync path and "look wrong" | Port them as observed; a delta here is a parity failure (`…contract.md:5-11`, defect cards are separate). `--refresh-gates` must not silently become the default |
| R14 | **Card-size mis-estimate** (card says ~5 500 LOC, actual sync surface is 8 424) | Escalated as finding F1 + Open Question Q1 before implementation starts |

---

## (f) Test strategy

### Layer 1 — differential oracles against Python (primary)

- **Whole-CLI**: `tests/parity/run.py`, wired in `tests/run.sh:19-20`; exit 1 on
  any delta, exit 2 on a missing/failed Go build (never a silent shim
  fallback); `--self-test` remains the explicit legacy-vs-legacy mode used by
  `tests/test_parity_harness.py`.
- **Module-level** for modules with no verb surface: Go tests that shell out to
  `python3` and require byte equality, using `testdata/*_ref.py` reference
  drivers (pattern: `internal/config/testdata/recipe_remove_ref.py`,
  `internal/lock/testdata/write_lock_ref.py`) or a canonical projection driver
  (pattern: `internal/schema/testdata/driver.py`).
- **Fallback-body oracle**: for each `_python_*` fallback scheduled for deletion,
  a Go differential test with the Python body as oracle, run before deletion.

### Layer 2 — fixture corpus additions to `tests/parity/parity.py`

Add fixtures (each one new `Fixture(...)` entry in `CORPUS`):

| New fixture | Freezes |
| --- | --- |
| `sync-verbose` | the **verbose** branch of `print_step_output` (no existing fixture passes `-v`) |
| `sync-adopt-brief` | `--adopt-brief` write governance |
| `sync-refresh-gates` | `--refresh-gates` + the immutable pre-refresh backup |
| `sync-ignore-cli-version` | the `--ignore-cli-version` warning path |
| `sync-brief-render-false` | the `ℹ skipped AGENTS.md (brief.render = false)` path |
| `deps-gitignore` | `[[deps]]`-driven `ai-specs/.gitignore` + root block |
| `mcp-per-agent` | every `mcp_key`/path pair incl. opencode `mcp` and codex `mcp_servers` |
| `hooks-five-runtimes` | the 5 hook formats byte-for-byte |
| `fanout-matrix` | all 8 selectors, symlink kinds, non-symlink refusal, D3' preservation |
| `conflict-pair` | tag/capability/primitive conflict detection using `tests/fixtures/recipes/test-{conflict,cmd-conflict,mcp-conflict}-{a,b}` |
| `override-ownership` | managed-override classification + ownership preservation (replaces the Python-coupled `tests/test_override_ownership.py`) |
| `orphan-cleanup` | orphan plan/apply + lock pruning |
| `partial-failure` | stop-on-first-failure tree + rc, no rollback |
| `tmpdir-unwritable` | the `mktemp` degradation warning path |
| `sync-agent-standalone` | standalone `sync-agent` (no `--source-root/--target`) |
| `nested-fanout-suppression` | `AI_SPECS_SYNC_NESTED=1` banner/footer suppression |

Every new fixture is only added together with the slice that owns the behavior,
and each runs in **both** gate modes (R7).

### Layer 3 — idempotency

`idempotent-resync` generalized: three consecutive `sync` runs plus a
`sync-agent --all` tail, with all recipes enabled and all five runtimes enabled;
assert the tree is unchanged between runs 2 and 3 and that the only differing
normalized bytes are the N5 timestamp rules.

### Layer 4 — partial failure

Fault injection at the step boundary (unwritable `TMPDIR`, missing dep repo,
conflicting recipe pair, non-symlink occupancy on a skills dir, gate absent),
asserting: exit code, full unfiltered output on stderr/stdout split, absence of
temp-file leaks in `TMPDIR`, and that earlier writes are **not** rolled back.

### Layer 5 — per-step matrix

`GO_SYNC_STEP_<NAME>` = `python|go` for each ported step, run against the whole
corpus: this is the per-slice acceptance evidence, and it is removed at cutover.

### Layer 6 — Python-coupled tests to convert (27% of the sync corpus)

| Test (lines) | Why coupled | Treatment |
| --- | --- | --- |
| `test_override_ownership.py` (494) | `spec_from_file_location` on `util.py`/materialize | replace with `override-ownership` parity fixture (category 1/2) |
| `test_project_cache.py` (280) | imports `project-cache.py` | Go module differential + keep for as long as Python stays |
| `test_recipe_conflicts.py` (221) | imports `recipe-conflicts.py` | Go module differential |
| `test_skill_contract.py` (188) | imports `skill_contract.py` | Go module differential + `sync-metadata` CLI |
| `test_materialize_bridge.py` (809), `test_recipe_conflict_bridge.py` (381) | pin the **bridges** | delete with the bridge they pin; the parity fixture replaces the coverage |
| `test_sync_output_verbosity.py` (1277), `test_sync_run_step_errexit.py` (323), `test_sync_recipe_capture.py` (225) | run `bash lib/sync.sh` / `lib/sync-agent.sh` **directly** — not the CLI | **re-point to the CLI boundary in S1** (replace `SYNC_SH`/`SYNC_AGENT_SH` with the `bin/ai-specs sync` / `sync-agent` invocation); otherwise the errexit and verbosity contract loses its gate the moment the Go spine lands |
| `test_docs_drift_fixes.py`, `test_false_success_fixes.py` | verify D-fixes | keep (black-box), re-run per slice |

### Layer 7 — Go-side hygiene

`gofmt -l .`, `go vet ./...`, `go test ./...` (root) and
`go -C catalog/recipes/worktree-flow/gate test ./...` per slice, plus
`./tests/validate.sh` before the PR.

---

## (g) Open questions for the human

Each with a recommendation. These block implementation, not this plan.

**Q1 — Card 07 is ~70 % bigger than the card estimates and needs ~16 chained
PRs. How should it be tracked?**
Evidence: the card lists ~5 500 LOC; the actual sync surface is **8 424 lines**
(`recipe-materialize.py` 4265 not 1410, `sync-agent.sh` 551 not 525,
`sync.sh` 299 not 292), and the 700-line review budget caps a slice at
~200–280 Python lines. Recommendation: **split card 07 into sub-cards by module
boundary** (spine S1; renderers S2–S5; foundation S6–S9; materialize S10–S14;
fan-out S15; cleanup S16), each with its own PR, keeping this document as the
shared plan. If sub-cards are not acceptable, the alternative is to accept a
~16-PR chain under one card with an explicit "epic-scale card" flag.

**Q2 — Where does the ported sync authority live relative to the gate module?**
The gate is `package main` in a nested module with its own release and SHA-256
trust root, so the root single binary cannot import it. Three options:
(a) refactor the gate's decision packages into an importable library and have
both the root binary and the gate `main` use it — one grader, one core, but a
structural change to a released artifact plus `SHA256SUMS` regeneration;
(b) keep delegating: the Go sync execs the gate binary with identical resolution
and fail-open semantics, and the CLI grows a Go port of `gate_binary.py`;
(c) let the root binary own those decisions outright and retire them from the
gate (risk: hook paths that run without the CLI lose the grader).
Recommendation at planning time was **(b) now, (a) as a card-16/structural
decision**. **RESOLVED 2026-10-03, human decision (supersedes the
recommendation): option (a) is pulled forward as slice SX0.** One authoritative
importable Go package inside `ai-specs.dev/worktree-gate`, sibling of `ledger`;
the root binary invokes it in-process; `gate_binary.py` is NOT ported; the gate
keeps a thin `main` with existing flags, dispatch and byte behavior. Root module
imports via `require ai-specs.dev/worktree-gate v0.0.0` +
`replace => ./catalog/recipes/worktree-flow/gate`. Constraints: extract only
the decision/actuator logic required by S10–S13; gate stays standalone (zero
external dependencies, go1.22 floor); checksum regeneration is local
(`scripts/build-gate.sh` + committed `SHA256SUMS` regen +
`scripts/verify-gate-sums.sh`), not a release. Tracked as [Go 07.SX0]
(card `6ac06c7a0d9b0e444869419b`, https://trello.com/c/aIusguGP; feature doc
`odd/tasks/go-07-sx0.md`).

**Q3 — What happens to the 768 lines of duplicated Python fallback authority
(`_python_*`)?**
Epic contract says "one authoritative grader per behavior", but the current
design deliberately keeps a Python duplicate for fail-open. Options: delete them
and accept that Go sync always requires gate authority; or port them to Go as
the fail-open path (preserving current behavior, adding a second grader in the
single binary). Recommendation: **keep the fail-open behavior, but implement it
as a Go path gated by a differential test against the Python body before
deletion** — behavior preserved, oracle documented, no second invention.

**Q4 — Should the parity harness gain the `gate-present` mode in this card?**
Recommendation: **yes**, as part of S1, because without it the corpus cannot see
the Go-authority path at all (R7). It is a test-harness change, not a behavior
change, and it makes every later slice's evidence meaningful.

**Q5 — Ownership of the shared modules.** `project-cache.py` (doctor, vendor,
skills-list), `gitignore-render.py` (init, sync-agent), `cli_version.py` (version,
refresh-bundled, doctor), `skill_contract.py`/`skill-resolution.py`
(rules-audit, vendoring) are invoked by cards 08/09/11/12 and by Bash verbs.
Recommendation: card 07 **adds** Go implementations and keeps the Python modules
until the owning card switches its callers; S16 deletes only what is provably
dead. The orchestrator must serialize the `project-cache.py` and
`gitignore-render.py` file touches across cards.

**Q6 — Is `--refresh-gates` in scope?** The card says "preserve `--adopt-brief`
and every existing flag", and the acceptance list does not name `--refresh-gates`.
It sits on the highest-risk path (gate-hook backup/rollback).
Recommendation: **in scope but in its own slice** (S12) with a dedicated fixture;
it is a flag of `sync` and cannot be dropped.

**Q7 — May the port collapse the two stdout-grep couplings (`▸ recipe`,
`RECIPE_MCP_TEMP:`)?** They are FROZEN today (`…contract.md:200-212`) and are the
main hazard while both implementations coexist. Recommendation: **collapse them
in S16 only**, together with a `…contract.md` amendment recording the deliberate
TOLERANT change; not before, because Bash `sync.sh` still parses them.

**Q8 — One `.worktrees/` branch per slice, or one branch with 16 commits?**
Recommendation: **one branch (`change/go-07-plan-sync` is planning-only; the
implementation should get its own branch) with one commit per slice, reviewed
against the previous commitment as base ref**, since the native-review budget is
per candidate, not per branch. This also keeps the epic's PR-per-card cadence.

---

## Findings

**F1 — The card's size estimate is stale by ~70 %.** Measured here, the card's
ten listed modules total **8 424 lines** vs the card's "~5 500":
`recipe-materialize.py` 4265 vs "1410" (+203 %), `sync-agent.sh` 551 vs 525,
`sync.sh` 299 vs 292, `agents-render.py` 881 vs 886. Adding
`brief-render-policy.py` (97) as an eleventh in-scope module gives 8 521. The
likely cause is the 12+ `GO_*_BRIDGE_*` strangler slices merged after the card
was written.

**F2 — The largest module is already ~46 % Go-delegating.** In
`recipe-materialize.py`, 38 bridge-client units (~1178 lines, 22 `GO_*`
constants) delegate decisions to `worktree-gate`, and 17 `_python_*` units
(768 lines) are duplicate fail-open authority. The genuine porting surface is
~2 300 lines, not 4 265 — but the port must also resolve the duplicate-grader
question (Q3), which the card does not anticipate.

**F3 — The parity harness cannot currently see the Go-authority path.** With an
empty `cache/` and `AI_SPECS_GATE_OFFLINE=1`, `resolve_verified_binary` returns
`None` in both legs, so every bridge degrades to Python. `all-recipes-enabled`
passing today therefore proves the *fallback* path, not the production path.
Without a fix, the port's central acceptance criterion ("zero deltas incl.
all-recipes-enabled") is measured on the wrong code path.

**F4 — The test net is much stronger than the card assumes, with one trap.**
~7,600 lines of the sync corpus already drive `bin/ai-specs` through its process
boundary and survive the port unchanged (`test_sync_pipeline.py` 3 529,
`test_recipe_materialize.py` 1 500, `test_agents_render_brief_fragments.py`
1 128, `test_hooks_render.py` 552, `test_recipe_entrypoint_parity.py` 305,
`test_agents_md_render_opt_out.py` 231, `test_sync_env_scaffold.py` 149,
`test_brief_render_policy.py` 242, …). Only ~2 400 lines are Python-coupled, and
809+381 of those pin bridges the port deletes. **The trap:**
`test_sync_output_verbosity.py` (1 277), `test_sync_run_step_errexit.py` (323)
and `test_sync_recipe_capture.py` (225) do **not** use the CLI boundary — they
`subprocess.run(["bash", ROOT/"lib"/"sync.sh", …])` (`test_sync_output_verbosity.py:22-23`,
`test_sync_run_step_errexit.py:26-32`, `test_sync_recipe_capture.py:33`). Once S1
makes `bin/ai-specs sync` native they would test only the orphaned Bash script and
silently stop gating the port, so S1 must re-point them to the CLI.

**F5 — The gate is a nested `package main` module with a released trust root.**
This is the port's structural blocker (Q2) and also the reason a "just import the
Go logic" shortcut is not available.

**F6 — The sync spine's most dangerous contract is not the artifacts but the
stream/process contract:** errexit discipline inside `run_step` and the recipe
capture block (`a95bb01`), the four filter glyphs, temp-file trap coverage with
`${VAR:-}`, `mktemp` failure degradation, and `exit $RECIPE_RC` pass-through.
All are FROZEN and all are invisible to a naive port.

**F7 — The root `.gitignore` managed block has two implementations.**
`gitignore-root-refresh.py` (reached from `sync.sh:218`) and `init.sh:262-292`
(`append_block`/`strip_block` with `awk`, `L274-279`) each manage the same
marker-delimited block through different mechanisms (in-place refresh vs
strip-and-append). Card 07 cannot port one of them in isolation without
freezing a divergence; the reconciliation belongs with card 09 and should be an
explicit cross-card note.

---

## Tasks

- [x] T1 — Verify worktree, branch, base and clean pre-write baseline; confirm no
      provisioning output to revert.
- [x] T2 — Inventory all ten in-scope modules plus the four adjacent spine
      dependencies: entry points, I/O, side effects, exit codes, env, flags,
      stdout contract.
- [x] T3 — Build the cross-module invocation matrix and the Go-side gap list
      against `internal/{cli,config,home,lock,schema,target,toml}` and `assets`.
- [x] T4 — Document the strangler mechanics (existing shim + bridge patterns,
      forward per-step design, parity gating) including the gate-mode blind spot.
- [x] T5 — Produce the ordered chained-PR slicing with per-slice scope, gate,
      size and risk.
- [x] T6 — Risk register with a concrete test/fixture per risk.
- [x] T7 — Test strategy: differential oracles, fixture corpus additions,
      idempotency, partial failure, per-step matrix, coupled-test conversion.
- [x] T8 — Open questions with recommendations.
- [x] T9 — Fast checks on the untouched tree (gofmt/vet/test) and the full suite.
- [ ] T10 — Commit the planning package and run native review to closure.

## Evidence

Real exit codes, run from this worktree at base `bdf0fe8` with the tree otherwise
unmodified:

| Command | Result | Exit |
| --- | --- | --- |
| `git rev-parse --show-toplevel` | `.worktrees/go-07-plan-sync` | 0 |
| `git branch --show-current` | `change/go-07-plan-sync` | 0 |
| `git status --short` (pre-write) | empty | 0 |
| `gofmt -l .` | no output (clean) | 0 |
| `go vet ./...` | no output | 0 |
| `go test ./...` | `ok` for `internal/{cli,config,home,lock,schema,target,toml}`; `cmd/ai-specs` no test files | 0 |
| `./tests/run.sh` (full suite) | log `/tmp/go07-plan-sync-run.log`; phase 1 `test_vault_fs_mcp.sh` passed; phase 2 gate `go test` → `ok ai-specs.dev/worktree-gate 47.194s`, `ok …/ledger 2.944s`; phase 3 root `go test` → `ok` ×7 packages; phase 4 parity → `fixtures: 8, failing: 0` with all eight fixtures reported `zero deltas` (`log:22-38`); phase 5 `Ran 2372 tests in 2050.065s` → `OK (skipped=164)` (`log:274-276`), zero `FAIL:`/`ERROR:` lines | 0 (see note) |

Note on the suite exit code: `tests/run.sh` runs under `set -euo pipefail`
(`tests/run.sh:2`) and its last command is the unittest discovery run; every
phase reported success and unittest reported `OK`, so the script exited **0**.
The run was launched before this document was written, so the tree it observed
differs by exactly this one untracked file. That file cannot affect the suite:
no test or helper references `odd/` (verified: zero matches for `odd/` across
`tests/*.py`, `tests/*.sh` and `tests/parity/*`), and the repo-wide scanners
(`tests/_fixture_catalog.py:15-17`) read only `catalog/recipes` and
`tests/fixtures/recipes`. No `echo EXIT: $?` capture was recorded; if the
reviewer wants a captured exit code rather than the inference, re-run
`./tests/run.sh; echo "EXIT: $?"` — the run is offline and makes no network
calls.

Also empirical from the same log: `GO_LOCK_WRITE_BRIDGE_FALLBACK: no verified
worktree-gate binary; using the temporary Python lock-write authority` — the
suite's sync paths really do take the Python fallback in both legs, confirming
finding **F3**.

## What remains

1. The eight open questions (Q1–Q8) need human answers; Q1 and Q2 gate any
   implementation work.
2. No Go production code was written, per the card. The next session implements
   **S1 only** (spine + both gate modes) after Q1/Q2 are answered.
3. `openspec/changes/**` deliberately not created (process decision recorded at
   the top of this document).
4. Slice ODD stubs deliberately not created (card instruction).
