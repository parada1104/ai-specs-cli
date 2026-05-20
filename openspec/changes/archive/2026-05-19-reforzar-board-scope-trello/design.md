# Design: Reforzar Board Scope en Trello MCP Workflow

## Technical Approach

Reinforce board isolation at the recipe-skill layer by adding guard rails, explicit parameters, and validation steps. The Trello MCP server retains broad API-key scope; the recipe constrains agent usage to a single configured `board_id`. Changes affect SKILL.md capability steps, recipe.toml schema, command reference, and card template.

---

## Architecture: Board Isolation per Capability

| Capability | Isolation Mechanism |
|---|---|
| `trello-session-bootstrap` | Forbidden-tools list + board guard post-set + config read |
| `trello-card-linking` | Board guard precondition (step 0) + explicit `boardId` on create + card `idBoard` validation |
| `trello-state-sync` | Board guard precondition (step 0) + explicit `boardId` on move + card `idBoard` validation |
| `trello-progress-comment` | Board guard precondition (step 0) + explicit `boardId` on comment + card `idBoard` validation |

---

## Board Guard Flow

```
Agent ──→ Read board_id from ai-specs.toml
   │
   ├──→ trello_set_active_board(board_id)
   │
   ├──→ trello_get_active_board_info()
   │         │
   │         └─→ id == board_id ?
   │              YES ──→ Guard pass, proceed
   │              NO  ──→ Warning to stderr + 1 retry
   │                        │
   │                        └─→ Retry pass ? proceed
   │                             Retry fail ? Log to warnings.log + skip Trello for session
```

Board guard is step 0 in all 4 capabilities, not only bootstrap.

---

## Forbidden Tools Enforcement

| Tool | Restriction | Rationale |
|---|---|---|
| `trello_get_my_cards` | **Forbidden** — never invoke | Returns cards across all boards; leaks scope |
| `trello_list_boards` | **Forbidden** — never invoke | Enumerates all accessible boards; leaks scope |
| `trello_set_active_board` | **Restricted** — bootstrap step 2 only | Required once per session; later calls bypass guard |

Enforcement is documented in SKILL.md as normative rules. The MCP server does not block these calls; the skill contract prohibits them.

---

## Explicit boardId Pattern

Every MCP call that accepts `boardId` MUST receive it explicitly:

```markdown
1. Read `board_id` from `[recipes.trello-mcp-workflow.config]`.
2. Call `trello_get_lists(boardId: <board_id>)`.
3. Call `trello_get_cards_by_list_id(listId: <id>, boardId: <board_id>)`.
4. Call `trello_add_card_to_list(listId: <id>, boardId: <board_id>, ...)`.
5. Call `trello_move_card(cardId: <id>, listId: <id>, boardId: <board_id>)`.
6. Call `trello_update_card_details(cardId: <id>, boardId: <board_id>, ...)`.
```

Eliminates reliance on implicit server-side active-board state.

---

## Card Validation Flow

Before `trello_get_card` or `trello_add_comment`:

```
Agent ──→ trello_get_card(cardId, fields="idBoard")
             │
             └─→ idBoard == configured board_id ?
                  YES ──→ Proceed with original operation
                  NO  ──→ Warning + abort operation
```

Validation is lightweight (single field fetch) and occurs per operation, not cached across the session.

---

## Config Schema

Add to `recipe.toml`:

```toml
[config.board_isolation]
forbidden_tools = ["trello_get_my_cards", "trello_list_boards"]
restricted_tools = ["trello_set_active_board"]
card_validation_required = true
```

| Field | Type | Description |
|---|---|---|
| `forbidden_tools` | list[string] | Tools the skill must never invoke |
| `restricted_tools` | list[string] | Tools allowed only in designated steps |
| `card_validation_required` | boolean | Whether to validate `idBoard` before card ops |

---

## File Changes

| File | Action | Description |
|---|---|---|
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` | Modify | Add Forbidden Tools section; add board guard steps; add explicit boardId to all MCP calls; add card validation rule; add board guard precondition to 4 capabilities |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | Modify | Append `[config.board_isolation]` block with 3 fields |
| `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md` | Modify | Add forbidden/restricted tools reference table |
| `catalog/recipes/trello-mcp-workflow/templates/card-feature.md` | Modify | Remove SDD Checklist section; add reference to `trello-pm-workflow` skill for SDD tracking |
| `ai-specs/skills/trello-pm-workflow/SKILL.md` | Modify | Add cross-reference to recipe board isolation rules; clarify that board/list config lives in recipe, not hardcoded here |

---

## Architecture Decisions

### ADR-1: Skill-Level Enforcement over MCP-Level Restriction

| Option | Tradeoff | Decision |
|---|---|---|
| Restrict MCP server config (env vars, proxy) | Requires server changes; breaks upgrade path | **Rejected** |
| Skill-level forbidden/restricted lists | Lightweight; recipe-governed; survives server upgrades | **Chosen** |

**Rationale**: The `@delorenj/mcp-server-trello` is third-party. Modifying it creates fork liability. Recipe-level rules are versioned with the project and enforceable via documentation and convention.

### ADR-2: Runtime Validation over Static Analysis

| Option | Tradeoff | Decision |
|---|---|---|
| Static analysis of agent prompt/tool use | Complex; false positives; not available in this stack | **Rejected** |
| Runtime board guard + card validation | Simple; explicit; fails safe | **Chosen** |

**Rationale**: The agent runtime invokes tools dynamically. Static analysis would require parsing agent reasoning traces. Runtime guards catch misuse at the point of occurrence.

### ADR-3: Graceful Degradation over Hard Abort

| Option | Tradeoff | Decision |
|---|---|---|
| Hard abort on board guard failure | Blocks productive sessions on transient Trello API issues | **Rejected** |
| Warning + skip Trello, continue session | Preserves agent productivity; logs audit trail | **Chosen** |

**Rationale**: Trello is operational tracking, not execution-critical. A board mismatch or API hiccup should not stop code implementation.

---

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | `recipe.toml` schema validity | `./tests/validate.sh` parses TOML; no custom tests needed |
| Integration | Board guard logic against mock responses | Not available in current test infra; validate manually via MCP dry-run |
| E2E | Full bootstrap flow with real board ID | Manual: run bootstrap capability, verify guard passes and forbidden tools are not invoked |

---

## Migration / Rollout

No data migration required. Rollout steps:

1. Merge modified files into `development`.
2. Run `ai-specs sync` to regenerate derived artifacts (when TOML schema supports it; currently manual per runtime brief).
3. Verify `[config.board_isolation]` appears in generated recipe docs.
4. Existing cards and board state are unaffected.

Rollback: revert commits and remove `[config.board_isolation]` block.

---

## Open Questions

- [ ] Should `trello_set_active_board` be wrapped in a retry loop for network flakes, or is the board guard retry sufficient?
- [ ] Does the Trello MCP server accept `boardId` on all listed tools, or do some ignore the parameter? (Verify before apply.)
