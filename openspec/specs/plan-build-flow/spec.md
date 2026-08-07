# Plan-Build-Flow Specification

## Purpose

Defines the `plan-build-flow` catalog recipe: an **ambient**, skill-only workflow
over the existing multi-phase change ceremony. No slash commands; the bundled
skill auto-invokes on substantial change work, classifies planning depth, and
enforces PR artifact and pre-merge archive gates. Additive, opt-in, coexists
with classic flows.

## Requirements

### Requirement: Recipe manifest and command naming

`catalog/recipes/plan-build-flow/recipe.toml` SHALL declare one bundled skill,
**zero** slash commands, and `on-sync = ["validate-config"]` only. Command and
skill names MUST NOT use `sdd`, `openspec`, or `spec-driven` in any
user-facing identifier.

#### Scenario: Materialization produces skill only

- GIVEN the recipe is enabled and synced
- WHEN materialization completes
- THEN the bundled skill exists
- AND no `/plan`, `/build`, or `/archive` command files are generated

#### Scenario: No new schema or materializer surface

- GIVEN the recipe's `recipe.toml`
- WHEN validated against the current manifest schema
- THEN it requires zero new fields, on-sync actions, or materializer branches

### Requirement: Change depth classifier

The bundled skill SHALL classify each substantial request into exactly one
planning depth before production edits:

- **Full** — explore → proposal → spec → design → tasks
- **Standard** — conditional explore → proposal → spec → tasks
- **Light** — proposal → tasks

Minimum artifacts per depth are normative in *Depth artifact minima*; this
requirement names the chain order only and MUST NOT restate the minima.

Classification SHALL compute a **signal** tier from size/scope heuristics AND
separately detect an **explicit user depth request** when the user names a tier
or clearly equivalent planning depth (including common English and Spanish
phrasings such as "full SDD", "flujo completo", "solo tasks", "tasks only").

The **decided** depth MUST be recorded in `tasks.md` as a standalone lowercase
line of the exact form `Depth: <light|standard|full>`, with no trailing text on
that line, so existing tier-inference consumers keep matching it. Direct
implementation verbs on a request with no existing change folder MUST NOT skip
planning.

#### Scenario: Full depth for ambiguous scope

- **GIVEN** a request for a new cross-cutting capability with unclear boundaries
- **AND** the user did not state a conflicting explicit depth
- **WHEN** planning starts
- **THEN** the full planning chain runs
- **AND** tier minimum artifacts exist before build

#### Scenario: Light depth for scoped fix

- **GIVEN** a one-file bug fix with an explicit file and expected edit
- **AND** the user did not state a conflicting explicit depth
- **WHEN** planning starts
- **THEN** `proposal.md` and `tasks.md` are required, and nothing else
- **AND** no production code is modified during planning

#### Scenario: Direct implement still plans first

- **GIVEN** the user says "implement X" with no `openspec/changes/<slug>/` folder
- **WHEN** the skill evaluates the request
- **THEN** it classifies depth and runs the plan phase before build
- **AND** stops for authorization unless the tier is trivially light and inline build is allowed

### Requirement: Adversarial depth conflict detection

When an explicit user depth request is present, the skill MUST compare it to the
signal tier. If they differ, the skill MUST treat the situation as a **depth
conflict** and MUST NOT silently adopt either value as decided.

Matching request and signal is not a conflict; the skill MAY proceed with that
shared tier.

Absence of an explicit user depth request MUST preserve today's signal-only
classification behavior.

#### Scenario: Explicit request conflicts with signal

- **GIVEN** the user asks for full planning ("flujo completo SDD" or equivalent)
- **AND** size/scope signals indicate Standard
- **WHEN** the classifier runs
- **THEN** a depth conflict is detected
- **AND** neither Full nor Standard is silently recorded as decided without resolution

#### Scenario: Explicit request matches signal

- **GIVEN** the user asks for Light / "solo tasks"
- **AND** size/scope signals also indicate Light
- **WHEN** the classifier runs
- **THEN** no conflict ask is required
- **AND** planning proceeds at Light

#### Scenario: No explicit request

- **GIVEN** the user describes work without naming a depth tier
- **WHEN** the classifier runs
- **THEN** the signal tier is used as decided
- **AND** adversarial conflict handling does not block planning

### Requirement: Conflict ask before planning chain

