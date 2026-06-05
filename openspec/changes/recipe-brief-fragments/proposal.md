## Why

Hoy `AGENTS.md` se genera (`lib/_internal/agents-render.py`) desde dos fuentes: valores estructurados de `resolved-config.json` (ya derivados de las recipes) y prosa de la tabla `[brief]` del manifest. La prosa es **100% hand-written**. Un proyecto que adopta ai-specs-cli y corre `ai-specs init` + `sync` obtiene un `AGENTS.md` casi vacío: solo campos estructurados y MCPs, sin guía de comportamiento.

La riqueza del `AGENTS.md` de ai-specs-cli (≈35 líneas de prosa `[brief]`) existe solo porque un humano la escribió. La clave: esa prosa es **derivable de las recipes habilitadas**. `worktree-flow` implica reglas de worktree; `trello-mcp-workflow` implica "Trello es la fuente de verdad"; `tdd-flow` implica "corre el test command antes de verificar"; `vault-canonical-store` implica "Vault controla decisiones canónicas". El gap: las recipes todavía no declaran la prosa de comportamiento que los agentes necesitan.

Objetivo: hacer el brief **rico por defecto y genérico**, haciendo que cada recipe contribuya fragmentos de brief. La tabla `[brief]` del manifest se reduce a la voz del proyecto (intro/purpose) más añadidos opcionales.

## What Changes

- **Nuevo slot `[provides.brief]` en `recipe.toml`**: cada recipe declara fragmentos de prosa por sección. Dos formas soportadas — array simple de strings y array de inline-tables con `{key, text}` (ver Approach).
- **El renderer fusiona fragmentos de recipes habilitadas + añadidos del manifest `[brief]`**: por cada sección contribuible, se emiten primero los fragmentos de recipes (en orden de declaración del manifest, deduplicados) y luego los bullets del manifest (modelo APPEND).
- **El manifest `[brief]` se reduce a la voz del proyecto**: `intro` y `purpose` siguen siendo project-only; las secciones contribuibles pasan a ser opcionales (augment/override).
- **Sustitución `{config.KEY}` capability-aware** en fragmentos de recipe, best-effort, con regla de escape de llaves (`{{`/`}}`).
- **Opt-in REPLACE** por sección vía `<section>_mode = "replace"` en `[brief]` para suprimir fragmentos de recipe.
- **Scaffold de `ai-specs.toml.tmpl`** reduce `[brief]` a `intro` + `purpose` con un comentario explicativo.

Capas/archivos tocados:
- `lib/_internal/recipe_schema.py` — parser de `[provides.brief]`.
- `lib/_internal/recipe-materialize.py` — incluir `brief_fragments` por recipe en `resolved-config.json`.
- `lib/_internal/agents-render.py` — colección, dedupe, merge order, modo append/replace, sustitución.
- `catalog/recipes/<id>/recipe.toml` — añadir bloques `[provides.brief]`.
- Contrato `resolved-config.json` — delta de schema (`recipes.<id>.brief_fragments`).
- `templates/` (scaffold del manifest) — `[brief]` reducido.

## Approach (decisiones cerradas)

1. **Schema (Decisión 1)**: inline TOML bajo `[provides.brief]`. Sin segundo read path; `recipe_schema.py` ya parsea `recipe.toml`. Dos formas, normalizadas internamente a `{key: str|None, text: str}`:
   ```toml
   # Forma simple (array de strings):
   [provides.brief]
   workflow_rules = ["Create a dedicated worktree...", "Do not push directly..."]

   # Forma con key (array de inline-tables, para dedupe semántico):
   [[provides.brief.context_sources]]
   key = "trello-source-of-truth"
   text = "Trello is the source of truth for work state and dependencies."
   ```

2. **Secciones contribuibles (Decisión 2)**: `runtime_flow`, `context_sources`, `conflict_policy`, `workflow_rules`, `useful_commands`, `mcp_descriptions` (default para el MCP propio de la recipe). `intro` y `purpose` quedan **project-only**.

3. **Merge order (Decisión 3)**: orden de declaración de recipes en el manifest, iterando la lista `enabled` de `resolved-config.json` (dicts de Python preservan orden de inserción). Determinista y controlado por el autor del manifest.

4. **Dedupe (Decisión 4)**: dedupe por string exacto + `key` opcional para dedupe semántico. Primera ocurrencia de un `key` gana; duplicados posteriores se descartan en silencio.

5. **Precedencia recipe vs. manifest (Decisión 5)**: **APPEND por defecto** (fragmentos de recipe primero, luego añadidos del manifest). **REPLACE opt-in** vía `<section>_mode = "replace"` en `[brief]` (sibling key, sin cambio estructural de TOML). El renderer chequea `brief.get("<section>_mode", "append")` por sección.

