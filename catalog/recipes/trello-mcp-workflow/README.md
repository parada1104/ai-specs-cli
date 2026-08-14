# Trello MCP Workflow

Automated Trello board integration for ai-specs projects.

## Installation

```bash
ai-specs recipe add trello-mcp-workflow
ai-specs recipe init trello-mcp-workflow
```

This installs:

- **Skill**: `trello-mcp-workflow` — runtime skill with 4 capabilities (session-bootstrap, card-linking, state-sync, progress-comment).
- **Templates**: `card-feature.md`, `card-bug.md`, `card-spike.md`, `card-epic.md`, `card-handoff.md` — Trello card description templates.
- **Command**: `trello-workflow` — quick-reference command file for agents.

`ai-specs recipe add` only declares the recipe in `ai-specs/ai-specs.toml` and writes placeholder config.
`ai-specs sync` materializes the bundled assets.
`ai-specs recipe init trello-mcp-workflow` prints a read-only setup brief so the
project can confirm `board_id`, list mappings, and MCP readiness before sync.

## Template override ownership

The card templates are governed `condition = "not_exists"` overrides. Sync records
the last CLI-written bytes in `[managed.*]` in `ai-specs/.ai-specs.lock`:

| State / policy | Behavior |
|---|---|
| Managed current | No rewrite and no warning. |
| Managed stale + `auto` (default) | Refresh from the current catalog and update the lock. |
| Managed stale + `confirm` or `never-force` | Preserve and warn; refresh explicitly when ready. |
| User-modified or untracked custom | Preserve and warn; never force-overwrite. |

To intentionally replace a customized template, delete that target and sync:

```bash
rm ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md
ai-specs sync
```

Runtime hook scripts follow gate provenance instead of template policy. Sync
records a baseline of the exact bytes the CLI last rendered for each generated
hook; a byte mismatch or missing baseline is preserved with a warning, and an
explicit refresh (`ai-specs sync --refresh-gates`, or `rm <hook> && ai-specs sync`)
replaces a customized hook only after its exact pre-refresh bytes are saved to a
cache-only immutable backup. The ownership policy for templates is separate.

## Configuration

Add configuration under `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`:

| Field | Required | Default | Description |
|---|---|---|---|
| `board_id` | Yes | — | Trello board ID for the project. |
| `default_list` | No | `In Progress` | List name where new cards are created. |
| `epic_list` | No | `Epic` | List name where epic-type cards are placed. |
| `gate_mode` | No | `warn` | Tracker card gate: `off` / `warn` / `always`. |

### Example

```toml
[recipes.trello-mcp-workflow]
enabled = true
version = "1.3.0"

[recipes.trello-mcp-workflow.config]
board_id = "69ec097f13e2d38ecd89a557"
```


## Card-per-change contract

Every active OpenSpec change must carry a `## Tracker` section in `proposal.md`
(fallback `tasks.md`) with a non-empty `card_id` (and preferably `url`):

```markdown
## Tracker

- **card_id**: `<24-hex>`
- **url**: https://trello.com/c/...
```

Doctor and the `tracker-card-gate` hook share this validity predicate. The only
documented exemption is `openspec/changes/<slug>/tracker.none` (conceptual name
`tracker:none`) with a one-line reason — logged and rare. Archives are
grandfathered.

The global contract is also declared in `openspec/config.yaml` under `tracking:`
(soft guidance for SDD agents). Operational `gate_mode` / `board_id` still come
from recipe config in `ai-specs.toml`.

## Gate modes

| Mode | Behavior |
|------|----------|
| `off` | Gate inactive |
| `warn` | stderr warning, never blocks (dogfood default) |
| `always` | block production writes + high-confidence `gh pr create` / archive shell |

Configured via `[recipes.trello-mcp-workflow.config] gate_mode`. One-shot env
override: `TRACKER_CARD_GATE_MODE`. Production dirs override:
`TRACKER_CARD_GATE_PATHS` (default `lib catalog bin src`). The gate **never**
blocks `openspec/**` and **fails open** on parse/lookup errors. It does **not**
call Trello MCP — presence of the `## Tracker` section is the proof.

Dual hooks share one script: `tracker-card-gate` (Edit/Write/…) and
`tracker-card-gate-shell` (Bash/Shell/…).

## Residual platform gaps

- **Cursor**: no pre-file-write hook — file-write matcher is skipped; shell id
  registers as `beforeShellExecution`.
- **OpenCode**: primary-agent pre-tool-use only — not subagent/MCP tool calls.
- **pi / omp**: this-process only — child processes are not covered.
- **MCP interception**: explicitly **not** implemented. Do not claim the gate
  prevents Trello MCP misuse; brief + skill anti-bypass cover that surface.

## Live evals (manual / nightly)

```bash
EVALS_LIVE=1 ./tests/evals/run-live-trello.sh
```

Not wired into `./tests/validate.sh`. See `tests/evals/scenarios/trello-mcp-workflow/`.

## Ceremony vocabulary note

Ceremony/depth classification lives in `plan-build-flow` (depth tiers `Light` /
`Standard` / `Full`). The legacy `trivial` / `local_fix` / `behavior_change` /
`domain_change` vocabulary is retired; see the `plan-build-flow` spec for the
retirement and migration mapping.