On a depth conflict, the skill MUST ask the user which depth to use (requested
vs signal) before writing the planning artifacts for the decided tier.

The ask is REQUIRED unless the same user turn that produced the conflict also
states which side wins. A turn resolves the conflict only when it names the
winning depth or expresses an unambiguous preference over the mismatch (for
example "use full even if it looks standard"); merely restating the requested
depth, or adding scope detail, does NOT resolve it and the ask still fires.

The ask SHOULD briefly state both values and MAY recommend one, but the user
choice (or an explicit same-turn resolution) decides. The ask fires in both
directions — requested deeper than signal and requested shallower than signal
are equally conflicts. Until resolution, the skill MUST NOT implement production
code and MUST NOT pretend the conflict is settled.

#### Scenario: Ask fires on conflict

- **GIVEN** a depth conflict is detected
- **AND** the user has not yet chosen requested vs signal
- **WHEN** planning would otherwise start writing tier artifacts
- **THEN** the agent asks which depth to use
- **AND** stops until the user answers

#### Scenario: Same-turn resolution skips repeat ask

- **GIVEN** the user says both the work description and which depth wins
  (e.g. "use full even if it looks standard")
- **WHEN** the classifier detects requested ≠ signal
- **THEN** it adopts the stated resolution without a second ask
- **AND** records annotation as decided by user

#### Scenario: Same-turn restatement does not count as resolution

- **GIVEN** the user asks for full planning and adds more scope detail in the same
  turn, without addressing the mismatch
- **WHEN** the classifier detects requested ≠ signal
- **THEN** the ask still fires
- **AND** no tier artifacts are written until the user answers

#### Scenario: Requested shallower than signal still asks

- **GIVEN** the user asks for "solo tasks" / Light
- **AND** size/scope signals indicate Full
- **WHEN** the classifier runs
- **THEN** a depth conflict is detected and the ask fires
- **AND** the agent MAY recommend the deeper tier in the ask text

### Requirement: Depth resolution annotation

Whenever a depth conflict was detected (including same-turn resolution),
`tasks.md` MUST annotate the resolution using these four labels, each on its own
line, separate from the `Depth:` line:

- `Requested depth: <tier>`
- `Signal depth: <tier>`
- `Decided depth: <tier>`
- `Decision source: <user|signal>` — `user` whenever a human chose, including
  same-turn resolution

Tier values MUST be lowercase `light`, `standard`, or `full`. `Decided depth`
MUST equal the tier on the `Depth:` line.

The decided depth MUST also appear in the ordinary standalone `Depth: <tier>`
line used by existing plan-build consumers. Annotation MUST NOT be appended to
that line as a suffix or parenthetical, because trailing text prevents existing
tier inference from matching it.

When there was no conflict, existing `Depth: <tier>` recording remains
sufficient; optional confirmation annotation is allowed but not required.

#### Scenario: Conflict annotated after user chooses requested

- **GIVEN** requested=full, signal=standard, user chooses full
- **WHEN** `tasks.md` is written
- **THEN** it contains a standalone line `Depth: full`
- **AND** it contains `Requested depth: full`, `Signal depth: standard`,
  `Decided depth: full`, and `Decision source: user` on separate lines

#### Scenario: Conflict annotated after user chooses signal

- **GIVEN** requested=full, signal=standard, user chooses standard
- **WHEN** `tasks.md` is written
- **THEN** it contains a standalone line `Depth: standard`
- **AND** it contains `Requested depth: full`, `Signal depth: standard`,
  `Decided depth: standard`, and `Decision source: user` on separate lines

#### Scenario: Annotation never suffixes the Depth line

- **GIVEN** any conflict resolution
- **WHEN** `tasks.md` is written
- **THEN** the `Depth:` line carries only the decided tier
- **AND** requested/signal/decided/source appear as separate lines

### Requirement: Higher decided tier completes its chain

If conflict resolution selects a deeper tier than the signal, the skill MUST
complete the entire planning chain required by the decided tier before build
authorization is considered satisfied for that change. Artifacts already written
for the shallower tier MUST NOT be relabelled as satisfying the deeper tier; the
missing phases MUST actually run.

#### Scenario: Upgrade from Standard signal to Full decision