6. **Capability-aware substitution (Decisión 7)**: `str.format_map` con namespace explícito `{config.KEY}` (no `{KEY}` desnudo). Best-effort: si falta una key, se deja el placeholder tal cual (no crashea el render). Regla de escape: `{{`/`}}` para llaves literales en fragmentos. La prosa del manifest `[brief]`, `intro` y `purpose` **nunca** se sustituyen (pueden contener `{` en backticks de código).

7. **Backward-compat / migración (Decisión 6)**: el marcador `<!-- ai-specs:runtime-brief -->` sigue protegiendo briefs hand-managed (regeneración suprimida). Para `AGENTS.md` generados sin marcador, los bullets del manifest se appendean tras los fragmentos de recipe — sin pérdida de datos, solo posible duplicación que requiere limpieza. Recipes sin `[provides.brief]` siguen funcionando (sin fragmentos) → no se necesita bump de versión del catálogo.

### Flagged alternatives (donde divergiría de la exploración)

- Ninguna divergencia material. La exploración resolvió las 8 decisiones de forma coherente y se adoptan tal cual. Las únicas aristas sin cerrar son las Open Questions (abajo), que se trasladan a la fase de specs/design en lugar de decidirse silenciosamente aquí.

## Before / After

**Recipe (`catalog/recipes/worktree-flow/recipe.toml`)** — añadir:
```toml
[provides.brief]
workflow_rules = [
  "Create a dedicated worktree for changes that write artifacts or modify code.",
  "Do not push directly to `{config.integration_branch}` without a PR.",
]
runtime_flow = [
  "Artifact phases run in a dedicated worktree when they write files.",
]
```

**Manifest del adoptante** (`[brief]` VACÍO — solo voz de proyecto):
```toml
[brief]
intro = "Demo service for orders."
purpose = "Process and reconcile order events."
```

**AGENTS.md generado (sección `## Workflow Rules`)** — antes vs. después:
```diff
  ## Workflow Rules

- (vacío — no había [brief].workflow_rules hand-written)
+ - Create a dedicated worktree for changes that write artifacts or modify code.
+ - Do not push directly to `main` without a PR.
```
(`{config.integration_branch}` se sustituyó por el valor real `main` del config mergeado de la recipe; con `[brief].workflow_rules` vacío, no hay añadidos del manifest que appendear.)

## Out of Scope

- **Enhancer LLM `ai-specs sync --rich`** para generar `intro`/`purpose` desde README/package.json — futuro, puramente aditivo, no es dependencia de esta feature.
- **Herencia de fragmentos vía grafo de dependencias de recipes** — los fragmentos son per-recipe; la herencia añade complejidad innecesaria. Cada recipe declara sus propios fragmentos.

## Capabilities

### New Capabilities

- `recipe-brief-fragments`: contrato del slot `[provides.brief]` en `recipe.toml` — formas soportadas (array simple / inline-table con key), secciones contribuibles, normalización interna `{key, text}`, y semántica de dedupe.

### Modified Capabilities

- `runtime-brief-rendering`: el renderer pasa a fusionar fragmentos de recipes habilitadas con la prosa del manifest — merge order (orden de `enabled`), dedupe, modo append/replace por sección, y sustitución `{config.KEY}` best-effort con escape de llaves.
- `recipe-schema`: el schema de recipe reconoce `[provides.brief]` como slot opcional con dos formas válidas, normalizadas a `{key, text}`.
- `recipe-manifest-contract`: `[brief]` se reduce a voz de proyecto (`intro`/`purpose`); las secciones contribuibles son opcionales con semántica de override (`<section>_mode = "replace"`).

## Impact

- **Specs (delta targets)**:
  - `openspec/specs/runtime-brief-rendering/spec.md` (modified) — merge de fragmentos, orden, dedupe, append/replace, sustitución.
  - `openspec/specs/recipe-schema/spec.md` (modified) — slot `[provides.brief]` y formas válidas.
  - `openspec/specs/recipe-manifest-contract/spec.md` (modified) — `[brief]` reducido y modos override.
  - Nuevo `openspec/specs/recipe-brief-fragments/spec.md` (capability del contrato de fragmentos).
- **Code**:
  - `lib/_internal/recipe_schema.py` — `BriefFragment`/`BriefFragments` dataclasses, `_parse_brief_fragments()`, campo en `Recipe`.
  - `lib/_internal/recipe-materialize.py` — `brief_fragments` por recipe en `build_resolved_config()`.
  - `lib/_internal/agents-render.py` — `collect_recipe_brief_fragments()`, merge/dedupe/modo, sustitución por sección.
