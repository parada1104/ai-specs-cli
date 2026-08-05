# Exploration: agent-assisted recipe configuration

> Change slug: `agent-assisted-recipe-config`
> Tracker: Trello #62 (`GjfV4sKA` / `6a72b44828a5b2547f679116`)
> Depth signal: **full** — new cross-cutting agent capability; ambiguous delivery surface; card explicitly defers design to exploration.

## Problem (from card — follow-up standard)

Users need a guided way to configure ai-specs recipes through agents. Today, successful setup still depends on deep knowledge of repository topology, manifest structure, recipe contracts, and synchronization behavior.

## Acceptance outcomes (from card)

1. User can request recipe configuration in natural language.
2. Agent produces a **grounded** recommendation from repository state.
3. Applying the recommendation updates canonical project configuration **idempotently**.
4. Existing configuration and overrides are **preserved**.
5. Agent **runs and verifies** the required synchronization flow.
6. Agent reports unresolved assumptions, configuration drift, and version/synchronization gaps.
7. Behavior is documented and covered by the project's existing validation conventions.

## Target scenario (motivating, not exclusive)

Melón / Alquimia-style **monorepo-submodules** consumers (lived evidence from archived
`worktree-flow-repo-topology`): agents mis-configure topology or skip sync/verify
because the interactive wizard + literacy skills do not form a closed NL loop.
Capability MUST remain generalizable to standalone / monorepo-apps and to other
catalog recipes (tracker, VCS, vault, plan-build, etc.).

## Current state (grounded in this worktree @ 12afc3f)

### Surfaces that already exist

| Surface | Role today | Agent / NL fitness |
|---|---|---|
| `harness-lifecycle` / `harness-recipes` (bundled literacy) | Order-of-ops, pitfalls, when to sync/doctor | Teaches *commands*, not grounded recommend→apply→verify |
| `ai-specs recipe list \| add` | Discover / declare recipes in manifest | Declarative only; no config fill |
| `ai-specs recipe init <id>` | Read-only setup brief (`recipe-init.py`) | Grounding helper; does not write |
| Recipe `init.md` contracts (`recipe-init-contract`) | Agent-executable Q&A → TOML target | Only 3 recipes ship `init.md` (trello, playwright-*); **Post-write forbids auto-sync** |
| `ai-specs configure-recipes` → `config_wizard.py` | Interactive questionary over `config_schema` | TTY-bound; poor for agents / NL |
| `recipe-config-write.update_recipe_config` | Surgical, comment-preserving `[recipes.<id>.config]` writes | Strong **idempotent apply** primitive already |
| `ai-specs sync` + `doctor` | Materialize + health / CLI version drift via `.ai-specs.lock` `[meta].cli_version` | Verify building blocks exist; not orchestrated for this flow |
| Overrides / `condition = "not_exists"` | Preserve consumer customizations | Sibling card **#63** owns lock provenance / force-update governance — out of scope here |

### Gaps relative to acceptance

1. **No closed NL loop** — literacy stops at "run configure-recipes"; wizard is interactive.
2. **Grounding is fragmented** — topology (`resolve_repo_topology`), existing config, MCP presence, CLI deps, lock version, and stale overrides are inspectable but not assembled into one recommendation artifact.
3. **Sync policy conflict** — `recipe-init-contract` says agents SHALL NOT invoke sync; card acceptance requires run + verify sync. Must reconcile deliberately (likely: distinguish *propose-only init* vs *authorized configure apply*).
4. **Coverage holes** — most recipes lack `init.md`; agents invent TOML edits.
5. **Reporting** — doctor reports some drift; no standard agent report shape for assumptions / config drift / version gaps after an assisted configure.

### Related / non-overlapping work

| Card / change | Boundary |
|---|---|
| #43 `agent-cli-literacy` (archived) | Literacy skills exist; this change *uses / extends* them, does not redo literacy from scratch |
| #63 Override ownership and lock governance (`wdwyRFTS`) | Owns override provenance + force-update policy; this change **preserves** overrides and may *report* suspected drift only |
| #59 / #60 plan-build-flow follow-ups | Orthogonal (classifier / artifact gates) |

## Approaches (options — not locked)

| # | Approach | Pros | Cons / risks |
|---|---|---|---|
| A | **Skill playbook only** — extend `harness-recipes` (+ lifecycle cross-links) with a recommend→apply→sync→report procedure using existing CLI | Smallest code surface; ships via refresh-bundled | Soft enforcement; agents may still skip steps; hard to unit-test end-to-end |
| B | **Non-interactive CLI** — e.g. inspect / recommend / apply flags on configure or a new subcommand, then skill wraps it | Testable, idempotent, agent-safe | Larger CLI contract; design must avoid duplicating wizard |
| C | **Expand `init.md` + revise sync rule** — more contracts; allow sync under assisted-configure after user confirm | Reuses existing contract shape | Most recipes still lack contracts; sync-rule change is a spec amendment |
| D | **Hybrid (leaning recommended)** — skill-orchestrated NL flow + thin **grounding/apply helpers** (reuse `update_recipe_config`, topology helper, doctor/sync) with explicit report schema; CLI flags only where evidence shows skill-only is insufficient | Matches acceptance; reuses primitives; keeps #63 boundary | Needs careful non-goals so apply does not invent lock format or MCP wrappers |

## Decision lean (for proposal — still authorizable)

Prefer **D (hybrid)** as the planning baseline:

- Primary UX: agent skill/playbook driven by NL + repo inspection.
- Apply path: surgical manifest updates via existing write helper (or a thin non-interactive wrapper around it) — never wholesale TOML rewrite.
- Sync: **authorized apply phase** may run/verify sync (reconcile with init-contract by scoping the exception to this assisted flow, not silent init).
- Preserve: leave unknown keys, comments, and override files untouched; do not implement #63 lock governance.
- Scenario: include Melón/Alquimia-style topology grounding as a **fixture/example**, not as hardcoding of Alquimia paths.

## Open questions for proposal / auth

1. Delivery: skill-only first vs ship a minimal non-interactive apply CLI in the same change?
2. Sync: amend `recipe-init-contract` Post-write, or introduce a separate "assisted configure" contract that permits sync after explicit user approval?
3. Recipe coverage MVP: all enabled recipes vs seed with `worktree-flow` + one MCP recipe (`trello-mcp-workflow`) as evidence?
4. Recommendation artifact: ephemeral chat only vs optional machine-readable stdout for tests?
5. How much of doctor/lock version drift is "good enough" vs new report fields?

## Ready for proposal

Yes — problem and acceptance outcomes are clear; current surfaces and gaps are grounded; approach options are framed without locking file-level implementation.