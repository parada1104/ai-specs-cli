# Proposal: playwright-ui-flow

## Why

Consumer projects that ship web UIs need agents to run **Playwright tests and
smokes** with the same discipline we already expect for unit/integration work
(`tdd-flow`, evidence before merge) — plus a path for **interactive browser
exploration** when writing or debugging UI coverage.

Today the catalog has no Playwright-aware recipe. Agents either improvise
(`npx playwright test`, ad-hoc MCP), or skip UI verification. Playwright itself
now offers two first-class agent surfaces — **CLI** (`@playwright/cli` + test
runner) and **MCP** (`@playwright/mcp`) — that serve different jobs (suites vs
exploratory loops). We need an ai-specs recipe shape that makes the right
surface obvious and configurable.

## What Changes

1. **Catalog recipe support for Playwright UI tests/smokes** under
   `catalog/recipes/` (exact recipe id(s) and count decided in design).
2. **A shared capability** (proposed id: `ui-browser-testing`) documenting the
   contract: when to run UI smokes/suites, how evidence is recorded, and how
   CLI vs MCP surfaces are chosen.
3. **Skill topology without full duplication:**
   - Shared **discipline** skill(s) for suite/smoke/evidence policy (owned once).
   - Thin **tool adapter** skill(s) for CLI and/or MCP — same idea as VCS
     siblings sharing `vcs-pr-flow` while splitting host skills
     (`git-merge-workflow` vs `gitlab-merge-workflow` vs `bitbucket-merge-workflow`).
   - Design MAY improve on VCS by extracting a true shared discipline skill
     rather than near-copy host playbooks.
4. **Hybrid product behavior** as the north star:
   - **CLI** for authoring/running tests and smokes (token-efficient, CI-shaped).
   - **MCP** for exploratory navigation, debugging, locator generation.
   - Precedence rules live in skill text (+ brief fragments); packaging may be
     one TOML-configured recipe or composable siblings.
5. **Init / config / docs / tests** so `recipe add` → `recipe init` → `sync`
   yields a working agent-facing setup for a Playwright project.
6. **Docs**: `docs/capabilities.md` + catalog README entries for the new
   capability/recipe(s).

## Locked decisions

| Decision | Choice | Rationale |
|---|---|---|
| Problem | Playwright UI tests + smokes for agents | User request; gap vs `tdd-flow` |
| Surfaces in scope | CLI, MCP, and hybrid behavior | User: evaluate all three |
| Hybrid north star | CLI = suites/smokes; MCP = explore/debug | Matches Playwright’s own CLI vs MCP guidance |
| Skill duplication | Forbidden for discipline | User: 3 recipes ≠ 3 full skills; share base via capability + thin tool skills |
| Relate to tdd-flow | Complement, do not replace | `test-runner` stays unit/generic; UI is a distinct capability |
| Harness conditional MCP | Out of scope unless design proves blocked | Avoid sync-schema expansion in v1 if packaging can work without it |

## Decisions deferred to design (Opus)

Design MUST lock these and record rationale in `design.md`:

1. **Recipe topology**: 1 configurable recipe vs 2 siblings (CLI+MCP) vs 3
   (including hybrid) — and how hybrid is expressed (third recipe vs enable both
   vs `mode` in TOML).
2. **Capability id** and whether a foundational tool-agnostic recipe is v1 or
   Playwright-only siblings.
3. **Skill ownership**: which recipe owns the shared discipline skill; how
   adapters are declared in each `recipe.toml`.
4. **MCP materialization** for CLI-only installs given static `[[provides.mcp]]`.
5. **Config schema**, init.md questions, commands, brief fragments, `[[deps.cli]]`.
6. **Conflicts / bindings**: `conflicts_with`, tags, and `[[bindings]]` rules if
   multiple recipes provide the same capability.
7. **Test plan** for catalog validation, materialize, and skill-sync.

## Non-goals

- Replacing `tdd-flow` or `testing-foundation`.
- Building a custom Playwright MCP fork; use `@playwright/mcp`.
- Full visual/regression platform (Percy, etc.).
- Teaching every Playwright API — focus on agent operating model for tests/smokes.
- Changing core sync to support config-conditional `[[provides.mcp]]` unless
  design shows v1 is blocked without it (then split or escalate).
- Cypress/Selenium/other browsers-as-primary in v1.

## Impact / scope surface

| Area | Touch? | Notes |
|---|---|---|
| `catalog/recipes/<playwright-*>/` | Yes | Primary deliverable |
| `docs/capabilities.md` | Yes | New capability row |
| `docs/recipes-catalog.md` | Yes | Catalog entry |
| `openspec/specs/ui-browser-testing/` | Yes | New capability spec (this change) |
| `tests/` | Yes | Recipe load/materialize/skill-sync coverage |
| Core sync / MCP merge | Prefer no | Only if design requires |
| `tdd-flow` | No / light cross-link | README pointer only |

## Success criteria

1. A project can enable the chosen recipe topology and, after init+sync, agents
   have skills/commands/brief guidance for Playwright UI tests and smokes.
2. Hybrid behavior is unambiguous: CLI for suite/smoke runs; MCP for explore/debug
   (when that surface is enabled).
3. No full duplication of discipline skills across recipes.
4. Catalog validation + focused tests pass; `./tests/validate.sh` green.
5. `docs/capabilities.md` lists the new capability with provider recipe(s).

## Tracker

- Trello: https://trello.com/c/QssRysPv (card 44)
- Branch / worktree: `feat/playwright-ui-flow` / `.worktrees/playwright-ui-flow`
- Depth: Full (explore → proposal → spec → design → tasks)
