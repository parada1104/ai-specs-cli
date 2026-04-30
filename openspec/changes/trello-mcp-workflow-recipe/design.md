## Context

The Trello project-management flow in ai-specs projects is currently a manual convention encoded in the `trello-pm-workflow` skill. Agents must discover board IDs, card states, and linking rules from scratch on each session. The `session-bootstrap` skill queries Trello ad-hoc during the 3-level consensus check. Nothing moves cards automatically when OpenSpec changes advance through SDD phases.

The recipe system (V2 schema) now supports capabilities, hooks, config schemas, and init workflows — all the primitives needed to package the Trello workflow as an installable, versioned recipe.

Current state:
- `recipe_schema.py` already defines `Capability`, `Hook`, `ConfigSchema`, `InitWorkflow`, `SddConfig`.
- `execute_hooks()` in `recipe-materialize.py` currently handles only `validate-config`.
- The recipe catalog has 7 test/fixture recipes; no production recipes yet.
- `trello-pm-workflow` skill is informational only — no automation.

## Goals / Non-Goals

**Goals:**

1. Provide a recipe `catalog/recipes/trello-mcp-workflow/` that bundles skills, templates, docs, and commands for Trello automation.
2. Extend `execute_hooks()` to recognize four new hook actions (`bootstrap-board`, `link-trello-card`, `sync-card-state`, `comment-verification`) at sync time.
3. Validate recipe config (`board_id` required, `default_list`/`epic_list` optional with defaults) during sync.
4. Define four capabilities that the bundled skill implements at agent runtime via Trello MCP.
5. Maintain coexistence with the legacy `trello-pm-workflow` skill.
6. Provide reusable Trello card templates derived from the existing skill.

**Non-Goals:**

1. Replacing the legacy `trello-pm-workflow` skill immediately.
2. Executing actual Trello MCP calls during `ai-specs sync` (sync is agent-less).
3. Real-time bidirectional sync (Trello → OpenSpec); sync is agent-driven only.
4. Webhooks or server-side triggers.
5. Integration with non-Trello trackers (Jira, GitHub Issues, Linear).

## Decisions

### 1. Recipe packaging vs. skill-only approach
**Decision**: Package as a recipe (with `recipe.toml`, hooks, capabilities, templates) rather than a standalone skill.

**Rationale**: The recipe system provides config validation, hook orchestration, and capability binding — all of which a bare skill cannot. A recipe can declare capabilities that the agent runtime resolves, and hooks that sync-time execute. A skill-only approach would require manual setup that the recipe automates.

### 2. Sync-time vs. runtime hook separation
**Decision**: `bootstrap-board` runs at sync time (validates config, writes marker). `link-trello-card`, `sync-card-state`, and `comment-verification` are deferred to agent runtime (no-op at sync time, just print a notice).

**Rationale**: Sync is agent-less — no MCP connection available. Only config validation and marker-file writing can happen at sync time. Runtime capabilities execute when the agent has MCP access.

### 3. Marker file as sync-to-agent contract
**Decision**: `bootstrap-board` writes `.recipe/trello-mcp-workflow/bootstrap-ready` with board metadata extracted from config.

**Rationale**: The marker file is the bridge between sync-time (validates config exists) and runtime (agent reads marker to know the recipe is active and configured). This follows the existing pattern where `.recipe/` stores materialized state.

### 4. Minimal config schema
**Decision**: Three config fields: `board_id` (required), `default_list` (optional, default `"In Progress"`), `epic_list` (optional, default `"Epic"`).

**Rationale**: These are the minimum required to bootstrap Trello integration. Phase-to-list mappings and label mappings live in the bundled skill's SKILL.md — they are conventions, not configuration, keeping the recipe config minimal.

### 5. Bundled skill naming
**Decision**: The bundled skill uses name `trello-mcp-workflow` (matching the recipe ID).

**Rationale**: Recipe convention is that a bundled skill shares the recipe's ID. The legacy `trello-pm-workflow` skill retains its name; both coexist without conflict since they have different IDs and purposes.