- **GIVEN** signal was Standard but decided depth is Full
- **WHEN** planning continues after resolution
- **THEN** Full-tier minimum artifacts are produced before build
- **AND** production code remains unmodified during planning

### Requirement: Ambient planning trigger

The bundled skill SHALL auto-invoke on substantial change requests, run the
classified planning chain, and stop for human authorization. Planning MUST NOT
require slash commands or a dedicated worktree.

#### Scenario: Plan stops before implementation

- GIVEN a developer requests a substantial change
- WHEN the planning phase chain for the classified depth completes
- THEN `tasks.md` exists and no production code files were modified

### Requirement: Ambient build trigger

After authorization, the skill SHALL run apply → verify → artifact/PR gates →
archive-tail (pre-merge) without exposing slash commands.

#### Scenario: Build implements, verifies, and closes after authorization

- GIVEN authorized tasks from a prior planning pass
- WHEN the developer approves implementation
- THEN implementation, verification, and change-folder close complete without a separate archive command

### Requirement: PR artifact gate

The skill and generated brief fragments SHALL block PR/MR creation until the
matching `openspec/changes/<slug>/` folder on the review branch contains the
tier minimum planning files and those files are committed.

#### Scenario: PR blocked without change folder

- GIVEN implementation is complete but no change folder exists on the branch
- WHEN an agent attempts to open a PR
- THEN the skill stops with a blocker to complete planning first

#### Scenario: PR allowed with tier minimum files

- **GIVEN** a standard-tier change with `proposal.md`, `tasks.md`, and spec deltas under `specs/`
- **WHEN** the artifact gate is evaluated before PR creation
- **THEN** PR creation may proceed

#### Scenario: PR blocked for Light without proposal

- **GIVEN** a light-tier change whose committed folder holds only `tasks.md`
- **WHEN** the artifact gate is evaluated before PR creation
- **THEN** the skill stops with a blocker naming `proposal.md`

### Requirement: Pre-merge archive gate

Archive-tail MUST run on the review branch before merge. Post-merge archive as
the boundary MUST be rejected. This aligns with the bound `vcs-pr-flow` contract.

#### Scenario: Archive before merge on review branch

- GIVEN a PR is ready to merge
- WHEN archive-tail runs
- THEN `openspec/changes/<slug>/` moves to `openspec/changes/archive/<slug>/` on the review branch
- AND merge proceeds only after that commit is pushed

#### Scenario: Post-merge archive rejected

- GIVEN a PR has already merged to the base branch
- WHEN archive-tail is invoked
- THEN the skill treats post-merge archive as invalid for the change boundary

### Requirement: Archive channel degradation

The automatic close step SHALL gracefully no-op vault and tracker outputs when
integrations are absent, while still completing the change-folder close.

#### Scenario: Close without vault/tracker recipes

- GIVEN neither `vault-canonical-store` nor `trello-mcp-workflow` is enabled
- WHEN the close step runs
- THEN it emits a note that vault/tracker output was skipped
- AND the change folder still closes successfully

### Requirement: Orchestrator-absence degradation

When no gentle-ai orchestrator is available, the bundled skill SHALL instruct
the single agent to run mapped phases inline as one conversation.

#### Scenario: Inline execution without orchestrator

- GIVEN gentle-ai is not present
- WHEN planning or build phases run
- THEN the skill runs equivalent phases inline and no phase is silently skipped

### Requirement: Artifact store degradation and default

When Engram is unavailable, the skill SHALL fall back to file artifacts. When
Engram is present but no preflight resolved a store, the default SHALL be file
artifacts under `openspec/changes/<slug>/`.

#### Scenario: Default store with Engram but no preflight

- GIVEN Engram is available and no artifact-store preflight ran
- WHEN planning starts producing artifacts
- THEN artifacts are written as files, not memory-only

### Requirement: Vocabulary hygiene in generated output

Generated `[provides.brief]` fragments and the recipe README MUST NOT contain
the strings "SDD", "OpenSpec", or "spec-driven", and MUST NOT reference
`/plan` or `/build`.

#### Scenario: Brief and README are vocabulary-clean

- GIVEN the recipe is synced
- WHEN brief fragments and README are scanned
- THEN forbidden vocabulary and slash-command names are absent

### Requirement: Worktree-flow cross-reference

Brief fragments SHALL cross-reference worktree usage for implementation work
when `worktree-flow` is enabled, without a hard `requires` dependency.

