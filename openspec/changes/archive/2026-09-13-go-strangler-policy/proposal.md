# Proposal: go-strangler-policy

Depth: light

## Tracker

- **card_id**: `6aa61f13786f559ac267fa45`
- **url**: https://trello.com/c/XN5cTJi5/126-adopt-project-level-python%E2%86%92go-strangler-policy
- **title**: Adopt project-level Python→Go strangler policy
- **board**: ai-specs-cli Roadmap

Related context: card 125 (Python→Go migration direction) — background only, not
implemented or linked by this change.

## Intent

Make the Python→Go strangler rule a **repository context** statement, not a recipe
contract. The canonical source for this project is `ai-specs/ai-specs.toml`
`[brief].workflow_rules`; normal `sync` renders it into the managed `AGENTS.md`.
This keeps the policy project-owned and avoids a custom AGENTS section or recipe
coupling. The existing brief is adopted and regenerated through the CLI; it is never
hand-edited.

## Policy meaning

- New authoritative logic, state machines, predicates, and durable state belong in Go.
- When touching Python on the active path, migrate the behavior/dependency being
  touched to Go where practical; Python may remain only as a thin
  compatibility/acquisition/JSON bridge during the transition.
- Keep one authoritative grader per behavior and add parity/contract tests at each
  migrated seam.
- Do not perform unrelated drive-by rewrites; the policy is incremental.

## Scope

### In scope

1. Append the policy as project-specific `[brief].workflow_rules` entries in
   `ai-specs/ai-specs.toml`, preserving existing ordering, comments, and TOML list
   syntax.
2. Regenerate the existing runtime brief through `ai-specs sync --adopt-brief` followed
   by normal `sync`, so the tracked `AGENTS.md` receives the policy without a hand edit.
3. Light planning artifacts in this change folder (`proposal.md`, `tasks.md`).

### Non-goals

- Adding a custom AGENTS section or editing generated `AGENTS.md` by hand.
- Putting the rule in `openspec/config.yaml`, a catalog recipe, or
  `[brief].workflow_rules_mode` machinery.
- Changing unrelated manifest values or any recipe file.
- Migrating Python code to Go in this change; this is policy only.

## Acceptance criteria

- [x] `ai-specs/ai-specs.toml` parses as valid TOML.
- [x] `[brief].workflow_rules` contains the four policy bullets above, appended
      after existing entries, with no other value or comment changed.
- [x] The policy is sourced from the manifest; no recipe or `openspec/config.yaml`
      entry adds it.
- [x] `AGENTS.md` contains the policy only as output of the controlled adopt/sync path;
      it is not hand-edited.

## Validation commands

```bash
cd .worktrees/go-strangler-policy
python3 -c "import tomllib,pathlib; d=tomllib.loads(pathlib.Path('ai-specs/ai-specs.toml').read_text()); print(d['brief']['workflow_rules'])"
python3 -m unittest tests.test_agents_render_brief_fragments
./bin/ai-specs sync --adopt-brief .
./bin/ai-specs sync .
```

## Risks

| Risk | Mitigation |
|---|---|
| Policy duplicates a recipe fragment in rendered `AGENTS.md` | Manifest `[brief]` prose is not substituted by recipe fragments; verify via render test |
| Edited another manifest value by accident | Keep the diff to the `workflow_rules` list only |

## Authorization

No production behavior change; this is a manifest + planning change. Stop for
human authorization before any further work.
