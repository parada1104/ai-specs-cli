## ADDED Requirements

### Requirement: Shared CLI helpers stay in AI_SPECS_HOME

Shared recipe helpers that are identical for every project (for example
`premerge_guardian.py`) SHALL live under `$AI_SPECS_HOME` (typically
`~/.ai-specs`). Sync MUST NOT stage those helpers into the project tree under
`ai-specs/bin/` or under the per-project cache.

#### Scenario: No in-project premerge helper

- GIVEN an enabled recipe that previously staged `ai-specs/bin/premerge_guardian.py`
- WHEN sync completes
- THEN the project MUST NOT require `ai-specs/bin/premerge_guardian.py`
- AND agents invoke the helper from `$AI_SPECS_HOME/lib/_internal/premerge_guardian.py`

### Requirement: Leftover skill-cache and staged-bin cleanup

Sync SHALL delete leftover in-project skill-cache directories
`ai-specs/.resolved-skills/` and `ai-specs/.internal/` when present, in addition
to `ai-specs/.recipe` and `ai-specs/.deps`. Sync SHALL also remove a stale
managed `ai-specs/bin/premerge_guardian.py` and an empty `ai-specs/bin/`
directory left from older CLI versions.

#### Scenario: Legacy resolved-skills leftover rm

- GIVEN leftover `ai-specs/.resolved-skills/` and/or `ai-specs/.internal/`
- WHEN sync runs
- THEN those trees are deleted from the project

#### Scenario: Stale bin helper leftover rm

- GIVEN leftover `ai-specs/bin/premerge_guardian.py`
- WHEN sync runs
- THEN that file is deleted
- AND `ai-specs/bin/` is removed when empty

### Requirement: Root agent gitignore refresh on sync

Sync SHALL refresh the managed agent-generated block in the project root
`.gitignore` from `templates/gitignore-root.tmpl` whenever the block markers
are present (or append the block when missing), so existing projects pick up
new agent dirs such as `.pi/` and `.omp/` without re-running init.

#### Scenario: Sync adds missing .pi ignore

- GIVEN a root `.gitignore` whose managed agent block lacks `.pi/`
- WHEN sync runs
- THEN the managed block matches the current template
- AND `.pi/` is ignored

## MODIFIED Requirements

### Requirement: Cache layout and project surface split

The cache SHALL contain `.recipe`, `.deps`, managed command staging, and
`resolved-skills`. The project SHALL retain `ai-specs.toml`, `ai-specs/skills/`,
and `ai-specs/recipes/` (docs, hooks, `not_exists` templates that are
project-specific). Origin skills, deps, managed commands, resolved-skills, and
shared CLI helpers MUST NOT remain required under the project tree.

#### Scenario: Surface split

- GIVEN an enabled recipe
- WHEN sync completes
- THEN skills, deps, managed commands, and resolved-skills live under the cache
- AND docs and hooks remain under `ai-specs/recipes/`
- AND `ai-specs/skills/` contains only local user skills
- AND shared helpers are invoked from `$AI_SPECS_HOME`, not from `ai-specs/bin/`
