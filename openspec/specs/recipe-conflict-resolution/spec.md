# recipe-conflict-resolution Specification

## Purpose

Define explicit error behavior when two or more recipes declare the same primitive ID, and precedence rules when the same skill ID exists across local, recipe, and dependency sources.

## Requirements

### Requirement: Primitive ID uniqueness across recipes
During sync, the system SHALL maintain a registry of claimed primitive IDs. If a second recipe attempts to claim an ID already registered, sync SHALL fail with an explicit error message.

#### Scenario: Conflicting skills
- **WHEN** recipe A declares `skill.id = "openmemory-proactive"` and recipe B also declares `skill.id = "openmemory-proactive"`
- **THEN** sync SHALL fail
- **AND** the error SHALL name both recipes and the conflicting primitive ID

#### Scenario: Conflicting commands
- **WHEN** recipe A declares `command.id = "handoff"` and recipe B also declares `command.id = "handoff"`
- **THEN** sync SHALL fail with an explicit conflict error

#### Scenario: Conflicting MCP presets
- **WHEN** recipe A declares `mcp.id = "openmemory"` and recipe B also declares `mcp.id = "openmemory"`
- **THEN** sync SHALL fail with an explicit conflict error

### Requirement: Conflict scope
Conflict detection SHALL apply across `[recipes.*]` primitives (recipe-recipe conflicts). A local skill in `ai-specs/skills/{id}/` SHALL always take precedence over a recipe-bundled or dependency skill with the same ID, without being treated as an error and without emitting a warning.

#### Scenario: Local skill overrides recipe skill
- **WHEN** `ai-specs/skills/my-skill/` exists
- **AND** a recipe also provides `my-skill`
- **THEN** sync SHALL NOT fail
- **AND** sync SHALL NOT emit a warning
- **AND** the local version in `ai-specs/skills/my-skill/` SHALL be used

#### Scenario: Local skill overrides dependency skill
- **WHEN** `ai-specs/skills/my-skill/` exists
- **AND** a dependency also provides `my-skill`
- **THEN** sync SHALL NOT fail
- **AND** sync SHALL NOT emit a warning
- **AND** the local version in `ai-specs/skills/my-skill/` SHALL be used

#### Scenario: Recipe vs user-local command
- **WHEN** a recipe provides a command that already exists as a user-created file in `ai-specs/commands/`
- **THEN** sync SHALL emit a warning
- **AND** sync SHALL proceed with the recipe version
- **AND** sync SHALL NOT fail

### Requirement: Multi-source skill precedence
When the same skill ID exists in multiple sources, the system SHALL resolve it using source precedence: local (`ai-specs/skills/`) > recipe (`.recipe/{recipe-id}/skills/`) > dependency (`.deps/{dep-id}/skills/`). This is not a conflict error; it is a deterministic resolution rule.

#### Scenario: Same skill in local and recipe
- **WHEN** `ai-specs/skills/shared-skill/` exists
- **AND** `.recipe/my-recipe/skills/shared-skill/` also exists
- **THEN** the system SHALL use the local version
- **AND** no conflict error SHALL be raised

#### Scenario: Same skill in recipe and dep
- **WHEN** `.recipe/my-recipe/skills/shared-skill/` exists
- **AND** `.deps/my-dep/skills/shared-skill/` also exists
- **THEN** the system SHALL use the recipe version
- **AND** no conflict error SHALL be raised

#### Scenario: Same skill in local, recipe, and dep
- **WHEN** the same skill ID exists in all three sources
- **THEN** the system SHALL use the local version
- **AND** no conflict error SHALL be raised

### Requirement: Recipe-recipe primitive conflicts still fail
If two or more recipes declare the same primitive ID (skill, command, or MCP preset) at the recipe level, sync SHALL fail with an explicit error, regardless of whether those primitives would be materialized to different external directories.

#### Scenario: Conflicting skills across recipes
- **WHEN** recipe A declares `skill.id = "openmemory-proactive"` and recipe B also declares `skill.id = "openmemory-proactive"`
- **THEN** sync SHALL fail
- **AND** the error SHALL name both recipes and the conflicting primitive ID

#### Scenario: Conflicting commands across recipes
- **WHEN** recipe A declares `command.id = "handoff"` and recipe B also declares `command.id = "handoff"`
- **THEN** sync SHALL fail with an explicit conflict error

#### Scenario: Conflicting MCP presets across recipes
- **WHEN** recipe A declares `mcp.id = "openmemory"` and recipe B also declares `mcp.id = "openmemory"`
- **THEN** sync SHALL fail with an explicit conflict error

### Requirement: Error message format
Conflict error messages SHALL include: the primitive type (skill, command, mcp), the conflicting ID, and the names of the conflicting recipes.

#### Scenario: Readable error
- **WHEN** a conflict occurs
- **THEN** the error message SHALL be actionable, e.g.: "recipe A and recipe B both declare mcp.id='openmemory'. Resolve manually in ai-specs.toml."

### Requirement: Tag-based conflict detection
The system SHALL provide `check_tag_conflicts(recipes)` that groups enabled
recipes by tag. For each tag shared by two or more recipes, it SHALL emit exactly
one `TagConflict` carrying the tag, the set of sharing recipe IDs, and a severity.
The severity SHALL be `fatal` when any sharing recipe lists another sharing
recipe in its `conflicts_with` (evaluated symmetrically — one side declaring the
relationship is sufficient); otherwise the severity SHALL be `warning`.

#### Scenario: No shared tag
- **WHEN** two enabled recipes share no tag
- **THEN** detection SHALL return no tag conflicts

#### Scenario: Single recipe
- **WHEN** only one recipe carries a given tag
- **THEN** detection SHALL return no tag conflict for that tag

#### Scenario: Shared tag without conflicts_with
- **WHEN** two enabled recipes share a tag and neither lists the other in `conflicts_with`
- **THEN** detection SHALL emit one `TagConflict` with severity `warning`
- **AND** its `recipes` set SHALL contain both recipe IDs

#### Scenario: Shared tag with conflicts_with
- **WHEN** two enabled recipes share a tag and one lists the other in `conflicts_with`
- **THEN** detection SHALL emit one `TagConflict` with severity `fatal`

#### Scenario: conflicts_with is symmetric
- **WHEN** two enabled recipes share a tag and only the second lists the first in `conflicts_with`
- **THEN** detection SHALL still emit a `fatal` `TagConflict` for the pair

#### Scenario: Output format
- **WHEN** a `TagConflict` for tag `vcs` between recipes `a` and `b` is serialized via `to_dict()`
- **THEN** the result SHALL be `{"type": "tag_conflict", "tag": "vcs", "recipes": ["a", "b"]}`

### Requirement: Tag conflicts are advisory during sync
During `ai-specs sync`, tag conflicts SHALL be surfaced as warnings on output and
SHALL NOT change the exit code or prevent materialization. Tags are advisory
metadata; the authority to block on competing capability providers belongs to the
capability-binding layer, not to tag detection. This preserves scenarios where
two recipes of the same category are intentionally enabled together and resolved
via `[[bindings]]` (e.g. `git-pr-flow` + `bitbucket-pr-flow`).

#### Scenario: Two same-tag recipes enabled with an explicit binding
- **WHEN** two recipes sharing a tag are enabled and an explicit `[[bindings]]` entry resolves the contested capability
- **THEN** sync SHALL emit a tag-overlap warning
- **AND** sync SHALL materialize both recipes
- **AND** sync SHALL NOT fail
