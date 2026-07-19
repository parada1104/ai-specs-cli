# Recipe Init Contract — playwright-ui-flow

> Follow this script to configure the recipe. Ask each block, validate the
> answer, and propose a reviewable diff to `ai-specs/ai-specs.toml`.

## Preguntas

### ui_test_command

- **Required**: no
- **Type**: string
- **Pregunta**: "¿Cuál es el comando de la suite UI de Playwright? (ej. `npx playwright test` o `pnpm test:e2e`)"
- **Hint**: Revisa `package.json` scripts y `playwright.config.*`. Deja vacío si aún no hay suite.

### ui_smoke_command

- **Required**: no
- **Type**: string
- **Pregunta**: "¿Cuál es el comando de smoke UI rápido? (ej. `npx playwright test --grep @smoke`)"
- **Hint**: Convención sugerida: etiquetar smokes con `@smoke` y filtrar con `--grep @smoke`.

### playwright_config

- **Required**: no
- **Type**: string
- **Pregunta**: "¿Ruta a `playwright.config.*` si no es la estándar? (vacío = default del proyecto)"

## Discovery

1. Busca `playwright.config.ts|js|mjs` en la raíz o `e2e/` / `tests/`.
2. Busca scripts en `package.json` que mencionen `playwright`.
3. Propón valores; no escribas secretos.

## TOML Target

```toml
[recipes.playwright-ui-flow]
enabled = true

[recipes.playwright-ui-flow.config]
ui_test_command = "<answer:ui_test_command>"   # omitir si vacío
ui_smoke_command = "<answer:ui_smoke_command>" # omitir si vacío
playwright_config = "<answer:playwright_config>" # omitir si vacío
```

## Post-write

- Recordar: `ai-specs sync` para materializar skills/commands/docs.
- No invocar `sync` automáticamente; requiere review humano.
- Para exploración interactiva, considerar también `playwright-mcp`.
