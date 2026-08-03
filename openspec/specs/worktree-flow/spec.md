# worktree-flow

> Spec for the worktree-flow recipe and its cleanup/detection heuristics.

## Purpose

Governs how worktree-related operations (creation, detection, cleanup) behave across
repo topologies (`standalone`, `monorepo-apps`, `monorepo-submodules`), with a focus on
accurate merge detection, conservative safety checks, and topology-aware create/clean paths.

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
runtime `pre-tool-use` hook will fire for delegated tool calls. Under a
resolved `monorepo-submodules` topology, that rule SHALL also require verifying
*which git repository* is active (superproject vs submodule path) via
`git rev-parse --show-toplevel` before dispatching write-capable subagents or
tasks — not only branch and worktree-list checks.

#### Scenario: Brief rule present in recipe declaration
- GIVEN the catalog `worktree-flow` recipe
- WHEN its `[provides.brief].workflow_rules` are read
- THEN at least one rule SHALL require verifying worktree/branch before
  dispatching write-capable subagents/tasks
- AND SHALL state that runtime pre-tool-use hooks must not be the sole guard
  for delegated work on harnesses where subprocess calls may bypass the hook

#### Scenario: Brief requires which-repo check under monorepo-submodules
- GIVEN the catalog `worktree-flow` recipe with resolved topology `monorepo-submodules`
- WHEN its `[provides.brief].workflow_rules` are read
- THEN at least one rule SHALL require verifying which git repository is active
  (superproject vs submodule path) via `rev-parse --show-toplevel` before
  dispatching write-capable subagents/tasks
- AND SHALL NOT treat branch or worktree-list checks alone as sufficient under
  that topology

### Requirement: worktree-cleanup.sh submodule enumeration

Under a resolved `monorepo-submodules` topology, `worktree-cleanup.sh` MUST iterate every initialized submodule (via `git submodule foreach` and/or per-module `git -C`) and MUST NOT trust the superproject `git worktree list` alone as the candidate source. Under `standalone` and `monorepo-apps`, cleanup MUST keep byte-identical single-pass behavior over the repository root so the existing Positive Base Candidate Resolution merge-detection scenarios remain unchanged. An optional `--submodule` / `--subrepo` scope flag MAY limit the scan; when omitted, the default MUST be all initialized submodules. Uninitialized (`-` prefix) submodules MUST be skipped, not scanned.

#### Scenario: Standalone repo cleanup unchanged
- GIVEN a standalone repo with no `.gitmodules`
- AND a clean merged feature worktree under the shared worktrees dir
- WHEN `worktree-cleanup.sh --base main --dry-run` runs
- THEN behavior MUST match today's single-pass cleanup
- AND it MUST report `would remove` for the merged worktree using the same output lines as before

#### Scenario: Merged feature worktree under one submodule is cleaned
- GIVEN a superproject with initialized submodule `apps/api`
- AND a clean feature worktree at `<super>/<worktrees_dir>/apps/api-feat-done` owned by that submodule
- AND the feature branch is merged into the submodule base
- WHEN `worktree-cleanup.sh --base main --dry-run` runs from the superproject
- THEN it MUST scan via the submodule's worktree list (not the superproject list alone)
- AND it MUST report `would remove` for `apps/api-feat-done`

#### Scenario: Worktrees under multiple submodules are all scanned
- GIVEN initialized submodules `apps/api` and `apps/web`
- AND each owns a linked feature worktree under the shared `<worktrees_dir>`
- WHEN `worktree-cleanup.sh` runs with no submodule scope
- THEN it MUST scan both submodule repositories
- AND candidates from both modules MUST be considered for merge detection and cleanup

#### Scenario: Scoped --submodule flag limits the scan
- GIVEN initialized submodules `apps/api` and `apps/web`
- AND each owns a merged feature worktree under the shared worktrees dir
- WHEN `worktree-cleanup.sh --submodule apps/api --dry-run` runs
- THEN it MUST scan only `apps/api`
- AND it MUST NOT remove or report cleanup for the `apps/web` worktree in that run

#### Scenario: Uninitialized submodules are skipped
- GIVEN `.gitmodules` lists `apps/api` (initialized) and `apps/legacy` (status prefix `-`)
- WHEN `worktree-cleanup.sh` runs with default scope
- THEN it MUST scan `apps/api`
- AND it MUST NOT attempt to `git -C` / enumerate worktrees for `apps/legacy`

### Requirement: Repo Topology Configuration

`recipes.worktree-flow.config.repo_topology` MUST be one of `auto`, `standalone`, `monorepo-apps`, `monorepo-submodules`; default `auto` when absent or empty. `ai-specs sync` MUST reject invalid values with non-zero exit and a diagnostic naming the value and the allowed enum, matching `gate_mode` validation. An explicit non-`auto` value SHALL bypass auto-detection and resolve to that topology.

#### Scenario: Default when unset is auto
- GIVEN a manifest with no `repo_topology` under `recipes.worktree-flow.config`
- WHEN `ai-specs sync` resolves the recipe config
- THEN the configured value is `auto`
- AND no error is raised for the missing key