### 6. Template materialization strategy
**Decision**: Templates use `condition = "not_exists"` and target `.recipe/trello-mcp-workflow/templates/` inside the project's ai-specs directory.

**Rationale**: Templates are reference material for the agent to create cards from. They should not overwrite user-customized templates. The `not_exists` condition ensures idempotent re-sync.

### 7. Error handling philosophy
**Decision**: All runtime Trello failures emit warnings to stderr and optionally log to `.recipe/trello-mcp-workflow/warnings.log`, never blocking agent progress.

**Rationale**: Trello is an operational convenience, not a gate. A network failure or API limit should not halt an SDD cycle. The agent continues its work regardless.

## Architecture

### Recipe directory structure
```
catalog/recipes/trello-mcp-workflow/
├── recipe.toml           # V2 manifest
├── skills/
│   └── trello-mcp-workflow/
│       └── SKILL.md       # Runtime logic
├── templates/
│   ├── card-feature.md
│   ├── card-bug.md
│   ├── card-spike.md
│   ├── card-epic.md
│   └── card-handoff.md
├── commands/
│   └── trello-workflow.md
└── docs/
    └── README.md
```

### recipe.toml structure (V2)
```toml
[recipe]
id = "trello-mcp-workflow"
name = "Trello MCP Workflow"
description = "Automated Trello board integration for ai-specs SDD workflows"
version = "1.0.0"

[[capabilities]]
id = "trello-session-bootstrap"

[[capabilities]]
id = "trello-card-linking"

[[capabilities]]
id = "trello-state-sync"

[[capabilities]]
id = "trello-progress-comment"

[[hooks]]
event = "on-sync"
action = "bootstrap-board"

[[hooks]]
event = "on-sync"
action = "link-trello-card"

[[hooks]]
event = "on-sync"
action = "sync-card-state"

[[hooks]]
event = "on-sync"
action = "comment-verification"

[config.board_id]
required = true
type = "string"

[config.default_list]
required = false
type = "string"
default = "In Progress"

[config.epic_list]
required = false
type = "string"
default = "Epic"

[provides]
skills = [{ id = "trello-mcp-workflow", source = "bundled" }]
commands = [{ id = "trello-workflow", path = "commands/trello-workflow.md" }]

[[provides.templates]]
source = "templates/card-feature.md"
target = ".recipe/trello-mcp-workflow/templates/card-feature.md"
condition = "not_exists"

[[provides.templates]]
source = "templates/card-bug.md"
target = ".recipe/trello-mcp-workflow/templates/card-bug.md"
condition = "not_exists"

[[provides.templates]]
source = "templates/card-spike.md"
target = ".recipe/trello-mcp-workflow/templates/card-spike.md"
condition = "not_exists"

[[provides.templates]]
source = "templates/card-epic.md"
target = ".recipe/trello-mcp-workflow/templates/card-epic.md"
condition = "not_exists"

[[provides.templates]]
source = "templates/card-handoff.md"
target = ".recipe/trello-mcp-workflow/templates/card-handoff.md"
condition = "not_exists"

[[provides.docs]]
source = "docs/README.md"
target = "docs/trello-mcp-workflow.md"
condition = "not_exists"
```

### Hook execution in recipe-materialize.py
```python
def execute_hooks(recipe: Any, merged_config: dict[str, Any]) -> None:
    for hook in recipe.hooks:
        if hook.action == "validate-config":
            # existing behavior: check required config fields
            ...
        elif hook.action == "bootstrap-board":
            # NEW: validate board_id, write bootstrap-ready marker
            marker_dir = project_root / ".recipe" / recipe.id
            marker_dir.mkdir(parents=True, exist_ok=True)
            (marker_dir / "bootstrap-ready").write_text(
                f"board_id={merged_config.get('board_id', '')}\n"
                f"default_list={merged_config.get('default_list', 'In Progress')}\n"
                f"epic_list={merged_config.get('epic_list', 'Epic')}\n"
            )
        elif hook.action == "link-trello-card":
            # DEFERRED: no-op at sync time, agent handles at runtime
            info(f"recipe '{recipe.name}': hook 'link-trello-card' deferred to agent runtime")
        elif hook.action == "sync-card-state":
            # DEFERRED: no-op at sync time, agent handles at runtime
            info(f"recipe '{recipe.name}': hook 'sync-card-state' deferred to agent runtime")
        elif hook.action == "comment-verification":
            # DEFERRED: no-op at sync time, agent handles at runtime
            info(f"recipe '{recipe.name}': hook 'comment-verification' deferred to agent runtime")
        else:
            warn(f"recipe '{recipe.name}': unknown hook action '{hook.action}' (skipped)")
```

