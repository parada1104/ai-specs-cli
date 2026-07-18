# Exploration: agent literacy for the full `ai-specs` CLI

> Persisted also as Engram `#1356` (`sdd/agent-cli-literacy/explore`).
> **Supersedes product direction** of `#1352` (`sdd/agent-cli-facility/explore` — path/shim facility).
> Clarified intent: `#1354`. Helpers/CLI from `$AI_SPECS_HOME`: `#1351`.

## Current State

- CLI surface (`bin/ai-specs`): `init`, `sync`, `sync-agent`, `refresh-bundled`, `skills add|list|remove` / `add-dep`, `doctor`, `rules-audit`, `recipe list|add|init`, `configure-recipes`, `upgrade`, `version`, `hub`. `help` is human-oriented, no order-of-ops.
- Agent-facing literacy today: only `bundled-skills/skill-creator` and `bundled-skills/skill-sync`. Gaps for lifecycle, recipes, deps, doctor, upgrade, hub.
- Bundled skills are always-on via `refresh-bundled.py` → consumer `ai-specs/skills/<name>/`.
- Auto-invoke works on Claude/Cursor/Gemini; OpenCode/pi/omp need an AGENTS.md pointer.
- Briefs are manifest + recipe fragment driven; **no bundled→brief path** today.

## Approaches (summary)

| # | Approach | Verdict |
|---|---|---|
| 1 | Always-on bundled literacy skill(s) | Strong delivery vehicle |
| 2 | Opt-in catalog recipe | Reject as primary (chicken/egg) |
| 3 | Brief-only bullets | Too thin alone |
| 4 | Slash commands | Optional complement later |
| 5 | MCP wrapping CLI | Reject for now |
| 6 | Hybrid: bundled skills + brief pointer | **Recommended** |
| 7 | Focused skill split vs mega-skill | Prefer focused |

## Recommendation (locked for this change)

Hybrid (#6) with focused split (#7):

1. Three always-on bundled skills: `harness-lifecycle`, `harness-recipes`, `harness-skills-deps`.
2. One always-on thin brief pointer via a **fixed render bullet** in `agents-render.py` (covers non-auto-invoke runtimes without depending on `session-context`).
3. Path resolution as a one-line footnote only.
4. No per-project `ai-specs/bin` shim.

## Ready for Proposal

Yes — clarifications closed by adopting explore recommendations as defaults for this plan.
