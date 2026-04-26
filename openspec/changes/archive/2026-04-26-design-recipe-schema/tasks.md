# Tasks: Design Recipe Schema

## 1. Infrastructure

- [x] 1.1 Create `catalog/recipes/` directory structure with `.gitkeep`
- [x] 1.2 Add `recipe.toml` JSON schema / Python dataclass for validation
- [x] 1.3 Create `lib/_internal/recipe-read.py` — parse and validate `recipe.toml`
- [x] 1.4 Create `lib/_internal/recipe-conflicts.py` — detect primitive ID collisions across recipes

## 2. Manifest Contract Extension

- [x] 2.1 Update `templates/ai-specs.toml.tmpl` to include commented `[recipes.<id>]` example
- [x] 2.2 Update `lib/_internal/toml-read.py` to recognize `[recipes.*]` tables (no validation failure if absent)
- [x] 2.3 Update root `ai-specs/ai-specs.toml` manifest contract documentation table
- [x] 2.4 Add tests for `toml-read.py` with recipes present and absent

## 3. Recipe Materialization

- [x] 3.1 Create `lib/_internal/recipe-materialize.py` — orchestrate recipe sync
  - [x] 3.1.1 Implement bundled skills copy (`catalog/recipes/<id>/skills/` → `ai-specs/skills/`)
  - [x] 3.1.2 Implement external skills vendoring (dep source via `vendor-skills.py`)
  - [x] 3.1.3 Implement commands copy (`catalog/recipes/<id>/commands/` → `ai-specs/commands/`)
  - [x] 3.1.4 Implement MCP preset merge into derived configs
  - [x] 3.1.5 Implement template application with `not_exists` condition
  - [x] 3.1.6 Implement docs copy to target paths
- [x] 3.2 Wire recipe materialization into `lib/sync.sh` pipeline (after target resolve, before AGENTS.md render)
- [x] 3.3 Add version-pinning validation: fail sync if manifest pin ≠ `recipe.toml` version

## 4. Conflict Detection

- [x] 4.1 Implement conflict registry in `recipe-conflicts.py` (skill.id, command.id, mcp.id)
- [x] 4.2 Emit explicit error messages naming both recipes and conflicting primitive
- [x] 4.3 Add tests for conflict scenarios (skill collision, command collision, MCP collision)
- [x] 4.4 Add tests for recipe vs user-local skill (warning, not error)

## 5. Testing

- [x] 5.1 Create minimal recipe fixture in `catalog/recipes/test-fixture/` for integration tests
- [x] 5.2 Add unit tests for `recipe-read.py` (valid, missing fields, invalid paths)
- [x] 5.3 Add integration tests for `recipe-materialize.py` (full sync cycle)
- [x] 5.4 Add integration tests for conflict detection during sync
- [x] 5.5 Add integration tests for version mismatch during sync
- [x] 5.6 Run `./tests/validate.sh` and fix any failures
- [x] 5.7 Run `./tests/run.sh` and ensure all tests pass

## 6. Documentation

- [x] 6.1 Update `README.md` with recipe concept and quickstart example
- [x] 6.2 Document `recipe.toml` schema in `docs/` or inline comments
- [x] 6.3 Add recipe section to root manifest contract docs
