# OpenSpec apply: commits and archive

## Purpose

This repository uses **spec-driven** OpenSpec changes (`openspec/config.yaml`). During
**apply**, git history should stay reviewable: one large squash at the end is allowed
only when recovering lost work; the default is **commits aligned with task phases**.

## Commits during apply

1. **Prefer one commit per top-level block** in `openspec/changes/<change>/tasks.md`
   (sections `## 1`, `## 2`, …), when that block is a coherent unit of work (e.g. test
   harness, CLI wiring, core engine, verification/docs).

2. **Within a long phase**, split by **sub-phase** (e.g. `3.1–3.3` then `3.4–3.6`) if
   the diff would otherwise be hard to review, but keep each commit **green**
   (`./tests/run.sh` or the narrowest test slice you are using for TDD).

3. **Update `tasks.md` checkboxes** as each task completes (`openspec-apply-change`);
   commits and checkboxes can advance together so progress stays auditable.

4. **End of apply** (before PR): run `./tests/validate.sh`, refresh
   `apply-progress.md` / `verify-report.md` as required by the change, then push.

The OpenSpec skill `openspec-apply-change` defines *what* to implement per task; this
document defines *how* we usually slice **git commits** during that work.

## Archive after merge

When the implementation is merged into **`development`** (or the integration branch
your team uses), close the change with **`openspec-archive-change`** so delta specs
and the change folder follow the project archive rules. Verify first with
`openspec-verify-change` / `verify-report.md` if the change required it.

## Related

- `docs/ai/testing-foundation.md` — default test commands and evidence.
- `ai-specs/skills/ai-specs-development-worktrees/SKILL.md` — worktree layout (`.worktrees/`).
