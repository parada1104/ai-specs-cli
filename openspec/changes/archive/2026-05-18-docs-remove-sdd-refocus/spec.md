# Delta Spec: Remove SDD/OpenSpec from Documentation

## Requirements

| # | Requirement | Priority |
|---|-------------|----------|
| R1 | All user-facing documentation MUST NOT reference SDD, OpenSpec, or spec-driven development as a product feature. | MUST |
| R2 | Deleted files (`docs/ai/sdd.md`, `docs/ai/examples/config.yaml`, `catalog/skills/openspec-sdd-conventions/SKILL.md`) MUST NOT exist in the repository. | MUST |
| R3 | `README.md` MUST accurately describe core primitives (fan-out, harness, skills, MCP, recipes) without SDD references. | MUST |
| R4 | `AGENTS.md` MUST remain internally consistent after stripping OpenSpec/SDD from runtime brief sections. | MUST |
| R5 | Cross-references between remaining documents MUST NOT produce broken links. | MUST |
| R6 | `catalog/recipes/trello-mcp-workflow/README.md` MUST use neutral "ai-specs projects" wording instead of "SDD workflows". | MUST |

## Scenarios

### Scenario: README reader sees accurate primitives
- GIVEN a user reads `README.md`
- WHEN they scan the key concepts and CLI table
- THEN they MUST NOT see "SDD", "OpenSpec", or `ai-specs sdd`
- AND they MUST see fan-out, harness engineering, skills, MCP, and recipes described

### Scenario: AGENTS.md remains coherent
- GIVEN a user reads `AGENTS.md` for runtime context
- WHEN they read Purpose, Context Sources, Conflict Policy, Workflow Rules
- THEN they MUST NOT see OpenSpec/SDD workflow prescriptions
- AND the document MUST still describe harness engineering and project tracking

### Scenario: Deleted files return 404
- GIVEN a user or crawler requests `docs/ai/sdd.md`
- WHEN the change is applied
- THEN the file MUST NOT exist
- AND gentle-ai MAY be referenced as the current SDD provider

### Scenario: TOML docs without `[sdd]`
- GIVEN a user reads `docs/ai-specs-toml.md`
- WHEN they review the manifest surface
- THEN they MUST NOT see a `[sdd]` section
- AND they MUST NOT see SDD links in See also

### Scenario: Troubleshooting without SDD failures
- GIVEN a user reads `docs/ai/troubleshooting.md`
- WHEN they look for failure categories
- THEN they MUST NOT see an "SDD failures" section
- AND remaining sections MUST still cover fan-out, MCP, and recipe issues

### Scenario: Catalog README without SDD skill
- GIVEN a user browses `catalog/README.md`
- WHEN they review available skills
- THEN they MUST NOT see `openspec-sdd-conventions`
- AND they MUST NOT see `[[deps]]` examples pointing to SDD

## Edge Cases

| Edge | Handling |
|------|----------|
| External bookmarks to deleted docs | Accept 404; gentle-ai hosts SDD docs now. No redirect needed. |
| Cross-links from internal docs to deleted files | Remove or rewrite the link anchor before deletion. |
| Search indexing of deleted pages | Will naturally resolve after merge; no action needed. |
| Recipe compatibility | Trello recipe rewording is cosmetic; no behavior change. |
| Generated artifacts (e.g., `ai-specs sync`) | Not affected; no code changes. |

## Non-Goals

- **N1**: Do NOT modify `ai-specs/ai-specs.toml` (internal dogfooding, not user-facing).
- **N2**: Do NOT modify `openspec/specs/` (internal SDD specs, not user-facing).
- **N3**: Do NOT modify `docs/recipe-schema.md`, `docs/mcp-distribution.md`, `docs/skills-by-agent.md`, `docs/bundled-merge-rules.md` (no SDD content).
- **N4**: Do NOT change any code in `lib/` or `src/`.
- **N5**: Do NOT add redirects or replacement pages for deleted docs.
