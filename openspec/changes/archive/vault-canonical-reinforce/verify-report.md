# Verify report: vault-canonical-reinforce

Date: 2026-07-19

## Commands

| Layer | Command | Result |
|-------|---------|--------|
| Unit (vault) | `python3 -m unittest tests.test_vault_canonical_store_recipe -v` | PASS (9) |
| Eval dry | fixtures via `eval_harness_smoke` vault tests | PASS |
| Full | `./tests/validate.sh` | PASS — 982 tests, 212.665s |

## AC map

| AC | Evidence |
|----|----------|
| Kepano deps declared + materialize | `test_recipe_declares_kepano_dep_skills`, `test_materializes_kepano_dep_skills_from_fixture` |
| MCP pin + single path arg | `test_recipe_mcp_pin_and_env_arg`, `test_sync_vault_mcp_preserves_single_path_arg_across_agents` |
| vault-context cross-links | `test_vault_context_cross_links_obsidian_skills` |
| Docs / spaced path | `test_readme_documents_mcp_and_spaced_paths`, catalog README |
| Eval fixtures | `test_vault_canonical_scenario_fixtures_load`, `test_materialize_vault_canonical_store` |
| Live runner present | `run-live-vault.sh` + `eval_vault_canonical_live.py` (opt-in; not executed this verify) |

## Residual risk

- Production sync clones kepano from GitHub default branch (no commit pin) — accepted for v1.
- Live LLM evals not run in this verify pass; run `./tests/evals/run-live-vault.sh` when credentials available.
