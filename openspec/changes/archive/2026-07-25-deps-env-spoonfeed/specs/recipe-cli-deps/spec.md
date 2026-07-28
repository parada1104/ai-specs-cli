# recipe-cli-deps (delta)

## ADDED Requirements

### Requirement: CLI dependency checks remain non-destructive by default

The system SHALL continue to detect recipe `[[deps.cli]]` binaries via PATH (and
optional version checks). Detection alone MUST NOT install software. Doctor and
non-interactive paths MUST remain check-only.

#### Scenario: Doctor does not install

- **GIVEN** an enabled recipe requires `gh` and `gh` is missing
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN for the missing binary
- **AND** the system MUST NOT invoke brew, apt, or any installer

#### Scenario: Non-TTY configure does not install

- **GIVEN** stdin/stdout are not a TTY
- **AND** a required CLI dep is missing
- **WHEN** dependency handling runs
- **THEN** the system MUST NOT prompt for install
- **AND** the system MUST NOT run an installer

### Requirement: TTY opt-in install for known packages

On an interactive TTY, when a required `[[deps.cli]]` binary is missing or
unusable, the system SHALL resolve an install plan and ask the user for explicit
confirmation before running any installer command. Supported resolvers:

- Homebrew: `brew install <formula>` when `brew` is on PATH and a formula mapping exists
- apt: show `sudo apt-get install -y <package>` when `apt-get` is on PATH and a
  package mapping exists; run only after confirm
- Otherwise: guidance-only using `install_url` (no command execution)

`npx` and `bb` SHALL be guidance-only (no blind Node/bb package install).

#### Scenario: User declines install

- **GIVEN** TTY session with missing `gh` and brew available
- **WHEN** the user answers No to the install prompt
- **THEN** no installer command MUST run
- **AND** the existing configure-anyway / skip behavior MUST remain available

#### Scenario: User accepts brew install

- **GIVEN** TTY session with missing `jq` and brew available
- **WHEN** the user answers Yes to install
- **THEN** the system MUST run `brew install jq` (or equivalent mapped formula)
- **AND** MUST re-check PATH for `jq` afterward

#### Scenario: Guidance-only for npx

- **GIVEN** TTY session with missing `npx`
- **WHEN** install resolution runs for `npx`
- **THEN** the plan MUST be guidance-only
- **AND** the system MUST NOT run `brew install node` without a separate future
  explicit design

### Requirement: Install command source is constrained

Installer argv MUST come from a static binary→package map plus fixed brew/apt
prefixes. The system MUST NOT execute user-supplied shell strings or
`curl | sh` installers.

#### Scenario: Unknown binary stays guidance-only

- **GIVEN** a required dep binary with no map entry
- **WHEN** resolve_install_plan runs
- **THEN** kind MUST be guidance
- **AND** command MUST be empty

### Requirement: direnv install offer on env scaffold path

When the harness env offer path needs `direnv allow` and `direnv` is missing on a
TTY, the system SHALL offer the same opt-in install flow used for recipe CLIs
before soft-failing allow.

#### Scenario: Offer direnv when missing during env scaffold

- **GIVEN** TTY and missing `direnv`
- **AND** enabled recipes require MCP env vars
- **WHEN** the env offer path reaches the allow step
- **THEN** the user MUST be prompted to install `direnv` (when a resolver exists)
- **OR** receive non-fatal install guidance if they decline / no resolver
