# Tasks: Documentar referencia completa de ai-specs.toml

## Phase 1: Documentation contract tests

- [x] 1.1 Add RED coverage in `tests/test_manifest_contract_docs.py` requiring README to link the dedicated manifest reference.
- [x] 1.2 Add RED coverage requiring the dedicated reference to document supported sections, field types/defaults, supported agents, and both MCP `env` forms.

## Phase 2: User reference

- [x] 2.1 Create `docs/ai-specs-toml.md` with the complete manifest reference.
- [x] 2.2 Document `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]`, `[recipes.<id>]`, and `[sdd]` with examples.
- [x] 2.3 Document MCP render behavior for Claude Code, Cursor, OpenCode, and Codex where applicable.
- [x] 2.4 Include complete examples for common setups.

## Phase 3: README integration and verification

- [x] 3.1 Link the new reference from README near the Manifest V1 contract section.
- [x] 3.2 Run the targeted documentation test and fix failures.
- [x] 3.3 Run `./tests/validate.sh` and record verification evidence.
