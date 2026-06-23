## MODIFIED Requirements

### Requirement: Recipe fragments merged into brief sections

When enabled recipes declare `[provides.brief]` fragments, `agents-render.py` MUST collect,
deduplicate, and merge those fragments into the corresponding prose sections of the rendered
brief. Recipe fragments are emitted BEFORE manifest `[brief]` additions for each section
(APPEND default).

#### Scenario: Recipe fragments populate an empty manifest section

- **GIVEN** a manifest `[brief]` that does not declare `workflow_rules`
- **AND** the enabled recipe `worktree-flow` declares `[provides.brief].workflow_rules = ["Create a dedicated worktree...", "Do not push to \`{config.integration_branch}\` without a PR."]`
- **AND** the recipe config contains `integration_branch = "development"`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the output MUST contain a `## Workflow Rules` section
- **AND** it MUST contain `- Create a dedicated worktree...` and `- Do not push to \`development\` without a PR.`
- **AND** NO manifest-authored bullets MUST appear in that section (empty manifest [brief].workflow_rules)

#### Scenario: Recipe fragments prepended before manifest additions (APPEND default)

- **GIVEN** the enabled recipe `worktree-flow` contributes one `workflow_rules` fragment `"Recipe rule."`
- **AND** the manifest `[brief].workflow_rules = ["Manifest rule."]`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the rendered `## Workflow Rules` MUST list `- Recipe rule.` BEFORE `- Manifest rule.`

#### Scenario: Recipe without [provides.brief] produces no fragments

- **GIVEN** an enabled recipe whose `recipe.toml` contains no `[provides.brief]` table
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the renderer MUST NOT fail
- **AND** the output MUST be identical to rendering without that recipe's fragments

---

### Requirement: Merge order follows manifest enabled declaration order

Fragment collection MUST iterate recipes in the order they appear in the manifest `enabled`
list (as recorded in `resolved-config.json`). This order is deterministic and controlled by
the project author.

#### Scenario: Two recipes, fragments ordered by enabled declaration

- **GIVEN** the manifest `enabled` list is `["worktree-flow", "tdd-flow"]`
- **AND** `worktree-flow` contributes fragment `"WF rule."` to `workflow_rules`
- **AND** `tdd-flow` contributes fragment `"TDD rule."` to `workflow_rules`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the rendered `## Workflow Rules` MUST list `- WF rule.` BEFORE `- TDD rule.`

#### Scenario: Reordering enabled list changes fragment order

- **GIVEN** the manifest `enabled` list is `["tdd-flow", "worktree-flow"]` (reversed)
- **AND** both recipes contribute fragments as above
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `- TDD rule.` MUST appear BEFORE `- WF rule.`

---

### Requirement: Fragment deduplication

During collection, the renderer MUST deduplicate fragments across all enabled recipes using
two deduplication strategies applied in order:

1. **Key-based deduplication**: if a fragment carries a non-None `key`, and a fragment with
   the same `key` was already collected for that section, the later fragment MUST be
   silently discarded. First occurrence wins.
2. **Exact-string deduplication**: if two fragments have the same `text` (after key-based
   dedup), the later one MUST be silently discarded. First occurrence wins.

Manifest `[brief]` additions participate in exact-string deduplication against the already-
collected recipe fragments but are NOT subject to key-based deduplication.

#### Scenario: Key-based dedup — second occurrence silently discarded

- **GIVEN** recipe A contributes `{key: "trello-sot", text: "Trello is the source of truth."}` to `context_sources`
- **AND** recipe B (listed after A in enabled) also contributes `{key: "trello-sot", text: "Trello: source of truth — updated wording."}` to `context_sources`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** only `"Trello is the source of truth."` MUST appear in `## Context Sources`
- **AND** `"Trello: source of truth — updated wording."` MUST NOT appear

#### Scenario: Exact-string dedup — duplicate text from two recipes discarded

