## ADDED Requirements

### Requirement: Campos mínimos obligatorios
Todo documento de memoria canónica SHALL incluir en su frontmatter YAML los campos: `type`, `scope`, `status`, `tags`, `source`, `updated_at`.

#### Scenario: Documento con campos mínimos completos
- **WHEN** un agente crea un documento de memoria canónica
- **THEN** el frontmatter del documento contiene los campos `type`, `scope`, `status`, `tags`, `source`, `updated_at`

#### Scenario: Documento incompleto es inválido
- **WHEN** un documento de memoria canónica carece de alguno de los campos mínimos obligatorios
- **THEN** se considera no conforme al estándar

### Requirement: Valores válidos para type
El campo `type` SHALL aceptar únicamente los valores: `decision`, `handoff`, `spec`, `context`, `proposed`.

#### Scenario: Type válido
- **WHEN** un documento especifica `type: decision`
- **THEN** el documento es conforme

#### Scenario: Type inválido
- **WHEN** un documento especifica `type: random`
- **THEN** el documento es no conforme

### Requirement: Valores válidos para scope
El campo `scope` SHALL aceptar únicamente los valores: `project`, `team`, `system`, `session`.

#### Scenario: Scope válido
- **WHEN** un documento especifica `scope: project`
- **THEN** el documento es conforme

#### Scenario: Scope inválido
- **WHEN** un documento especifica `scope: global`
- **THEN** el documento es no conforme

### Requirement: Valores válidos para status
El campo `status` SHALL aceptar únicamente los valores: `proposed`, `accepted`, `deprecated`.

#### Scenario: Status válido
- **WHEN** un documento especifica `status: accepted`
- **THEN** el documento es conforme

#### Scenario: Status inválido
- **WHEN** un documento especifica `status: draft`
- **THEN** el documento es no conforme

### Requirement: Campo confidence opcional
El campo `confidence` MAY estar presente. Si está presente, SHALL ser un número decimal entre 0.0 y 1.0 inclusive.

#### Scenario: Confidence presente y válido
- **WHEN** un documento incluye `confidence: 0.85`
- **THEN** el documento es conforme

#### Scenario: Confidence fuera de rango
- **WHEN** un documento incluye `confidence: 1.5`
- **THEN** el documento es no conforme

### Requirement: Documento de referencia canónico
El estándar SHALL publicarse como archivo de referencia en `ai-specs/docs/canonical-memory-frontmatter.md`.

#### Scenario: Documento de referencia existe
- **WHEN** se busca el estándar en `ai-specs/docs/canonical-memory-frontmatter.md`
- **THEN** el archivo existe y contiene los campos, valores válidos y ejemplos

#### Scenario: Ejemplos por tipo de documento
- **WHEN** se inspecciona el documento de referencia
- **THEN** contiene al menos un ejemplo de frontmatter para cada tipo: doc canónica, handoff, proposed memory

### Requirement: Extensibilidad controlada
Nuevos valores para `type`, `scope` o `status` SHALL agregarse únicamente mediante actualización del documento de referencia canónico, no ad-hoc en documentos individuales.

#### Scenario: Extensión del estándar
- **WHEN** surge la necesidad de un nuevo valor `type: retrospective`
- **THEN** el nuevo valor se añade al documento `ai-specs/docs/canonical-memory-frontmatter.md` antes de ser usado
