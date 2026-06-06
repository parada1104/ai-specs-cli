# recipe-brief-fragments Specification

## Purpose

Define the `[provides.brief]` authoring contract in `recipe.toml`: the two supported forms,
the contributable sections, `{config.KEY}` substitution with `{{`/`}}` escape, the
append/replace merge semantics, and the `mcp_descriptions` override-fills-gap rule.
This spec also mandates that the authoring contract be documented for users — recipe authors
MUST have reference documentation covering everything they can declare under `[provides.brief]`.

## Non-Goals

- The merge algorithm implementation (deferred to design).
- Fragment inheritance across recipe dependency graphs (explicitly excluded — see below).
- The `resolved-config.json` contract shape (covered by `runtime-brief-rendering`).
- The `intro` and `purpose` sections (project-only; recipes MUST NOT contribute them).

---

## Constraints

### Constraint: No fragment inheritance

Fragments are strictly per-recipe. A recipe MUST NOT inherit or propagate fragments from
recipes it depends on. The merge only spans explicitly enabled recipes in the manifest.
This constraint is intentional: inheritance adds complexity and makes merge results
unpredictable for project authors.

---

## Requirements

### Requirement: [provides.brief] slot in recipe.toml

A `recipe.toml` MAY declare a `[provides.brief]` table. When absent, the recipe contributes
no fragments and MUST NOT cause any change in renderer behavior.

#### Scenario: Recipe without [provides.brief]

- **GIVEN** a `recipe.toml` that does not contain `[provides.brief]`
- **WHEN** the manifest parser processes the recipe
- **THEN** the recipe SHALL be considered valid
- **AND** no brief fragments SHALL be associated with it
- **AND** `agents-render.py` SHALL produce output identical to rendering without that recipe

---

### Requirement: Two supported fragment forms

`[provides.brief]` SHALL support two syntactic forms for each contributable section.
Both forms MUST be accepted and normalized internally to `{key: str|None, text: str}`.

**Form 1 — simple string array** (key is `None`):
```toml
[provides.brief]
workflow_rules = [
  "Create a dedicated worktree for changes that write artifacts or modify code.",
  "Do not push to `{config.integration_branch}` without a PR.",
]
```

**Form 2 — array of inline-tables with explicit key** (enables semantic deduplication):
```toml
[[provides.brief.context_sources]]
key = "trello-source-of-truth"
text = "Trello is the source of truth for work state and dependencies."
```

A single section MAY NOT mix both forms in the same recipe declaration. Mixed-form
declarations SHALL cause a validation error.

#### Scenario: Simple array form parsed and normalized

- **GIVEN** a `recipe.toml` where `[provides.brief].workflow_rules` is a TOML string array
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** each string SHALL be normalized to `{key: None, text: <string>}`
- **AND** the resulting `BriefFragments.workflow_rules` list SHALL preserve declaration order

#### Scenario: Inline-table form parsed and normalized

- **GIVEN** a `recipe.toml` where `[[provides.brief.context_sources]]` entries each declare `key` and `text`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** each entry SHALL be normalized to `{key: <string>, text: <string>}`
- **AND** the `key` field SHALL be preserved for semantic deduplication

#### Scenario: Mixed-form declaration rejected

- **GIVEN** a `recipe.toml` where a section uses both a string array value AND inline-table entries
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the section and stating that mixed forms are not allowed

#### Scenario: Empty section array accepted

- **GIVEN** a `recipe.toml` where `[provides.brief].workflow_rules = []`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the recipe SHALL contribute zero fragments for that section

#### Scenario: Inline-table missing text field

- **GIVEN** a `[[provides.brief.context_sources]]` entry that declares `key` but omits `text`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing `text` field

#### Scenario: Inline-table missing key field

- **GIVEN** a `[[provides.brief.context_sources]]` entry that declares `text` but omits `key`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing `key` field

---

### Requirement: Contributable sections

A recipe MAY contribute fragments to the following sections only:
`runtime_flow`, `context_sources`, `conflict_policy`, `workflow_rules`,
`useful_commands`, `mcp_descriptions`.

