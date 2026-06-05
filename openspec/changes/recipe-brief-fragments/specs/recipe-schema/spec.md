## MODIFIED Requirements

### Requirement: [provides.brief] optional slot in recipe.toml

The `[provides]` table in `recipe.toml` SHALL support an optional `[provides.brief]` table.
When present, it declares per-section brief fragments that `agents-render.py` merges into
the runtime brief. When absent, the recipe contributes no fragments and all existing parse,
add, and sync behavior is unchanged.

#### Scenario: [provides.brief] absent — recipe parses and syncs unchanged

- **GIVEN** a valid `recipe.toml` with no `[provides.brief]` table
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** the parsed `Recipe` object SHALL have `brief_fragments = None` (or equivalent empty value)
- **AND** sync SHALL process the recipe without error

#### Scenario: [provides.brief] present with simple array form

- **GIVEN** a `recipe.toml` containing:
  ```toml
  [provides.brief]
  workflow_rules = [
    "Create a dedicated worktree for changes that modify code.",
    "Do not push to \`{config.integration_branch}\` without a PR.",
  ]
  ```
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** the `BriefFragments.workflow_rules` list SHALL contain two `BriefFragment` entries
- **AND** each entry SHALL have `key = None` and `text` equal to the corresponding string

#### Scenario: [provides.brief] present with inline-table form

- **GIVEN** a `recipe.toml` containing:
  ```toml
  [[provides.brief.context_sources]]
  key = "trello-source-of-truth"
  text = "Trello is the source of truth for work state and dependencies."
  ```
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** `BriefFragments.context_sources` SHALL contain one entry with `key = "trello-source-of-truth"` and the specified `text`

#### Scenario: [provides.brief] with both contributable sections

- **GIVEN** a `recipe.toml` with `[provides.brief]` declaring both `workflow_rules` (simple array) and `context_sources` (inline-tables)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** both section lists SHALL be populated in the `BriefFragments` object

---

### Requirement: BriefFragment and BriefFragments dataclasses

`recipe_schema.py` SHALL define:

- `BriefFragment`: a dataclass with fields `key: Optional[str]` and `text: str`.
- `BriefFragments`: a dataclass (or equivalent typed container) with one optional field per
  contributable section (`runtime_flow`, `context_sources`, `conflict_policy`,
  `workflow_rules`, `useful_commands`, `mcp_descriptions`), each typed as
  `Optional[List[BriefFragment]]`.

The `Recipe` dataclass SHALL include a field `brief_fragments: Optional[BriefFragments]`.

#### Scenario: BriefFragment normalized correctly for simple string

- **GIVEN** a fragment text parsed from a simple string array element `"Do X."`
- **WHEN** the parser normalizes it
- **THEN** the resulting `BriefFragment` SHALL have `key = None` and `text = "Do X."`

#### Scenario: BriefFragment normalized correctly for inline-table

- **GIVEN** a fragment inline-table `{key = "foo", text = "Do Y."}`
- **WHEN** the parser normalizes it
- **THEN** the resulting `BriefFragment` SHALL have `key = "foo"` and `text = "Do Y."`

---

### Requirement: Validation of [provides.brief] content

`recipe_schema.py` SHALL validate `[provides.brief]` entries and fail with explicit errors for:

1. Fragment declared for `intro` or `purpose` (project-only sections).
2. Fragment declared for an unknown section name.
3. Inline-table entry missing `text` or `key`.
4. Mixed forms in a single section (TOML array value AND inline-table entries simultaneously).

#### Scenario: intro section in [provides.brief] causes validation failure

- **GIVEN** a `recipe.toml` with `[provides.brief].intro = ["My intro."]`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error stating `intro` is a project-only section

#### Scenario: purpose section in [provides.brief] causes validation failure

- **GIVEN** a `recipe.toml` with `[provides.brief].purpose = ["My purpose."]`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error stating `purpose` is a project-only section

#### Scenario: Unknown section name causes validation failure

- **GIVEN** a `recipe.toml` with `[provides.brief].custom_rules = ["A rule."]`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming `custom_rules` as unknown and listing the valid section names

#### Scenario: Inline-table with missing text field fails validation

- **GIVEN** a `[[provides.brief.workflow_rules]]` entry containing only `key = "foo"` (no `text`)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming the missing `text` field

#### Scenario: Inline-table with missing key field fails validation

- **GIVEN** a `[[provides.brief.workflow_rules]]` entry containing only `text = "A rule."` (no `key`)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming the missing `key` field
