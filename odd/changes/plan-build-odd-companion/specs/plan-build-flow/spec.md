# Delta for plan-build-flow

## ADDED Requirements

### Requirement: Plan Build execution modes

Plan Build SHALL resolve exactly one execution mode per repository from
`[recipes.plan-build-flow.config] mode` in the project manifest, with the values
`auto`, `odd` and `openspec`, and the default `auto`. `auto` SHALL infer the mode
from the presence of the Gentle AI state directory inside the repository's Git
common directory (`<git-common-dir>/gentle-ai/`): present resolves to `odd`,
absent resolves to `openspec`. An explicit `odd` or `openspec` value SHALL win
over the inference. An absent, unreadable or invalid `mode` value SHALL resolve to
`auto`, and an unresolvable Git common directory SHALL resolve to `openspec`.

The mode SHALL select which planning tree Plan Build reads and writes. It MUST NOT
waive the plan-before-build precondition in any mode: `openspec` mode keeps
requiring the OpenSpec change folder, and `odd` mode requires the paired ODD plan.
`openspec` mode SHALL retain the current OpenSpec-centered behavior unchanged.

#### Scenario: An explicit mode wins over inference

- GIVEN a repository without a `<git-common-dir>/gentle-ai/` directory
- AND the manifest declares `mode = "odd"`
- WHEN Plan Build resolves the mode
- THEN the resolved mode is `odd`

#### Scenario: Auto infers ODD from the Gentle AI state directory

- GIVEN the manifest declares no `mode`, or declares `mode = "auto"`
- AND `<git-common-dir>/gentle-ai/` exists
- WHEN Plan Build resolves the mode
- THEN the resolved mode is `odd`

#### Scenario: Auto without Gentle AI state stays OpenSpec

- GIVEN the manifest declares no `mode`, or declares `mode = "auto"`
- AND `<git-common-dir>/gentle-ai/` does not exist
- WHEN Plan Build resolves the mode
- THEN the resolved mode is `openspec`

#### Scenario: An unusable value still follows the inference

- GIVEN the manifest is missing, unreadable, or carries an unknown `mode` value
- WHEN Plan Build resolves the mode
- THEN the declared value is treated as `auto` and the state-directory inference decides
- AND an unresolvable Git common directory resolves to `openspec`, keeping production writes on `openspec/changes/<slug>/tasks.md`

### Requirement: ODD change workspace

In `odd` mode Plan Build SHALL keep every planning artifact under `odd/`:
canonical specs at `odd/specs/<domain>/spec.md`, the change plan at
`odd/tasks/<change>.md`, and spec deltas at
`odd/changes/<change>/specs/<domain>/spec.md`. A delta SHALL exist only for a
change that adds, modifies or removes an observable product rule, or corrects a
documented rule to match verified behavior; a behavior-preserving change SHALL
carry no delta and no change folder.

Plan Build MUST NOT introduce `odd/config.yml`: the mode lives in the project
manifest. In `odd` mode the `openspec/` tree SHALL NOT be read as the planning,
promotion or parity source, and existing `openspec/` content SHALL be left
byte-identical.

#### Scenario: An observable rule change carries one delta

- GIVEN `odd` mode is active and the change modifies an observable product rule
- WHEN the behavior is verified
- THEN the change carries one delta at
  `odd/changes/<change>/specs/<domain>/spec.md`
- AND that delta is promoted into `odd/specs/<domain>/spec.md` before merge

#### Scenario: A behavior-preserving change carries no delta

- GIVEN `odd` mode is active and the change preserves observable behavior
- WHEN it reaches delivery
- THEN it carries no delta, no change folder, and no promotion step

#### Scenario: A task-only ODD change reports nothing to promote

- GIVEN `odd` mode is active, the change carries no delta, and
  `odd/tasks/<change>.md` exists
- WHEN promotion runs for that change
- THEN it reports nothing to promote and raises no blocker
- AND it creates neither a change folder nor a canonical spec

#### Scenario: No ODD configuration file is invented

- GIVEN `odd` mode is active
- WHEN a change adds an observable rule
- THEN the mode still comes from the project manifest
- AND no `odd/config.yml` is created

### Requirement: ODD change plan pairing

