# ui-browser-testing Specification

## Purpose

Define the `ui-browser-testing` capability and the catalog contract for
Playwright-backed UI test and smoke workflows that agents can follow. Packaging
(recipe count / ids) is decided in design; requirements here are binding for
whatever topology design locks.

## Non-Goals

- Replacing the `test-runner` capability (`tdd-flow`) for non-UI tests.
- Forking or reimplementing `@playwright/mcp` / `@playwright/cli`.
- Config-conditional MCP materialization in the sync harness (unless design
  escalates a blocking gap).
- Non-Playwright UI runners in v1.

---

## ADDED Requirements

### Requirement: Capability declaration

The catalog MUST introduce capability id `ui-browser-testing` meaning:
agent-operable browser UI verification — running project UI test suites and
smokes, and (when enabled) interactive exploration/debugging via a browser
automation surface.

At least one catalog recipe MUST declare `[[capabilities]] id =
"ui-browser-testing"`.

#### Scenario: Capability is provided by an enabled recipe

- GIVEN a project enables a recipe that declares `ui-browser-testing`
- WHEN sync resolves capabilities
- THEN `ui-browser-testing` MUST be available for binding / auto-binding per
  existing capability rules

#### Scenario: Docs list the capability

- GIVEN the change is implemented
- WHEN `docs/capabilities.md` is read
- THEN it MUST document `ui-browser-testing` with its provider recipe(s)

---

### Requirement: Hybrid surface semantics

Regardless of recipe topology, the shipped skill guidance MUST encode this
precedence:

| Job | Preferred surface |
|---|---|
| Run UI test suite / smoke | CLI / project test command (`playwright test` or configured equivalent) |
| Author or extend UI tests in-repo | CLI (+ repo test files); MCP optional for locator discovery |
| Exploratory UI navigation / debug | MCP (`@playwright/mcp`) when that surface is enabled |
| Generate locators while writing tests | MCP when enabled; otherwise CLI snapshot/codegen paths |

When only one surface is enabled for the project, guidance MUST degrade
gracefully to that surface and MUST NOT require the missing one.

#### Scenario: Suite run prefers CLI

- GIVEN the project has a configured UI test/smoke command (or discoverable
  Playwright test script)
- WHEN an agent verifies UI behavior before merge
- THEN the agent MUST prefer the CLI/suite command over MCP tool calls for the
  verification run

#### Scenario: Explore prefers MCP when available

- GIVEN MCP Playwright is enabled for the project
- WHEN an agent needs interactive page exploration or locator discovery
- THEN the agent SHOULD use Playwright MCP tools rather than inventing ad-hoc
  browser drivers

#### Scenario: CLI-only project still works

- GIVEN only the CLI surface is enabled
- WHEN an agent runs UI smokes or authors tests
- THEN the workflow MUST complete without requiring Playwright MCP

---

### Requirement: No full discipline skill duplication

If design ships more than one recipe for this capability family, the recipes
MUST share discipline through a **single canonical discipline skill** (one
skill id / one authoritative `SKILL.md` source in the catalog) and MAY add
**thin tool-adapter skills** for CLI vs MCP specifics.

Recipes MUST NOT ship three near-identical full playbooks that restate suite,
smoke, and evidence policy.

This mirrors the VCS sibling idea (shared `vcs-pr-flow` contract + host-specific
skills) and improves on it by forbidding copy-pasted discipline bodies.

#### Scenario: Shared discipline skill exists once

- GIVEN multiple Playwright-family recipes are present in the catalog
- WHEN catalog skill sources for UI discipline are inventoried
- THEN suite/smoke/evidence policy MUST live in exactly one canonical skill
  source reused or provided by those recipes

#### Scenario: Tool adapters stay thin

- GIVEN a CLI adapter skill and an MCP adapter skill exist
- WHEN each adapter `SKILL.md` is reviewed
- THEN each MUST focus on tool invocation and surface-specific pitfalls
- AND MUST defer suite/smoke/evidence policy to the shared discipline skill

---

### Requirement: Recipe packaging completeness

Whatever topology design locks (1 configurable recipe, 2 siblings, or 3), each
shipped recipe MUST include:

- Valid `recipe.toml` (schema-valid, with capability declaration as required)
- README materialized via `[[provides.docs]]`
- Init contract when project-specific values are required (`init.md` + `[init]`)
- Declared CLI deps and/or MCP presets appropriate to the surfaces it enables
- Brief fragments that state the hybrid precedence (or the single-surface subset)

#### Scenario: Schema validation passes

- GIVEN each new Playwright-family recipe in the catalog
- WHEN recipe schema validation runs
- THEN validation MUST succeed

#### Scenario: Sync materializes assets

- GIVEN a temp project enables a Playwright-family recipe with required config
- WHEN `ai-specs sync` runs
- THEN declared skills, commands (if any), docs, and MCP presets (if any) MUST
  materialize to the expected project paths

---

### Requirement: Configurability for project commands

Projects MUST be able to configure how UI suites/smokes are invoked (e.g. a
string command analogous to `tdd-flow`’s `test_command`), rather than hardcoding
a single npm script name in skill text.

Exact key names are design-owned; the skill MUST read them from
`[recipes.<id>.config]`.

#### Scenario: Configured smoke command is used

- GIVEN recipe config defines a UI test/smoke command
- WHEN the discipline skill says to run UI verification
- THEN the agent MUST run that configured command (not an invented default that
  ignores config)

#### Scenario: Missing command prompts discovery

- GIVEN the UI test/smoke command is unset
- WHEN an agent needs to run UI verification
- THEN the skill MUST instruct asking the user or discovering the project’s
  Playwright script and proposing a config value
- AND MUST NOT silently invent a permanent default without recording it

---

### Requirement: Relationship to test-runner

`ui-browser-testing` MUST complement `test-runner` (`tdd-flow`). Enabling UI
recipes MUST NOT disable or replace `tdd-flow`. Guidance MAY cross-link: unit /
integration via `tdd-flow`; browser UI via `ui-browser-testing`.

#### Scenario: Both capabilities can be enabled

- GIVEN a project enables `tdd-flow` and a Playwright-family recipe
- WHEN sync runs
- THEN both recipes MUST materialize successfully without fatal tag conflict
  that forces choosing only one

---

### Requirement: Evidence policy for UI verification

UI suite/smoke runs that gate merge readiness MUST leave evidence (command +
pass/fail observation) in the change’s verify/apply artifacts, PR body, or
tracker update — same spirit as `tdd-flow` evidence policy.

#### Scenario: Smoke evidence recorded

- GIVEN an agent runs the configured UI smoke/suite before claiming UI readiness
- WHEN the run completes
- THEN the agent MUST record the command used and the observed result before
  treating the UI check as done

---

### Requirement: MCP safety and secrets

If a recipe provides Playwright MCP, the preset MUST NOT embed secrets in
committed docs. Any env-backed values MUST use the project’s env-reference
conventions. Brief/MCP descriptions MUST NOT paste raw credentials.

#### Scenario: MCP preset has no literal secrets

- GIVEN a Playwright MCP `[[provides.mcp]]` block exists
- WHEN catalog and generated docs are inspected
- THEN they MUST NOT contain API keys, tokens, or password literals
