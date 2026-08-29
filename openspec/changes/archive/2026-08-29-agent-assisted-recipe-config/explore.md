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

## Decision lean, first pass (superseded — see "Open questions — resolved")

Preferred **D (hybrid)** as the planning baseline. Retained for provenance; the
helper is now locked in rather than conditional:

- Primary UX: agent skill/playbook driven by NL + repo inspection.
- Apply path: surgical manifest updates via existing write helper (or a thin non-interactive wrapper around it) — never wholesale TOML rewrite.
- Sync: **authorized apply phase** may run/verify sync (reconcile with init-contract by scoping the exception to this assisted flow, not silent init).
- Preserve: leave unknown keys, comments, and override files untouched; do not implement #63 lock governance.
- Scenario: include Melón/Alquimia-style topology grounding as a **fixture/example**, not as hardcoding of Alquimia paths.

## Second-pass grounding (post-evaluation, @ this worktree)

Findings that changed the plan. All verified in this worktree; none required
production edits.

### A runtime eval harness already exists — reuse it

`tests/evals/` is a full slow-tier harness, deliberately excluded from
`./tests/run.sh` by the `eval_*.py` naming (`unittest discover -p 'test_*.py'`
never loads it):

- `SUPPORTED_RUNTIMES = ("claude", "cursor-agent", "opencode", "pi", "omp")` —
  **`omp` is already wired** (`omp -p --mode json --model <m> --no-session
  --cwd <root> --no-extensions --approval-mode yolo`, NDJSON-parsed).
- Per-capability opt-in runners (`run-live-worktree.sh`, `run-live-vcs.sh`, …)
  plus a dry `run.sh`; `EVALS_LIVE`, `EVALS_RUNTIMES`, `EVALS_SCENARIOS`,
  `EVALS_TRIALS` (N-of-M), `EVALS_TIMEOUT_SEC`, `EVALS_MAX_TURNS`.
- Scenario contract: `scenario.toml` with `required_path_globs`,
  `forbidden_path_globs`, `absent_path_globs`, `required_content`,
  `forbidden_phrases`, `fixture`, `mode`; `assert_natural_prompt` rejects
  meta-prompts so prompts stay real user sentences.
- Isolation: `tempfile.TemporaryDirectory()` + `init_git_repo` + baseline
  commit + `git_paths_changed` — fixtures live outside the repo entirely.
- Everything is a CLI process. No browser, no UI automation anywhere.

The corollary the plan takes seriously: this system is **canonical and stays
unchanged**. The capability adds a client (module + runner + scenarios) exactly
like every other capability, plus one strictly additive fixture helper. Any
cross-runtime orchestration sits *above* the runners and changes none of the
scenario, fixture, assertion, or isolation semantics.

**Gap:** `setup_runtime_skills` resolves skills from `catalog/recipes/<id>/
skills/` (or the materialized `.recipe` dir) only. The assisted flow ships in
`bundled-skills/harness-recipes`, which that resolver cannot reach — a small
additive `setup_bundled_skills` is required.

### Two defects reproduced in the primitive this capability depends on

Probe against `lib/_internal/recipe-config-write.py` on a temp manifest:

1. **Inline comments are destroyed on key replacement.** Replacement rebuilds
   the whole line as `f"{indent}{key} = {value}\n"`.
   `integration_branch = "main"  # team decision` → `= "development"`, comment
   gone. `tests/test_recipe_config_write.py::test_replace_existing_key` seeds
   exactly such a comment and asserts only the *own-line* one survives, which
   is how this shipped unnoticed.
2. **A semantic no-op is not byte-identical.** Applying values already equal to
   the effective config still rewrites the lines, normalizing
   `worktrees_dir='.worktrees'` → `worktrees_dir = ".worktrees"`. Probe printed
   `true no-op byte-identical: False`. Convergent re-apply (same values twice
   in a row) *is* stable — a weaker property than the acceptance criterion.

These are code properties; skill prose cannot fix them. They are the technical
basis for the human's "helper, not skill-only" decision.

### `worktree-flow` ships no `init.md`

Only `playwright-mcp`, `playwright-ui-flow`, and `trello-mcp-workflow` do (3 of
11). `worktree-flow` has `README.md`, `commands`, `hooks`, `recipe.toml`,
`skills`, `templates`. Its `[config.repo_topology]` enum
(`auto|standalone|monorepo-apps|monorepo-submodules`) plus `help_text`, and
`util.resolve_repo_topology`, are therefore the grounding sources — an
`init.md` must never be a precondition.

Sharp edge: `resolve_repo_topology` documents that `auto` **never** resolves to
`monorepo-apps`; without `.gitmodules` it returns `standalone`. An apps-style
monorepo is indistinguishable from standalone by detection, so it has to be an
explicit question rather than a silent default.

### Sync and `cli_version` semantics are sharper than assumed

From `lib/sync.sh` and `lib/_internal/cli_version.py`:

- `cli_version.py check-sync` runs **before any write** and `|| exit 1`. A
  `[tool]` pin violation aborts sync with zero side effects — so the assisted
  flow must gate on it *before* apply, or it strands an edited manifest that
  sync refuses to process.
- Target-loop failure prints "Stopped on first failure; previous writes are not
  rolled back" and exits 1; `stamp-meta` runs only after every step succeeds,
  so a partial sync leaves the lock unstamped. Partial is a real third state.
- Lock staleness (`[meta].cli_version` != installed) is a doctor **WARN** only;
  sync proceeds and restamps. Distinct class from the pin violation.
- `doctor.py` has no JSON mode; it is line-oriented with a final
  `Summary: N OK, N INFO, N WARN, N ERROR`, and exits non-zero only on ERROR.

## Open questions — resolved

| Question | Resolution | Source |
|---|---|---|
| Skill-only vs non-interactive apply CLI | **Helper ships in this change** | Human decision + reproduced defects |
| Amend `recipe-init-contract` vs separate contract | **Add-only** assisted-configure requirements; init posture untouched | Design |
| Recipe coverage MVP | **Broader**: `worktree-flow` + `trello-mcp-workflow` + `plan-build-flow` | Human decision |
| Recommendation artifact shape | **Machine-readable `--json`**, versioned and deterministic | Testability |
| How much doctor/lock drift is enough | Two `cli_version` classes; doctor parsed not modified; `parsed: false` when unreadable | Grounded semantics above |
| How to evidence runtime behavior | **New client on the existing, unchanged `tests/evals/` system**, 5 scenarios, any supported runtime (none mandated) | Human decision + harness discovery |
| How to compare runtimes | **Optional** Orca/OMP orchestration skill that only invokes the existing runners and aggregates per runtime — not a runtime, not a runner, not required | Human correction |

## Ready for proposal

Yes — problem and acceptance outcomes are clear; current surfaces, defects, and
harness capabilities are grounded; delivery mode and scope are human-locked.