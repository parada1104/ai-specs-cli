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

When `--resolved-config` includes a `bindings` map (`capability_id → recipe_id`), the
renderer MUST use it to name which recipe provides the `tracker`, `canonical-store`, and
`vcs-pr-flow` capabilities. The brief MUST reference the BOUND recipe name, not a
hardcoded vendor string.

#### Scenario: Tracker capability named from binding

- GIVEN the bindings map contains `tracker: trello-mcp-workflow`
- WHEN the brief is rendered
- THEN the Trello Tracking section MUST reference `trello-mcp-workflow` (or its resolved label) rather than a hardcoded vendor name

#### Scenario: No tracker binding present

- GIVEN the bindings map does not contain a `tracker` entry
- WHEN the brief is rendered
- THEN the Trello Tracking section MUST be omitted

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
output on both runs.

#### Scenario: Second sync produces no diff

- GIVEN `ai-specs sync` has been run once and `AGENTS.md` exists
- WHEN `ai-specs sync` is run a second time with no manifest changes
- THEN the resulting `AGENTS.md` MUST be byte-identical to the file from the first run

---

### Requirement: --preserve-if-runtime-brief escape hatch preserved

When `agents-render.py` is invoked with `--preserve-if-runtime-brief` and the output
file already contains the marker `<!-- ai-specs:runtime-brief -->`, the renderer MUST
return without modifying the file.

#### Scenario: File with marker left untouched

- GIVEN an existing `AGENTS.md` containing `<!-- ai-specs:runtime-brief -->`
- AND `agents-render.py` is invoked with `--preserve-if-runtime-brief`
- WHEN the renderer runs
- THEN `AGENTS.md` MUST NOT be modified
- AND the renderer MUST exit with code 0

#### Scenario: File without marker is overwritten

- GIVEN an existing `AGENTS.md` that does NOT contain `<!-- ai-specs:runtime-brief -->`
- AND `agents-render.py` is invoked with `--preserve-if-runtime-brief`
- WHEN the renderer runs
- THEN `AGENTS.md` MUST be overwritten with the newly rendered content

---

### Requirement: Subrepos receive enriched output

`sync-agent.sh` invokes `agents-render.py` without `--preserve-if-runtime-brief`.
When the root manifest contains a `[brief]` table and a resolved config is passed,
subrepo `AGENTS.md` files MUST also receive the enriched output.

#### Scenario: Subrepo AGENTS.md contains structured fields

- GIVEN a subrepo wired via `sync-agent.sh`
- AND the root `ai-specs.toml` contains `[brief]` and recipe configs with `board_id` and `test_command`
- WHEN `ai-specs sync` runs for the subrepo
- THEN the subrepo's `AGENTS.md` MUST contain the `board_id` and `test_command` values
