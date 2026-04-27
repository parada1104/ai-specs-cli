---
name: openspec-sdd-conventions
description: >
  OpenSpec config shape for spec-driven SDD plus commit/archive conventions
  during apply. Trigger: editing openspec/config.yaml, slicing apply commits,
  or closing a change after merge.
license: MIT
metadata:
  author: parada1104
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Editing openspec/config.yaml for spec-driven workflow"
---

# OpenSpec SDD Conventions

## OpenSpec config shape

`openspec/config.yaml` is the project-local OpenSpec configuration. It should be
valid for the installed `spec-driven` schema so normal SDD commands do not emit
configuration warnings.

The `rules` map should only contain schema artifacts, and each artifact value
should be a list of strings. For this repository that means:

- `rules.proposal`
- `rules.specs`
- `rules.design`
- `rules.tasks`
- `rules.apply`
- `rules.verify`

Do not put nested objects under `rules.apply` or `rules.verify`. Do not add
`rules.archive` while `archive` is not an artifact in the installed schema.

Richer project-specific metadata that does not fit the OpenSpec `rules` shape
lives under `ai_specs_guidance`. That section is for humans and local tooling;
it is not part of the OpenSpec artifact rules contract.

For default verification commands, see [`testing-foundation`](../testing-foundation/SKILL.md).

---

## Apply: commits and archive

This repository uses **spec-driven** OpenSpec changes (`openspec/config.yaml`). During
**apply**, git history should stay reviewable: one large squash at the end is allowed
only when recovering lost work; the default is **commits aligned with task phases**.

### Commits during apply

1. **Prefer one commit per top-level block** in `openspec/changes/<change>/tasks.md`
   (sections `## 1`, `## 2`, …), when that block is a coherent unit of work (e.g. test
   harness, CLI wiring, core engine, verification).

2. **Within a long phase**, split by **sub-phase** (e.g. `3.1–3.3` then `3.4–3.6`) if
   the diff would otherwise be hard to review, but keep each commit **green**
   (`./tests/run.sh` or the narrowest test slice you are using for TDD).

3. **Update `tasks.md` checkboxes** as each task completes (`openspec-apply-change`);
   commits and checkboxes can advance together so progress stays auditable.

4. **End of apply** (before PR): run `./tests/validate.sh`, refresh
   `apply-progress.md` / `verify-report.md` as required by the change, then push.

The OpenSpec skill `openspec-apply-change` defines *what* to implement per task; this
section defines *how* we usually slice **git commits** during that work.

### Archive after merge

When the implementation is merged into **`development`** (or the integration branch
your team uses), close the change with **`openspec-archive-change`** so delta specs
and the change folder follow the project archive rules. Verify first with
`openspec-verify-change` / `verify-report.md` if the change required it.

## Related

- [`testing-foundation`](../testing-foundation/SKILL.md) — default test commands and evidence.
- Worktree layout (e.g. `.worktrees/`) — follow your project’s worktree skill when vendored or present under `ai-specs/skills/`.
