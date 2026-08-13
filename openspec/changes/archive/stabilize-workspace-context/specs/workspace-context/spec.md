# workspace-context Specification

## Purpose

Boundary between runtime event context and launcher asset context for the worktree-flow hook. Event cwd, installation root, process cwd MUST stay distinct; gate policy and fail-open unchanged.

## Requirements

### Requirement: Launcher root from BASH_SOURCE

The Bash launcher MUST derive its installation root from `BASH_SOURCE[0]`, not `$PWD`, supporting relative and symlinked invocation, resolving assets as `hooks/../bin`. With no usable implementation it MUST warn on stderr and exit `0`.

#### Scenario: Project-local binary

- GIVEN a launcher with a binary under its root
- WHEN the hook starts elsewhere
- THEN it selects `hooks/../bin` under that root

#### Scenario: Relative or symlinked invocation

- GIVEN relative or symlinked launcher invocation
- WHEN process cwd differs
- THEN it resolves to the physical target installation

### Requirement: Module-derived adapter assets

OpenCode, Pi, and OMP extensions MUST resolve the launcher from a runtime-supported absolute module location (`import.meta.url` or equivalent), never a relative `SCRIPT` or sync-time machine-specific path.

#### Scenario: Relocated extension

- GIVEN a relocated generated extension
- WHEN it handles an event from an unrelated cwd
- THEN the child executable path is module-derived

#### Scenario: Pi and OMP semantics

- GIVEN a Pi or OMP extension handles a tool call
- THEN its launcher path is module-derived
- AND event cwd stays `process.cwd()`

### Requirement: OpenCode explicit directory

The OpenCode adapter MUST outer-trim `directory`, require a string absolute existing directory, and use the same normalized value for event cwd and child `spawnSync` cwd. Invalid input falls back to process cwd for both; child errors and throws fail open, with status `2` the only block.

#### Scenario: Valid directory

- GIVEN a string directory absolute and existing after outer trim
- WHEN a `tool.execute.before` event is handled
- THEN event cwd and `spawnSync` cwd equal the trimmed value

#### Scenario: Invalid directory

- GIVEN `directory` is absent, non-string, relative, or nonexistent
- WHEN a matching event is handled
- THEN event cwd and `spawnSync` cwd become process cwd

#### Scenario: Node boundary proof

- GIVEN a generated plugin runs from an unrelated cwd
- WHEN a `spawnSync` test double records the invocation
- THEN executable path, event cwd, options cwd, status are observed

### Requirement: Go and Bash cwd parity

Go `ParseEvent` and legacy Bash MUST trim outer whitespace only, accept a trimmed absolute existing directory, and fall back to process cwd for unusable values. Neither MAY clean paths or add installation-root fallback.

#### Scenario: Trimmed cwd accepted

- GIVEN an event with an absolute existing directory wrapped in whitespace
- WHEN Go or Bash normalizes
- THEN both use the trimmed directory and match decisions

#### Scenario: Decision-differentiating trim

- GIVEN a whitespace-wrapped protected event cwd and allowing process cwd
- WHEN Go and Bash normalize
- THEN both trim and block the protected path

### Requirement: Harness limitations

Claude and Cursor MUST keep their project-directory variables. Cursor MUST remain without a pre-file-write hook. OpenCode MUST NOT claim subagent or MCP coverage. Pi and OMP MUST keep process-cwd-only events and MUST NOT claim a workspace root.

#### Scenario: Cursor limitation

- GIVEN a Cursor pre-file-write hook target
- WHEN hooks are rendered
- THEN the renderer keeps warning-and-skip behavior

### Requirement: Documentation contract

The runtime hook documentation MUST describe the three-context model, module-derived adapter paths, `BASH_SOURCE[0]` resolution, OpenCode validation and fallback, and unchanged limitations. The stale `docs/runtime-hooks.md:133-138` MUST be updated.

#### Scenario: Docs match behavior

- GIVEN the implementation and adapters satisfy this spec
- WHEN a reviewer reads `docs/runtime-hooks.md:133-138` and recipe docs
- THEN they state the final asset-root, fallback, limitation behavior

### Requirement: Compatibility stability

The change MUST preserve explicit CLI/worktree target propagation covered by `tests/test_worktree_root_propagation.py`, launcher filename, gate exit codes, stamped values, stdin, and OpenCode status mapping.

#### Scenario: Propagation stays green

- GIVEN the existing propagation scenarios run
- WHEN adapters and launcher are exercised
- THEN all existing assertions pass
