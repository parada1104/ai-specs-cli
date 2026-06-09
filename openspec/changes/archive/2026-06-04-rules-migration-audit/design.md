# Design: rules-migration-audit (`/rules-audit`)

## Technical Approach

Option B (proposal): a **read-only Python scanner** emits a deterministic JSON
inventory; the **agent classifies** items into the 7-bucket taxonomy and writes
an advisory plan. New `lib/_internal/rules-inventory.py` mirrors `doctor.py`
(`Severity`/`Check`/`Doctor` → `Source`/`InventoryItem`/`RulesInventory`).
`lib/rules-audit.sh` wraps it (clone of `doctor.sh`). `bin/ai-specs` gets a
`rules-audit` case parallel to `doctor`. Bundled command `rules-audit.md`
distributes via the existing `refresh-bundled.py` + `sync-agent.sh` fan-out —
no pipeline change. Reuses `split_frontmatter`/`parse_frontmatter` (skill_contract),
`collect_skills` (skill-resolution), `load_recipe_toml` (recipe_schema).

## Deterministic vs Judgment Boundary

| Layer | Owner | Output |
|-------|-------|--------|
| Inventory (what exists, where, raw content, candidate signals) | Python | JSON |
| Classification (which of 7 buckets, plan prose) | Agent | `ai-specs/plans/rules-migration-<date>.md` |

Python NEVER assigns a final bucket. It emits `candidate_recipes` (keyword
matches) and booleans (`already_resolved`) as hints. The agent reads the JSON,
applies judgment (a rule may span buckets), and authors the plan. This keeps the
inventory testable/reproducible while classification stays context-aware.

## Architecture Decisions

| Decision | Choice | Alternatives rejected | Rationale |
|----------|--------|-----------------------|-----------|
| Inventory vs classify split | Python inventory + agent classify | All-agent (Opt A); Python writes plan (Opt C) | Deterministic shape is testable; LLM keeps nuance; no write from Python |
| Class pattern | Clone `doctor.py` `Severity/Check/Doctor` | argparse/new abstraction | One proven read-only pattern; reviewers already know it |
| `.mdc` parsing | Reuse `split_frontmatter`+`parse_frontmatter` | New YAML parser; PyYAML dep | `.mdc` frontmatter is the same YAML subset; zero new deps |
| Recipe keywords | Static `RECIPE_KEYWORDS` map in scanner | Parse recipe.toml for triggers | Triggers aren't declared in recipe.toml (explore #4); keep map explicit + testable |
| Read-only enforcement | No write API imported; test asserts zero fs mutation | Trust-by-convention | Hard guarantee; matches D5 |
| Output JSON to stdout | `print(json.dumps(...))`, no `--report` text | Human table like doctor | Agent is the consumer; one machine format |
| Mode A vs B detection | Absence of `ai-specs/ai-specs.toml` AND presence of legacy files → A; neither → B | Flag-driven | Auto-detect matches the migration use case |

## Sequence Diagram — rules-audit flow

```
Dev          /rules-audit (agent)       ai-specs rules-audit         rules-inventory.py            filesystem
 │  invoke         │                            │                           │                          │
 ├────────────────>│                            │                           │                          │
 │                 │  Bash: ai-specs rules-audit│                           │                          │
 │                 ├───────────────────────────>│ exec python3 …            │                          │
 │                 │                            ├──────────────────────────>│ scan (READ ONLY)         │
 │                 │                            │                           ├─.cursor/rules/**/*.mdc──> │
 │                 │                            │                           ├─.cursorrules ───────────> │
 │                 │                            │                           ├─AGENTS.md sections ─────> │
 │                 │                            │                           ├─ai-specs.toml manifest──> │
 │                 │                            │                           ├─collect_skills() ───────> │
 │                 │                            │                           ├─recipe catalog ─────────> │
 │                 │                            │                           ├─.atl/skill-registry.md──> │
 │                 │                            │<──JSON inventory (stdout)─┤ (no writes)              │
 │                 │<──JSON inventory───────────┤                           │                          │
 │                 │ classify into 7 buckets (judgment)                     │                          │
 │                 │ write ai-specs/plans/rules-migration-<date>.md ───────────────────────────────────>│
 │<──plan summary──┤                            │                           │                          │
```

## Data Schema — inventory JSON (the owed decision)

