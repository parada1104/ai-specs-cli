# Trello Board Audit — Repository Evidence

## Methodology

Audited the `development` branch, all local/remote branches, merged PRs (#1–#93), OpenSpec changes (active + archived), and commit history for evidence of card completion status. Card numbers are mapped from commit messages, OpenSpec artifacts, and PR descriptions.

---

## Evidence Table: Cards with Repository Evidence

| Card | Title/Topic (from repo evidence) | Verdict | Evidence |
|------|----------------------------------|---------|----------|
| **#11** | `.omp/AGENTS.md` per-harness mirror opt-out | **keep** | Referenced as "out of scope" in `agents-md-render-opt-out` exploration (line 187) and proposal (line 37, 90). No implementation found. Follow-up item, not started. |
| **#14** | `/rules-audit` — read-only rules migration inventory | **archive** | Merged PR #73 (`7b51d3b`). Archived in `openspec/changes/archive/2026-06-04-rules-migration-audit/`. Verify report exists. Spec promoted to `openspec/specs/rules-audit/`. |
| **#16** | Boolean TOML footgun (`True`/`False` vs `true`/`false`) | **archive** | Referenced as design constraint in `agents-md-render-opt-out` (design.md:23, exploration.md:102). Implemented as part of PR #80 (`6faf783`): doctor validates `render` as boolean. No standalone card implementation needed — absorbed into #18. |
| **#17** | Recipes catalog gaps vs `recipe.toml` | **archive** | Merged PR #78 (`f97ad8d`), commit message: "docs: close recipes-catalog gaps vs recipe.toml (card #17)". Branch `feat/card-17-recipes-catalog-gaps` squash-merged. |
| **#18** | Opt-out: sync no modifica `AGENTS.md` | **archive** | Full SDD cycle archived: `openspec/changes/archive/2026-06-09-agents-md-render-opt-out/`. Merged PR #80 (`6faf783`). Archive report present. Spec promoted to `openspec/specs/runtime-brief-rendering/`. |
| **#21** | `recipe list` CLI command | **archive** | Compactada con #22 en PR #11 (`feat/implementar-recipe-list-add`). Archived in `openspec/changes/archive/2026-04-26-implementar-recipe-list-add/`. Spec promoted to `openspec/specs/recipe-cli/`. |
| **#22** | `recipe add <id>` CLI command | **archive** | Same as #21 — compacted into single SDD cycle. PR #11. Same archive. |
| **#23** | VCS drop deferred cleanup (3 items) | **archive** | Full SDD cycle archived: `openspec/changes/archive/2026-06-11-vcs-drop-deferred-cleanup/`. Merged PR #93 (`c98e969`). Archive report present. Spec promoted to `openspec/specs/vcs-pr-flow/`. |

---

## Active OpenSpec Changes (Not Archived — Need Housekeeping)

These changes have all tasks checked `[x]` and are merged into `development`, but their OpenSpec directories were never archived:

| OpenSpec Change | PR | Commit | Tasks | Verdict |
|-----------------|-----|--------|-------|---------|
| `documentar-referencia-ai-specs-toml-y-recipes` | #23 | `414053e` | All `[x]`, only `tasks.md` remains | **archive** — done, needs spec archive |
| `feat-skills-commands` | #72 | `e701e9c` | All `[x]`, full artifacts (proposal/design/spec/tasks) | **archive** — done, needs spec archive |
| `motor-agents-md-runtime-brief` | #20 | `e483faf` | All `[x]`, only `tasks.md` remains | **archive** — done, needs spec archive |
| `recipe-anatomy-init-readme` | #29 | `75014d8` (locally merged) | All `[x]`, full artifacts | **archive** — done, needs spec archive |
| `recipe-brief-fragments` | #75 | `2668028` | All 40 `[x]`, verify-report PASS WITH WARNINGS | **archive** — done, needs spec archive |

---

## Open PR (Active Work)

| PR | Branch | Title | Status |
|----|--------|-------|--------|
| **#52** | `feat/mcp-compartido-por-proyecto` | Shared mcp-proxy daemon per git repo | **OPEN** — 26 commits ahead of development. Archive dir exists at `openspec/changes/archive/2026-05-28-mcp-compartido-por-proyecto/` but contains only `judgment-report-r1.md`. A `feat/mcp-compartido-v2` branch extends it further. |

---

## Stale Remote Branches (Squash-Merged, Safe to Delete)

These branches were squash-merged via PRs but the remote branches were never deleted:

| Branch | PR | Note |
|--------|-----|------|
| `origin/card78-boardid-validation` | #40 | Squash-merged |
| `origin/cursor/967b14f2` | — | Old cursor session, 1 commit behind |
| `origin/definir-sdd-adaptive-contract` | #18 | Squash-merged |
| `origin/docs-ai-specs-toml-reference` | #23 | Squash-merged |
| `origin/docs-cleanup` | #42 | Squash-merged |
| `origin/docs-remove-sdd-refocus` | #48 | Squash-merged |
| `origin/feat/capabilities-doc` | #61 | Squash-merged |
| `origin/feat/dogfood-recipes` | #60 | Squash-merged |
| `origin/feat/git-pr-flow` | #56 | Squash-merged |
| `origin/feat/git-pr-flow-decouple` | #62 | Squash-merged |
| `origin/feat/omp-agent-target` | #70 | Squash-merged |
| `origin/feat/option-c-runtime-brief` | #67 | Squash-merged |
| `origin/feat/recipe-brief-fragments` | #75 | Squash-merged |
| `origin/feat/rewire-dogfood` | #66 | Squash-merged |
| `origin/feat/runtime-brief-baseline` | #76 | Squash-merged |
| `origin/feat/session-context` | #57 | Squash-merged |
| `origin/feat/session-context-decouple` | #64 | Squash-merged |
| `origin/feat/skills-commands` | #72 | Squash-merged |
| `origin/feat/tdd-flow` | #63 | Squash-merged |
| `origin/feat/trello-consolidate-pm` | #55 | Squash-merged |
| `origin/feat/trello-recipe-init` | #27 | Squash-merged |
| `origin/feat/trello-tracker-cap` | #65 | Squash-merged |
| `origin/feat/worktree-cleanup-squash` | #58 | Squash-merged |
| `origin/feat/worktree-flow-recipe` | #54 | Squash-merged |
| `origin/fix/opencode-env-renderer` | #50 | Squash-merged |
| `origin/fix/recipe-contract-boundaries` | #26 | Squash-merged |
| `origin/fix/recipe-lock-roundtrip` | #59 | Squash-merged |
| `origin/motor-agents-md-runtime-brief` | #20 | Squash-merged |
| `origin/remove-sdd-from-product` | #47 | Squash-merged |
| `origin/sdd-subagentes-especializados` | #44 | Merged then reverted (#45) |
| `origin/simplify-remove-subagents` | #45 | Squash-merged |

---

## Cards That Need Explore (No Direct Repo Evidence)

Cards #1–#10, #12, #13, #15, #19, #20, #24, #25 have no explicit card number references in the repository. They may correspond to early PRs (#1–#10) or be entirely board-only items. Without the actual Trello card titles, these cannot be decided from repo evidence alone.

| Card Range | Reason | Likely PR Match |
|------------|--------|-----------------|
| #1 | No explicit card ref | PR #1 (gitmodules-root-subrepo-sync) — merged |
| #2 | No explicit card ref | PR #3 (definir-contrato-ai-specs-toml) — merged |
| #3 | No explicit card ref | PR #5 (definir-precedence-de-contexto) — merged |
| #4–#10 | No explicit card refs | PRs #6–#10 (skill-frontmatter, doctor, sdd-integration, recipe-schema, design-recipe-schema) — all merged |
| #12, #13 | No repo references found | Unknown — need Trello titles |
| #15 | No explicit card ref | Likely PR #15/#16 (motor-restructurar-dirs-externos) — merged |
| #19, #20 | No explicit card refs | Likely PR #19 (motor-mcp-preset-merge-seguro) and PR #20 (motor-agents-md-runtime-brief) — both merged |
| #24, #25 | No repo references found | Unknown — need Trello titles |

---

## Summary Verdicts

| Verdict | Count | Cards |
|---------|-------|-------|
| **archive** | 7 | #14, #16, #17, #18, #21, #22, #23 |
| **keep** | 1 | #11 (not started, valid follow-up) |
| **needs-explore** | ~12 | #1–#10, #12, #13, #15, #19, #20, #24, #25 |
| **active (open PR)** | 1 | mcp-compartido (PR #52, not a backlog card) |

### OpenSpec Housekeeping (not cards, but related)

5 OpenSpec change directories need archival: `documentar-referencia-ai-specs-toml-y-recipes`, `feat-skills-commands`, `motor-agents-md-runtime-brief`, `recipe-anatomy-init-readme`, `recipe-brief-fragments`. All are fully merged with all tasks complete.

### Branch Cleanup

~31 squash-merged remote branches can be safely deleted. 1 local branch (`recipe-anatomy-init-readme`) is merged and can be deleted. `feat/mcp-compartido-por-proyecto` and `feat/mcp-compartido-v2` are active.

---

## Files Retrieved

1. `openspec/changes/` — directory listing of active changes
2. `openspec/changes/archive/` — directory listing of 31 archived changes
3. `openspec/changes/archive/2026-06-09-agents-md-render-opt-out/` — exploration.md, proposal.md, design.md, archive-report.md (card #18, #11, #16 references)
4. `openspec/changes/archive/2026-06-11-vcs-drop-deferred-cleanup/` — proposal.md, tasks.md, archive-report.md (card #23 references)
5. `openspec/changes/archive/2026-06-04-rules-migration-audit/` — verify-report.md (Trello #14 reference)
6. `openspec/changes/archive/2026-04-26-implementar-recipe-list-add/` — PRE-APPLY-ANALYSIS.md (Trello #21, #22 references)
7. `openspec/changes/archive/2026-05-28-mcp-compartido-por-proyecto/` — judgment-report-r1.md
8. `openspec/changes/recipe-brief-fragments/` — verify-report.md, tasks.md (all 40 tasks complete)
9. `openspec/changes/feat-skills-commands/` — tasks.md (all tasks complete)
10. `openspec/changes/documentar-referencia-ai-specs-toml-y-recipes/` — tasks.md (all tasks complete)
11. `openspec/changes/motor-agents-md-runtime-brief/` — tasks.md (all tasks complete)
12. `openspec/changes/recipe-anatomy-init-readme/` — tasks.md (all tasks complete)

## Architecture

The project uses OpenSpec changes as SDD lifecycle containers. Each change goes through: exploration → proposal → spec → design → tasks → apply → verify → archive. Archived changes land in `openspec/changes/archive/<date>-<slug>/`. Promoted specs land in `openspec/specs/<name>/`. Trello card references appear in OpenSpec artifacts and commit messages, but not all cards are explicitly numbered in the repo.

## Start Here

Open `openspec/changes/archive/` to see the full list of completed and archived work. Cross-reference with `gh pr list --state merged` for PR-to-card mappings. For cards without repo evidence, an explore pass with Trello API access is needed.
