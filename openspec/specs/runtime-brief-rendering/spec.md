# runtime-brief-rendering Specification

## Purpose

Define how `agents-render.py` composes a rich `AGENTS.md` runtime brief from `[project]`,
`[agents]`, a `[brief]` table, and structured fields supplied via `--resolved-config` JSON.
This spec covers the new capability introduced by Option C-2.

## Non-Goals

- Per-recipe `[brief]` fragments (deferred to a future change).
- Removing the `--preserve-if-runtime-brief` escape hatch (it remains permanent).
- Auto-invoke table in AGENTS.md (handled by skill-sync / SKILL.md frontmatter).

---
## Requirements
### Requirement: Brief sections rendered from [brief] table

When `[brief]` is present in the manifest, `agents-render.py` MUST emit the following
prose sections in this fixed order: intro (blockquote), purpose, runtime_flow,
context_sources, conflict_policy, workflow_rules. Sections whose key is absent from
`[brief]` MUST be silently omitted. String-array values MUST be rendered as bullet lists.

#### Scenario: All prose sections present

- GIVEN an `ai-specs.toml` with a `[brief]` table containing `intro`, `runtime_flow`, `context_sources`, `conflict_policy`, and `workflow_rules` keys
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain each section heading and its content
- AND the sections MUST appear in the fixed order defined above

#### Scenario: Partial [brief] table

- GIVEN an `ai-specs.toml` with a `[brief]` table that contains only `workflow_rules`
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain the `## Workflow Rules` section
- AND sections for absent keys (intro, runtime_flow, etc.) MUST NOT appear in the output

#### Scenario: String-array value rendered as bullets

- GIVEN `[brief].workflow_rules` is a TOML array of strings `["Rule A", "Rule B"]`
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain `- Rule A` and `- Rule B` as bullet items

---

### Requirement: Structured fields resolved from --resolved-config

`agents-render.py` MUST accept a `--resolved-config <path>` argument whose value is a
JSON file. When supplied, the renderer MUST extract and emit the following structured
fields in the brief: `project_name` (`[project].name`), `enabled_runtimes`
(`[agents].enabled`), `integration_branch`, `base_branch`, `provider` (from vcs-pr-flow
config), `test_command` (from tdd-flow config), `board_id` (from tracker config),
`vault_scope` (from canonical-store config), and the list of configured MCP server names.

#### Scenario: board_id needle present

- GIVEN an `ai-specs.toml` with a trello-mcp-workflow recipe whose `board_id` is `abc123`
- AND `sync.sh` pre-computes and passes `--resolved-config` with that value
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain `abc123`

#### Scenario: integration_branch needle present

- GIVEN the resolved config contains `integration_branch: development`
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain the string `development`

#### Scenario: test_command needle present

- GIVEN the resolved config contains `test_command: ./tests/run.sh`
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain `./tests/run.sh`

#### Scenario: vault_scope needle present

- GIVEN the resolved config contains `vault_scope: nnodes/proyectos/ai-specs`
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST contain `nnodes/proyectos/ai-specs`

#### Scenario: --resolved-config absent

- GIVEN `agents-render.py` is invoked without `--resolved-config`
- WHEN the renderer runs
- THEN it MUST render successfully using only `[project]`, `[agents]`, `[brief]`, and `[mcp.*]`
- AND structured-field sections that require resolved config MUST be omitted

---

### Requirement: Capability-binding lookup names the provider recipe

The renderer MUST use the `bindings` map (`capability_id → recipe_id`) from `--resolved-config`
to name which recipe provides the `tracker`, `canonical-store`, and `vcs-pr-flow` capabilities.
The brief MUST reference the BOUND recipe name, not a hardcoded vendor string.

#### Scenario: Tracker capability named from binding

- GIVEN the bindings map contains `tracker: trello-mcp-workflow`
- WHEN the brief is rendered
- THEN the Trello Tracking section MUST reference `trello-mcp-workflow` (or its resolved label) rather than a hardcoded vendor name

#### Scenario: No tracker binding present

