# recipe-schema Specification

## ADDED Requirements

### Requirement: Recipe tags and conflicts_with metadata
The `[recipe]` table MAY contain `tags` (array of strings) and `conflicts_with`
(array of strings). Both are optional and SHALL default to an empty list. `tags`
labels the recipe's domain/category. `conflicts_with` lists recipe IDs this recipe
is incompatible with. A recipe MUST NOT list its own ID in `conflicts_with`.

#### Scenario: Tags and conflicts_with absent
- **WHEN** `recipe.toml` omits both `tags` and `conflicts_with`
- **THEN** validation SHALL pass
- **AND** the parsed recipe SHALL expose `tags == []` and `conflicts_with == []`

#### Scenario: Valid tags array
- **WHEN** `recipe.toml` declares `tags = ["vcs", "github"]`
- **THEN** validation SHALL pass
- **AND** the parsed recipe SHALL expose `tags == ["vcs", "github"]`

#### Scenario: Valid conflicts_with array
- **WHEN** `recipe.toml` declares `conflicts_with = ["git-pr-flow", "gitlab-mr-flow"]`
- **THEN** validation SHALL pass
- **AND** the parsed recipe SHALL expose those IDs in `conflicts_with`

#### Scenario: tags is not an array
- **WHEN** `tags` is set to a non-array value (e.g. a string)
- **THEN** validation SHALL fail with an error naming `tags`

#### Scenario: tags contains a non-string element
- **WHEN** `tags` is an array containing a non-string element (e.g. `["vcs", 3]`)
- **THEN** validation SHALL fail with an error naming `tags`

#### Scenario: conflicts_with is not an array
- **WHEN** `conflicts_with` is set to a non-array value
- **THEN** validation SHALL fail with an error naming `conflicts_with`

#### Scenario: conflicts_with references the recipe itself
- **WHEN** a recipe with `id = "selfref"` declares `conflicts_with = ["selfref"]`
- **THEN** validation SHALL fail with an error naming `conflicts_with` and `selfref`
