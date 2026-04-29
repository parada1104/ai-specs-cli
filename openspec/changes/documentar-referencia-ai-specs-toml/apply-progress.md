# Apply Progress: Documentar referencia completa de ai-specs.toml

## Summary

Implemented the user-facing `ai-specs.toml` reference and linked it from README without changing manifest runtime behavior. A follow-up audit narrowed the scope to the mature TOML surface: `[project]`, `[[deps]]`, and `[mcp.<name>]`, with `[agents]` only as fan-out context.

## Completed Tasks

- Added documentation contract tests in `tests/test_manifest_contract_docs.py`.
- Created `docs/ai-specs-toml.md` with mature supported sections, field tables, examples, MCP env contract, and agent render behavior.
- Linked the reference from README near the Manifest V1 contract section.
- Reduced README roadmap/detail and removed recipe/SDD promotion from the stable TOML reference path.
- Added guard tests so recipes and SDD are not documented as stable TOML reference surface.
- Updated OpenSpec tasks to reflect completed work.

## Test Evidence

| Command | Result | Notes |
|---|---|---|
| `python3 -m unittest tests.test_manifest_contract_docs` | PASS | Targeted docs contract test. |
| `grep -RInE 'recipes|recipe|\\[sdd\\]|artifact_store|runtime-memory-openmemory|roadmap|EPIC' README.md docs/ai-specs-toml.md` | PASS | No unstable reference terms remain in README/reference docs. |
| `./tests/validate.sh` | PASS | Syntax checks plus full unittest suite. |

## Non-Goals Preserved

- No runtime parser/render changes.
- No recipe docs, recipe v2 binding, capabilities, hooks, or tracker adapter changes.
- No SDD/OpenSpec TOML reference treatment.
- No standalone `[memory]` manifest section.
