# Design: relocate bundled COMMANDS to the CLI cache

## Governance model (extends the shipped table)

| Content | Declared by | Materialized at | Git | Rationale |
|---|---|---|---|---|
| Local commands (hand-authored) | project | `ai-specs/commands/` | committed | authored in-repo; git is the source |
| Recipe-managed commands (`[[provides.commands]]`) | recipe → CLI | `{cache}/commands/` | outside repo | transitive, CLI-owned (unchanged) |
| **CLI-bundled commands** (`rules-audit`, `skills-as-rules`) | CLI | `{cache}/.bundled/commands/` (new) | outside repo | ships with the CLI; identical for every project |

Committed project surface for commands after this change: `ai-specs/commands/`
holds **hand-authored files only** — the same rule already applied to
`ai-specs/skills/`.

## Command source precedence (3 tiers, mirrors the skill four-tier)

1. `ai-specs/commands/{name}.md` — local hand-authored (highest)
2. `{cache}/commands/{name}.md` — recipe-managed, materialized by
   `recipe-materialize.materialize_command` (**unchanged**)
3. `{cache}/.bundled/commands/{name}.md` — **CLI-bundled (new, lowest)**,
   flattened from `$AI_SPECS_HOME/bundled-commands/`

Merge order in `merge_commands()` (and the CLI `merge-commands` subcommand it
backs): copy bundled first (silent), then recipe-managed (silent overwrite —
both are CLI-driven tiers, no user-facing signal needed), then local
hand-authored last (existing `_warn(...)` on collision, extended to fire for
either lower tier). This mirrors skills' "whole-source, no file-level merge,
local always wins" rule while keeping the existing warning UX for the case a
human actually cares about (their file got shadowed by something managed).

`command-merge.py` (the standalone duplicate of `project-cache.merge_commands`,
already flagged as a dead-code INFO residual in the `cli-bound-recipes`
verify-report, zero callers confirmed by grep) is deleted rather than updated.

## `refresh-bundled` becomes fully flatten-only

Today `refresh-bundled.py`'s `iter_bundled()` still yields both `skill` and
`command` kinds, but `flatten_bundled_skills()` bypasses it entirely (its own
`copytree` loop) — `iter_bundled` is only consumed by the command branch of
`refresh()` for the content-hash/`.new`-sidecar algorithm. Once commands also
flatten-only:

- `iter_bundled()`, `project_path_for()`, `display_name()`, `lock_get`/
  `lock_set`/`lock_del`, `save_new_sidecar()` become dead — **removed**.
- `refresh()` collapses to two calls: `flatten_bundled_skills(...)` +
  `flatten_bundled_commands(...)`. No lock read, no lock write, no `touched`
  diffing, no `opted_out` bookkeeping — refresh-bundled no longer opens
  `ai-specs/.ai-specs.lock` at all.
- `flatten_bundled_commands(cli_source, project, cli_home)`: wipe + rebuild
  `pc.bundled_commands_root(project, cli_home=cli_home)`, flat-copy every
  `*.md` directly under `cli_source / "bundled-commands"` (no subdirectories,
  unlike skills which copy whole skill directories).

## Leftover migration (existing projects upgrading from 0.16.0)

Exact mirror of `remove_bundled_skill_leftovers`, applied to
`ai-specs/commands/*.md` instead of `ai-specs/skills/*/`:

```
remove_bundled_command_leftovers(ai_specs, cli_home, lock_commands=None):
    for each *.md directly under ai-specs/commands/:
        bundled_src = {cli_home}/bundled-commands/{name}.md
        if bundled_src doesn't exist: skip (genuine local command, different name)
        if proj content == bundled_src content (CRLF-normalized): delete (leftover)
        elif sha256(proj content) == lock_commands.get(name): delete (untouched
             copy from an older CLI version — legacy-lock migration signal)
        else: keep + warn "customized" (user edited it in-project)
```

Called from `refresh()` **before** `write_lock` drops the `[commands]` section
— same ordering constraint the skill migration already established (the
legacy hash must still be in memory to recognize an untouched-but-stale copy).

