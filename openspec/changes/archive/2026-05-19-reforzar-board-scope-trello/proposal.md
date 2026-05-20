# Proposal: Reforzar Board Scope en Trello MCP Workflow

## Intent

El Trello MCP Workflow recipe carece de aislamiento de board. La API key del usuario tiene scope amplio (accede cualquier board), pero el agente debe confinarse al `board_id` configurado en `ai-specs.toml`. Actualmente `trello_get_my_cards` retorna cards de todos los boards, `trello_list_boards` enumera boards accesibles, y `trello_set_active_board` puede cambiar el board activo sin validación. Esto expone datos de otros proyectos al agente.

## Scope

### In Scope

- **P1 - Forbidden Tools (CRITICAL)**: Prohibir `trello_get_my_cards`, `trello_list_boards`; restringir `trello_set_active_board` solo al bootstrap.
- **P2 - Board Guard (HIGH)**: Verificar post-bootstrap que el board activo (`trello_get_active_board_info`) coincide con el `board_id` configurado. Si no: warning + retry; si persiste, abortar operaciones Trello.
- **P3 - Explicit boardId (HIGH)**: Pasar `boardId` explícito en `trello_get_lists`, `trello_get_cards_by_list_id`, `trello_add_card_to_list`, `trello_move_card`, `trello_update_card_details`.
- **P4 - Card Validation (MEDIUM)**: Validar `idBoard` de la card antes de `trello_get_card` y `trello_add_comment`.
- **P5 - Board Guard Precondition (MEDIUM)**: Board guard como step 0 en las 4 capabilities, no solo bootstrap.
- **P6 - Template Cleanup (LOW)**: Eliminar sección "SDD Checklist" de `card-feature.md`.
- **P7 - Config Schema (LOW)**: Agregar `[config.board_isolation]` en `recipe.toml`.

### Out of Scope

- Restricción de scope de API key (nivel Trello, no recipe).
- Soporte multi-board.
- Audit logging más allá del `warnings.log` existente.
- N1 comment en `openspec/config.yaml` sobre `[sdd]` removal (difiere a cambio separado).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

| Capability | Changes |
|---|---|
| `trello-session-bootstrap` | Board guard post-set_active_board, forbidden tools enforcement, `board_isolation` config read |
| `trello-card-linking` | Explicit boardId, card idBoard validation, board guard precondition (step 0) |
| `trello-state-sync` | Explicit boardId, card idBoard validation, board guard precondition (step 0) |
| `trello-progress-comment` | Explicit boardId, card idBoard validation, board guard precondition (step 0) |

## Approach

**P1-P2**: SKILL.md gana sección "Forbidden Tools" con lista explícita de tools prohibidas y restringidas. Bootstrap step 2 (post `set_active_board`) agrega verificación con `trello_get_active_board_info`: comparar `id` retornado con `board_id` configurado; warning + un retry; si persiste, log a `warnings.log` y abortar operaciones Trello.

**P3**: Leer `board_id` de `ai-specs.toml` una vez al inicio de cada capability. Pasarlo explícito en toda llamada MCP que acepte el parámetro. Eliminar dependencia en estado global implícito del MCP server.

**P4-P5**: Antes de `trello_get_card`/`trello_add_comment`, obtener `idBoard` del card (vía `trello_get_card` con `fields=idBoard`), validar match contra `board_id` configurado. Board guard como precondition step 0 en las 4 capabilities.

**P6-P7**: Template `card-feature.md`: reemplazar "SDD Checklist" con referencia al project skill `trello-pm-workflow`. `recipe.toml`: nuevo `[config.board_isolation]` con `forbidden_tools: [trello_get_my_cards, trello_list_boards]`, `restricted_tools: [trello_set_active_board]`, `card_validation_required: true`.

## Affected Files

| File | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` | Modified | Forbidden tools section, board guard steps, explicit boardId, card validation rules |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | Modified | Add `[config.board_isolation]` block with 3 fields |
| `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md` | Modified | Forbidden tools reference table |
| `catalog/recipes/trello-mcp-workflow/templates/card-feature.md` | Modified | Remove SDD Checklist, add reference to trello-pm-workflow |
| `ai-specs/skills/trello-pm-workflow/SKILL.md` | Modified | Reference/link to recipe board isolation rules |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Board guard failures bloquean sesiones productivas | Low | Graceful degradation: warning + skip Trello, no abort de sesión |
| boardId mismatch en cards creadas antes del cambio | Low | P4 valida al momento de operación; no requiere migración retroactiva |
| Cambios de API en Trello MCP server rompen validaciones | Low | Schema versionado en recipe.toml; `board_isolation` deshabilitable vía config |

## Rollback Plan

1. Revertir commits en `catalog/recipes/trello-mcp-workflow/`.
2. Remover bloque `[config.board_isolation]` de `recipe.toml`.
3. Ejecutar `ai-specs sync` para regenerar artefactos derivados.
4. Sin migración de datos requerida — solo reglas de runtime.

## Dependencies

- Trello MCP server (`@delorenj/mcp-server-trello`) — ya configurado en recipe.toml.
- `board_id` válido en `ai-specs.toml` — ya requerido por el recipe.

## Success Criteria

- [ ] `trello_get_my_cards` y `trello_list_boards` no son invocadas por ninguna capability del recipe
- [ ] `trello_set_active_board` solo se llama en bootstrap step 2, con verificación posterior
- [ ] Toda llamada MCP que acepta `boardId` lo recibe explícitamente (5 tools)
- [ ] `trello_get_card` / `trello_add_comment` validan `idBoard` antes de operar
- [ ] Board guard presente como step 0 en las 4 capabilities
- [ ] Template `card-feature.md` sin sección "SDD Checklist"
- [ ] `recipe.toml` incluye `[config.board_isolation]` con `forbidden_tools`, `restricted_tools`, `card_validation_required`
