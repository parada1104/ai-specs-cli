## 1. Preserve And Rebase Existing Work

- [x] 1.1 Audit `.worktrees/skill-frontmatter-contract` and confirm it contains real pending work rather than disposable stale files.
- [x] 1.2 Stash the existing dirty worktree, fast-forward the branch to current `development`, and reapply the stashed changes.
- [x] 1.3 Resolve conflicts in lockfile, `agents-md-render.py`, and `tests/test_sync_pipeline.py` without dropping `development` changes.
- [x] 1.4 Regenerate derived artifacts with `./bin/ai-specs sync .`.

## 2. Contract And Enforcement

- [x] 2.1 Add `ai-specs/contracts/skill-frontmatter.md` as the human-owned contract for skill frontmatter.
- [x] 2.2 Add `lib/_internal/skill_contract.py` for parsing, normalizing, validating, and rendering supported frontmatter.
- [x] 2.3 Update `lib/_internal/agents-md-render.py` to use the shared contract reader while preserving the context precedence generated section.
- [x] 2.4 Update `lib/_internal/vendor-skills.py` to generate canonical vendored frontmatter from manifest dependency inputs.
- [x] 2.5 Update `skill-sync` bundled/local scripts to validate complete Auto-invoke metadata.

## 3. Tests And Generated Assets

- [x] 3.1 Add `tests/test_skill_contract.py` coverage for canonical local skills, compatibility mode, vendored generation, invalid sync metadata, and CLI JSON output.
- [x] 3.2 Extend `tests/test_sync_pipeline.py` for vendored frontmatter normalization, hand-edited vendored rewrites, local Auto-invoke authoring, manual-only local skills, and invalid metadata failure.
- [x] 3.3 Update target/sync fixtures and generated bundled assets required by the contract.

## 4. Verification And Archive

- [x] 4.1 Run targeted tests for skill contract, sync pipeline, and target resolution.
- [x] 4.2 Run `./tests/run.sh` and `./tests/validate.sh`.
- [x] 4.3 Create apply progress and verify report.
- [x] 4.4 Sync the main spec and archive the OpenSpec change.
