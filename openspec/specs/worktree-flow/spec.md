# worktree-flow

> Spec for the worktree-flow recipe and its cleanup/detection heuristics.

## Purpose

Governs how worktree-related operations (creation, detection, cleanup) behave across
repo topologies (`standalone`, `monorepo-apps`, `monorepo-submodules`), with a focus on
accurate merge detection, conservative safety checks, and topology-aware create/clean paths.

## Requirements

### Requirement: Positive Base Candidate Resolution for Merge Detection

The system MUST treat a worktree branch as removable only when an ordered local
base candidate proves either ancestry or complete patch-id equivalence for the
branch's unique commits. Complete patch-id equivalence MUST account for every
commit on a multi-commit branch. A base that represents only a subset of the
branch changes MUST remain unmerged. A later revert of the branch changes MUST
not be treated as proof that the branch is merged.

The existing candidate order remains authoritative: exact `--base`, configured
upstream, configured remote-tracking ref, and the conditional `origin/<base>`
fallback only when the configured remote-tracking ref does not resolve. The
cleanup MUST use only local refs and MUST NOT fetch. The implementation MUST
preserve the current ancestry-first and `git cherry` patch-id decision points;
this requirement does not authorize a new merge heuristic.

#### Scenario: Multi-commit regular merge is eligible

- GIVEN a clean feature worktree whose branch contains at least two commits
- AND the complete branch tip is integrated into the selected base by a regular
  merge or fast-forward
- WHEN `worktree-cleanup.sh --base <base> --dry-run` runs
- THEN it MUST report `would remove <name>`
- AND the branch and worktree MUST be removable in normal mode

#### Scenario: Multi-commit squash merge is eligible

- GIVEN a clean feature worktree whose branch contains at least two commits
- AND the feature changes are integrated into the selected base as one or more
  new squash commits
- AND the original branch tip is not an ancestor of the base
- WHEN cleanup evaluates the branch
- THEN complete patch-id equivalence MUST prove the branch as merged
- AND dry-run MUST report `would remove <name>`

#### Scenario: Partial squash is preserved

- GIVEN a feature branch contains at least two commits
- AND the selected base represents only a strict subset of those changes
- WHEN cleanup evaluates the branch
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve both the worktree and branch

#### Scenario: Reverted change is preserved

- GIVEN a feature branch's changes were integrated into the base and later
  reverted so the branch's complete patch is no longer present
- WHEN cleanup evaluates the branch
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve both the worktree and branch

### Requirement: Conservative Skip for Dirty Worktrees

The system MUST preserve dirty, main, detached, unmerged, and topology-protected
worktrees before any removal. Dirty status MUST be checked before merge proof.
The main worktree MUST never be removed even when its branch is fully merged.
Detached worktrees under the configured directory MUST be reported as detached
and preserved. Under topology-aware cleanup, uninitialized modules, explicit
out-of-scope modules, and unproven relationships MUST not become removal
candidates.

#### Scenario: Detached worktree is preserved

- GIVEN a detached worktree exists under the configured worktree directory
- WHEN cleanup runs
- THEN it MUST report `skipped <name> (detached)`
- AND it MUST not remove the worktree or any branch

#### Scenario: Main worktree is never removed

- GIVEN the main repository worktree is on a protected or integration branch
- WHEN cleanup runs from the repository root
- THEN it MUST not report the main worktree as removable
- AND it MUST leave the main worktree unchanged

#### Scenario: Topology-protected worktree is preserved

- GIVEN a worktree belongs to an uninitialized, unproven, or explicitly
  out-of-scope submodule topology
- WHEN cleanup runs from the superproject or with a different module scope
- THEN it MUST not scan that worktree as an eligible candidate
- AND it MUST preserve the worktree and branch

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

### Requirement: Topology-aware `gate_scope` worktree protection

