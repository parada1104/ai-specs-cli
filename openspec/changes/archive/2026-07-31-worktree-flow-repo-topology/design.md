# Design: worktree-flow repo topologies

## Technical Approach

Introduce one `repo_topology` recipe config field (enum, default `auto`, validated by the existing `merge_config()` enum path — no new stamping) and one **shared Python resolution helper** that every harness surface calls to answer a single question: *given a project root and its configured topology value, which topology are we in and — under submodules — which submodule checkouts exist?*

The helper (`util.resolve_repo_topology`) is the load-bearing architectural decision of this phase. `.gitmodules` + `git submodule status` parsing lives in exactly one place; the wizard prefill, hub/status header, agent brief, and any doctor check are thin readers of its result. The materialized `worktree-cleanup.sh` override is the **one intentional second implementation**: it runs detached as bash in consumer repos with no Python available, so it self-detects topology in shell — this is a documented bash mirror of the same rule, not accidental per-call-site duplication.

`/worktree-new` and `/worktree-clean` are prose/skill + script contracts. Under a resolved `monorepo-submodules` topology they switch to the verified `git -C <subrepo>` create contract (absolute destination under the superproject `worktrees_dir`) and per-submodule cleanup enumeration. `standalone` and `monorepo-apps` keep today's exact single-repo mechanics as a no-op path.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Where topology is decided | One shared helper `util.resolve_repo_topology(repo_root, config_value)`; all Python surfaces call it | Inline `.gitmodules`/`submodule status` parsing in wizard, hub, brief, doctor | Four copies of prefix-parsing logic drift; the explore already flagged misclassification traps (name≠path, `-` prefix). Single helper makes the rule auditable and testable once. |
| Helper location | `lib/_internal/util.py` (stdlib-only, `subprocess` already imported, universally loaded via `_load_sibling("util")`) | New `topology.py` module | util.py is already the shared stdlib home every sibling loads; a new module adds a loader stanza to 4 files for one function group. |
| Helper input | Caller passes the already-read `config_value`; helper never reads `ai-specs.toml` | Helper reads the manifest itself | Keeps util.py free of the hyphenated `toml-read.py` dependency and its import-time contract (stdlib only); every call site already holds the resolved recipe config. |
| `auto` classification | Initialized `.gitmodules` entries → `monorepo-submodules`; else `standalone`. Never `monorepo-apps`. | Classify apps-with-vendored-submodules heuristically | Locked in proposal; `monorepo-apps` is a self-describing naming convention only, mechanically identical to standalone. |
| Cleanup runtime detection | `worktree-cleanup.sh` self-detects in bash (not via the Python helper) | Shell out to Python helper from the script | `condition = "not_exists"` means the script lives standalone in consumer repos with no guaranteed Python/ai-specs on PATH; a bash mirror is the only reliable runtime. |
| Cleanup helper reuse | Wrap today's scan+flush block in `_cleanup_one()`; outer loop over submodules; `resolve_base_candidates`/`is_merged`/`candidate_has_*` byte-unchanged | Plumb `git -C`/`--cwd` through every helper | Explore Approach A: smallest delta on a well-tested script; helpers already operate on process cwd, so `cd <module>` per iteration reuses them verbatim. |
| Stale `not_exists` override | Non-blocking WARN on sync + optional mirror in doctor via one shared `util.override_is_stale()` | Silently overwrite; or flip `condition` to always-copy | Overwriting destroys consumer customizations (melon/venturi); WARN + refresh steps preserve the `not_exists` guarantee (user decision 3). |
| Gate | No functional change | Stamp topology into the gate | Gate is already correct per-git-dir (explore §5); topology is orthogonal. |

## 1. Topology detection & resolution (shared helper)

New in `lib/_internal/util.py` (stdlib + `subprocess`, no rich/questionary, no `toml-read`):