#### Scenario: Cross-reference present without hard dependency

- GIVEN both recipes are enabled
- WHEN the generated brief is inspected
- THEN it references worktree usage for implementation
- AND the recipe syncs standalone without `worktree-flow` enabled

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


### Requirement: Depth artifact minima

The bundled skill SHALL require these minimum planning artifacts before build,
and the pre-merge guardian SHALL enforce the same sets against the archived
folder before merge:

- **Light** — `proposal.md` and `tasks.md`; a short Why / What / Non-goals
  proposal is sufficient and `design.md` is not required.
- **Standard** — `proposal.md`, `tasks.md`, and at least one `specs/**/*.md`.
  `explore.md` is additionally required at plan time when the explore criteria
  match.
- **Full** — `tasks.md`, plus `proposal.md` or `design.md`, plus at least one
  `specs/**/*.md`; explore remains skill-enforced, not guardian-enforced.

#### Scenario: Light requires proposal and tasks

- **GIVEN** a one-file bug fix classified as Light
- **WHEN** planning completes
- **THEN** both `proposal.md` and `tasks.md` exist under the change folder

#### Scenario: Standard requires proposal, tasks, and spec

- **GIVEN** a scoped multi-file feature classified as Standard
- **WHEN** planning completes
- **THEN** `proposal.md`, `tasks.md`, and at least one `specs/**/*.md` exist
- **AND** either `explore.md` exists or `tasks.md` contains `Explore: skipped —`

#### Scenario: Full minima do not require explore on disk for merge

- **GIVEN** Full minima and a conforming verify report exist without `explore.md`
- **WHEN** the pre-merge guardian runs
- **THEN** it reports OK and leaves explore enforcement to the skill

### Requirement: Standard explore enforcement criteria

For Standard depth, the skill SHALL require `explore.md` when any of these hold
at plan start: two plausible approaches with material trade-offs, unknown
concrete files, conflicting project guidance, user uncertainty, or a prior
attempt that failed or was reverted. It SHALL skip explore only when concrete
paths and expected behavior are known, one obvious approach exists, and no
conflict, uncertainty, or retry signal applies. A skipped decision MUST be
recorded as `Explore: skipped — <reason>` in `tasks.md`.

Explore at Standard and Full is a plan-phase skill responsibility. No machine
gate SHALL block PR creation, archive-tail, or merge for a missing `explore.md`.

### Requirement: Staged verify gate

After apply and before archive-tail, and again before merge, verification
evidence SHALL be evaluated by decided depth:

- **Light — advisory**: missing evidence MAY warn but MUST NOT block.
- **Standard — enforcement**: dedicated `verify-report.md` is required with a
  non-failing verdict, command, exit `0`, valid `YYYY-MM-DD` calendar date, and
  7–40 hex commit SHA. Evidence in `tasks.md` does not count.
- **Full — required**: dedicated `verify-report.md` requires strict `PASS`,
  `ready_for_archive: true`, and deterministic mapping to every success
  criterion.

The canonical evidence block uses `Verdict`, `Command`, `Exit`, `Date`,
`Commit`, and (for Full) `ready_for_archive: true`, followed by a
`## Success-criteria mapping` block. The authoritative source is `proposal.md`
when present, otherwise `design.md`. An existing proposal with a missing or
empty `## Success Criteria` section MUST block rather than fall back to design;
the authoritative source MUST contain exactly one non-empty heading, and
duplicate `## Success Criteria` headings MUST be rejected. Each top-level bullet
there is assigned a 1-based ordinal; Full reports MUST contain exactly one
`- Criterion N: PASS` mapping row for each ordinal, with no duplicate, missing,
unknown, or non-PASS rows. Accepted synonyms are `Status`/`Overall`, `Exit
code`/`Exit status`, and `SHA`/`Revision`.

#### Scenario: Standard archive-tail blocks without report

- **GIVEN** Standard minima are present and no conforming `verify-report.md`
- **WHEN** archive-tail is attempted
- **THEN** the verify gate blocks before archiving

#### Scenario: Light archive without evidence is allowed

- **GIVEN** Light minima are present and no `verify-report.md` exists
- **WHEN** archive-tail and the pre-merge guardian run
- **THEN** neither adds a verify blocker

#### Scenario: Full merge requires strict PASS, ready marker, and complete mapping

