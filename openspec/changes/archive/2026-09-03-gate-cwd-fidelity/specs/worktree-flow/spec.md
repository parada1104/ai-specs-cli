# Delta for worktree-flow

## Scope

This delta changes only the **worktree-flow Go shell gate** cwd-fidelity behavior
(`catalog/recipes/worktree-flow/gate/` — `main.go`, `decide.go`, tokenizer, and
`message.go`). The Go gate is shared by both hook surfaces (`worktree-gate` and
`worktree-gate-shell` via the same `worktree-gate.sh` launcher), so one source
covers both.

**Explicitly unchanged (frozen contracts):**

- Pi/OMP event cwd semantics remain `process.cwd()`-only and MUST NOT claim a
  workspace root — pinned by `openspec/specs/workspace-context` and
  `tests/test_hooks_render.py::_assert_process_cwd_event`. This delta MUST NOT
  touch `lib/_internal/hooks-render.py` cwd semantics.
- The doctor version-drift check (`doctor.py::_check_worktree_gate`) and
  `stamped_gate_version` are already implemented and out of scope.
- The `worktree-gate.sh` launcher contract (bash 3.2, synchronized) is not
  changed by this delta; the Go gate does the work.

## ADDED Requirements

### Requirement: Shell-gate command cwd fidelity

The worktree-flow Go gate MUST recover the effective working directory that the
gated command itself determines — `git -C <dir> ...` and compound
`cd <dir> && ...` forms — and MUST use that recovered command cwd as the
authoritative base for absolutizing relative candidate write paths, taking
precedence over the event cwd. The gate MUST perform this recovery before any
candidate path is absolutized or classified. Because both hook surfaces share
the same launcher and gate binary, the contract MUST hold identically for
structured file-path events and shell-command events.

The gate is not a shell interpreter: it MUST recover only statically present
`git -C` / `cd` directory operands from the command text. Dynamic control flow
(`cd -`, variable-derived destinations, subshell indirection) yields no
recovered cwd and falls to the degrade requirement below.

#### Scenario: git -C relative paths evaluate against the worktree, not the host

- GIVEN the host session process cwd is the main checkout on a protected branch
- AND the event cwd is that main checkout (Pi/OMP report `process.cwd()`)
- AND the shell command is `git -C <worktree> mv <relative-planning-paths>`
  where the relative paths resolve inside the linked worktree
- WHEN the gate evaluates the candidates
- THEN it MUST absolutize the relative paths against `<worktree>` recovered
  from `git -C`, NOT against the host `process.cwd()`
- AND the decision MUST be `allow`, matching the decision the same command
  would receive with the event cwd set to the worktree

#### Scenario: Compound cd command uses the cd target as base

- GIVEN the event cwd is the main checkout on a protected branch
- AND the shell command is `cd <worktree> && echo x > <relative-path>` where
  the relative path resolves inside the worktree
- WHEN the gate evaluates the candidate
- THEN it MUST resolve the candidate against `<worktree>` from the `cd`
  operand
- AND it MUST NOT block because of the main-checkout event cwd

#### Scenario: Both hook surfaces inherit the fix from one source

- GIVEN the fix is implemented in the shared Go gate
- WHEN the same `git -C <worktree> mv <relative>` command is evaluated through
  the `worktree-gate` file-write surface and the `worktree-gate-shell` surface
- THEN both surfaces MUST reach the same decision from the same command-cwd
  recovery
- AND no launcher-specific duplication of the recovery logic is required

### Requirement: Honest degrade when effective cwd is unrecoverable

When the gate cannot recover an effective cwd — neither from the command text
nor from a usable event cwd — and a candidate write path is relative, the gate
MUST degrade to a warn/ask outcome instead of block-on-guess against the host
`process.cwd()`. The gate MUST NOT absolutize a relative candidate against the
host process cwd as a basis for a blocking decision. When the candidate path is
absolute, classification MUST proceed unchanged. When a cwd IS recoverable
(command cwd or usable event cwd), the existing block-on-trust protected-branch
decision MUST be preserved.