```python
from dataclasses import dataclass

@dataclass(frozen=True)
class TopologyResolution:
    resolved: str            # "standalone" | "monorepo-apps" | "monorepo-submodules"
    configured: str          # "auto" | one of the above
    via: str                 # "config" (explicit) | "auto" (detected)
    submodules: tuple[str, ...]      # initialized submodule paths, relative to repo_root
    gitmodules_present: bool

def detect_submodules(repo_root: Path) -> tuple[bool, tuple[str, ...]]:
    """Return (gitmodules_present, initialized_submodule_paths).

    Pure inspection; no worktree/branch mutation. Non-recursive (v1).
    """
    gm = repo_root / ".gitmodules"
    if not gm.is_file():
        return (False, ())

    # name→path from .gitmodules via git config (robust vs hand-parsing INI):
    #   git -C <root> config -f .gitmodules --get-regexp '^submodule\..*\.path$'
    #   → lines "submodule.<name>.path <path>"
    registered_paths = _run_git_config_paths(repo_root)   # set[str]

    # init state from `git -C <root> submodule status` (non-recursive).
    # Each line: "<prefix><sha> <path> (<describe>)".
    #   prefix ' '  → initialized, in sync with superproject index
    #   prefix '+'  → initialized, checked-out commit differs from index
    #   prefix 'U'  → initialized, merge conflicts
    #   prefix '-'  → NOT initialized (needs `submodule update --init`)
    initialized: list[str] = []
    for line in _run_submodule_status(repo_root):
        if not line:
            continue
        prefix = line[0]
        rest = line[1:].split()          # [sha, path, ...]
        if len(rest) < 2:
            continue
        path = rest[1]
        if path not in registered_paths:  # ignore stray/legacy status noise
            continue
        if prefix != "-":                  # ' ', '+', 'U' all have a usable checkout
            initialized.append(path)
    return (True, tuple(sorted(initialized)))

def resolve_repo_topology(repo_root: Path, config_value: str = "auto") -> TopologyResolution:
    configured = (config_value or "auto").strip() or "auto"

    if configured in ("standalone", "monorepo-apps"):
        # No-op path: no submodule inspection needed.
        return TopologyResolution(configured, configured, "config", (), False)

    if configured == "monorepo-submodules":
        present, subs = detect_submodules(repo_root)   # still enumerate for create/clean
        return TopologyResolution("monorepo-submodules", configured, "config", subs, present)

    # configured == "auto"
    present, subs = detect_submodules(repo_root)
    resolved = "monorepo-submodules" if subs else "standalone"
    return TopologyResolution(resolved, "auto", "auto", subs, present)
```

**Contract summary**

- Input: `repo_root: Path` (superproject / project root where `ai-specs.toml` lives), `config_value: str` (the raw `repo_topology` from resolved recipe config; `"auto"` when absent).
- Output: `TopologyResolution` — resolved topology, how it was decided (`via`), and the initialized submodule paths (empty for standalone/apps).
- Git failures (not a repo, git missing) degrade to `standalone` with `submodules=()` — surfaces render "standalone" rather than raising; a non-fatal note is acceptable but never blocks.
- Non-recursive: `git submodule status` (not `--recursive`); nested submodules out of scope (proposal).

## 2. `<subrepo>` resolution for `/worktree-new` (monorepo-submodules)

Only runs when `resolve_repo_topology(...).resolved == "monorepo-submodules"`. Under `standalone`/`monorepo-apps`, `<subrepo>` MUST be absent (ignored with a warning); create stays at repo root.

Inputs: `super_root` (project root), `worktrees_dir`, initialized submodule paths (from the helper), current `cwd`, optional explicit `<subrepo>` arg.

```text
resolve_subrepo(super_root, worktrees_dir, initialized_paths, cwd, explicit):
  inferred = None

  # (a) cwd inference — no reliance on --show-superproject-working-tree,
  #     which the verified contract proved EMPTY from linked worktrees.
  top = `git -C <cwd> rev-parse --show-toplevel`      # may fail → top = None
  if top:
    rel = relpath(top, super_root)
    if rel in initialized_paths:
        inferred = rel                                 # primary submodule checkout
    else:
        # linked worktree: top == <super>/<worktrees_dir>/<name>-<slug>
        base = basename(top)                           # "<name>-<slug>"
        parent = relpath(dirname(top), super_root)     # must equal worktrees_dir
        if parent == worktrees_dir.rstrip("/"):
            # path-prefix match against registered paths; names may contain
            # hyphens (e.g. "alquimia-front-web"), so pick the LONGEST path P
            # such that base startswith f"{P}-". Longest wins to disambiguate
            # "alquimia-front" vs "alquimia-front-web".
            cands = [p for p in initialized_paths if base.startswith(p + "-")]
            if cands:
                inferred = max(cands, key=len)

  # (b) reconcile explicit vs inferred
  if explicit and inferred and normalize(explicit) != inferred:
      ERROR: "cwd is inside submodule '<inferred>' but you passed '<explicit>'"
  subrepo = explicit or inferred
  if not subrepo:
      ERROR: "monorepo-submodules: pass <subrepo> (cannot infer from cwd)"

  # (c) validate against .gitmodules (path first, then unique name)
  if subrepo in registered_paths:
      resolved_path = subrepo
  else:
      by_name = [p for (name, p) in gitmodules_entries if name == subrepo]
      if len(by_name) == 0: ERROR "unknown submodule '<subrepo>'"
      if len(by_name) > 1:  ERROR "ambiguous name '<subrepo>'; use its path"
      resolved_path = by_name[0]

  # (d) reject uninitialized
  if resolved_path not in initialized_paths:
      ERROR: "submodule '<resolved_path>' not initialized; run "
             "git submodule update --init <resolved_path>"

  return resolved_path
```

