# Design: playwright-ui-flow

> Locks packaging/topology for the `ui-browser-testing` capability. Does not
> implement catalog recipes and does not enumerate tasks — those follow in
> `tasks.md`. Binding for whatever the implementer ships.
> Tracker: https://trello.com/c/QssRysPv (card 44).

## Context

Agents have no catalog recipe for Playwright-backed UI tests and smokes.
`tdd-flow` (`test-runner`) covers tool-agnostic RED/GREEN against a generic
`test_command`; `testing-foundation` covers this repo's unit/validate commands.
Neither covers browser UI suites, interactive exploration, or Playwright's two
first-class agent surfaces: **CLI** (`@playwright/cli` + the project
`playwright test` command) and **MCP** (`@playwright/mcp`).

The product north star (locked upstream) is **hybrid**: CLI for suites/smokes,
MCP for explore/debug/locators. The remaining question this design closes is
*packaging*: how many recipes, how the canonical discipline skill is shared
without triplication, and how a CLI-only project avoids materializing an MCP
server it never asked for.

Three harness facts drive every decision below (verified against
`lib/_internal/recipe-materialize.py`, `recipe-conflicts.py`,
`skill-resolution.py`):

1. **Bundled skills are physical.** `materialize_bundled_skill` copies from
   `catalog/recipes/<recipe-id>/skills/<skill-id>/`. A skill's source lives in
   exactly one recipe's tree. Two recipes cannot "point at" one shared file.
2. **Duplicate primitive ids are FATAL.** `check_recipe_conflicts` fails sync if
   two *enabled* recipes declare the same `skill`, `command`, or `mcp` id. So a
   shared discipline skill can be *provided* by only one recipe; the other
   recipes must reference it, not re-declare it.
3. **Tag overlap and capability ambiguity are WARN-only, never fatal.**
   `materialize_recipes` downgrades tag conflicts to advisories, and >1 provider
   of a capability with no `[[bindings]]` is a non-fatal ambiguity warning.
   `conflicts_with` between two enabled recipes is also only a warning today.

Consequence: the only way to satisfy "one canonical discipline skill" with the
current harness is **exactly one recipe owns the discipline skill id**; any other
recipe ships only its own distinct adapter skill id and *defers* discipline to
the shared one.

## Goals

- Lock a recipe topology that ships hybrid behavior with **zero** discipline-skill
  duplication and **zero** new sync/materialize features.
- Guarantee a CLI/suite-only project works standalone and never materializes the
  Playwright MCP server.
- Confirm the capability id and the skill ownership map so `tasks.md` can be
  written mechanically.
- Keep the design symmetric with — and a clean improvement on — the VCS sibling
  pattern (shared capability + split host skills), by extracting a real shared
  discipline skill instead of near-copy playbooks.

## Non-Goals (v1)

- Config-conditional `[[provides.mcp]]` in the sync harness. **Proven unneeded**
  (D4): packaging removes the blocker.
- Replacing or disabling `tdd-flow` / `testing-foundation`.
- Forking or reimplementing `@playwright/mcp` or `@playwright/cli`.
- A standalone "MCP-only" install that carries full suite/smoke/evidence
  discipline on its own. MCP ships as an **add-on** to the base recipe (see D1);
  the CLI-only standalone requirement from the spec is met, MCP-only is not a
  supported topology.
- Visual/regression platforms (Percy, Chromatic), Cypress/Selenium, or teaching
  the full Playwright API.

## Decisions

### D1 — Recipe topology: **Option B (2 recipes), hybrid = enable both**

Ship **two** recipes that share the `ui-browser-testing` capability by
*reference*, not by co-declaration:

| Recipe id | Role | Owns capability? | Ships MCP? |
|---|---|---|---|
| `playwright-ui-flow` | Base: canonical discipline + CLI/suite surface | **Yes** (`ui-browser-testing`) | No |
| `playwright-mcp` | Add-on: exploratory MCP surface | No | Yes (`@playwright/mcp`) |

