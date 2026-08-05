# Delta for Plan-Build-Flow

## ADDED Requirements

### Requirement: Topology-aware planning artifact root

The plan-build gate MUST derive its planning-artifact root from the existing
repository topology and worktree facts at runtime. For a target in an initialized
submodule linked worktree under the supported shared superproject worktree layout,
the artifact root MUST be the containing superproject root. For standalone
repositories and non-submodule worktrees, the artifact root MUST remain the
nearest repository root used by the existing gate. The resolver MUST use the
recognized submodule relationship and path layout, not a user-configured root.

#### Scenario: Linked submodule worktree uses the central superproject root

- GIVEN a superproject has an initialized submodule
- AND the submodule owns a linked worktree under the shared superproject worktree directory
- AND the superproject contains `openspec/changes/demo/tasks.md`
- WHEN the gate evaluates a production write in that linked submodule worktree
- THEN it MUST look for active plans beneath the superproject's `openspec/changes/`
- AND it MUST allow the production write under the existing any-active-plan semantics

#### Scenario: Standalone repository keeps its repository root

- GIVEN a standalone repository has no initialized submodules
- AND its repository root contains `openspec/changes/demo/tasks.md`
- WHEN the gate evaluates a production write in that repository
- THEN it MUST use that repository root for the active-plan lookup
- AND its decision MUST be unchanged from the existing standalone contract

#### Scenario: Non-submodule worktree keeps nearest-root behavior

- GIVEN a worktree is not owned by an initialized submodule of a recognized superproject
- AND the worktree repository has its own `openspec/changes/demo/tasks.md`
- WHEN the gate evaluates a production write
- THEN it MUST use the worktree repository root
- AND it MUST NOT redirect the lookup to an unrelated parent directory

#### Scenario: Central root is not user-configured

- GIVEN a manifest enables the plan-build recipe without a planning-root setting
- AND the manifest has no `[sdd]` section or artifact-root field
- WHEN the gate resolves the artifact root
- THEN it MUST derive the root from repository topology
- AND it MUST NOT require, create, or read a new `[sdd]` configuration, decision matrix, or `artifact_root` setting

### Requirement: Robust submodule root discovery

The resolver MUST identify the Git repository owning the target using the existing
`rev-parse --show-toplevel` behavior as its first repository fact, then associate a
recognized submodule worktree with the correct superproject using the existing
`worktree-flow` topology and shared-layout facts. It MUST NOT rely solely on
`git rev-parse --show-superproject-working-tree`, because that value may be empty
from a linked submodule worktree. A submodule match MUST be initialized and tied to
the actual containing superproject; similarly named submodules or unrelated parent
repositories MUST NOT be accepted as the match.

#### Scenario: Linked worktree resolves when superproject probe is empty

- GIVEN a linked submodule worktree where `git rev-parse --show-superproject-working-tree` returns empty output
- AND the worktree path matches an initialized submodule and the supported shared superproject layout
- WHEN the gate resolves the planning root
- THEN it MUST still resolve the owning superproject
- AND it MUST use that superproject for active-plan lookup and central artifact writes

#### Scenario: Similar submodule names do not select the wrong parent

- GIVEN two repositories contain initialized submodules with similar names
- AND only one submodule owns the target linked worktree
- WHEN the gate resolves the planning root
- THEN it MUST select the superproject that actually registers and contains the target submodule
- AND it MUST NOT use the similarly named repository's `openspec/changes/` directory

#### Scenario: Unresolved topology does not grant production access

- GIVEN a target appears to be in a submodule worktree
- AND the resolver cannot establish an initialized submodule-to-superproject relationship
- WHEN the gate evaluates a production write
- THEN it MUST fall back to the safe nearest-repository gate behavior
- AND it MUST NOT allow the write merely because a possible parent directory contains a plan

### Requirement: Canonical path normalization and symlink boundaries

