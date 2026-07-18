# Design: agent-cli-literacy

## 1. Architecture approach

**Content-first always-on literacy**, delivered through the existing bundled-skills
pipeline, plus a **single always-on brief pointer** so runtimes without skill
auto-invoke still discover the playbooks.

```
┌─────────────────────────────────────────────────────────────┐
│ CLI install (~/.ai-specs)                                   │
│   bundled-skills/harness-lifecycle/                         │
│   bundled-skills/harness-recipes/                           │
│   bundled-skills/harness-skills-deps/                       │
└───────────────────────────┬─────────────────────────────────┘
                            │ refresh-bundled / init / sync
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Consumer project                                            │
│   ai-specs/skills/harness-*/SKILL.md   ← literacy depth     │
│   AGENTS.md (rendered)                 ← 1-line pointer     │
│   .claude/.cursor/.../skills → cache   ← fan-out (existing) │
└─────────────────────────────────────────────────────────────┘
```

This reuses:

- `refresh-bundled.py` (copy + lock + `.new` sidecars + user delete opt-out)
- `sync-agent.sh` fan-out to enabled runtimes
- `skill_contract.py` / `skill-sync` for frontmatter validation
- `agents-render.py` section composition (extend Useful Commands or Context Sources)

No recipe materializer, no MCP, no project bin shim.

## 2. Skill split

| Skill | Owns | Cross-links |
|---|---|---|
| `harness-lifecycle` | `init`, `configure-recipes` (as lifecycle step), `sync`, `sync-agent`, `refresh-bundled`, `doctor`, `upgrade`, `hub` | mentions recipes/skills skills for domain depth |
| `harness-recipes` | `recipe list\|add\|init`, `configure-recipes` (recipe config), capability/binding mental model | lifecycle for sync after add |
| `harness-skills-deps` | local skill authoring posture, `skills add\|list\|remove`, `add-dep` | `skill-creator`, `skill-sync` |

**Frontmatter (all three):**

```yaml
metadata:
  scope: [root]
  auto_invoke:
    # intent-specific phrases — no ultra-broad "use ai-specs"
```

Triggers must map to user intents (e.g. "add a recipe", "install an external skill",
"sync the harness", "doctor the project") so auto-invoke stays precise.

**Content rules:**

- Curate *operating decisions*: when, order, pitfalls, copy-paste examples.
- Do **not** paste README wholesale.
- Include path footnote once (preferably in `harness-lifecycle`):
  `command -v ai-specs >/dev/null && ai-specs … || "${AI_SPECS_HOME:-$HOME/.ai-specs}/bin/ai-specs" …`
- Keep `version` / bare `help` as footnotes only.

Optional `assets/cheatsheet.md` per skill or one shared cheatsheet only if it
reduces SKILL.md length without becoming a second source of truth.

## 3. Brief pointer (fixed render)

**Problem:** recipe/manifest brief fragments are not always-on for every project.
Bundled skills cannot inject AGENTS.md bullets today.

**Solution:** hard-code one additive bullet in `agents-render.py` inside
`## Useful Commands` (preferred) or `## Context Sources` if Useful Commands would
otherwise be empty and we still need the pointer visible.

Preferred copy (English, brief language):

> For ai-specs harness operations (init, sync, recipes, skills/deps, doctor), load
> the `harness-lifecycle`, `harness-recipes`, or `harness-skills-deps` skills under
> `ai-specs/skills/`.

**Render rules:**

- Always emit when rendering a normal (non–marker-preserved) brief.
- Idempotent: if the project already has an identical manifest/recipe bullet, dedupe
  if the section already dedupes by exact string; otherwise accept a single fixed
  emission (tests pin exact text).
- Respect `--preserve-if-runtime-brief`: user-managed briefs stay untouched.

**Rejected alternatives for this change:**

- `session-context` fragment — not enabled in every project.
- Manifest-only convention — not always-on.
- Baking absolute `$AI_SPECS_HOME` paths into AGENTS.md at sync time — goes stale.

## 4. Data / update flow

1. Developer ships new dirs under CLI `bundled-skills/`.
2. Consumer runs `ai-specs sync` (or `refresh-bundled`).
3. New skills copy into `ai-specs/skills/` (first install) or offer `.new` if user-edited.
4. Fan-out symlinks resolved skills into runtime skill dirs.
5. `agents-render.py` emits the pointer bullet into AGENTS.md.

Existing lock/`refresh-bundled` semantics apply unchanged.

## 5. Testing strategy

| Layer | What |
|---|---|
| Unit | New `tests/test_harness_cli_literacy.py` (or split): skills exist under `bundled-skills/`; frontmatter valid via `skill_contract`; refresh-bundled into tmp project creates `ai-specs/skills/harness-*`; agents-render includes pointer needle |
| Contract | skill-sync metadata PASS on the three skills |
| Drift guard | Assert every `ai-specs <cmd>` mentioned in the three SKILL.md files is a known subcommand from `bin/ai-specs` help (parse commands list) |
| Regression | Existing refresh-bundled / agents-render tests still pass |

RED → GREEN under project TDD skill: write failing ship/pointer tests first, then
add skills + render change.

## 6. Docs touch (light)

- Optionally one paragraph in `docs/skills-by-agent.md` noting harness literacy skills
  + auto-invoke limits (pointer covers non-auto-invoke runtimes).
- Do not rewrite README CLI table; skills own agent pedagogy.

## 7. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Trigger dilution | Intent-specific `auto_invoke`; three focused skills |
| README/skill drift | Operate-decisions only + subcommand existence test |
| Brief noise | Single fixed bullet; exact-text pin in tests |
| Surface growth (+3 skills) | Same precedent as skill-creator; acceptable |
| User deleted a harness skill | Existing permanent opt-out via delete; document in lifecycle skill |

## 8. Open questions (non-blocking)

- Exact section for the pointer (`Useful Commands` vs `Context Sources`) — default
  **Useful Commands**; switch only if empty-section UX is worse in fixtures.
- Whether to ship optional `assets/cheatsheet.md` — decide during apply if SKILL.md
  exceeds ~200 lines of operating content.

## 9. Implementation order

1. RED: ship + frontmatter + pointer tests.
2. GREEN: author three SKILL.md files.
3. GREEN: agents-render fixed bullet.
4. Validate: `./tests/validate.sh`, skill-sync.
5. Light docs if needed.
