# auto-binding Specification

## Purpose

Define implicit capability binding when exactly one enabled recipe declares a given capability, eliminating the need for explicit `[[bindings]]` declarations.

## Requirements

### Requirement: Auto-bind for unambiguous capabilities

If exactly one enabled recipe declares a capability and no explicit `[[bindings]]` table exists for that capability, sync SHALL automatically bind the capability to that recipe.

#### Scenario: Single provider auto-bound
- **GIVEN** exactly one enabled recipe declares the `"tracker"` capability
- **AND** the manifest contains no `[[bindings]]` for `"tracker"`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL implicitly bind `"tracker"` to that recipe
- **AND** sync SHALL emit a log message naming the auto-bound capability and recipe

#### Scenario: Multiple providers prevent auto-bind
- **GIVEN** two enabled recipes both declare the `"tracker"` capability
- **AND** the manifest contains no `[[bindings]]` for `"tracker"`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL NOT auto-bind `"tracker"`
- **AND** sync SHALL NOT fail solely due to the unbound capability
- **AND** sync SHALL emit a log message noting the ambiguity

#### Scenario: Zero providers prevent auto-bind
- **GIVEN** no enabled recipe declares the `"tracker"` capability
- **AND** the manifest contains no `[[bindings]]` for `"tracker"`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL NOT auto-bind `"tracker"`
- **AND** sync SHALL NOT fail

#### Scenario: Explicit binding overrides auto-bind
- **GIVEN** exactly one enabled recipe declares the `"tracker"` capability
- **AND** the manifest contains `[[bindings]]` with `capability = "tracker"` and `recipe = "trello-pm"`
- **WHEN** sync resolves bindings
- **THEN** the explicit binding SHALL take precedence
- **AND** auto-bind SHALL NOT occur for that capability

#### Scenario: Disabled recipe excluded from auto-bind count
- **GIVEN** one enabled recipe declares `"tracker"`
- **AND** one disabled recipe also declares `"tracker"`
- **AND** the manifest contains no `[[bindings]]` for `"tracker"`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL auto-bind `"tracker"` to the enabled recipe
- **AND** the disabled recipe SHALL NOT be considered in the count