The degrade outcome MUST follow the configured `gate_mode` (warn/ask
presentation per existing mode semantics); it MUST NOT introduce a new bypass
surface and MUST NOT announce `WORKTREE_GATE_MODE=off`.

#### Scenario: Unrecoverable cwd with relative path degrades instead of blocking

- GIVEN the event cwd is the main checkout on a protected branch
- AND the command contains no recoverable `git -C` or `cd` directory
- AND a relative candidate write path resolves, against the event cwd, into
  that protected checkout
- AND the gate cannot establish that the command will actually execute there
- WHEN the gate evaluates the candidate
- THEN it MUST NOT exit `2` with a protected-branch block derived from the
  host process cwd guess
- AND it MUST degrade to the warn/ask outcome for the configured `gate_mode`

#### Scenario: Absolute candidate path classifies unchanged

- GIVEN any cwd state (recoverable or not)
- AND the candidate write path is absolute and resolves inside a protected
  primary checkout on a protected branch
- WHEN the gate evaluates the candidate
- THEN it MUST block with exit `2` exactly as before this change

#### Scenario: Recoverable cwd keeps block-on-trust

- GIVEN the command is `cd <protected-primary> && echo x > file.txt` (or
  `git -C <protected-primary> mv a b`)
- WHEN the gate recovers the command cwd
- THEN the relative candidates MUST be resolved against that recovered cwd
- AND the protected-branch decision MUST block with exit `2` as before

## MODIFIED Requirements

### Requirement: Shell Command Write-Bypass Detection

The `worktree-flow` worktree gate (`hooks/worktree-gate.sh`) MUST accept shell/bash
command strings in addition to structured file paths. When no
`tool_input.file_path` / `tool_input.notebook_path` is present, the gate MUST
extract a command string from stdin JSON using this precedence (first non-empty
string wins): `tool_input.command` → `tool_input.script` → `tool_input.cmd` →
top-level `command` → top-level `script`. Top-level `command`/`cwd` MUST be
accepted so Cursor's native `beforeShellExecution` payload works without wrapper
normalization.

The gate MUST run best-effort, high-precision write-redirection heuristics over
the command string and collect candidate write paths from at least:

- shell redirections `>` / `>>` targeting a path token
- `tee` / `tee -a` destination operands
- in-place editors `sed -i` / `perl -i` (last non-flag path operand)
- `cp` / `mv` destination (last non-flag path operand)
- interpreter write APIs in `-c` / heredoc bodies, including Python
  `Path(...).write_text(` / `write_bytes(` and `open(..., 'w'|'a'|'x')` (and
  close mode variants)

Each confident candidate path MUST be resolved as follows: relative candidates
MUST resolve against the recovered command cwd when the command determines one
(`git -C <dir>` / `cd <dir> && ...`); otherwise against a usable event cwd;
otherwise the gate MUST degrade per *Honest degrade when effective cwd is
unrecoverable*. The hook process `$PWD` MUST NOT be used as a blocking
fallback. Classified candidates MUST be routed through the same topology-aware
`gate_mode`, `gate_scope`, `repo_topology`, linked-worktree, canonical-boundary,
and exact protected-branch decision used for structured path events. In effective
`monorepo-submodules`, `auto` enforces both proven owner classes, `superrepo`
enforces only superrepo, and `subrepo` enforces only subrepo. Explicit
`standalone`/`monorepo-apps` MUST not grant a topology-based central bypass.
Central shell writes are allowed only beneath proven canonical
`<superrepo>/openspec/changes/**`; protected candidates inside the selected
enforcement scope MUST block with exit `2`.

The gate MUST fail open (exit `0`) when confidence is insufficient, including:
missing/empty command field, unparseable or ambiguous command text that yields
no confident write target, candidate path outside the repository, candidate path
inside a linked worktree, non-write shell commands, and read-only interpreter
bodies with no write API call. Path-mode behavior for structured
`file_path`/`notebook_path` events MUST remain unchanged.

#### Scenario: Redirection write blocks on protected main worktree

