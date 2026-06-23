# Exploration: Per-project CLI version pinning

## Problem statement

Users need to know **which ai-specs CLI version** each project was last synced with,
and optionally **enforce** that only a compatible CLI runs sync against that project.
Today only **document/recipe** versions are pinned; the **tool** is global and opaque.

## Current state (as of 0.12.2)

### What is versioned today

| Artifact | Mechanism | Enforced at sync? |
|----------|-----------|-------------------|
| Recipes | `[recipes.<id>].version` in manifest | Yes — mismatch fails |
| Deps | `[[deps]].version` optional | Metadata only |
| Bundled skills/commands | SHA-256 in `.ai-specs.lock` | Implicit via refresh-bundled |
| CLI itself | `VERSION` file in global checkout | No per-project link |

### Lock file today

`ai-specs/.ai-specs.lock` tracks content hashes for skills, commands, recipes, deps,
agents, and opted-out files. **No `[meta]` section** records CLI identity or sync time.

Relevant code: `lib/_internal/lock.py`, `lib/_internal/refresh-bundled.py`.

### Doctor today

`lib/_internal/doctor.py` validates manifest, AGENTS.md, agents, bundled assets, MCP,
recipes, deps. **No CLI version check.**

Spec: `openspec/specs/project-doctor/spec.md`.

### Sync today

`lib/sync.sh` pipeline: target resolve → gitignore → refresh-bundled → vendor →
recipe materialize → agents render → fan-out. **No CLI version pre-flight.**

### Upgrade today

`ai-specs upgrade` updates the global checkout and prints old→new version diff.
Spec: `openspec/specs/upgrade-command/spec.md`. Upgrade is global-only; projects are
not notified or re-validated.

## User scenarios

### S1 — Fleet visibility (venturi_coffee)

Operator opens an old project, runs `ai-specs doctor`, sees:

```
OK    cli-version     installed 0.12.2, last sync 0.10.1 (2026-02-14)
WARN  cli-version     no [tool] pin — consider pinning before production deploy
```

No sync required for diagnosis.

### S2 — Production pin

Project declares:

```toml
[tool]
version = "0.12.2"
policy = "exact"
```

Developer on 0.11.0 runs `ai-specs sync` → fails before any write:

```
ERROR: CLI version 0.11.0 does not match pinned 0.12.2.
Run: ai-specs upgrade
```

### S3 — Gradual adoption (min policy)

Legacy project sets `min_version = "0.11.0"` while team upgrades globally.
Sync succeeds on 0.12.2; fails on 0.10.x.

### S4 — Fresh init

`ai-specs init` on a greenfield project optionally writes:

```toml
[tool]
version = "0.12.2"
policy = "exact"
```

using the running CLI version — opt-in via flag or default-on (design decision).

## Design constraints

1. **Backward compatible** — absent `[tool]`, no sync failure; optional WARN in doctor.
2. **Same semver rules** as recipe pins — reuse patterns from `skill_contract.VERSION_RE`
   where practical.
3. **Lock meta is derived** — written by CLI, not hand-edited; `[meta]` ignored on read
   for hash decisions.
4. **Single source for installed version** — read `AI_SPECS_HOME/VERSION` (same as
   `lib/version.sh`).
5. **Sync gate runs before writes** — align with recipe version check ordering in
   `recipe-materialize.py`.

## Alternatives considered

| Alternative | Verdict |
|-------------|---------|
| Pin only in lock (no manifest) | Visibility yes, enforcement no — insufficient |
| Pin in `AGENTS.md` comment | Derived artifact, not source of truth |
| Require local `AI_SPECS_HOME` per project | High friction; document as advanced pattern only |
| `ai-specs migrate` in MVP | Deferred — needs CHANGELOG + breaking-change catalog |

## Open questions (resolved in design.md)

1. Default on init: seed `[tool]` or leave empty? → **Leave empty; document opt-in pin**
2. Doctor ERROR vs WARN when unpinned but stale last-sync? → **WARN only**
3. Sync failure code when pin mismatch? → **Exit 1, same as recipe version mismatch**

## Classification

**domain_change** — new manifest surface, lock schema extension, cross-cutting sync/doctor
behavior, migration documentation. Requires full SDD artifact set.
