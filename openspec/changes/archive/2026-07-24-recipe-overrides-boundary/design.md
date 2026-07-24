# Design: complete D2 recipe overrides-boundary migration (trello-mcp-workflow, worktree-flow)

## Problem restated in one line

Two recipes declare `[[provides.templates]]` with `condition = "not_exists"`
targeting bare `ai-specs/recipes/{id}/templates|bin/` paths — paths the shipped
D2 `.gitignore` rule ignores — so once materialized they can never be committed,
defeating the whole point of a `not_exists` (project-owned, hand-editable)
template. The fix is a pure declaration relocation into the already-un-ignored
`overrides/` surface plus the content references that name those paths.

## Governance model (this is the rule the change enforces)

The D2 boundary shipped by `2026-07-23-minimal-project-materialization` splits
`ai-specs/recipes/` into exactly two zones:

| Content under `ai-specs/recipes/{id}/` | Git status | Who owns it | How it is declared |
|---|---|---|---|
| `overrides/**` | committed (negated back in) | project | `[[provides.templates]]` with `condition = "not_exists"`, `[[provides.docs]]` a project is meant to edit, `overrides/config.toml`, `overrides/templates/` |
| everything else (`README.md`, `templates/` at recipe root, `bin/`, hooks, default docs) | gitignored | CLI / recipe (cache-resolved) | bundled content, regenerated every sync |

The invariant this change makes true: **any `[[provides.templates]]` entry with
`condition = "not_exists"` MUST target a path under
`ai-specs/recipes/{id}/overrides/`.** A `not_exists` template is by definition
"write once, then the project owns it" — that is only coherent if the file is
committable. A bundled/regenerable template would use no `condition` (or the
doc channel) and belongs in the gitignored zone. The two offending recipes
violated this because their declarations predate the D2 boundary.

## D1 — target shape (confirmed: `overrides/templates/` and `overrides/bin/`)

Accepted per proposal recommendation. The relocation preserves the existing
subdirectory name and simply nests it under `overrides/`:

- `templates/card-*.md` → `overrides/templates/card-*.md`
- `bin/worktree-cleanup.sh` → `overrides/bin/worktree-cleanup.sh`

