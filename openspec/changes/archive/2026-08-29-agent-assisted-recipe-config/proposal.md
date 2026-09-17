# Proposal: agent-assisted recipe configuration

## Why

Agents and humans can list/add recipes and open an interactive wizard, but there
is no guided natural-language path that **grounds** a recommendation in the
actual repository, **applies** config safely, **preserves** existing values and
overrides, **runs/verifies sync**, and **reports** assumptions and drift.

Literacy skills (`harness-lifecycle`, `harness-recipes`) teach order-of-ops;
`recipe init` is read-only; `configure-recipes` is TTY-bound; per-recipe
`init.md` coverage is sparse (3 of 11 catalog recipes) and currently forbids
auto-sync. Consumer pain (including Melón/Alquimia-style monorepo-submodules)
shows agents invent partial edits and skip verify.

Planning also uncovered two **behavioral defects** in the primitive this
capability must build on (`update_recipe_config`), reproduced in this worktree —
see `design.md` › "Defects reproduced during planning". Skill prose cannot fix
them; that is what settles the delivery-mode question below.

## What changes (capability outcomes)

Deliver an **agent-assisted recipe configuration** capability such that:

1. A natural-language request ("configure worktree-flow for this monorepo",
   "set up trello recipe") starts a structured assisted flow.
2. The agent inspects repository + manifest + recipe schema state **through a
   deterministic non-interactive helper** and produces a **grounded
   recommendation** (proposed keys, rationale, assumptions).
3. On approval/apply, canonical config under `[recipes.<id>.config]` updates
   **idempotently** via the surgical write path — a semantic no-op leaves the
   manifest **byte-identical**, and inline comments survive key replacement.
4. Existing config keys not in the recommendation, comments, and override
   trees are **preserved** (no force-overwrite of user overrides — #63 owns
   deeper lock governance).
5. The flow **runs sync** (when apply is authorized) and **verifies** via sync
   exit status plus `doctor`/lock signals, with explicit **partial-failure**
   semantics (sync does not roll back).
6. The agent emits a **structured closing report** covering applied changes,
   unresolved assumptions, configuration drift, and version/synchronization
   gaps — including the two distinct `cli_version` drift classes.
7. Behavior is documented and covered by **two distinct evidence tiers**:
   deterministic unit tests (`./tests/run.sh`) and **real-runtime agent evals**
   added as a new client on the existing `tests/evals/` system, whose scenario
   contract, fixtures, assertions, and isolation are unchanged.

## Locked decisions (human-authorized planning baseline)

| Decision | Choice | Notes |
|---|---|---|
| Depth | **Full** | New capability; cross-cutting literacy + config + sync |
| **Delivery mode** | **Minimum non-interactive helper** (not skill-only) | Human decision. Skill prose cannot guarantee determinism, byte-identical no-op, or comment preservation; the two reproduced defects require code |
| **MVP scope** | **Broader** (3 evidence recipes, 5 eval scenarios) | Human decision, taken over the evaluator's narrower recommendation; bounded explicitly below |
| Apply mechanism | Surgical config write | Extend `update_recipe_config` semantics; no full-file TOML round-trip |
| **Runtime evidence** | **A new client on the existing, unchanged `tests/evals/` system** | Human decision. Scenarios, fixtures, assertions, isolation, and runners keep their canonical semantics; no new eval platform, no browser/UI automation |
| **Orchestration layer** | **Optional, additive** | Orca/OMP is the IDE/harness where an orchestration skill is installed; it lets a parent agent drive the *existing* eval runners across several real runtimes (`cursor-agent`, `claude`, a separate OMP session) and compare them. It is **not** a runtime, **not** an eval runner, and **never** required to run the evals |
| Override / lock ownership | **Out of scope** | Preserve; report only. Card #63 owns provenance/force-update |
| Interactive wizard | **Behavior preserved** | `ai-specs configure-recipes` keeps its current TTY UX; helper is a sibling entry point |
| Topology | Generalizable | `resolve_repo_topology` + config schema as grounding; `init.md` is optional enrichment, never a prerequisite; no hardcoded Alquimia paths |
| Secrets | Never literal | Keep `${env:…}` / redaction conventions from recipe-init |

### What "broader MVP" includes — and where it stops

**Includes** (evidence set):

| Recipe | Why it is in the set | Class it proves |
|---|---|---|
| `worktree-flow` | Ships **no** `init.md`; has `[config.repo_topology]` enum | Topology grounding without an init contract |
| `trello-mcp-workflow` | Ships `init.md`; needs MCP + env-backed secrets | MCP/deps grounding + no-secret-literals |
| `plan-build-flow` | Plain scalar/enum config, no MCP | Baseline apply/no-op/preserve path |

