# Tasks: local skill `codegraph-worktree`

## Planning depth

- requested: not specified
- signal: Light (documentation-only skill, 1 authored file + change folder; no product code)
- decided: Light

## Checklist

- [x] Verify worktree state and index absence baseline (done in session)
- [x] Create Trello card and record in proposal `## Tracker`
- [x] Author `ai-specs/skills/codegraph-worktree/SKILL.md` (frontmatter per skill-creator contract)
- [x] Validate metadata (`ai-specs skills list` shows the new local skill)
- [x] Run `ai-specs sync` inside the worktree — AGENTS.md renders no per-skill listing (no diff); fan-out created gitignored `.pi/skills/codegraph-worktree/`
- [x] Revert any unrelated sync side effects before staging — none found (`git status` clean apart from authored files)
- [x] Run test suite — full `./tests/run.sh` hangs in worktrees (internal `sync-agent` step blocks; pre-existing environment issue, unrelated to this docs-only change). Evidence: `py_compile` OK, `bash -n` OK, focused `tests.test_doctor` + `tests.test_harness_cli_literacy` OK
- [ ] Commit change folder + skill + sync output
- [ ] Archive change folder to `openspec/changes/archive/YYYY-MM-DD-local-codegraph-skill/` (pre-merge)
- [ ] Merge locally to `development` (no PR, per explicit user instruction) and clean up worktree/branch