Hybrid is **composition** (enable both). There is no third "hybrid" recipe and
no `mode` flag.

**Enablement matrix**

| Project intent | Enable | Result |
|---|---|---|
| Suites/smokes only (north-star default) | `playwright-ui-flow` | Discipline + CLI adapter skills, `ui-smoke` command, brief precedence, **no MCP server materialized** |
| Hybrid (suites + explore) | `playwright-ui-flow` + `playwright-mcp` | All of the above **plus** the MCP adapter skill and the `playwright` MCP preset |
| MCP add-on without base | `playwright-mcp` alone | Explore tools work; discipline guidance points to the base recipe (documented, not a first-class topology) |

**Why B (not A or C):**

- **Rejected A (1 recipe + `mode`).** `[[provides.mcp]]` is static. In `mode = cli`
  the Playwright MCP server would still materialize into every enabled harness,
  violating the spec's "CLI-only project still works … MUST NOT require the
  missing one." The only escapes are (a) a new conditional-MCP harness feature
  (explicit v1 non-goal) or (b) always shipping MCP even to CLI-only users
  (unwanted surface + `npx @playwright/mcp` noise). Both are worse than B.
- **Rejected C (3 recipes incl. a `playwright-hybrid`).** A hybrid recipe would
  be pure "enable the other two" glue with no primitives of its own, or it would
  re-declare skills and hit the FATAL duplicate-id rule (fact #2). It adds a
  catalog entry that documents nothing the enablement matrix doesn't. Composition
  already *is* hybrid.
- **Chosen B.** Two recipes map exactly onto the two Playwright surfaces, make
  MCP strictly opt-in (solving D4 by packaging), and keep discipline in one place
  (D3). It is the smallest topology that satisfies every locked constraint with
  the current harness.

**Why the base recipe = discipline + CLI (not a separate inert foundational
recipe):** CLI is the always-present north-star surface; a UI project that runs
suites always wants the discipline. Folding discipline into the CLI/base recipe
means the common case is **one** `recipe add`, not two. A discipline-only
foundational recipe with no runnable surface would be inert on its own (unlike
`tdd-flow`, which is runnable via `test_command`) and would force even CLI-only
users to enable two recipes. The MCP add-on, being genuinely optional, is the
natural second recipe.

### D2 — Capability: keep **`ui-browser-testing`**

Confirmed, no rename. It reads as a role ("agent-operable browser UI
verification"), not a tool, matching the `docs/capabilities.md` foundational vs
specific model. It is broad enough to admit future non-Playwright providers
without churn, and the spec (`specs/ui-browser-testing/spec.md`) already binds
requirements to this id. Narrower alternatives (`playwright-testing`,
`e2e-runner`) were rejected: the first bakes the vendor into the capability
(anti-pattern per the capabilities doc); the second overlaps semantically with
`test-runner` and invites confusion with `tdd-flow`.

Only **`playwright-ui-flow`** declares `[[capabilities]] id = "ui-browser-testing"`.
`playwright-mcp` declares **no** capability, so auto-bind stays unambiguous (one
provider → clean auto-bind, no ambiguity warning, no required `[[bindings]]`).

### D3 — Skill ownership map (one canonical discipline, thin adapters)

Three skill ids, each owned by exactly one recipe (no id ever appears in two
recipes → no FATAL conflict, no `skill-resolution` "multiple recipes" warning):

| Skill id | Kind | Owning recipe | Physical source |
|---|---|---|---|
| `ui-browser-testing` | **Canonical discipline** (when to smoke, suite vs explore, evidence policy, relationship to `tdd-flow`) | `playwright-ui-flow` | `catalog/recipes/playwright-ui-flow/skills/ui-browser-testing/` |
| `playwright-cli` | Thin CLI adapter (invocation, config command, browser install, snapshot/codegen) | `playwright-ui-flow` | `catalog/recipes/playwright-ui-flow/skills/playwright-cli/` |
| `playwright-mcp` | Thin MCP adapter (tool names, when to reach for MCP, teardown pitfalls) | `playwright-mcp` | `catalog/recipes/playwright-mcp/skills/playwright-mcp/` |

Rules that keep this honest and enforceable:

- Suite/smoke/evidence **policy lives only in `ui-browser-testing`.** Both adapter
  skills must open with a one-line "defer to the `ui-browser-testing` discipline
  skill for when/whether; this skill covers *how* on <surface>." This is directly
  testable against the spec's "tool adapters stay thin" scenario.
- The `playwright-mcp` adapter must state it augments `playwright-ui-flow` and, if
  the base is not enabled, degrade to explore-only guidance.
- **Improvement over VCS:** VCS ships three near-duplicate host skills
  (`git-merge-workflow` / `gitlab-merge-workflow` / `bitbucket-merge-workflow`)
  that each restate the merge discipline. Here the discipline body exists **once**
  (`ui-browser-testing`); `playwright-cli` and `playwright-mcp` are strictly
  surface-specific. Adding a future surface (e.g. a component-test adapter) adds
  one thin skill, not a fourth discipline copy.

`provides.skills` per recipe:

```toml
# playwright-ui-flow/recipe.toml
[provides]
skills = [
  { id = "ui-browser-testing", source = "bundled" },  # canonical discipline
  { id = "playwright-cli",      source = "bundled" },  # thin CLI adapter
]
```

```toml
# playwright-mcp/recipe.toml
[provides]
skills = [{ id = "playwright-mcp", source = "bundled" }]  # thin MCP adapter only
```

### D4 — MCP materialization for CLI-only installs: **solved by packaging**

Because the Playwright MCP preset is declared **only** in `playwright-mcp`, a
CLI-only project (`playwright-ui-flow` alone) never materializes an
`@playwright/mcp` server — `materialize_recipes` only iterates enabled recipes.
No conditional-`[[provides.mcp]]` harness feature is required, so the proposal's
"prove v1 is blocked before touching sync" bar is **not** tripped. This is the
decisive advantage of B over A and the reason the topology is B.

`playwright-mcp` ships a **static** preset. Runtime knobs (browser, headless,
base URL) are handled without new features by leaning on the existing
manifest-wins MCP merge (`build_recipe_mcp`): the recipe ships safe defaults, and
a project overrides `args`/`env` under `[mcp.playwright]` in its manifest. Per the
MCP-secrets spec requirement, the preset uses only env references (`$VAR`), never
literal credentials.

```toml
# playwright-mcp/recipe.toml
[[provides.mcp]]
id = "playwright"
command = "npx"
args = ["-y", "@playwright/mcp@latest", "--headless"]
timeout = 30000
```

### D5 — Config schema

Only the base recipe carries UI-command config (mirrors `tdd-flow`'s single
`test_command`, split into suite vs smoke). All keys `required = false` with **no
silent hardcoded default** for the run commands — the spec forbids inventing a
permanent default; `validate-config` therefore never fails on a fresh install,
and discovery is driven by `init` (D6).

`playwright-ui-flow` `[config.*]`:

| Key | Type | Required | Default | Purpose |
|---|---|---|---|---|
| `ui_test_command` | string | no | *(unset)* | Full UI suite command (e.g. `npx playwright test`) |
| `ui_smoke_command` | string | no | *(unset)* | Fast smoke subset (e.g. `npx playwright test --grep @smoke`) |
| `playwright_config` | string | no | *(unset)* | Optional path to `playwright.config.*` when non-standard |

`playwright-mcp` `[config.*]`: **none** in v1. Browser/headless/base-URL tuning
goes through the manifest `[mcp.playwright]` override path (D4), which needs no
schema. This keeps the add-on trivial and avoids implying config→MCP-arg wiring
that the harness does not perform.

Config is read by skills from `[recipes.playwright-ui-flow.config]` (spec:
"skill MUST read them from `[recipes.<id>.config]`"). When `ui_smoke_command` /
`ui_test_command` are unset, the discipline skill instructs the agent to discover
the project's Playwright script and propose a config value rather than invent one.

### D6 — Init / commands / brief (minimum set)

**Init**

- `playwright-ui-flow` `[init]`: `prompt = "init.md"`, `needs_manifest = true`.
  Guides discovery of the project's Playwright test script (package.json scripts,
  `playwright.config.*`) and proposes `ui_test_command` / `ui_smoke_command`
  values plus a `@smoke` tagging convention. Read-only/reviewable per the init
  contract.
- `playwright-mcp` `[init]`: `prompt = "init.md"`, `needs_mcp = ["playwright"]`.
  Covers `npx playwright install` (browsers), headless/browser selection via the
  manifest MCP override, and the "augments the base recipe" note.

**Commands** (minimum)

- `playwright-ui-flow`: one command `ui-smoke` (`commands/ui-smoke.md`) — run the
  configured smoke/suite command and record command + result as evidence.
- `playwright-mcp`: no slash command in v1 (MCP use is interactive; the adapter
  skill carries the explore loop). Reconsider only if a concrete need appears.

**Brief fragments** (generic; keyed inline-table form for `workflow_rules` so
hybrid dedup is stable)

- `playwright-ui-flow` → `workflow_rules` (keyed: hybrid precedence stated
  CLI-first + evidence-before-done) and `useful_commands`. The
  `useful_commands` bullet points at the config path
  (`[recipes.playwright-ui-flow.config].ui_smoke_command` / `ui_test_command`)
  rather than embedding a bare `{config.ui_smoke_command}` placeholder: on the
  default unset path the substitution helper preserves an unresolved
  `{config.KEY}` verbatim (no default is merged when the key is unset), so a raw
  placeholder would leak into `AGENTS.md`. Pointing at the config path keeps the
  bullet actionable whether or not the value is set.
- `playwright-mcp` → `workflow_rules` (keyed: explore/debug via Playwright MCP
  when enabled) and a `mcp_descriptions` entry for the `playwright` server.

Both recipes use the keyed inline-table form for `workflow_rules`
(`[[provides.brief.workflow_rules]]` with `key` + `text`, mirroring how
`mcp_descriptions` is declared) so semantic dedup across the hybrid pair is
stable regardless of enablement order.

Precedence text must match the spec's hybrid table and degrade to the enabled
subset when only one surface is present.

### D7 — Deps (`[[deps.cli]]`)

- `playwright-ui-flow`: `binary = "npx"`, `required = true`,
  `purpose = "Run the project's Playwright UI test/smoke command"`,
  `version_check = "npx --version"`, `install_url = "https://nodejs.org/en/download"`.
  `npx` is the common denominator for `npx playwright test`; browser install
  (`npx playwright install`) is runtime guidance in the CLI adapter skill, not a
  dep binary. (If a project drives suites through `pnpm`/`yarn`, that is captured
  in `ui_test_command`; `npx` remains the default runner assumption and doctor
  only WARNs.)
- `playwright-mcp`: `binary = "npx"`, `required = true`,
  `purpose = "Run the @playwright/mcp server"` (mirrors `trello-mcp-workflow`).

No `playwright` global binary is required — projects pin Playwright locally and
invoke via `npx`/scripts.

### D8 — Test strategy (for `tasks.md` to implement)

| Layer | Assertion |
|---|---|
| Schema | Both `recipe.toml` files load via `recipe_schema` (schema-valid); capability declared only on base. |
| Conflicts (hybrid) | Enabling **both** recipes yields **no FATAL** primitive conflict (distinct skill/command/mcp ids), **no** capability ambiguity (one provider), and **no** fatal tag conflict. |
| Tag hygiene | Base and MCP tags do **not** overlap, so hybrid enablement produces no tag-overlap WARN (see D-note below). |
| Materialize (CLI-only) | Enable base only → `ui-browser-testing` + `playwright-cli` skills materialize; recipe-MCP output contains **no** `playwright` server. |
| Materialize (hybrid) | Enable both → `playwright-mcp` skill + `playwright` MCP preset present. |
| Skill resolution | `ui-browser-testing` resolves exactly once; no "found in multiple recipes" warning. |
| Config | Merged config surfaces `ui_smoke_command`; `validate-config` passes with all keys unset (none required). |
| Brief render | Hybrid precedence text renders; `{config.ui_smoke_command}` substitution works. |
| Secrets | `playwright-mcp` preset and generated docs contain no literal secrets (env refs only). |
| Docs | `docs/capabilities.md` lists `ui-browser-testing` with provider `playwright-ui-flow`; `docs/recipes-catalog.md` gets both entries. |
| Cross-capability | A project enabling `tdd-flow` + `playwright-ui-flow` syncs cleanly (no forced either/or). |

### D9 — v1 non-goals (restated crisply)

No conditional-MCP harness change; no `tdd-flow`/`testing-foundation`
replacement; no `@playwright/mcp` fork; no third "hybrid" recipe and no `mode`
flag; no MCP-only standalone discipline; no visual-regression platform; no
non-Playwright UI runners.

## Skill / recipe layout sketch

```
catalog/recipes/
├── playwright-ui-flow/              # base: discipline + CLI surface
│   ├── recipe.toml                  # capability ui-browser-testing; deps npx; config; init; command; brief
│   ├── skills/
│   │   ├── ui-browser-testing/      # CANONICAL discipline skill (owned here, once)
│   │   │   └── SKILL.md
│   │   └── playwright-cli/          # thin CLI adapter (defers policy to discipline)
│   │       └── SKILL.md
│   ├── commands/
│   │   └── ui-smoke.md
│   ├── init.md
│   └── README.md                    # via [[provides.docs]]
└── playwright-mcp/                  # add-on: MCP surface only
    ├── recipe.toml                  # provides.mcp playwright; deps npx; init; brief; NO capability
    ├── skills/
    │   └── playwright-mcp/          # thin MCP adapter (defers policy to discipline)
    │       └── SKILL.md
    ├── init.md
    └── README.md
```

Note the asymmetry is intentional: the discipline skill physically lives in the
base recipe's tree (fact #1), and `playwright-mcp` references it by name from its
adapter skill — it never re-ships it.

## Config / init sketch (TOML)

`catalog/recipes/playwright-ui-flow/recipe.toml` (shape):

```toml
[recipe]
id = "playwright-ui-flow"
name = "Playwright UI Flow"
description = "Playwright UI test/smoke discipline + CLI surface for agents"
version = "1.0.0"
author = "ai-specs"
license = "MIT"
tags = ["ui-testing"]

[[capabilities]]
id = "ui-browser-testing"

[[hooks]]
event = "on-sync"
action = "validate-config"

[[deps.cli]]
binary = "npx"
purpose = "Run the project's Playwright UI test/smoke command"
required = true
install_url = "https://nodejs.org/en/download"
version_check = "npx --version"

[config.ui_test_command]
required = false
type = "string"
help_text = "Full UI suite command (e.g. `npx playwright test`). Leave unset to have init discover it."

[config.ui_smoke_command]
required = false
type = "string"
help_text = "Fast smoke subset (e.g. `npx playwright test --grep @smoke`)."

[config.playwright_config]
required = false
type = "string"
help_text = "Optional path to playwright.config.* when non-standard."

[init]
prompt = "init.md"
description = "Discover the project's Playwright test/smoke command and propose config"
needs_manifest = true

[provides]
skills = [
  { id = "ui-browser-testing", source = "bundled" },
  { id = "playwright-cli",      source = "bundled" },
]
commands = [{ id = "ui-smoke", path = "commands/ui-smoke.md" }]

[provides.brief]
useful_commands = [
  "Run UI smokes via the command configured at `[recipes.playwright-ui-flow.config].ui_smoke_command` (or `ui_test_command` for the full suite); if unset, discover the project's Playwright script and propose a config value.",
]

[[provides.brief.workflow_rules]]
key = "ui-suites-via-cli"
text = "Run UI suites and smokes via the CLI/project test command; use the browser MCP only for exploratory navigation, debugging, and locator discovery."

[[provides.brief.workflow_rules]]
key = "ui-evidence-before-done"
text = "Record the UI smoke/suite command and its pass/fail result as evidence before treating UI verification as done."

[[provides.docs]]
source = "README.md"
target = "ai-specs/recipes/playwright-ui-flow/README.md"
```

`catalog/recipes/playwright-mcp/recipe.toml` (shape):

```toml
[recipe]
id = "playwright-mcp"
name = "Playwright MCP"
description = "Exploratory browser automation surface via @playwright/mcp (augments playwright-ui-flow)"
version = "1.0.0"
author = "ai-specs"
license = "MIT"
tags = ["mcp", "browser-automation"]

[[deps.cli]]
binary = "npx"
purpose = "Run the @playwright/mcp server"
required = true
install_url = "https://nodejs.org/en/download"
version_check = "npx --version"

[init]
prompt = "init.md"
description = "Install browsers and select headless/browser via the manifest MCP override"
needs_mcp = ["playwright"]

[provides]
skills = [{ id = "playwright-mcp", source = "bundled" }]

[provides.brief]

[[provides.brief.workflow_rules]]
key = "ui-explore-via-mcp"
text = "For interactive UI exploration, debugging, or locator discovery, prefer the Playwright MCP tools over ad-hoc browser drivers."

[[provides.brief.mcp_descriptions]]
key = "playwright"
text = "Interactive browser automation for UI exploration, debugging, and locator discovery."

[[provides.mcp]]
id = "playwright"
command = "npx"
args = ["-y", "@playwright/mcp@latest", "--headless"]
timeout = 30000

[[provides.docs]]
source = "README.md"
target = "ai-specs/recipes/playwright-mcp/README.md"
```

Manifest usage:

```toml
# CLI/suite-only
[recipes.playwright-ui-flow]
enabled = true
[recipes.playwright-ui-flow.config]
ui_test_command  = "npx playwright test"
ui_smoke_command = "npx playwright test --grep @smoke"

# Hybrid: add the MCP surface
[recipes.playwright-mcp]
enabled = true
# Optional: override browser/headless without any new schema
[mcp.playwright]
args = ["-y", "@playwright/mcp@latest", "--browser", "chromium"]
```

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Adapter skills drift back into restating discipline (duplication creeps in) | Spec scenario + D8 test: each adapter must open by deferring to `ui-browser-testing`; review adapters for policy leakage. |
| Base + MCP share a tag → noisy WARN on every hybrid sync | D1/D8: give non-overlapping tags (`ui-testing` vs `mcp`/`browser-automation`); do **not** set `conflicts_with` (they compose). |
| Base + `tdd-flow` share a `quality` tag → WARN on the common `tdd-flow` + `playwright-ui-flow` combo | The base recipe ships `tags = ["ui-testing"]` only (drops `quality`): `tdd-flow` already owns `quality`, and both are commonly enabled together, so a shared `quality` tag would fire a tag-overlap WARN on every such sync. `ui-testing` is specific to this recipe and does not collide with `tdd-flow`. |
| User enables `playwright-mcp` without the base and expects full discipline | MCP adapter skill + README state it augments `playwright-ui-flow`; brief degrades to explore-only. |
| Static MCP `@playwright/mcp@latest` version drift | Pin acceptable range in the preset later; v1 tracks `@latest` like `trello-mcp-workflow` and documents manifest override. |
| Project drives suites via `pnpm`/`yarn`, not `npx` | Actual command lives in `ui_test_command`; `npx` dep is a doctor WARN only, never blocks sync. |
| Consumers expect MCP to auto-tune from config | D4/D5: documented that browser/headless tuning is a manifest `[mcp.playwright]` override, not recipe config — no false wiring promise. |

## Open questions

None blocking. Deferred, non-blocking nits for `tasks.md`/authoring:
whether to also ship a `ui-test` (full-suite) command alongside `ui-smoke`;
exact `@smoke` tagging convention wording; whether to pin a Playwright MCP
version range instead of `@latest`.
