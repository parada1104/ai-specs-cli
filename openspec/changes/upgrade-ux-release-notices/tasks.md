# Tasks: upgrade-experience

Depth: full

Tracker: card #80 — https://trello.com/c/x7Pj31vF

Discipline: red-green-refactor. Every work unit writes a failing test first,
then the minimum implementation, then cleanup. `./tests/validate.sh` must pass
before the PR.

## WU0 — Verify the exclusion list before excluding anything

Blocks WU4. The runtime reference count is evidence, not proof.

- [ ] Enumerate every read of `$AI_SPECS_HOME` across `lib/`, `bin/`, and
      `lib/_internal/*.py`, including dynamic path construction
- [ ] Confirm nothing resolves into `openspec/`, `tests/`, `.github/`, or `tmp/`
      at runtime — including bundled skills that may reference spec paths
- [ ] Record the finding in `apply-progress.md`; if any read exists, shrink the
      exclusion list and update the spec before proceeding

## WU1 — Changelog parser

- [ ] RED: unit tests for `lib/_internal/changelog.py` — section extraction,
      semver ordering, `(current, new]` range selection, `### Upgrade notes`
      extraction
- [ ] RED: degradation tests — missing file, malformed headings, no matching
      section, version present with no notice
- [ ] GREEN: implement the parser as pure functions over text
- [ ] Verify against the real `CHANGELOG.md` for `0.19.0 -> 0.22.0`

## WU2 — Compact output

- [ ] RED: test that a successful upgrade emits no `remote:` lines, no transfer
      progress, and no diffstat
- [ ] RED: test that `-v`/`--verbose` restores full git output
- [ ] RED: test that a failing step dumps full stdout and stderr and preserves
      its exit code
- [ ] RED: regression test that every existing abort path keeps its exact
      message and exit code (1, 2, 3, 4, 5)
- [ ] GREEN: adopt the `run_step` pattern from `lib/sync.sh:119` in
      `lib/upgrade.sh`; add `-v`/`--verbose` parsing
- [ ] Confirm `--dry-run` output is unchanged

## WU3 — Version summary and notice replay

- [ ] RED: single-version and multi-version summary, newest first
- [ ] RED: notices replay oldest first across multiple crossed versions
- [ ] RED: notices are printed in compact mode
- [ ] RED: a notice containing a command is displayed, never executed
- [ ] RED: no crossed version declares a notice — no section, no placeholder
- [ ] RED: unreadable changelog degrades to the plain version line, upgrade
      still exits 0
- [ ] GREEN: wire the parser into `upgrade.sh` per the D7 output shape
- [ ] Author the `### Upgrade notes` subsection for `0.22.0` in `CHANGELOG.md`

## WU4 — Narrowed checkout

Depends on WU0.

- [ ] RED: fresh install on modern Git excludes the four subtrees and retains
      every runtime path
- [ ] RED: `git merge-base --is-ancestor` still resolves after narrowing —
      the divergence guard is intact
- [ ] RED: an existing full install narrows once; a second upgrade is a no-op
- [ ] RED: Git without `--filter` or cone mode falls back to a full checkout and
      still succeeds
- [ ] RED: narrowing failure warns and the upgrade still exits 0
- [ ] GREEN: `--filter=blob:none` + cone-mode sparse checkout in `install.sh`,
      with capability detection and fallback
- [ ] GREEN: idempotent narrowing step in `upgrade.sh`
- [ ] Measure and record the resulting install footprint

## WU5 — Make notices part of the release ritual

Without this, the capability silently rots.

- [ ] Update `ai-specs/skills/release-flow/SKILL.md`: authoring
      `### Upgrade notes` is a step of the version bump
- [ ] Correct the two known staleness bugs in the same skill while it is open:
      the release is created by CI (`softprops/action-gh-release`), so
      `gh release create` fails — the ritual must use `gh release edit`
- [ ] Document that `git sparse-checkout disable` restores a full checkout
- [ ] Update `docs/` where the install footprint is described

## WU6 — Close out

- [ ] `./tests/validate.sh` green
- [ ] Manual check: real upgrade against a scratch clone, compact and verbose
- [ ] `verify-report.md` with RED/GREEN evidence per work unit
- [ ] Archive the change folder on the review branch before merge

## Deliberately excluded

- Any change to upgrade safety checks, abort conditions, or exit codes
- Shallow clone (breaks the divergence guard — see design D4)
- Conditional or executable notices (see design D2)
- Runtime-brief / `AGENTS.md` ownership — card #81
