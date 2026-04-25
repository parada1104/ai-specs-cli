## Why

`ai-specs` ya combina contexto desde documentación canónica, skills, packs, handoffs, memoria de sesión y propuestas del agente, pero no existe una regla auditable para resolver contradicciones entre esas fuentes. Definir una precedence MVP ahora reduce ambigüedad operativa antes de introducir `doctor`, memoria Markdown y handoffs más estructurados.

## What Changes

- Define the MVP context precedence order: canonical docs > project skills > packs > handoffs > session memory > proposed context.
- Adds a canonical document at `docs/ai/context-precedence.md` explaining the source classes, conflict-resolution rule, and concrete conflict examples.
- References the rule from generated agent-facing documentation so agents can discover it during normal project setup.
- Keeps the rule intentionally documentation-first and auditable; no runtime merge engine, no new manifest section, and no behavioral change to existing sync logic.
- No breaking changes.

## Capabilities

### New Capabilities
- `context-precedence`: Defines the canonical ordering and conflict-resolution expectations for project context sources.

### Modified Capabilities
- None.

## Impact

- Affected documentation: `docs/ai/context-precedence.md`, `README.md`, and the generated-agent documentation source that renders `AGENTS.md`.
- Affected generated artifacts: `AGENTS.md` may change after running `ai-specs sync` because it is generated and must reference the precedence rule indirectly through source templates/rendering inputs.
- Affected tests: documentation/rendering tests should prove the precedence order and discoverability reference remain aligned.
- Rollback plan: remove the new docs/reference and revert any generated documentation updates; because this change is documentation-first and does not alter runtime merge behavior, rollback should not require data or manifest migration.