```jsonc
{
  "schema_version": 1,
  "mode": "A",                     // "A" legacy migration | "B" greenfield
  "target": "/abs/project/root",
  "stack_hints": ["python", "node"],   // Mode B: detected from lockfiles
  "sources": {
    "cursor_rules": [               // .cursor/rules/**/*.mdc
      { "path": ".cursor/rules/api.mdc", "description": "...",
        "globs": ["apps/api/**"], "always_apply": false,
        "body_excerpt": "first ~400 chars",
        "candidate_recipes": ["worktree-flow"],   // keyword hints, NOT final
        "already_resolved": false }
    ],
    "cursorrules": [                // .cursorrules (root, monolithic)
      { "path": ".cursorrules", "body_excerpt": "...",
        "candidate_recipes": ["tdd-flow"] }
    ],
    "agents_md_sections": [         // ## headings parsed from AGENTS.md
      { "heading": "Workflow Rules", "body_excerpt": "...",
        "candidate_recipes": ["git-pr-flow"] }
    ],
    "manifest": { "present": true, "enabled_agents": ["claude"],
                  "recipes": ["worktree-flow"], "has_runtime_brief": true },
    "resolved_skills": [            // from collect_skills()
      { "id": "worktree-flow", "source": "recipe" } ],
    "recipe_catalog": ["worktree-flow","git-pr-flow","session-context",
                       "tdd-flow","trello-mcp-workflow","vault-canonical-store"],
    "atl_registry": { "present": false, "skill_ids": [] }
  },
  "summary": { "cursor_rules": 3, "cursorrules": 1, "agents_md_sections": 6 }
}
```

`candidate_recipes` come from `RECIPE_KEYWORDS` (case-insensitive substring match
against description + body): `worktree-flow` ← worktree; `git-pr-flow` ← git/PR/
pull request; `tdd-flow` ← tdd/test; `trello-mcp-workflow` ← trello/board/card;
`vault-canonical-store` ← vault/obsidian/canonical; `session-context` ← session/
bootstrap. A rule may match several — all are emitted; agent decides.

## 7-Bucket Taxonomy (agent applies)

`keep_in_brief` · `enable_recipe` · `use_catalog_dep` · `create_local_skill` ·
`merge_into_skill` · `already_in_atl` · `deprecate_rule_file`. Heuristic guide for
the agent (in `rules-audit.md`): `candidate_recipes` non-empty → `enable_recipe`;
`already_resolved` true → `already_in_atl`; AGENTS.md section matching brief →
`keep_in_brief`; project-specific convention, no recipe → `create_local_skill`;
legacy file fully covered elsewhere → `deprecate_rule_file`.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `lib/_internal/rules-inventory.py` | Create | Read-only scanner; emits inventory JSON to stdout |
| `lib/rules-audit.sh` | Create | CLI wrapper (clone of `doctor.sh`); `exec python3 rules-inventory.py <path>` |
| `bin/ai-specs` | Modify | Add `rules-audit) bash "$LIB_DIR/rules-audit.sh" "$@" ;;` + help line |
| `bundled-commands/rules-audit.md` | Create | Agent command: run CLI → consume JSON → classify → write plan |
| `bundled-commands/skills-as-rules.md` | Modify | Remove stale "auto-invoke table" claims; link `/rules-audit` |
| `tests/test_rules_audit.py` | Create | unittest over inventory: JSON shape + zero-write assertion |

## Interfaces / Contracts

```python
# rules-inventory.py — mirrors doctor.py
RECIPE_KEYWORDS: dict[str, tuple[str, ...]]          # recipe_id -> keywords
@dataclass
class InventoryItem: path: str; ...; candidate_recipes: list[str]
@dataclass
class RulesInventory:
    root: Path
    def scan(self) -> dict          # builds the JSON dict above; NO writes
def main() -> int                   # argv[1]=path; print(json.dumps(scan())); 0
```

CLI: `ai-specs rules-audit [path]` → JSON on stdout, exit 0; `--help` like doctor.

## Testing Strategy (strict_tdd)

| Layer | What | Approach |
|-------|------|----------|
| Unit | JSON shape + keys | `tmp_path` fixtures: fake `.mdc`, `.cursorrules`, `AGENTS.md`, manifest; assert top-level keys, `schema_version`, `mode` |
| Unit | Keyword heuristic | `.mdc` body "run tests in a worktree" → `candidate_recipes` ⊇ {`tdd-flow`,`worktree-flow`} |
| Unit | Read-only invariant | snapshot `set(root.rglob('*'))` + mtimes before/after `scan()`; assert unchanged (the hard guarantee for D5) |
| Unit | Mode B | empty project (no manifest, no legacy) → `mode == "B"`, `stack_hints` from lockfiles |
| Validation | py_compile + bash -n | `./tests/validate.sh` covers `rules-inventory.py`, `rules-audit.sh`, `bin/ai-specs` |

RED first: write `test_rules_audit.py` asserting shape; run `./tests/run.sh` (fail);
implement scanner to GREEN.

## Migration / Rollout

Additive. Bundled command auto-distributes on next `refresh-bundled` + `sync-agent`.
Rollback: delete new files, revert `bin/ai-specs` case + `skills-as-rules.md`,
re-run `refresh-bundled.py` + `sync-agent.sh` to drop distributed copies. No state/data touched.

## Open Questions

- None blocking. `ai-specs/plans/` is created by the agent at plan-write time
  (Python never writes), consistent with the read-only invariant.