#### Scenario: Invalid enum rejected at sync
- GIVEN `repo_topology = "nested"`
- WHEN `ai-specs sync` runs
- THEN it exits non-zero
- AND stderr names `nested` as invalid
- AND stderr lists `auto | standalone | monorepo-apps | monorepo-submodules`

#### Scenario: Explicit standalone bypasses detection
- GIVEN `repo_topology = "standalone"`
- AND the project root has initialized `.gitmodules` entries
- WHEN topology is resolved
- THEN the resolved topology is `standalone`
- AND submodule auto-detection MUST NOT override the explicit value

#### Scenario: Explicit monorepo-apps bypasses detection
- GIVEN `repo_topology = "monorepo-apps"`
- AND the project root has initialized `.gitmodules` entries
- WHEN topology is resolved
- THEN the resolved topology is `monorepo-apps`
- AND submodule auto-detection MUST NOT override the explicit value

#### Scenario: Explicit monorepo-submodules bypasses detection
- GIVEN `repo_topology = "monorepo-submodules"`
- AND the project root has no `.gitmodules`
- WHEN topology is resolved
- THEN the resolved topology is `monorepo-submodules`
- AND absence of `.gitmodules` MUST NOT force `standalone`

### Requirement: Auto Topology Detection

When `repo_topology` is `auto` (including the default), the system MUST resolve to `monorepo-submodules` only when `.gitmodules` exists and at least one entry has a non-`-` `git submodule status` prefix (initialized: space, `+`, or `U`). Otherwise it MUST resolve to `standalone`. Auto-detection MUST NEVER resolve to `monorepo-apps`; that topology is explicit-only.

#### Scenario: Initialized submodules resolve to monorepo-submodules
- GIVEN `repo_topology = "auto"`
- AND `.gitmodules` lists submodule path `apps/api`
- AND `git submodule status` shows an initialized (non-`-`) entry for `apps/api`
- WHEN topology is resolved
- THEN the resolved topology is `monorepo-submodules`
- AND `via` is `auto`

#### Scenario: Only uninitialized submodules resolve to standalone
- GIVEN `repo_topology = "auto"`
- AND `.gitmodules` lists submodule path `vendor/lib`
- AND `git submodule status` shows only a `-` prefix entry for `vendor/lib`
- WHEN topology is resolved
- THEN the resolved topology is `standalone`
- AND it MUST NOT resolve to `monorepo-submodules`

#### Scenario: No gitmodules resolves to standalone
- GIVEN `repo_topology = "auto"`
- AND the project root has no `.gitmodules` file
- WHEN topology is resolved
- THEN the resolved topology is `standalone`

#### Scenario: monorepo-apps is never auto-selected
- GIVEN `repo_topology = "auto"`
- AND any combination of missing, uninitialized, or initialized `.gitmodules` entries
- WHEN topology is resolved
- THEN the resolved topology MUST be either `standalone` or `monorepo-submodules`
- AND it MUST NEVER be `monorepo-apps`

### Requirement: Submodule Worktree Creation Contract

Under a resolved `monorepo-submodules` topology, `/worktree-new` MUST require or infer a `<subrepo>`, validate it against `.gitmodules` (path first, then unique name), and reject uninitialized, unknown, or ambiguous names before any create. Creation MUST use `git -C <subrepo_path> worktree add <absolute-destination> -b <branch> <integration_branch>` with a mandatory absolute destination under the shared superproject `<worktrees_dir>/<subrepo>-<slug>`. Cwd inference MUST use `git rev-parse --show-toplevel`: a toplevel that is an initialized submodule path yields that path; a linked feature worktree under `worktrees_dir` yields the longest initialized-path prefix match on the basename (`<path>-<slug>`). Explicit and inferred values that disagree MUST hard-error.

#### Scenario: Cwd inference from submodule primary checkout
- GIVEN resolved topology `monorepo-submodules` with initialized submodule path `apps/api`
- AND the current working directory is inside the primary checkout at `<super>/apps/api`
- AND no explicit `<subrepo>` argument is passed
- WHEN `/worktree-new` runs
- THEN it MUST infer `apps/api`
- AND create via `git -C <super>/apps/api worktree add <super>/<worktrees_dir>/apps/api-<slug> -b <branch> <integration_branch>`

#### Scenario: Cwd inference from linked feature worktree uses longest-path-prefix
- GIVEN initialized submodule paths `alquimia-front` and `alquimia-front-web`
- AND cwd is inside `<super>/<worktrees_dir>/alquimia-front-web-feat-x`
- AND no explicit `<subrepo>` is passed
- WHEN `/worktree-new` runs
- THEN it MUST infer `alquimia-front-web` (longest prefix match)
- AND MUST NOT infer `alquimia-front`

