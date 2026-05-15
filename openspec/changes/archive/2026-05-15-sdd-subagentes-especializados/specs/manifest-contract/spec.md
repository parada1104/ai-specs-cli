## ADDED Requirements

### Requirement: Campo opcional sub_agents en la tabla SDD

El sistema SHALL reconocer en `[sdd]` un campo opcional `sub_agents` de tipo booleano cuya semántica está definida por el contrato `sdd-subagent-deployment`. Cuando el campo está ausente, MUST tratarse como `false`. Cuando está presente, MUST ser estrictamente booleano.

#### Scenario: sub_agents ausente preserva semántica V1
- **GIVEN** un manifiesto con `[sdd]` que omite `sub_agents`
- **WHEN** el validador V1 evalúa el manifiesto
- **THEN** la validación MUST pasar
- **AND** los lectores que consulten `sub_agents` MUST recibir el valor `false`

#### Scenario: sub_agents con valor booleano válido
- **GIVEN** un manifiesto con `[sdd]` que declara `sub_agents = true` o `sub_agents = false`
- **WHEN** el validador V1 evalúa el manifiesto
- **THEN** la validación MUST pasar
- **AND** el lector MUST exponer el valor declarado a los comandos consumidores

#### Scenario: sub_agents con tipo no booleano
- **GIVEN** un manifiesto con `[sdd].sub_agents = "true"` u otro valor no booleano
- **WHEN** el validador V1 evalúa el manifiesto
- **THEN** la validación MUST fallar con mensaje explícito que nombra el campo y el tipo esperado
- **AND** el comando consumidor MUST NOT proceder con materialización

### Requirement: Plantilla y README documentan sub_agents

La plantilla `templates/ai-specs.toml.tmpl` y la documentación pública del manifiesto MUST declarar `[sdd].sub_agents` como opcional con default `false`, describir su efecto y dejar claro que su omisión preserva el comportamiento V1.

#### Scenario: Plantilla incluye sub_agents comentado
- **GIVEN** `templates/ai-specs.toml.tmpl` después de este cambio
- **WHEN** un mantenedor inspecciona la plantilla
- **THEN** la plantilla MUST contener una entrada `sub_agents = false` comentada bajo `[sdd]`
- **AND** los comentarios MUST explicar que activarla despliega subagentes especializados para el ciclo SDD

#### Scenario: Documentación pública refleja el campo
- **GIVEN** el README o documento de contrato del manifiesto
- **WHEN** se publica este cambio
- **THEN** la documentación MUST listar `sub_agents` como campo opcional de `[sdd]`
- **AND** MUST advertir que activarlo es feature de producto y requiere harnesses compatibles
