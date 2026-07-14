# Spec delta: plan-build-flow

## ADDED Requirements

### Requirement: Prefer runtime plan mode for artifact creation

When the host runtime exposes a plan/review mode, the skill SHALL prefer that
mode for the planning phase. All classified planning artifacts for the chosen
depth SHALL be written during planning. Production code MUST NOT be modified
during planning.

#### Scenario: Plan mode produces tier artifacts only

- GIVEN a substantial user request with no existing change folder
- AND the runtime is in plan mode
- WHEN planning completes
- THEN tier-minimum artifacts exist under `openspec/changes/<slug>/`
- AND no production code paths were modified
- AND the agent stops for authorization

### Requirement: Pre-merge merge guardian

Before merge, the skill SHALL treat missing tier artifacts or a still-active
(non-archived) change folder as a hard stop. Archive MUST already be on the
review branch (`openspec/changes/archive/<slug>/`) before merge proceeds.

#### Scenario: Merge blocked when change folder still active

- GIVEN `openspec/changes/<slug>/` still exists (not archived)
- WHEN an agent attempts to merge the PR/MR
- THEN the skill stops with a plain-language blocker requiring archive-tail first

#### Scenario: Merge blocked when archive missing tier files

- GIVEN archive path exists but lacks the tier minimum files
- WHEN merge is attempted
- THEN the skill stops with a blocker listing missing files

## MODIFIED Requirements

### Requirement: Ambient planning trigger

The bundled skill SHALL auto-invoke on substantial change requests, classify
depth, run the classified planning chain (preferring runtime plan mode when
available), and stop for human authorization. Planning MUST NOT require slash
commands. Prompts and user-facing guidance MUST NOT tell the user to "run
/plan" or equivalent.

#### Scenario: Plan stops before implementation

- GIVEN a developer requests a substantial change in natural language
- WHEN the planning phase chain for the classified depth completes
- THEN `tasks.md` exists and no production code files were modified
- AND for Standard or Full depth, required spec artifacts also exist
