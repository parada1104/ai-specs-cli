---
name: harness-skills-deps
description: >
  Manage ai-specs skills and external dependencies: create local skills,
  skills add/list/remove, and add-dep, then sync. Trigger: When creating a
  local skill, installing a vendored skill from git, listing registered
  skills, or removing a skill dependency.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Create a local ai-specs skill in the project"
    - "Install or add an external skill dependency"
    - "List or remove vendored skills"
    - "Register a skill with skills add or add-dep"
---

# Harness skills and deps

Operate **local skills** and **vendored skill deps**. For recipe catalog ops
see `harness-recipes`. For sync/doctor see `harness-lifecycle`.

## Local skills

1. Author under `ai-specs/skills/<name>/SKILL.md`.
2. Follow **`skill-creator`** for structure, naming, and frontmatter
   (`scope` + `auto_invoke` when the skill should auto-invoke).
3. Validate metadata with **`skill-sync`** after create/edit.
4. Run `ai-specs sync` so fan-out and AGENTS.md stay current.

Do not hand-edit generated agent instruction files to "register" a skill —
sync owns that path.

## External / vendored deps

```bash
ai-specs skills add <git-url> [path]    # register [[deps]] + sync
# alias:
ai-specs add-dep <git-url> [path]

ai-specs skills list [path]             # bundled / local / recipe / registered
ai-specs skills remove <id> [path]      # drop from manifest
```

After add/remove, ensure sync completed (add already triggers sync; if you
edited toml by hand, run `ai-specs sync`).

## Precedence (remember)

Local > recipe > dependency when skill names collide. Prefer local overrides
only when intentional.

## Pitfalls

- **Creating files under `.deps/` or `.recipe/` by hand** — those trees are
  managed/gitignored; use CLI + local `ai-specs/skills/`.
- **Missing frontmatter** — sync warns; auto-invoke will not work. Use
  `skill-sync`.
- **Forgetting sync after local skill edits** — other runtimes never see it.
- **Confusing recipe skills with deps** — recipes come from `ai-specs recipe add`;
  git skill packages use `ai-specs skills add`.

## Checklist

- [ ] Local skill path is `ai-specs/skills/<name>/SKILL.md`
- [ ] Frontmatter valid (`skill-creator` + `skill-sync`)
- [ ] External deps added via `ai-specs skills add` / `ai-specs add-dep`
- [ ] `ai-specs skills list` shows the expected sources
- [ ] `ai-specs sync` after changes
