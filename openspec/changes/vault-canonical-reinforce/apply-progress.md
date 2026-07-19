# Apply progress: vault-canonical-reinforce

## RED evidence

```text
python3 -m unittest tests.test_vault_canonical_store_recipe -v
# Before recipe changes: FAILED (failures=5)
# - missing kepano dep skills
# - version != 1.2.0
# - vault-context missing Obsidian cross-links
# - README missing CANONICAL_VAULT_PATH / Mobile Documents
# - kepano materialize missing (deps not declared)
# MCP spaced-path sync test already passed (renderer OK)
```

## GREEN evidence

```text
python3 -m unittest tests.test_vault_canonical_store_recipe -v
# OK — 9 tests

./tests/evals/run.sh  (via eval_harness_smoke vault fixtures)
# vault scenario fixtures load; materialize vendors kepano deps via fixture

./tests/validate.sh
# Ran 982 tests in 212.665s — OK
```

## Implementation notes

- Recipe `source = "dep"` for five kepano skills; floating default branch (no SHA pin by auth).
- `AI_SPECS_VENDOR_FIXTURE_ROOT` + plain-tree copy in `vendor-skills.clone` keep unit/eval offline.
- `materialize_dep_skill` ensures `lib/_internal` on `sys.path` (skill_contract import).
- MCP path: `vault-fs-mcp.sh` reads absolute `CANONICAL_VAULT_PATH` from env at exec time
  (no `${VAR}` in MCP args). Smoke: real iCloud path with spaces passed as one argv.
- Live vault evals: `./tests/evals/run-live-vault.sh` (not run in this verify; dry smoke only).

## Path bug (user report)

`${CANONICAL_VAULT_PATH}` as MCP argv fails when the host does not expand args from
process env; `~/${path}` can appear to work for home-relative values. Fix: wrapper
+ env-owned absolute path. Prefer `.envrc` shell expansion over nested `$VAR` in `.env`.
