# Apply Progress: definir-contrato-frontmatter-skills

## Current Status

- Mode: recovered SDD from existing dirty worktree, then verified against current `development`.
- Tasks complete: 12/12.
- Worktree branch: `feat/skill-frontmatter-contract`.
- Trello card: https://trello.com/c/B1zFj6Bs

## Recovery Evidence

- Audited `.worktrees/skill-frontmatter-contract` and found real implementation work, not disposable stale files.
- Relevant pre-update tests were green: `tests.test_skill_contract`, `tests.test_sync_pipeline`, and `tests.test_target_resolve`.
- Stashed dirty worktree, fast-forwarded branch from old base to current `development`, and reapplied stash.
- Resolved conflicts in:
  - `ai-specs/.ai-specs.lock`
  - `lib/_internal/agents-md-render.py`
  - `tests/test_sync_pipeline.py`
- Regenerated derived artifacts with `./bin/ai-specs sync .`.

## Implementation Summary

- Added `ai-specs/contracts/skill-frontmatter.md` as the human-owned contract.
- Added `lib/_internal/skill_contract.py` for supported frontmatter parsing, normalization, validation, and rendering.
- Updated `lib/_internal/agents-md-render.py` to use the shared contract reader while preserving the generated context precedence section.
- Updated `lib/_internal/vendor-skills.py` to render canonical vendored frontmatter from manifest dependency inputs.
- Updated root/bundled `skill-sync` assets to validate complete auto-invoke metadata.
- Updated bundled `skill-creator`, `skill-sync`, `skills-as-rules`, manifest metadata, generated lockfile, and tests.

## Test Evidence

Targeted commands:

```bash
python3 -m unittest tests.test_skill_contract
python3 -m unittest tests.test_sync_pipeline
python3 -m unittest tests.test_target_resolve
```

Results:

- `tests.test_skill_contract`: PASS, 6/6.
- `tests.test_sync_pipeline`: PASS, 14/14.
- `tests.test_target_resolve`: PASS, 6/6.

Final commands:

```bash
./tests/run.sh
./tests/validate.sh
```

Results:

- `./tests/run.sh`: PASS, 36/36.
- `./tests/validate.sh`: PASS, 36/36 plus Python compile and Bash syntax checks.
