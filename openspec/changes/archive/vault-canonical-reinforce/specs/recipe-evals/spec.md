# recipe-evals (delta)

## Purpose

Add a vault-canonical-store client to the slow-tier eval harness, matching the
existing plan-build and vcs-pr-flow patterns: dry smoke always, live only with
`EVALS_LIVE=1` and a dedicated runner.

## Requirements

### Requirement: vault-canonical-store scenario pack

The repo SHALL provide scenario fixtures under
`tests/evals/scenarios/vault-canonical-store/` covering at least:

| Scenario id | Intent |
|-------------|--------|
| `ac_kepano_skills_present` | After enable/sync, kepano Obsidian skills are available to the runtime |
| `ac_mcp_path_with_spaces` | MCP configs preserve env-backed vault path args (dry asserts OK) |
| `ac_vault_context_guidance` | Agent follows vault-context note shape / Engram vs Vault split |

Each scenario SHALL include `scenario.toml` and a natural-language `prompt.txt`
(no slash-command planning verbs).

#### Scenario: Dry smoke loads vault fixtures

- **GIVEN** `EVALS_LIVE` is unset
- **WHEN** `tests/evals/run.sh` runs
- **THEN** vault-canonical-store scenario fixtures load without network
- **AND** materializing a project with the recipe enabled succeeds in smoke tests

### Requirement: Dedicated live runner

Live vault evals SHALL live in `tests/evals/eval_vault_canonical_live.py` and
SHALL be invoked via `tests/evals/run-live-vault.sh` (not mixed into
`run-live.sh` / `run-live-vcs.sh`). Without `EVALS_LIVE=1`, the live module
MUST skip rather than fail dry discovery.

#### Scenario: Live module skips offline

- **GIVEN** `EVALS_LIVE` is unset
- **WHEN** dry eval discovery imports `eval_vault_canonical_live`
- **THEN** live cases are skipped (or not executed), and dry smoke still passes

### Requirement: README documents the vault client

`tests/evals/README.md` SHALL document the vault-canonical-store client,
scenario table, and how to select runtimes/scenarios for
`./tests/evals/run-live-vault.sh`.

## Acceptance Criteria (test map)

| AC | Coverage | Req |
|----|----------|-----|
| AC-V1 | `eval_harness_smoke` vault fixture load | dry fixtures |
| AC-V2 | `eval_vault_canonical_live` + skip gate | live opt-in |
| AC-V3 | `run-live-vault.sh` + README section | discoverability |
| AC-V4 | scenario dirs for three ACs above | scenario pack |