- GIVEN the bindings map does not contain a `tracker` entry
- WHEN the brief is rendered
- THEN the Trello Tracking section MUST be omitted

---

### Requirement: VCS provider bullet uses binding recipe id

The Runtime Flow VCS supplemental bullet MUST be emitted from the bound `vcs-pr-flow`
recipe id using a fixed id→label map. It MUST NOT read `provider` from
`recipes[<bound-id>].config`.

#### Scenario: Recipe id drives provider label
- GIVEN `resolved-config` has `bindings.vcs-pr-flow = "git-pr-flow"`
- AND `recipes.git-pr-flow.config` contains only `base_branch`
- WHEN `_section_runtime_flow` renders
- THEN the VCS bullet names GitHub and the `gh` CLI
- AND does not require a `provider` config key

#### Scenario: Base branch still configurable
- GIVEN `bindings.vcs-pr-flow = "gitlab-mr-flow"`
- AND `recipes.gitlab-mr-flow.config.base_branch = "main"`
- WHEN `_section_runtime_flow` renders
- THEN the VCS bullet includes `base branch: \`main\``

---

### Requirement: MCP env secrets redacted

The renderer MUST NOT emit literal secret values from `[mcp.*].env`. Any value that is
a plain string (not a `$VAR` or `${VAR}` reference) MUST be replaced with `***REDACTED***`.
This requirement is inherited from the existing mcp-env-rendering behavior and MUST be
preserved in the enriched renderer.

#### Scenario: Literal secret replaced

- GIVEN an `ai-specs.toml` with `[mcp.vault]` whose `env` contains `{ TOKEN = 'supersecret' }`
- WHEN `agents-render.py` renders the manifest
- THEN the output MUST NOT contain `supersecret`
- AND MUST contain `***REDACTED***` in its place

---

### Requirement: Idempotent output

Running `ai-specs sync` twice with the same manifest MUST produce byte-identical `AGENTS.md`
output on both runs when `[brief].render` is not `false`. When `[brief].render = false`,
two consecutive syncs MUST also leave `AGENTS.md` byte-identical (trivially satisfied by
the skip contract).

#### Scenario: Second sync produces no diff

- GIVEN `ai-specs sync` has been run once and `AGENTS.md` exists
- AND `[brief].render` is not `false`
- WHEN `ai-specs sync` is run a second time with no manifest changes
- THEN the resulting `AGENTS.md` MUST be byte-identical to the file from the first run

#### Scenario: Two syncs with render disabled produce no diff

- **GIVEN** `AGENTS.md` exists with manual content
- **AND** `[brief] render = false`
- **WHEN** `ai-specs sync` is run twice with no manifest changes
- **THEN** `AGENTS.md` MUST be byte-identical after both runs

---

### Requirement: --preserve-if-runtime-brief escape hatch preserved

`agents-render.py` MUST return without modifying the output file when invoked with
`--preserve-if-runtime-brief` and the file already contains the marker
`<!-- ai-specs:runtime-brief -->`.

When `[brief].render = false`, callers MUST NOT invoke `agents-render.py` at all;
the marker check is irrelevant in that case.

#### Scenario: File with marker left untouched

- GIVEN an existing `AGENTS.md` containing `<!-- ai-specs:runtime-brief -->`
- AND `[brief].render` is not `false`
- AND `agents-render.py` is invoked with `--preserve-if-runtime-brief`
- WHEN the renderer runs
- THEN `AGENTS.md` MUST NOT be modified
- AND the renderer MUST exit with code 0

#### Scenario: File without marker is overwritten

- GIVEN an existing `AGENTS.md` that does NOT contain `<!-- ai-specs:runtime-brief -->`
- AND `[brief].render` is not `false`
- AND `agents-render.py` is invoked with `--preserve-if-runtime-brief`
- WHEN the renderer runs
- THEN `AGENTS.md` MUST be overwritten with the newly rendered content

---

### Requirement: Subrepos receive enriched output

Subrepo `AGENTS.md` files MUST receive the enriched output when the root manifest
has brief rendering enabled (`[brief].render` is not `false`), contains a
`[brief]` table, and a resolved config is passed. When `[brief].render = false`
on the root manifest, `sync-agent.sh` MUST NOT invoke `agents-render.py` for
subrepo targets but MUST still mirror skills, commands, and gitignore.

