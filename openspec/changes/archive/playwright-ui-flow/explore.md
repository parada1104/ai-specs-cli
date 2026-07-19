# Exploration: Playwright UI tests & smokes recipe support

> Persisted in Engram as `sdd/playwright-ui-flow/explore` (obs family `#1374`).
> Trello: https://trello.com/c/QssRysPv (card 44)

## Problem

Agents lack a catalog recipe that teaches when/how to run Playwright-backed UI
tests and smokes. Today `tdd-flow` covers tool-agnostic RED/GREEN against a
generic `test_command`, and `testing-foundation` covers this repo's unit/validate
commands — neither covers browser UI suites, interactive exploration, or the
Playwright CLI vs MCP surfaces.

## Surfaces evaluated

| Surface | Package / entry | Best for | Weak for |
|---|---|---|---|
| **CLI** | `@playwright/cli` + project `npx playwright test` (or configured cmd) | Coding agents, suites, smokes, lower token cost | Sandboxed agents without shell/fs |
| **MCP** | `@playwright/mcp` (`[[provides.mcp]]`) | Exploratory UI, debug, locator generation, interactive loops | Long suite runs (schemas/snapshots in context) |
| **Hybrid** | Both, with precedence rules | Tests/smokes via CLI; explore/debug via MCP | Needs clear when-to-use matrix |

Official Playwright docs already contrast MCP vs CLI along these lines.

## Packaging evaluated

| Shape | Idea | Pros | Cons |
|---|---|---|---|
| **1 recipe + TOML** | `mode = cli\|mcp\|hybrid` (or flags) | One install; init asks once; `{config.*}` in brief | `[[provides.mcp]]` is static today — true conditional MCP needs harness work or always-declare MCP |
| **2 recipes** | CLI + MCP; hybrid = enable both | Clean composition; opt out of MCP materialization | Shared discipline must not duplicate; tag/capability rules |
| **3 recipes** | CLI + MCP + hybrid | Explicit install names | Hybrid often just “both”; catalog weight unless skills are shared |

### Skill-sharing constraint (user clarification)

Multiple recipes MUST NOT imply triplicated discipline skills. Pattern to follow
(and improve on) is VCS siblings:

- **Shared capability** `vcs-pr-flow` — one openspec contract / semantics base.
- **Tool-specific skills** — `git-merge-workflow` / `gitlab-merge-workflow` /
  `bitbucket-merge-workflow` separate the host CLIs.

For Playwright, design SHOULD prefer a **shared discipline skill** (when to
smoke, evidence, suite vs explore) plus **thin tool adapter skills** (CLI vs MCP
commands/tools), rather than three full copies of the same playbook.

## Catalog precedents

| Recipe | Pattern |
|---|---|
| `tdd-flow` | Foundational; `test_command` config; capability `test-runner` |
| `git-pr-flow` family | Sibling recipes; shared capability; host skills |
| `trello-mcp-workflow` | Single MCP recipe; `[[provides.mcp]]` + `[[deps.cli]]` (npx) |
| `worktree-flow` | Single recipe; `gate_mode` config dispatch |

## Recommendation (input to proposal; packaging not locked)

1. Ship Playwright UI support as catalog recipe work under change
   `playwright-ui-flow`.
2. Treat **hybrid behavior** as the product north star (CLI for suites/smokes,
   MCP for explore/debug) whether that lands as one configurable recipe or
   composable siblings.
3. **Leave recipe count / topology to design** with the hard constraint: no full
   skill duplication; share capability + split tool skills.
4. Relate to (do not replace) `tdd-flow` / `testing-foundation`.

## Open for design

1. One configurable recipe vs 2 vs 3 siblings (and whether hybrid is a third
   recipe or composition).
2. Capability id naming (`ui-browser-testing` vs narrower).
3. Whether a foundational tool-agnostic recipe is in v1 or Playwright-only.
4. How MCP declaration interacts with CLI-only installs given static
   `[[provides.mcp]]`.
5. Config keys, init contract, commands, brief fragments, doctor/deps.
6. Exact skill split (shared discipline + adapters).

## Ready for Proposal

Yes — product intent clear; packaging/skill topology deferred to design with the
anti-duplication constraint above.