Rationale: `overrides/templates/` is already the established, spec'd shape for
skill-provided override templates (`recipe-overrides-runtime` → "Override
template loading" reads `ai-specs/recipes/{id}/overrides/templates/`). Reusing
it means the recipe-provided card templates land in the same directory a human
already knows to look in for customization, and the `.gitignore` negation
(`!recipes/*/overrides/**`) already re-includes the whole subtree — no new
negation pattern is needed. `overrides/bin/` is the natural sibling for an
executable; nothing in the boundary rule is subdirectory-name-specific, so no
alternative shape improves on this. No other option was viable.

## Exact recipe.toml rewrites

### `catalog/recipes/trello-mcp-workflow/recipe.toml` — 6 target rewrites

Only the `target` value of each of the 6 `[[provides.templates]]` blocks
(lines 103–131) changes. `source`, `condition`, and every other field are
untouched. The `[[provides.docs]]` README block (lines 133–136) is **not**
touched (see "Docs channel left alone").

| `source` (unchanged) | old `target` | new `target` |
|---|---|---|
| `templates/card-feature.md` | `ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md` | `ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md` |
| `templates/card-bug.md` | `…/templates/card-bug.md` | `…/overrides/templates/card-bug.md` |
| `templates/card-spike.md` | `…/templates/card-spike.md` | `…/overrides/templates/card-spike.md` |
| `templates/card-epic.md` | `…/templates/card-epic.md` | `…/overrides/templates/card-epic.md` |
| `templates/card-handoff.md` | `…/templates/card-handoff.md` | `…/overrides/templates/card-handoff.md` |
| `templates/card-decision.md` | `…/templates/card-decision.md` | `…/overrides/templates/card-decision.md` |

(Each `…` expands to `ai-specs/recipes/trello-mcp-workflow`.)

### `catalog/recipes/worktree-flow/recipe.toml` — 1 target rewrite

Only the `target` of the single `[[provides.templates]]` block (lines 84–87):

| `source` (unchanged) | old `target` | new `target` |
|---|---|---|
| `templates/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |

Note the `source` stays `templates/worktree-cleanup.sh` (the file's location in
the *catalog*, which is unchanged); only the materialized `target` moves under
`overrides/bin/`.

## Exact hardcoded path-reference substitutions

Recipe content (READMEs, commands, skills) hardcodes the materialized paths in
prose and shell snippets. Every occurrence must move in lockstep with the
`target` rewrites or the docs will point users at now-gitignored, stale paths.
Grep-confirmed set — **9 occurrences across 7 files**:

| File | Line(s) | Old substring | New substring |
|---|---|---|---|
| `catalog/recipes/worktree-flow/README.md` | 30 | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/worktree-flow/commands/worktree-clean.md` | 18, 30 | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | 71 | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` | 187 | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md` | 182 | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md` | 193 | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` | 133 | `ai-specs/recipes/trello-mcp-workflow/templates/` | `ai-specs/recipes/trello-mcp-workflow/overrides/templates/` |
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` | 250 | `ai-specs/recipes/trello-mcp-workflow/templates/` | `ai-specs/recipes/trello-mcp-workflow/overrides/templates/` |

Substitution rule for the apply phase: it is a literal string replace of the
old path prefix with the `overrides/`-nested prefix. The trello references name
a directory (trailing `/`), the worktree references name the full script path;
both are covered by the same "insert `overrides/` after `{id}/`" transform.
The cross-recipe references (gitlab/bitbucket/git-pr merge-workflow skills)
invoke worktree-flow's cleanup script by absolute project path, so they must
track the worktree-flow relocation even though they live in other recipes —
this is why the affected-file set is wider than the two recipes being
relocated. Apply MUST re-grep after editing to confirm zero remaining
`recipes/worktree-flow/bin/` or `recipes/trello-mcp-workflow/templates/`
(non-`overrides`) occurrences catalog-wide.

## `materialize_template()` needs zero code changes (confirmed)

`lib/_internal/recipe-materialize.py::materialize_template()` (lines 318–329)
writes literally to `project_root / tpl.target`:

```python
def materialize_template(recipe_dir, tpl, project_root):
    src = recipe_dir / tpl.source
    dest = project_root / tpl.target          # ← literal target, no rewriting
    if tpl.condition == "not_exists" and dest.exists():
        print(...); return
    dest.parent.mkdir(parents=True, exist_ok=True)   # creates overrides/templates/ or overrides/bin/ automatically
    shutil.copy2(src, dest)
```

The function does no path interpretation, no `overrides/` awareness, no
allow-listing — `dest.parent.mkdir(parents=True)` already creates whatever
depth the new `target` implies. Changing the `.toml` `target` string is
sufficient and complete; there is nothing in the materializer to change. This
is why the change is confined to declarations + content, with **zero
production Python edits**. (Same holds for `materialize_doc()`, which we don't
touch anyway.)

## Docs channel left alone (and the pre-existing README inconsistency)

`[[provides.docs]]` READMEs for both recipes keep their current bare
`ai-specs/recipes/{id}/README.md` targets. `materialize_doc()` (lines 332–339)
ignores `condition` and always overwrites — READMEs are regenerable bundled
content and correctly live in the gitignored zone. trello-mcp-workflow's
README block declares `condition = "not_exists"` (line 136) which
`materialize_doc` silently ignores; that is a harmless, pre-existing
declaration/behavior mismatch **out of scope** for this change. Design note
only: if a later cleanup wants consistency, drop the dead `condition` from the
docs block — but doing so here would widen the diff without changing behavior,
so we leave it.

`skill-resolution.py` (`_overrides_dir()`, `resolve_skill_template()`,
`load_skill_config()`) is the *skill*-provided override loader and already
resolves `overrides/` correctly; it is unrelated to recipe-provided card
templates and is not touched.

## D2 — backward compatibility: Option A (no migration, just document)

Accepted per proposal recommendation. Consumer projects that ran
`ai-specs sync` before this change already have files materialized at the OLD
paths:

- `ai-specs/recipes/trello-mcp-workflow/templates/card-*.md` (up to 6 files)
- `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` (1 file)

After this change, sync's `not_exists` check looks at the NEW `overrides/`
paths (which don't exist yet), so it materializes fresh copies there. The old
files are left in place, gitignored, never read again by any code path.

**Why Option A over B/C.** The old files are already gitignored — invisible to
git, invisible to `doctor`'s tracked-file checks, and read by nothing (the
content that referenced them now points at `overrides/`). They are inert. A
migrator (Option B, mirroring `remove_bundled_*_leftovers`) or a doctor WARN
(Option C) would add real production code + tests to sweep files that cause no
functional harm — cost with no correctness payoff for v1. If leftover clutter
ever becomes a support burden, Option C is the clean follow-up (design sketch
retained in the proposal's D3), and its detection would mirror
`_tracked_bundled_leftovers()` in `project-cache.py` against the two old path
prefixes. We explicitly do NOT build that now.

### What "document" means concretely (minimal)

Two low-cost touchpoints, both text-only, no code:

1. **CHANGELOG `[Unreleased]` entry** (the one required artifact). Under a
   `### Fixed` heading, note the boundary fix and the one-line consumer
   cleanup hint. Draft:

   > - **Recipe override boundary completed for trello/worktree templates**:
   >   `trello-mcp-workflow` card templates and the `worktree-flow` cleanup
   >   script now materialize under `ai-specs/recipes/{id}/overrides/` so they
   >   are committable (previously written to gitignored `templates/`/`bin/`
   >   paths). Projects synced before this release may delete the orphaned
   >   old-path copies (`ai-specs/recipes/trello-mcp-workflow/templates/`,
   >   `ai-specs/recipes/worktree-flow/bin/`); they are gitignored and no
   >   longer used.

2. **No doctor change, no README migration section.** The CHANGELOG line is
   the single canonical place a human upgrading will look. Adding a
   doctor-adjacent mention was considered and rejected as scope creep for
   Option A — doctor stays silent because the files are gitignored and
   therefore outside every doctor check's remit. (This is a deliberate D2
   consequence, restated here so the apply phase does not "helpfully" add a
   warning.)

