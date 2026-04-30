# Verification Report

## Change
motor-agents-md-runtime-brief

## Mode
auto

## Test Results
- `./tests/run.sh`: **PASS** — 205 tests ran in 47.953s, all OK.
- `./tests/validate.sh`: **PASS** — exited 0.

## Spec Compliance

### agents-md-runtime-brief
| Scenario | Status | Notes |
|---|---|---|
| Generated brief contains project identity | COMPLIANT | `lib/_internal/agents-md-render.py` emits Project section with name, manifest path, purpose, enabled runtimes, and integration branch. |
| Generated brief contains MCP configuration | COMPLIANT | MCP section renders server names, URLs/commands, and redacts secrets. Env references are preserved in normalized form; literals become `***`. |
| Generated brief contains active recipes and bindings | COMPLIANT | Active Recipes / Bindings / Capabilities section lists enabled recipes deterministically. |
| Generated brief contains safety and workflow rules | COMPLIANT | Optional sections for Safety Rules, Workflow Rules, Context Sources, Conflict Policy, and Useful Commands are rendered when present in manifest. |
| Generated brief contains context source precedence | COMPLIANT | Context Sources and Conflict Policy sections rendered via `render_optional_section`. |
| Sync does not emit skills table into AGENTS.md | COMPLIANT | Renderer no longer includes skills catalog or Auto-invoke table. Tests assert skills are absent from generated `AGENTS.md`. |
| AGENTS.md size is reduced compared to legacy registry mode | COMPLIANT | Runtime brief is concise; skills moved to separate artifact. |
| Re-sync produces identical AGENTS.md | COMPLIANT | `test_sync_produces_identical_agents_md_on_second_run` asserts byte equality. Manual idempotency smoke test in worktree also passed. |
| Manual runtime brief is preserved | COMPLIANT | `test_sync_preserves_runtime_brief_marker_in_agents_md` asserts file is untouched when `<!-- ai-specs:runtime-brief -->` is present. Worktree `AGENTS.md` (which has the marker) was verified preserved. |
| MCP with env-backed secret | COMPLIANT | `test_sync_redacts_literal_mcp_secrets_in_agents_md` confirms literals are redacted to `***` and env refs like `$DEMO_MODE` are preserved as `${DEMO_MODE}`. |

### skill-registry-artifact
| Scenario | Status | Notes |
|---|---|---|
| Sync generates registry artifact | COMPLIANT | `lib/_internal/registry-render.py` creates `ai-specs/.skill-registry.md` on every sync. `AGENTS.md` does not contain skill index or Auto-invoke mappings. |
| Registry artifact path is conventional | COMPLIANT | Hardcoded conventional path is `ai-specs/.skill-registry.md`. |
| Local skills are indexed | COMPLIANT | Skill Index includes local skills with correct Source=`local`. |
| Recipe skills are indexed | COMPLIANT | Skill Index includes recipe skills with correct Source=`recipe`. |
| Dependency skills are indexed | COMPLIANT | Skill Index includes dependency skills with correct Source=`dep`. |
| Auto-invoke rows are emitted to registry artifact | COMPLIANT | One row per trigger per scope per skill with complete metadata. `AGENTS.md` does not contain Auto-invoke rows. |
| Skills without auto-invoke metadata are omitted from Auto-invoke table | COMPLIANT | Manual-only skills appear in Skill Index but not in Auto-invoke Mappings. |
| Hand-edited registry artifact is overwritten on sync | COMPLIANT | Registry artifact is fully regenerated on every sync; no merge logic preserves hand-edits. |
| Re-sync produces identical registry artifact | COMPLIANT | `test_sync_produces_identical_registry_on_second_run` asserts byte equality. Manual smoke test also passed. |
| README mentions registry artifact | COMPLIANT | README MVP table lists "Skill registry artifact" with description of auto-generated `ai-specs/.skill-registry.md`. |

### recipe-sync-materialization (delta)
| Scenario | Status | Notes |
|---|---|---|
| Full materialization with external directories | COMPLIANT | Recipe skills materialize to `.recipe/{recipe-id}/skills/`; dep skills to `.deps/{dep-id}/skills/`; other primitives to `ai-specs/`. Registry artifact reflects new skills. |
| Re-sync idempotency with external directories | COMPLIANT | Tests and manual smoke test confirm no unintended modifications on second sync. |
| Recipe skill materialization path | COMPLIANT | Bundled skills preserved under `.recipe/{recipe-id}/skills/{skill-id}/`. |
| Dependency skill materialization path | COMPLIANT | Dependency skills placed under `.deps/{dep-id}/skills/{skill-id}/`. |
| Local skills directory untouched | COMPLIANT | `ai-specs/skills/` remains exclusively for local skills. |

### skill-frontmatter-contract (delta)
| Scenario | Status | Notes |
|---|---|---|
| Canonical local skill parses successfully | COMPLIANT | `skill_contract.py` accepts canonical frontmatter and extracts `description_summary` with trigger text removed. |
| Manual-only local skill remains valid | COMPLIANT | Skills without `scope`/`auto_invoke` are valid and produce no Auto-invoke row. |
| Compatibility mode normalizes legacy local metadata | COMPLIANT | Scalar `auto_invoke` normalized to list; missing rollout fields get compatibility defaults with warnings. |
| Vendored metadata is derived from manifest inputs | COMPLIANT | `vendor-skills.py` generates canonical vendored frontmatter from `[[deps]]` fields. |
| Hand-edited vendored frontmatter is rewritten | COMPLIANT | Vendored skills are regenerated from manifest on every sync. |
| Incomplete sync metadata fails actionably | COMPLIANT | `test_sync_fails_with_actionable_contract_error_for_invalid_skill_metadata` and `test_sync_fails_on_auto_invoke_without_scope` confirm sync exits non-zero with actionable error naming the missing field and skill path. |
| Complete sync metadata generates registry artifact rows | COMPLIANT | Complete `scope` + `auto_invoke` produces rows in `.skill-registry.md`; `AGENTS.md` does not contain them. |
| Contract document describes required and generated fields | COMPLIANT | `ai-specs/contracts/skill-frontmatter.md` documents required local fields, optional sync metadata, generated vendored fields, compatibility behavior, and hard-fail cutover. |
| Generated skill files are treated as derived output | COMPLIANT | Contract directs changes to manifest or root local skill source, not hand-editing derived output. |

## Task Completion
- Total tasks: 34
- Complete: 34
- Incomplete: none

## Findings
- All 205 tests pass and `./tests/validate.sh` exits cleanly.
- Manual idempotency smoke test in the worktree passed: running `ai-specs sync` twice produced byte-identical `AGENTS.md` and `ai-specs/.skill-registry.md`.
- The worktree's existing `AGENTS.md` contains `<!-- ai-specs:runtime-brief -->` and was confirmed NOT overwritten by sync.
- `ai-specs/.skill-registry.md` exists, has the "Do not edit by hand" header, Skill Index table, and Auto-invoke Mappings table.
- README and `ai-specs/contracts/skill-frontmatter.md` correctly reference the registry artifact instead of `AGENTS.md` for Auto-invoke mappings.
- No unintended modifications occurred in `.recipe/`, `.deps/`, or `ai-specs/` on re-sync.

## Verdict
PASS