**Stops at** (explicit bounds on the broader scope):

- No #63 governance: no per-override hashes in `.ai-specs.lock`, no
  force-update of user-modified overrides, no per-artifact governance
  categories.
- No change to interactive `configure-recipes` behavior.
- No `--json` mode added to `ai-specs doctor` (shared surface; the helper
  parses doctor's existing summary line and reports `parsed: false` when it
  cannot).
- No new `init.md` files authored for recipes that lack one.
- No new eval platform, runner, scoring service, or change to the canonical
  eval system (scenario contract, fixtures, assertions, isolation).
- No mandatory runtime: the evals stay runnable from a plain shell with no
  orchestration skill present.

## Non-goals

- Implementing override lock provenance or force-refresh policy (#63).
- Wrapping the entire CLI as an MCP.
- Replacing interactive `configure-recipes` for humans (additive only).
- Requiring every catalog recipe to gain a full `init.md` in this change.
- Changing sync materialization semantics, conflict resolution, or capability
  binding algorithms except as needed to *invoke* them correctly.
- Per-project `ai-specs/bin` shims.
- Browser/UI-driven evaluation.

## Impact / likely surfaces

| Area | Touch? | Notes |
|---|---|---|
| `lib/_internal/recipe-config-write.py` | **Yes** | Fix inline-comment loss + byte-identical no-op (shared with wizard) |
| New `lib/_internal/recipe-configure.py` | **Yes** | Minimum non-interactive helper (inspect / apply / report) |
| `lib/recipe.sh` | **Yes** | Register `recipe configure` subcommand + usage |
| Bundled skills `harness-recipes` / `harness-lifecycle` | Yes | Playbook that calls the helper |
| Docs (recipes catalog / troubleshooting / capabilities) | Yes | Document the assisted flow + two evidence tiers |
| `tests/test_recipe_config_write.py`, `tests/test_config_wizard.py` | Yes | Regression for the two defects |
| New `tests/test_recipe_configure.py` | Yes | Helper deterministic contract |
| `tests/evals/lib/project_fixture.py` | Yes | **Strictly additive** `setup_bundled_skills`; existing resolver reaches catalog recipe skills only. No existing caller or behavior changes |
| New `tests/evals/eval_assisted_configure_live.py` + scenarios + runner | Yes | New eval **client**, following the existing per-capability convention |
| `ai-specs configure-recipes` / `config_wizard.py` behavior | **No** | Preserved (benefits from the write-path fix only) |
| `lib/_internal/doctor.py` | **No** | Parsed, not modified |
| Override lock / materialize force-update | **No** | #63 |

## Success criteria (acceptance mapping)

| Acceptance outcome | How we will know |
|---|---|
| NL request | Skill/auto_invoke + documented entry phrases; runtime eval `ac_recommend_stops_before_apply` |
| Grounded recommendation | Helper `inspect --json` emits the cited signals; eval asserts recommendation cites detected topology |
| Idempotent canonical update | Unit test: no-op apply leaves manifest byte-identical and reports `status="no-op"` |
| Preserve config/overrides | Unit tests: unmentioned keys, own-line comments, **inline comments**, override files untouched |
| Run + verify sync | Helper `--sync` records exit code, failed step, `lock_stamped`; partial failure reported as `partial`, never `ok` |
| Report assumptions/drift/gaps | Versioned JSON report schema in spec; both `cli_version` drift classes represented |
| Documented + validated | Docs updated; `./tests/run.sh` + `./tests/validate.sh` green; live eval evidence recorded in `verify-report.md` |

## Resolved open decisions

| Was open | Resolution |
|---|---|
| Skill-only MVP vs non-interactive apply CLI | **Helper ships in this change** (human decision) |
| Spec amendment strategy for sync invocation | **Add-only**: assisted-configure requirements; `recipe-init-contract` propose-only posture untouched |
| MVP recipe evidence set | **Broader**: `worktree-flow` + `trello-mcp-workflow` + `plan-build-flow` |
| Machine-readable recommendation for tests | **Yes** — `--json` with a versioned, deterministic schema |
| Evaluation approach | **New client on the existing `tests/evals/` system**, unchanged semantics; optional orchestration skill for cross-runtime comparison |

## Tracker

- **card_id**: `6a72b44828a5b2547f679116`
- **shortLink**: `GjfV4sKA`
- **url**: https://trello.com/c/GjfV4sKA
- **card**: #62 Agent-assisted recipe configuration
- **branch / worktree**: `change/agent-assisted-recipe-config` / `.worktrees/agent-assisted-recipe-config`
- **base**: `development` @ `12afc3f`
