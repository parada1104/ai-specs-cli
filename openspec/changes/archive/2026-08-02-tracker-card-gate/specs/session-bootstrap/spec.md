# session-bootstrap (delta)

## ADDED Requirements

### Requirement: Mandatory tracker consult when tracker is bound

The `session-bootstrap` skill (session-context recipe) is capability-agnostic.
When a project has a **tracker** capability bound (auto-bound or explicitly
bound to a provider such as `trello-mcp-workflow`), session bootstrap MUST
consult the tracker capability for **new** or **ambiguous** changes / focus
resolution.

Memory-first ordering MAY remain (query memory, then runtime brief), but
tracker consultation is no longer optional on the new/ambiguous path. The soft
phrase "only if needed" / "only checks the tracker capability when gaps or
contradictions remain" MUST NOT be the governing rule when a tracker capability
is bound and the focus is new or ambiguous.

If the tracker capability/MCP is unavailable, bootstrap SHALL continue with
available sources and state the gap (availability degrade), without treating a
missing link artifact on an active change as unavailability.

#### Scenario: Bound tracker consulted for ambiguous focus

- **GIVEN** the project binds a `tracker` capability provider
- **AND** the user request is ambiguous (e.g. "continuar", "apply" without a
  change name, "siguiente card")
- **WHEN** `session-bootstrap` resolves session focus
- **THEN** it MUST consult the tracker capability after memory/brief as part of
  focus resolution
- **AND** MUST NOT skip the tracker solely because memory returned some context

#### Scenario: Bound tracker consulted for a new structured change

- **GIVEN** the project binds a `tracker` capability provider
- **AND** the session is starting or focusing a new structured OpenSpec change
- **WHEN** `session-bootstrap` runs
- **THEN** it MUST consult the tracker capability for work-state context
  relevant to that change (active card / link status)

#### Scenario: No tracker bound keeps consult optional

- **GIVEN** the project has no bound `tracker` capability provider
- **WHEN** `session-bootstrap` resolves focus for an ambiguous request
- **THEN** it MUST NOT require a tracker consultation
- **AND** MAY resolve focus from memory, brief, and other configured sources

#### Scenario: Tracker unavailable degrades without blocking bootstrap

- **GIVEN** a tracker capability is bound
- **AND** the tracker provider/MCP is unavailable
- **WHEN** `session-bootstrap` would consult the tracker for a new/ambiguous change
- **THEN** it MUST state the gap and continue with available sources
- **AND** MUST NOT block the session start