`recipes.worktree-flow.config.gate_scope` MUST be `auto`, `superrepo`, or
`subrepo`; absent or empty values resolve to `auto`, and invalid values MUST be
rejected during sync with the exact allowed enum. The setting is independent of
`gate_mode` and `repo_topology`. Sync MUST stamp the validated scope and
`repo_topology` into the materialized hook.
A non-empty valid `WORKTREE_GATE_SCOPE` override wins for one invocation;
invalid overrides warn and fall back to the stamp, while missing or invalid
stamps warn and fall back to `auto`. `gate_mode=off` MUST exit before scope or
topology evaluation.

The hook MUST classify repository ownership only from canonical nearest-existing
paths and proven Git facts when effective `repo_topology` is
`monorepo-submodules`: a real superproject `.git` and `.gitmodules`, a
component-contained registered path, initialized `git submodule status`, and a
matching module common Git directory. Explicit `standalone` or `monorepo-apps`
MUST disable scope classification even if vendored initialized modules exist.
Ambiguous, nested, symlink-escaping, uninitialized, or unresolved relationships
MUST NOT grant a scope exception. Linked worktrees remain allowed before scope
evaluation.

`gate_scope=auto` MUST enforce both proven superrepo and subrepo protected
primaries. `gate_scope=superrepo` MUST enforce only proven superrepo primaries;
proven subrepo writes are outside that selected enforcement scope.
`gate_scope=subrepo` MUST enforce only proven initialized subrepo primaries;
proven superrepo writes are outside that selected scope for the explicit Melón
workflow.

For a proven superproject primary checkout on an exact protected branch, the
canonical `<superrepo>/openspec/changes/**` descendant (including archive and
nonexistent descendants) is the explicit central planning exception for the
enforcing scope. Component-aware containment is mandatory. The worktree gate
MUST remain separate from `plan-build-flow` production authorization.

The hook MUST treat owner root and planning root as distinct request-context
facts: owner primaries are enforced per the selected scope, while only the
canonical superrepo `openspec/changes/**` descendant is excepted as the central
planning boundary. A subrepo request whose planning root is the superrepo keeps
its subrepo production primaries protected under the enforcing scope.

#### Scenario: Missing scope defaults safely
- GIVEN a manifest without `gate_scope`
- WHEN sync resolves worktree-flow
- THEN the effective value and stamped hook value MUST be `auto`

#### Scenario: Proven central planning path is allowed
- GIVEN a proven initialized submodule relationship and protected superrepo branch
- AND a target is a canonical descendant of `<superrepo>/openspec/changes/`
- WHEN the worktree gate evaluates the write under any valid scope
- THEN it MUST allow the write

#### Scenario: Scope selects the enforced owner
- GIVEN a proven superrepo and initialized subrepo primary are both on protected branches
- WHEN a superrepo production write is evaluated under `auto`, `superrepo`, and `subrepo`
- THEN `auto` and `superrepo` MUST block while `subrepo` MUST allow
- AND when a subrepo production write is evaluated under the same values
- THEN `auto` and `subrepo` MUST block while `superrepo` MUST allow

#### Scenario: Ambiguous topology receives no exception
- GIVEN submodule registration or Git directory proof is missing, ambiguous, or symlink-escaping
- WHEN a protected primary write is evaluated
- THEN the central exception MUST NOT apply

#### Scenario: Subrepo owner stays protected under a central planning root
- GIVEN a proven initialized submodule primary on a protected branch
- AND the request planning root is the superrepo
- WHEN a production write to the subrepo primary is evaluated under `gate_scope=auto`
- THEN it MUST block
- AND the central planning exception MUST NOT extend to subrepo production paths

### Requirement: Repo Topology Configuration

`recipes.worktree-flow.config.repo_topology` MUST be one of `auto`, `standalone`, `monorepo-apps`, `monorepo-submodules`; default `auto` when absent or empty. `ai-specs sync` MUST reject invalid values with non-zero exit and a diagnostic naming the value and the allowed enum, matching `gate_mode` validation. An explicit non-`auto` value SHALL bypass auto-detection and resolve to that topology.

An explicit topology MUST remain stable: `monorepo-apps` MUST NOT be silently reclassified to `standalone` or `monorepo-submodules` without an explicit manifest change and evidence.

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

