# Apply Progress: Documentar referencia completa de ai-specs.toml

## Summary

Implemented the user-facing `ai-specs.toml` reference and linked it from README without changing manifest runtime behavior.

## Completed Tasks

- Added documentation contract tests in `tests/test_manifest_contract_docs.py`.
- Created `docs/ai-specs-toml.md` with supported sections, field tables, examples, MCP env contract, and agent render behavior.
- Linked the reference from README near the Manifest V1 contract section.
- Updated OpenSpec tasks to reflect completed work.

## Test Evidence

| Command | Result | Notes |
|---|---|---|
| `python3 -m unittest tests.test_manifest_contract_docs` | PASS | Targeted docs contract test. |
| `./tests/validate.sh` | PASS | Syntax checks plus full unittest suite. |

## Non-Goals Preserved

- No runtime parser/render changes.
- No recipe v2 binding, capabilities, hooks, or tracker adapter changes.
- No standalone `[memory]` manifest section.
