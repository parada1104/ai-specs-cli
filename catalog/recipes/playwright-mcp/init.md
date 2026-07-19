# Recipe Init Contract — playwright-mcp

> Add-on exploratory surface. Prefer enabling `playwright-ui-flow` first for
> suite/smoke discipline. This init configures the Playwright MCP preset.

## Preguntas

### browsers_installed

- **Required**: no
- **Type**: bool
- **Pregunta**: "¿Ya corriste `npx playwright install` en esta máquina/CI?"
- **Hint**: Si no, proponlo como paso manual post-sync.

### headed_override

- **Required**: no
- **Type**: string
- **Pregunta**: "¿Quieres override de browser/headless en `[mcp.playwright]`? (vacío = preset headless del recipe)"
- **Hint**: Ejemplo de args: `["-y", "@playwright/mcp@latest", "--browser", "chromium"]`

## MCP Discovery

- Si `[mcp.playwright]` no existe, el sync materializa el preset del recipe
  (`npx -y @playwright/mcp@latest --headless`).
- Si el usuario pide overrides, propón un bloque reviewable bajo
  `[mcp.playwright]` (sin secretos literales).

## TOML Target

```toml
[recipes.playwright-mcp]
enabled = true

# Opcional — solo si hay override:
# [mcp.playwright]
# args = ["-y", "@playwright/mcp@latest", "--browser", "chromium"]
```

## Post-write

- Recordar: `ai-specs sync`.
- Documentar que este recipe **aumenta** a `playwright-ui-flow`; por sí solo no
  constituye la topología soportada para la disciplina completa de smokes.