- **GIVEN** recipe A contributes `{key: None, text: "Run tests before committing."}` to `workflow_rules`
- **AND** recipe B contributes `{key: None, text: "Run tests before committing."}` to `workflow_rules`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `"Run tests before committing."` MUST appear exactly ONCE in `## Workflow Rules`

#### Scenario: Exact-string dedup — manifest addition not repeated

- **GIVEN** recipe A contributes fragment `"Create a worktree."` to `workflow_rules`
- **AND** the manifest `[brief].workflow_rules` also contains `"Create a worktree."`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `"Create a worktree."` MUST appear exactly ONCE in `## Workflow Rules`

---

### Requirement: APPEND default and REPLACE opt-in per section

By default (APPEND mode), each section's rendered content consists of: collected recipe
fragments (after deduplication) followed by any manifest `[brief]` additions for that
section.

A project author MAY suppress all recipe fragments for a specific section by setting
`<section>_mode = "replace"` as a sibling key in the manifest `[brief]` table. In REPLACE
mode, only the manifest-authored bullets for that section are rendered; recipe fragments
for that section are silently suppressed.

#### Scenario: REPLACE mode suppresses recipe fragments for one section

- **GIVEN** recipe `worktree-flow` contributes fragments to `workflow_rules`
- **AND** the manifest `[brief]` declares `workflow_rules_mode = "replace"`
- **AND** the manifest `[brief].workflow_rules = ["Only this rule."]`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `## Workflow Rules` MUST contain ONLY `"Only this rule."`
- **AND** ALL recipe-contributed `workflow_rules` fragments MUST NOT appear

#### Scenario: REPLACE mode for one section does not affect other sections

- **GIVEN** the manifest declares `workflow_rules_mode = "replace"`
- **AND** recipe `worktree-flow` also contributes fragments to `runtime_flow`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `## Runtime Flow` MUST still contain the recipe-contributed fragments (APPEND default for `runtime_flow`)

#### Scenario: Default APPEND mode when no _mode key present

- **GIVEN** the manifest `[brief]` has no `workflow_rules_mode` key
- **AND** a recipe contributes fragments to `workflow_rules`
- **AND** the manifest `[brief].workflow_rules = ["Extra rule."]`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** recipe fragments MUST appear BEFORE `"Extra rule."` in `## Workflow Rules`

---

### Requirement: {config.KEY} substitution in fragment text

When rendering a recipe fragment, `agents-render.py` MUST apply `{config.KEY}` substitution
using the recipe's merged config namespace. Substitution is best-effort: missing keys MUST
leave the placeholder verbatim without error. Manifest `[brief]` prose (including additions
to contributable sections) is NEVER substituted.

#### Scenario: {config.KEY} resolved in recipe fragment

- **GIVEN** recipe `worktree-flow` contributes `"Do not push to \`{config.integration_branch}\` without a PR."` to `workflow_rules`
- **AND** the recipe's merged config has `integration_branch = "main"`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the rendered bullet MUST be `"Do not push to \`main\` without a PR."`

#### Scenario: Missing config key leaves placeholder verbatim

- **GIVEN** a fragment text `"Run {config.test_command} first."`
- **AND** `test_command` is NOT in the recipe's merged config
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the rendered text MUST be `"Run {config.test_command} first."` (unchanged)
- **AND** the render MUST NOT raise an exception

#### Scenario: {{ and }} escape to literal braces in fragments

- **GIVEN** a fragment text `"Use {{config.KEY}} to reference config."`
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the rendered text MUST be `"Use {config.KEY} to reference config."`

#### Scenario: Manifest [brief] additions not substituted

- **GIVEN** the manifest `[brief].workflow_rules = ["Check {config.test_command}"]`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the rendered bullet MUST be `"Check {config.test_command}"` (verbatim, not substituted)

---

### Requirement: mcp_descriptions override-fills-gap rule in renderer

When rendering the MCP section, the renderer MUST apply the override-fills-gap rule for
`mcp_descriptions`:
- For each MCP server name, if `[brief].mcp_descriptions.<server>` is present in the
  manifest, that value is used (project override takes precedence).