A recipe MUST NOT declare fragments for `intro` or `purpose`. Those sections are
project-only and belong exclusively in the manifest `[brief]` table.

#### Scenario: Valid contributable section accepted

- **GIVEN** a `recipe.toml` with `[provides.brief].workflow_rules = ["..."]`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass

#### Scenario: intro declared in [provides.brief] rejected

- **GIVEN** a `recipe.toml` with `[provides.brief].intro = ["..."]`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error stating that `intro` is a project-only section and MUST NOT be contributed by recipes

#### Scenario: purpose declared in [provides.brief] rejected

- **GIVEN** a `recipe.toml` with `[provides.brief].purpose = ["..."]`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error stating that `purpose` is a project-only section and MUST NOT be contributed by recipes

#### Scenario: Unknown section name rejected

- **GIVEN** a `recipe.toml` with `[provides.brief].custom_section = ["..."]`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error listing the set of valid contributable section names

---

### Requirement: {config.KEY} substitution in fragments

Fragment text strings MUST support `{config.KEY}` placeholder substitution using values
from the recipe's merged config (the same config namespace used elsewhere in the recipe).
Substitution is best-effort: a missing key MUST leave the placeholder verbatim and MUST
NOT cause the render to fail or raise an error.

The substitution namespace is `config.KEY` (not bare `KEY`): `{config.integration_branch}`
resolves to the value of `integration_branch` from the merged recipe config.

#### Scenario: {config.KEY} substituted with resolved value

- **GIVEN** a `recipe.toml` fragment text `"Do not push to \`{config.integration_branch}\` without a PR."`
- **AND** the recipe's merged config contains `integration_branch = "development"`
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the rendered text MUST be `"Do not push to \`development\` without a PR."`

#### Scenario: Missing config key leaves placeholder verbatim

- **GIVEN** a fragment text `"Push to {config.base_branch} via PR."`
- **AND** the recipe's merged config does NOT contain `base_branch`
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the rendered text MUST be `"Push to {config.base_branch} via PR."` (placeholder preserved)
- **AND** the render MUST NOT fail or raise an exception

#### Scenario: Bare key reference (without config. prefix) is not substituted

- **GIVEN** a fragment text `"Use {integration_branch} branch."`
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the text `{integration_branch}` MUST NOT be substituted
- **AND** the rendered text MUST be `"Use {integration_branch} branch."` (verbatim)

---

### Requirement: Brace escape rule

In fragment text strings, `{{` MUST render as a literal `{` and `}}` MUST render as a
literal `}`. This escape rule applies only in recipe `[provides.brief]` fragment strings.
It MUST NOT apply to manifest `[brief]` prose, `intro`, or `purpose`.

#### Scenario: {{ and }} render as literal braces

- **GIVEN** a fragment text `"Use {{config.KEY}} syntax in your templates."`
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the rendered text MUST be `"Use {config.KEY} syntax in your templates."`

#### Scenario: Mixed escape and substitution in the same string

- **GIVEN** a fragment text `"Run \`{config.test_command}\` (not {{skip}})."`
- **AND** `config.test_command = "./tests/run.sh"`
- **WHEN** `agents-render.py` renders the fragment
- **THEN** the rendered text MUST be `"Run \`./tests/run.sh\` (not {skip})."`

#### Scenario: Manifest [brief] prose is never substituted

- **GIVEN** a manifest `[brief].workflow_rules` containing `"Run {config.test_command}"`
- **WHEN** `agents-render.py` renders the brief
- **THEN** the text `{config.test_command}` MUST NOT be substituted
- **AND** the rendered text MUST be `"Run {config.test_command}"` (verbatim)

---

### Requirement: mcp_descriptions override-fills-gap rule

A recipe MAY declare default MCP descriptions for the MCP servers it owns via
`[provides.brief].mcp_descriptions`. The project manifest `[brief].mcp_descriptions`
entries OVERRIDE the recipe defaults on a per-server basis. If the project declares no
entry for a given server, the recipe's declared value fills the gap.