- GIVEN the main worktree is checked out on a protected branch (e.g. `main`)
- AND stdin is a shell event whose command redirects with `>` (or `>>`) to a
  path inside that worktree
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`

#### Scenario: tee write blocks on protected main worktree

- GIVEN the main worktree is checked out on a protected branch
- AND stdin is a shell event whose command pipes to `tee` (or `tee -a`) writing
  a path inside that worktree
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`

#### Scenario: sed -i write blocks on protected main worktree

- GIVEN the main worktree is checked out on a protected branch
- AND stdin is a shell event whose command runs `sed -i` (or equivalent in-place
  form) against a path inside that worktree
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`

#### Scenario: Python heredoc write_text blocks on protected main worktree

- GIVEN the main worktree is checked out on a protected branch
- AND stdin is a shell event whose command is a `python3` (or `python`) heredoc
  or `-c` body that calls `Path(...).write_text(...)` (or `open(..., 'w')`) on a
  path inside that worktree
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`

#### Scenario: Ambiguous or unparseable command fails open

- GIVEN a shell event whose command is unbalanced, obfuscated, or otherwise
  yields no confident write-target candidate
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Missing command field fails open

- GIVEN a shell-shaped event with no extractable command string (no
  `tool_input.command` / aliases and no top-level `command`/`script`) and no
  structured file path
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Write target outside the repository fails open

- GIVEN the main worktree is on a protected branch
- AND the shell command writes only to a path outside the repository (e.g.
  `/tmp/out.txt`)
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Write target inside a linked worktree is allowed

- GIVEN a linked worktree exists under the configured worktrees directory
- AND the shell command writes to a path inside that linked worktree
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Non-write shell command is allowed

- GIVEN the main worktree is on a protected branch
- AND the shell command is a non-write invocation (e.g. `git status`, `ls`,
  `cat path`)
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Read-only heredoc without write call is allowed

- GIVEN the main worktree is on a protected branch
- AND the shell command is a Python (or similar) heredoc/`-c` body that reads
  or prints only and does not call a write API (`write_text`, write-mode
  `open`, etc.)
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

### Requirement: Internal URI allowlist and event-cwd precedence

The worktree gate MUST allow only the project's known non-filesystem internal
protocol URIs before Git path classification. Unknown URI schemes MUST remain
subject to normal gating and MUST NOT receive a general URI bypass.

For filesystem candidates, absolute paths MUST remain unchanged. Relative
candidates MUST resolve against the effective cwd recovered from the command
itself (`git -C <dir>`, `cd <dir> && ...`) when the command determines one;
otherwise they MUST resolve against the tool event's `cwd` when it is present
and usable. The hook process `$PWD` MUST NOT be used as the base for a
blocking decision on a relative candidate when no command cwd and no usable
event cwd is recoverable; in that case the gate MUST degrade per the
"Honest degrade when effective cwd is unrecoverable" requirement instead of
block-on-guess. Path parsing and classification failures MUST remain
fail-open. The event cwd contract applies to the command invocation; the gate
does not implement a shell interpreter for arbitrary dynamic `cd` control
flow. The URI allowlist MUST apply in PATH mode only: a URI-looking token in a
SHELL command is a literal write target and MUST NOT bypass classification. A
known scheme that masks a filesystem path MUST be classified normally —
candidates carrying `../` traversal or an absolute path after the scheme never
receive the internal-URI bypass, even in PATH mode.
(Previously: relative candidates resolved against the event cwd, with the
process `$PWD` as fallback; the command-determined cwd did not exist as a
concept and process-cwd fallback could produce a block-on-guess.)

#### Scenario: Shell-mode URI-looking literal is not allowlisted

- GIVEN the main worktree is on a protected branch
- AND a shell command writes to a bare URI-looking token such as `xd://out.txt`
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2` because the candidate is a literal write target

#### Scenario: Known scheme masking a filesystem path stays gated

- GIVEN the main worktree is on a protected branch
- AND a path-mode candidate is `xd://<abs-repo-path>` or carries `../` traversal
  into the repository
- WHEN `worktree-gate.sh` runs
- THEN it MUST be classified like the filesystem path it masks and exit `2`

