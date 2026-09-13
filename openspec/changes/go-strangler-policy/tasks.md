# Tasks: go-strangler-policy

Depth: light

Tracker: card #126 — https://trello.com/c/XN5cTJi5/126-adopt-project-level-python%E2%86%92go-strangler-policy
(`card_id: 6aa61f13786f559ac267fa45`). Related context: card #125 (background only).

## Depth / classification

- **Depth: light** — a manifest `[brief].workflow_rules` edit plus Light planning
  artifacts. No production code, no recipe, no schema.

## WU1 — Config source (canonical policy location)

- [x] Append the four policy bullets to `ai-specs/ai-specs.toml`
      `[brief].workflow_rules`, after the existing entries.
- [x] Preserve existing ordering/comments and the existing TOML list syntax.
- [x] Change no other manifest value.

## WU2 — Rendering validation (policy reaches AGENTS.md via normal sync)

- [x] Confirm no recipe `brief_fragments` provide this rule and that a custom
      AGENTS section is not needed.
- [x] Validate the `workflow_rules` render/merge contract with the focused
      render test (`tests.test_agents_render_brief_fragments`).
- [x] Parse the TOML to confirm the manifest stays valid and the bullets read
      back as expected.
- [x] Adopt the existing brief once, then run normal `sync` so the tracked
      `AGENTS.md` receives the policy through the CLI.

## WU3 — Generated-file ownership (guardrail)

- [x] Do **not** hand-edit generated `AGENTS.md`; the policy enters it only through
      the controlled adopt/sync rendering of the manifest.
- [x] Keep unrelated dogfood outputs (lock, env example, refreshed recipe files) out
      of the feature commit.
- [x] Do **not** place the rule in `openspec/config.yaml` or a catalog recipe.

## Out of scope

- Migrating Python code to Go.
- Any recipe, capability, or `openspec/config.yaml` change.

## Acceptance

- [x] TOML parses; `workflow_rules` contains exactly the four appended bullets.
- [x] Focused render test passes.
- [x] Full `./tests/validate.sh` passes (1912 tests, 135 skipped).
- [x] `AGENTS.md` contains the generated policy output; no hand edit, recipe, or config edit.

## Authorization gate

READY_FOR_AUTH: complete — user authorized the project-policy change; validation passed.
