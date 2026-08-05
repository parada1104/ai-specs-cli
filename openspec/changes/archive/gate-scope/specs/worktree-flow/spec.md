# Delta for worktree-flow

## ADDED Requirements

### Requirement: `gate_scope` configuration

`recipes.worktree-flow.config.gate_scope` MUST be one of `auto`, `superrepo`, or
`subrepo`. When absent or empty, it MUST resolve to `auto`. `ai-specs sync` MUST
reject any other value with a non-zero exit and a diagnostic that names the
invalid value and lists the allowed enum. The setting MUST remain independent
from `gate_mode` and `repo_topology`: `gate_mode` controls whether enforcement
runs, `repo_topology` controls worktree topology, and `gate_scope` controls the
repository scope considered by the gate. No aliases or alternate spellings are
valid.

#### Scenario: Missing scope defaults to auto

- GIVEN a manifest with no `gate_scope` under `[recipes.worktree-flow.config]`
- WHEN `ai-specs sync` resolves the worktree-flow configuration
- THEN the effective `gate_scope` MUST be `auto`
- AND sync MUST complete without a missing-key error

#### Scenario: Empty scope defaults to auto

- GIVEN `gate_scope = ""` under `[recipes.worktree-flow.config]`
- WHEN the worktree-flow configuration is resolved
- THEN the effective `gate_scope` MUST be `auto`

#### Scenario: Invalid scope is rejected

- GIVEN `gate_scope = "repository"`
- WHEN `ai-specs sync` runs
- THEN it MUST exit non-zero
- AND stderr MUST name `repository` as invalid
- AND stderr MUST list `auto | superrepo | subrepo` as the allowed values

#### Scenario: Scope dimensions remain independent

- GIVEN `gate_scope = "subrepo"`
- AND `gate_mode = "always"`
- AND `repo_topology = "monorepo-submodules"`
- WHEN the gate configuration is resolved
- THEN each value MUST retain its own meaning
- AND resolving `gate_scope` MUST NOT change `gate_mode` or `repo_topology`

#### Scenario: Alternate scope spelling is rejected

- GIVEN `gate_scope = "super-repo"`
- WHEN `ai-specs sync` runs
- THEN it MUST reject the value rather than normalize it to `superrepo`

### Requirement: Stamped and runtime-resolved scope

The validated effective `gate_scope` MUST be stamped into the distributed
`worktree-gate.sh` hook using a dedicated scope value. The hook MUST be
self-contained at runtime and MUST NOT read a consumer manifest or Python
project internals. A non-empty valid `WORKTREE_GATE_SCOPE` environment override
MUST take precedence over the stamped value. An invalid override MUST emit a
warning and fall back to the stamped value. A missing or invalid stamp MUST emit
a warning and fall back to `auto`. Scope evaluation MUST occur only after
`gate_mode` has been resolved; `gate_mode=off` MUST disable the gate before scope
or topology evaluation.

#### Scenario: Sync stamps the configured scope

- GIVEN `gate_scope = "superrepo"` in a valid manifest
- WHEN `ai-specs sync` materializes the worktree gate
- THEN the generated hook MUST contain the resolved `superrepo` scope stamp
- AND the hook MUST be runnable without loading the manifest or project Python
  modules

#### Scenario: Valid environment scope overrides the stamp

- GIVEN a hook stamped with `superrepo`
- AND `WORKTREE_GATE_SCOPE=subrepo`
- WHEN the hook evaluates a write event
- THEN it MUST use `subrepo` for the scope decision

#### Scenario: Invalid environment scope falls back safely

- GIVEN a hook stamped with `superrepo`
- AND `WORKTREE_GATE_SCOPE=repository`
- WHEN the hook evaluates a write event
- THEN it MUST warn that the environment scope is invalid
- AND it MUST evaluate using the stamped `superrepo` value
- AND it MUST NOT select a different permissive scope

#### Scenario: Missing or invalid stamp falls back to auto

