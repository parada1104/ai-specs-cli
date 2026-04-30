## 1. Infrastructure — Recipe Packaging & V2 Manifest

- [x] 1.1 Create `catalog/recipes/trello-mcp-workflow/` directory skeleton: `skills/trello-mcp-workflow/`, `templates/`, `commands/`, `docs/`
- [x] 1.2 Write `recipe.toml` with V2 schema: 4 capabilities, 4 hooks, 3 config fields, provides declarations
- [x] 1.3 Extend `execute_hooks()` signature to accept `project_root` parameter; update the single call site in `materialize_recipes()`; add `elif` branches for 4 new hook actions (`bootstrap-board`, `link-trello-card`, `sync-card-state`, `comment-verification`)
- [x] 1.4 Implement `bootstrap-board` sync-time hook: validate `board_id` presence, create `.recipe/<recipe-id>/` directory, write `bootstrap-ready` marker file using `project_root`
- [x] 1.5 Add `info()` helper to `recipe-materialize.py` (matching existing `fail()`/`warn()` pattern); implement deferred hook stubs (`link-trello-card`, `sync-card-state`, `comment-verification`) using `info()` not `warn()`
- [x] 1.6 Ensure unknown hook actions still use `warn()` and skip (regression guard — existing behavior preserved)
- [x] 1.7 Add config schema validation in materialize: `board_id` required, `default_list`/`epic_list` optional with defaults

## 2. Implementation — Bundled Skill, Templates & Docs

- [x] 2.1 Write bundled skill `skills/trello-mcp-workflow/SKILL.md`: runtime MCP call logic, phase-to-list mapping, phase-to-label mapping, graceful degradation rules
- [x] 2.2 Create card template `templates/card-feature.md` with OpenSpec change placeholders; target path `ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md`
- [x] 2.3 Create card template `templates/card-bug.md` with bug-specific placeholders; target path `ai-specs/recipes/trello-mcp-workflow/templates/card-bug.md`
- [x] 2.4 Create card template `templates/card-spike.md` with research/spike placeholders; target path `ai-specs/recipes/trello-mcp-workflow/templates/card-spike.md`
- [x] 2.5 Create card template `templates/card-epic.md` with epic/overview placeholders; target path `ai-specs/recipes/trello-mcp-workflow/templates/card-epic.md`
- [x] 2.6 Create card template `templates/card-handoff.md` with session handoff placeholders; target path `ai-specs/recipes/trello-mcp-workflow/templates/card-handoff.md`
- [x] 2.7 Write `commands/trello-workflow.md` quick-reference for agent usage
- [x] 2.8 Write `docs/README.md` with installation, configuration, and rollback instructions

## 3. Implementation — Runtime Capabilities

- [x] 3.1 Write SKILL.md section for `trello-session-bootstrap` capability: marker reading, board query via MCP, active card detection by list position + labels, consensus check integration
- [x] 3.2 Write SKILL.md section for `trello-card-linking` capability: detect linked card from session context, post structured comment with change folder path and artifact list
- [x] 3.3 Write SKILL.md section for card creation offer in `trello-card-linking`: when no card exists, prompt agent to create from bundled template, then post initial comment
- [x] 3.4 Write SKILL.md section for `trello-state-sync` capability: resolve target list from phase mapping, move card, replace single phase label, post phase-transition comment with metadata
- [x] 3.5 Write SKILL.md section for `trello-progress-comment` capability: collect changed files from apply metadata, read test results from verify-report, assemble structured comment with verdict and archive link
- [x] 3.6 Write SKILL.md section for graceful degradation: all runtime Trello failures emit warnings to stderr and optionally log to `.recipe/trello-mcp-workflow/warnings.log`, never blocking agent progress

## 4. Testing — Unit & Integration

- [x] 4.1 Write unit tests for `execute_hooks()` new actions: `bootstrap-board` creates marker directory and file, deferred actions print `info()` notice, unknown actions print `warn()`
- [x] 4.2 Write unit tests for `execute_hooks()` signature change: verify `project_root` parameter is used correctly by `bootstrap-board`
- [x] 4.3 Write unit tests for config validation: `board_id` required, `default_list`/`epic_list` optional with defaults
- [x] 4.4 Write integration tests for recipe materialization: sync `trello-mcp-workflow` recipe creates all assets, templates with `not_exists` condition, marker file
- [x] 4.5 Verify `sync-hooks` delta spec compliance (8 scenarios): validate-config success, validate-config failure, declaration order, bootstrap-board marker creation, bootstrap-board missing board_id, project_root propagation, deferred `info()` notice, unknown action `warn()`
- [x] 4.6 Verify `trello-session-bootstrap` spec compliance (6 scenarios): verify SKILL.md content covers all scenarios
- [x] 4.7 Verify `trello-card-linking` spec compliance (6 scenarios): verify SKILL.md content covers all scenarios
- [x] 4.8 Verify `trello-state-sync` spec compliance (6 scenarios): verify SKILL.md content covers all scenarios
- [x] 4.9 Verify `trello-progress-comment` spec compliance (7 scenarios): verify SKILL.md content covers all scenarios
- [x] 4.10 Run `./tests/validate.sh` and confirm all tests pass