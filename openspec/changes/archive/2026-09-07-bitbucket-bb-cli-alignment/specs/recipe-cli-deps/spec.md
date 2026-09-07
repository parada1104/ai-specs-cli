# Delta for recipe-cli-deps

## ADDED Requirements

### Requirement: Bitbucket host CLI version metadata

The `bitbucket-pr-flow` `[[deps.cli]]` entry for binary `bb` MUST declare
`version_check = "bb --version"` and `min_version = "1.4.1"`. That host floor
is the minimum supported PHP `bb-cli` version. The recipe's own `version`
field MUST remain `1.3.0` and MUST NOT be treated as the host CLI version.

Identity mapping is unchanged: PHP `bb-cli` at https://bb-cli.github.io,
Homebrew formula `bb-cli`, binary `bb`. The numeric version check MUST NOT
be treated as proof that the binary is PHP `bb-cli`.

#### Scenario: Recipe declares host floor distinct from recipe version

- **GIVEN** `catalog/recipes/bitbucket-pr-flow/recipe.toml`
- **WHEN** the `[[deps.cli]]` row for `bb` is read
- **THEN** `version_check` is `bb --version`
- **AND** `min_version` is `1.4.1`
- **AND** `[recipe].version` is `1.3.0`

#### Scenario: Host version metadata does not change identity mapping

- **GIVEN** the Bitbucket CLI dependency row
- **WHEN** install identity is inspected
- **THEN** the upstream remains PHP `bb-cli` at https://bb-cli.github.io
- **AND** the Homebrew formula remains `bb-cli`
- **AND** the binary remains `bb`
- **AND** the plan MUST NOT propose formula or cask `bb`

## MODIFIED Requirements

### Requirement: TTY opt-in install for known packages

On an interactive TTY, when a required `[[deps.cli]]` binary is missing or
unusable, the system SHALL resolve an install plan from the constrained static
binary-to-package map and ask the user for explicit confirmation before running
any installer command. Supported resolvers:

- Homebrew: `brew install <formula>` when `brew` is on PATH and a formula mapping exists
- apt: show `sudo apt-get install -y <package>` when `apt-get` is on PATH and a
  non-empty package mapping exists; run only after confirm
- Otherwise: guidance-only using `install_url` (no command execution)

The static package map MUST map the `bb` binary to Homebrew formula `bb-cli` and
an empty apt package. Therefore, when Homebrew is available, the install plan for
`bb` MUST offer `brew install bb-cli`; on apt-only Linux or another unsupported
resolver, it MUST fall back to guidance using `install_url` because the apt side
is empty. The system MUST never propose or execute `brew install bb`, whose
Homebrew cask is an unrelated `getbb.app` product.

Only `npx` SHALL remain guidance-only in this known guidance-only set. The system
MUST NOT blindly install Node or any package for `npx`. TTY confirmation-before-
install, non-TTY check-only behavior, and doctor check-only behavior MUST remain
unchanged.

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

#### Scenario: Bitbucket binary offers the bb-cli Homebrew formula

- **GIVEN** an interactive TTY session with missing `bb`
- **AND** Homebrew is available
- **WHEN** install resolution runs for `bb`
- **THEN** the plan MUST offer `brew install bb-cli`
- **AND** the plan MUST NOT offer `brew install bb`
- **AND** installation MUST run only after explicit user confirmation

#### Scenario: Apt-only Bitbucket resolution remains guidance-only

- **GIVEN** an interactive TTY session with missing `bb`
- **AND** apt-get is available
- **AND** the static apt package mapping for `bb` is empty
- **WHEN** install resolution runs for `bb`
- **THEN** the plan MUST be guidance-only
- **AND** the guidance MUST use the recipe `install_url`
- **AND** the system MUST NOT run an apt installer

#### Scenario: Non-TTY and doctor paths never install bb

- **GIVEN** `bb` is missing and either the session is non-interactive or doctor is running
- **WHEN** dependency handling runs
- **THEN** the system MUST NOT prompt for installation
- **AND** the system MUST NOT invoke brew or apt
- **AND** the report or guidance MUST not propose `brew install bb`