- GIVEN no valid scope stamp is present in the distributed hook
- WHEN the hook evaluates a write event
- THEN it MUST warn about the missing or invalid stamp
- AND it MUST evaluate using `auto`

#### Scenario: Off mode bypasses scope evaluation

- GIVEN `gate_mode=off`
- AND `WORKTREE_GATE_SCOPE=repository`
- WHEN the hook receives any structured or shell write event
- THEN it MUST allow the event before branch or topology checks
- AND it MUST NOT use `gate_scope` as a second bypass mechanism

### Requirement: Proven repository ownership and topology scope

For each candidate target, the gate MUST canonicalize the event cwd and target
using component-aware, symlink-safe path resolution, retaining existing
ancestors when the final target does not yet exist. It MUST resolve the owning
Git repository from the nearest existing ancestor and Git repository facts
before applying any scope exception. A primary checkout MAY be classified as a
superproject or initialized subrepository only when all of the following agree:

the candidate's Git common directory identifies the repository; the containing
superproject has a real `.git` directory and `.gitmodules`; the repository is
registered by a `.gitmodules` path; the submodule status is initialized (a
non-empty status whose prefix is not `-`); and the relationship is unique and
component-contained. `git rev-parse --show-superproject-working-tree` MAY
corroborate the relationship but MUST NOT be its sole proof. Nested, ambiguous,
symlink-escaping, unrelated, or otherwise unresolved relationships MUST NOT
receive a scope-based allow.

#### Scenario: Initialized submodule primary checkout is proven as subrepo

- GIVEN a superproject has a real `.git` directory and `.gitmodules`
- AND `.gitmodules` registers the candidate repository path
- AND `git submodule status` reports that path with an initialized (non-`-`)
  prefix
- AND the candidate repository has its own primary checkout
- WHEN the gate classifies the candidate
- THEN it MUST classify the owner as a proven `subrepo`

#### Scenario: Superproject primary checkout is proven as superrepo

- GIVEN a project has a real `.git` directory and `.gitmodules` with a unique
  initialized submodule relationship available for topology resolution
- AND the candidate is owned by the containing superproject primary checkout
- WHEN the gate classifies the candidate
- THEN it MUST classify the owner as the proven `superrepo`

#### Scenario: Uninitialized submodule does not prove scope

- GIVEN `.gitmodules` registers `apps/api`
- AND `git submodule status` reports `apps/api` with a `-` prefix
- AND a write targets that repository's primary checkout
- WHEN the gate classifies the candidate
- THEN the relationship MUST remain unproven
- AND no central planning exception MUST be granted

#### Scenario: Similar names do not prove ownership

- GIVEN two repositories or submodules have similar basenames
- AND the candidate path is not uniquely registered and component-contained by
  the proposed superproject
- WHEN the gate classifies the candidate
- THEN it MUST leave the relationship unresolved
- AND it MUST NOT use basename or cwd-only inference to grant an allow

#### Scenario: Linked submodule worktree remains an allowed worktree

- GIVEN a candidate is in a linked worktree whose Git directory differs from
  its common Git directory
- WHEN the gate resolves the candidate owner
- THEN it MUST allow the linked worktree under the existing linked-worktree rule
- AND it MUST not require a scope exception to make the write writable

#### Scenario: Standalone or apps topology provides no superrepo proof

- GIVEN `repo_topology` resolves to `standalone` or `monorepo-apps`
- AND the candidate is on a protected primary branch
- WHEN the gate evaluates the candidate
- THEN it MUST retain the existing protected-primary decision
- AND it MUST NOT infer a superrepo relationship from naming or directory layout

### Requirement: Topology-aware protected-branch decision