Rejection rules restated: unknown subrepo, ambiguous **name** (resolve by path to disambiguate), uninitialized (`-` prefix), and explicit/inferred mismatch all hard-error before any `git worktree add`.

## 3. Command sequences (copy-pasteable)

### Create — monorepo-submodules

```bash
super_abs="$(git -C "$super_root" rev-parse --show-toplevel)"
git -C "$super_abs/$subrepo_path" worktree add \
  "$super_abs/${worktrees_dir%/}/${subrepo}-${slug}" \
  -b "$branch" "$integration_branch"
```

- Runs from anywhere via `-C`; **no `cd`/pushd required** (verified contract).
- Destination is **absolute** and mandatory: under `git -C <subrepo>`, a relative `worktree add` path resolves *inside* the submodule (bug).
- `${subrepo}` in the directory name is the submodule **path** basename (melon: path == name), yielding the lived `<super>/.worktrees/<subrepo>-<slug>` layout.

### Create — standalone / monorepo-apps (unchanged)

```bash
git worktree add "${worktrees_dir%/}/${slug}" -b "$branch" "$integration_branch"
```

### Cleanup enumeration — monorepo-submodules

```bash
# Locked enumeration: never trust superproject `git worktree list` alone.
git submodule foreach --quiet 'git worktree list --porcelain'
# equivalently, per initialized submodule path P:
#   git -C "$P" worktree list --porcelain
```

Each initialized submodule's `worktree list` includes the shared `<super>/<worktrees_dir>/<P>-*` linked worktrees it owns (verified). The superproject `git worktree list` shows only the superproject main worktree and is never the sole candidate source under submodules.

## worktree-cleanup.sh restructure (interface + backward compat — #7)

Today (standalone-only): one `ROOT`, one `WT_PREFIX`, one scan loop feeding `is_merged`/`flush` after a single `cd "$ROOT"`.

New structure — **merge/scan helpers unchanged**; wrap the existing scan block in a function and add an outer submodule loop:

```bash
# New flags (added to the existing --dir/--base/--dry-run parser):
#   --submodule <path>   scope cleanup to one initialized submodule (repeatable);
#                        default = all initialized submodules
#   (--subrepo <path>    accepted as an alias of --submodule)
SUBMODULE_SCOPE=()        # empty = all

SUPER_ROOT="$(git rev-parse --show-toplevel)"
WT_ROOT="$SUPER_ROOT/${WORKTREES_DIR%/}"     # shared superproject worktrees dir

# --- topology self-detect (bash mirror of util.resolve_repo_topology) --------
# monorepo-submodules iff .gitmodules exists AND has >=1 non-'-' status line.
enumerate_modules() {   # prints one repo dir per line
    if [[ -f "$SUPER_ROOT/.gitmodules" ]] && \
       git -C "$SUPER_ROOT" submodule status 2>/dev/null | grep -qv '^-'; then
        # monorepo-submodules
        while IFS= read -r line; do
            [[ -z "$line" ]] && continue
            [[ "${line:0:1}" == "-" ]] && continue           # skip uninitialized
            local p; p="$(awk '{print $2}' <<<"${line:1}")"
            if ((${#SUBMODULE_SCOPE[@]})); then
                _in_scope "$p" || continue
            fi
            printf '%s\n' "$SUPER_ROOT/$p"
        done < <(git -C "$SUPER_ROOT" submodule status)
    else
        printf '%s\n' "$SUPER_ROOT"                          # standalone / apps
    fi
}

# --- one cleanup pass over the CURRENT cwd (unchanged inner logic) -----------
_cleanup_one() {   # assumes: cwd == repo dir, WT_PREFIX set, BASE_BRANCH set
    while IFS= read -r line; do
        ...                       # EXACT current parse → is_merged → flush block
    done < <(git worktree list --porcelain)
    flush
}

# --- outer loop --------------------------------------------------------------
while IFS= read -r repo_dir; do
    cd "$repo_dir"
    WT_PREFIX="$WT_ROOT/"         # shared prefix for ALL modules (not per-module)
    if [[ -z "$BASE_BRANCH" ]]; then
        base="$(git symbolic-ref --quiet --short HEAD || echo main)"
    else
        base="$BASE_BRANCH"
    fi
    BASE_BRANCH="$base" _cleanup_one
done < <(enumerate_modules)
```