#### Scenario: Known internal URI is allowed on protected branch

- GIVEN the main worktree is on a protected branch
- AND a path-mode event targets a known internal URI such as `xd://resolve`,
  `artifact://id`, `local://name.md`, or `vault://path`
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`
- AND it MUST NOT invoke Git filesystem classification for that candidate

#### Scenario: Unknown URI is not allowlisted

- GIVEN the main worktree is on a protected branch
- AND a candidate uses `https://`, `file://`, or `custom://`
- WHEN the candidate resolves inside the protected repository
- THEN it MUST remain subject to the ordinary protected-path decision

#### Scenario: Event cwd takes precedence over process cwd

- GIVEN the hook process cwd is inside a protected repository
- AND the event cwd is an external directory
- AND a relative shell write targets a file under that external directory
- AND the command does not determine its own cwd
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0` because the resolved destination is outside the repository

#### Scenario: Relative event-cwd path inside protected repository remains blocked

- GIVEN the hook process cwd is unrelated to the repository
- AND the event cwd is the protected repository primary checkout
- AND a relative shell or path candidate resolves under that checkout
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`

#### Scenario: Missing command cwd and event cwd degrades instead of process-cwd block

- GIVEN no usable event cwd is supplied and the command determines no cwd
- AND the process cwd is the protected repository primary checkout
- AND a relative candidate targets a repository file
- WHEN `worktree-gate.sh` runs
- THEN the gate MUST NOT exit `2` from a block-on-guess against the process cwd
- AND it MUST degrade per the "Honest degrade when effective cwd is
  unrecoverable" requirement

### Requirement: Ask-mode and message parity for shell blocks

When the shell-write path blocks (exit `2`), stderr MUST name the actual
command worktree cwd recovered for the blocked candidate (the `git -C` / `cd`
destination or the resolved primary checkout), so the agent can see where the
write was headed. The generic "create a dedicated worktree (e.g.
`/worktree-new`)" guidance applies only when the blocked command cwd is the
protected primary checkout; when the blocked candidate's command cwd is a
linked worktree or another existing worktree, the message MUST NOT instruct
creating another worktree and MUST name that cwd instead. The stderr MUST
still name the bash-bypass risk. `gate_mode=ask` MUST NOT introduce a fourth
bypass surface beyond the three destinations already described by the
existing ask-mode message; for the shell block path, the ask-mode stderr MUST
convey the same three-destination guidance and MUST NOT advertise the
`WORKTREE_GATE_MODE=off` one-shot disable (that hatch is not surfaced by the
Go `AskMessage` today and a frozen test forbids advertising it).
`gate_mode=off` MUST disable shell gating the same
way it disables path gating. No additional bypass surface beyond existing
`gate_mode` / `WORKTREE_GATE_MODE` controls is permitted.
(Previously: the block message always said "Create a dedicated worktree first
(e.g. /worktree-new)", which is a wrong exit when the command already executes
inside a worktree.)

#### Scenario: Block message names the real command cwd

- GIVEN a shell write is blocked with exit `2` and the gate recovered a
  command cwd of `<worktree>` (via `git -C` or `cd`)
- WHEN the block message is emitted
- THEN stderr MUST name `<worktree>` as the command cwd for the blocked write
- AND stderr MUST NOT instruct the agent to create a dedicated worktree for
  that blocked candidate

#### Scenario: Block from protected primary keeps worktree-creation guidance

- GIVEN a shell write is blocked with exit `2` and the recovered command cwd
  is the protected primary checkout
- WHEN the block message is emitted
- THEN stderr MUST guide the agent to create a dedicated worktree
  (e.g. `/worktree-new`)
- AND stderr MUST name the bash/shell bypass risk

#### Scenario: Ask-mode shell block uses the same three-destination guidance

- GIVEN `gate_mode=ask` (stamped or via env) and the main worktree is on a
  protected branch
- AND a high-confidence shell write would otherwise block
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`
- AND stderr MUST present the same three-destination guidance used by the
  existing ask-mode path block
- AND stderr MUST NOT advertise the `WORKTREE_GATE_MODE=off` one-shot disable