Before comparing repository, worktree, and artifact paths, the resolver MUST
normalize the event target and working directory, including paths for files that do
not yet exist. Artifact-root checks MUST compare canonical path components with a
repository-boundary-aware prefix rather than a textual string prefix. A path reached
through a symlink MUST qualify only when its resolved target remains beneath the
resolved repository or central artifact root; symlink escapes and unrelated
outside-repository paths MUST NOT receive the central allowance.

#### Scenario: Non-existent plan file uses the central boundary

- GIVEN a recognized submodule linked worktree
- AND the superproject's `openspec/changes/demo/` directory exists but `tasks.md` does not yet exist
- WHEN a write targets the not-yet-created `tasks.md` path
- THEN normalization MUST preserve the intended central `openspec/changes/demo/` location
- AND the gate MUST apply the nearest-root artifact allowance without requiring the file to pre-exist

#### Scenario: Symlinked central path cannot escape the artifact root

- GIVEN a central `openspec/changes/` path contains a symlink that resolves outside the superproject
- AND a write is addressed through that symlink from a submodule worktree
- WHEN the gate evaluates the write
- THEN it MUST reject the additional nearest-root artifact allowance
- AND it MUST apply the ordinary production/non-production decision to the resolved destination

#### Scenario: Prefix lookalikes are outside the central artifact root

- GIVEN the resolved central artifact root is `/repo/openspec/changes`
- AND a target normalizes to `/repo/openspec/changes-archive/demo/tasks.md`
- WHEN the gate evaluates the target
- THEN it MUST NOT treat the target as a descendant of `openspec/changes/`
- AND it MUST NOT grant the nearest-root artifact allowance

#### Scenario: Unrelated outside-repository path is not broadened

- GIVEN a hook event has a target outside both the submodule repository and its resolved superproject
- WHEN the gate evaluates the event
- THEN it MUST NOT reinterpret that path as a central artifact write
- AND it MUST retain the existing safe handling for unrelated out-of-repository paths

### Requirement: Centralized artifact convention

For a recognized `monorepo-submodules` project, the superproject's
`openspec/changes/<slug>/` tree MUST be the single canonical planning-artifact
location for this gate. The gate MAY continue to observe a subrepository-local
change folder according to its safe nearest-root fallback, but this change MUST
NOT create, synchronize, migrate, or delete duplicate subrepository plans.

#### Scenario: Central active plan gates subrepository production work

- GIVEN an initialized submodule owns the target linked worktree
- AND the superproject contains an active `openspec/changes/demo/tasks.md`
- AND no central plan is present in the submodule worktree's own repository root
- WHEN a production write is evaluated
- THEN the write MUST be allowed using the central active plan
- AND the gate MUST NOT require a duplicate subrepository plan

#### Scenario: Central absence blocks production work

- GIVEN an initialized submodule owns the target linked worktree
- AND the superproject has no active `openspec/changes/*/tasks.md`
- WHEN a production write is evaluated
- THEN the gate MUST block the write
- AND its diagnostic MUST identify the central planning location as the missing prerequisite

#### Scenario: Archived-only central plans do not satisfy the gate

- GIVEN an initialized submodule owns the target linked worktree
- AND the superproject contains only `openspec/changes/archive/demo/tasks.md`
- WHEN a production write is evaluated
- THEN the gate MUST block the write
- AND an archived plan MUST NOT count as an active plan

### Requirement: Central artifact writes are narrowly allowed

For a recognized submodule worktree, the gate MUST allow writes whose normalized
destination is beneath the resolved superproject `openspec/changes/` path,
including active change folders and the existing archive subtree, because that
target resolves to the superproject as the nearest repository root and is
covered by the existing nearest-root artifact allowance before production
classification. No separate central-write branch is required. This allowance
MUST NOT authorize arbitrary writes to the superproject, its production
directories, or other repository roots. Existing production-directory scope and
non-production handling remain otherwise unchanged.

