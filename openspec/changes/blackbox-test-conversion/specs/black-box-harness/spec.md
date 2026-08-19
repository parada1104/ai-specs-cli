# Delta for the black-box test harness

This delta defines what counts as a valid black-box conversion. It is the
contract every conversion work unit is executed and verified against, and it
governs no production behavior — the CLI is not modified by this change.

## ADDED Requirements

### Requirement: The process boundary is `bin/ai-specs`

A converted test MUST exercise the CLI through `bin/ai-specs <verb>` as a
subprocess. It MUST NOT import a module from `lib/_internal/`, and MUST NOT
invoke a `lib/*.sh` script directly.

Both couplings die on the Go port: `lib/_internal/*.py` and `lib/*.sh` are both
replaced by the single binary. A test that merely stops using
`spec_from_file_location` while still shelling out to `lib/doctor.sh` satisfies
the card's literal wording and none of its intent.

Exactly one exception is permitted: the cache-key parity assertion (see below),
because comparing against the implementation IS its assertion.

#### Scenario: A converted test drives the CLI

- **GIVEN** a test that previously called an internal Python function
- **WHEN** it is converted
- **THEN** it invokes `bin/ai-specs <verb>` as a subprocess
- **AND** it asserts on some combination of exit code, emitted file tree, and output
- **AND** `grep -c 'spec_from_file_location\|load_module'` over the file returns 0

#### Scenario: A test shelling out to a lib script is treated as coupled

- **GIVEN** a test invoking `bash lib/skills-add.sh`
- **WHEN** conversion scope is determined
- **THEN** that test is in scope and is converted to `bin/ai-specs skills add`

### Requirement: One CLI home per command sequence

`AI_SPECS_HOME` is simultaneously the CLI install root — `bin/ai-specs` resolves
`lib/*.sh` beneath it — and the cache root, `$AI_SPECS_HOME/cache/projects/<key>`.
There is no separate cache variable.

A test that runs more than one CLI command in a single scenario MUST share one
`cli_home` across every invocation in that scenario, built by `isolated_home()`.

Pointing `AI_SPECS_HOME` at an empty directory MUST be treated as a defect: the
CLI cannot find its own code and exits 127 with empty output.

#### Scenario: A command sequence shares one home

- **GIVEN** a test that runs `sync` and then `doctor` against one project
- **WHEN** each invocation builds its own isolated home
- **THEN** `doctor` reports 7 phantom `bundled-skill ... missing` ERRORs and exits 1
- **WHEN** both invocations share one home from `isolated_home()`
- **THEN** `doctor` exits 0 and reports `0 ERROR`
- **AND** only the shared-home result is a valid assertion target

### Requirement: Current behavior is frozen, defects included

A converted test MUST assert the behavior the CLI exhibits today, as recorded in
`docs/go-migration-parity-contract.md`, including behavior that document records
as a defect (D1-D35).

A conversion MUST NOT "correct" a FROZEN behavior it finds wrong. Each defect is
a separate card, and a defect card updating the test that froze it is expected
work, not a regression.

#### Scenario: A conversion meets a recorded defect

- **GIVEN** a test covering behavior recorded as a defect in the parity contract
- **WHEN** it is converted
- **THEN** the converted test asserts the current, defective behavior
- **AND** the conversion does not change any file under `lib/` or `bin/`

### Requirement: Assertion intent is preserved, never silently dropped

A conversion MUST preserve the intent of every existing assertion. Where an
assertion stood in for an observable effect, it is replaced by that effect.

Where an assertion has NO observable equivalent, the original test MUST be left
in place carrying a `# TRIAGE:` comment naming what it covers and why nothing
observable captures it. It MUST NOT be deleted, and an approximation MUST NOT be
invented in its place.

No test is deleted without explicit human approval, and approval is requested
once, as a single list covering every such case.

#### Scenario: An assertion has no observable equivalent

- **GIVEN** a test that mocks a function and asserts it was called
- **WHEN** conversion reaches it
- **THEN** the test is left in place with a `# TRIAGE:` marker naming its coverage
- **AND** it is added to the list awaiting human approval
- **AND** it is not deleted, and no approximation replaces it

### Requirement: The cache key is asserted against the implementation

