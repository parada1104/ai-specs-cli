# Apply Progress: card-74-clean-materialization

Worktree: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card-74-clean-materialization`
Branch: `change/card-74-clean-materialization`
Command (both runs): `python3 -m unittest tests.test_release_materialization -v`

## Resolved Scope

- Isolated lock stamped `0.22.0`; dogfood lock was not rewritten.
- `SHA256SUMS` already declared v0.22.0 + four platforms (no catalog change).
- Release-flow pointer added to `ai-specs/skills/release-flow/SKILL.md` (authored skill; `.claude/skills/` is generated and absent in this worktree).
- No product/lib/catalog changes required for GREEN.

## RED

First run of `tests/test_release_materialization.py`, before the MCP assertion fix:

```
test_isolated_init_sync_doctor_materializes_clean_consumer ... FAIL
test_sha256sums_declares_candidate_version_and_four_platforms ... ok
AssertionError: False is not true : missing generated output: .mcp.json
Ran 2 tests in 3.799s
FAILED (failures=1)
```

Cause: the test required Doctor.PLATFORM `mcp_config_path` files, but the authorized representative manifest has no `[mcp.*]`. Sync correctly skips MCP adapters; doctor WARNs. Not a product defect.

## GREEN

After skipping `mcp_config_path` when no MCP is declared:

```
test_isolated_init_sync_doctor_materializes_clean_consumer ... ok
test_sha256sums_declares_candidate_version_and_four_platforms ... ok
Ran 2 tests in 2.151s
OK
```

GREEN was a test-contract fix only. No product change.

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Clean materialization gate | `tests/test_release_materialization.py` | Integration | N/A (new test file); later `./tests/validate.sh` passed 1672 tests with 116 skipped | ✅ Written; first focused run failed because the test over-required `.mcp.json` while no `[mcp.*]` was declared; test-contract correction, not a product fix | ✅ Passed; focused test passed 2/2 after correcting the contract | ⚠️ Limited: 2 test methods cover 8 spec scenarios; not 8 test cases | No production refactor; documentation/test-contract cleanup only |

## P4 — validate.sh

```
./tests/validate.sh
Ran 1672 tests in 416.182s
OK (skipped=116)
exit 0
```

## Remaining

- Verify report, archive, and PR are still pending (P5).
