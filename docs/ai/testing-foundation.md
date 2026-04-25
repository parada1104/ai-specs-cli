# Testing Foundation

## Purpose

Testing is part of the ai-specs MVP as a cross-cutting quality foundation. It is
not extra functional scope for every card; it is the minimum evidence needed to
trust each change.

## Canonical Commands

Use these commands from the repository root:

- `./tests/run.sh` runs the Python unittest suite.
- `./tests/validate.sh` runs syntax checks and then `./tests/run.sh`.

`./tests/validate.sh` is the default final verification command for normal
changes until stronger tooling is configured.

## Minimum Test Layers

Changes should use the smallest relevant set of these layers:

- Unit tests for pure parsing, normalization, rendering, and validation logic.
- Integration-style CLI or sync tests for generated files and command flows.
- Documentation/generated artifact tests when README, AGENTS.md, templates, or
  contract docs are the source of truth.
- Smoke validation through `./tests/validate.sh` before a change is considered
  ready to merge.

## Evidence Policy

Every implementation should leave clear evidence of what was run and what
passed. In SDD changes, record RED/GREEN and final validation in
`apply-progress.md` and `verify-report.md`. In non-SDD changes, record the same
evidence in the commit, PR, or task/card update.

## Deferred Tooling

Coverage, linting, type checking, and formatting are desirable but not blocking
signals until they are configured in this repository. Do not imply those checks
passed unless the tooling exists and was actually run.
