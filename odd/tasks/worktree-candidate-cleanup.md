# ODD Tasks: Worktree candidate cleanup

## Tracker
- Card: #139
- URL: https://trello.com/c/FnYgdyNO/139-skill-clean-merged-native-review-candidates-with-worktree-cleanup

## Goal
Add a project-local skill that extends merged worktree cleanup with a safe, explicit sweep for stale native review candidate views.

## Scope
- Document the candidate-view lifecycle and the boundary between native controller ownership and worktree cleanup.
- Define conservative eligibility checks: registered candidate worktree, clean immutable tree, no active review authority, owner process proven dead, and candidate changes already present in the integration branch.
- Add the cleanup sequence to the project-local worktree cleanup guidance without modifying the native review controller.
- Validate skill metadata and the resulting workflow documentation.

## Non-goals
- Do not remove an active or live candidate owner.
- Do not infer native review approval or mutate review authority state.
- Do not add a new CLI or change the Go worktree cleanup implementation in this slice.
- Do not delete the current candidate while its owner process remains live.

## Tasks
- [ ] Create the local candidate-cleanup skill with frontmatter and safe commands.
- [ ] Update local worktree-flow guidance to invoke the candidate sweep after merged worktree cleanup.
- [ ] Validate skill metadata and focused repository checks.
- [ ] Commit the work unit on `feat/worktree-candidate-cleanup`.

## Evidence
- Project-local skill path: `ai-specs/skills/worktree-candidate-cleanup/SKILL.md`.
- Current canonical development: `234006f`.
- Current candidate view: `.git/gentle-ai/candidate-views/1dbd0a6a-c05a-49c3-8886-714d5f78af06`.
- The current candidate tree is already present in `development`, but its owner PID is still alive, so it is not eligible for manual cleanup by this skill.
