## 1. Implementation

- [x] 1.1 Modificar `build_recipe_mcp()` para shallow merge con manifest precedence
- [x] 1.2 Modificar `materialize_recipes()` para shallow merge con manifest precedence
- [x] 1.3 Emitir warnings en conflicto de keys

## 2. Testing

- [x] 2.1 Agregar test: proyecto con `[mcp.trello]` existente + recipe con `[mcp.trello]` → manifest keys se preservan
- [x] 2.2 Agregar test: proyecto sin `[mcp.trello]` + recipe con `[mcp.trello]` → recipe keys se crean
- [x] 2.3 Agregar test: warning emitido cuando hay conflicto
- [x] 2.4 Verificar `./tests/run.sh` pasa (186 tests OK)
- [x] 2.5 Verificar `./tests/validate.sh` pasa

## 3. Verification & Archive

- [ ] 3.1 Ejecutar verify contra specs
- [ ] 3.2 Archivar change