`sync-agent.sh` invokes `agents-render.py` without `--preserve-if-runtime-brief`
for subrepos when rendering is enabled.

#### Scenario: Subrepo AGENTS.md contains structured fields

- GIVEN a subrepo wired via `sync-agent.sh`
- AND the root `ai-specs.toml` contains `[brief]` and recipe configs with `board_id` and `test_command`
- AND `[brief].render` is not `false`
- WHEN `ai-specs sync` runs for the subrepo
- THEN the subrepo's `AGENTS.md` MUST contain the `board_id` and `test_command` values

#### Scenario: Subrepo render skipped when root render disabled

- **GIVEN** a subrepo wired via `sync-agent.sh`
- **AND** the root manifest declares `[brief] render = false`
- **AND** the subrepo has an existing `AGENTS.md` with known content
- **WHEN** `ai-specs sync` runs for the subrepo
- **THEN** `agents-render.py` MUST NOT be invoked for the subrepo target
- **AND** the subrepo's `AGENTS.md` MUST be byte-identical to its pre-sync content
- **AND** skills and commands MUST still be mirrored to the subrepo

#### Scenario: Subrepo missing AGENTS.md with render disabled fails clearly

- **GIVEN** a subrepo target with no `AGENTS.md`
- **AND** the root manifest declares `[brief] render = false`
- **WHEN** `sync-agent.sh` runs for the subrepo target
- **THEN** sync MUST fail with an explicit error indicating `AGENTS.md` is missing
- **AND** the error MUST guide the user to create the file manually or enable rendering

### Requirement: Default template pre-enables session-context recipe

The `ai-specs.toml` template (`templates/ai-specs.toml.tmpl`) MUST ship with
`[recipes.session-context]` enabled by default (`enabled = true`). This ensures
`recipe-materialize.py` produces a non-empty `enabled` list on a fresh project
without any user edits.

#### Scenario: Fresh template parse yields session-context enabled

- GIVEN a freshly written `ai-specs.toml` produced from the default template
- WHEN `recipe-materialize.py` builds `resolved-config.json`
- THEN the `enabled` list in the resolved config MUST contain `"session-context"`
- AND `session-context.brief_fragments` MUST be present in the resolved output

#### Scenario: Template default does not include project-specific values

- GIVEN the default template is applied with only a placeholder `PROJECT_NAME`
- WHEN `recipe-materialize.py` resolves the config
- THEN the resolved output MUST NOT contain board IDs, vault scopes, tracker URLs,
  or any other project-specific token beyond `PROJECT_NAME`

---

### Requirement: init renders a non-empty AGENTS.md immediately

After writing `ai-specs.toml`, `ai-specs init` MUST run `recipe-materialize.py`
followed by `agents-render.py --preserve-if-runtime-brief` to produce a
semantically complete `AGENTS.md` **when `[brief].render` is not `false`**.

When `[brief].render = false`, `ai-specs init` MUST NOT invoke `agents-render.py`.
If `AGENTS.md` does not exist, init MUST write the one-line placeholder
`# AGENTS.md - Runtime context` and print a stderr message indicating render
was disabled. If `AGENTS.md` already exists, init MUST leave it unchanged.

The bare one-line placeholder MUST NOT be the final init output when render is
enabled and the render succeeds.

The rendered brief (when render is enabled) MUST include at least the baseline
behavioral sections contributed by `session-context`: one `## Workflow Rules`
bullet and two `## Conflict Policy` bullets.

#### Scenario: Fresh init produces non-empty behavioral brief

- GIVEN a new project directory with no existing `ai-specs.toml` or `AGENTS.md`
- AND the manifest does NOT set `[brief].render = false`
- WHEN `ai-specs init` completes successfully
- THEN `AGENTS.md` MUST contain a `## Workflow Rules` section with at least one bullet
- AND MUST contain a `## Conflict Policy` section with at least two bullets
- AND those bullets MUST match the fragments declared in
  `catalog/recipes/session-context/recipe.toml [provides.brief]`