After `gate_mode` and linked-worktree handling, the gate MUST apply exact
protected-branch matching using `WORKTREE_GATE_PROTECTED`, defaulting to
`main development`. Scope selection MUST NOT add globbing, substring matching,
branch aliases, or new protected names. Every `gate_scope` value MUST retain a
safety floor for a proven initialized subrepository primary checkout: writes to
production or other non-central paths on an exact protected branch MUST remain
blocked. A proven superproject primary checkout MUST remain blocked on exact
protected branches for all paths except the canonical planning subtree defined
below. The central planning exception MUST be evaluated independently of the
selected scope value and MUST never become a broad superproject bypass.

#### Scenario: Protected subrepo production write stays blocked under auto

- GIVEN a proven initialized subrepository primary checkout is on `main`
- AND `gate_scope=auto`
- AND a write targets a production path in that subrepository
- WHEN the gate evaluates the write
- THEN it MUST block with exit `2`

#### Scenario: Protected subrepo production write stays blocked under explicit scopes

- GIVEN a proven initialized subrepository primary checkout is on `development`
- AND the superproject contains an active central `tasks.md`
- WHEN the same production write is evaluated once with `gate_scope=superrepo`
  and once with `gate_scope=subrepo`
- THEN both evaluations MUST block with exit `2`
- AND the central plan MUST NOT authorize the production write by itself

#### Scenario: Protected superproject non-planning write stays blocked

- GIVEN a proven superproject primary checkout is on an exact protected branch
- AND a write targets `<superrepo>/src/`, `.gitmodules`, root configuration,
  release metadata, or another non-planning path
- WHEN the gate evaluates the write under any valid `gate_scope`
- THEN it MUST block with exit `2`

#### Scenario: Exact branch matching is preserved

- GIVEN `WORKTREE_GATE_PROTECTED=main development`
- AND a proven primary checkout is on branch `main-feature`
- WHEN a write targets that primary checkout
- THEN the gate MUST NOT treat `main-feature` as protected solely because it
  starts with `main`
- AND scope selection MUST NOT change this exact-token behavior

#### Scenario: Existing linked-worktree allowance is preserved for every scope

- GIVEN a linked feature worktree owned by a subrepository
- WHEN a write is evaluated with `gate_scope=auto`, `superrepo`, or `subrepo`
- THEN the gate MUST allow it as it does today

### Requirement: Canonical superproject planning boundary

For a proven `monorepo-submodules` relationship, the gate MUST allow a write
from a protected superproject primary checkout only when the canonical target is
a component-aware descendant of the exact subtree
`<superrepo>/openspec/changes/`. The subtree includes active change folders and
`<superrepo>/openspec/changes/archive/`. The final target MAY be nonexistent when
its existing ancestors canonicalize beneath that boundary. The exception MUST
not include prefix lookalikes, sibling root paths, symlink escapes, unrelated
repositories, or any path outside the resolved superproject. All three valid
`gate_scope` values MAY allow this central path, but none MAY allow it without
independent superproject and path proof.

#### Scenario: Active central change artifact is allowed

- GIVEN an initialized submodule relationship proves the containing
  superproject
- AND the superproject primary checkout is on `main`
- AND a write targets `<superrepo>/openspec/changes/gate-scope/specs/worktree-flow/spec.md`
- WHEN the gate evaluates the write under any valid `gate_scope`
- THEN it MUST allow the write

#### Scenario: New central artifact may be created

- GIVEN `<superrepo>/openspec/changes/new-change/` exists beneath the canonical
  planning boundary
- AND `tasks.md` does not yet exist
- WHEN a write targets that nonexistent `tasks.md`
- THEN the gate MUST treat the target as inside the central planning subtree
- AND it MUST allow the write after canonicalizing existing ancestors

#### Scenario: Archived central artifact remains in the boundary

- GIVEN a proven superproject primary checkout on a protected branch
- AND a write targets `<superrepo>/openspec/changes/archive/old-change/`
- WHEN the gate evaluates the write
- THEN it MUST allow the central planning/archive write

#### Scenario: Similar prefix is outside the boundary

