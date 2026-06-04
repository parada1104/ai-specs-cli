# Tasks: rules-migration-audit (/rules-audit)

## Phase 1 — Infrastructure

- [x] 1.1 Create `tests/test_rules_audit.py` — skeleton with import + placeholder test (ensures test file is discoverable by `./tests/run.sh`)
- [x] 1.2 **RED** Write `test_read_only_invariant` — snapshot `rglob + mtimes` in `tmp_path` before/after calling scanner; assert zero filesystem mutations
- [x] 1.3 **RED** Write `test_json_shape` — given `tmp_path` fixture with fake `.mdc` + `.cursorrules` + `AGENTS.md`, assert top-level keys (`schema_version`, `mode`, `target`, `sources`, `summary`, `classification_is_suggestion`) present in parsed JSON stdout
- [x] 1.4 **RED** Write `test_mode_a_detection` — fixture has `.mdc`; assert `mode == "A"` and non-empty `sources.cursor_rules`
- [x] 1.5 **RED** Write `test_mode_b_detection` — fixture is empty project; assert `mode == "B"` and `stack_hints` list present
- [x] 1.6 **RED** Write `test_missing_sources_absent` — fixture has no `.cursorrules`; assert `sources.cursorrules` entry has `status == "absent"`
- [x] 1.7 **RED** Write `test_keyword_heuristic` — body containing "run tests in a worktree" → `candidate_recipes` includes `tdd-flow` AND `worktree-flow`
- [x] 1.8 Run `./tests/run.sh` — confirm all 6 new tests fail (RED evidence)

## Phase 2 — Core Implementation

- [x] 2.1 **GREEN** Create `lib/_internal/rules-inventory.py` — `Source / InventoryItem / RulesInventory` classes mirroring `doctor.py`; implement `scan(path)` emitting JSON to stdout; reuse `split_frontmatter`, `parse_frontmatter`, `collect_skills`, `load_recipe_toml` (import from sibling modules)
- [x] 2.2 Implement `.cursor/rules/**/*.mdc` scan in `RulesInventory.scan()` — per-item: path, description, globs, always_apply, body_excerpt, candidate_recipes (from `RECIPE_KEYWORDS` map), already_resolved flag
- [x] 2.3 Implement `.cursorrules` scan — single-item: full body_excerpt, candidate_recipes; emit `{"status":"absent"}` when file missing
- [x] 2.4 Implement `AGENTS.md` presence flag, manifest parse (`ai-specs.toml`), `collect_skills()` integration, recipe catalog (6 entries), `.atl/skill-registry.md` scan
- [x] 2.5 Implement mode detection: Mode A if any `.mdc` or `.cursorrules` exists; Mode B otherwise; add `stack_hints` from lockfile presence for Mode B
- [x] 2.6 Add `RECIPE_KEYWORDS` static map: `worktree-flow←worktree`, `git-pr-flow←git/PR/pull request`, `tdd-flow←tdd/test`, `trello-mcp-workflow←trello/board/card`, `vault-canonical-store←vault/obsidian/canonical`, `session-context←session/bootstrap`
- [x] 2.7 Emit `classification_is_suggestion: true` at top level; classify each item into one of 7 buckets as candidate only
- [x] 2.8 Run `./tests/run.sh` — confirm all Phase 1 tests pass (GREEN evidence)

## Phase 3 — CLI Wiring

- [x] 3.1 Create `lib/rules-audit.sh` — clone `doctor.sh` pattern; resolve path arg or default to `$(pwd)`; exec `python3 lib/_internal/rules-inventory.py <path>`; exit non-zero on missing path with human-readable error
- [x] 3.2 Modify `bin/ai-specs` — add `rules-audit` case dispatching to `lib/rules-audit.sh`; add `rules-audit` to help output
- [x] 3.3 **RED** Write `test_cli_help_lists_rules_audit` in `test_rules_audit.py` — run `bin/ai-specs help`; assert `rules-audit` appears in output
- [x] 3.4 **RED** Write `test_cli_missing_path_exits_nonzero` — run `bin/ai-specs rules-audit /nonexistent`; assert exit code != 0 and stderr non-empty
- [x] 3.5 **GREEN** Run `./tests/run.sh` — confirm 3.3 and 3.4 pass

## Phase 4 — Bundled Commands

- [x] 4.1 Create `bundled-commands/rules-audit.md` — agent command: invoke `ai-specs rules-audit`, parse JSON, classify items into 7 buckets, write `ai-specs/plans/rules-migration-<YYYY-MM-DD>.md` grouped by bucket; include heuristic guide for bucket assignment
- [x] 4.2 Modify `bundled-commands/skills-as-rules.md` — remove stale AGENTS.md auto-invoke table references at lines 11, 106, 119, 134; replace with runtime-brief reality; add link to `/rules-audit` for batch migration
- [x] 4.3 **Verification** Run `refresh-bundled.py` (or `ai-specs sync`) — assert `rules-audit.md` appears in each harness `commands_dir` (claude, cursor, opencode, omp); assert `skills-as-rules.md` distributed copy has no auto-invoke table reference

## Phase 5 — Documentation & Validation

- [x] 5.1 Update `README.md` — document `ai-specs rules-audit [path]` command; describe 7-bucket taxonomy and plan output location; mention `/rules-audit` slash command
- [x] 5.2 Run `./tests/validate.sh` — confirm py_compile passes on `rules-inventory.py` and `test_rules_audit.py`; bash -n passes on `rules-audit.sh` and `bin/ai-specs`; all unit tests pass
