## MODIFIED Requirements

### Requirement: Harness CLI literacy pointer

The always-on Useful Commands bullet that points agents at harness CLI literacy
skills SHALL name `harness-lifecycle`, `harness-recipes`, and
`harness-skills-deps` without claiming they materialize under
`ai-specs/skills/`. CLI-bundled skills resolve from the agent skill fan-out /
cache, not the committed project surface.

#### Scenario: Pointer omits in-project path

- **WHEN** `agents-render` emits `## Useful Commands`
- **THEN** the harness literacy bullet mentions the three harness skill ids
- **AND** it does NOT say the skills live under `ai-specs/skills/`