**Signatures / boundaries**

- New: `enumerate_modules()` (prints repo dirs), `_cleanup_one()` (wraps the existing scan+`flush`), `_in_scope()` (path membership for `--submodule`).
- Unchanged (byte-identical): `flush`, `debug_log`, `resolve_base_candidates`, `candidate_has_merged_tip`, `candidate_has_patch_equivalence`, `is_merged`, and the `WORKTREE_CLEANUP_SOURCE_ONLY` sourcing guard.
- `WT_PREFIX` stays `SUPER_ROOT/<worktrees_dir>/` for every module so shared `<module>-<slug>` entries are recognized (not recomputed per module).

**Backward compatibility (no-op path)**

- No `.gitmodules` (standalone) or apps: `enumerate_modules` prints exactly one line (`SUPER_ROOT`), the loop runs once, `WT_PREFIX == SUPER_ROOT/.worktrees/`, and behavior is byte-for-byte today's. Existing `--dir`/`--base`/`--dry-run` callers and output lines (`removed`/`would remove`/`skipped …`) are unchanged.
- `--submodule` on a standalone repo: no submodules → single sentinel pass; flag is inert (documented), not an error.

## 4. Wizard data flow / interfaces (#4)

New question node in `init_tui.run_wizard`, immediately after the "Project name:" prompt (`init_tui.py` L241) and before the agents checkbox:

```text
project_name = questionary.text("Project name:", ...)     # existing L241
# NEW node:
det = _util.resolve_repo_topology(target, "auto")          # auto-detect for default
default = det.resolved                                      # e.g. "monorepo-submodules"
topology = hub-style select over
           ["auto", "standalone", "monorepo-apps", "monorepo-submodules"]
           with default = "auto" (label shows "auto → <default> (detected)")
```

- Returned value: a plain string `topology` (one of the enum members).
- Persistence path: the wizard threads it into the `configured` map under the worktree-flow recipe so `_render_manifest` emits it in the existing `[recipes.worktree-flow.config]` block (init_tui.py L167–169):

  ```toml
  [recipes.worktree-flow.config]
  repo_topology = "auto"
  ```

  Exact key path in the staged manifest: `recipes."worktree-flow".config.repo_topology`. When worktree-flow is not among the selected recipes, the wizard records the answer only if the recipe is enabled (matches "always record intent for later enable" — otherwise it is dropped, since a config block for a disabled recipe is noise).
- `config_wizard.run_config_wizard` needs **no special-casing**: once `[config.repo_topology]` (enum) exists in `recipe.toml`, the existing `if field.enum:` branch (config_wizard.py L101–108) renders the `questionary.select` automatically, with `help_text` shown at L96–97. The init identity prompt is the first-time human confirm; the recipe-config wizard is the later edit path. Both write the same key.

## 5. Hub / status + brief surfacing (#5) — call sites of the shared helper

"Resolved topology" is **computed by `util.resolve_repo_topology`** (never re-parsed inline). The configured value is read from the manifest by each surface using its existing manifest reader, then passed in.

