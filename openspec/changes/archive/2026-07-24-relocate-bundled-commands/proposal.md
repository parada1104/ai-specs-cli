# Proposal: relocate bundled COMMANDS to the CLI cache

## Context

`minimal-project-materialization` (PR #145, archived
`openspec/changes/archive/2026-07-23-minimal-project-materialization/`) moved
CLI-bundled SKILLS out of the committed project surface into
`{cache}/.bundled/skills/`, resolved by a four-tier precedence
(local > recipe > dep > bundled), with zero in-project writes and zero lock
hashes. Bundled COMMANDS were explicitly deferred (tasks.md follow-up line,
Trello card #47): they still materialize into `ai-specs/commands/` (the
committed project surface) via two paths — `init.sh` step 2b (idempotent copy)
and `refresh-bundled.py`'s content-hash + `.new`-sidecar algorithm — and the
lock still carries `[commands]` / `[opted-out]` sections to drive that
algorithm.

This inconsistency means `ai-specs/commands/` today mixes CLI-owned bundled
commands with genuinely hand-authored ones in the same committed directory,
with no doctor check for tracked-but-removed bundled-command leftovers (the
skills side has one). Sibling follow-up card #49
(`materialization-followup-guidance`, PR #146, merged) explicitly listed
"Relocate bundled COMMANDS" as out of scope and deferred it here.

## Objective

Apply the exact same governance model already shipped for skills to commands:
CLI-bundled commands (`rules-audit.md`, `skills-as-rules.md`) resolve from
`{cache}/.bundled/commands/`, never materialize in-project, and the lock drops
`[commands]` / `[opted-out]` — its last non-`[meta]` remnant — becoming a pure
`[meta]` provenance stamp.

## Scope — In

- `lib/_internal/refresh-bundled.py` — add `flatten_bundled_commands()`
  (mirrors `flatten_bundled_skills`); remove the per-file content-hash /
  `.new`-sidecar loop and all lock read/write for commands. `refresh-bundled`
  becomes flatten-only for BOTH asset kinds — a pure cache-repair verb with
  zero in-project writes and zero lock interaction.
- `lib/_internal/project-cache.py` — new `bundled_commands_root()`,
  `bundled_command_ids()`, `remove_bundled_command_leftovers()` (mirrors the
  skill leftover-cleanup + lock-hash migration guard), and
  `tracked_bundled_command_leftovers()` / remediation formatting extended to
  cover commands. `merge_commands()` adds the bundled tier at lowest
  precedence: `ai-specs/commands/` (local) > `{cache}/commands/` (recipe) >
  `{cache}/.bundled/commands/` (CLI-bundled).
- `lib/_internal/lock.py` — drop `[commands]` / `[opted-out]` from
  `load_lock`/`write_lock`; lock becomes `[meta]` + `[agents.*]` only.
- `lib/_internal/doctor.py` — replace the aggregate "any commands present"
  check with a per-bundled-command-id check (mirrors the bundled-skill check);
  extend `_check_tracked_bundled_leftovers` to cover commands.
- `lib/init.sh` — remove step 2b (bundled-commands copy); mirror the existing
  comment explaining skills already don't copy into the project.
- `lib/recipe-remove.sh` — drop `[commands]`/`[opted-out]` from the embedded
  lock rewrite (mirrors `lock.py`).
- `lib/refresh-bundled.sh`, `bin/ai-specs` — fix help text (no more "keeps your
  edits, drops `.new` sidecars"; it's flatten-only now, matching the skills
  wording).
- Remove `lib/_internal/command-merge.py` — dead code (flagged as an INFO
  residual in the `cli-bound-recipes` verify-report; superseded by
  `project-cache.py merge-commands`, confirmed zero callers).
- Tests: `tests/test_command_merge.py` (3-tier precedence), migration/leftover
  tests mirroring `tests/test_external_dirs.py`'s bundled-skill suite,
  `tests/test_doctor.py` bundled-command checks, `tests/test_rules_audit.py`
  distribution test.

## Scope — Out

- Subrepo sync-agent mirror behavior (`mirror_directory` into a *different*
  target's `ai-specs/commands/`) — pre-existing, symmetric with how resolved
  skills already mirror into a subrepo's `ai-specs/skills/`; not part of this
  follow-up.
- Migration guidance docs for the `ai-specs/recipes/` gitignore follow-up
  (separate, already-listed follow-up from the parent change).
- Any recipe-provided command behavior change (`recipe-materialize.py`
  `materialize_command` keeps writing to `{cache}/commands/` unchanged).

## Decisions

- **D1 — Bundled commands cache path**: `{cache}/.bundled/commands/`
  (new `bundled_commands_root()`), sibling to `{cache}/.bundled/skills/`
  (`bundled_skills_root()`), NOT reusing `{cache}/commands/` (the existing
  recipe-managed tier written by `recipe-materialize.materialize_command`).
  Keeping them physically separate keeps the "who owns this tier" governance
  question answerable by directory alone, exactly like skills' four tiers.
- **D2 — Command precedence**: 3 tiers, lowest to highest —
  `.bundled/commands` (CLI) < `commands` (recipe-managed, cache) <
  `ai-specs/commands` (local hand-authored). Recipe silently overrides bundled
  (both CLI-driven, no user-visible warning, same as skill tier-shadowing).
  Local hand-authored logs a warning on collision with either lower tier
  (existing behavior for the local-vs-recipe case, extended to local-vs-bundled).
- **D3 — Leftover migration**: exact mirror of
  `remove_bundled_skill_leftovers` — an `ai-specs/commands/{name}.md` whose
  content is byte-identical (CRLF-normalized) to the current bundled source OR
  matches the legacy lock hash (`lock["commands"][name]`) is deleted as a
  resolved leftover; anything that differs is a genuine customization and is
  kept with a warning. Runs inside `refresh()` before the lock is normalized,
  same ordering constraint as the skill migration.
- **D4 — Lock**: `[commands]` / `[opted-out]` are the last non-`[meta]`,
  non-`[agents.*]` sections in the lock; both are dropped once commands no
  longer materialize in-project (no more per-file hash needed, no more
  "user deleted it, don't reinstall" bookkeeping needed — same reasoning
  the parent change applied to skills).

## Rollback

Additive/reversible: revert the diff, delete `{cache}/.bundled/commands/`
(cache is disposable, machine-local), no data migration or git history
rewrite needed. A project mid-migration (customized bundled command kept as a
genuine local file) is unaffected — the leftover-cleanup guard only deletes
byte-identical copies.

## Success Criteria

- CLI-bundled commands (`rules-audit`, `skills-as-rules`) resolve from
  `{cache}/.bundled/commands/`; `ai-specs/commands/` contains only
  hand-authored files after `sync`/`refresh-bundled`.
- `ai-specs init` on a fresh project does not write bundled commands into
  `ai-specs/commands/`.
- A project upgrading from 0.16.0 (bundled commands committed) cleanly
  migrates: byte-identical bundled-command copies are removed from
  `ai-specs/commands/`; customized copies are preserved with a warning.
- `.ai-specs.lock` contains no `[commands]` / `[opted-out]` after `sync`.
- `doctor` reports per-bundled-command OK/ERROR (mirrors bundled-skill checks)
  and WARNs on tracked-but-removed bundled-command leftovers.
- Per-agent fan-out (`.cursor/commands/`, `.opencode/commands/`, etc.) is
  unaffected — bundled commands still land there via the cache merge.
- `./tests/validate.sh` passes.