The precedence rule is: project `[brief].mcp_descriptions.<server>` wins if present;
recipe `[provides.brief].mcp_descriptions.<server>` is used only when the project has
no entry for that server.

#### Scenario: Recipe fills mcp_descriptions gap

- **GIVEN** a recipe that declares `[provides.brief].mcp_descriptions.trello = "Project tracking via Trello."`
- **AND** the project manifest `[brief]` does NOT contain an `mcp_descriptions.trello` entry
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the rendered MCP section for `trello` MUST use `"Project tracking via Trello."`

#### Scenario: Project overrides recipe mcp_descriptions

- **GIVEN** a recipe that declares `[provides.brief].mcp_descriptions.trello = "Recipe default."`
- **AND** the project manifest `[brief].mcp_descriptions.trello = "Project override."`
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the rendered MCP section for `trello` MUST use `"Project override."`
- **AND** the recipe value `"Recipe default."` MUST NOT appear in the output

#### Scenario: Multiple recipes, non-overlapping mcp_descriptions

- **GIVEN** recipe A declares `mcp_descriptions.trello = "A's description."`
- **AND** recipe B declares `mcp_descriptions.github = "B's description."`
- **AND** the project manifest `[brief]` contains no `mcp_descriptions` entries
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the trello server entry MUST use `"A's description."`
- **AND** the github server entry MUST use `"B's description."`

---

### Requirement: Recipe brief-fragment authoring MUST be documented for users

The repository MUST include authoritative user-facing documentation for recipe authors
covering the `[provides.brief]` authoring contract. This documentation MUST be written
and kept in sync with the implementation.

The documentation MUST cover:
1. Both fragment forms (simple string array and inline-table with `key`/`text`) with
   TOML examples for each.
2. All contributable sections (`runtime_flow`, `context_sources`, `conflict_policy`,
   `workflow_rules`, `useful_commands`, `mcp_descriptions`) and the explicit exclusion
   of `intro` and `purpose`.
3. The `{config.KEY}` substitution capability: namespace syntax, resolution source,
   and best-effort (missing key → verbatim placeholder) behavior.
4. The `{{`/`}}` brace escape rule with a concrete example.
5. The append/replace merge semantics: how recipe fragments combine with manifest
   `[brief]` additions (append default) and how `<section>_mode = "replace"` in the
   project manifest suppresses recipe contributions for that section.
6. The `mcp_descriptions` override-fills-gap rule with a concrete example showing
   project override vs. recipe default.

#### Scenario: Documentation file exists and covers both fragment forms

- **GIVEN** the repository has been built with this change applied
- **WHEN** a user reads the recipe authoring documentation
- **THEN** they SHALL find TOML examples demonstrating the simple string array form
- **AND** they SHALL find TOML examples demonstrating the inline-table `{key, text}` form

#### Scenario: Documentation covers substitution and escape

- **GIVEN** the recipe authoring documentation
- **WHEN** a recipe author reads the substitution section
- **THEN** they SHALL find a concrete example of `{config.KEY}` resolving to a value
- **AND** they SHALL find the `{{`/`}}` escape rule with a worked example
- **AND** they SHALL understand that missing keys leave placeholders verbatim (no crash)

#### Scenario: Documentation covers append/replace semantics

- **GIVEN** the recipe authoring documentation
- **WHEN** a recipe author reads the merge semantics section
- **THEN** they SHALL find an explanation of APPEND default (recipe fragments listed first,
  manifest additions appended after)
- **AND** they SHALL find an explanation of how `<section>_mode = "replace"` in the
  project manifest suppresses all recipe fragments for that section
- **AND** they SHALL find the `mcp_descriptions` override-fills-gap rule documented

#### Scenario: Documentation covers contributable sections list

- **GIVEN** the recipe authoring documentation
- **WHEN** a recipe author reads the contributable sections
- **THEN** they SHALL find the exact list of valid section names
- **AND** they SHALL find an explicit callout that `intro` and `purpose` are project-only
  and MUST NOT be declared in `[provides.brief]`
