# Design: Recipe tags/categories + conflicts_with

## Context

The spike (current branch diff) implemented the schema fields and the
`check_tag_conflicts` detector, then wired a **blocking** (`return 1`) tag-conflict
check into `recipe-materialize.py` and declared
`conflicts_with = ["git-pr-flow", "gitlab-mr-flow"]` on `bitbucket-pr-flow`.

Running the full suite revealed a regression: two tests in
`BitbucketPrFlowDualProviderTests` failed because they enable **both**
`git-pr-flow` and `bitbucket-pr-flow` with an explicit `[[bindings]]` entry for
the `vcs-pr-flow` capability — a supported scenario. The blocking conflict
aborted sync before materialization.

## Decision 1 — Tag conflicts are advisory; sync never blocks on them

`_spec-tags.md` states tags are "purely advisory metadata — they don't affect
materialization" and that "`ai-specs sync` warns on stdout if conflicting recipes
are enabled." The capability layer (`check_capability_conflicts`) already owns the
*blocking* decision about which provider serves a capability, resolved by
`[[bindings]]`.

Therefore sync surfaces tag conflicts as **warnings only** and never changes its
exit code or skips materialization because of them. Both warning- and
fatal-severity tag conflicts are printed; fatal ones get a stronger message, but
neither aborts sync.

Rationale: blocking on tags would duplicate (and contradict) the capability-binding
authority and break the dual-provider scenario. The `fatal` severity remains
meaningful at the library/reporting level (e.g. a future `doctor` section) without
gating materialization.

## Decision 2 — Catalog VCS recipes do NOT declare `conflicts_with` each other

`git-pr-flow`, `bitbucket-pr-flow`, and `gitlab-mr-flow` are alternative providers
of the same capability, resolved by binding — not mutually exclusive installs.
They share the `vcs` tag, which yields a **warning** ("two VCS flows enabled; bind
one"). That is the correct, non-fatal signal.

`conflicts_with` stays in the schema as a tool for recipe authors to declare a
*genuine* incompatibility (e.g. two recipes that would corrupt each other's
state). No catalog recipe needs it today, so none declares it.

## Decision 3 — Parsing lives on the `[recipe]` table

`tags` and `conflicts_with` are recipe identity/metadata, parsed alongside
`id`/`name`/`version` in `validate_recipe_toml`, not under `[provides]`. Both
default to `[]`. Validation:
- `tags`: array; every element a string.
- `conflicts_with`: array; every element a string; no self-reference.

## Detection algorithm (`check_tag_conflicts`)

Group enabled recipes by tag. For each tag shared by ≥2 recipes, emit one
`TagConflict{tag, recipes(set of ids), severity}`. Severity is `fatal` if any
recipe in the group lists another group member in its `conflicts_with` (treated
**symmetrically** — one side declaring it is enough); otherwise `warning`.

`TagConflict.to_dict()` → `{"type": "tag_conflict", "tag": ..., "recipes": sorted}`
matching the spec output format.

## Flow

```
ai-specs sync
  └─ check_capability_conflicts  → fatal (blocks) | warning
  └─ check_tag_conflicts         → warning only (advisory, never blocks)  ← NEW
  └─ check_conflicts (primitives)→ fatal (blocks)
  └─ materialize …
```

## Test impact

- 14 new RED tests (schema fields/validation + `check_tag_conflicts`) → GREEN.
- `BitbucketPrFlowDualProviderTests` (2) → restored to GREEN once Decisions 1 & 2
  are applied (remove `conflicts_with` from bitbucket; sync warn-only).
