## ADDED Requirements

### Requirement: Recipe MCP presets merge shallowly with manifest precedence
The system SHALL merge recipe MCP presets using shallow merge where the project manifest always prevails over recipe defaults.

#### Scenario: Project has existing MCP config
- **WHEN** a recipe provides an MCP preset with the same ID as one already configured in the project's `ai-specs.toml`
- **THEN** the project manifest keys are preserved
- **AND** recipe keys not present in the manifest are added
- **AND** a warning is emitted for every conflicting key

#### Scenario: Project has no existing MCP config
- **WHEN** a recipe provides an MCP preset with an ID not present in the project's `ai-specs.toml`
- **THEN** the entire MCP preset from the recipe is created in the merged configuration

#### Scenario: Multiple recipes provide the same MCP preset
- **WHEN** two or more recipes provide MCP presets with the same ID
- **THEN** the project manifest still prevails
- **AND** recipe keys not in the manifest are added from the first recipe encountered
- **AND** subsequent recipes only add keys not already present in the accumulated configuration
