# Proposal: agent-cli-literacy

## Why

Agents in consumer projects can create skills in the narrow sense (`skill-creator`)
and validate metadata (`skill-sync`), but they do **not** know how to operate the
rest of the harness CLI: `init`, `sync`, recipe add/init/configure, external deps,
doctor, refresh-bundled, upgrade, hub.

Humans have `README.md` and `ai-specs help`. Agents do not reliably load those, and
there is no always-on, fan-out literacy surface that teaches *when / in what order /
with which pitfalls* to run CLI ops.

Prior framing that treated this as a binary path/shim problem (`#1352`) is the wrong
product direction (`#1354`). The facility we need is **CLI literacy**.

## What Changes

1. **Three always-on bundled literacy skills** under `bundled-skills/`:
   - `harness-lifecycle` — init, configure-recipes, sync, sync-agent, refresh-bundled
     (incl. `.new` sidecars), doctor, upgrade, hub; order-of-ops + troubleshooting.
   - `harness-recipes` — recipe list/add/init + configure-recipes; bindings/capabilities;
     when to enable a recipe.
   - `harness-skills-deps` — local skills (cross-link `skill-creator`), `skills add|list|remove`
     / `add-dep`, cross-link `skill-sync`.
2. **Always-on brief pointer** — `agents-render.py` emits one fixed Context Sources /
   Useful Commands bullet pointing agents at those skills (covers OpenCode/pi/omp where
   auto-invoke does not fire).
3. **Tests + light docs** — assert refresh-bundled ships the new dirs; assert brief
   pointer present; optional validate that skill playbooks only reference real CLI
   subcommands.
4. **Footnote only** — path resolution
   (`command -v ai-specs || "${AI_SPECS_HOME:-$HOME/.ai-specs}/bin/ai-specs"`) lives
   inside the skills, not as a project bin shim (`#1351`).

## Locked decisions

| Decision | Choice | Rationale |
|---|---|---|
| Delivery | Always-on **bundled** skills | Every project must operate the harness; recipe opt-in has chicken/egg |
| Skill shape | **3 focused** skills | Precise `auto_invoke`; reuse skill-creator/skill-sync |
| Brief pointer | **Fixed render bullet** | Always-on; does not require `session-context` enabled |
| MCP / slash commands | Out of scope | Optional follow-ups |
| Project `ai-specs/bin` shim | Forbidden | `#1351` |

## Non-goals

- Wrapping the CLI as an MCP.
- Duplicating the full README inside skills.
- Auto-generating playbooks from `ai-specs help`.
- Shipping literacy as an opt-in catalog recipe (primary path).
- Changing public CLI subcommand contracts/flags.
- Reintroducing per-project helper bins for literacy.

## Impact / scope surface

| Area | Touch? | Notes |
|---|---|---|
| `bundled-skills/harness-*` | Yes | New content (primary deliverable) |
| `lib/_internal/agents-render.py` | Yes | Fixed pointer bullet |
| `lib/_internal/refresh-bundled.py` | No logic change | Auto-ships new dirs |
| `tests/` | Yes | Ship + brief + content/command checks |
| `docs/` | Optional light | Pointer in troubleshooting or skills-by-agent |
| `catalog/recipes/` | No | Not the primary vehicle |
| Consumer project dirtiness | +3 skill dirs | Same pattern as skill-creator today |

## Success criteria

1. After `ai-specs sync` / refresh-bundled on a fresh or existing project, the three
   skills exist under `ai-specs/skills/harness-*`.
2. Generated `AGENTS.md` contains a stable pointer to harness CLI literacy skills.
3. Skills pass `skill-sync` / frontmatter contract (`scope: [root]`, intent-specific
   `auto_invoke`).
4. Playbooks cover the full public command surface listed in `bin/ai-specs` help
   (except trivial `version`/`help` which may be footnotes).
5. No new `ai-specs/bin/*` literacy shim.

## Tracker

- Trello: https://trello.com/c/FjH6H1Ae (card 43)
- Branch / worktree: `feat/agent-cli-literacy` / `.worktrees/agent-cli-literacy`
