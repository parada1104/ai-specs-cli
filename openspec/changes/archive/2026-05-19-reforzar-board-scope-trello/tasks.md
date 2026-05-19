# Tasks: Reforzar Board Scope en Trello MCP Workflow

## Phase 1: Foundation

- [ ] 1.1 Add `[config.board_isolation]` to `recipe.toml` with `forbidden_tools`, `restricted_tools`, `card_validation_required`
- [ ] 1.2 Remove "SDD Checklist" section from `card-feature.md`; add `trello-pm-workflow` skill reference

## Phase 2: Core SKILL.md — Guard Rails

- [ ] 2.1 Add Forbidden/Restricted Tools section to SKILL.md: `trello_get_my_cards` and `trello_list_boards` forbidden; `trello_set_active_board` restricted to bootstrap step 2
- [ ] 2.2 Add board guard to bootstrap step 2: `set_active_board` → `get_active_board_info` → verify id match → retry once → log + abort Trello on persistent failure
- [ ] 2.3 Pass explicit `boardId` to 5 MCP calls (`get_lists`, `get_cards_by_list_id`, `add_card_to_list`, `move_card`, `update_card_details`) across all capabilities
- [ ] 2.4 Add card `idBoard` validation before `trello_get_card` and `trello_add_comment`
- [ ] 2.5 Add board guard precondition (step 0) to `card-linking`, `state-sync`, `progress-comment` capabilities

## Phase 3: Secondary Files

- [ ] 3.1 Add forbidden/restricted tools reference table to `commands/trello-workflow.md`
- [ ] 3.2 Add board isolation cross-reference in `ai-specs/skills/trello-pm-workflow/SKILL.md`

## Phase 4: Verification

- [ ] 4.1 Validate `recipe.toml` schema via `./tests/validate.sh`
- [ ] 4.2 Verify all 7 P-items match spec scenarios (forbidden tools, board guard, explicit boardId, card validation, precondition, template cleanup, config schema)
