# Design: Stabilize explicit workspace context across runtimes

- **Change slug**: `stabilize-workspace-context`
- **Depth**: full
- **Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/stabilize-workspace-context`
- **Reference generator**: `lib/_internal/hooks-render.py`
- **Reference launcher**: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
- **Reference parser**: `catalog/recipes/worktree-flow/gate/event.go`

## Design objective

Keep the workspace being evaluated separate from the directory that contains the
installed hook assets. An adapter may know an explicit target directory, but it must not
use that target directory as an implicit substitute for the launcher's installation
root. Conversely, the launcher must find its binary and legacy fallback beside itself,
even when its process was started from an unrelated cwd.

## Resolved context model

The implementation uses three distinct values:

| Context | Producer | Meaning | Fallback | Must not be used for |
|---------|----------|---------|----------|----------------------|
| Event cwd | Native runtime adapter and normalized event | Directory whose repository/worktree context the gate evaluates | Gate process cwd when the event value is invalid | Locating launcher assets |
| Installation root | Materialized launcher or generated module location | Root used to find project-local hook assets | No process-cwd substitution for project-local assets | Selecting the repository target for the event |
| Process cwd | Actual hook child process | Fallback context when the event cwd is unusable | The OS/runtime process cwd | Proving an explicit workspace root for Pi/OMP |

The boundary is:

```text
adapter event cwd ------------------------------> Go/Bash event normalization -> gate decision
                                                        ^
launcher/module installation root -> launcher assets and legacy fallback
```

The launcher may pass stdin unchanged and an adapter may set child cwd, but asset lookup
and event evaluation remain distinct operations.

## Current flow

```text
runtime event
    |
    | generated adapter
    +--> Claude: $CLAUDE_PROJECT_DIR/<script_path>
    +--> Cursor: $CURSOR_PROJECT_DIR/<script_path>
    +--> OpenCode: relative SCRIPT, event cwd = directory ?? process.cwd()
    +--> Pi/OMP: relative SCRIPT, event cwd = process.cwd()
    |
    v
worktree-gate.sh
    |
    +--> project-local Go binary from process $PWD
    +--> legacy Bash fallback from process $PWD
    +--> Go gate receives event and evaluates event cwd
```

The current OpenCode path has two independent problems: the event can name an explicit
directory while the child process continues with the parent process cwd, and the
relative `SCRIPT` is interpreted from that parent cwd. Pi and OMP have the same relative
script problem even though their event cwd must remain process cwd. The launcher has a
related problem because project-local and legacy assets are looked up under `$PWD`.

## Proposed adapter flow

### Claude and Cursor

`render_claude` and `render_cursor` retain their existing project-directory variables.
The implementation must not replace the correct explicit CLI/worktree target
propagation covered by `tests/test_worktree_root_propagation.py`. Cursor's lack of a
pre-file-write hook remains unchanged.

### OpenCode

`render_opencode` applies one deterministic directory normalizer before constructing the
event and the `spawnSync` options:

```text
normalizeDirectory(raw, processCwd):
  if raw is not a string: return processCwd
  trimmed = outer-trim(raw)
  if trimmed is not absolute: return processCwd
  if trimmed is not an existing directory: return processCwd
  return trimmed
