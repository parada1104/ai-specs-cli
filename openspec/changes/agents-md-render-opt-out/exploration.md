## Exploration: agents-md-render-opt-out

**Trello:** [#18 — Opt-out en manifest: sync no modifica AGENTS.md (ni recipes)](https://trello.com/c/jVqeBkCr/18-spike-opt-out-en-manifest-sync-no-modifica-agentsmd-ni-recipes)

### Problem Statement

Since v0.11.0, `ai-specs sync` and `ai-specs init` regenerate root `AGENTS.md` on every run via `agents-render.py`, composing:

1. Structured fields from `resolved-config.json` (board_id, integration_branch, test_command, vault_scope, MCP list, capability bindings).
2. Prose from manifest `[brief]`.
3. Behavioral bullets from enabled recipes' `[provides.brief]` fragments (merged append-first, with per-section `_mode = "replace"` opt-in).

This is the right default for greenfield adopters. It is the wrong default for projects that:

- Migrated prose with `/rules-audit` and want a **fully hand-managed** runtime brief.
- Curate `AGENTS.md` as canonical project voice and do not want recipe fragments prepended on every sync.
- Need manifest-level intent ("do not touch my brief") without editing the markdown file to inject an HTML comment.

**Today the only first-class escape hatch is file-level:** if `AGENTS.md` contains `<!-- ai-specs:runtime-brief -->`, callers pass `--preserve-if-runtime-brief` and the renderer exits without writing. That works but is opaque (users must know the marker), lives outside `ai-specs.toml`, and is easy to miss in docs/onboarding.

**Gap:** No manifest flag to disable the entire AGENTS.md write pipeline globally for a project.

---

### Architecture Map

```
CURRENT FLOW (root)
───────────────────
ai-specs.toml
  [brief] prose ────────────────────────────────┐
  [recipes.*] enabled ──► recipe-materialize.py ──┤
                                                  ▼
                                         resolved-config.json
                                                  │
sync.sh / init.sh                                 │
  python3 agents-render.py                        │
    --preserve-if-runtime-brief  ◄── marker only │
    --resolved-config ...                         │
                                                  ▼
                                         AGENTS.md (overwritten)
                                                  │
sync-agent.sh (root)                              │
  CLAUDE.md ──symlink──► AGENTS.md                │
  .cursor/...                                     │

CURRENT FLOW (subrepo target)
─────────────────────────────
sync-agent.sh --target <subrepo>
  ensure_target_workspace()
    agents-render.py (NO --preserve-if-runtime-brief)
    → always overwrites subrepo/AGENTS.md

TARGET FLOW
───────────
ai-specs.toml
  [brief]
    render = false   ◄── NEW manifest opt-out (default: true)

sync.sh / init.sh
  IF brief.render == false:
    skip agents-render entirely
    preserve existing AGENTS.md if present
    emit: "· skipped AGENTS.md (brief rendering disabled)"
  ELSE:
    existing pipeline (marker escape hatch still applies)

sync-agent.sh (subrepo)
  IF root manifest brief.render == false:
    skip agents-render for subrepo
    preserve / require existing file (same as root policy)
  ELSE:
    existing subrepo render
```

**Call sites today (verified in tree):**

| Location | Invokes `agents-render.py` | `--preserve-if-runtime-brief` |
|----------|---------------------------|-------------------------------|
| `lib/sync.sh:116` | root `AGENTS.md` | yes |
| `lib/init.sh:189-190` | baseline on init | yes |
| `lib/sync-agent.sh:229-233` | subrepo `AGENTS.md` | **no** |

**Non-goals for this change (per card):** skills, MCP presets, hooks, recipe materialize, agent symlinks — all continue to run. Only the **write** of `AGENTS.md` is suppressed.

---

### Decision 1: Flag name and location

**Options:**

| Option | Shape | Pros | Cons |
|--------|-------|------|------|
| A | `[brief] render = false` | Co-located with brief config; discoverable in docs § `[brief]` | Slightly overloaded — `[brief]` also holds prose that becomes irrelevant when render=false |
| B | `[brief] managed = false` | Reads naturally ("unmanaged brief") | Ambiguous vs "managed block" in `.gitignore` |
| C | `[project] agents_md_managed = false` | Project-wide policy | Far from `[brief]` docs; prose keys still in `[brief]` |
| D | `[runtime_brief] enabled = false` | New top-level section | Extra schema surface; duplicates concept of `[brief]` |

**Recommendation: Option A — `[brief] render = false`.**

- Default **`true`** (omit key = current behavior). Zero breaking change for existing manifests.
- Boolean TOML lowercase (`true`/`false`) — aligns with card #16 footgun lesson; validate in doctor.
- When `render = false`, manifest `[brief]` prose and `[provides.brief]` fragments are **not emitted** to disk. They may remain in config for documentation or a future re-enable, but have no runtime effect on `AGENTS.md`.

**Rejected names:** `sync_agents_md` (implementation-leaky), `hands_off` (informal).

---

### Decision 2: Behavior when `render = false`

| Condition | Behavior |
|-----------|----------|
| `render = false`, `AGENTS.md` exists | **Never modify** on sync or init (same outcome as marker, but manifest-driven) |
| `render = false`, `AGENTS.md` missing on **init** | **Do not create** managed content; write one-line placeholder *or* skip entirely (see Decision 5) |
| `render = false`, `AGENTS.md` missing on **sync** | Skip render; `sync-agent` root path already errors if missing — keep ERROR with guidance to create file manually or set `render = true` once |
| `render = true`, marker present | **Unchanged** — marker wins, file preserved (`--preserve-if-runtime-brief`) |
| `render = false`, marker absent | Skip render, preserve file if exists |

**Recommendation:** Treat `render = false` as a **hard skip** at the shell layer (`sync.sh`, `init.sh`, `sync-agent.sh`) *before* invoking `agents-render.py`. Do not add a second code path inside the renderer for the flag — keeps `agents-render.py` a pure "compose and write" tool.

---

### Decision 3: Precedence — flag vs marker vs per-section `_mode`

```
Precedence (highest wins first):
  1. [brief].render = false     → no write; fragments + [brief] prose ignored for output
  2. <!-- ai-specs:runtime-brief --> in file → no write (when render=true)
  3. Normal render              → fragments + [brief] with _mode append/replace
```

| `render` | Marker | Result |
|----------|--------|--------|
| false | absent | Skip; preserve existing or placeholder policy |
| false | present | Skip (redundant; doctor may note) |
| true | present | Skip (current contract) |
| true | absent | Render (current contract) |

**Coexistence:** Both mechanisms remain permanently. The marker is the **file-managed** opt-out; the flag is the **manifest-managed** opt-out. Document that flag is preferred for new adopters post-migration; marker remains for file-only workflows and backward compat.

**Per-section `_mode`:** Irrelevant when `render = false` (no merge happens). Doctor should not require `_mode` cleanup when render is disabled.

---

### Decision 4: Subrepos

**Current:** Subrepos receive a full `agents-render.py` pass without `--preserve-if-runtime-brief` (`sync-agent.sh:229-233`). Root manifest `[brief]` and root `resolved-config` are forwarded (`--resolved-config`).

**Recommendation:** Root manifest `[brief].render` **propagates to subrepo targets.**

- `sync-agent.sh` reads `SOURCE_ROOT/ai-specs/ai-specs.toml` (already does for TOML_PATH on fan-out).
- When `render = false`, subrepo `ensure_target_workspace()` skips `agents-render.py` but still mirrors skills/commands/gitignore.
- If subrepo `AGENTS.md` missing and `render = false`: same ERROR as root ("create manually or enable render").

**Non-goal:** Per-subrepo override (would need subrepo-local manifest — V1 subrepos don't have one).

---

### Decision 5: `init` behavior when `render = false`

**Options:**

A. **No file** — init does not create `AGENTS.md`; sync-agent later fails until user creates it.
B. **One-line placeholder** — `# AGENTS.md - Runtime context` (current failure fallback) with stderr note.
C. **Scaffold template** — copy a static `templates/AGENTS.md.manual.tmpl` once if missing.

**Recommendation: Option B for init only** — matches today's failure fallback (`init.sh:195`) so downstream steps (gitignore, lock, symlink creation) don't break on a fresh tree. Message:

```
· skipped AGENTS.md render (brief.render = false in manifest)
! created placeholder — replace with your manual brief
```

**`init --force`:** If `render = false` and user already has a real `AGENTS.md`, never overwrite (same as marker preservation semantics).

---

### Decision 6: Harness symlinks (CLAUDE.md, etc.)

**Current:** `sync-agent.sh` symlinks each agent's `instructions_path` → `AGENTS.md` (`make_relative_symlink`).

**When `render = false`:**

- Symlinks **still created** if `AGENTS.md` exists (manual or placeholder).
- If user maintains `CLAUDE.md` as a separate file instead of symlink, that's outside ai-specs scope (unchanged).

**Card #11 (`.omp/AGENTS.md`):** Out of scope. This change does not skip omp mirror writes. Note as follow-up: extend opt-out to per-harness mirrors or document that omp users need `.omp/AGENTS.md` manual + card #11.

---

### Decision 7: Doctor diagnostics

Add checks to `lib/_internal/doctor.py`:

| Check | Severity | Condition |
|-------|----------|-----------|
| `brief-render-disabled` | INFO | `[brief].render = false` — remind that sync will not update AGENTS.md |
| `brief-render-disabled-missing-file` | ERROR | `render = false` and no `AGENTS.md` |
| `brief-render-disabled-recipe-fragments` | WARN | `render = false` and any enabled recipe has non-empty `brief_fragments` in resolved-config — config is coherent but fragments are dead weight |
| `brief-render-disabled-with-marker` | INFO | Both flag false and marker present — redundant, harmless |

**Card requirement:** WARN when recipes have `[provides.brief]` but rendering disabled — implement as above.

---

### Decision 8: Observability

Sync/init stdout (match existing style):

```
▸ agents-render (root)
  · skipped AGENTS.md (brief.render = false)
```

Stderr on init placeholder path:

```
! AGENTS.md render disabled — placeholder written; add your manual brief
```

No change to exit codes (skip is success, same as marker skip).

---

### Implementation Shape

```
Files to change:
  ai-specs.toml schema / docs
    docs/ai-specs-toml.md          — document [brief].render (default true)
    templates/ai-specs.toml.tmpl   — optional commented example

  lib/_internal/agents-render.py
    — NO flag logic (skip at callers) OR optional early-exit helper read_manifest_render_flag()
    — keep --preserve-if-runtime-brief unchanged

  lib/sync.sh
    — read [brief].render before agents-render block
    — skip with message when false

  lib/init.sh
    — same guard before agents-render
  lib/sync-agent.sh
    — guard in ensure_target_workspace() using SOURCE_ROOT manifest
    — subrepo fan-out inherits via same ensure_target_workspace

  lib/_internal/doctor.py
    — new checks (Decision 7)

  tests/
    tests/test_agents_md_render_opt_out.py (new)
      — sync skip when render=false
      — init placeholder when render=false
      — subrepo skip
      — render=true unchanged (regression)
      — marker precedence when render=true
      — flag precedence when render=false (file untouched without marker)

  openspec/
    proposal.md, design.md, tasks.md, delta specs
    — runtime-brief-rendering/spec.md (new requirements)
    — recipe-manifest-contract/spec.md ([brief].render field)
```

**Helper option:** Small Python function in `toml-read` or shared module:

```python
def brief_render_enabled(manifest: dict) -> bool:
    brief = manifest.get("brief") or {}
    return brief.get("render", True) is not False  # only explicit false disables
```

Use strict boolean: non-boolean values → doctor WARN, treat as true (safe default) or render-time error (stricter — prefer doctor at sync time).

---

### Relationship to existing mechanisms

| Mechanism | Status after this change |
|-----------|-------------------------|
| `<!-- ai-specs:runtime-brief -->` | **Preserved** per `runtime-brief-rendering` spec non-goals |
| `[brief].<section>_mode = "replace"` | Unchanged; only applies when `render = true` |
| `recipe-brief-fragments` | Unchanged; fragments simply not merged when render=false |
| `/rules-audit` migration | **Primary consumer** — docs should recommend `[brief].render = false` instead of marker for manifest-first workflows |
| ai-specs-cli dogfood | Project may adopt `render = false` later to stop sync overwriting hand-curated `AGENTS.md` + partial `[brief]`; out of scope for apply |

---

### Key Risks

1. **User expects `[brief]` to still apply when `render = false`** — docs must be explicit: flag disables all writes, including manifest prose and recipe fragments.

2. **Subrepo surprise** — adopters with `subrepos = [...]` may not realize root flag silences subrepo renders too. Document in `docs/ai-specs-toml.md` § subrepos.

3. **Init placeholder vs empty** — placeholder file may be committed by mistake. Mitigation: doctor ERROR until user replaces placeholder content (heuristic: still one-line header) — optional, may be overkill for V1.

4. **Boolean coercion** — `render = True` invalid TOML; align with card #16 doctor check.

5. **sync-agent root error** — root sync-agent requires existing `AGENTS.md` even when render=false; init must create placeholder so first `sync` succeeds.

---

### Approach Classification

**Recommended:** Manifest-level `[brief].render` boolean (default `true`), enforced at shell callers, propagating to subrepos, with doctor WARN for dead recipe fragments, preserving marker escape hatch.

### Open Questions (for proposal/design)

1. **Strict vs loose parsing:** Should `render = "false"` (string) error or doctor-warn? Recommendation: doctor ERROR at `ai-specs sync` time via materialize validation.

2. **Placeholder detection in doctor:** Should one-line `# AGENTS.md - Runtime context` trigger WARN "replace with real brief"?

3. **Future CLI override:** `ai-specs sync --force-brief` to render once despite `render = false`? Defer to design; useful for debugging but not required for MVP.

4. **Engram / brief fragments in resolved-config:** When render=false, should `recipe-materialize.py` still embed `brief_fragments` in resolved-config (for tooling) or strip them? Recommendation: keep embedding (other tools may read); only skip write.

---

### Next Step

**Proposal** → `openspec/changes/agents-md-render-opt-out/proposal.md` with capability delta (`runtime-brief-rendering`, `recipe-manifest-contract`), then design matriz flag/marker/subrepos/init, then TDD tasks.