- **GIVEN** Full minima are present and the report is missing, failing, lacks
  `ready_for_archive: true`, or omits any success-criteria mapping row
- **WHEN** the pre-merge guardian runs
- **THEN** it blocks until the report conforms

#### Scenario: Verify is checked at both enforcement points

- **GIVEN** an authorized Standard or Full change
- **WHEN** build runs
- **THEN** verification is produced before archive-tail and rechecked after archive

### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts, missing staged verify evidence, or a
still-active (non-archived) change folder is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` (defaulting
`AI_SPECS_HOME` to `$HOME/.ai-specs` when unset). Sync MUST NOT materialize a
per-project copy under `ai-specs/bin/`.

Hard blockers are: active change folder; missing archive; Light missing
`proposal.md` or `tasks.md`; Standard missing `proposal.md`, `tasks.md`, a
`specs/**/*.md` delta, or a conforming `verify-report.md`; and Full missing
`tasks.md`, `proposal.md` or `design.md`, a `specs/**/*.md` delta, or a
conforming report with strict `PASS` and `ready_for_archive: true`. Missing
`explore.md` is never a blocker, and only the requested slug is evaluated.

#### Scenario: Merge blocked when change folder still active

- GIVEN `openspec/changes/<slug>/` still exists (not archived)
- WHEN an agent attempts to merge the PR/MR
- THEN the skill stops with a plain-language blocker requiring archive-tail first

#### Scenario: Guardian path is CLI-home

- GIVEN `plan-build-flow` (or a VCS merge skill) is enabled
- WHEN an agent runs the pre-merge guardian
- THEN it uses `${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py`
- AND the recipe does not target `ai-specs/bin/premerge_guardian.py`
#### Scenario: Light archive requires proposal but not verify evidence

- **GIVEN** Depth Light and an archive with `tasks.md` but no `proposal.md`
- **WHEN** the pre-merge guardian runs
- **THEN** it fails naming `proposal.md` but does not add a verify blocker

#### Scenario: Standard archive requires proposal, spec, and verify report

- **GIVEN** Depth Standard and the archive lacks a minimum or conforming report
- **WHEN** the pre-merge guardian runs
- **THEN** it fails with a tier-minima or verify-evidence blocker

#### Scenario: Missing explore is never a guardian blocker

- **GIVEN** Full minima and a conforming report exist without `explore.md`
- **WHEN** the pre-merge guardian runs
- **THEN** it reports OK

#### Scenario: Guardian ignores unrelated archived changes

- **GIVEN** unrelated archived folders predate this contract and are non-conforming
- **WHEN** the guardian runs for one slug
- **THEN** only that slug is evaluated
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
outside-repository writes.
The gate SHALL be non-bypassable: it exposes no off/on/ask mode, so the only
way past a blocked production edit is to provide the active plan required by
the resolved artifact root.

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

## Acceptance Criteria (test map)

| AC | Test | Req |
|----|------|-----|
| AC1 | `test_recipe_materializes_skill_only` | manifest |
| AC2 | `test_recipe_adds_no_schema_surface` | manifest |
| AC3 | `eval_plan_build_flow_live` / `ac3_plan_stops_before_apply` (live); materialization partial | plan stops before implementation |
| AC4 | `tests/evals/scenarios/plan-build-flow/ac4_*` (planned live) | ambient build mapping |
| AC5 | `tests/evals/scenarios/plan-build-flow/ac5_*` (planned live) | archive degradation |
| AC6 | transcript judge layer (deferred) | orchestrator absence |
| AC7 | `tests/evals/scenarios/plan-build-flow/ac7_*` (planned live) | artifact store default |
| AC8 | `test_brief_and_readme_vocabulary_clean` | vocabulary |
| AC9 | `test_implementation_brief_references_worktree_flow` | worktree |
| AC10 | `test_classic_sdd_commands_unchanged` | coexistence |
| AC11 | `test_skill_has_change_depth_classifier` | classifier |
| AC12 | `test_skill_has_pr_and_archive_gates` | PR/archive gates |
| AC13 | `test_brief_mentions_depth_and_pr_gate` | brief fragments |
| AC14 | `test_plan_build_gate_hook` (unit); `ac8_approval_verb_without_folder` (live) | pre-tool-use artifact gate |
