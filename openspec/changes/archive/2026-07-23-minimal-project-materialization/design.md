# Design: minimal project materialization

## Governance model

The decision axis is **ownership**, expressed as "declared in the project's
`ai-specs.toml`?" — not "committed to git?". Materialization (bytes on disk) is
an independent axis.

| Content | Declared by | Materialized at | Git | Rationale |
|---|---|---|---|---|
| Local skills (`release-flow`, `testing-foundation`) | project (`[[skills]]`/local dir) | `ai-specs/skills/` | committed | authored in-repo; git is the source |
| toml-deps (`add-dep` → `[[deps]]`) | project (toml) | `ai-specs/.deps/` (D1) | gitignored | project-governed but regenerable from git source |
| recipe-deps (recipe vendors a skill) | recipe → CLI | `{cache}/.deps/` | outside repo | transitive, CLI-owned |
| CLI-bundled skills (harness-*, skill-creator, skill-sync) | CLI | cache flatten from `$AI_SPECS_HOME/bundled-skills/` | outside repo | ships with the CLI; identical for every project |
| Recipe docs/hooks/templates (bundled) | recipe → CLI | cache | outside repo | regenerable from catalog |
| Recipe overrides | project | `ai-specs/recipes/{id}/overrides/` | committed | genuine per-project customization |

Committed project surface after this change: `ai-specs.toml`, `.ai-specs.lock`
(meta only), `ai-specs/skills/` (local only), `ai-specs/commands/` (hand-authored
only), `ai-specs/recipes/{id}/overrides/`, `ai-specs/contracts/`.

## Skill source precedence (four tiers)

Extend the current three-tier resolver with a CLI-bundled tier at the bottom:

1. `ai-specs/skills/{id}/` — local (highest)
2. `{cache}/.recipe/{recipe-id}/skills/{id}/` — recipe-bundled
3. `{cache}/.deps/{dep-id}/skills/{id}/` — vendored dep
4. `{cache}/.bundled/skills/{id}/` — **CLI-bundled (new, lowest)**, flattened
   from `$AI_SPECS_HOME/bundled-skills/`

Precedence stays source-level (whole-directory, no file merge). A local skill of
the same id still shadows a bundled one — this preserves the current
dogfooding case where this repo authors `skill-creator`/`skill-sync` locally
while consumer projects get them from tier 4.

## Why `refresh-bundled` goes away (or flattens)

`refresh-bundled`'s three behaviors — auto-update untouched, `<name>.new` for
customized, respect deletions — all serve **in-project user edits**. Removing
user modification removes their reason to exist. Two options (D3):

- **Remove:** delete `refresh-bundled.sh` + `refresh-bundled.py`; sync resolves
  bundled skills from the cache flatten directly. Simplest; smallest surface.
- **Flatten-only:** keep a `refresh-bundled` verb that only (re)flattens
  `$AI_SPECS_HOME/bundled-skills/` into `{cache}/.bundled/`, never touches the
  project, never writes `.new`. Keeps a named entry point for cache repair.

Recommendation: **flatten-only**, so `doctor`/cache-repair has a verb, but with
zero in-project writes.

## Lock evaluation

Current `.ai-specs.lock` = `[meta]` + per-file SHA-256 for every bundled/recipe
`SKILL.md`. The hashes exist **only** to drive `refresh-bundled`'s edit
detection. Trace:

- Bundled/recipe skills leave the project → their files aren't there to hash.
- Local skills + overrides stay → but they're git-tracked; git already gives
  integrity + diff. Hashing them in a committed lock is redundant with git.
- Resolution inputs → recipes are CLI-bound (no version pins since 0.14.0), so
  "what resolves" is fully determined by `cli_version`.

Residual non-redundant value = **`cli_version` + `synced_at`**: the only
CLI-provenance signal that travels with a fresh clone (the cache `meta.toml` is
machine-local and absent on clone). `doctor`/`upgrade` read it for version-drift.

**Decision:** lock collapses to `[meta]`. Drop `[skills.*]` / `[recipes.*]`
hash sections. `cli_version.py stamp-meta` becomes the whole writer;
`skills-add`/`skills-remove`/`recipe-remove`/`init` stop writing hash entries.

## Migration (existing projects)

Sync is the migration vehicle (matches `project-recipe-cache` leftover-cleanup
precedent):

1. Flatten CLI-bundled skills into `{cache}/.bundled/`.
2. Delete leftover bundled skills from `ai-specs/skills/` (harness-*,
   skill-creator, skill-sync) — but only when a local skill of that id is NOT
   authored in-repo (protects the dogfooding repo's own authored copies).
3. Migrate toml-deps into `ai-specs/.deps/` (gitignored) if D1 = in-project.
4. Gitignore `ai-specs/recipes/` with `!overrides/` negations; leave existing
   committed overrides in place.
5. Rewrite `.ai-specs.lock` to meta-only; drop stale hash sections.
6. Refresh root/`ai-specs` `.gitignore` managed blocks from templates.

Idempotent: a second sync is a no-op. The dogfooding-repo guard in step 2 is the
sharp edge — needs an explicit "is this id a locally-authored skill vs a
materialized bundled copy?" signal (candidate: presence in `bundled-skills/` at
the CLI home AND absence of a local `[[skills]]` declaration).

## Risks

- **Dogfooding self-collision** (this repo IS the CLI): the guard in migration
  step 2 must not delete this repo's authored `bundled-skills/` sources or its
  local mirrors. Covered by test.
- **Consumer upgrade churn**: first sync after upgrade deletes committed bundled
  skills → a large, surprising diff. Mitigate with a clear sync summary line
  listing removed paths.
- **Override path ambiguity** (D2): if we require `overrides/`, trello templates
  currently at `recipes/trello-mcp-workflow/templates/` need relocation +
  migration.
