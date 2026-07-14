# Verify Report: hub-fixes

## Status: PASS

## Verification results

### Stage A — Bugs
| # | Bug | Status | Evidence |
|---|---|---|---|
| 1 | importlib NameError in Agents | ✅ FIXED | `import importlib.util` at module scope (hub.py:11). `test_hub_has_importlib_at_module_scope` passes. |
| 2 | Skills categorization (bundled vs local) | ✅ FIXED | skills-list.sh has new "Bundled skills (CLI-shipped)" section (lines 128-159). Bundled skills excluded from Local section (lines 180-183). `test_bundled_skills_not_under_local` passes. |
| 3 | Recipe text prompts | ✅ FIXED | Add/remove use `pick_one` from `recipe_add_choices`/`recipe_remove_choices` built from `list_recipes()`. `test_add_uses_pick_one_not_text` passes. |
| 4 | Version not in StatusPanel | ✅ FIXED | `version` field in `StatusSummary`, rendered in `StatusPanel` (hub.py:319) and `_run_noninteractive` (hub.py:215). `test_status_summary_includes_version` passes. |

### Stage B — Consistency
| # | Feature | Status | Evidence |
|---|---|---|---|
| 1 | Shared helpers (pick_one/pick_many/confirm_action/pause) | ✅ DONE | Defined and used throughout hub.py. Dep-free empty-option short-circuit tested (`test_pick_one_empty_returns_none_without_questionary`). |
| 2 | All pauses via shared pause() helper | ✅ DONE | No bare `input()` or `try/except EOFError` blocks remain. `test_no_bare_press_enter_input_outside_pause` passes. |
| 3 | Skills interactive submenu | ✅ DONE | Commands: List (categorized), Inspect, Back. Uses `categorize_skills()` for buckets. `test_skills_shows_categorized_headers` passes. |
| 4 | _SUB_ARGS deleted | ✅ DONE | Map removed; the `_SUB_ARGS.get()` lookup deleted from `_run_interactive_hub`. |
| 5 | README updated | ✅ DONE | 11 lines added describing hub features. |

### Tests
- **44/44 tests pass** (test_hub.py + test_hub_tui.py)
- **Dep-free import contract preserved**: hub.py imports without rich/questionary at module scope
- **py_compile + bash -n**: clean

### Diff
- 5 files changed: hub.py (+474/-133), skills-list.sh (+51/-16), test_hub.py (+244), test_hub_tui.py (+140), README.md (+11)
- Total: +787/-133

### Files changed
- lib/_internal/hub.py — all changes
- lib/skills-list.sh — bundled skills categorization
- tests/test_hub.py — 34 new tests (importlib, version, skills, recipe pickers, helpers, non-interactive)
- tests/test_hub_tui.py — 10 new tests (recipe picker, skills submenu, version inline, pause only site)
- README.md — hub documentation