#### Scenario: monorepo-apps is never silently reclassified
- GIVEN `repo_topology = "monorepo-apps"` and initialized `.gitmodules` entries appear later
- WHEN topology is resolved
- THEN the resolved topology remains `monorepo-apps`
- AND no reclassification to `standalone` or `monorepo-submodules` occurs without an explicit manifest change

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

A request whose context is the superrepo MUST NOT infer a subrepo: creation requires an explicit, validated `<subrepo>`; without it, `/worktree-new` MUST hard-error before any `git worktree add`.

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

When a `[[provides.templates]]` entry with `condition = "not_exists"` already
has a materialized target, sync and doctor MUST classify it using
override-ownership rules (lock-backed last-managed hash versus current
would-write catalog bytes), not catalog-only comparison.

- Managed current: no warning and no rewrite.
- Managed stale with effective policy `auto`: overwrite with current catalog
  content and update the managed lock record, without a user-modified warning.
- User-modified or untracked diverged: warn with refresh instructions and never
  overwrite.
- Missing target: normal fresh-copy path and not a warning case.

#### Scenario: Managed stale override refreshes under auto policy
- GIVEN a cleanup override whose bytes still match its managed lock hash
- AND the current catalog would-write bytes differ
- AND effective policy is `auto`
- WHEN `ai-specs sync` runs
- THEN it MUST overwrite the override and update its managed lock hash
- AND sync MUST exit successfully

#### Scenario: User-modified override warns and sync succeeds
- GIVEN a cleanup override whose content differs from its managed lock hash
- WHEN `ai-specs sync` runs
- THEN it MUST warn with the path and indicate user modification
- AND the warning MUST include `rm <target> && ai-specs sync` guidance
- AND sync MUST exit successfully without overwriting the override

#### Scenario: Missing metadata migrates conservatively
- GIVEN an existing cleanup override has no managed lock entry
- WHEN its bytes match the current would-write catalog bytes
- THEN sync MUST seed the managed entry without rewriting or warning
- BUT when bytes differ, sync MUST preserve and warn without seeding ownership

#### Scenario: Missing override gets a fresh copy
- GIVEN no materialized cleanup override exists at the `not_exists` target
- WHEN `ai-specs sync` runs
- THEN it MUST copy/render the catalog template and record a managed lock entry
- AND it MUST NOT emit a stale-override warning

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
relative) and routed through the same topology-aware `gate_mode`, `gate_scope`,
`repo_topology`, linked-worktree, canonical-boundary, and exact protected-branch
decision used for structured path events. In effective
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

### Requirement: Internal URI allowlist and event-cwd precedence

The worktree gate MUST allow only the project's known non-filesystem internal
protocol URIs before Git path classification. Unknown URI schemes MUST remain
subject to normal gating and MUST NOT receive a general URI bypass.