In `odd` mode the plan for the current change SHALL be `odd/tasks/<change>.md` at
the repository root, where `<change>` is the last path segment of the current
branch name: any leading prefix segment (`feat/`, `fix/`, `chore/`, or any other)
SHALL be stripped, so `feat/thing` and `fix/thing` both resolve to `thing`. The
gate SHALL accept only a regular file at that path.

When the plan is missing, is not a regular file, or the current branch cannot be
resolved (detached HEAD), the gate SHALL block the production write and the
refusal SHALL name the expected path, state that `odd` mode is active, and name
both escapes: write the plan, or declare `mode = "openspec"`. `odd/**` SHALL
always be writable in `odd` mode, so writing the plan is never blocked by the
gate that requires it.

#### Scenario: A prefixed branch resolves to its ODD plan

- GIVEN `odd` mode is active and the current branch is `feat/add-thing`
- AND `odd/tasks/add-thing.md` is a regular file
- WHEN a production write is attempted
- THEN the gate exits 0

#### Scenario: A missing ODD plan blocks a production write

- GIVEN `odd` mode is active and no `odd/tasks/<change>.md` exists for the branch
- WHEN a `Write` targets a production file
- THEN the gate exits 2
- AND the refusal names `odd/tasks/<change>.md` as the missing plan
- AND it names `mode = "openspec"` as the alternative flow

#### Scenario: A directory at the plan path does not satisfy the gate

- GIVEN `odd` mode is active and `odd/tasks/<change>.md` exists as a directory
- WHEN a production write is attempted
- THEN the gate exits 2

#### Scenario: Writing the ODD plan is never blocked

- GIVEN `odd` mode is active and no plan exists yet for the branch
- WHEN a `Write` targets `odd/tasks/<change>.md` or any path under `odd/`
- THEN the gate exits 0

#### Scenario: A detached HEAD blocks instead of guessing a plan

- GIVEN `odd` mode is active and HEAD is detached with no branch name
- WHEN a production write is attempted
- THEN the gate exits 2 instead of selecting an arbitrary plan

## MODIFIED Requirements

### Requirement: Pre-tool-use artifact gate hook

The `plan-build-flow` recipe SHALL distribute a `pre-tool-use` runtime hook
(recipe key `hooks/plan-build-gate.sh`, served in process by the
`ai-specs plan-build-gate` verb with no script materialized,
`matcher = Edit|Write|MultiEdit|NotebookEdit`, `blocking = true`) that
machine-enforces the plan-before-build artifact precondition. The hook SHALL
follow the normalized hook contract: read stdin JSON
`{event, tool_name, tool_input, cwd}`, exit `0` to allow, exit `2` to block, and
fail open (exit `0`) on malformed input or an event that cannot be associated with
a repository target. For a valid target in a recognized submodule worktree,
inability to discover the central root MUST NOT grant production-write access; the
hook MUST use the safe nearest-repository behavior instead.

The hook SHALL derive the nearest repository root from the normalized target
before performing active-plan lookup or the nearest-root planning-write
allowance. For a production target in a recognized submodule worktree with no
nearest-root plan, it SHALL lazily resolve the central superproject root for the
active-plan lookup. It SHALL use the central superproject root for that lookup in
an initialized submodule linked worktree in the supported shared layout, and the
current repository root for standalone and other non-submodule worktrees. The
hook MUST normalize target and working paths, including non-existent
destinations, and enforce repository-boundary-aware path comparisons.

The hook SHALL block a matched edit only when BOTH hold: (a) the target path is
under a production directory (default top-level `src`, `lib`, `catalog`,
overridable via `PLAN_BUILD_GATE_PATHS` — scope configuration only), AND (b) the
resolved execution mode finds no active plan. In `openspec` mode that means no
`openspec/changes/*/tasks.md` outside `archive/` beneath the resolved artifact
root; in `odd` mode it means no `odd/tasks/<change>.md` regular file at the
resolved repository root, `<change>` being the last path segment of the current
branch name. It SHALL allow edits under the resolved artifact root's
`openspec/changes/**` path, under `odd/**` in `odd` mode, non-production paths,
and gitignored agent config (`.claude/settings*.json`, `.claude/hooks/*`)
unconditionally, subject to the canonical-path and symlink-boundary rules above.
The nearest-root artifact allowance MUST NOT authorize arbitrary superproject or
outside-repository writes.

The gate SHALL be non-bypassable: it exposes no environment on/off switch, and
`PLAN_BUILD_GATE_MODE` SHALL have no effect. The repository-declared mode selects
which plan tree the precondition reads and never waives it: `openspec` mode still
requires the change folder, and `odd` mode requires the paired ODD plan.

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

