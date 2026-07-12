# Delta for vcs-pr-flow

## ADDED Requirements

### Requirement: Pre-merge archive artifacts

The system MUST archive and record SDD/OpenSpec artifacts before a VCS PR/MR is merged. The archive boundary MUST occur while the change is still on the review branch, not after the merge commit lands on the base branch.

#### Scenario: Archive runs before merge

- GIVEN a provider-backed PR/MR is ready to merge
- WHEN the archive step runs for the change
- THEN the change artifacts are persisted before merge completes
- AND the archive records the pre-merge state as the source of truth

#### Scenario: Post-merge archive is rejected

- GIVEN a PR/MR has already been merged into the base branch
- WHEN the archive step tries to treat the merged state as the archive boundary
- THEN the system rejects that interpretation
- AND the archive must reference the pre-merge branch state instead

#### Scenario: Provider behavior stays aligned

- GIVEN GitHub, GitLab, or Bitbucket provider flows are enabled
- WHEN the pre-merge archive rule is rendered into workflow guidance
- THEN the provider guidance matches the same archive-before-merge contract
- AND no provider introduces a different timing rule

#### Scenario: Hidden ceremony remains hidden

- GIVEN the user follows the normal plan/build flow
- WHEN the archive rule is applied
- THEN no new slash command or extra user-facing mode is introduced
- AND the archive step remains part of the existing invisible workflow

(Previously: archive timing was not explicitly fixed at the pre-merge boundary.)