- GIVEN the canonical boundary is `/repo/openspec/changes`
- AND a write targets `/repo/openspec/changes-archive/new-change/tasks.md`
- WHEN the gate evaluates the write
- THEN it MUST NOT treat the target as a central descendant
- AND the ordinary protected-primary decision MUST apply

#### Scenario: Superproject structure remains protected

- GIVEN a proven superproject primary checkout on `main`
- WHEN a write targets `/repo/.gitmodules`, `/repo/src/app.py`, or another
  sibling of `openspec/changes/`
- THEN it MUST remain blocked
- AND it MUST not be allowed merely because a central change folder or plan exists

#### Scenario: Symlink escape is not central

- GIVEN a path beneath `<superrepo>/openspec/changes/` contains a symlink that
  resolves outside the superproject
- WHEN a write is addressed through that symlink
- THEN the gate MUST reject the central exception
- AND it MUST apply the ordinary decision to the resolved destination

#### Scenario: Outside path is not reinterpreted as central

- GIVEN a hook event target is outside both the owning repository and the
  resolved superproject
- WHEN the gate evaluates the event
- THEN it MUST NOT reinterpret the target as a central planning write
- AND it MUST retain existing safe handling for unrelated paths

### Requirement: Scope and plan-build authorization remain separate

The worktree gate MUST decide only whether the worktree/branch policy permits a
candidate path. The `plan-build-flow` gate MUST remain the owner of active-plan
authorization for production paths and its existing canonical central-artifact
semantics. A central `tasks.md` or other active plan MUST NOT authorize a
subrepository production write, and a worktree scope allow MUST NOT remove the
plan-build check for production paths. This change MUST NOT alter worktree
creation, cleanup enumeration, shared `.worktrees` layout, or topology
initialization behavior.

#### Scenario: Central artifact write does not require production authorization

- GIVEN a proven central superproject planning path
- AND no active production plan is present
- WHEN the worktree gate evaluates a write to `<superrepo>/openspec/changes/demo/`
- THEN the worktree gate MUST apply only its central-path decision
- AND it MUST not claim that a production path has been authorized

#### Scenario: Active central plan does not bypass subrepo protection

- GIVEN a central `<superrepo>/openspec/changes/demo/tasks.md` exists
- AND a subrepository primary checkout is on protected branch `main`
- WHEN a write targets a production file in that subrepository
- THEN the worktree gate MUST block the protected-primary write
- AND plan-build authorization MUST remain a separate required decision

#### Scenario: Worktree topology behavior is unchanged

- GIVEN a manifest changes only `gate_scope`
- WHEN worktree creation or cleanup runs
- THEN the existing `repo_topology` resolution, shared layout, and submodule
  enumeration behavior MUST remain unchanged

### Requirement: Existing materialized hook safety

Adding `gate_scope` MUST be source-compatible for manifests that omit the key.
Existing materialized hooks or consumer overrides MUST NOT be silently
overwritten merely to add the new scope contract. When sync or doctor detects a
catalog-owned or customized materialized worktree hook that lacks the new scope
contract, it MUST emit a non-blocking warning naming the path and providing
refresh/removal guidance. User-customized bytes MUST remain unchanged unless the
user explicitly refreshes or removes the override through the existing
materialization and ownership workflow.

#### Scenario: Existing manifest keeps safe default behavior

- GIVEN an existing manifest with no `gate_scope`
- WHEN it is synced after this change
- THEN it MUST resolve `gate_scope=auto`
- AND standalone and `monorepo-apps` protected-primary behavior MUST remain
  unchanged
- AND linked worktrees MUST remain allowed

#### Scenario: Customized hook is not silently replaced

- GIVEN a consumer has a customized materialized `worktree-gate.sh` override
  without the new scope stamp
- WHEN `ai-specs sync` or doctor inspects it
- THEN it MUST warn with the path and explicit refresh guidance
- AND it MUST preserve the customized bytes

#### Scenario: Catalog-owned stale hook receives non-blocking guidance

