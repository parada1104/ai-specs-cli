# sync-output-verbosity (delta)

## ADDED Requirements

### Requirement: Compact step output is the default

`ai-specs sync` and `ai-specs sync-agent` SHALL, by default, print one
`  syncing <label>` line per step and SHALL suppress each captured output line whose
first non-whitespace character is a success/detail marker (`✓`, `·`, `⇢`, `▸`). Every
other non-blank captured line SHALL be printed unchanged, on its original stream.

#### Scenario: Success detail is suppressed

- **GIVEN** a step whose captured stdout contains `    ✓ bundled skill worktree-flow`
- **WHEN** the command runs without `--verbose`
- **THEN** stdout MUST contain `  syncing <label>`
- **AND** stdout MUST NOT contain the `✓ bundled skill` line

#### Scenario: Warnings and notices survive compaction

- **GIVEN** a step whose captured output contains a line starting with `!`, `✗`, or `ℹ`
- **WHEN** the command runs without `--verbose`
- **THEN** each such line MUST appear in the output, byte-identical to the original
- **AND** it MUST appear on the same stream (stdout or stderr) the step wrote it to

#### Scenario: Blank lines are dropped in compact mode

- **GIVEN** a step whose captured output contains blank or whitespace-only lines
- **WHEN** the command runs without `--verbose`
- **THEN** those lines MUST NOT be printed

### Requirement: Verbose flag restores full detail

Both commands SHALL accept `-v` and `--verbose`. In verbose mode the system SHALL print
each step's captured output unfiltered and byte-identical to what the step produced.

#### Scenario: Verbose is byte-identical to the step's own output

- **GIVEN** a step that emits a mix of `✓`, `·`, and `!` lines
- **WHEN** the command runs with `--verbose`
- **THEN** every line MUST appear, in the step's original order within each stream

#### Scenario: Unknown flags still fail

- **GIVEN** an unrecognized flag such as `--verbos`
- **WHEN** either command is invoked with it
- **THEN** the command MUST exit non-zero with an `unknown flag` message

### Requirement: Failure always prints full unfiltered output

When a step exits non-zero, the system SHALL print that step's complete captured stdout
and stderr without filtering, in both compact and verbose mode, before propagating the
step's exit status to the caller's existing error handling.

#### Scenario: Compact mode does not hide a failure diagnosis

- **GIVEN** a step that writes diagnostic `✓` and `·` lines and then exits 1
- **WHEN** the command runs without `--verbose`
- **THEN** the full captured stdout MUST be printed, including the marker lines
- **AND** the full captured stderr MUST be printed on stderr
- **AND** the command MUST exit with the step's non-zero status

### Requirement: Verbose propagates through public-root fan-out

When `sync-agent` resolves more than one target and fans out to child `sync-agent`
invocations, it SHALL forward `--verbose` to each child when the parent was invoked with
it, and SHALL NOT forward it otherwise.

#### Scenario: Children inherit the parent's mode

- **GIVEN** a public root resolving to two targets
- **WHEN** `sync-agent --verbose` runs
- **THEN** each child invocation MUST receive `--verbose` in its argument list

### Requirement: Nested runs do not repeat the banner

A `sync-agent` run executing as a fan-out child SHALL NOT print the
`ai-specs sync-agent` header block or the `✓ sync-agent complete` footer. The parent
SHALL own that framing. Suppression SHALL be signalled by the `AI_SPECS_SYNC_NESTED`
environment variable being `1`.

#### Scenario: Child output carries no banner

- **GIVEN** a public root resolving to two targets
- **WHEN** the fan-out runs
- **THEN** `ai-specs sync-agent` header MUST appear exactly once in the combined output
- **AND** `✓ sync-agent complete` MUST NOT appear for either child

### Requirement: Fan-out terminates after dispatching children

After dispatching one child `sync-agent` per resolved target, the parent SHALL terminate
successfully and SHALL NOT execute an additional sync pass of its own.

#### Scenario: No duplicate parent pass

- **GIVEN** a public root resolving to two targets
- **WHEN** `sync-agent` runs with neither `--source-root` nor `--target`
- **THEN** exactly two child `sync-agent` invocations MUST occur
- **AND** the parent MUST NOT perform a further materialize/render pass after the loop
- **AND** the parent MUST exit 0

#### Scenario: First child failure stops the fan-out

- **GIVEN** a public root resolving to two targets where the first child fails
- **WHEN** the fan-out runs
- **THEN** the second child MUST NOT be invoked
- **AND** the parent MUST exit non-zero
- **AND** stderr MUST report the failing target path

### Requirement: Notices that must survive compaction do not use suppressed markers

Any line the system intends the user to read in compact mode SHALL NOT begin with a
suppressed marker (`✓`, `·`, `⇢`, `▸`). Informational notices SHALL use `ℹ`.

#### Scenario: Skip notices are visible in compact mode

- **GIVEN** a manifest with no `[mcp.*]` entries
- **WHEN** `sync-agent` runs without `--verbose`
- **THEN** the "mcp skipped" notice MUST appear in the output

- **GIVEN** a manifest with `[brief].render = false`
- **WHEN** `sync-agent` runs without `--verbose`
- **THEN** the "skipped AGENTS.md" notice MUST appear in the output

## Documented properties (not defects)

### Requirement: Cross-stream ordering is not preserved

Because each step's stdout and stderr are captured separately and replayed per stream,
the system SHALL NOT guarantee the relative ordering of stdout lines against stderr lines
within a single step. Ordering within each individual stream SHALL be preserved.

#### Scenario: Within-stream order holds

- **GIVEN** a step writing three stdout lines in a known order
- **WHEN** the command runs in either mode
- **THEN** those three lines MUST appear in that same relative order