#### Scenario: Init with render disabled creates placeholder only

- **GIVEN** a new project directory with no existing `AGENTS.md`
- **AND** the manifest declares `[brief] render = false`
- **WHEN** `ai-specs init` completes
- **THEN** `agents-render.py` MUST NOT be invoked
- **AND** `AGENTS.md` MUST exist with content exactly `# AGENTS.md - Runtime context`
- **AND** stderr MUST contain a message indicating brief rendering was disabled
- **AND** `ai-specs init` MUST exit with code 0

#### Scenario: Init with render disabled preserves existing AGENTS.md

- **GIVEN** an existing `AGENTS.md` with custom manual content
- **AND** the manifest declares `[brief] render = false`
- **WHEN** `ai-specs init` runs (including with `--force` for other artifacts)
- **THEN** `AGENTS.md` MUST be byte-identical to its pre-init content
- **AND** `agents-render.py` MUST NOT be invoked

#### Scenario: Init render failure falls back to placeholder

- GIVEN a new project directory
- AND `[brief].render` is not `false`
- AND `agents-render.py` or `recipe-materialize.py` exits non-zero (e.g. Python
  not found, offline dependency)
- WHEN `ai-specs init` runs
- THEN `AGENTS.md` MUST still be created (with at minimum a one-line placeholder)
- AND `ai-specs init` MUST exit with code 0 (render failure is non-fatal)
- AND an error message MUST be printed to stderr indicating the render was skipped

#### Scenario: Baseline brief contains no project-specific tokens

- GIVEN a freshly initialized project with only the default template
- AND `[brief].render` is not `false`
- WHEN `ai-specs init` completes and `AGENTS.md` is inspected
- THEN `AGENTS.md` MUST NOT contain board IDs, vault paths, tracker identifiers,
  or any value that requires a project-specific binding to resolve
- AND all `{config.KEY}` placeholders for unbound keys MUST be absent or verbatim

---

### Requirement: init→sync idempotency

Running `ai-specs sync` after `ai-specs init` on an unmodified manifest MUST
produce a byte-identical `AGENTS.md` when rendering is enabled. When
`[brief].render = false`, subsequent sync MUST also leave `AGENTS.md` unchanged.
The `--preserve-if-runtime-brief` marker contract MUST be honored at both
init-time and sync-time when rendering is enabled.

#### Scenario: Second render after init is byte-stable

- GIVEN `ai-specs init` has completed and `AGENTS.md` exists
- AND the manifest has not been modified
- AND `[brief].render` is not `false`
- WHEN `ai-specs sync` is run
- THEN the resulting `AGENTS.md` MUST be byte-identical to the file written by init

#### Scenario: Sync with render disabled leaves AGENTS.md unchanged

- **GIVEN** `AGENTS.md` exists with known manual content
- **AND** the manifest declares `[brief] render = false`
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST be byte-identical to its pre-sync content
- **AND** `agents-render.py` MUST NOT be invoked
- **AND** `ai-specs sync` MUST exit with code 0

#### Scenario: User-authored marker prevents re-render

- GIVEN `AGENTS.md` contains the line `<!-- ai-specs:runtime-brief -->`
  (user has opted out of managed rendering)
- AND `[brief].render` is not `false`
- WHEN `ai-specs init` is run again or `ai-specs sync` runs
- THEN `AGENTS.md` MUST NOT be modified
- AND both commands MUST exit with code 0

---

### Requirement: Fragment deduplication on additional recipe enable

The renderer MUST NOT produce duplicate bullets when additional recipes contribute
fragments to the same sections as `session-context`. Key-based and exact-string
deduplication (from the existing `Fragment deduplication` requirement) applies
across `session-context` and all additionally enabled recipes.

#### Scenario: No duplication when second recipe provides same key

- GIVEN `session-context` is enabled (contributing `conflict-policy-source-authority`)
- AND the user enables a second recipe contributing a fragment with the same
  `key = "conflict-policy-source-authority"`
