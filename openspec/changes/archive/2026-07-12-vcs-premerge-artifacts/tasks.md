# Tasks: vcs-premerge-artifacts

Source spec: `openspec/changes/vcs-premerge-artifacts/specs/vcs-pr-flow/spec.md`
Source design: `openspec/changes/vcs-premerge-artifacts/design.md`

## Phase 1 — Contract and golden tests

- [x] **T1.1** — Promote pre-merge archive requirement into `openspec/specs/vcs-pr-flow/spec.md`.
- [x] **T1.2** — Mirror archive-before-merge guidance in provider merge-workflow skills.
- [x] **T1.3** — Clarify SDD artifact phases in `worktree-flow` skill table.
- [x] **T1.4** — Add golden tests in recipe test modules.

## Phase 2 — Recipe metadata and lock

- [x] **T2.1** — Patch-bump affected recipe versions.
- [x] **T2.2** — Bump dogfood pins in `ai-specs/ai-specs.toml`.
- [x] **T2.3** — Refresh `ai-specs/.ai-specs.lock` hashes.

## Phase 3 — Verify and deliver

- [x] **T3.1** — Run `./tests/run.sh`.
- [x] **T3.2** — Run `./tests/validate.sh`.
- [ ] **T3.3** — Open PR to `development`.