#### Scenario: ODD mode requires the paired plan instead of tasks.md

- GIVEN the resolved mode is `odd`
- AND the current branch is `fix/thing` with no `odd/tasks/thing.md`
- AND a `Write` targets a production file
- WHEN the hook runs
- THEN it MUST exit 2 naming `odd/tasks/thing.md`
- AND an existing `openspec/changes/<slug>/tasks.md` MUST NOT satisfy the gate

#### Scenario: Declared OpenSpec mode is unaffected

- GIVEN the manifest declares `mode = "openspec"`
- WHEN a production write is attempted with and without an active change folder
- THEN the outcome is identical to the pre-change behavior

### Requirement: Promotion and archive ownership

Plan Build owns the review-branch tail of a change and MUST run it in this order:
verify evidence → promote canonical delta specs → run the read-only artifact
guardian → archive the change folder. Canonical delta-spec promotion is the
explicit writer (`ai-specs specs promote <slug> --root <root>`): before the first
pre-archive guardian and before the change folder moves, it composes every
applicable ADDED/MODIFIED/REMOVED delta requirement into the matching canonical
spec, preserving unrelated canonical requirements and sections and staying
idempotent on a rerun. The composer, its blockers, its collision rule and its
idempotence are mode-independent; only the roots follow the resolved execution
mode. In `openspec` mode it reads `openspec/changes/<slug>/specs/<domain>/spec.md`
and writes `openspec/specs/<domain>/spec.md`. In `odd` mode it reads
`odd/changes/<slug>/specs/<domain>/spec.md` and writes
`odd/specs/<domain>/spec.md`, and it SHALL NOT read or write `openspec/specs/`.

In `odd` mode the pre-merge guardian SHALL discover the change from the branch pairing
and validate only the paired plan and the promotion parity of its deltas; the archive
step and the tier artifact minima belong to `openspec` mode.

The artifact guardian validates **promotion parity** — every archived
Standard/Full delta requirement is present in its canonical spec and no
destructive or unresolved delta remains. Promotion parity is a read-only check:
the guardian MUST NOT write, rewrite, promote, or repair canonical specs, change
folders, or tracker state. The parity root is the canonical root of the resolved
mode.

Light changes, and ODD task-only work that keeps no spec delta, carry no spec
deltas: promotion and promotion-parity validation apply only to Standard and Full
changes in `openspec` mode and to ODD changes that carry a delta, and neither a
canonical spec nor a promotion blocker is invented for them.

#### Scenario: Ordered tail promotes before the guardian

- GIVEN an authorized Standard or Full change with at least one delta
- WHEN the review-branch tail runs
- THEN verify evidence is produced first
- AND the deltas are composed into canonical specs before the pre-archive guardian
- AND the change folder is archived only after the guardian passes

#### Scenario: Guardian validates parity without writing

- GIVEN an archived Standard or Full change whose delta requirement is absent
  from the matching canonical spec
- WHEN the pre-archive or pre-merge guardian runs
- THEN it blocks with a promotion-parity blocker
- AND the canonical specs of the resolved mode are byte-identical afterward

#### Scenario: Light and ODD task-only work have no promotion step

- GIVEN a Light change, or an ODD change that carries no delta
- WHEN the tail runs
- THEN no canonical promotion is required
- AND no promotion-parity blocker is raised

#### Scenario: ODD mode promotes into the odd canonical tree

- GIVEN the resolved mode is `odd` and the change carries a delta at
  `odd/changes/<slug>/specs/<domain>/spec.md`
- WHEN `ai-specs specs promote <slug> --root <root>` runs
- THEN the requirement is composed into `odd/specs/<domain>/spec.md`
- AND `openspec/specs/<domain>/spec.md` is byte-identical afterward

#### Scenario: ODD pre-merge validates the paired plan and delta parity

- GIVEN the resolved mode is `odd` and the branch pairs with `odd/tasks/<change>.md`
- WHEN the pre-merge guardian runs
- THEN it blocks when that plan is missing or is not a regular file of the repository
- AND it blocks when a delta under `odd/changes/<change>/specs/` is not promoted into `odd/specs/`
- AND it requires neither an archive nor a tier artifact minimum
- AND a change with no delta passes with its plan alone