#### Scenario: Central plan creation is allowed before an active plan exists

- GIVEN a recognized submodule linked worktree has no active central plan
- AND a write targets the superproject's `openspec/changes/demo/tasks.md`
- WHEN the hook evaluates the write
- THEN it MUST exit 0
- AND the agent MUST be able to create the central planning artifact

#### Scenario: Central artifact updates remain allowed

- GIVEN a recognized submodule linked worktree
- AND a write targets an existing file beneath the superproject's `openspec/changes/demo/`
- WHEN the hook evaluates the write
- THEN it MUST exit 0 regardless of whether the change is already active
- AND the allowance MUST remain limited to the canonical artifact subtree

#### Scenario: Central production path remains gated

- GIVEN a recognized submodule linked worktree has no active central plan
- AND a write targets a superproject production path such as `src/app.py`
- WHEN the hook evaluates the write
- THEN it MUST block the write
- AND the nearest-root artifact allowance MUST NOT be used as a superproject-wide bypass

#### Scenario: Central archive preparation retains artifact allowance

- GIVEN a recognized submodule linked worktree
- AND a write targets a path beneath the superproject's `openspec/changes/archive/`
- WHEN the hook evaluates the write
- THEN it MUST apply the existing nearest-root planning-artifact allowance
- AND it MUST not infer that the archive path authorizes production writes

### Requirement: Cross-repository planning has no orchestration side effects

Evaluating the plan-build gate or resolving a planning-artifact root MUST NOT create
or remove worktrees, branches, plans, pull requests, or archive entries; coordinate
multiple repositories; or introduce per-subrepository stores or synchronization.
The existing `worktree-flow` topology and layout contract remains the source of
repository facts, while `plan-build-flow` only consumes those facts for gate
resolution.

#### Scenario: Gate evaluation is read-only with respect to repository topology

- GIVEN a recognized submodule worktree and a hook event
- WHEN the gate resolves the artifact root and evaluates the event
- THEN it MUST only inspect the existing repository and path state
- AND it MUST NOT create or delete a worktree, branch, or repository artifact as a side effect

#### Scenario: One central plan remains canonical

- GIVEN a cross-repository change affects multiple initialized submodules
- WHEN the gate evaluates production writes in those submodule worktrees
- THEN all eligible worktrees MUST consult the same superproject change tree
- AND the gate MUST NOT create one plan store or synchronization protocol per submodule

## MODIFIED Requirements

### Requirement: Pre-tool-use artifact gate hook

The `plan-build-flow` recipe SHALL distribute a `pre-tool-use` runtime hook
(`hooks/plan-build-gate.sh`, `matcher = Edit|Write|MultiEdit|NotebookEdit`,
`blocking = true`) that machine-enforces the plan-before-build artifact
precondition. The hook SHALL follow the normalized hook contract: read stdin
JSON `{event, tool_name, tool_input, cwd}`, exit `0` to allow, exit `2` to
block, and fail open (exit `0`) on malformed input or an event that cannot be
associated with a repository target. For a valid target in a recognized
submodule worktree, inability to discover the central root MUST NOT grant
production-write access; the hook MUST use the safe nearest-repository behavior
instead.

The hook SHALL derive the nearest repository root from the normalized target
before performing active-plan lookup or the nearest-root planning-write
allowance. For a production target in a recognized submodule worktree with no
nearest-root plan, it SHALL lazily resolve the central superproject root for
the active-plan lookup. It SHALL use the central superproject root for that
lookup in an initialized submodule linked worktree in the supported shared
layout, and the current repository root for standalone and other non-submodule
worktrees. The hook MUST normalize target and working paths, including
non-existent destinations, and enforce repository-boundary-aware path
comparisons.

