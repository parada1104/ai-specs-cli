# Archive report — relocate-bundled-commands

**Archived:** 2026-07-24
**Branch:** `change/relocate-bundled-commands`
**Status:** verified, judgment-day approved, archived

## Outcome

Apply the exact same governance model already shipped for skills
(`minimal-project-materialization`, archived
`openspec/changes/archive/2026-07-23-minimal-project-materialization/`) to
CLI-bundled COMMANDS:

- CLI-bundled commands (`rules-audit.md`, `skills-as-rules.md`) now resolve
  from `{cache}/.bundled/commands/` (a new, dedicated tier, sibling to
  `{cache}/.bundled/skills/`) instead of materializing into the committed
  `ai-specs/commands/` directory.
- `merge_commands()` becomes a 3-tier precedence: `ai-specs/commands/` (local,
  highest) > `{cache}/commands/` (recipe-managed) >
  `{cache}/.bundled/commands/` (CLI-bundled, lowest). Recipe-managed silently
  overrides bundled (both CLI-driven tiers); local hand-authored still warns
  on collision with either lower tier.
- `ai-specs/.ai-specs.lock` drops its last non-`[meta]`/`[agents.*]`
  remnant — `[commands]` / `[opted-out]` — becoming a pure CLI-provenance
  stamp, mirroring what the skills migration already did for
  `[skills.*]`/`[recipes.*]`.
- `doctor` gained a per-bundled-command-id OK/ERROR check (replacing the old
  aggregate "any commands present" check, mirroring the existing
  per-bundled-skill check) plus a tracked-bundled-command-leftover WARN
  (`git rm --cached` guidance, git index left untouched by the CLI).
- Dead code removed: `lib/_internal/command-merge.py` (superseded by
  `project-cache.py`'s `merge_commands`, confirmed zero remaining callers
  before and after deletion) and `refresh-bundled.py`'s content-hash /
  `.new`-sidecar loop for commands — `refresh-bundled` is now flatten-only
  for both bundled asset kinds, with zero in-project writes and zero lock
  interaction.
- This repo's own dogfooded `ai-specs/` project was live-migrated during
  verify: real legacy state (`cli_version = "0.15.0"`, `[commands]` /
  `[opted-out]` sections, two committed byte-identical bundled-command
  copies) was run through `./bin/ai-specs sync .` and `./bin/ai-specs
  doctor .` end-to-end — lock trimmed to `[meta]`-only, bundled-command
  leftovers removed from disk, doctor's WARN + exact `git rm --cached`
  guidance reproduced, guidance applied, doctor clean afterward. Not a
  synthetic `tempfile.mkdtemp()` fixture.

## Specs synced

| Domain | Action |
|--------|--------|
| `skill-source-precedence` | Updated — `Command merge` requirement rewritten for the 3-tier bundled/recipe/local precedence, with bundled-resolution and both shadow scenarios |
| `sync-lock` | Updated — `Lock is a provenance stamp` requirement drops the `[commands]`/`[opted-out]` carve-out; lock is now `[meta]` + `[agents.*]` only |
| `project-doctor` | Updated — `Bundled asset diagnostics` requirement gains per-bundled-command present/missing scenarios; new `Tracked bundled-command leftover guidance` requirement added |

## Verification

- Verify **PASS** (no CRITICAL/blocking WARNING): `verify-report.md`
- Judgment Day **APPROVED** — dual blind adversarial review. `jd-judge-a` ran
  as configured; `jd-judge-b`'s configured model hit a provider infrastructure
  outage (repeated `400 Upstream request failed`, confirmed via HTTP-400
  request logs, not a content rejection) and was substituted with the
  general-purpose `reviewer` agent for the second pass, given the identical
  blind-review brief and zero visibility into Judge A's output. All 5
  independently confirmed findings (1 WARNING, 3 SUGGESTION, 1 INFO — a stale
  lock-description comment, a dead `init_mode` parameter, a variable shadowing
  a module-level function, a missing lock-migration test case, and a stale
  apply-progress risk note) were fixed and re-verified green.
- `./tests/validate.sh` — **1045 tests, exit 0** (run independently after the
  spec-promotion + directory move in this archive step; markdown-only changes
  under `openspec/` do not touch any test fixture, all of which use
  `tempfile.mkdtemp()`-based tmpdirs).

## Follow-ups (not this change)

1. `doctor`'s per-bundled-command check id is plural (`"bundled-commands"`)
   rather than singular (`"bundled-command"`) like the skill side's
   `"bundled-skill"`. This was required during implementation to avoid
   breaking a pre-existing substring-filtering test. Cosmetic only, no
   functional or spec impact — noted by judgment-day as an optional future
   rename (would need the one coupled test updated in the same commit for
   exact symmetry).

## Archive move

- Source: `openspec/changes/relocate-bundled-commands/`
- Destination: `openspec/changes/archive/2026-07-24-relocate-bundled-commands/`