- WHEN `agents-render.py` renders the manifest
- THEN the `## Conflict Policy` section MUST contain that bullet exactly ONCE
- AND the second recipe's version MUST be silently discarded (first-wins)

---

### Requirement: [brief].render manifest opt-out disables AGENTS.md writes

The manifest MAY declare `[brief].render` as a boolean. When omitted, the default
SHALL be `true` (rendering enabled — current behavior). When explicitly `false`,
`ai-specs sync`, `ai-specs init`, and subrepo fan-out via `sync-agent.sh` MUST
skip `agents-render.py` entirely. No merge of manifest `[brief]` prose or recipe
`[provides.brief]` fragments SHALL be written to `AGENTS.md`.

Skills, MCP presets, hooks, and `recipe-materialize.py` MUST continue to run
normally when `render = false`.

#### Scenario: Sync skips render when render is false

- **GIVEN** a manifest declaring `[brief] render = false`
- **AND** an existing `AGENTS.md` with known manual content
- **AND** enabled recipes declare `[provides.brief]` fragments
- **WHEN** `ai-specs sync` runs
- **THEN** `agents-render.py` MUST NOT be invoked
- **AND** `AGENTS.md` MUST be byte-identical to its pre-sync content
- **AND** stdout MUST indicate that AGENTS.md rendering was skipped

#### Scenario: Default render true preserves current behavior

- **GIVEN** a manifest with a `[brief]` table that does NOT declare `render`
- **WHEN** `ai-specs sync` runs
- **THEN** `agents-render.py` MUST be invoked as today
- **AND** the resulting `AGENTS.md` MUST include merged recipe fragments and manifest prose

#### Scenario: Render false does not block other sync artifacts

- **GIVEN** a manifest declaring `[brief] render = false`
- **WHEN** `ai-specs sync` runs
- **THEN** skills, commands, MCP configs, hooks, and gitignore MUST still be updated
- **AND** only the `AGENTS.md` write step MUST be skipped

---

### Requirement: Precedence of render flag over marker and per-section modes

When `[brief].render = false`, brief rendering is fully disabled regardless of:
- the presence of `<!-- ai-specs:runtime-brief -->` in `AGENTS.md`
- manifest `[brief]` prose entries
- enabled recipe `[provides.brief]` fragments
- per-section `<section>_mode` keys

When `[brief].render` is not `false`, the existing precedence applies: marker
(with `--preserve-if-runtime-brief`) suppresses overwrite; otherwise normal
render with append/replace per section.

#### Scenario: Render false skips even without marker

- **GIVEN** `AGENTS.md` does NOT contain `<!-- ai-specs:runtime-brief -->`
- **AND** the manifest declares `[brief] render = false`
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST NOT be modified
- **AND** `agents-render.py` MUST NOT be invoked

#### Scenario: Render false with marker present is redundant but valid

- **GIVEN** `AGENTS.md` contains `<!-- ai-specs:runtime-brief -->`
- **AND** the manifest declares `[brief] render = false`
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST NOT be modified
- **AND** sync MUST exit with code 0

#### Scenario: Render true with marker still preserves file

- **GIVEN** `AGENTS.md` contains `<!-- ai-specs:runtime-brief -->`
- **AND** the manifest does NOT declare `[brief] render = false`
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST NOT be modified (marker contract unchanged)

---

### Requirement: Observability when render is disabled

When brief rendering is skipped due to `[brief].render = false`, the sync or init
command MUST print a user-visible message on stdout under the agents-render step
header. Init MUST additionally print guidance on stderr when it writes the
one-line placeholder.

#### Scenario: Sync stdout names skip reason

- **GIVEN** a manifest declaring `[brief] render = false`
- **WHEN** `ai-specs sync` runs
- **THEN** stdout MUST contain a line indicating AGENTS.md rendering was skipped
- **AND** the message MUST reference `brief.render = false` or equivalent clear wording

#### Scenario: Init stderr guides manual brief authoring

- **GIVEN** a new project with `[brief] render = false` and no existing `AGENTS.md`
- **WHEN** `ai-specs init` completes
- **THEN** stderr MUST indicate the user should replace the placeholder with a manual brief