| Surface | File / locus | Wiring |
|---|---|---|
| Interactive hub panel | `hub.py` `StatusPanel.render` (L314–341) | `StatusSummary` gains `topology: str` and `topology_via: str`. `status_summary()` (L183) reads `repo_topology` from the manifest and calls `_util.resolve_repo_topology(root, cfg)`. `render` adds one grid row: `topology  monorepo-submodules (auto→…)`. |
| Noninteractive status | `hub.py` `_run_noninteractive` (L214–222) | Prints one line: `  topology: {summary.topology} ({via})`, mirroring the version/target lines. |
| Agent brief | `agents-render.py` `_section_project` (L174) | After the integration_branch block (L211–212), read `recipes.<wf>.config.repo_topology` from `resolved` and call the shared helper; append `- **Repo topology**: \`<resolved>\` (via <config|auto>)`. |
| Doctor (optional) | `doctor.py` new `_check_repo_topology` | INFO check echoing resolved topology + initialized submodule count; calls the same helper. Non-blocking. |

`config_wizard.py` is intentionally **not** a call site (schema-driven enum only). The helper is loaded the standard way: `_util = _load_sibling("util")` (already present in hub.py L34 and init_tui.py L66; add to agents-render.py and doctor.py via their existing sibling-load stanzas).

## 6. Sync-time stale `not_exists` override WARN (#6)

**"Stale" defined:** for a `[[provides.templates]]` entry with `condition = "not_exists"` whose `target` already exists in the project, the materialized file's bytes differ from the current catalog `source` bytes. Compare by content (sha256 or `filecmp`-style byte compare), not mtime.

Shared comparator in `util.py` (parallels the topology-helper decision — one implementation, two callers):

```python
def override_is_stale(catalog_src: Path, materialized_dest: Path) -> bool:
    """True when a not_exists override exists but no longer matches the catalog
    template it was seeded from. Missing dest → not stale (fresh copy path)."""
    if not materialized_dest.is_file() or not catalog_src.is_file():
        return False
    return sha256(catalog_src.read_bytes()) != sha256(materialized_dest.read_bytes())
```

**Plug point (sync):** `recipe-materialize.materialize_template` (L334–345), inside the existing `if tpl.condition == "not_exists": if dest.exists():` branch (L339–341). Before `return`, if `override_is_stale(src, dest)` → emit a non-blocking `warn(...)` (not `fail`). Message:

```
WARN: worktree-flow: your override
  ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh
differs from the current catalog template (condition=not_exists, not refreshed).
Review upstream changes, then either re-apply your customizations or refresh with:
  rm <target> && ai-specs sync
```

**Doctor:** does **not** duplicate the diff — `doctor.py` adds `_check_stale_template_overrides` that resolves each enabled recipe's `not_exists` templates and calls the same `util.override_is_stale`, emitting a WARN check. Sync WARN is the primary surface; doctor mirrors it via the shared function.

## File Changes

| File | Action | Description |
|---|---|---|
| `lib/_internal/util.py` | Modify | Add `TopologyResolution`, `detect_submodules`, `resolve_repo_topology`, `override_is_stale` (stdlib + `subprocess` only). |
| `catalog/recipes/worktree-flow/recipe.toml` | Modify | `[config.repo_topology]` enum (`auto\|standalone\|monorepo-apps\|monorepo-submodules`, default `auto`, help_text); brief `workflow_rules` "which repo" clause; version bump. |
| `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` | Modify | `--submodule`/`--subrepo` flags; `enumerate_modules`/`_cleanup_one`/`_in_scope`; shared `WT_PREFIX`; merge/scan helpers unchanged. |
| `catalog/recipes/worktree-flow/commands/worktree-new.md` | Modify | `<subrepo>` signature + resolution rules + locked `git -C` absolute-destination create command. |
| `catalog/recipes/worktree-flow/commands/worktree-clean.md` | Modify | Optional `--submodule` scope; submodule enumeration; default = all. |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Modify | Per-topology create/clean matrix; `git -C` contract; strengthened pre-delegation "which git repository" rule. |
| `lib/_internal/init_tui.py` | Modify | Topology auto-detect + confirm node after project name; write `recipes.worktree-flow.config.repo_topology`. |
| `lib/_internal/hub.py` | Modify | `StatusSummary.topology`/`topology_via`; `status_summary` calls helper; `StatusPanel.render` + `_run_noninteractive` surface it. |
| `lib/_internal/agents-render.py` | Modify | `_section_project` adds resolved **Repo topology** line via helper. |
| `lib/_internal/recipe-materialize.py` | Modify | `materialize_template` emits stale-override WARN via `util.override_is_stale`. |
| `lib/_internal/doctor.py` | Modify (optional) | `_check_repo_topology` (INFO) + `_check_stale_template_overrides` (WARN) via shared helpers. |
| `catalog/recipes/worktree-flow/README.md`, `docs/recipes-catalog.md`, `docs/ai-specs-toml.md` | Modify | Document `repo_topology`, shared `<worktrees_dir>/<subrepo>-<slug>` layout, stale-override refresh. |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Optional | Message-only hint mentioning `/worktree-new <subrepo>`; no decision change. |
| `openspec/specs/worktree-flow/spec.md` (via delta) | Modify | Topology requirements + brief "which repo" scenarios. |

