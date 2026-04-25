# context-precedence Specification

## Purpose

Definir una politica canonica y auditable para resolver conflictos entre fuentes de contexto del proyecto sin introducir un router runtime ni nuevas secciones de manifiesto.

## Requirements

### Requirement: Canonical context source precedence
The system MUST define a single MVP precedence order for resolving conflicts between project context sources: canonical docs > project skills > packs > handoffs > session memory > proposed context.

#### Scenario: Conflict resolved by canonical docs
- **GIVEN** canonical project documentation and session memory disagree about a project rule
- **WHEN** an agent decides which context to follow
- **THEN** the agent MUST follow canonical project documentation
- **AND** the decision MUST be explainable by the published precedence order

#### Scenario: Conflict resolved by project skills
- **GIVEN** a project skill and a pack-provided convention disagree about an agent workflow
- **WHEN** an agent decides which instruction to follow
- **THEN** the agent MUST follow the project skill
- **AND** the pack-provided convention MUST NOT override the project skill

#### Scenario: Proposed context has lowest authority
- **GIVEN** proposed agent output conflicts with any existing documented source, project skill, pack, handoff, or session memory
- **WHEN** an agent decides whether to use the proposed output as context
- **THEN** the proposed output MUST yield to the higher-precedence source

### Requirement: Context precedence documentation
The system MUST publish the precedence rule in `docs/ai/context-precedence.md` with source definitions, conflict examples, and an audit-friendly decision process.

#### Scenario: Documentation states the full order
- **GIVEN** `docs/ai/context-precedence.md`
- **WHEN** a user or agent reads the document
- **THEN** it MUST state the full precedence order exactly once as the canonical rule
- **AND** it MUST define each source class used by the order

#### Scenario: Documentation gives conflict examples
- **GIVEN** `docs/ai/context-precedence.md`
- **WHEN** a user or agent needs to resolve a context conflict
- **THEN** the document MUST include examples covering docs vs memory, project skills vs packs, handoffs vs session memory, and proposed context vs existing sources

### Requirement: Generated agent instructions reference precedence
The system MUST make the context precedence rule discoverable from generated agent instructions without requiring manual edits to generated files.

#### Scenario: AGENTS includes generated reference
- **GIVEN** `ai-specs sync` regenerates `AGENTS.md`
- **WHEN** an agent reads the generated instructions
- **THEN** `AGENTS.md` MUST reference `docs/ai/context-precedence.md`
- **AND** it MUST include the MVP precedence order in compact form

#### Scenario: Generated files are not hand-edited
- **GIVEN** `AGENTS.md` is a generated artifact
- **WHEN** this change adds the precedence reference
- **THEN** the source change MUST be made in the renderer or generation source
- **AND** the generated output MUST be reproducible by the normal sync command

### Requirement: Documentation-first MVP boundary
The system MUST keep this MVP as a documentation and generated-instruction change, without introducing runtime merge behavior or new manifest schema.

#### Scenario: No manifest schema expansion
- **GIVEN** the context precedence rule is added
- **WHEN** `ai-specs/ai-specs.toml` is evaluated against the Manifest V1 contract
- **THEN** the rule MUST NOT require a new `[memory]`, `[precedence]`, `[packs]`, or other manifest section

#### Scenario: No runtime merge engine
- **GIVEN** multiple context sources conflict at runtime
- **WHEN** this MVP is applied
- **THEN** the system MUST NOT claim that conflicts are resolved automatically by a runtime merge engine
- **AND** the rule MUST remain an auditable decision policy for humans and agents
