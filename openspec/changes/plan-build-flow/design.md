# Design: plan-build-flow — a two-verb catalog recipe over hidden ceremony

## 1. Architecture Approach

**Chosen pattern: skill/instructions-only catalog recipe (exploration Approach 1).**

The recipe is a pure *content bundle* materialized by the existing `ai-specs sync`
pipeline. It adds no Python, no schema fields, no on-sync action, no materializer
branch. Every behavior lives inside artifacts the current pipeline already knows
how to place: a bundled skill, two command prompt files, a README doc, and
`[provides.brief]` fragments.

**Layering.** Three concentric layers, from most durable to most volatile:

1. **Contract layer (`recipe.toml`)** — declares WHAT the recipe provides and the
   only config knob. Must validate against `lib/_internal/recipe_schema.py`
   unmodified. This is the stable interface consumers pin a version against.
2. **Behavior layer (`SKILL.md`)** — the auto-invoked policy that maps `plan`/`build`
   to the underlying phased ceremony, and encodes degradation, slug derivation, and
   artifact-store defaults. This is where "the intelligence" lives.
3. **Surface layer (`commands/plan.md`, `commands/build.md`, `README.md`)** — the
   developer-facing entry points and the human-readable explanation.

**Why this pattern.** It is the ONLY option that satisfies both hard constraints
simultaneously: (a) "catalog recipe, not core CLI logic" and (b) the archived
`2026-05-18-docs-remove-sdd-refocus` boundary. Any approach that adds a config
section, a new on-sync action, or shells out to `gentle-ai` reintroduces coupling
the proposal explicitly deferred. The recipe wraps an *external* orchestrator by
naming convention only; it ships zero ceremony logic inside `ai-specs-cli`.

**Boundary of correctness.** `ai-specs-cli` can only guarantee *materialization*
(files land in the right place with the right content). It cannot test whether an
external agent honors the phase mapping, because orchestration lives in gentle-ai.
This boundary is deliberate and is reflected in the test strategy (Section 7).

## 2. Component Map

```
catalog/recipes/plan-build-flow/
├── recipe.toml                                  # contract layer
├── skills/plan-build-flow/SKILL.md              # behavior layer (bundled skill)
├── commands/plan.md                             # surface: /plan prompt
├── commands/build.md                            # surface: /build prompt
└── README.md                                    # surface: materialized doc
```

Materialization targets in a consumer project (derived from the sync pipeline,
confirmed via `tests/test_tdd_flow_recipe.py` and `tests/test_worktree_flow_recipe.py`):

| Source in recipe | Materialized target in consumer |
|---|---|
| `skills/plan-build-flow/SKILL.md` | `ai-specs/.recipe/plan-build-flow/skills/plan-build-flow/SKILL.md` |
| `commands/plan.md` | `ai-specs/commands/plan.md` (→ `/plan`) |
| `commands/build.md` | `ai-specs/commands/build.md` (→ `/build`) |
| `README.md` | `ai-specs/recipes/plan-build-flow/README.md` |
| `[provides.brief]` fragments | merged into generated `AGENTS.md` |

## 3. Data Flow

### `/plan` invocation
```
developer types /plan <intent>
  → agent loads commands/plan.md prompt
  → prompt defers to the plan-build-flow SKILL for the phase mapping + policies
  → agent derives change-slug from <intent>
  → agent resolves artifact store (preflight? else OpenSpec files)
  → runs explore → proposal → spec → design → tasks
      (orchestrated by gentle-ai if present; inline single-conversation if absent)
  → produces reviewable artifacts; STOPS for human review/authorization
```

### `/build` invocation
```
developer types /build [slug]
  → agent loads commands/build.md prompt
  → prompt defers to the plan-build-flow SKILL
  → agent resolves the same change-slug and artifact store used by /plan
  → (worktree-flow enabled? → ensure work happens in a worktree)
  → runs apply → verify
  → runs the archive tail: change-folder close
      + vault summary  (no-op with note if canonical-store recipe absent)
      + tracker comment (no-op with note if tracker recipe absent)
  → reports completion
```

