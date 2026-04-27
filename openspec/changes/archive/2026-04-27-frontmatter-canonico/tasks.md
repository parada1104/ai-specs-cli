## 1. Estructura

- [x] 1.1 Crear directorio `ai-specs/docs/` si no existe
- [x] 1.2 Verificar que `.gitignore` no excluye `ai-specs/docs/`

## 2. Documento de referencia

- [x] 2.1 Crear `ai-specs/docs/canonical-memory-frontmatter.md`
- [x] 2.2 Documentar campos mínimos obligatorios (`type`, `scope`, `status`, `tags`, `source`, `updated_at`)
- [x] 2.3 Documentar valores válidos para `type`: `decision`, `handoff`, `spec`, `context`, `proposed`
- [x] 2.4 Documentar valores válidos para `scope`: `project`, `team`, `system`, `session`
- [x] 2.5 Documentar valores válidos para `status`: `proposed`, `accepted`, `deprecated`
- [x] 2.6 Documentar campo opcional `confidence` (float 0.0–1.0)
- [x] 2.7 Incluir ejemplo de frontmatter para documentación canónica
- [x] 2.8 Incluir ejemplo de frontmatter para handoff
- [x] 2.9 Incluir ejemplo de frontmatter para proposed memory
- [x] 2.10 Documentar regla de extensibilidad controlada

## 3. Verificación

- [x] 3.1 Revisar que el documento cubre todos los requerimientos del spec
- [x] 3.2 Revisar que los ejemplos son sintácticamente válidos YAML
- [x] 3.3 Ejecutar `./tests/validate.sh` para confirmar que no se rompe nada existente
- [x] 3.4 Actualizar `apply-progress.md` con evidencia de cumplimiento

## 4. Cierre

- [x] 4.1 Commit de los cambios con mensaje alineado a la convención del repo
- [x] 4.2 Generar `verify-report.md`
