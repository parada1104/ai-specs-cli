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
- **Standard** — spec → tasks (explore/proposal/design optional)
- **Light** — tasks only

The chosen depth MUST be recorded in `tasks.md`. Direct implementation verbs on
a request with no existing change folder MUST NOT skip planning.

#### Scenario: Full depth for ambiguous scope

- GIVEN a request for a new cross-cutting capability with unclear boundaries
- WHEN planning starts
- THEN the full planning chain runs
- AND tier minimum artifacts exist before build

#### Scenario: Light depth for scoped fix

- GIVEN a one-file bugfix with an explicit file and expected edit
- WHEN planning starts
- THEN only `tasks.md` is required
- AND no production code is modified during planning

#### Scenario: Direct implement still plans first

- GIVEN the user says "implement X" with no `openspec/changes/<slug>/` folder
- WHEN the skill evaluates the request
- THEN it classifies depth and runs the plan phase before build
- AND stops for authorization unless the tier is trivially light and inline build is allowed

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

- GIVEN a standard-tier change with `tasks.md` and spec deltas under `specs/`
- WHEN the artifact gate is evaluated before PR creation
- THEN PR creation may proceed

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

### Requirement: Coexistence with classic SDD

Enabling `plan-build-flow` MUST NOT modify, remove, or rename any existing
classic SDD command, skill, or recipe outside this recipe's own surface.

#### Scenario: Classic flow unaffected

- GIVEN a project with classic SDD commands already synced
- WHEN `plan-build-flow` is enabled and synced
- THEN all pre-existing non-plan-build-flow commands and skills remain unchanged


### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts or a still-active (non-archived) change
folder is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` (defaulting
`AI_SPECS_HOME` to `$HOME/.ai-specs` when unset). Sync MUST NOT materialize a
per-project copy under `ai-specs/bin/`.

#### Scenario: Merge blocked when change folder still active

- GIVEN `openspec/changes/<slug>/` still exists (not archived)
- WHEN an agent attempts to merge the PR/MR
- THEN the skill stops with a plain-language blocker requiring archive-tail first

#### Scenario: Guardian path is CLI-home

- GIVEN `plan-build-flow` (or a VCS merge skill) is enabled
- WHEN an agent runs the pre-merge guardian
- THEN it uses `${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py`
- AND the recipe does not target `ai-specs/bin/premerge_guardian.py`

### Requirement: Pre-tool-use artifact gate hook

The `plan-build-flow` recipe SHALL distribute a `pre-tool-use` runtime hook
(`hooks/plan-build-gate.sh`, `matcher = Edit|Write|MultiEdit|NotebookEdit`,
`blocking = true`) that machine-enforces the plan-before-build artifact
precondition. The hook SHALL follow the normalized hook contract: read stdin
JSON `{event, tool_name, tool_input, cwd}`, exit `0` to allow, exit `2` to
block, and fail open (exit `0`) on any parse or lookup error.

The hook SHALL block a matched edit only when BOTH hold: (a) the target path is
under a production directory (default top-level `src`, `lib`, `catalog`,
overridable via `PLAN_BUILD_GATE_PATHS` — scope configuration only), AND (b) no
active change folder exists (no `openspec/changes/*/tasks.md` outside
`archive/`). It SHALL allow edits under `openspec/changes/**`, non-production
paths, and gitignored agent config (`.claude/settings*.json`, `.claude/hooks/*`)
unconditionally. The gate SHALL be non-bypassable: it exposes no on/off/ask
mode, so the only way past it is to write the plan the gate requires.

Because the hook pipeline exposes no pre-file-write event for `cursor`, this
hook enforces on `claude`, `opencode`, `pi`, and `omp` only; `cursor` retains
the advisory skill + workflow-rules layer.

#### Scenario: Production edit blocked without a change folder

- GIVEN no `openspec/changes/*/tasks.md` exists outside `archive/`
- AND a `Write` targets a file under a production directory (e.g. `src/app.py`)
- WHEN the hook receives the normalized event
- THEN it MUST exit 2 and surface a plain-language reason to the agent

#### Scenario: Production edit allowed once a plan exists

- GIVEN `openspec/changes/<slug>/tasks.md` exists outside `archive/`
- AND a `Write` targets a file under a production directory
- WHEN the hook receives the event
- THEN it MUST exit 0

#### Scenario: Writing the plan is never blocked

- GIVEN no change folder exists yet
- AND a `Write` targets `openspec/changes/<slug>/tasks.md`
- WHEN the hook receives the event
- THEN it MUST exit 0

#### Scenario: Fail-open on malformed input

- GIVEN malformed JSON or a missing `file_path` on stdin
- WHEN the hook runs
- THEN it MUST exit 0 (a buggy guard must never wedge all editing)

#### Scenario: No mode bypass

- GIVEN a production `Write` with no active change folder
- AND any `PLAN_BUILD_GATE_MODE` value is set in the environment
- WHEN the hook runs
- THEN it MUST still exit 2 (the mode env has no effect; the gate has no off switch)

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