- **Catalog**: `catalog/recipes/*/recipe.toml` — añadir `[provides.brief]` a worktree-flow, git-pr-flow, tdd-flow, trello-mcp-workflow, vault-canonical-store, session-context (según aplique).
- **Contrato `resolved-config.json`**: delta `recipes.<id>.brief_fragments: {section → [{key, text}, ...]}`.
- **Template**: scaffold de `ai-specs.toml` reduce `[brief]` a `intro` + `purpose` con comentario explicativo.
- **Tests**: archivo dedicado `tests/test_agents_render_brief_fragments.py` (evitar inflar `test_sync_pipeline.py`, ya >1900 líneas); fragmentos de schema en `tests/test_recipe_schema.py` o archivo nuevo.

## Risks & Migration

1. **Duplicación para adoptantes con `[brief]` completo** (como el propio ai-specs-cli): los fragmentos de recipe se prepondrán a los bullets existentes. Mitigación: (a) marcador `<!-- ai-specs:runtime-brief -->` protege briefs hand-managed; (b) `<section>_mode = "replace"`; (c) limpieza de bullets ahora redundantes. ai-specs-cli ya usa el marcador en su CLAUDE.md (symlink), así que su brief humano queda protegido y el `AGENTS.md` generado se enriquece automáticamente.
2. **Genericidad de fragmentos**: deben ser verdaderos para TODOS los proyectos que usan la recipe. "Do not push to `development`" es específico; "Do not push to `{config.integration_branch}`" es genérico. Carga sobre los autores de recipes.
3. **Crecimiento de `resolved-config.json`**: ~30-40 entries de fragmento → JSON despreciable.
4. **Superficie de tests**: aislar en archivo dedicado para no inflar `test_sync_pipeline.py`.
5. **Sustitución sobre bullets con código**: bullets con backticks (`` `./tests/run.sh` ``) o expansión de llaves pueden chocar con `format_map`. Mitigación: regla de escape `{{`/`}}` (ver Open Question 2).

## Acceptance Signals

- `./tests/run.sh` verde, incluyendo nuevos tests de fragmentos (schema parse de ambas formas, merge order por `enabled`, dedupe por key y por string exacto, modo append/replace, sustitución `{config.KEY}` y escape de llaves).
- `./tests/validate.sh` verde.
- Un sync dry-run sobre un manifest con `[brief]` vacío (solo intro/purpose) y recipes habilitadas produce un `AGENTS.md` con secciones `## Workflow Rules`, `## Context Sources`, `## Conflict Policy`, `## Runtime Flow` pobladas por fragmentos de recipe, con `{config.integration_branch}` / `{config.test_command}` resueltos a valores reales.
- Recipes sin `[provides.brief]` no rompen el render (sin fragmentos).
- Un manifest con `<!-- ai-specs:runtime-brief -->` no se regenera (escape hatch intacto).

## Open Questions (for human / specs phase)

1. **Default de `mcp_descriptions` por recipe**: ¿una recipe provee la descripción de su propio MCP, overridable por `[brief.mcp_descriptions]`? Falta decidir si `[brief.mcp_descriptions]` mantiene precedencia total o solo las entries project-specific overridean.
2. **Política de escape para bullets con código**: confirmar `{{`/`}}` como regla de escape de llaves literales en fragmentos que también contienen `{config.KEY}`.
3. **Inheritance vía dependencias de recipes**: recomendado NO (fragmentos per-recipe). Confirmar para cerrar.

## Rollback

Reversible por archivos:
- Revertir `recipe_schema.py`, `recipe-materialize.py`, `agents-render.py` desde git.
- Quitar bloques `[provides.brief]` de `catalog/recipes/*/recipe.toml`.
- Revertir el delta de `resolved-config.json` (campo opcional; ausencia = sin fragmentos).
- Revertir specs modificadas y eliminar la nueva spec `recipe-brief-fragments`.
- Sin migración de datos: proyectos ya sincronizados con marcador no se ven afectados; los demás simplemente vuelven al `AGENTS.md` previo en el próximo sync.

## Next Phases

- **specs** → escribir delta specs en los 3 targets modificados + la nueva capability `recipe-brief-fragments` (resolver Open Questions 1-3 ahí).
- **design** → forma exacta de dataclasses, algoritmo de dedupe/merge, contrato del campo `brief_fragments` en `resolved-config.json`, regla de escape.
- **tasks** → breakdown TDD (strict_tdd=true): tests primero por capa.

(NO ejecutar specs/design/tasks/code en esta ronda.)
