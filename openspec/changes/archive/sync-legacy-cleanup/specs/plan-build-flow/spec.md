## MODIFIED Requirements

### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts or a still-active (non-archived) change
folder is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` (defaulting
`AI_SPECS_HOME` to `$HOME/.ai-specs` when unset). Sync MUST NOT materialize a
per-project copy under `ai-specs/bin/`.

#### Scenario: Merge blocked when change folder still active

- GIVEN `openspec/changes/<slug>/` still exists (not archived)
- WHEN an agent attempts to merge the PR/MR
- THEN the skill stops with a plain-language blocker requiring archive-tail first

#### Scenario: Guardian path is CLI-home

- GIVEN `plan-build-flow` (or a VCS merge skill) is enabled
- WHEN an agent runs the pre-merge guardian
- THEN it uses `${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py`
- AND the recipe does not target `ai-specs/bin/premerge_guardian.py`