```

The same returned value is used for both `event.cwd` and `spawnSync(..., { cwd })`.
Outer trimming changes only leading and trailing whitespace; internal path bytes are
preserved. An absent, non-string, whitespace-only, relative, nonexistent, or non-directory
`directory` therefore yields process cwd for both values.

The generated plugin resolves the materialized launcher from a runtime-supported absolute
module location, preferring derivation from `import.meta.url` or an equivalent module
location API. It must not emit a relative `SCRIPT`, and it must not embed a machine-
specific absolute path produced at sync time. The process-boundary test must prove that a
plugin relocated into a temporary installation and launched from an unrelated cwd still
invokes the intended launcher.

The child invocation keeps the existing input JSON and status mapping. A `spawnSync`
error, thrown child-process exception, missing status, or other non-status failure is
caught and treated as fail-open. Status `2` remains the only blocking result; all other
statuses remain non-blocking.

OpenCode's pre-tool hook still does not cover subagent or MCP calls. This change does not
claim coverage for those calls.

### Pi and OMP

`render_pi` and `render_omp` use the same module-location asset-path stabilization when
their generated extensions have the relative `SCRIPT` defect. The generated launcher
path is absolute at runtime through `import.meta.url` or an equivalent supported module
location mechanism, not a sync-time machine path.

Their event remains `cwd: process.cwd()`. No runtime field is promoted to an authoritative
workspace root, and no explicit workspace propagation is claimed. Their child process
continues to inherit process cwd unless the runtime contract later proves a stronger
value, which is outside this change.

## Proposed Bash launcher flow

`worktree-gate.sh` derives the installation root from `BASH_SOURCE[0]`, not from `$PWD`:

1. Read the executing source reference from `BASH_SOURCE[0]`.
2. If it is relative, prefix it with the invocation process cwd only to form an initial
   absolute reference. This is the only point where process cwd participates in locating
   the launcher itself.
3. Follow the final launcher symlink and any parent symlinks to a physical launcher path.
   A Bash 3.2-compatible loop may use the host `readlink` utility. If the physical path
   cannot be resolved, mark the installation root unusable and continue with the existing
   cache/override/fail-open paths; never guess from `$PWD`.
4. Set `launcher_dir` to the physical launcher's directory and derive
   `recipe_root = launcher_dir/..` using a physical directory change.
5. Resolve the project-local binary as
   `recipe_root/bin/worktree-gate` and the legacy fallback as
   `recipe_root/hooks/worktree-gate-legacy.sh`.

For the materialized layout, `launcher_dir` is `.../worktree-flow/hooks` and
`recipe_root` is `.../worktree-flow`, so `hooks/../bin` is the required project-local
asset relationship. Relative invocation and symlinked invocation both resolve to the
physical target installation. If a symlink cannot be followed, no project-local asset is
selected from the symlink directory or process `$PWD`.

The existing precedence remains unchanged:

1. Executable `WORKTREE_GATE_BIN` override.
2. Project-local Go binary under the derived installation root.
3. Versioned `AI_SPECS_HOME` cache binary.
4. Legacy Bash fallback under the derived installation root when stamped `gate_impl` is
   `bash` or permitted `auto` fallback.
5. Diagnostic on stderr and exit `0` when no implementation is usable.

Stamped values, `WORKTREE_GATE_PROTECTED`, `WORKTREE_GATE_MODE`, scope behavior, stdin,
`exec`, exit codes, and Bash 3.2 syntax remain compatible. `$PWD` is still the process
cwd supplied to the gate as invalid-event-cwd fallback; it is not a project-local asset
root.

## Go and Bash cwd normalization

The Go `eventCwd` path trims only outer whitespace from a string before checking that it
is an absolute existing directory. It returns the trimmed bytes unchanged otherwise. A
non-string, empty, whitespace-only, relative, nonexistent, or non-directory value returns
the supplied process cwd.

The legacy Bash path retains its existing outer trim and validation behavior. Tests must
prove equivalent normalized values and, where repository fixtures differ, equivalent gate
decisions. No path cleaning, slash normalization, internal-space normalization, or new
repository fallback is permitted.

## Failure and fallback behavior

| Condition | Required behavior |
|-----------|-------------------|
| Invalid OpenCode `directory` | Use process cwd for event and child `spawnSync` cwd |
| OpenCode child error or throw | Catch, warn if the existing adapter channel permits, and fail open |
| Missing project-local binary | Continue through cache and legacy precedence |
| Missing or unusable binary with permitted legacy fallback | Execute legacy script under the installation root |
| Unresolvable Bash installation root | Do not use `$PWD` for local assets; continue to cache or fail open |
| No usable implementation | Warn on stderr and exit `0` |
| Binary self-test failure on opt-in verification | Warn and exit `0` without blocking the editor |
| Ambiguous repository/worktree context | Preserve the existing gate fail-open decision |
| Malformed or unsupported event | Preserve existing fail-open event behavior |

## Compatibility considerations

- The materialized launcher filename and `script_path` contract do not change.
- Claude and Cursor project-directory variables remain in generated output.
- OpenCode continues to block only on status `2`; non-blocking statuses remain fail-open.
- Pi and OMP retain process-cwd-only event semantics and make no workspace-authority
  claim.
- Cursor's lack of a pre-file-write hook remains unchanged.
- OpenCode's subagent and MCP coverage gap remains unchanged.
- Explicit CLI/worktree target propagation remains covered by
  `tests/test_worktree_root_propagation.py`.
- Cleanup-root mechanics, repository policy, topology rules, and gate exit codes remain
  outside this change.
- `docs/runtime-hooks.md:133-138` is updated so its runtime-flow description matches
  the generated adapter and launcher behavior instead of claiming zero renderer change.

## Implementation proof gates

Product choices are resolved. Implementation is accepted only after these proof gates:

1. A deterministic Node process-boundary harness observes normalized OpenCode event cwd,
   child `spawnSync` cwd, absolute module-derived launcher path, status `2`, invalid
   directory fallback, and thrown-child fail-open behavior.
2. Launcher fixtures prove `BASH_SOURCE[0]` behavior for relative invocation, symlinked
   invocation, `hooks/../bin`, override precedence, unrelated process cwd, and legacy
   fallback.
3. Direct Go `ParseEvent` table tests and Bash decision-differentiating fixtures prove
   outer-whitespace parity for path and shell events.
4. The runtime documentation update at `docs/runtime-hooks.md:133-138` and recipe docs
   accurately state the final contract and limitations.
5. `go -C catalog/recipes/worktree-flow/gate test ./...` and `./tests/validate.sh` pass.

## Test strategy

### OpenCode process boundary

Use a deterministic Node harness, planned at
`tests/fixtures/workspace-context/opencode_process_boundary.mjs`, with a supported
`spawnSync` test double at the child-process boundary. The test renders the generated
plugin into a temporary installation, loads it through the same supported module form as
the runtime, and records the executable path, input JSON, options cwd, and returned
status. Cases cover:

- valid explicit directory surrounded by outer whitespace;
- absent, non-string, relative, nonexistent, and non-directory directory;
- plugin launched from an unrelated process cwd;
- status `2` block mapping;
- child `spawnSync` error and throw, both fail-open;
- Pi/OMP module-location asset resolution while their event cwd remains process cwd.

The test must execute the generated module and observe the double's inputs. A regex,
substring, or source-text assertion alone is not sufficient evidence.

### Launcher and parity

Launcher tests use temporary installations and temporary process directories. They invoke
the real Bash launcher through relative and symlinked paths, assert `BASH_SOURCE[0]`
physical-root behavior and `hooks/../bin`, and use an executable marker or controlled
implementation to prove which candidate was selected. The parity fixture creates a
protected main checkout and a feature or linked-worktree process context so a trimmed
event cwd produces a block while process-cwd fallback would allow. This makes whitespace
normalization decision-differentiating rather than merely textual.

Direct Go tests live in the deliberately new
`catalog/recipes/worktree-flow/gate/event_cwd_test.go`; no pre-existing `event_test.go`
is assumed. `TestParseEventCwdNormalization` uses table-driven `ParseEvent` cases for
path and shell events and asserts the returned `Event.Cwd` directly.
