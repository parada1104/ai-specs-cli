# runtime-brief-rendering Delta — harness CLI literacy pointer

## Purpose

Extend runtime brief rendering so every generated `AGENTS.md` includes a thin,
always-on pointer to the harness CLI literacy skills, covering runtimes that do
not auto-invoke skills.

## ADDED Requirements

### Requirement: Always-on harness literacy pointer bullet

When `agents-render.py` generates a normal runtime brief (i.e. not skipped by
`--preserve-if-runtime-brief`), it MUST emit exactly one fixed bullet that
directs agents to the harness literacy skills
`harness-lifecycle`, `harness-recipes`, and `harness-skills-deps` under
`ai-specs/skills/`.

The bullet MUST appear in `## Useful Commands` when that section is emitted.
If the implementation places it in another always-present section instead, that
section MUST still be part of the fixed brief order and covered by tests — but
the default design target is `## Useful Commands`.

#### Scenario: Pointer present on generated brief

- GIVEN a manifest that triggers normal AGENTS.md generation
- WHEN `agents-render.py` renders the brief
- THEN the output MUST contain a bullet mentioning `harness-lifecycle`,
  `harness-recipes`, and `harness-skills-deps`
- AND the bullet MUST mention harness operations or equivalent wording that
  points agents to those skills

#### Scenario: User-managed brief remains untouched

- GIVEN an existing `AGENTS.md` containing the
  `<!-- ai-specs:runtime-brief -->` marker
- AND `--preserve-if-runtime-brief` is set
- WHEN `agents-render.py` runs
- THEN the file MUST remain unchanged (no forced pointer injection)

#### Scenario: Pointer does not require recipes enabled

- GIVEN a project with no recipes enabled and an empty `[brief].useful_commands`
- WHEN `agents-render.py` renders the brief with resolved config
- THEN the harness literacy pointer bullet MUST still appear