### Sequence diagrams

#### Session bootstrap (`trello-session-bootstrap`)
```
Agent session starts
        |
        v
+----------------------------+
| Read bootstrap-ready marker|
+----------------------------+
        |
        v
+----------------------------+
| Query board via Trello MCP |
| (board_id from config)     |
+----------------------------+
        |
        v
+----------------------------+
| Detect active card         |
| (by list + labels)         |
+----------------------------+
        |
        v
Feed primitive to
3-level consensus check
```

#### Card linking (`trello-card-linking`)
```
openspec new change "my-change"
        |
        v
+----------------------------+
| Detect linked Trello card  |
| (from session context)     |
+----------------------------+
        |
   +----+----+
   |         |
  yes        no
   |         |
   v         v
Post comment  Offer to create
on card       card from template
   |              |
   |             yes -> Create card -> Post comment
   |              |
   |             no  -> Skip linking
   v
Continue change creation
```

#### State sync (`trello-state-sync`)
```
SDD phase transition
(e.g. design -> tasks)
        |
        v
+----------------------------+
| Resolve linked card        |
+----------------------------+
        |
        v
+----------------------------+
| Map phase -> Trello list   |
| (skill-defined mapping)    |
+----------------------------+
        |
        v
Move card to target list
        |
        v
+----------------------------+
| Map phase -> label         |
| (replace single phase      |
|  label)                    |
+----------------------------+
        |
        v
Post phase-transition comment
        |
        v
Continue agent workflow
```

#### Progress comment (`trello-progress-comment`)
```
Verify report generated
        |
        v
+----------------------------+
| Collect changed files       |
| (from apply-progress.md)   |
+----------------------------+
        |
        v
+----------------------------+
| Read verify report verdict |
+----------------------------+
        |
        v
+----------------------------+
| Post structured comment    |
| on linked Trello card      |
+----------------------------+
        |
        v
+----------------------------+
| Include archive link if    |
| change is archived         |
+----------------------------+
```

## Risks / Trade-offs

1. **[Risk] Sync-time hook execution adds permanent branches to `execute_hooks()`** → Mitigation: new actions are isolated `elif` branches; unknown actions still fall through to `warn()`. The function remains < 50 lines.

2. **[Risk] Runtime capabilities depend on Trello MCP availability** → Mitigation: graceful degradation — all Trello failures emit warnings, never block agent work.

3. **[Risk] Template drift between recipe and legacy skill** → Mitigation: templates are derived from the existing `trello-pm-workflow` skill. When the legacy skill is eventually retired, the recipe templates become canonical.

4. **[Risk] Config schema might need extension in the future** → Mitigation: Only 3 fields now; additional fields can be added with defaults so existing configs remain valid.

5. **[Trade-off] Deferred hooks print a notice at sync time rather than being silent** → Rationale: the notice confirms to the operator that the recipe recognized the hook and intentionally deferred it, rather than silently skipping.

## Rollback considerations

- Removing the recipe directory and its `.recipe/` output fully reverts the change.
- Reverting `execute_hooks()` changes removes 4 `elif` branches; the `else: warn()` fallback remains.
- The legacy `trello-pm-workflow` skill continues to work independently throughout.

## Testing strategy

- **Unit tests**: `tests/test_recipe_materialize_hooks.py` — test each new hook action in isolation.
- **Integration tests**: `tests/test_trello_recipe_materialize.py` — test full recipe sync with config validation and marker file creation.
- **Spec compliance**: each scenario in the 4 runtime capability specs maps to a testable behavior in the bundled skill.