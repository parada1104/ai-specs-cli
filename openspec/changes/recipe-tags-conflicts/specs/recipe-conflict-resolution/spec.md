# recipe-conflict-resolution Specification

## ADDED Requirements

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