The hook SHALL block a matched edit only when BOTH hold: (a) the target path is
under a production directory (default top-level `src`, `lib`, `catalog`,
overridable via `PLAN_BUILD_GATE_PATHS` — scope configuration only), AND (b) no
active change folder exists (no `openspec/changes/*/tasks.md` outside
`archive/`) beneath the resolved artifact root. It SHALL allow edits under the
resolved artifact root's `openspec/changes/**` path, non-production paths, and
gitignored agent config (`.claude/settings*.json`, `.claude/hooks/*`)
unconditionally, subject to the canonical-path and symlink-boundary rules above.
The nearest-root artifact allowance MUST NOT authorize arbitrary superproject or
outside-repository writes. The gate SHALL be non-bypassable: it exposes no
off/on/ask mode, so the only way past a blocked production edit is to provide the
active plan required by the resolved artifact root.

Because the hook pipeline exposes no pre-file-write event for `cursor`, this hook
enforces on `claude`, `opencode`, `pi`, and `omp` only; `cursor` retains the
advisory skill + workflow-rules layer.

#### Scenario: Production edit blocked without a central change folder

- GIVEN no `openspec/changes/*/tasks.md` exists beneath the resolved central superproject root outside `archive/`
- AND a `Write` targets a production file under a linked submodule worktree
- WHEN the hook receives the normalized event
- THEN it MUST exit 2 and surface a plain-language reason to provide the central plan

#### Scenario: Production edit allowed with a central active plan

- GIVEN `openspec/changes/<slug>/tasks.md` exists beneath the resolved central root outside `archive/`
- AND a `Write` targets a production file under a linked submodule worktree
- WHEN the hook receives the event
- THEN it MUST exit 0

#### Scenario: Standalone production behavior remains unchanged

- GIVEN a standalone repository has no active `openspec/changes/*/tasks.md`
- AND a `Write` targets a production file under that repository
- WHEN the hook receives the event
- THEN it MUST exit 2 as before
- AND when an active plan is added at that repository root, the same production write MUST exit 0

#### Scenario: Writing the central plan is never blocked

- GIVEN no active change folder exists at the resolved central root
- AND a `Write` targets the resolved superproject `openspec/changes/<slug>/tasks.md`
- WHEN the hook receives the event from a recognized submodule worktree
- THEN it MUST exit 0

#### Scenario: Non-artifact superproject production write is not allowed by central scope

- GIVEN no active central change folder exists
- AND a `Write` targets a production file in the superproject outside `openspec/changes/**`
- WHEN the hook receives the event from a recognized submodule worktree
- THEN it MUST exit 2
- AND the nearest-root artifact allowance MUST NOT broaden the write scope

#### Scenario: Fail-open on malformed input

- GIVEN malformed JSON, a missing `file_path`, or an event that cannot identify a target repository
- WHEN the hook runs
- THEN it MUST exit 0
- AND it MUST NOT make a broad superproject allowance based on incomplete input

#### Scenario: No mode bypass

- GIVEN a production `Write` with no active plan beneath the resolved artifact root
- AND any `PLAN_BUILD_GATE_MODE` value is set in the environment
- WHEN the hook runs
- THEN it MUST still exit 2
- AND the mode environment variable MUST have no effect because the gate has no off switch

### Requirement: Coexistence with classic SDD

Enabling `plan-build-flow` MUST NOT modify, remove, or rename any existing classic
SDD command, skill, or recipe outside this recipe's own surface. This change MUST
also leave removed `[sdd]` configuration and artifact-store concepts removed: it
MUST NOT reintroduce a `[sdd]` section, a planning decision matrix, an
`artifact_root` selector, or a per-subrepository artifact store as part of root
discovery.

#### Scenario: Classic flow and removed configuration remain unaffected

- GIVEN a project with classic SDD commands already synced
- AND the project does not declare `[sdd]` or `artifact_root`
- WHEN `plan-build-flow` is enabled and synced
- THEN all pre-existing non-plan-build-flow commands and skills remain unchanged
- AND the gate still resolves standalone or central roots from topology without new configuration
