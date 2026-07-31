# worktree-flow

> Spec for the worktree-flow recipe and its cleanup/detection heuristics.

## Purpose

Governs how worktree-related operations (creation, detection, cleanup) behave, with a focus on accurate merge detection and conservative safety checks.

## Requirements

### Requirement: Positive Base Candidate Resolution for Merge Detection
The system MUST treat a worktree branch as merged when any local candidate ref proves ancestry or patch-id equivalence for the branch tip. Candidates are evaluated in order: the exact `--base` ref, the base branch's configured upstream ref, and the remote-tracking ref for the base branch's configured remote (`branch.<base>.remote`, or `origin` when no remote is configured). The remote-tracking ref `origin/<base>` is a conditional last-resort candidate: it is consulted ONLY when the configured remote-tracking ref above did not resolve. If a different, valid configured remote resolves, `origin/<base>` MUST NOT be consulted, even if it exists and points elsewhere (dual-remote safety). If no candidate proves merge, the system MUST fall back to existing `git cherry` patch-id equivalence.

#### Scenario: Regular merge on origin/base with stale local base
- GIVEN a temp repo with clean worktree `feat-regular`
- AND `origin/main` contains a merge commit that includes `feat-regular`
- AND local `main` still points before that merge
- WHEN `worktree-cleanup.sh --base main --dry-run` runs
- THEN it MUST report `would remove feat-regular`

#### Scenario: Configured remote resolving blocks the origin fallback
- GIVEN a temp repo where `branch.main.remote` is set to `upstream`
- AND `refs/remotes/upstream/main` exists locally but does NOT contain `feat-dual-remote`
- AND `refs/remotes/origin/main` exists and DOES contain `feat-dual-remote` (e.g. a personal fork)
- WHEN `worktree-cleanup.sh --base main --dry-run` runs
- THEN it MUST report `skipped feat-dual-remote (unmerged)`
- AND it MUST NOT consult `refs/remotes/origin/main` as proof of merge

#### Scenario: Squash merge still resolves by patch-id
- GIVEN a temp repo where `feat-squash` was squash-merged into `main`
- AND local `main` does not contain the branch tip by ancestry
- WHEN cleanup runs
- THEN it MUST report `would remove feat-squash`

#### Scenario: Rebase merge still resolves by patch-id
- GIVEN a temp repo where `feat-rebase` was rebased onto `main`
- AND the branch commits are already present by patch-id
- WHEN cleanup runs
- THEN it MUST report `would remove feat-rebase`

#### Scenario: Fast-forward merge remains merged
- GIVEN a temp repo where local `main` already contains the tip of `feat-ff`
- WHEN cleanup runs
- THEN it MUST report `would remove feat-ff`

#### Scenario: Local-only branch with no match stays unmerged
- GIVEN a temp repo where `feat-local` has no remote ref and no upstream ref
- AND its changes are not patch-equivalent to `main`
- WHEN cleanup runs
- THEN it MUST report `skipped feat-local (unmerged)`

#### Scenario: Branch ahead of base stays unmerged
- GIVEN a temp repo where `feat-ahead` has commits not present in `main`
- WHEN cleanup runs
- THEN it MUST report `skipped feat-ahead (unmerged)`

#### Scenario: Remote-deleted branch still merges from local base
- GIVEN a temp repo where `feat-gone` was deleted on the remote
- AND local `main` already contains the branch tip
- WHEN cleanup runs
- THEN it MUST report `would remove feat-gone`

### Requirement: Conservative Skip for Dirty Worktrees
The system MUST still skip worktrees with uncommitted changes, blocking untracked files, or active in-progress merges before any merge detection.

#### Scenario: Dirty worktree overrides merged verdict
- GIVEN a temp repo where `feat-dirty` is otherwise merged into `main`
- AND the worktree has uncommitted changes
- WHEN cleanup runs
- THEN it MUST report `skipped feat-dirty (dirty)`
- AND it MUST not remove the worktree even if merge evidence exists

### Requirement: Bounded Candidate Resolution
Candidate-base resolution MUST use only refs already present in the local repository. It MUST NOT trigger `git fetch` or any network operation.

#### Scenario: Missing remote does not fetch
- GIVEN `origin/main` is absent locally
- AND local `main` plus its upstream ref are already present
- WHEN cleanup resolves base candidates
- THEN it MUST complete without fetch or network access
- AND it MUST decide using only local refs

### Requirement: Pre-delegation worktree/branch check in the always-on brief

The `worktree-flow` recipe SHALL publish an always-on `workflow_rules` brief
fragment requiring the orchestrator to verify the current worktree and branch
before dispatching a write-capable subagent or task, independent of whether a
runtime `pre-tool-use` hook will fire for delegated tool calls.

#### Scenario: Brief rule present in recipe declaration
- GIVEN the catalog `worktree-flow` recipe
- WHEN its `[provides.brief].workflow_rules` are read
- THEN at least one rule SHALL require verifying worktree/branch before
  dispatching write-capable subagents/tasks
- AND SHALL state that runtime pre-tool-use hooks must not be the sole guard
  for delegated work on harnesses where subprocess calls may bypass the hook

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

Each confident candidate path MUST be resolved against the event `cwd` (when
relative) and subjected to the **same** main-worktree + protected-branch check
used for structured path events. When a candidate resolves inside the protected
main worktree on a protected branch, the gate MUST block with exit `2`.

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

### Requirement: Dual Hook Registration for Shell Matchers

The `worktree-flow` recipe MUST declare two distinct `[[provides.hooks]]`
entries that share the same gate script:

1. Existing file-write hook (`id = "worktree-gate"`, matcher
   `Edit|Write|MultiEdit|NotebookEdit`) — unchanged.
2. Sibling shell hook (`id = "worktree-gate-shell"`, matcher
   `Bash|Shell|Execute|Terminal`, case-insensitive at runtime) with
   `event = "pre-tool-use"`, `script = "hooks/worktree-gate.sh"`, and
   `blocking = true`.

After `ai-specs sync` / hooks render, harnesses that match tools by name
(claude, omp, pi, opencode) MUST receive both matchers (two managed entries or
two generated extension/plugin shims). Cursor MUST receive a **genuinely
separate** shell-only registration mapped to `beforeShellExecution` for the
shell hook, and MUST NOT reuse or merge the file-write matcher into that
registration (a combined matcher containing any file-write token would still be
skipped entirely by the Cursor renderer).

#### Scenario: omp/pi extensions carry both file-write and shell matchers

- GIVEN the catalog `worktree-flow` recipe with both hook entries
- WHEN hooks are rendered for omp or pi
- THEN the generated extension set MUST include a file-write shim whose matcher
  is `Edit|Write|MultiEdit|NotebookEdit`
- AND a distinct shell shim whose matcher is `Bash|Shell|Execute|Terminal`
- AND both shims MUST invoke the same materialized `worktree-gate.sh` script

#### Scenario: Cursor registers a separate shell-only beforeShellExecution hook

- GIVEN the catalog `worktree-flow` recipe with both hook entries
- WHEN hooks are rendered for cursor
- THEN the file-write hook MUST remain skipped (no pre-file-write API)
- AND a shell-only wrapper (e.g. `worktree-flow-worktree-gate-shell.sh`) MUST be
  emitted
- AND `.cursor/hooks.json` MUST register that wrapper under
  `beforeShellExecution` as a managed entry distinct from any file-write hook
- AND the shell hook matcher MUST NOT include `Edit`, `Write`, `MultiEdit`, or
  `NotebookEdit`

### Requirement: Ask-mode and message parity for shell blocks

When the shell-write path blocks (exit `2`), stderr MUST name the bash-bypass
risk and point the agent to create a dedicated worktree (e.g. `/worktree-new`).
Under `gate_mode=ask`, the shell block path MUST emit the same
`WORKTREE_GATE_MODE=off` one-shot bypass hint used by the existing path-write
block. `gate_mode=off` MUST disable shell gating the same way it disables path
gating. No additional bypass surface beyond existing `gate_mode` /
`WORKTREE_GATE_MODE` controls is permitted.

#### Scenario: Shell block message names bash-bypass and worktree creation

- GIVEN the main worktree is on a protected branch and `gate_mode` is `always`
  or `ask`
- AND a high-confidence shell write targets a path inside that worktree
- WHEN `worktree-gate.sh` blocks the command
- THEN it MUST exit `2`
- AND stderr MUST mention bash/shell bypass risk
- AND stderr MUST guide the agent to create a worktree (e.g. `/worktree-new`)

#### Scenario: Ask-mode shell block includes the same bypass hint as path blocks

- GIVEN `gate_mode=ask` (stamped or via env) and the main worktree is on a
  protected branch
- AND a high-confidence shell write would otherwise block
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`
- AND stderr MUST include the `WORKTREE_GATE_MODE=off` bypass hint identical in
  meaning and form to the path-write ask-mode hint

### Requirement: Anti-Fallback Skill and Brief Guidance

The `worktree-flow` skill (`SKILL.md`) and the recipe's always-on
`[provides.brief].workflow_rules` MUST state explicitly that a blocked **or**
errored structured Edit/Write/MultiEdit/NotebookEdit on a protected branch in
the main worktree is never grounds to retry the write via bash/shell/python/
node/ruby, heredocs, or redirections. The correct response MUST be to create a
dedicated worktree first (e.g. `/worktree-new`) and continue there. This rule
complements the existing pre-delegation brief rule and applies on every harness
regardless of hook fidelity.

#### Scenario: SKILL.md and brief contain the anti-fallback rule

- GIVEN the catalog `worktree-flow` recipe after this change
- WHEN `skills/worktree-flow/SKILL.md` and `[provides.brief].workflow_rules` are
  read
- THEN both MUST contain an explicit anti-fallback rule forbidding bash/shell
  retry after a blocked or errored structured write on a protected main
  worktree
- AND both MUST direct the agent to create a dedicated worktree instead

### Requirement: Honest per-harness shell-gate coverage documentation

Product-facing documentation for `worktree-flow` and runtime hooks MUST describe
shell-write gating as **best-effort and uneven by harness**, not as a uniform
sandbox. Docs MUST state residual gaps: incomplete heuristics (obfuscated /
multi-stage writers fail open), OpenCode subagent/MCP pre-hook non-firing,
pi/omp child-process boundary, and Cursor's continued lack of a pre-file-write
hook. Docs MUST NOT claim absolute prevention of all shell writes on protected
branches.

#### Scenario: Docs state residual gaps without overclaiming

- GIVEN `docs/runtime-hooks.md` and the `worktree-flow` recipe README (and
  catalog blurb if it describes gate scope)
- WHEN a reader consults shell / worktree-gate coverage
- THEN the docs MUST distinguish structured file-write coverage from shell
  pre-exec coverage per harness
- AND MUST list residual heuristic and process-boundary gaps
- AND MUST NOT claim that bash writes are fully or uniformly gated on every
  harness