- GIVEN a materialized hook is catalog-owned but does not contain the current
  scope contract
- WHEN sync or doctor evaluates its ownership
- THEN it MUST report that the hook is stale and how to refresh it
- AND it MUST NOT treat the warning as a gate bypass or a sync failure solely
  because the stale artifact is present

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

Each confident candidate path MUST be resolved against the event `cwd` (when
relative) and subjected to the same topology-aware worktree gate policy as
structured path events. A candidate inside a linked worktree MUST remain
allowed. A candidate inside a protected primary checkout MUST block with exit
`2` unless it independently satisfies the proven canonical superproject
planning exception. The gate MUST fail open (exit `0`) when confidence is
insufficient, including missing/empty command fields, unparseable or ambiguous
command text that yields no confident write target, candidates outside the
repository, and read-only commands. Structured path events and shell events MUST
therefore share the same `gate_mode`, `gate_scope`, topology-proof, branch, and
canonical-path decisions.

(Previously: Shell candidates used the existing main-worktree and protected-branch check without a topology-aware central planning exception.)

#### Scenario: Non-central redirection remains blocked

- GIVEN a proven superproject primary checkout is on protected branch `main`
- AND a shell command redirects with `>` to `<superrepo>/src/generated.py`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST exit `2`

#### Scenario: Tee write remains blocked outside central planning

- GIVEN a proven superproject primary checkout is on a protected branch
- AND a shell command pipes to `tee` (or `tee -a`) writing
  `<superrepo>/src/generated.py`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST exit `2`

#### Scenario: In-place editor write remains blocked outside central planning

- GIVEN a proven superproject primary checkout is on a protected branch
- AND a shell command runs `sed -i` or an equivalent in-place editor against
  `<superrepo>/src/generated.py`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST exit `2`

#### Scenario: Interpreter write remains blocked outside central planning

- GIVEN a proven superproject primary checkout is on a protected branch
- AND a Python `-c` or heredoc body calls `Path(...).write_text(...)` or
  `open(..., 'w')` for `<superrepo>/src/generated.py`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST exit `2`

#### Scenario: Read-only interpreter body remains allowed

- GIVEN the main worktree is on a protected branch
- AND a Python (or similar) heredoc or `-c` body only reads or prints and does
  not call a write API such as `write_text` or write-mode `open`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST exit `0`


#### Scenario: Central redirection uses the same exception as path writes

- GIVEN a proven superproject primary checkout is on protected branch `main`
- AND a shell command redirects with `>` to
  `<superrepo>/openspec/changes/demo/tasks.md`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST allow the event under each valid `gate_scope`

#### Scenario: Central Python write uses the canonical boundary

- GIVEN a proven superproject primary checkout is on protected branch `development`
- AND a Python `-c` or heredoc body calls `Path(...).write_text(...)` for a file
  under `<superrepo>/openspec/changes/demo/`
- WHEN `worktree-gate.sh` evaluates the shell event
- THEN it MUST allow the event after canonical path and topology proof

#### Scenario: Linked shell write remains allowed

- GIVEN a linked subrepository feature worktree
- AND a shell command writes to a path inside that worktree
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0` for the same linked-worktree reason as structured writes

#### Scenario: Ambiguous shell command still fails open

- GIVEN a shell event whose command is unbalanced, obfuscated, or otherwise
  yields no confident write-target candidate
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Missing command field still fails open

- GIVEN a shell-shaped event with no extractable command string and no
  structured file path
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`

#### Scenario: Outside shell target is not central

- GIVEN the shell command writes only to `/tmp/out.txt`
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`
- AND it MUST NOT reinterpret the outside target as a central planning path

#### Scenario: Non-write shell command remains allowed

- GIVEN the main worktree is on a protected branch
- AND the shell command is read-only, such as `git status`, `ls`, or `cat path`
- WHEN `worktree-gate.sh` runs
- THEN it MUST exit `0`