- If the manifest has no entry for that server, the renderer MUST check collected recipe
  `mcp_descriptions` fragments and use the first non-empty value found (fills gap).
- If neither the manifest nor any recipe supplies a description for a server, the server
  is rendered without a description (existing behavior preserved).

#### Scenario: Project mcp_descriptions override wins

- **GIVEN** recipe A declares `mcp_descriptions.trello = "Recipe default."`
- **AND** the project manifest `[brief].mcp_descriptions.trello = "Project override."`
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the trello server entry MUST use `"Project override."`
- **AND** `"Recipe default."` MUST NOT appear

#### Scenario: Recipe fills mcp_descriptions when project has no entry

- **GIVEN** recipe A declares `mcp_descriptions.trello = "Recipe default."`
- **AND** the manifest `[brief]` does NOT contain `mcp_descriptions.trello`
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the trello server entry MUST use `"Recipe default."`

#### Scenario: No mcp_descriptions anywhere renders server without description

- **GIVEN** an MCP server `vault` is configured in `[mcp.vault]`
- **AND** neither the manifest `[brief].mcp_descriptions.vault` nor any recipe provides a description for `vault`
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the vault server entry MUST still appear in the output (no crash or omission)
- **AND** no description line MUST be rendered for it

---

### Requirement: brief_fragments included in resolved-config.json per recipe

`recipe-materialize.py` MUST include a `brief_fragments` field in the `resolved-config.json`
entry for each recipe that declares `[provides.brief]`. Recipes without `[provides.brief]`
MUST have either an absent `brief_fragments` key or an empty map — not a parse error.

The shape of `brief_fragments` in `resolved-config.json` SHALL be:
`{section_name: [{key: str|null, text: str}, ...], ...}`

#### Scenario: brief_fragments included for recipe with [provides.brief]

- **GIVEN** recipe `worktree-flow` declares `[provides.brief].workflow_rules = ["Create a worktree."]`
- **WHEN** `recipe-materialize.py` builds `resolved-config.json`
- **THEN** the `worktree-flow` entry MUST contain `brief_fragments.workflow_rules`
- **AND** the array MUST contain `{key: null, text: "Create a worktree."}`

#### Scenario: brief_fragments absent for recipe without [provides.brief]

- **GIVEN** an enabled recipe whose `recipe.toml` has no `[provides.brief]`
- **WHEN** `recipe-materialize.py` builds `resolved-config.json`
- **THEN** the recipe's entry in `resolved-config.json` MUST NOT raise an error
- **AND** the renderer MUST handle the missing `brief_fragments` field gracefully

---

### Requirement: Backward compatibility — existing behaviors preserved

The following behaviors from the existing `runtime-brief-rendering` spec MUST be preserved
unchanged after this change:

- The `<!-- ai-specs:runtime-brief -->` marker still suppresses regeneration.
- MCP secrets are still redacted (literal values replaced with `***REDACTED***`).
- `--preserve-if-runtime-brief` escape hatch still works as specified.
- Structured fields from `--resolved-config` are still emitted.
- Output remains idempotent (byte-identical on two consecutive runs with the same manifest).
- Subrepos receive the enriched output (including recipe fragments) when wired via `sync-agent.sh`.

#### Scenario: Manifest with runtime-brief marker not regenerated

- **GIVEN** an `AGENTS.md` containing `<!-- ai-specs:runtime-brief -->`
- **AND** recipes in the manifest now have `[provides.brief]` fragments
- **WHEN** `agents-render.py` is invoked with `--preserve-if-runtime-brief`
- **THEN** `AGENTS.md` MUST NOT be modified
- **AND** the renderer MUST exit with code 0

#### Scenario: Output with fragments is idempotent

- **GIVEN** recipes with `[provides.brief]` fragments are enabled
- **WHEN** `ai-specs sync` is run twice with no manifest changes
- **THEN** the resulting `AGENTS.md` MUST be byte-identical on both runs