#### Scenario: Explicit arg validated against gitmodules path
- GIVEN resolved topology `monorepo-submodules`
- AND `.gitmodules` registers path `apps/api`
- AND the caller passes explicit `<subrepo>` `apps/api`
- WHEN `/worktree-new` runs
- THEN it MUST accept `apps/api` as the resolved submodule path
- AND create the worktree with an absolute destination under `<worktrees_dir>/apps/api-<slug>`

#### Scenario: Explicit arg validated against unique gitmodules name
- GIVEN `.gitmodules` maps name `api` to path `apps/api` uniquely
- AND the caller passes explicit `<subrepo>` `api`
- WHEN `/worktree-new` runs
- THEN it MUST resolve `api` to path `apps/api`
- AND proceed with create for that path

#### Scenario: Explicit and inferred mismatch errors
- GIVEN cwd toplevel resolves to initialized submodule `apps/api`
- AND the caller passes explicit `<subrepo>` `apps/web`
- WHEN `/worktree-new` runs
- THEN it MUST hard-error before any `git worktree add`
- AND the diagnostic MUST name both the inferred and explicit values

#### Scenario: Uninitialized submodule rejected
- GIVEN `.gitmodules` lists path `apps/api`
- AND `git submodule status` shows a `-` prefix for `apps/api`
- AND the caller passes `<subrepo>` `apps/api`
- WHEN `/worktree-new` runs
- THEN it MUST hard-error
- AND the diagnostic MUST instruct running `git submodule update --init` for that path
- AND it MUST NOT run `git worktree add`

#### Scenario: Unknown submodule rejected
- GIVEN resolved topology `monorepo-submodules`
- AND the caller passes `<subrepo>` `does-not-exist`
- WHEN `/worktree-new` runs
- THEN it MUST hard-error naming the unknown submodule
- AND it MUST NOT run `git worktree add`

#### Scenario: Ambiguous name requires path
- GIVEN `.gitmodules` has two entries that share the same name key resolution to different paths when the caller passes that ambiguous name
- WHEN `/worktree-new` is invoked with that ambiguous name
- THEN it MUST hard-error asking the caller to pass the submodule path instead
- AND it MUST NOT run `git worktree add`

### Requirement: Stale Cleanup Override Detection

When a `[[provides.templates]]` entry with `condition = "not_exists"` already has a materialized target, sync (and optionally doctor) MUST compare catalog source bytes to the materialized file. If they differ, the system MUST emit a non-blocking WARN with refresh instructions and MUST NOT overwrite the override. An unmodified override MUST produce no stale warning. A missing target remains the normal fresh-copy path under `not_exists` and is not a warning case.

#### Scenario: Unmodified override produces no warning
- GIVEN a materialized `worktree-cleanup.sh` override whose bytes match the current catalog template
- WHEN `ai-specs sync` runs
- THEN it MUST NOT emit a stale-override WARN for that file
- AND it MUST leave the override untouched

#### Scenario: Diverged override warns and sync succeeds
- GIVEN a materialized `worktree-cleanup.sh` override whose content differs from the current catalog template
- WHEN `ai-specs sync` runs
- THEN it MUST emit a non-blocking WARN naming the override path
- AND the WARN MUST include refresh instructions (`rm <target> && ai-specs sync` or equivalent)
- AND sync MUST exit successfully without overwriting the override

#### Scenario: Missing override gets a fresh copy
- GIVEN no materialized cleanup override exists at the `not_exists` target
- WHEN `ai-specs sync` runs
- THEN it MUST copy the catalog template to the target as the normal `not_exists` path
- AND it MUST NOT emit a stale-override WARN for that missing file

### Requirement: Topology Surfacing

The resolved topology (not merely the configured `repo_topology` value) MUST be shown in the init wizard as a confirmable default next to project identity, in the interactive hub status panel, in noninteractive status output, and in the agent-facing project brief Project section.

#### Scenario: Wizard proposes auto-detected default and accepts override
- GIVEN an init wizard run on a repo whose auto-detect resolves to `monorepo-submodules`
- WHEN the topology prompt is shown after the project name
- AND the user selects an explicit override such as `standalone`
- THEN the default MUST have presented `auto` resolving to the detected topology
- AND the staged manifest MUST write `recipes.worktree-flow.config.repo_topology = "standalone"`

#### Scenario: Hub panel shows resolved topology
- GIVEN a project with `repo_topology = "auto"` that resolves to `monorepo-submodules`
- WHEN the interactive hub status panel renders
- THEN it MUST display the resolved topology `monorepo-submodules`
- AND it MUST indicate the resolution was via `auto`

#### Scenario: Noninteractive status shows resolved topology
- GIVEN a project with an explicit `repo_topology = "standalone"`
- WHEN noninteractive status output runs
- THEN it MUST print a topology line showing resolved `standalone`
- AND it MUST indicate resolution via config

#### Scenario: Brief Project section includes resolved topology
- GIVEN worktree-flow is enabled and topology resolves to `monorepo-submodules` via `auto`
- WHEN the agent-facing project brief is rendered
- THEN the Project section MUST include a Repo topology line with the resolved value
- AND it MUST indicate via `auto` (or equivalent config-vs-auto provenance)

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