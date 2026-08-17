# Exploration: dated OpenSpec archive resolution

> Change slug: `premerge-guardian-dated-openspec`
> Follow-up: GitHub issue #216
> Tracker: Trello card #79
> Worktree: `change/premerge-guardian-dated-openspec`

## Explore decision

`Explore: required` because the live `plan-build-flow` skill and its canonical
spec currently describe an undated archive path, while the OpenSpec provider
creates dated archive directories. This is also a retry/follow-up after issue
#216, so the existing behavior and its compatibility boundary must be made
explicit before implementation.

## Problem

The pre-merge guardian currently resolves only:

```text
openspec/changes/archive/<slug>/
```

The canonical OpenSpec archive convention is:

```text
openspec/changes/archive/YYYY-MM-DD-<slug>/
```

As a result, a review branch can contain a complete, valid OpenSpec archive and
still be blocked before merge. The live recipe and canonical spec repeat the
same undated assumption, so changing only the helper would leave the provider
contract contradictory.

## Grounded findings

| Surface | Finding | Consequence |
|---|---|---|
| `lib/_internal/premerge_guardian.py` | `check_premerge` constructs exactly `archive/<slug>` | Canonical dated archives are reported missing |
| `tests/test_premerge_guardian.py` | The worktree already contains the provided regression cases for dated pass, legacy fallback, dated ambiguity, mixed ambiguity, invalid date, and active-folder behavior | Treat this pre-existing change as RED context; do not rewrite the file during planning |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Archive-tail and guardian instructions use `archive/<slug>` | Agents are instructed to produce/expect the wrong provider path |
| `catalog/recipes/plan-build-flow/recipe.toml` | The generated workflow rule names archive-tail but not the provider-specific path | Generated runtime guidance does not state the dated contract |
| `catalog/recipes/plan-build-flow/README.md` and `docs/recipes-catalog.md` | The live documentation says archive before merge but does not state the dated OpenSpec layout | Reviewers and users cannot distinguish the canonical path from the legacy fallback |
| `openspec/specs/plan-build-flow/spec.md` | Archive scenarios and guardian blockers normatively require `archive/<slug>` | The canonical contract conflicts with the provider behavior |

## Candidate approaches

### A. Provider-aware exact resolver (selected)

Resolve one exact canonical dated directory first, preserve one exact undated
directory as a legacy fallback, and fail closed for ambiguity or malformed
candidate names. Keep artifact minima, verification evidence, active-folder,
and root-resolution checks unchanged.

**Why selected:** It fixes the provider mismatch without renaming historical
archives, preserves already-landed undated archives, and makes unsafe archive
selection impossible.

### B. Rename existing archives to the undated path

Normalize current dated OpenSpec archives so the helper can keep its existing
lookup.

**Rejected:** It violates the canonical OpenSpec provider contract and rewrites
the historical archive boundary that this follow-up is meant to respect.

### C. Accept any directory containing the slug

Use a broad glob or substring match and select the first matching directory.

**Rejected:** It accepts near-match names, makes selection order observable, and
can silently inspect the wrong change. A pre-merge gate must fail closed.

## Resolution contract to carry into the spec

For the requested slug, the resolver will inspect direct child directories of
`openspec/changes/archive/` and accept only these exact forms:

1. `YYYY-MM-DD-<slug>` where the prefix is a valid ISO calendar date and the
   suffix is exactly `<slug>`.
2. `<slug>` as a legacy compatibility form, only when no dated candidate exists.

The resolver must:

- return the single valid dated candidate when exactly one exists;
- return the exact undated candidate only when no dated candidate exists;
- block when two or more dated candidates exist;
- block when a dated and an undated candidate both exist;
- block when a candidate-shaped name has an invalid calendar-date prefix;
- block rather than select a near-match name or use substring/glob matching;
- continue to ignore unrelated archive directories that do not represent a
  candidate for the requested slug;
- run all existing artifact and verification checks against the resolved path;
- retain the active-folder blocker even when a valid archive is present.

Historical archive directories are read-only compatibility data. This change
does not rename, move, or rewrite any directory under
`openspec/changes/archive/`.

## Open questions

None. The provider path, compatibility behavior, fail-closed ambiguity policy,
and documentation surfaces are fixed by the request and the existing staged
regression context.
