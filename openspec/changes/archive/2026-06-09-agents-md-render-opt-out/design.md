# Design: agents-md-render-opt-out

## Technical Approach

Add `[brief].render` (boolean, default `true`) to the manifest contract. When `false`,
shell orchestrators (`sync.sh`, `init.sh`, `subrepo path in sync-agent.sh`) skip
`agents-render.py` entirely — no change to renderer composition logic. A small shared
Python module (`brief-render-policy.py`) parses the flag once with strict boolean
semantics; bash callers branch on its stdout. `doctor.py` reuses the same helper and
inspects resolved-config for dead recipe fragments.

`recipe-materialize.py`, skills, MCP, and hooks are **unchanged** — only the AGENTS.md
write step is gated.

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where flag is enforced | Shell callers before `agents-render.py` | Matches existing `--preserve-if-runtime-brief` orchestration; renderer stays pure compose-and-write |
| Shared parser | `lib/_internal/brief-render-policy.py` | Single source for sync/init/sync-agent/doctor; avoids duplicating TOML logic in 3 bash files |
| Default when key absent | `true` | Zero breaking change; omit `render` = current behavior |
| Only `false` disables | `brief.get("render", True) is not False` | Explicit opt-out; `true` and absent both enable |
| Invalid `render` type | Validation error in doctor; sync/init fail fast with stderr | Aligns with strict parsing and card #16 boolean footgun |
| Init placeholder | `# AGENTS.md - Runtime context` (existing string) | Reuses fallback; downstream symlink steps need a file |
| Subrepo policy | Root manifest `TOML_PATH` on `SOURCE_ROOT` | V1 subrepos have no local manifest; inherit root flag |
| `resolved-config` when render off | Still produced by `recipe-materialize` | Other artifacts (MCP, hooks) need it; fragments in JSON are harmless |
| Placeholder doctor WARN | **Deferred (non-MVP)** | Optional follow-up; not blocking apply |

## Precedence Matrix

```
Priority  Condition                              Action
────────  ─────────────────────────────────────  ──────────────────────────────
1         [brief].render = false                 Skip agents-render; preserve file
2         render enabled + marker in AGENTS.md     Skip (agents-render early return)
3         render enabled + no marker               Full render (fragments + [brief])
```

Marker is irrelevant when priority 1 applies (renderer not invoked).

## `brief-render-policy.py`

New module at `lib/_internal/brief-render-policy.py`.

```python
PLACEHOLDER_LINE = "# AGENTS.md - Runtime context"

def brief_render_enabled(manifest: dict) -> bool:
    """Return False only when [brief].render is explicitly false."""
    brief = manifest.get("brief")
    if not isinstance(brief, dict):
        return True
    render = brief.get("render", True)
    if render is False:
        return False
    if render is True:
        return True
    raise ValueError(
        "[brief].render must be a boolean (true or false); "
        f"got {type(render).__name__}"
    )

def load_brief_render_enabled(toml_path: Path) -> bool:
    with toml_path.open("rb") as f:
        data = tomllib.load(f)
    return brief_render_enabled(data)

def has_dead_recipe_fragments(resolved: dict) -> bool:
    """True if any enabled recipe has non-empty brief_fragments."""
    for rid in resolved.get("enabled", []):
        frags = (resolved.get("recipes", {}).get(rid, {}) or {}).get("brief_fragments") or {}
        if any(frags.get(sec) for sec in frags):
            return True
    return False
```

CLI (for bash):

```
python3 brief-render-policy.py <toml_path>
  → prints "true" or "false" to stdout, exit 0

python3 brief-render-policy.py <toml_path> --validate
  → exit 0 if render key absent/true/false boolean
  → exit 1 + stderr if non-boolean render value
```

Shell callers use:

```bash
BRIEF_RENDER_POLICY_PY="$AI_SPECS_HOME/lib/_internal/brief-render-policy.py"

if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
  python3 "$AGENTS_RENDER_PY" ...
else
  echo "  · skipped AGENTS.md (brief.render = false)"
fi
```

## `sync.sh` integration

Replace unconditional `agents-render` block (lines ~115-116):

```bash
echo "▸ agents-render (root)"
if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
  python3 "$AGENTS_RENDER_PY" "$TOML_PATH" "$ROOT_PATH/AGENTS.md" \
    --preserve-if-runtime-brief --resolved-config "$RESOLVED_CONFIG_TEMP"
else
  echo "  · skipped AGENTS.md (brief.render = false)"
fi
```

`recipe-materialize` block above is **unchanged**.

## `init.sh` integration

Replace block 3b (materialize + render) with:

```bash
if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
  # existing materialize + agents-render pipeline (unchanged flags)
  ...
else
  if [[ -f "$AGENTS_PATH" ]]; then
    echo "  · skipped AGENTS.md (brief.render = false)"
  else
    echo "$PLACEHOLDER_LINE" > "$AGENTS_PATH"
    echo "  · skipped AGENTS.md render (brief.render = false)" >&2
    echo "  ! created placeholder — replace with your manual brief" >&2
  fi
fi
```

When render is disabled and `AGENTS.md` exists (including `init --force`), file is
never modified — no materialize call needed for brief purposes.

## `sync-agent.sh` integration

In `ensure_target_workspace()` subrepo branch (non-root `TARGET_PATH`):

```bash
if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
  python3 "$AGENTS_RENDER_PY" "${render_args[@]}"
else
  [[ -f "$TARGET_AGENTS_MD" ]] || {
    echo "ERROR: $TARGET_AGENTS_MD not found and brief.render = false." >&2
    echo "       Create AGENTS.md manually or set [brief].render = true." >&2
    exit 1
  }
  echo "    · skipped AGENTS.md (brief.render = false)"
fi
```

Root path (`TARGET_PATH == SOURCE_ROOT`) unchanged: only verifies `AGENTS.md` exists.

Skills/commands/gitignore mirroring runs regardless of render flag.

## `doctor.py` integration

New method `_check_brief_render_policy()` called from `run()` after `_check_agents_md()`:

| Condition | Severity | Check name |
|-----------|----------|------------|
| `[brief].render = false` | INFO | `brief-render` |
| `render = false` + no `AGENTS.md` | ERROR | `brief-render` |
| `render = false` + enabled recipes with fragments | WARN | `brief-fragments-unused` |
| `render = false` + marker in AGENTS.md | INFO | `brief-render-marker` |

Fragment detection: build resolved-config (reuse existing doctor materialize pattern)
and call `has_dead_recipe_fragments(resolved)`.

Adjust `_check_agents_md()` guidance when `render=false`: suggest manual creation, not
"run ai-specs sync".

## Messages (exact strings)

| Context | Channel | Text |
|---------|---------|------|
| sync skip | stdout | `  · skipped AGENTS.md (brief.render = false)` |
| init skip (file exists) | stdout | `  · skipped AGENTS.md (brief.render = false)` |
| init placeholder | stderr | `  ! created placeholder — replace with your manual brief` |
| subrepo skip | stdout | `    · skipped AGENTS.md (brief.render = false)` |
| subrepo missing file | stderr | `ERROR: ... not found and brief.render = false.` |

## Documentation targets

- **`docs/ai-specs-toml.md`** — add `[brief].render` row to table; subsection on precedence
  (flag vs marker vs `_mode`); subrepo inheritance; migration from marker-only opt-out.
- **`templates/ai-specs.toml.tmpl`** — commented example:
  ```toml
  # [brief]
  # render = false   # opt out of managed AGENTS.md generation (default: true)
  ```

## Testing Strategy (strict TDD)

| File | Scope |
|------|-------|
| `tests/test_brief_render_policy.py` (NEW) | `brief_render_enabled()` — absent→true, true, false, non-boolean raises; CLI stdout |
| `tests/test_agents_md_render_opt_out.py` (NEW) | E2E sync/init/subrepo skip; byte-stability; placeholder; regression render=true |
| `tests/test_doctor.py` (extend) | INFO/WARN/ERROR checks for render=false configs |
| `tests/test_sync_pipeline.py` (extend) | Marker regression when render=true (unchanged) |
| `tests/test_runtime_brief_baseline.py` (extend) | Init/sync idempotency when render=false |

## Edge Cases

- `render = false` with only `[brief]` table `{render=false}` → disabled.
- `render = false` + recipes with fragments → sync OK, doctor WARN.
- `init --force` + existing manual AGENTS.md + render=false → byte-identical.
- TOML parse error → existing manifest ERROR in doctor.
- Subrepo fan-out with render=false → skip render, still sync skills.

## Sequencing

1. `brief-render-policy.py` + unit tests.
2. `sync.sh` guard + E2E sync skip test.
3. `init.sh` guard + E2E init tests.
4. `sync-agent.sh` guard + subrepo E2E tests.
5. `doctor.py` checks + doctor tests.
6. Docs + template comment.
7. Full `./tests/validate.sh`.

## Open Questions (resolved for apply)

| # | Question | Resolution |
|---|----------|------------|
| 1 | String `"false"` | ERROR via `brief_render_enabled` ValueError |
| 2 | Placeholder heuristic WARN | Deferred post-MVP |
| 3 | Keep `brief_fragments` in resolved-config | Yes — materialize unchanged |
| 4 | `--force-brief` CLI | Out of scope |