For filesystem candidates, absolute paths MUST remain unchanged. Relative
candidates MUST resolve against the tool event's `cwd` when it is present and
usable; the hook process `$PWD` MAY be used only as fallback when event `cwd` is
absent or unusable. Path parsing and classification failures MUST remain
fail-open. The event cwd contract applies to the command invocation; the gate
does not implement a shell interpreter for arbitrary dynamic `cd` control flow.
The URI allowlist MUST apply in PATH mode only: a URI-looking token in a SHELL
command is a literal write target and MUST NOT bypass classification. A known
scheme that masks a filesystem path MUST be classified normally — candidates
carrying `../` traversal or an absolute path after the scheme never receive the
internal-URI bypass, even in PATH mode.

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
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0` because the resolved destination is outside the repository

#### Scenario: Relative event-cwd path inside protected repository remains blocked

- GIVEN the hook process cwd is unrelated to the repository
- AND the event cwd is the protected repository primary checkout
- AND a relative shell or path candidate resolves under that checkout
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `2`

#### Scenario: Missing event cwd falls back to process cwd

- GIVEN no usable event cwd is supplied
- AND the process cwd is the protected repository primary checkout
- AND a relative candidate targets a repository file
- WHEN `worktree-gate.sh` runs
- THEN the existing protected-branch decision MUST be preserved

### Requirement: Request context owner and planning root separation

The system MUST resolve one explicit ai-specs request context carrying an `owner_root` (the repository that owns code and VCS work for the request) and a `planning_root` (the canonical planning-artifact tree). A subrepo request MUST resolve its owner from `git rev-parse --show-toplevel` plus validated `.gitmodules` (path-first, then unique name, initialized) and MUST use a subrepo-owned worktree at the absolute `<super>/.worktrees/<subrepo>-<slug>`; its planning root MUST be the proven superrepo. A superrepo request MUST own the superrepo, create its worktree at `<super>/.worktrees/<slug>`, and use the superrepo as its planning root. A superrepo-context request MUST NOT infer a subrepo; an explicit subrepo is required. Missing, ambiguous, detached, or uninitialized topology MUST fail safe: no owner inference, no worktree creation, and no planning-root exception.

#### Scenario: Subrepo request owns subrepo worktree with central planning root

- GIVEN a proven initialized submodule and a request from its primary checkout
- WHEN `/worktree-new` runs for that request
- THEN the worktree is created through the subrepo at the absolute `<super>/.worktrees/<subrepo>-<slug>`
- AND the request planning root resolves to the proven superrepo

#### Scenario: Superrepo request owns its own worktree and planning root

- GIVEN a request whose context is the superrepo
- WHEN `/worktree-new` runs
- THEN the worktree is created at `<super>/.worktrees/<slug>` owned by the superrepo
- AND the superrepo is the planning root

#### Scenario: Superrepo context cannot infer a subrepo

- GIVEN resolved topology `monorepo-submodules` and a request from the superrepo primary
- AND no explicit `<subrepo>` argument is passed
- WHEN `/worktree-new` runs
- THEN it MUST hard-error requiring an explicit submodule
- AND it MUST NOT run `git worktree add`

#### Scenario: Ambiguous, detached, or uninitialized topology fails safe

- GIVEN a request context whose owner cannot be proven (detached HEAD, uninitialized submodule, or ambiguous `.gitmodules` match)
- WHEN the request context is resolved
- THEN no owner is inferred and no worktree is created
- AND no planning-root exception is granted

### Requirement: Explicit fan-out target semantics

`project.subrepos` in the root manifest MUST remain the sole authoritative fan-out target set. The system MUST NOT auto-expand fan-out from `.gitmodules` entries or any other discovery, regardless of how many initialized submodules exist. An empty `project.subrepos` list MUST mean no fan-out and MUST NOT be treated as "fan out to every initialized submodule". Fan-out MUST preserve each declared target owner and one shared planning root, MUST NOT duplicate planning artifacts per target, and MUST stop at the first incompatible target.

#### Scenario: Declared targets fan out with one planning root

- GIVEN a root manifest declares explicit `project.subrepos` targets
- WHEN sync fans out derived artifacts
- THEN exactly the declared targets are updated with the owning-target context
- AND all targets share one central planning root with no per-target plan duplication

#### Scenario: Empty subrepos list produces no fan-out

- GIVEN a root manifest with `project.subrepos = []` and many initialized `.gitmodules` entries
- WHEN sync fans out derived artifacts
- THEN no subrepo target is updated
- AND the empty list is honored as an intentional no-fan-out decision

#### Scenario: .gitmodules never expands the target set

- GIVEN initialized `.gitmodules` entries not listed in `project.subrepos`
- WHEN sync fans out derived artifacts
- THEN those entries are NOT added to the fan-out set
- AND the manifest list remains authoritative

#### Scenario: First incompatible target stops fan-out

- GIVEN multiple declared fan-out targets and one target that is incompatible
- WHEN sync processes the targets
- THEN it stops at the first incompatible target
- AND it does not partially continue past it

### Requirement: Forced Latest-Canonical Refresh for Governed Worktree-Flow Assets

The worktree-flow cleanup override, generated Go launcher, and materialized
legacy gate MUST be classified using the existing lock-backed provenance and
current would-write bytes before replacement or execution. A managed-current
asset MAY be used without rewriting after its current bytes remain verified. A
missing asset MAY be materialized and recorded. A managed-stale,
user-modified, or unknown/untracked governed asset MUST be force-replaced by
the latest verified canonical bytes during ordinary sync/materialization.

The operation MUST use an existing immutable cache-only backup and rollback
mechanism where supported, write the replacement atomically, verify the
installed bytes, and update provenance only after replacement succeeds. It MUST
report the exact project-relative target, prior classified state, observed and
desired digests when available, relevant recipe/source, replacement result, and
backup/recovery location when one exists. Unknown or user-modified bytes are
recoverable evidence, not a reason to block or defer the canonical update.

If canonical verification, backup, replacement, rollback, or lock update fails,
the operation MUST fail closed, leave the target and lock consistent, and MUST
NOT accept or execute an unverified asset. `ai-specs sync --refresh-gates` MUST
use the same forced replacement transaction as ordinary sync; it is an
explicit retry/diagnostic path, not the only replacement path. Doctor MUST use
the same read-only classification and verification evidence without mutating
the project.

This requirement applies only to worktree-flow assets. It MUST NOT change
generic template ownership policies for unrelated recipes.

#### Scenario: Stale cleanup override forces verified replacement

- GIVEN the materialized cleanup override matches its recorded managed digest
- AND the catalog would-write bytes have changed
- WHEN ordinary sync or materialization runs
- THEN the materializer MUST back up the prior bytes where the existing cache
  mechanism supports it
- AND it MUST atomically replace the override with the verified catalog bytes
- AND it MUST update the managed lock entry only after the replacement verifies
- AND the operation MUST report the prior state/digest, desired digest, and
  replacement/backup result

#### Scenario: Unknown cleanup override forces canonical ownership replacement

- GIVEN a cleanup override exists with no managed lock entry
- AND its bytes diverge from the current catalog would-write bytes
- WHEN ordinary sync runs
- THEN ordinary sync MUST replace it with the verified catalog bytes
- AND it MUST seed the managed entry from the installed canonical bytes only
  after successful replacement
- AND the result MUST identify unknown provenance, the observed digest, and the
  replacement/backup result
- AND doctor MUST remain read-only and report that ordinary sync will perform
  the forced replacement

#### Scenario: Customized gate is force-replaced by ordinary sync

- GIVEN a materialized `worktree-gate.sh` or legacy gate differs from its
  recorded baseline or has no baseline
- WHEN ordinary sync or `ai-specs sync --refresh-gates` runs
- THEN the pre-refresh bytes MUST be saved through the existing cache-only
  immutable backup mechanism where that mechanism applies
- AND the gate or legacy fallback MUST be atomically replaced with verified
  canonical bytes
- AND its baseline/lock evidence MUST be updated only after replacement succeeds
- AND the operation MUST report the replacement rather than block on the local
  customization

#### Scenario: Current worktree-flow assets remain idempotent

- GIVEN the cleanup override and gate assets match their current recorded
  provenance and expected bytes
- WHEN ordinary sync or doctor runs
- THEN no freshness warning or hard failure MUST be emitted
- AND no asset MUST be rewritten

#### Scenario: Failed canonical verification fails closed

- GIVEN a worktree-flow asset or version-keyed Go cache candidate is stale,
  mismatched, or unknown
- AND the latest canonical bytes fail digest, version, or self-test verification
- WHEN ordinary sync, materialization, or acquisition evaluates it
- THEN no unverified bytes MUST be accepted or executed
- AND the operation MUST report the target/cache path and expected/observed
  verification evidence
- AND any prior bytes MUST remain recoverable or quarantined without being
  selected as the current verified asset

#### Scenario: Failed replacement rolls back governed state

- GIVEN a stale, user-modified, or unknown governed asset is selected for forced
  canonical replacement
- AND its backup, atomic write, verification, or lock update fails
- WHEN the replacement transaction runs
- THEN the operation MUST fail closed
- AND the prior target bytes and lock state MUST be restored or remain
  internally consistent
- AND no partial temporary file or unverified asset MUST become executable

#### Scenario: Canonical preflight precedes project writes

- GIVEN worktree-flow is enabled in a project manifest
- AND the catalog cleanup template, launcher, legacy gate, and supported gate
  trust-root inputs are available
- WHEN ordinary `ai-specs sync` starts
- THEN a read-only worktree-flow freshness preflight MUST verify those canonical
  inputs before the first consumer-project write
- AND materialization MUST repeat classification and verification immediately
  before each governed replacement
- AND the preflight MUST NOT create or rewrite the project's materialized assets
  or lock

### Requirement: Current Gate Asset and Release Freshness

The version-keyed Go gate cache MUST not treat an executable file as current
solely because it exists. For the current platform and CLI version, acceptance
MUST be based on the existing committed `SHA256SUMS` trust root plus the
current binary version and self-test checks. A missing, stale, mismatched, or
unknown cached asset MUST trigger forced re-acquisition during ordinary
acquisition/materialization or the explicit gate-refresh path. It MUST not be
executed as a verified current gate before those checks pass. If verification
or replacement fails, the operation MUST fail closed and the stale/unknown
candidate MUST remain unselected. The diagnostic MUST name the attempted
replacement or failure and its recovery evidence.

The normal launcher invocation MUST retain the existing no-digest hot-path
contract except for the bounded pre-exec rejection required to avoid executing
an unverified cache candidate. Release build flags, exact toolchain pin, asset
names, tag/version stamp, canonical digest comparison, and `ai-specs doctor`
evidence MUST remain consistent with the committed trust root. The legacy Bash
fallback remains a distinct governed asset and MUST not be used to bless an
unverified Go cache file. A successful cache acquisition MUST leave an atomic
`<binary>.verified` receipt containing the accepted version, digest, and passing
self-test; the launcher MUST reject a cache executable without a current
receipt. Sync and doctor MUST revalidate the trust-root digest before running
version or self-test commands, so stale bytes are never executed merely for
diagnostics.

#### Scenario: Stale cache binary is not accepted as current

- GIVEN the version-keyed cache contains an executable gate binary
- AND its observed digest, reported version, or self-test does not match the
  current accepted asset state
- WHEN acquisition or materialization evaluates the cache
- THEN it MUST force re-acquisition of the latest canonical asset
- AND it MUST report the cache path, expected/observed digest, version, and
  self-test evidence
- AND it MUST not execute the stale/unknown bytes before the replacement is
  verified
- AND if re-acquisition or replacement fails, it MUST fail closed and leave the
  candidate unselected

#### Scenario: Committed release digest remains authoritative

- GIVEN a release matrix artifact differs from the committed
  `catalog/recipes/worktree-flow/bin/SHA256SUMS` entry
- WHEN the release checksum gate runs
- THEN the release MUST fail with the existing regeneration guidance
- AND no mismatched artifact MUST become an accepted cache asset

#### Scenario: Doctor exposes actionable freshness evidence

- GIVEN a worktree-flow cleanup, launcher, legacy gate, or cached Go asset is
  stale, unknown, or digest-invalid
- WHEN `ai-specs doctor` runs
- THEN it MUST report an ERROR naming the asset state and evidence
- AND it MUST state that ordinary sync will force the latest verified
  replacement, plus the explicit retry/re-acquisition action where applicable
- AND doctor MUST not mutate the project or lock
- AND the diagnostic MUST NOT turn the ordinary sync replacement into a
  preserve-and-defer requirement

#### Scenario: Version and lock drift is distinguishable

- GIVEN the repository `VERSION`, stamped gate version, cache key, and
  `.ai-specs.lock [meta].cli_version` do not describe the same sync state
- WHEN the freshness checks run
- THEN the result MUST identify which version relationship is stale or unknown
- AND it MUST not silently rewrite the lock as part of reporting
