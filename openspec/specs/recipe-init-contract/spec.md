## ADDED Requirements

### Requirement: Init contract document structure

A recipe's `init.md` SHALL be an executable contract that an agent can follow mechanically to configure the recipe. It SHALL be authored in Markdown and SHALL contain at least the following sections in this order: `Preguntas` (or `Questions`), `MCP Discovery` (when the recipe declares `[init].needs_mcp`), `TOML Target`, and `Post-write`.

Other top-level sections (e.g., narrative context, recipe-specific notes) MAY appear before `Preguntas` but SHALL NOT replace any required section.

#### Scenario: Init contract with all required sections

- **GIVEN** a recipe declares `[init].prompt = "init.md"`
- **AND** `init.md` contains sections `Preguntas`, `MCP Discovery`, `TOML Target`, and `Post-write`
- **WHEN** an agent reads the contract
- **THEN** the agent SHALL execute the contract sequentially without ambiguity

#### Scenario: Init contract missing required section

- **GIVEN** `init.md` omits the `TOML Target` section
- **WHEN** the recipe author validates the contract
- **THEN** the contract SHALL be considered ill-formed
- **AND** the agent SHALL NOT proceed to write to `ai-specs.toml`

### Requirement: Question block format

Each question under `Preguntas` SHALL be declared as a `### <key>` subsection where `<key>` matches a key in the recipe's `[config]` schema. Each question subsection SHALL include the following declarative fields:

- `Required`: `yes` or `no` (matches `[config.<key>].required`)
- `Type`: matches `[config.<key>].type`
- `Pregunta` (or `Question`): the prompt text the agent SHALL present to the user
- `Default` (when applicable): the default value (matches `[config.<key>].default`)
- `Validación` (or `Validation`, optional): expected format or constraints
- `Hint` (optional): user-facing guidance

#### Scenario: Required question with no default

- **GIVEN** a question subsection `### board_id` declares `Required: yes` and no `Default`
- **WHEN** the agent runs the contract
- **THEN** the agent SHALL ask the user the `Pregunta` text
- **AND** SHALL NOT proceed until a non-empty value is provided

#### Scenario: Optional question with default

- **GIVEN** a question subsection `### default_list` declares `Required: no` and `Default: In Progress`
- **WHEN** the user provides an empty answer
- **THEN** the agent SHALL omit the key from the proposed TOML target (relying on the recipe schema default)

#### Scenario: Question key mismatches schema

- **GIVEN** `init.md` declares a question `### unknown_key` not present in `[config]` schema
- **WHEN** the agent validates the contract
- **THEN** the agent SHALL flag the mismatch as a contract error and SHALL NOT write the unknown key to the manifest

### Requirement: TOML target section

The `TOML Target` section SHALL contain a fenced TOML block under header `[recipes.<recipe-id>.config]` showing the literal target shape. The block SHALL use placeholder syntax `<answer:<key>>` to indicate where each user answer is substituted.

#### Scenario: Agent writes manifest from TOML target

- **GIVEN** the `TOML Target` section declares `board_id = "<answer:board_id>"`
- **AND** the user provided `board_id = "abc123"`
- **WHEN** the agent assembles the diff for `ai-specs/ai-specs.toml`
- **THEN** the diff SHALL include `board_id = "abc123"` under `[recipes.<recipe-id>.config]`

#### Scenario: Agent omits keys with default values

- **GIVEN** `default_list` resolved to its declared default
- **AND** the `TOML Target` block annotates the key as default-omittable (e.g., trailing comment `# omitir si es default`)
- **WHEN** the agent assembles the diff
- **THEN** the diff SHALL NOT include the `default_list` key

### Requirement: MCP discovery section

When the recipe declares `[init].needs_mcp`, the `MCP Discovery` section SHALL describe how the agent treats each declared MCP id: whether to propose a `[mcp.<id>]` block for review, what credentials handling to apply, and whether the project manifest already declares the server.

The agent SHALL NOT write secret values literally; secret references SHALL use environment-variable syntax (e.g., `${env:VAR_NAME}`) consistent with existing manifest conventions.

#### Scenario: MCP not configured in manifest

- **GIVEN** the recipe declares `[init].needs_mcp = ["trello"]`
- **AND** the project manifest has no `[mcp.trello]` table
- **WHEN** the agent runs the contract
- **THEN** the agent SHALL propose a reviewable `[mcp.trello]` block using `${env:VAR_NAME}` placeholders for credentials
- **AND** SHALL flag the proposal as requiring human review before sync

#### Scenario: MCP already configured

- **GIVEN** `[mcp.trello]` already exists in the project manifest
- **WHEN** the agent runs the contract
- **THEN** the agent SHALL NOT propose changes to credentials
- **AND** SHALL only propose `[recipes.<id>.config]` updates

### Requirement: Post-write section

The `Post-write` section SHALL declare the next steps the agent communicates to the user after writing the manifest diff. At minimum it SHALL remind the user that `ai-specs sync` is required to materialize the configuration and SHALL state that sync is not invoked automatically.

#### Scenario: Agent reminds about sync

- **WHEN** the agent finishes proposing the manifest diff
- **THEN** the agent SHALL echo the `Post-write` instructions to the user
- **AND** SHALL NOT invoke `ai-specs sync` on the user's behalf

### Requirement: Slash command wrapper invocation

The slash command `/recipe-init <id>` SHALL invoke `ai-specs recipe init <id>` via shell, capture its stdout (the recipe init brief plus the embedded `init.md` contract), and instruct the agent to follow the contract sequentially. The slash command SHALL NOT add logic beyond invocation and instruction; all configuration logic SHALL remain in `init.md`.

#### Scenario: Successful slash command invocation

- **GIVEN** the user invokes `/recipe-init trello-mcp-workflow`
- **WHEN** the slash command runs
- **THEN** it SHALL execute `ai-specs recipe init trello-mcp-workflow`
- **AND** SHALL pass the captured output to the agent context
- **AND** the agent SHALL proceed to follow the embedded contract

#### Scenario: Slash command with unknown recipe id

- **GIVEN** the user invokes `/recipe-init nonexistent-recipe`
- **WHEN** the slash command runs
- **THEN** the underlying shell SHALL exit with a non-zero status and a human-readable error
- **AND** the agent SHALL relay the error to the user without proceeding to write the manifest

### Requirement: Read-only init runtime

The `ai-specs recipe init` shell command and `lib/_internal/recipe-init.py` SHALL remain read-only with respect to project state. Neither SHALL modify `ai-specs/ai-specs.toml`, materialize primitives, nor invoke `sync`. All writes SHALL be performed by the agent following the `init.md` contract, subject to human review.

#### Scenario: Init command does not modify manifest

- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** `ai-specs/ai-specs.toml` SHALL remain byte-identical to its pre-run state
- **AND** no files under `ai-specs/recipes/<id>/` SHALL be created or modified
