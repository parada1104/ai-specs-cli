# Tasks: bash-inherit-errexit-compatibility

Depth: light

Branch: `fix/bash-inherit-errexit-compatibility`

Worktree: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/bash-inherit-errexit-compatibility`

**Implementation authorized:** the light plan was approved before production
and test changes. This change intentionally does not add an SDD proposal, spec,
or design.

## Context

On macOS, the stock `/bin/bash` is Bash 3.2.57. It exits at runtime when either
sync entry point executes `shopt -s inherit_errexit`, because that option was
introduced in Bash 4.4. The line was introduced by commit `a766a85` and is
currently unconditional in both `lib/sync.sh:79` and
`lib/sync-agent.sh:199`. `bash -n` cannot detect this runtime-only option
compatibility failure.

The existing T4.1 regression in `tests/test_sync_output_verbosity.py` assumes
that the executing Bash supports `inherit_errexit`. That assumption is valid for
the modern Homebrew Bash used by the current test environment, but it does not
exercise the stock macOS runtime that reports the bug.

## Root Cause

**Root cause cluster:** `Bash compatibility contract`

Both scripts require an optional Bash 4.4 capability without probing or
guarding it. The failure occurs before the sync pipeline can run on Bash 3.2,
while syntax-only validation remains green and the existing regression test
only covers the newer interpreter behavior.

## Scope and Boundaries

### In scope

- `lib/sync.sh`: make `inherit_errexit` activation opportunistic and safe when
  the option is unavailable.
- `lib/sync-agent.sh`: apply the same compatibility treatment at its existing
  activation point.
- `tests/test_sync_output_verbosity.py`, or the closest existing sync test
  module if the current test structure requires it: add runtime regression
  coverage for both the legacy compatibility path and the modern semantic
  path.

### Explicitly out of scope

- Do not change the repository's general Bash version requirement or declare a
  new minimum/maximum Bash version.
- Do not remove `inherit_errexit` for Bash versions that support it.
- Do not repair, patch, or otherwise modify any external repository or
  installed copy of `ai-specs`.
- Do not modify unrelated production code, existing tests outside the focused
  regression, documentation, release metadata, or generated artifacts.
- Do not create `proposal.md`, `spec.md`, `design.md`, or any other planning
  artifact for this light change.

## Expected Behavior

| Runtime | Expected result |
| --- | --- |
| Bash 3.2.57, including macOS `/bin/bash` | `sync.sh` and `sync-agent.sh` start without `invalid shell option name`; the unsupported optional setting is skipped without turning startup into a failure, and the existing sync behavior remains available. |
| Bash >=4.4 | The scripts successfully enable `inherit_errexit`; command-substitution failure propagation covered by T4.1 remains intact. |
| `bash -n` | Remains a syntax check only and is not accepted as proof of this runtime compatibility fix. |

The two production sites must use the same minimal guarded activation idiom.
Only the unsupported `shopt` activation failure may be tolerated; unrelated
sync failures must continue to propagate normally.

## TDD Implementation Tasks

### RED — characterize the real runtime contract first

- [x] **T1.1** Update or extend T4.1 before changing production code so the
  regression distinguishes the two required interpreter behaviors: a Bash 3.2
  run of each sync surface must not fail on the unsupported option, while a
  Bash >=4.4 run must retain the command-substitution failure propagation
  assertion.
- [x] **T1.2** Make the legacy test reach the activation path in each entry
  point; `--help` alone is insufficient because it exits before the affected
  lines. Use `/bin/bash` when it is the macOS Bash 3.2 interpreter and use a
  modern Bash interpreter for the semantic assertion. If a matrix interpreter
  is unavailable on the host, skip that matrix leg explicitly with its reason
  rather than treating `bash -n` as coverage.
- [x] **T1.3** Run the focused sync regression test with the existing production
  lines unchanged and record the expected RED evidence: Bash 3.2 exposes the
  invalid-option failure, while the modern Bash semantic assertion continues to
  demonstrate why the option must remain enabled where supported.

### GREEN — apply the smallest compatibility fix

- [x] **T2.1** Replace the unconditional activation in `lib/sync.sh` with a
  guarded, quiet capability-tolerant activation that succeeds when
  `inherit_errexit` exists and does not fail startup when it does not.
- [x] **T2.2** Apply the identical guarded activation in
  `lib/sync-agent.sh`; do not add a new configuration flag, fallback state, or
  alternate implementation of `inherit_errexit`.
- [x] **T2.3** Run the focused regression test and record GREEN evidence for
  both sync surfaces on Bash 3.2 and for preserved failure propagation on Bash
  >=4.4.

### REFACTOR — keep the compatibility boundary narrow

- [x] **T3.1** Review the changed shell snippets for identical behavior,
  minimal scope, and correct `set -euo pipefail` interaction; ensure the guard
  cannot swallow failures from subsequent sync commands.
- [x] **T3.2** Simplify the regression test so it checks observable runtime
  behavior rather than asserting an unconditional source line, while retaining
  explicit coverage that modern Bash enables the option.
- [x] **T3.3** Re-run the focused regression after any test or shell cleanup.

## Acceptance Criteria

- [x] Both `lib/sync.sh` and `lib/sync-agent.sh` run through their affected
  startup path under Bash 3.2 without `invalid shell option name`.
- [x] Bash >=4.4 still enables `inherit_errexit`, and T4.1 still proves that a
  failing command substitution does not reach the success marker.
- [x] The regression covers both production surfaces and no longer requires
  every test interpreter to support `inherit_errexit`.
- [x] Unsupported-option handling is limited to the optional shell setting;
  ordinary sync errors retain their existing exit and reporting behavior.
- [x] No general Bash requirement changes, no external repository is modified,
  and no unrelated file is changed.
- [x] Final verification passes with exactly: `./tests/validate.sh`.

## Risks and Rollback Boundary

### Risks

- A broad `|| true` or equivalent could accidentally hide a real failure if it
  is applied beyond the single optional `shopt` command. Keep the guard local
  and verify normal sync failures still propagate.
- Test selection can accidentally validate only the modern Homebrew Bash. The
  regression must identify the interpreter version/capability and report an
  unavailable matrix leg explicitly.
- Removing the option instead of guarding it would make Bash 3.2 pass while
  regressing the intended Bash >=4.4 errexit semantics.

### Rollback boundary

Rollback is limited to the three in-scope files: restore the prior activation
lines and the prior T4.1 test only if the guarded implementation causes a
verified regression. No rollback may touch unrelated files, alter the general
Bash requirement, or modify any external repository.

## Final Gate

- [x] Human authorizes implementation after reviewing this plan.
- [x] After authorization and the RED/GREEN/REFACTOR cycle, run
  `./tests/validate.sh` from the repository root and retain its result as the
  final verification evidence.

## Implementation Evidence

- **RED**: The new Bash 3.2 matrix leg failed before the production change with
  `invalid shell option name` at both activation sites.
- **GREEN**: The focused `ErrexitInteractionTests` regression passed 4/4 after
  the guarded activation was added.
- **REFACTOR**: The behavior-based focused regression passed 4/4 again after
  cleanup; both guard snippets are byte-identical.
- **Full validation**: `./tests/validate.sh` exited 0 with 1617 tests passed
  and 116 environment/pre-existing skips. A separate bounded `./tests/run.sh`
  attempt exceeded its 120-second runner limit, while the implementation run
  observed 1617 tests passed.

## Tracker

- **card_id**: `6a7cadebfb3b957529926508`
- **shortLink**: `omB2CUU8`
- **url**: https://trello.com/c/omB2CUU8/73-story-add-optional-gentle-ai-compatibility-boundary
- **list**: In Progress
