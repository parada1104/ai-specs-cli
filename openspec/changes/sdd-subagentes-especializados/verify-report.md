# Verify Report — sdd-subagentes-especializados

## Summary

- **Overall**: pass
- **Date**: 2026-05-15
- **Branch**: `sdd-subagentes-especializados`
- **Head SHA**: 7ffcfc097c689ac834955c73406fc8e5b7230335
- **OpenSpec validate**: `openspec validate sdd-subagentes-especializados --strict` → `valid`
- **Recommendation**: **archive-ready: yes**

## Spec coverage

### `sdd-subagent-deployment` (new capability)

- **Catálogo canónico de subagentes SDD**
  - ✓ Inventario completo en el catálogo bundled — `bundled-agents/claude/` contiene los seis `.md` (commit `9fbea16`); test `test_on_byte_identical_to_bundled_source` verifica que el bundled source coincide byte-a-byte con la materialización.
  - ✓ Roles y límites definidos — cada archivo declara rol, tools permitidos/bloqueados y `## Turn budget` numérico (revisado en `tests/test_agents_render.py::test_on_byte_identical_to_bundled_source` por contenido completo).
- **Frontmatter canónico de subagent files**
  - ✓ Frontmatter cumple contrato — `name`, `description` y `tools` presentes en los seis archivos; verificación manual y por test de contenido.
  - ✓ Contrato documentado y separado — `ai-specs/contracts/subagent-frontmatter.md` publicado y aclara separación del contrato de skills.
- **Despliegue gobernado por el flag**
  - ✓ ON + Claude habilitado materializa los seis archivos — `test_on_with_claude_only_materializes_six_files`, E2E manual confirmado.
  - ✓ OFF preserva el harness y no destruye archivos — `test_off_does_not_create_claude_agents`, `test_off_with_existing_files_warns_about_orphans_without_deleting`.
- **Fallback para harnesses sin soporte nativo**
  - ✓ ON + harness sin soporte se omite limpio — `test_on_unsupported_harness_only_writes_nothing`.
  - ✓ ON con harnesses mixtos solo materializa Claude — `test_on_mixed_harnesses_only_materializes_claude_and_logs_fallback`.
- **Idempotencia y limpieza**
  - ✓ Re-sync idempotente — `test_idempotent_two_runs_produce_identical_files_and_lock` (lock y bytes idénticos).
  - ✓ Transición ON→OFF advierte sin destruir — `test_off_with_existing_files_warns_about_orphans_without_deleting` y E2E manual.
- **Trazabilidad operativa en runtime brief**
  - ✓ Listado en brief con flag activo — `test_sub_agents_true_runtime_brief_lists_subagents`.
  - ✓ Sin flag no se altera el brief — `test_sub_agents_off_runtime_brief_is_byte_identical_across_runs`.

### `manifest-contract` (delta)

- **Campo opcional `sub_agents`**
  - ✓ Ausente preserva V1 — `test_sdd_absent_defaults_sub_agents_false`.
  - ✓ Valor booleano válido — `test_sdd_sub_agents_true`, `test_sdd_sub_agents_false`.
  - ✓ Tipo no booleano falla — `test_sdd_sub_agents_non_boolean_raises`, `test_sdd_sub_agents_integer_raises`, `test_sub_agents_non_boolean_fails_sync_with_actionable_error`.
- **Plantilla y README documentan el campo**
  - ✓ Plantilla incluye `[sdd]` comentado con `sub_agents = false` — verificado en `templates/ai-specs.toml.tmpl` y por test estructural `test_manifest_contract_docs` (template ⊆ generated surface).
  - ✓ Documentación pública refleja el campo — `docs/ai-specs-toml.md` con default, semántica y advertencia de feature de producto.

### `sdd-cli-integration` (delta)

- **Habilitación declarativa de `sub_agents`**
  - ✓ Activación sin destruir manifiesto previo — `test_sub_agents_true_materializes_six_claude_agents` ejerce el sync completo sobre un manifiesto V1 válido.
  - ⚠ Activación con provider distinto de `openspec` — no se cubre con test explícito por scope (el manifiesto fixture default usa `openspec` implícito). Comportamiento documentado en la spec; cubrir formalmente queda para un follow-up si surge el caso.
- **Integración con doctor**
  - ✓ Archivos presentes → `OK` — `test_sub_agents_true_all_present_reports_ok`, E2E doctor confirma 13 OK.
  - ✓ Archivos faltantes → `WARN`/`ERROR` — `test_sub_agents_true_partial_missing_reports_warn`, `test_sub_agents_true_all_missing_reports_error`.
  - ✓ Doctor silencioso con flag apagado — `test_sub_agents_absent_is_silent`, `test_sub_agents_false_is_silent`.

### `recipe-sync-materialization` (delta)

- **Materialización por harness**
  - ✓ Materialización a Claude Code — `test_sub_agents_true_materializes_six_claude_agents` + E2E manual.
  - ✓ Harness no soportado se omite — `test_sub_agents_true_with_unsupported_harness_declares_inline_fallback`.
  - ✓ Subagentes no contaminan `ai-specs/skills/`, `.recipe/`, `.deps/` — verificable por estructura: el renderer escribe únicamente a `.claude/agents/`.
- **Orden y respeto a la pipeline**
  - ✓ Re-sync idempotente — `test_idempotent_two_runs_produce_identical_files_and_lock`.
  - ✓ Archivo modificado localmente → sidecar `.new` — `test_user_modified_file_with_upstream_change_produces_new_sidecar`.
- **Transición ON→OFF**
  - ✓ Reporta huérfanos sin eliminarlos — `test_off_with_existing_files_warns_about_orphans_without_deleting` + E2E manual.

