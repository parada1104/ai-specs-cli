# sync-output-verbosity Specification

## Purpose

Define the output-verbosity contract shared by `ai-specs sync` and
`ai-specs sync-agent`: compact per-step output by default, full detail under
`--verbose`, and the errexit and temporary-file safety rules of the `run_step`
helper in `lib/sync.sh` and `lib/sync-agent.sh`.

## Requirements

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

`sync-agent` SHALL forward `--verbose` to each child when it resolves more than one
target and fans out to child `sync-agent` invocations, and SHALL NOT forward it
otherwise.

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

### Requirement: `run_step` restores errexit only after its own cleanup

`run_step` SHALL temporarily disable errexit while running the wrapped command,
capture the exit status, and restore errexit only after the helper has finished
printing captured output and removing its temporary files — not immediately
after capturing the status.

Restoring earlier means a failure inside the helper's own output handling (a
`cat` hitting SIGPIPE on an early-closed stdout, or a full disk) aborts the
script from inside the helper: the temporary files leak, the caller's error
handling never runs, and bash's status is returned instead of the wrapped
command's.

`run_step` SHALL NOT leave errexit disabled on return. `set` options are
shell-global rather than function-local, so a helper that disables errexit and
does not restore it silently disables it for the remainder of the script.

#### Scenario: errexit is active after a successful step
- **WHEN** `run_step` returns from a command that exited 0
- **THEN** errexit is enabled
- **AND** a subsequent failing command aborts the script

#### Scenario: errexit is active after a failed step
- **WHEN** `run_step` returns from a command that exited non-zero, in a context
  where the caller handles the failure
- **THEN** errexit is enabled for the statements that follow

#### Scenario: a bare failing step still aborts
- **WHEN** `run_step` is invoked without `if !` or `||` and its command fails
- **THEN** the script aborts
- **AND** the wrapped command's exit status is preserved

#### Scenario: a guarded failing step yields the real status
- **WHEN** `run_step` is invoked as `if ! run_step …` and its command exits 42
- **THEN** the caller observes 42

#### Scenario: temporary files do not survive a step
- **WHEN** `run_step` returns, for either a successful or a failing command
- **THEN** neither of its temporary files remains on disk

### Requirement: every capture block restores errexit after its own cleanup

The rule above SHALL apply to every block in `lib/sync.sh` and
`lib/sync-agent.sh` that disables errexit to capture a command's output — not
only to `run_step`. The hand-rolled `recipe-materialize` capture
(`lib/sync.sh:210-234`) predates `run_step` and is subject to the same
requirement.

Temporary files created by such a block SHALL be covered by a cleanup safety
net, so that an abort on any path cannot strand them.

#### Scenario: a failing `cat` in the recipe-materialize block
- **WHEN** the recipe-materialize step fails and printing its captured output
  fails as well
- **THEN** the script does not abort from inside the block before its cleanup
- **AND** the exit status reflects the step's own failure, not `cat`'s

#### Scenario: recipe-materialize temporary files are never stranded
- **WHEN** the script exits on any path after the recipe-materialize capture
  files are created
- **THEN** neither file remains on disk

### Requirement: a temporary-file failure names itself

`run_step` SHALL detect a `mktemp` failure and report it as such rather than
letting it surface as whatever abort message the wrapped command's caller
happens to produce. The step SHALL still run.

#### Scenario: unusable TMPDIR
- **WHEN** `mktemp` cannot create a file
- **THEN** a message naming the temporary-file failure is printed to stderr
- **AND** the wrapped command still runs and its exit status is preserved

### Requirement: Cross-stream ordering is not preserved (documented property, not a defect)

Because each step's stdout and stderr are captured separately and replayed per stream, the system SHALL
NOT guarantee the relative ordering of stdout lines against stderr lines within a
single step. Ordering within each individual stream SHALL be preserved.

#### Scenario: Within-stream order holds

- **GIVEN** a step writing three stdout lines in a known order
- **WHEN** the command runs in either mode
- **THEN** those three lines MUST appear in that same relative order