`tracked_bundled_command_leftovers()` / `format_tracked_bundled_remediation()`
generalize the skill versions to accept a `(kind, path_prefix)` pair (or a thin
`_tracked_bundled_leftovers(project_root, cli_home, bundled_ids, path_template)`
helper both skill and command variants call) rather than duplicating the git
plumbing — same `git ls-files` + working-tree-absence check, different path
template (`ai-specs/commands/{name}.md` vs `ai-specs/skills/{id}/`).

## Lock schema

`[commands]` / `[opted-out]` were the last non-`[meta]`/`[agents.*]` sections
(`[skills.*]`/`[recipes.*]`/`[deps.*]` were already dropped by the parent
change). Once commands stop needing per-file hash tracking or delete-memory,
both go. `load_lock()` drops the two keys from its default dict and TOML
parse; `write_lock()` drops the two `out.append` blocks; `LOCK_HEADER` comment
updated to remove the "`[commands]` / `[opted-out]` still track..." line.
`recipe-remove.sh`'s embedded Python heredoc mirrors the same trim (it
currently re-serializes `[commands]`/`[opted-out]` verbatim on every recipe
removal).

Post-change lock is `[meta]` (+ `[agents.*]` for generated-file hashes used by
`doctor`'s stale-file check) — nothing else.

## `doctor` checks

- **Bundled-command presence**: replace the aggregate
  `local_ok = ai-specs/commands has *.md OR cache_ok = commands_dir non-empty`
  check (which conflated "any command exists" with "bundled commands
  resolved" and is now moot — an empty hand-authored `ai-specs/commands/` is
  healthy, same as an empty `ai-specs/skills/`) with a **per-bundled-command-id**
  loop against `{cache}/.bundled/commands/{name}.md`, mirroring the existing
  per-bundled-skill loop exactly (`OK`/`ERROR` per name, `ai-specs sync`
  guidance).
- **Tracked-leftover WARN**: extend `_check_tracked_bundled_leftovers` to also
  call the command variant and report tracked-but-removed
  `ai-specs/commands/{name}.md` paths, same `git rm --cached` guidance,
  never touches the index.

## `init.sh`

Remove step 2b (the `BUNDLED_COMMANDS_DIR` copy loop) entirely. Add a comment
next to the existing skills-removal comment (lines 204-208) explaining bundled
commands now flatten into the cache via `refresh-bundled` (step 7), never
copied into the project — same wording pattern already used for skills.

## Migration risk

- **Consumer upgrade churn**: first `sync`/`refresh-bundled` after upgrade
  deletes committed bundled-command copies from `ai-specs/commands/` — a
  visible diff. Mitigated the same way as the skill migration: the leftover
  removal prints one line per deleted file (`✓ removed leftover bundled
  command ai-specs/commands/{name}.md`).
- **Dogfooding self-collision** (this repo authors `bundled-commands/*.md`
  directly, and its own `ai-specs/commands/` — if it has one — must not be
  treated as a stale copy of itself). Same guard structure as the skill side:
  the leftover check only fires when the project copy is byte-identical to
  the bundled source; this repo's `bundled-commands/` sources ARE NOT under
  `ai-specs/commands/` (they're the CLI source tree, a sibling directory), so
  there's no path collision to guard against — simpler than the skills case
  (which had a real dogfooding collision because `skill-creator`/`skill-sync`
  are BOTH bundled AND locally authored in this repo).
- **`test_bundled_commands_missing_reports_warn`** (doctor test) currently
  deletes `ai-specs/commands/` and expects a WARN — that premise is now false
  (empty is healthy); the test is rewritten to assert the new per-id ERROR
  when a bundled command is missing from the cache instead.

## Rollback

Additive at the code level; revert the diff. `{cache}/.bundled/commands/` is
disposable (machine-local cache, recreated by the next `refresh-bundled` or
`sync`). No git history rewrite, no data loss — a project on either side of
this change resolves commands correctly (worst case on downgrade: an old CLI
re-copies bundled commands into `ai-specs/commands/` again, same as it did
before).