### Integration points (all soft, by convention — no `requires` in schema)
- **gentle-ai orchestrator** — consumed if present; the phase engine.
- **Engram (memory)** — consumed if present for cross-session artifact recovery.
- **worktree-flow recipe** — deferred to (not depended on) for `/build` isolation.
- **canonical-store recipe (e.g. vault)** — archive-tail vault summary channel.
- **tracker recipe (e.g. trello)** — archive-tail tracker comment channel.

## 4. `recipe.toml` — exact content shape

```toml
[recipe]
id = "plan-build-flow"
name = "Plan / Build Flow"
description = "Two-verb change workflow: /plan produces reviewable artifacts, /build implements and closes"
version = "1.0.0"
author = "ai-specs"
license = "MIT"
tags = ["workflow"]

[[capabilities]]
id = "plan-build-flow"

[[hooks]]
event = "on-sync"
action = "validate-config"

[provides]
skills = [{ id = "plan-build-flow", source = "bundled" }]
commands = [
    { id = "plan", path = "commands/plan.md" },
    { id = "build", path = "commands/build.md" },
]

[provides.brief]
workflow_rules = [
    "Use `/plan` to turn an intent into reviewable planning artifacts before writing code; review and authorize them before building.",
    "Use `/build` to implement an authorized plan, validate the result, and close out the change.",
    "Run `/build` where file-writing change work belongs; when isolated worktrees are enabled, build inside the change's worktree.",
]
useful_commands = [
    "Plan a change: `/plan <intent>`",
    "Build an authorized plan: `/build [change]`",
]

[[provides.docs]]
source = "README.md"
target = "ai-specs/recipes/plan-build-flow/README.md"
```

**Decision rationale (field by field):**

- **`id` / directory name** — `plan-build-flow`, matching the locked user decision.
  Schema requires `id` to match the directory name.
- **`tags = ["workflow"]`** — a fresh category. It must NOT collide with
  `worktree-flow` (`infra`/`worktree`) or `tdd-flow` (`quality`), because a shared
  tag with no `conflicts_with` triggers a sync *warning*, and this recipe is meant
  to coexist with all of them. `workflow` is unused by any current catalog recipe
  (verified against `docs/recipes-catalog.md`), so no conflict fires.
- **No `conflicts_with`** — the recipe coexists with classic ceremony and every
  other recipe. Nothing to exclude.
- **`[[capabilities]] id = "plan-build-flow"`** — one self-named capability so the
  recipe is addressable in the capability model, mirroring how `tdd-flow` exposes
  `test-runner`. It does not *bind* any consumed capability (schema has no consume
  primitive); consumption stays a documented convention.
- **`on-sync validate-config`** — matches every sibling recipe. It validates
  declared config fields. This recipe declares NO `[config.*]` fields (there is no
  project-specific knob — slug and store are derived at runtime), so the hook is a
  no-op guard that keeps the recipe consistent with catalog conventions. The card's
  "verify spec/design/tasks dirs" is intentionally NOT implemented: no filesystem-
  dir validation action exists and adding one is out of scope (Non-Goals).