Cache key derivation is FROZEN (parity contract §4): a port that derives it
differently silently orphans every existing project's cache.

The harness MUST assert its independently derived key equals
`lib/_internal/project-cache.py::cache_key` across names exercising boundary
punctuation. A weaker assertion — such as checking the key merely does not end
in punctuation — MUST be treated as absent, because it passes on a derivation
that is wrong in every other respect.

#### Scenario: Boundary punctuation is compared, not approximated

- **GIVEN** project names `_leading`, `trailing_`, `.dotfile`, `a.b.`, `__x__`, `normal`
- **WHEN** the parity assertion runs
- **THEN** the helper's key equals the implementation's key for every one of them
- **AND** the test is marked as the single permitted coupled assertion

### Requirement: The suite stays green against the unmodified implementation

Every work unit MUST leave `./tests/run.sh` at exit 0 against the untouched
Bash/Python implementation. The reference is exit 0 with `OK (skipped=116)`.

A work unit's completion MUST be established by running the suite and reading a
real exit code, never by a report asserting success.

#### Scenario: A work unit reports completion

- **GIVEN** a work unit that claims to be complete
- **WHEN** completion is assessed
- **THEN** `./tests/run.sh` is executed and its exit code read from a redirected file
- **AND** a claim of success unaccompanied by that exit code is rejected

### Requirement: A conversion never reduces assertion count unjustified

A converted file MUST NOT end with fewer assertions than it started with, unless
every removed assertion carries a written justification naming what it covered
and why the coverage is preserved elsewhere or acceptably lost.

Failure-path assertions are the highest risk: `assertNotEqual(returncode, 0)`,
`assertEqual(returncode, 1)`, `assertEqual(returncode, 2)`, and assertions on
error text. When a converted invocation no longer produces the failure the
original exercised, the correct response is to reconstruct the failure state —
never to rewrite the assertion to match whatever the new invocation happened to
return.

A test that passes because its assertion was adjusted to the observed result is
worse than a deleted test: it reports coverage it does not have, and the harness
built from it will certify a Go port that broke the behavior.

#### Scenario: A converted test no longer reproduces its failure state

- **GIVEN** `test_missing_ai_specs_home`, which asserted a non-zero exit and `install.sh` in stderr
- **WHEN** conversion to `bin/ai-specs upgrade` yields exit 0 and "up to date"
- **THEN** the conversion is rejected
- **AND** the correct fix reconstructs the missing-install state so the failure path is still exercised
- **AND** rewriting the assertion to `assertEqual(returncode, 0)` is never acceptable

#### Scenario: Assertion count is checked before a conversion is accepted

- **GIVEN** a work unit reporting a converted file
- **WHEN** the orchestrator validates it
- **THEN** removed and added assertion counts are compared per file
- **AND** any net reduction without per-assertion justification blocks acceptance

### Requirement: Executed test count is the primary acceptance metric

A converted file MUST NOT run fewer tests than it ran before conversion.

Skipping is not converting. `unittest` reports a skipped test as success, so a
module that skips everything reports `Ran 0 tests ... OK` and exits 0. Coupling
counts, assertion counts, and the exit code can all pass while coverage is
entirely gone.

`@unittest.skip`, `self.skipTest`, `raise unittest.SkipTest`, and module-level
skip guards are therefore treated as coverage deletion, and coverage deletion
needs the same human approval as deleting a test.

Where a test cannot yet be converted, the correct action is to LEAVE IT COUPLED
and unconverted, carrying a `# TRIAGE:` comment. A coupled test that still runs
is strictly better than a black-box test that does not: it keeps its coverage
until a real conversion replaces it.

#### Scenario: A conversion skips what it cannot convert

- **GIVEN** `test_recipe_materialize.py` running 65 tests before conversion
- **WHEN** conversion removes every loader reference but skips the module
- **THEN** the file reports `Ran 0 tests ... OK` and exits 0
- **AND** coupled references read 0 and assertion count is unchanged
- **AND** the conversion is REJECTED, because 65 tests became 0

#### Scenario: Acceptance is measured before a conversion is merged

- **GIVEN** a work unit reporting converted files
- **WHEN** the orchestrator validates it
- **THEN** the executed test count per file is compared against its pre-conversion count
- **AND** any decrease blocks acceptance regardless of the other three metrics
