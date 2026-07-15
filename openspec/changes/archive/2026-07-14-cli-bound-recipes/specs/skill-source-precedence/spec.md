# Spec delta: skill-source-precedence

## MODIFIED Requirements

### Requirement: Three-tier scan

Skill resolution SHALL scan, in order:
1. `ai-specs/skills/`
2. `{cache}/.recipe/...`
3. `{cache}/.deps/...`

Whole-directory precedence rules remain unchanged. Fan-out targets remain unchanged.

(Previously: project-local `.recipe` and `.deps` paths under `ai-specs/`.)

#### Scenario: Precedence

- GIVEN the same skill id in local, recipe, and dep tiers
- WHEN resolution runs
- THEN the local skill wins
- GIVEN the same skill id in recipe and dep tiers only
- WHEN resolution runs
- THEN the recipe-tier skill wins

## ADDED Requirements

### Requirement: Command merge

Managed recipe commands SHALL stage in the cache. Hand-authored commands remain in `ai-specs/commands/`. On conflict, hand-authored commands win. Fan-out targets remain unchanged.

#### Scenario: Merge and fan-out

- GIVEN a command id present in both cache-managed and hand-authored locations
- WHEN fan-out runs
- THEN the hand-authored command wins
- AND agent command targets match pre-change behavior aside from source relocation
