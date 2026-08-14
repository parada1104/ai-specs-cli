# Delta for plan-build-flow

## MODIFIED Requirements

### Requirement: Orchestrator-absence degradation

When no gentle-ai orchestrator is available, the bundled skill SHALL instruct
the single agent to run mapped phases inline as one conversation.

Absent and disabled external orchestration MUST behave identically and MUST NOT
introduce a new provider prerequisite.
(Previously: only the absent external-orchestrator state was specified; the
disabled state was not pinned.)

#### Scenario: Inline execution without orchestrator

- GIVEN gentle-ai is not present or is disabled
- WHEN planning or build phases run
- THEN the skill runs equivalent phases inline and no phase is silently skipped
- AND no new external provider prerequisite is introduced

### Requirement: Topology-aware planning artifact root

The plan-build gate MUST derive its planning-artifact root from the existing
repository topology and worktree facts at runtime. For a target in an initialized
submodule linked worktree under the supported shared superproject worktree layout,
the artifact root MUST be the containing superproject root. For standalone
repositories and non-submodule worktrees, the artifact root MUST remain the
nearest repository root used by the existing gate. The resolver MUST use the
recognized submodule relationship and path layout, not a user-configured root.

The resolved planning root MUST be propagated as explicit request context to
artifact writers, renderers, and the pre-merge guardian. Artifact phases MUST
resolve `openspec/changes/<slug>/` against that propagated root; they MUST NOT
resolve it relative to the process cwd or a subrepo primary checkout (no relative
subrepo plan leakage). When the planning root cannot be resolved — missing,
ambiguous, detached, or uninitialized topology — the system MUST fail safe to
nearest-root behavior and MUST NOT grant production access on the strength of a
possible parent directory.
(Previously: the gate derived the root at runtime but the resolved planning root
was not propagated to artifact writers or the guardian, so artifact paths could
still resolve relative to a subrepo checkout.)

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

#### Scenario: Subrepo-context artifact write lands on the canonical superrepo path

- GIVEN a subrepo request whose planning root is the proven superrepo
- AND an artifact phase writes `openspec/changes/<slug>/tasks.md`
- WHEN the write resolves the artifact path
- THEN the artifact exists only under `<super>/openspec/changes/<slug>/`
- AND no relative `openspec/changes/...` artifact appears inside the subrepo checkout

#### Scenario: Unresolvable planning root fails safe

- GIVEN a detached or uninitialized target state
- AND the request context cannot establish owner or planning root
- WHEN an artifact write or gate evaluation occurs
- THEN it falls back to the safe nearest-root behavior
- AND it MUST NOT grant production access merely because a possible parent directory contains a plan

### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts, a still-active (non-archived) change
folder, or (for Standard and Full) missing verify evidence per the staged verify
gate is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` (defaulting
`AI_SPECS_HOME` to `$HOME/.ai-specs` when unset). Sync MUST NOT materialize a
per-project copy under `ai-specs/bin/`.

The guardian MUST accept the resolved planning root from the propagated request
context (an explicit root argument or equivalent context) and MUST NOT depend on
the process cwd to locate the canonical change tree. When the planning root
cannot be resolved, the guardian MUST fail safe: it MUST NOT skip tier-minima or
verify checks.
(Previously: the guardian accepted an explicit root but did not require it and
could rely on cwd-derived paths.)

Hard blockers (do **not** merge):

1. `openspec/changes/<slug>/` still exists (active, not archived).
2. `openspec/changes/archive/<slug>/` is missing.
3. Archived folder lacks the tier minimum files from *Depth artifact minima*:
   - Light: `tasks.md` and `proposal.md`
   - Standard: `tasks.md`, `proposal.md`, and at least one `specs/**/*.md`
   - Full: `tasks.md`, `proposal.md` or `design.md`, and at least one
     `specs/**/*.md`
4. Standard: archived folder lacks a `verify-report.md` that satisfies the
   Standard evidence shape in *Staged verify gate*.
5. Full: archived folder lacks a `verify-report.md` that satisfies the Full
   evidence shape in *Staged verify gate*.

The guardian MUST NOT add a blocker for a missing `explore.md` at any depth, and
MUST NOT add a verify blocker at Light. The guardian SHALL evaluate only the
slug under check; it MUST NOT inspect or require changes to other archived
changes.

#### Scenario: Merge blocked when change folder still active

- GIVEN `openspec/changes/<slug>/` still exists (not archived)
- WHEN an agent attempts to merge the PR/MR
- THEN the skill stops with a plain-language blocker requiring archive-tail first

#### Scenario: Guardian path is CLI-home

- GIVEN `plan-build-flow` (or a VCS merge skill) is enabled
- WHEN an agent runs the pre-merge guardian
- THEN it uses `${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py`
- AND the recipe does not target `ai-specs/bin/premerge_guardian.py`

#### Scenario: Light archive requires proposal

- GIVEN Depth light and the archive contains `tasks.md` but not `proposal.md`
- WHEN the pre-merge guardian runs
- THEN it fails with a tier-minima blocker naming `proposal.md`

#### Scenario: Standard archive requires proposal and spec

- GIVEN Depth standard and the archive lacks `proposal.md` or any `specs/**/*.md`
- WHEN the pre-merge guardian runs
- THEN it fails with a tier-minima blocker

#### Scenario: Missing explore is never a guardian blocker

- GIVEN Depth full and an archive with tier minima and a passing `verify-report.md`
- AND no `explore.md` in the archived folder
- WHEN the pre-merge guardian runs
- THEN it reports OK
- AND explore enforcement remains a plan-phase skill responsibility

#### Scenario: Guardian ignores unrelated archived changes

- GIVEN other folders under `openspec/changes/archive/` predate this contract and
  lack `proposal.md` or verify evidence
- WHEN the pre-merge guardian runs for the slug under merge
- THEN only that slug is evaluated
- AND no blocker mentions the older archived changes

#### Scenario: Guardian consumes the propagated planning root

- GIVEN a subrepo-context change whose planning root is the proven superrepo
- AND the guardian runs with the propagated planning root context
- WHEN the archive and verify checks evaluate
- THEN it inspects `<super>/openspec/changes/archive/<slug>/`
- AND it MUST NOT consult a subrepo-local change folder in place of the canonical tree

#### Scenario: Guardian without a resolvable planning root fails safe

- GIVEN the planning root context is missing or ambiguous
- WHEN the pre-merge guardian runs
- THEN it fails safe and MUST NOT skip tier-minima or verify blockers