The tasks phase will carry exactly one documentation task: the CHANGELOG
entry above. That is the entire realization of "document."

## Test strategy — regression via `git check-ignore`

Add one regression test that pins the boundary invariant using the same
mechanism as the existing
`InitExternalDirsTests::test_gitignore_ignores_recipes_except_overrides`
(`tests/test_external_dirs.py:84`). That test already proves the *generic*
rule (a `overrides/` file is committable, a bundled-doc sibling is ignored);
the new test proves the rule holds for the *specific relocated targets* this
change introduces, so a future regression that moves a target back out of
`overrides/` fails loudly.

Placement: extend `tests/test_external_dirs.py`, same `InitExternalDirsTests`
class, reusing the established fixture recipe:
`git init` + `ai-specs init --no-tui` a temp project, then drive the exact
`git check-ignore -q <rel>` helper the sibling test defines.

New test `test_gitignore_committable_relocated_recipe_templates` asserts, using
the concrete post-change target paths:

- `ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md`
  is **NOT** ignored (committable) — representative of the 6 card templates.
- `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`
  is **NOT** ignored (committable).
- The OLD bare paths
  `ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md` and
  `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`
  **ARE** ignored (un-committable) — this negative assertion is what makes the
  test a genuine boundary guard rather than a tautology: it demonstrates the
  relocation crossed a real ignore boundary.

Fixture detail (mirror the sibling test exactly): create the files on disk
under the temp project before probing, because `git check-ignore` matches path
patterns whether or not the file exists, but creating them keeps the test
faithful to the materialized layout and guards against a future
directory-vs-file negation subtlety. Use the same inline `ignored(rel)` helper
(returns `r.returncode == 0`); no new dependency, no CLI invocation beyond the
`init` already used by the class.

Determinism/isolation: fully self-contained in a `TemporaryDirectory`, no
network, no shared state — matches the existing suite's conventions and is
safe under the full `./tests/validate.sh` run.

**Not tested (deliberately):** actual materialization output (that
`materialize_template` writes to the new path) is already covered generically
by the recipe-materialize tests and needs no per-target duplication, since we
made zero materializer code changes — the `target` string is data, and the
`git check-ignore` test is the meaningful new contract. A `.toml`-parse
assertion that the 7 targets literally contain `/overrides/` MAY be added as a
cheap catalog-lint guard if the spec phase wants belt-and-suspenders, but the
ignore-boundary test is the primary regression gate.

## Rollout / verification checklist (for apply + verify phases)

1. Rewrite the 7 `target` values (6 trello + 1 worktree).
2. Apply the 9 content substitutions across 7 files.
3. Re-grep catalog for residual non-`overrides` old paths → must be zero.
4. Add the `git check-ignore` regression test.
5. Add the CHANGELOG `[Unreleased]` entry.
6. `./tests/validate.sh` green.

## Rollback

Fully additive/reversible text change: revert the `target` rewrites, the 9
content substitutions, the test, and the CHANGELOG line. No data migration, no
git-history rewrite. Consumers that synced against the new targets keep files
at `overrides/` paths; reverting simply orphans those instead (symmetric with
the Option A leftover situation, zero data loss either direction).