## Interfaces / Contracts

- `recipes.worktree-flow.config.repo_topology`: `auto | standalone | monorepo-apps | monorepo-submodules`, default `auto`. Validated by `merge_config()` enum reject exactly like `gate_mode`.
- `util.resolve_repo_topology(repo_root: Path, config_value: str = "auto") -> TopologyResolution` — see §1. `via ∈ {"config","auto"}`.
- `util.detect_submodules(repo_root: Path) -> tuple[bool, tuple[str, ...]]`.
- `util.override_is_stale(catalog_src: Path, materialized_dest: Path) -> bool`.
- `worktree-cleanup.sh` CLI: adds `--submodule <path>` (alias `--subrepo`), repeatable, default all initialized submodules; standalone/apps ignore it (single pass). Existing flags/output lines unchanged.
- `/worktree-new` create contract (monorepo-submodules): `git -C <subrepo_path> worktree add <super_abs>/<worktrees_dir>/<subrepo>-<slug> -b <branch> <integration_branch>` — absolute destination, no `cd`.

## Testing Strategy (pointers only — future tasks own authoring)

| Layer | What to cover | Fixture |
|---|---|---|
| Helper unit | `detect_submodules` prefix parsing (`' '`/`'+'`/`'U'` initialized, `'-'` skipped); name≠path; longest-prefix linked-worktree match; `resolve_repo_topology` auto vs explicit vs no-gitmodules → standalone | New temp-repo fixture with an **initialized submodule** (a second local repo added via `git submodule add`), mirroring `tests/test_worktree_cleanup.py` temp-repo patterns; add an uninitialized `-` entry for rejection. |
| Enum | default `auto`, `merge_config` reject of bad value | Extend `tests/test_worktree_flow_recipe.py` (sibling of `test_sync_rejects_invalid_gate_mode`). |
| Cleanup loop | standalone no-op parity; submodule iteration finds `<module>-<slug>` under shared `.worktrees/`; `--submodule` scoping; merged/dirty/unmerged classification unchanged inside a submodule | Submodule fixture above added to `tests/test_worktree_cleanup.py`; assert existing output lines. |
| Wizard/status | wizard writes `repo_topology`; hub panel + noninteractive + brief show resolved topology | `tests/test_init_tui.py`, hub/agents-render tests. |
| Stale WARN | modified `not_exists` override emits WARN, not overwrite; doctor mirror | recipe-materialize + doctor tests using a hand-edited override file. |

design.md commits future authors to: one submodule fixture builder reused across helper + cleanup tests; assertions on `TopologyResolution` fields; and WARN-path (not overwrite) assertions.

## Migration / Compatibility (#9)

- Manifests without `repo_topology` → `merge_config` default `auto`; `auto` resolves to `standalone` when no initialized `.gitmodules`, so existing standalone/apps projects are unchanged.
- Existing melon/venturi materialized `worktree-cleanup.sh` overrides keep working: `not_exists` still never overwrites them; they only gain a non-blocking sync/doctor WARN with refresh steps. Nothing forces a rewrite.
- Rollback: revert recipe/version, helper, and surface wiring; consumer overrides untouched (WARN-only is non-destructive); no data migration.

## Open Questions

None blocking. Deferred (proposal out-of-scope): nested/recursive submodules; per-submodule `integration_branch`; a `--superproject` escape hatch for root-level changes under submodules (docs may suggest temporarily setting `repo_topology = "standalone"`); doctor heuristic WARN for `vendor/`-style submodules misclassified by `auto`.
