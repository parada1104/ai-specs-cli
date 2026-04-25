## Why

Project-local and vendored `SKILL.md` files are now central to `ai-specs`, but their frontmatter contract is implicit and partially duplicated between rendering, vendoring, and `skill-sync`. A canonical contract plus shared enforcement reduces drift, keeps generated metadata reproducible, and makes auto-invoke behavior auditable.

## What Changes

- Defines a human-readable contract for `SKILL.md` frontmatter at `ai-specs/contracts/skill-frontmatter.md`.
- Adds a shared Python helper for parsing, normalizing, validating, and rendering skill frontmatter.
- Updates skill rendering and vendoring paths to use the shared contract instead of separate ad hoc parsers.
- Updates `skill-sync` to consume canonical metadata and fail actionably when auto-invoke metadata is incomplete.
- Extends tests for local skills, vendored skills, compatibility mode, invalid sync metadata, fan-out consistency, and generated AGENTS rows.
- No breaking change for existing first-party skills during the compatibility window.

## Capabilities

### New Capabilities
- `skill-frontmatter-contract`: Defines and enforces canonical frontmatter metadata for local and vendored skills.

### Modified Capabilities
- None.

## Impact

- Affected source: `lib/_internal/skill_contract.py`, `lib/_internal/agents-md-render.py`, `lib/_internal/vendor-skills.py`, `ai-specs/skills/skill-sync/assets/sync.sh`, `bundled-skills/skill-sync/assets/sync.sh`.
- Affected generated/local assets: bundled `skill-creator`, `skill-sync`, `skills-as-rules`, root `AGENTS.md`, `ai-specs/.ai-specs.lock`, and root manifest skill dependency metadata.
- Affected tests: `tests/test_skill_contract.py`, `tests/test_sync_pipeline.py`, `tests/test_target_resolve.py`.
- Rollback plan: revert the shared helper, contract doc, consumer changes, generated bundled assets, and tests; existing sync behavior then returns to prior ad hoc parsing.
