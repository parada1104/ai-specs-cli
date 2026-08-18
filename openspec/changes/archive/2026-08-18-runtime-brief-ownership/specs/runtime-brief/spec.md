# Specification: runtime-brief

Requirements for deciding whether `ai-specs` may write `AGENTS.md`.

## MODIFIED Requirements

### Requirement: the write decision is governed by provenance

Whether `AGENTS.md` is written SHALL be decided by classifying the target with
the same ownership states used for managed overrides — from disk bytes, the
recorded baseline, and the bytes that would be written — never by the presence
of a marker alone.

The decision SHALL live inside the renderer, so that every entry point reaches
an identical result for identical inputs.

#### Scenario: a pre-existing hand-written brief survives
- **GIVEN** a repository with an `AGENTS.md` that ai-specs has never written,
  and no runtime-brief marker
- **WHEN** `ai-specs init`, `ai-specs sync`, or `ai-specs sync-agent` runs
- **THEN** the file is left byte-for-byte unchanged
- **AND** the command reports that it was left alone and why

#### Scenario: an edited generated brief survives
- **GIVEN** a brief ai-specs wrote, which the user has since edited
- **WHEN** any of the three commands runs
- **THEN** the file is left unchanged and the command says so

#### Scenario: an untouched generated brief still updates
- **GIVEN** a brief ai-specs wrote and the user has not edited
- **WHEN** the manifest changes and sync runs
- **THEN** the brief is regenerated with no prompt and no extra output
- **AND** the new bytes are recorded as the baseline

#### Scenario: a missing brief is created
- **WHEN** no `AGENTS.md` exists
- **THEN** it is rendered and its bytes recorded

#### Scenario: every entry point agrees
- **GIVEN** identical disk, lock, and manifest state
- **WHEN** the decision is computed for `init`, `sync`, and `sync-agent`
- **THEN** all three produce the same outcome

### Requirement: adoption is proven or user-initiated, never inferred

A target with no recorded baseline SHALL be adopted automatically **only** when
its bytes exactly equal the bytes that would be written. In every other case it
SHALL be preserved.

Adoption SHALL otherwise require an explicit user action.

#### Scenario: an up-to-date brief adopts silently
- **GIVEN** no baseline, and a brief identical to what would be written
- **WHEN** sync runs
- **THEN** the baseline is recorded and the sync proceeds with no extra output

#### Scenario: a stale brief is preserved, not adopted
- **GIVEN** no baseline, and a brief that differs from what would be written
- **WHEN** sync runs
- **THEN** the file is left unchanged
- **AND** the reported remedy names both adopting it and claiming it permanently

#### Scenario: explicit adoption is honored
- **GIVEN** a brief with no baseline
- **WHEN** the user runs sync with the adopt option
- **THEN** the current bytes become the baseline
- **AND** subsequent syncs treat it as a managed brief

### Requirement: a skipped write is always reported

A decision not to write SHALL print the detected state and the available
remedies. It SHALL NOT be silent.

#### Scenario: the message names both exits
- **WHEN** a write is skipped because the brief is not ours
- **THEN** the output names how to hand management to ai-specs
- **AND** names how to keep the file permanently user-owned

### Requirement: the marker remains an unconditional opt-out

A file containing the runtime-brief marker SHALL be preserved regardless of
classification, so that projects relying on it are unaffected.

#### Scenario: the marker still wins
- **GIVEN** a brief containing the marker, with a baseline recorded and bytes
  that differ from what would be written
- **WHEN** sync runs
- **THEN** the file is left unchanged

### Requirement: ownership is reported by doctor

`ai-specs doctor` SHALL report the runtime brief's ownership state.

#### Scenario: an unadopted brief is discoverable
- **GIVEN** a project whose brief is preserved for lack of a baseline
- **WHEN** `ai-specs doctor` runs
- **THEN** the state is reported with the same remedy

### Requirement: undetermined ownership never writes

If classification cannot be completed — an unreadable lock, an unreadable
target — the target SHALL be preserved.

#### Scenario: an unreadable lock preserves the brief
- **GIVEN** a lock file that cannot be parsed
- **WHEN** sync runs
- **THEN** `AGENTS.md` is not written
- **AND** the command does not fail with a traceback

### Requirement: `[brief].render = false` is unchanged

The existing switch SHALL continue to skip rendering entirely, independent of
classification.

#### Scenario: rendering stays disabled
- **GIVEN** `[brief].render = false`
- **WHEN** any of the three commands runs
- **THEN** no classification is performed and no write is attempted
