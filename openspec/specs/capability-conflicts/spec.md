# capability-conflicts Specification

## Purpose

Define how the sync pipeline detects and reports conflicts when multiple enabled recipes declare the same capability.

## Requirements

### Requirement: Conflict detection for ambiguous capability bindings

If two or more enabled recipes declare the same capability and that capability is bound (explicitly or via auto-bind), sync SHALL fail with a conflict error unless exactly one binding resolves the ambiguity.

#### Scenario: Auto-bind ambiguity prevents binding with warning
- **GIVEN** two enabled recipes both declare the `"tracker"` capability
- **AND** the manifest contains no explicit `[[bindings]]` for `"tracker"`
- **WHEN** sync attempts to resolve bindings
- **THEN** auto-bind SHALL NOT occur due to ambiguity
- **AND** sync SHALL emit a warning naming the `"tracker"` capability and both conflicting recipes
- **AND** sync SHALL NOT fail solely due to the unresolved ambiguity

#### Scenario: Explicit binding resolves ambiguity
- **GIVEN** two enabled recipes both declare the `"tracker"` capability
- **AND** the manifest contains `[[bindings]]` with `capability = "tracker"` and `recipe = "trello-pm"`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL bind `"tracker"` to `"trello-pm"`
- **AND** sync SHALL NOT fail
- **AND** the other recipe SHALL NOT be considered bound to `"tracker"`

#### Scenario: No conflict when only one recipe is enabled
- **GIVEN** two recipes declare the `"tracker"` capability
- **AND** only one of them is enabled
- **AND** the manifest contains no explicit `[[bindings]]` for `"tracker"`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL auto-bind `"tracker"` to the enabled recipe
- **AND** sync SHALL NOT fail

#### Scenario: No conflict when capability is unbound
- **GIVEN** two enabled recipes both declare the `"tracker"` capability
- **AND** the manifest contains no explicit `[[bindings]]` for `"tracker"`
- **AND** auto-bind is prevented by ambiguity
- **WHEN** sync evaluates whether to fail
- **THEN** sync SHALL NOT fail if the capability is not required by any other system component
- **AND** sync SHALL emit a warning about the unresolved ambiguity

### Requirement: Conflict detection ordering

Capability conflict detection SHALL occur before primitive materialization and before hook execution.

#### Scenario: Conflict prevents materialization
- **GIVEN** a capability conflict exists between two enabled recipes
- **WHEN** sync runs
- **THEN** conflict detection SHALL run first
- **AND** no primitives from either conflicting recipe SHALL be materialized
