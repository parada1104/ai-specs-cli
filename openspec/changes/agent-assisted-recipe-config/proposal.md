# Proposal: agent-assisted recipe configuration

## Why

Agents and humans can list/add recipes and open an interactive wizard, but there
is no guided natural-language path that **grounds** a recommendation in the
actual repository, **applies** config safely, **preserves** existing values and
overrides, **runs/verifies sync**, and **reports** assumptions and drift.

Literacy skills (`harness-lifecycle`, `harness-recipes`) teach order-of-ops;
`recipe init` is read-only; `configure-recipes` is TTY-bound; per-recipe
`init.md` coverage is sparse and currently forbids auto-sync. Consumer pain
(including Melón/Alquimia-style monorepo-submodules) shows agents invent
partial edits and skip verify.

## What changes (capability outcomes)

Deliver an **agent-assisted recipe configuration** capability such that:

1. A natural-language request ("configure worktree-flow for this monorepo",
   "set up trello recipe") starts a structured assisted flow.
2. The agent (or thin helper it calls) inspects repository + manifest + recipe
   schema state and produces a **grounded recommendation** (proposed keys,
   rationale, assumptions).
3. On approval/apply, canonical config under `[recipes.<id>.config]` updates
   **idempotently** via the existing surgical write path (or a thin wrapper).
4. Existing config keys not in the recommendation, comments, and override
   trees are **preserved** (no force-overwrite of user overrides — #63 owns
   deeper lock governance).
5. The flow **runs sync** (when apply is authorized) and **verifies** via
   sync exit status and/or `doctor` (or equivalent project conventions).
6. The agent emits a closing report covering unresolved assumptions,
   configuration drift signals, and version/synchronization gaps (at least
   CLI lock `cli_version` drift where doctor already surfaces it).
7. Docs + validation conventions cover the behavior (skill/docs + tests under
   `./tests/run.sh` / `./tests/validate.sh` as appropriate).

## Locked decisions (planning baseline)

| Decision | Choice | Notes |
|---|---|---|
| Depth | **Full** | New capability; cross-cutting literacy + config + sync |
| Delivery baseline | **Hybrid** | Skill-orchestrated NL flow + reuse of inspect/write/sync/doctor primitives; add CLI only if skill-only cannot meet testable acceptance |
| Apply mechanism | Surgical config write | Prefer `update_recipe_config` semantics; no full-file TOML round-trip |
| Override / lock ownership | **Out of scope** | Preserve; report only. Card #63 owns provenance/force-update |
| Topology | Generalizable | Use `repo_topology` / detection as grounding signal when present; do not hardcode Alquimia paths |
| Secrets | Never literal | Keep `${env:…}` / redaction conventions from recipe-init |

## Non-goals

- Implementing override lock provenance or force-refresh policy (#63).
- Wrapping the entire CLI as an MCP.
- Replacing interactive `configure-recipes` for humans (may remain; assisted flow is additive).
- Requiring every catalog recipe to gain a full `init.md` in this change (MVP may seed evidence recipes; broader coverage can follow).
- Changing sync materialization semantics, conflict resolution, or capability binding algorithms except as needed to *invoke* them correctly.
- Per-project `ai-specs/bin` shims.

## Impact / likely surfaces (indicative — design refines)

| Area | Likely touch? | Notes |
|---|---|---|
| Bundled skills `harness-recipes` / `harness-lifecycle` | Yes | Playbook for NL assisted configure |
| Thin CLI / helper for inspect or non-interactive apply | Maybe | Only if needed for idempotent, testable apply |
| `recipe-init-contract` / init Post-write | Maybe | Reconcile "no auto-sync" with authorized assisted apply |
| Docs (recipes / troubleshooting / capabilities) | Yes | Document the assisted flow |
| Tests / validate | Yes | Unit and/or skill-content / CLI contract tests |
| Catalog `init.md` for seed recipes | Optional | Evidence for grounded Q&A without claiming universal coverage |
| Override lock / materialize force-update | No | #63 |

## Success criteria (acceptance mapping)

| Acceptance outcome | How we will know |
|---|---|
| NL request | Skill/auto_invoke + documented entry phrases; agent follows playbook |
| Grounded recommendation | Recommendation cites inspected signals (topology, existing config, schema, MCP/deps as relevant) |
| Idempotent canonical update | Re-apply same recommendation → no spurious churn; surgical write preserves comments |
| Preserve config/overrides | Untouched keys and override files remain; tests assert non-clobber |
| Run + verify sync | Playbook (and any helper) requires sync + verification step with recorded outcome |
| Report assumptions/drift/gaps | Structured closing report fields defined in spec |
| Documented + validated | Docs updated; tests green under project runners |

## Open decisions (need auth / apply-time evidence)

1. Skill-only MVP vs add non-interactive apply CLI in the same change.
2. Spec amendment strategy for sync invocation vs leaving init-contract untouched and defining a separate assisted-configure contract.
3. MVP recipe evidence set (`worktree-flow` ± `trello-mcp-workflow` vs broader).
4. Whether recommendation output needs a stable machine-readable form for tests.

## Tracker

- **card_id**: `6a72b44828a5b2547f679116`
- **shortLink**: `GjfV4sKA`
- **url**: https://trello.com/c/GjfV4sKA
- **card**: #62 Agent-assisted recipe configuration
- **branch / worktree**: `change/agent-assisted-recipe-config` / `.worktrees/agent-assisted-recipe-config`
- **base**: `development` @ `12afc3f`