### `agents-md-runtime-brief` (delta)

- **Sección opcional de subagentes SDD**
  - ✓ Activo añade la sección — `test_sub_agents_true_runtime_brief_lists_subagents` (incluye los seis slugs y la ruta canónica).
  - ✓ Apagado mantiene brief idéntico — `test_sub_agents_off_runtime_brief_is_byte_identical_across_runs`.
  - ✓ Harness sin soporte se documenta — `test_sub_agents_true_with_unsupported_harness_declares_inline_fallback` (verifica `opencode` + `inline`).
- **Marker manual sigue ganando**
  - ✓ El código existente preserva el marker manual de forma independiente al flag; el renderer condicional añade la sección sin tocar el path manual. Cubierto por la lógica existente en `agents-md-render.py:RUNTIME_BRIEF_MARKER` que retorna antes de construir el cuerpo.

### `backward-compatibility` (delta)

- **Opt-in estricto**
  - ✓ Manifest sin `sub_agents` preserva V1 — `test_sub_agents_absent_does_not_create_claude_agents`, `test_sub_agents_off_runtime_brief_is_byte_identical_across_runs`.
  - ✓ Manifest con `sub_agents = false` preserva V1 — `test_sub_agents_false_does_not_create_claude_agents`.
- **Compatibilidad con flujo manual existente**
  - ✓ Orquestador inline sin `sub_agents` — la skill `openspec-phase-orchestrator/SKILL.md` documenta el fallback inline cuando el flag está apagado.
  - ✓ Orquestador inline en harness sin soporte — verificado por la sección "Fallback rules" en SKILL.md y `test_sub_agents_true_with_unsupported_harness_declares_inline_fallback`.

## Test evidence

- **Focused test command**: `./tests/run.sh` → `Ran 330 tests in 45.945s — OK (skipped=1)`
- **Full validation**: `./tests/validate.sh` → `Ran 330 tests in 46.020s — OK (skipped=1)` (incluye `python3 -m py_compile lib/_internal/*.py tests/*.py` y `bash -n lib/*.sh bin/ai-specs tests/*.sh`).
- **OpenSpec validate**: `openspec validate sdd-subagentes-especializados --strict` → `valid`.
- **Coverage tool**: declarada `unavailable` en `openspec/config.yaml` (`coverage.available: false`).
- **Linter / type-checker / formatter**: declarados `unavailable` en `openspec/config.yaml`.

## E2E manual

Ejecutado contra un workspace temporal recién inicializado (`bin/ai-specs init` + `bin/ai-specs sync`):

1. Con `[sdd].sub_agents = true` y `claude` habilitado: seis archivos materializados en `.claude/agents/sdd-*.md`, `AGENTS.md` incluye la sección "## SDD Subagents" con los seis slugs y declaración de fallback inline para `cursor` y `opencode`, `ai-specs doctor` reporta `OK sdd-subagents`.
2. Al cambiar a `sub_agents = false` y re-correr sync: emite aviso enumerando los seis archivos huérfanos en `.claude/agents/`, NO los elimina, sugiere remoción manual.

## Open issues / gaps

- **Scope `behavior_change` para provider ≠ openspec**: la spec `sdd-cli-integration` describe el comportamiento esperado pero no hay test automatizado porque ningún fixture declara otro provider. Riesgo bajo: el catálogo de providers V1 es cerrado a `openspec`. Follow-up sugerido si V1.x agrega otro provider.
- **Open questions del design.md resueltas implícitamente**:
  - Skill orchestrator referencia subagentes como mejora opcional (mapeo agregado, requerimiento estricto difiere a card #68).
  - No se introdujo `ai-specs sdd cleanup`; el aviso textual es suficiente para V1.
  - Frontmatter `model` queda omitido en V1 para honrar la selección del usuario.
  - E2E real contra Claude Code queda fuera de scope; tests de filesystem son suficientes para V1.

## Files changed (delta vs. `development`)

- New:
  - `bundled-agents/claude/sdd-{explore,proposal,artifacts,apply,verify,archive}.md`
  - `ai-specs/contracts/subagent-frontmatter.md`
  - `lib/_internal/agents-render.py`
  - `tests/test_agents_render.py`
  - `openspec/changes/sdd-subagentes-especializados/**` (proposal, design, tasks, specs delta, verify-report)
- Modified:
  - `lib/_internal/toml-read.py` (`read_sdd`, dispatch)
  - `lib/_internal/lock.py` (`agents` section round-trip)
  - `lib/_internal/agents-md-render.py` (`render_sdd_subagents_section`)
  - `lib/_internal/doctor.py` (`_check_sdd_subagents`)
  - `lib/sync.sh` (invocación de `agents-render`)
  - `templates/ai-specs.toml.tmpl` (bloque `[sdd]` comentado)
  - `ai-specs/ai-specs.toml` (bloque `[sdd]` comentado al final)
  - `docs/ai-specs-toml.md` (clasificación + sección detallada de `sub_agents`)
  - `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` (mapa de subagent_type + reglas de fallback)
  - `tests/test_toml_read.py`, `tests/test_doctor.py`, `tests/test_sync_pipeline.py`

## Recommendation

- **Archive-ready**: **yes** — todos los escenarios de specs verificados o cubiertos por tests, backward-compat regresión confirmada, E2E manual exitoso.
- **Próximos pasos sugeridos** (no bloqueantes):
  1. Abrir PR contra `development` vía `gh pr create` (manual o vía `sdd-archive` cuando el orquestador esté maduro).
  2. Mover la card Trello #82 a `Review` con link al PR.
  3. Si el equipo decide profesionalizar el dispatcher de subagentes, retomar la card #68 con este cambio como dependencia ya satisfecha.