- **`[provides.brief]` — sections chosen: `workflow_rules` + `useful_commands` only.**
  - `intro`/`purpose` are forbidden for recipes (project-only; schema raises a
    validation error). Confirmed in `recipe_schema.py:PROJECT_ONLY_SECTIONS`.
  - **No SDD/OpenSpec vocabulary** appears in any fragment: the words "spec",
    "design", "tasks", "SDD", "OpenSpec", "spec-driven", "proposal", "archive",
    "explore", "verify" are absent from brief text. The brief speaks only "plan"
    and "build". (The SKILL.md *may* name internal phases because it is not brief
    text and is the wrapper's private mapping — see Section 5 note.)
  - `workflow_rules` communicates the two-verb discipline and the worktree
    deference as generic prose (the third rule cross-references worktree isolation
    without hard-depending on the recipe).
  - `useful_commands` advertises `/plan` and `/build` so they surface in the brief
    the same way `tdd-flow` advertises its test command.
  - `runtime_flow`, `context_sources`, `conflict_policy`, `mcp_descriptions` are
    NOT contributed: this recipe installs no MCP, defines no new context source,
    and does not alter conflict precedence — contributing there would be noise.
- **No `[[provides.templates]]`** — nothing needs a runtime script or a
  create-once file (contrast worktree-flow's cleanup script). All behavior is
  prompt/skill text.
- **No `[[provides.hooks]]` (runtime hooks)** — the recipe blocks nothing and
  observes no tool events; `/build` isolation is delegated to worktree-flow's own
  gate hook when that recipe is enabled.
- **No `[config.*]`** — the two runtime variables (change-slug, artifact store)
  are *derived per invocation*, not per-project constants. Encoding them as config
  would wrongly freeze them at sync time. This is the key departure from tdd-flow
  (which has a genuine per-project `test_command`).

## 5. `SKILL.md` — structure

Front-matter (YAML) mirrors the worktree-flow skill shape:

```yaml
---
name: plan-build-flow
description: >
  Two-verb change workflow. Map `/plan` to producing reviewable planning
  artifacts and `/build` to implementing, validating, and closing an authorized
  plan. Degrade gracefully when no external orchestrator or memory backend is
  present.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Planning a change with /plan before implementation"
    - "Building an authorized plan with /build"
    - "Deciding how to run plan/build when no orchestrator or memory is available"
---
```

Body sections (in order):

1. **What plan and build mean** — the two-verb contract in one table:
   `/plan` = produce reviewable artifacts, stop for authorization;
   `/build` = implement authorized plan, validate, close.

2. **Phase mapping (the wrapper's private detail).**
   - `/plan` → explore → proposal → spec → design → tasks.
   - `/build` → apply → verify → archive-tail.
   - Archive runs as the automatic closing step at the *tail* of `/build`
     (change-folder close + optional vault summary + optional tracker comment),
     NOT as a visible third verb. This matches the card's three-stage sketch
     without leaking a third command.
   - *Vocabulary note:* these phase names live only inside the SKILL body (the
     wrapper's internal map for an agent). They MUST NOT appear in `[provides.brief]`
     fragments or the README, which stay plan/build-only. This keeps the generated
     `AGENTS.md` free of ceremony vocabulary per the 2026-05-18 boundary.

3. **Orchestrator degradation policy.**
   - gentle-ai present → let it drive the phases as it normally would.
   - gentle-ai absent → the single agent runs the phases inline as ONE
     conversation, in order, without pausing for sub-agent handoffs. Never fail,
     never silently skip a phase. Produce the same artifacts inline.

4. **Memory degradation policy.**
   - Engram present → may persist artifacts for cross-session recovery.
   - Engram absent → fall back to OpenSpec file artifacts on disk. If the user
     explicitly wants no files, use `none` (inline-only) but say so.

5. **Artifact-store default policy.**
   - If an orchestrator preflight already resolved a store, honor it.
   - Otherwise default to **OpenSpec files** (locked user decision #3). Files are
     the reviewable deliverable the workflow centers on.

6. **Change-slug derivation.**
   - Derive a short kebab-case slug from the user's intent on `/plan`
     (e.g. "add rate limiting to the login endpoint" → `rate-limit-login`).
   - Persist/record the slug so `/build [change]` resolves the same slug and the
     same artifact store `/plan` used. If `/build` is called with no argument and
     exactly one plan is outstanding, resolve to it; if ambiguous, ask.

7. **Worktree deference.**
   - `/plan` needs no worktree (it can write planning artifacts, but by policy the
     planning surface is review-first; where worktree-flow is enabled its gate
     governs writes).
   - `/build` writes production code and MUST run inside a worktree when
     `worktree-flow` is enabled; defer to that recipe's conventions rather than
     re-implementing isolation. Expressed as deference, not a hard dependency
     (schema has no `requires`).

8. **Archive-tail graceful no-op (locked decision #2).**
   - The change-folder close always completes.
   - The vault summary step no-ops with an informative note when no
     canonical-store recipe (e.g. `vault-canonical-store`) is enabled.
   - The tracker comment step no-ops with an informative note when no tracker
     recipe (e.g. `trello-mcp-workflow`) is enabled.
   - The rest of the closing still completes; `/build` never fails solely because
     an optional output channel is absent.

## 6. Command prompt files — content outline

Both prompts are thin: they set the entry context and defer the real logic to the
SKILL, exactly as `tdd-flow`'s `/tdd` defers to the `tdd-flow` skill.

### `commands/plan.md` (`/plan`)
- **Title:** `# /plan — Turn an intent into a reviewable plan`
- **Purpose line:** operationalize the `plan-build-flow` skill for planning.
- **Steps:**
  1. Load the `plan-build-flow` skill; follow its phase mapping and policies.
  2. Capture the user's intent; derive and confirm the change-slug.
  3. Resolve the artifact store (preflight → else OpenSpec files).
  4. Run the planning phases (explore → proposal → spec → design → tasks) either
     via gentle-ai or inline as one conversation if absent.
  5. Produce reviewable artifacts and STOP: present them and ask the human to
     review/authorize before any building. Do not implement in `/plan`.
- **Note:** never emit SDD/OpenSpec vocabulary to the *user*; speak "plan".

### `commands/build.md` (`/build`)
- **Title:** `# /build — Implement, validate, and close an authorized plan`
- **Purpose line:** operationalize the `plan-build-flow` skill for building.
- **Steps:**
  1. Load the `plan-build-flow` skill.
  2. Resolve the target change-slug and the artifact store used by `/plan`
     (argument, else the single outstanding plan, else ask).
  3. Confirm the plan was authorized; if not, stop and point back to `/plan`.
  4. If `worktree-flow` is enabled, ensure work runs in the change's worktree.
  5. Run implementation and validation (apply → verify).
  6. Run the archive tail: close the change folder; write the vault summary and
     tracker comment when those recipes are enabled, otherwise no-op each with an
     informative note; complete the rest of the closing regardless.
  7. Report what was built, validated, and closed.

## 7. Versioning

- **Start at `1.0.0`.** New recipe, stable initial contract, matching `tdd-flow`'s
  debut version. The manifest pin in any consumer must equal this exact string or
  `ai-specs sync` fails (version-pinning rule in `docs/recipe-schema.md`).
- The catalog entry and the recipe test read the version dynamically so a future
  bump touches one place (`recipe.toml`); tests must not hardcode `1.0.0`.

## 8. Test Strategy

**Scope: materialization only.** This repo cannot test external orchestrator
behavior (the correctness boundary in Section 1). Tests assert that the recipe
validates and that `ai-specs sync` places the right files with the right content.

New file: `tests/test_plan_build_flow_recipe.py`, following the
`tests/test_tdd_flow_recipe.py` conventions (dynamic version read via regex,
`load_module` for schema + materializer, tmp project fixture, `materialize_recipes`
returns `0`). Planned test cases:

1. **`test_recipe_validates_and_declares_capability`** — load `recipe.toml` via the
   schema; assert `id == "plan-build-flow"`, capability `plan-build-flow` present,
   bundled skill `plan-build-flow` declared, commands `plan` and `build` declared.
2. **`test_materialize_produces_skill_commands_and_doc`** — materialize into a tmp
   project and assert the four targets exist:
   - `ai-specs/.recipe/plan-build-flow/skills/plan-build-flow/SKILL.md`
   - `ai-specs/commands/plan.md`
   - `ai-specs/commands/build.md`
   - `ai-specs/recipes/plan-build-flow/README.md`
3. **`test_brief_has_no_ceremony_vocabulary`** — read the recipe's
   `[provides.brief]` fragments (via schema or raw TOML) and assert none contain
   `SDD`, `OpenSpec`, `spec-driven`, `spec`, `design`, `tasks`, `proposal`,
   `explore`, `verify`, or `archive` (case-insensitive). This guards the primary
   risk: vocabulary leakage into the generated brief. Also assert the same for the
   materialized README content.
4. **`test_no_config_and_no_runtime_hooks`** — assert the parsed recipe declares no
   `[config.*]` fields and no runtime hooks, locking the "no per-project knob"
   decision so a later edit that adds config is caught.

**Version handling:** read the version dynamically (regex on `recipe.toml`), never
hardcode, matching `_recipe_version()` in the tdd-flow test.

## 9. ADR-style Decisions

| # | Decision | Rationale | Rejected alternative |
|---|---|---|---|
| D1 | Skill/instructions-only recipe (no schema/materializer/binary changes) | Only option honoring both "catalog not core" and the 2026-05-18 boundary | Config + new on-sync action (couples to schema); shell out to gentle-ai (hard binary dependency) — both deferred |
| D2 | Name `plan-build-flow`, commands `/plan` `/build` | Vocabulary hygiene beats card fidelity; hiding ceremony is the whole point | Card's `sdd-plan-mode` / `/sdd-plan` — leaks the terms the recipe exists to hide |
| D3 | Archive runs as the tail of `/build`, not a third verb | Preserves the two-verb UX while matching the card's three-stage sketch | Visible `/archive` or `/close` command — leaks a third surface |
| D4 | `on-sync validate-config` only; no dir-validation action | No filesystem-dir validation action exists; adding one is out of scope | Faking the card's "verify dirs" step — silent dishonesty |
| D5 | No `[config.*]` fields | Slug and store are per-invocation runtime values, not per-project constants | A `default_store`/`slug_prefix` config — freezes runtime decisions at sync time |
| D6 | Brief contributes only `workflow_rules` + `useful_commands`, ceremony-free | Other sections would be noise; `intro`/`purpose` are forbidden for recipes | Contributing `runtime_flow`/`context_sources` — misrepresents the recipe |
| D7 | Default artifact store = OpenSpec files when preflight didn't resolve one | Files are the reviewable deliverable the card centers on (locked #3) | `hybrid` default — higher token cost, not needed for review |
| D8 | Archive vault/tracker channels no-op with a note when their recipes are absent | Keeps `/build` resilient; the close still completes (locked #2) | Requiring those recipes — makes `/build` brittle for minimal setups |
| D9 | Worktree isolation via `workflow_rules` deference, not a dependency | Recipe schema has no `requires`; soft cross-reference matches worktree-flow's own brief style | Hard dependency declaration — unsupported by the schema |
| D10 | `tags = ["workflow"]`, fresh category | Avoids a shared-tag sync warning with existing recipes; must coexist with all | Reusing `infra`/`quality` — would warn against coexisting recipes |

## 10. Risks & Assumptions Requiring Validation

- **Vocabulary leakage (Med).** Mitigated by test case #3 asserting brief + README
  are ceremony-free; still requires a human eyeball on the generated `AGENTS.md`
  during apply/verify.
- **Empty-promise degradation (Med).** If gentle-ai is absent, quality depends on
  the SKILL's inline-run instructions being clear enough for a single agent. This
  is documentation-quality risk, not testable here.
- **Slug resolution ambiguity (Low).** `/build` with no argument and multiple
  outstanding plans must ask rather than guess; SKILL step 6 must state this
  explicitly and the command prompt must not shortcut it.
- **Assumption:** `tags = ["workflow"]` is unused elsewhere in the catalog
  (verified today against `docs/recipes-catalog.md`); re-verify at apply time in
  case a sibling change introduced the tag.
- **Assumption:** the archive-tail no-op behavior is expressible purely in SKILL
  prose because the archive channels are themselves external recipes/MCPs, not
  `ai-specs-cli` code paths.
