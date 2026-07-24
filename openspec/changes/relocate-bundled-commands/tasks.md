# Tasks: relocate bundled COMMANDS to the CLI cache

## Planning depth

- **Classification**: domain_change (proposal → design → spec → tasks). Same
  tier as the parent `minimal-project-materialization` change: touches
  materialization (`refresh-bundled.py`), the merge/precedence model
  (`project-cache.py`), the lock schema (`lock.py`), a diagnostic surface
  (`doctor.py`), and init (`init.sh`) — but scoped to one asset kind with the
  design already resolved by mirroring a shipped pattern (lower ambiguity than
  the parent).
- **Authorization**: PENDING. Do not begin implementation until the maintainer
  confirms D1–D4 in proposal.md (cache path, precedence order, leftover
  migration rule, lock trim).

## Implementation (red-green-refactor)

### Phase 1 — `project-cache.py`: bundled-command primitives

- [x] 1.1 RED: `bundled_commands_root(project_root, cli_home)` returns
      `{cache}/.bundled/commands`.
- [x] 1.2 RED: `bundled_command_ids(cli_home)` returns `.md` stems under
      `bundled-commands/`.
- [x] 1.3 RED: `remove_bundled_command_leftovers` deletes an
      `ai-specs/commands/{name}.md` byte-identical to the bundled source;
      keeps one that differs (customized) with a warning; keeps one whose name
      has no bundled counterpart untouched.
- [x] 1.4 RED: `remove_bundled_command_leftovers` also removes via legacy lock
      hash match (untouched copy from an older CLI, mirrors the skill
      lock-hash migration guard).
- [x] 1.5 RED: `tracked_bundled_command_leftovers` finds a git-tracked
      `ai-specs/commands/{name}.md` for a bundled name whose working-tree copy
      is gone; returns `[]` when the file still exists on disk.
- [x] 1.6 GREEN: implement all of the above (factor the shared git-ls-files +
      remediation-formatting plumbing out of the skill versions rather than
      duplicating it, per design.md).

### Phase 2 — Command precedence (3-tier merge)

- [x] 2.1 RED (`tests/test_command_merge.py`): bundled-only command appears in
      merge output.
- [x] 2.2 RED: recipe-managed command silently overrides a bundled command of
      the same name (no warning).
- [x] 2.3 RED: local hand-authored command wins over both bundled and
      recipe-managed, with a warning on either collision.
- [x] 2.4 GREEN: extend `project-cache.merge_commands` to copy in ascending
      precedence order (bundled → recipe-managed → local).
- [x] 2.5 GREEN: delete `lib/_internal/command-merge.py` (confirmed dead;
      zero callers).

### Phase 3 — `refresh-bundled.py`: flatten-only for commands

- [x] 3.1 RED: `refresh-bundled --init` on a fresh project flattens bundled
      commands into `{cache}/.bundled/commands/` and does NOT write
      `ai-specs/commands/*.md`.
- [x] 3.2 RED: a project with a pre-existing byte-identical bundled-command
      copy in `ai-specs/commands/` has it removed on `refresh-bundled` (no
      `.new` sidecar ever written).
- [x] 3.3 RED: a project with a customized bundled-command copy (content
      differs) keeps it and prints a "customized" notice; no `.new` sidecar.
- [x] 3.4 GREEN: add `flatten_bundled_commands`; call
      `remove_bundled_command_leftovers` before `write_lock` (ordering mirrors
      the skill migration); remove `iter_bundled`, `project_path_for`,
      `display_name`, `lock_get`/`lock_set`/`lock_del`, `save_new_sidecar`,
      and the per-command content-hash loop in `refresh()`.

### Phase 4 — Lock schema

- [ ] 4.1 RED: `.ai-specs.lock` has no `[commands]`/`[opted-out]` after
      `sync`.
- [ ] 4.2 RED: a legacy lock with `[commands]`/`[opted-out]` sections has them
      dropped on the next `sync`/`refresh-bundled`.
- [ ] 4.3 GREEN: strip both sections from `lock.py` (`load_lock`,
      `write_lock`, `LOCK_HEADER`) and from `recipe-remove.sh`'s embedded
      rewrite.

### Phase 5 — `doctor.py`

- [ ] 5.1 RED: per-bundled-command-id OK check when
      `{cache}/.bundled/commands/{name}.md` exists.
- [ ] 5.2 RED: per-bundled-command-id ERROR check when it's missing, with
      `ai-specs sync` guidance.
- [ ] 5.3 RED: rewrite `test_bundled_commands_missing_reports_warn` — an empty
      `ai-specs/commands/` is now healthy (no WARN); a missing bundled command
      in the cache is the new ERROR signal.
- [ ] 5.4 RED: tracked-but-removed `ai-specs/commands/{name}.md` produces a
      WARN with `git rm --cached` guidance; index is unchanged.
- [ ] 5.5 GREEN: implement per-id check + tracked-leftover extension in
      `doctor.py`.

### Phase 6 — `init.sh` + help text

- [ ] 6.1 GREEN: remove init.sh step 2b (bundled-commands copy loop); add the
      explanatory comment (mirrors the skills-removal comment).
- [ ] 6.2 GREEN: fix `refresh-bundled.sh` usage text and `bin/ai-specs` help
      line — flatten-only, no `.new` sidecars, no in-project writes (mirrors
      current skills wording).
- [ ] 6.3 RED→GREEN (`tests/test_rules_audit.py::test_bundled_commands_distribution_after_refresh`):
      after `refresh-bundled --init`, assert commands land in
      `{cache}/.bundled/commands/`, NOT `ai-specs/commands/`; after
      `init`+`sync`, assert commands still land in each harness `commands_dir`
      (`.cursor/commands/`, `.opencode/commands/`) via the merge, while
      `ai-specs/commands/` stays absent/empty for a project with no local
      commands.

## Migration smoke test

- [ ] 7.1 End-to-end: simulate a pre-upgrade project (committed bundled
      commands, legacy lock with `[commands]`/`[opted-out]`, one customized
      bundled command, one genuine local command) → `sync` → clean migration:
      byte-identical bundled copies removed, customization preserved with
      warning, genuine local command untouched, lock trimmed to `[meta]`
      (+ `[agents.*]`), fan-out to per-agent dirs still includes the bundled
      command from cache.

## Validation

- [ ] `./tests/run.sh` green after every RED→GREEN pair (focused feedback
      during implementation, per config.yaml `apply` rules).
- [ ] `./tests/validate.sh` exit 0 (py_compile, bash -n, full unittest suite).
- [ ] README update if `ai-specs refresh-bundled`/`doctor` user-facing text
      changed (config.yaml `tasks` rule: update README when CLI/user-facing
      behavior changes) — check `README.md` bundled-commands section (line
      ~168) for accuracy.
- [ ] `verify-report.md` written; compare against every spec scenario in
      `specs/{skill-source-precedence,sync-lock,project-doctor}/spec.md`.
- [ ] Promote spec deltas into `openspec/specs/{skill-source-precedence,
      sync-lock,project-doctor}/spec.md` at archive.
