## 1. Infraestructura y contratos

- [x] 1.1 Añadir entrada `sdd` en `bin/ai-specs` que despache a `lib/sdd.sh` con convención de flags y `--help` alineada a specs.
- [x] 1.2 Crear `lib/sdd.sh` (bash strict mode) con subcomandos mínimos: `enable`, `disable`, `status` (o flags equivalentes documentados en apply).
- [x] 1.3 Crear `lib/_internal/sdd.py` con validación de `[sdd]`, merge idempotente de TOML, y tabla interna de adaptadores por `provider`.
- [x] 1.4 Añadir plantillas bajo `templates/` para fragmentos de `openspec/config.yaml` compatibles con `openspec-sdd-conventions` (testing, schema spec-driven).
- [x] 1.5 Actualizar `templates/ai-specs.toml.tmpl` y documentación del manifiesto con bloque `[sdd]` comentado de ejemplo.

## 2. Proveedor OpenSpec

- [x] 2.1 Implementar verificación de `openspec` en `PATH` y versión mínima de Node (≥ 20.19) cuando aplique, con mensajes que citen `@fission-ai/openspec`.
- [x] 2.2 Implementar flag explícito de instalación global vía `npm` y manejo de fallo si `npm` ausente.
- [x] 2.3 Implementar detección de `openspec/` y ejecución no interactiva de `openspec init` mapeando `[agents].enabled` a `--tools` cuando sea posible; respetar ausencia de `--force`.
- [x] 2.4 Post-init: fusionar o copiar plantilla de `openspec/config.yaml` sin pisar claves críticas definidas en diseño (lista blanca de merge).
- [x] 2.5 Opcional: invocar `openspec update` o equivalente documentado tras init cuando el perfil extendido lo requiera; fijar constante de perfil según upstream al momento del apply.

## 3. Catálogo y refresh-bundled

- [x] 3.1 Definir preset `openspec` (lista de skills/commands del repo CLI) consumible desde `refresh-bundled` o wrapper que llame al mismo entrypoint Python.
- [x] 3.2 Integrar preset en el flujo `sdd enable` y asegurar que `.ai-specs.lock` refleje hashes coherentes.
- [x] 3.3 Documentar en README si el preset añade `[[deps]]` recomendadas (`openspec-sdd-conventions`, `testing-foundation`) o solo copia bundle local.

## 4. Doctor y status

- [x] 4.1 Extender `lib/_internal/doctor.py` con chequeos SDD condicionados a `[sdd].enabled` (PATH, `openspec/`, parseo ligero de config).
- [x] 4.2 Añadir pruebas unittest para matrices OK/WARN/ERROR según `artifact_store` y presencia de `openspec/`.
- [x] 4.3 Implementar `sdd status` (o salida de `sdd enable --dry-run` si se elige) que resuma estado sin mutar.

## 5. Pruebas finales y documentación

- [x] 5.1 Tests de integración livianos con `TMPDIR` y `openspec` mockeado o skipped condicional si el binario no está en CI (documentar en test).
- [x] 5.2 Actualizar README principal con flujo `ai-specs sdd`, requisitos de Node/npm, y tabla `artifact_store`.
- [x] 5.3 Ejecutar `./tests/validate.sh` y `./tests/run.sh` antes de cierre de apply; corregir reglas `openspec/config.yaml` del repo si `openspec validate` falla por IDs `apply`/`verify` bajo schema spec-driven.
