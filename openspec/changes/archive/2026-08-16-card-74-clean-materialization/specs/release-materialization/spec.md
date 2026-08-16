# release-materialization (delta)

## Purpose

Define the isolated clean-materialization gate that must pass before a
release candidate is considered publishable. This repository's own dogfood
`ai-specs/` state is not release evidence.

## ADDED Requirements

### Requirement: Isolated consumer project is the evidence surface

Release-candidate materialization evidence SHALL come from an isolated
temporary consumer project synced with the in-tree CLI
(`AI_SPECS_HOME` pointing at the candidate tree). The candidate
repository's committed dogfood lock, generated adapters, and
`AGENTS.md` SHALL NOT be treated as that evidence.

#### Scenario: Isolated project is the inspected surface

- **GIVEN** a release-candidate tree with a stale dogfood
  `.ai-specs.lock` `[meta].cli_version`
- **WHEN** the clean-materialization gate runs
- **THEN** it MUST init and sync a temporary project, not this
  repository's `ai-specs/` directory
- **AND** it MUST NOT rewrite or commit the candidate's dogfood lock
  as part of passing the gate

### Requirement: Lock version matches the candidate VERSION

After isolated `init` + `sync`, the temporary project's
`.ai-specs.lock` `[meta].cli_version` SHALL equal the candidate
repository `VERSION` file.

#### Scenario: Fresh sync stamps the candidate version

- **GIVEN** repository `VERSION` is `0.22.0`
- **WHEN** isolated `init` and `sync` complete against that tree
- **THEN** the temporary lock `[meta].cli_version` MUST be `0.22.0`

### Requirement: Doctor is ERROR-free after isolated sync

`ai-specs doctor` on the isolated project SHALL exit 0 and SHALL NOT
report `ERROR` checks. `WARN` checks MAY remain when they are
documented as non-blocking (for example missing optional host tools).

#### Scenario: Healthy isolated project exits zero

- **GIVEN** isolated `sync` completed for the representative consumer
  manifest
- **WHEN** `ai-specs doctor` runs on that temporary project
- **THEN** the command MUST exit `0`
- **AND** the report MUST contain no `ERROR` lines

### Requirement: Generated adapters match enabled agents

Isolated `sync` SHALL materialize the expected generated outputs for
every agent enabled in the representative consumer manifest.

#### Scenario: Enabled agent outputs are present

- **GIVEN** the isolated manifest enables `claude`, `cursor`,
  `opencode`, `pi`, and `omp`
- **WHEN** isolated `sync` completes
- **THEN** each enabled agent's expected generated outputs MUST exist
  (instruction, skills, and commands paths; MCP adapter files only
  when the manifest declares `[mcp.*]`)
- **AND** `AGENTS.md` MUST exist in the temporary project

### Requirement: Catalog and cache reconcile without in-project leftovers

Isolated `sync` SHALL populate CLI-bundled skills and commands through
the cache layout. It SHALL NOT leave CLI-bundled skill copies under
the temporary `ai-specs/skills/`.

#### Scenario: Bundled skills stay out of the local skill tree

- **GIVEN** isolated `sync` has run
- **WHEN** the temporary `ai-specs/skills/` directory is inspected
- **THEN** it MUST NOT contain CLI-bundled skill ids such as
  `harness-lifecycle` or `skill-creator`
- **AND** those ids MUST resolve from the cache / bundled-skill tier

### Requirement: Gate release evidence matches VERSION and SHA256SUMS

The candidate's committed `catalog/recipes/worktree-flow/bin/SHA256SUMS`
SHALL name the same version as repository `VERSION`. Isolated
`worktree-flow` sync SHALL materialize the launcher and SHALL treat
those committed digests as the trust root. The gate MUST NOT require a
already-published GitHub release.

#### Scenario: SHA256SUMS tracks the candidate version

- **GIVEN** repository `VERSION` is `0.22.0`
- **WHEN** the clean-materialization gate inspects
  `catalog/recipes/worktree-flow/bin/SHA256SUMS`
- **THEN** the file MUST declare the `v0.22.0` asset set
- **AND** it MUST contain the four platform digest lines

#### Scenario: Isolated worktree-flow sync materializes the launcher

- **GIVEN** the isolated manifest enables `worktree-flow`
- **WHEN** isolated `sync` completes with `AI_SPECS_GATE_OFFLINE=1`
- **THEN**
  `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` MUST exist
- **AND** acquisition MUST NOT be required to pass the gate

### Requirement: Drift is a product defect

If isolated materialization, doctor, adapter, cache, lock, or
SHA256SUMS evidence fails, the failure SHALL be fixed in product
code, catalog, or tests. Refreshing this repository's dogfood lock
or generated files SHALL NOT be used to make the gate pass.

#### Scenario: Stale dogfood lock does not pass the gate

- **GIVEN** the candidate dogfood lock still reads `0.21.0`
- **AND** isolated sync stamps `0.22.0` on the temporary project
- **WHEN** the gate is evaluated
- **THEN** the isolated result MUST be the pass/fail signal
- **AND** the dogfood lock MUST remain unchanged
