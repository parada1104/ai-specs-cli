# SDD Cycle: Card #78 — Fortalecer validación de board_id en trello-mcp-workflow

Eres un agente de coding trabajando en el proyecto ai-specs-cli. Tu tarea es implementar la card #78 del board "ai-specs-cli Roadmap" siguiendo el SDD cycle completo con OpenSpec.

## Contexto

La card #78 está en: https://trello.com/c/Qips0wuO/78-fortalecer-validación-de-boardid-en-trello-mcp-workflow-y-config-safety

Problema: La recipe `trello-mcp-workflow` acepta un shortLink de 8 caracteres como board_id en vez del ID real de 24 caracteres hex. El hook `validate-config` no valida formato. El `init.md` da un hint incorrecto. Y el `AGENTS.md` generado no incluye los config fields.

## Reglas estrictas

1. **Worktree**: ya estás dentro de `.worktrees/card78-boardid-validation/`. TODOS los cambios van acá. No toques `development`.
2. **SDD mode**: Usa `auto-artifacts` — genera proposal, specs, design, tasks, y **detente**. No hagas apply hasta que el humano lo autorice.
3. **No mergees a development**: Cuando llegue el momento, crea un PR vía `gh`. Nada de push directo.
4. **Explora libremente**: La card tiene mejoras priorizadas (A-F) pero tú debes explorar el problema completo. Puedes encontrar aristas que no están en la card. Documenta cualquier hallazgo adicional en los artifacts.

## Proceso

1. Corre `openspec new change "fortalecer-boardid-validation"` para iniciar el cambio.
2. Sigue el flujo de artifacts: proposal → specs → design → tasks.
3. Para cada artifact, usa `openspec instructions <artifact-id> --change "fortalecer-boardid-validation"` para obtener el template.
4. Cuando todos los artifacts pre-apply estén listos, **detente y muestra el estado**.
5. NO ejecutes apply sin autorización explícita.

## Referencias

- Worktree root: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card78-boardid-validation/`
- Repo root: `/Users/robert/proyectos/nnodes/ai-specs-cli/`
- Card context: validate-config hook (recipe-materialize.py line 310), ConfigField (recipe_schema.py line 67), init.md, agents-md-render.py, SKILL.md
