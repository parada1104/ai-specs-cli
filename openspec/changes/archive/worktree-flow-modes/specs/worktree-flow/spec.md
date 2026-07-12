# Worktree-Flow Specification

## Purpose

NEW capability. Defines `gate_mode` for `worktree-flow` — how `worktree-gate.sh` gates writes to the main worktree on a protected branch. Modes `always` / `ask` / `off`.

## Requirements

### Requirement: gate_mode configuration schema

`recipes.worktree-flow.config.gate_mode` MUST be one of `always`, `ask`, `off`; default `always` when absent/empty. `ai-specs sync` MUST reject invalid values with non-zero exit and a diagnostic naming the value and enum.

#### Scenario: Default when unset

- GIVEN a manifest with no `gate_mode`
- WHEN `ai-specs sync` resolves the mode
- THEN the value is `always` and no error raised

#### Scenario: Invalid enum rejected

- GIVEN `gate_mode = "lazy"`
- WHEN `ai-specs sync` runs
- THEN it exits non-zero, stderr names `lazy` invalid, lists `always | ask | off`

### Requirement: Resolution precedence

The hook SHALL resolve: `WORKTREE_GATE_MODE` env override beats the stamped value; empty/unset env falls back to the stamped value. `ai-specs sync` materializes the stamped value into `worktree-gate.sh`; the hook MUST NOT do runtime manifest lookup.

#### Scenario: Env override beats stamped value

- GIVEN a hook stamped `always`
- WHEN the host sets `WORKTREE_GATE_MODE=off`
- THEN the hook behaves as `off`

#### Scenario: Empty env falls back to stamped value

- GIVEN a hook stamped `ask` and `WORKTREE_GATE_MODE` unset
- WHEN the hook runs
- THEN it behaves as `ask`

### Requirement: `always` mode behavior

`always` MUST run existing trunk-protection unchanged for write tools; linked-worktree edits (`git_dir != common_dir`) stay allowed. `WORKTREE_GATE_PROTECTED` remains orthogonal to mode.

#### Scenario: Block on protected branch (always)

- GIVEN `gate_mode = "always"` on branch `development`
- WHEN an Edit targets the main worktree (not under `.worktrees/`)
- THEN the hook exits `2` and stderr names the branch

#### Scenario: Allow in linked worktree (always)

- GIVEN `gate_mode = "always"` and target lives under `.worktrees/`
- WHEN any write tool runs
- THEN the hook exits `0`

### Requirement: `off` mode behavior

`off` MUST self-disable early and exit `0` for every write-tool invocation before the protected-branch check runs.

#### Scenario: Writes pass on protected branch (off)

- GIVEN `gate_mode = "off"` on branch `main`
- WHEN an Edit targets the main worktree
- THEN the hook exits `0` before any branch comparison

### Requirement: `ask` mode behavior (agent-mediated)

`ask` MUST behave like `always` (exit `2`) but stderr MUST also name the
`WORKTREE_GATE_MODE=off` one-shot bypass. Confirmation is orchestrator-mediated
via env — NOT a TTY dialog; the hook MUST NOT read interactive stdin.

#### Scenario: Block with bypass hint (ask)

- GIVEN `gate_mode = "ask"` on branch `development`
- WHEN an Edit targets the main worktree
- THEN the hook exits `2`, stderr mentions `WORKTREE_GATE_MODE=off`

#### Scenario: Proceed after orchestrator confirmation (ask)

- GIVEN `gate_mode = "ask"` and the orchestrator obtained confirmation
- WHEN the tool re-runs with `WORKTREE_GATE_MODE=off`
- THEN the hook exits `0`

#### Scenario: Host does not surface stderr (ask caveat)

- GIVEN `gate_mode = "ask"` and a host that does not surface hook stderr
- WHEN an Edit targets the main worktree
- THEN the edit fails (exit `2`), the hint is invisible, and README MUST document this caveat so teams pick `always`/`off`

### Requirement: sync stamps resolved mode

`ai-specs sync` SHALL materialize the resolved `gate_mode` into `worktree-gate.sh`; env override still applies at dispatch.

#### Scenario: Stamping writes mode into hook

- GIVEN `gate_mode = "ask"` in the manifest
- WHEN `ai-specs sync` runs
- THEN `worktree-gate.sh` carries the resolved `ask` mode and a later edit with no env override is governed by `ask`

## Acceptance Criteria (test map)

| AC | Test | Req |
|----|------|-----|
| AC1 | `test_gate_always_blocks_protected` | always |
| AC2 | `test_gate_off_self_disables` | off |
| AC3 | `test_gate_ask_blocks_with_bypass_hint` | ask |
| AC4 | `test_env_override_beats_stamped` | precedence |
| AC5 | `test_empty_env_keeps_stamped` | precedence |
| AC6 | `test_sync_rejects_invalid_gate_mode` | schema |
| AC7 | `test_sync_defaults_to_always` | schema |
| AC8 | `test_linked_worktree_always_allowed_in_always` | always |