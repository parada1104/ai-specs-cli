# Delta for worktree-flow

## ADDED Requirements

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

## MODIFIED Requirements

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
(Previously: gate classification did not distinguish the request owner root from
the canonical planning root.)

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
(Previously: an explicit `monorepo-apps` value bypassed detection, but silent reclassification was not pinned.)

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

### Requirement: Submodule Worktree Creation Contract

Under a resolved `monorepo-submodules` topology, `/worktree-new` MUST require or infer a `<subrepo>`, validate it against `.gitmodules` (path first, then unique name), and reject uninitialized, unknown, or ambiguous names before any create. Creation MUST use `git -C <subrepo_path> worktree add <absolute-destination> -b <branch> <integration_branch>` with a mandatory absolute destination under the shared superproject `<worktrees_dir>/<subrepo>-<slug>`. Cwd inference MUST use `git rev-parse --show-toplevel`: a toplevel that is an initialized submodule path yields that path; a linked feature worktree under `worktrees_dir` yields the longest initialized-path prefix match on the basename (`<path>-<slug>`). Explicit and inferred values that disagree MUST hard-error.

A request whose context is the superrepo MUST NOT infer a subrepo: creation requires an explicit, validated `<subrepo>`; without it, `/worktree-new` MUST hard-error before any `git worktree add`.
(Previously: the creation contract covered subrepo inference and validation but did not require an explicit subrepo for superrepo-context requests.)

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
