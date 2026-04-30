## 1. Infrastructure — Recipe Packaging & V2 Manifest

- [ ] 1.1 Create `catalog/recipes/trello-mcp-workflow/` directory skeleton: `skills/trello-mcp-workflow/`, `templates/`, `commands/`, `docs/`
- [ ] 1.2 Write `recipe.toml` with V2 schema: 4 capabilities, 4 hooks, 3 config fields, provides declarations
- [ ] 1.3 Extend `lib/_internal/recipe-materialize.py` `execute_hooks()` to branch on 4 new hook actions (`bootstrap-board`, `link-trello-card`, `sync-card-state`, `comment-verification`)
- [ ] 1.4 Implement `bootstrap-board` sync-time hook: validate `board_id` presence, write `.recipe/<recipe-id>/bootstrap-ready` marker
- [ ] 1.5 Implement deferred hook stubs: `link-trello-card`, `sync-card-state`, `comment-verification` print informational notice to stdout
- [ ] 1.6 Ensure unknown hook actions still warn and skip (regression guard — existing behavior preserved)
- [ ] 1.7 Add config schema validation in materialize: `board_id` required, `default_list`/`epic_list` optional with defaults

## 2. Implementation — Bundled Skill, Templates & Docs

- [ ] 2.1 Write bundled skill `skills/trello-mcp-workflow/SKILL.md`: runtime MCP call logic, phase-to-list mapping, phase-to-label mapping, graceful degradation rules
- [ ] 2.2 Create card template `templates/card-feature.md` with OpenSpec change placeholders
- [ ] 2.3 Create card template `templates/card-bug.md` with bug-specific placeholders
- [ ] 2.4 Create card template `templates/card-spike.md` with research/spike placeholders
- [ ] 2.5 Create card template `templates/card-epic.md` with epic/overview placeholders
- [ ] 2.6 Create card template `templates/card-handoff.md` with session handoff placeholders
- [ ] 2.7 Write `commands/trello-workflow.md` quick-reference for agent usage
- [ ] 2.8 Write `docs/README.md` with installation, configuration, and rollback instructions

## 3. Implementation — Runtime Capabilities

- [ ] 3.1 Implement `trello-session-bootstrap` capability: read marker, query board via MCP, detect active card by list position + labels, feed structured primitive to consensus check
- [ ] 3.2 Implement `trello-card-linking` capability: detect linked card from session context, post structured comment with change folder path and artifact list
- [ ] 3.3 Implement card creation offer in `trello-card-linking`: when no card exists, prompt agent to create from bundled template, then post initial comment
- [ ] 3.4 Implement `trello-state-sync` capability: resolve target list from phase mapping, move card, replace single phase label, post phase-transition comment with metadata
- [ ] 3.5 Implement `trello-progress-comment` capability: collect changed files from apply metadata, read test results from `verify-report.md`, assemble structured comment with verdict and archive link
- [ ] 3.6 Add graceful degradation: all runtime Trello failures emit warnings to stderr and optionally log to `.recipe/trello-mcp-workflow/warnings.log`, never blocking agent progress

## 4. Testing — Unit & Integration

- [ ] 4.1 Write unit tests for `execute_hooks()` new actions: `bootstrap-board` creates marker, deferred actions print notice, unknown actions warn
- [ ] 4.2 Write unit tests for config validation: `board_id` required, optional fields with defaults
- [ ] 4.3 Write integration tests for recipe materialization: sync `trello-mcp-workflow` recipe creates all assets, templates, marker file
- [ ] 4.4 Verify spec compliance for `sync-hooks` delta spec (6 scenarios)
- [ ] 4.5 Verify spec compliance for `trello-session-bootstrap` (6 scenarios)
- [ ] 4.6 Verify spec compliance for `trello-card-linking` (7 scenarios)
- [ ] 4.7 Verify spec compliance for `trello-state-sync` (6 scenarios)
- [ ] 4.8 Verify spec compliance for `trello-progress-comment` (7 scenarios)
- [ ] 4.9 Run `./tests/validate.sh` and confirm all tests pass