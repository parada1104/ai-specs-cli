# Verification Report: option-c-runtime-brief

**Change**: option-c-runtime-brief
**Mode**: Strict TDD (runner `./tests/validate.sh`)
**Verdict**: PASS WITH WARNINGS — merge needs fixes first (auto-binding gap + committed artifacts)

---

## Full Suite Result

`./tests/validate.sh` → **Ran 297 tests in ~56s — OK** (exit 0). Claimed 297 confirmed.
Worktree git status clean after two consecutive runs (suite uses isolated temp dirs; no worktree mutation).

---

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 24 |
| Tasks complete | 24 |
| Tasks incomplete | 0 |

All tasks checked off and genuinely implemented (verified against source, not just the checkbox).

---

## Four Focus Verdicts

### 1. test_command regression — WARNING (real, but partly intentional)
`[recipes.tdd-flow.config].test_command` changed `./tests/validate.sh` → `./tests/run.sh` (dev → this change).
- `run.sh` = `python3 -m unittest discover` only.
- `validate.sh` = py_compile + `bash -n` syntax checks + `run.sh` (strictly more thorough).
- The change was made deliberately to feed the renderer heuristic (`run.sh`→`validate.sh`) which produces BOTH "Focused tests: ./tests/run.sh" and "Full validation: ./tests/validate.sh" in the brief. The rendered Useful Commands section is therefore CORRECT/richer.
- BUT `tdd-flow.test_command` is the machine-readable runner consumed by sdd-init/strict-TDD as THE verification gate. Agents that read it will now run the weaker `run.sh` (skips compile + bash syntax checks) for the TDD gate. That is a semantic regression of the gate, independent of the brief output.
- Verdict: WARNING. Brief output fine; the gate-runner downgrade should be reconsidered. Fix options: keep `test_command = ./tests/validate.sh` and instead add a separate `[brief].useful_commands` line for the focused command, OR add a dedicated `focused_command` field — do not overload the gate runner to drive a rendering heuristic.

### 2. project name change ai-specs → ai-specs-cli — SUGGESTION (in-scope, low blast radius)
- Task 5.2 explicitly calls for "fix project name to ai-specs-cli", so it is in-scope for the change.
- Blast radius: NO code depends on the literal `[project].name` value. Grep of lib/tests/docs found only `lib/init.sh` using a name as a CLI default arg. All `ai-specs/` references are directory paths, not the project name. The H1 of the generated brief is now "# ai-specs-cli Runtime Brief" (was "# ai-specs Runtime Brief").
- Verdict: SUGGESTION. Legitimate rename matching the repo/board identity; harmless. Note it in the changelog so it is not a silent contract change.

### 3. auto-binding gap — CRITICAL (confirmed empirically; design contract violated)
- Design decision #4 said: "Reuses `resolve_bindings`/`merge_config` already run in sync." `resolve_bindings()` (recipe-materialize.py:224) does Step-2 AUTO-BIND: any capability with exactly one enabled provider is auto-bound.
- The implementation instead added `build_resolved_config()` (recipe-materialize.py:403) which reads ONLY explicit `[[bindings]]` from raw TOML and does NOT auto-bind, NOT use the catalog, NOT call `resolve_bindings`.
- Empirically confirmed: a manifest with single-provider capabilities and NO `[[bindings]]` (the documented convention in docs/capabilities.md and this manifest's own comment lines 33-35) yields `"bindings": {}`. The renderer's `_section_trello` then returns `[]` and `_section_project` drops vault_scope.
  - Repro: `board_id` and `vault_scope` present in manifest → ABSENT from rendered AGENTS.md; Trello Tracking section ABSENT.
- Consequence: This repo had to ADD explicit `[[bindings]]` (lines 102-112) purely to make the brief work — contradicting its own comment "sync auto-binds them; no explicit [[bindings]] needed." ANY other project adopting Option C with single-provider capabilities gets SILENTLY EMPTY Trello/VCS/vault brief sections unless it hand-adds bindings.
- The renderer is correctly catalog-free (design decision #2 — that part is good). The DEFECT is in the PRODUCER: `build_resolved_config` should run the catalog-aware `resolve_bindings(catalog_dir, enabled_ids, manifest_bindings)` (which materialize already computes as `resolved_bindings` at line 484) and emit THAT bindings map, rather than re-deriving a bindings-only-from-explicit map.
- Verdict: CRITICAL. Correct fix: in `materialize_recipes`, pass the already-computed `resolved_bindings` (auto-bind included) into the resolved-config JSON instead of recomputing in `build_resolved_config`. Keep the no-catalog `build_resolved_config` only as the fallback for the 0-enabled / standalone path.

### 4. committed materialization artifacts — WARNING (likely should not be in the change commit)
- Commit `99c2c74` added 25 recipe-materialized files: `ai-specs/.recipe/**` (8 SKILL.md + bootstrap-ready), `ai-specs/commands/*.md` (5), `ai-specs/recipes/**/README.md` + templates (11).
- These were NEVER tracked on `development` (only `ai-specs/commands/skills-as-rules.md`, a hand-authored command, was). `git log --all` shows `.recipe/`/`recipes/` first appear in this very commit.
- The repo's root `.gitignore` (identical on development) does NOT ignore these paths, so git tracks them — but the canonical `templates/gitignore-root.tmpl` (line 15) treats `ai-specs/.recipe/` as ignorable local output. The design's File Changes table does NOT list any of these 25 files.
- Verdict: WARNING. These are local sync materialization output that leaked into the change commit. They are not part of the Option-C feature. Recommend reverting them from the commit (or, if the repo intends to dogfood committed artifacts, do it as a separate explicit decision/commit and add gitignore policy). `.ai-specs.lock` additions for recipe skill hashes are a borderline-acceptable side effect, but the bulk of `.recipe/commands/recipes` should not be here.

---

## Requirements Coverage

| Req | Requirement | Status | Evidence |
|-----|-------------|--------|----------|
| R1 | Brief sections from [brief] table, fixed order, omit-if-absent, arrays→bullets | PASS (partial scenario untested) | `_render_lines` fixed order (agents-render.py:264-318); generated AGENTS.md shows all sections in order; `test_sync_renders_rich_brief_from_manifest` covers "all present"; bullets confirmed. NO direct test for "partial [brief]" scenario, but omit logic verified in code per-section. |
| R2 | Structured fields from --resolved-config (board_id, integration_branch, test_command, vault_scope, provider, base_branch, enabled, mcp names) | PASS | `test_sync_renders_rich_brief_from_manifest` asserts board_id/integration_branch/test_command/vault_scope needles; `--resolved-config absent` covered by `test_agents_render_standalone_degradation`. |
| R3 | Capability-binding lookup names provider recipe; omit if no tracker binding | PASS-but-fragile | `_section_trello` references bound recipe id `trello-mcp-workflow` (AGENTS.md:42). No-tracker omission UNTESTED. NOTE: omission is correct behavior, but combined with finding #3 it MASKS the auto-bind gap (omits when it should auto-bind). |
| R4 | MCP env secrets redacted | PASS | `test_sync_redacts_literal_mcp_secrets_in_agents_md` GREEN; `_redact_env_value` preserved (agents-render.py:31-41). |
| R5 | --preserve-if-runtime-brief escape hatch | PASS | `test_sync_preserves_runtime_brief_marker_in_agents_md` GREEN (uses own fixture, still meaningful). Both scenarios honored in `render()` (agents-render.py:322-324). |
| R6 | Idempotent output | PASS (with newline caveat) | Re-render byte-identical (A.md == B.md). `test_sync_rich_brief_identical_on_second_run` GREEN. CAVEAT: committed AGENTS.md lacks the trailing `\n` the renderer always emits → first sync after merge would churn 1 byte. |
| R7 | Subrepos receive enriched output | PASS (untested) | sync-agent.sh `ensure_target_workspace` passes `--resolved-config` through (lines 209-213); sync.sh passes it on fan-out (line 121). NO test asserts board_id/test_command propagate into a subrepo AGENTS.md — R7 scenario UNTESTED. |

**Compliance summary**: 7/7 requirements structurally met; 4 spec scenarios have NO dedicated behavioral test (R1 partial, R3 no-tracker, R7 subrepo-structured-fields), and R6 has a trailing-newline mismatch between committed file and fresh render.

---

## Spec Compliance Matrix (behavioral)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| R1 | All prose sections present | `test_sync_renders_rich_brief_from_manifest` | COMPLIANT |
| R1 | Partial [brief] table | (none) | UNTESTED |
| R1 | String-array → bullets | `test_sync_renders_rich_brief_from_manifest` | COMPLIANT |
| R2 | board_id needle | `test_sync_renders_rich_brief_from_manifest` | COMPLIANT |
| R2 | integration_branch needle | `test_sync_renders_rich_brief_from_manifest` | COMPLIANT |
| R2 | test_command needle | `test_sync_renders_rich_brief_from_manifest` | COMPLIANT |
| R2 | vault_scope needle | `test_sync_renders_rich_brief_from_manifest` | COMPLIANT |
| R2 | --resolved-config absent | `test_agents_render_standalone_degradation` | COMPLIANT |
| R3 | Tracker named from binding | `test_sync_renders_rich_brief_from_manifest` (explicit binding) | PARTIAL (only explicit-binding path; auto-bind path broken) |
| R3 | No tracker binding | (none) | UNTESTED |
| R4 | Literal secret replaced | `test_sync_redacts_literal_mcp_secrets_in_agents_md` | COMPLIANT |
| R6 | Second sync no diff | `test_sync_rich_brief_identical_on_second_run` | COMPLIANT |
| R5 | File with marker untouched | `test_sync_preserves_runtime_brief_marker_in_agents_md` | COMPLIANT |
| R5 | File without marker overwritten | (covered transitively by sync tests) | COMPLIANT |
| R7 | Subrepo structured fields | (none) | UNTESTED |
| R1/R2 | useful_commands rendered | `test_brief_useful_commands_renders_extra_items` | COMPLIANT |

---

## Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| #1 Prose in [brief] table | YES | Implemented as specified. |
| #2 Temp JSON via --resolved-config; renderer catalog-free | YES | sync.sh mktemp + cleanup; renderer never touches catalog. Good. |
| #3 Compute-in-Python, fixed order, per-section helpers | YES | 10 helpers, fixed order. |
| #4 Binding lookup reuses resolve_bindings/merge_config | **NO — DEVIATED (CRITICAL)** | New `build_resolved_config` reads explicit [[bindings]] only; does NOT auto-bind; ignores catalog and the already-computed `resolved_bindings`. Breaks single-provider auto-bind convention. |
| #5 Migration in one commit | YES (with collateral) | Marker removed + [brief] added in 99c2c74, but 25 unrelated materialization files leaked in. |
| #6 Degradation when JSON absent | YES | Standalone render works; degradation test GREEN. |

---

## Issues Found

**CRITICAL**
- Auto-binding gap (finding #3): `build_resolved_config` does not auto-bind single-provider capabilities; projects without explicit `[[bindings]]` get silently empty Trello/VCS/vault brief sections. Violates design decision #4 and docs/capabilities.md convention. Empirically reproduced.

**WARNING**
- test_command gate downgraded `validate.sh`→`run.sh` (finding #1): the strict-TDD/sdd-init gate runner is now weaker (no compile/bash-syntax checks). Brief output itself is fine.
- Committed materialization artifacts (finding #4): 25 local sync-output files in commit 99c2c74, never tracked before, not in design File Changes.
- Trailing-newline mismatch (R6): committed AGENTS.md has no trailing `\n`; renderer emits one. First sync post-merge would rewrite the file (1-byte churn). Re-fix by letting sync write the file (do not hand-trim) so committed == fresh-render.
- Untested scenarios: R1 partial-brief, R3 no-tracker-omission, R7 subrepo-structured-fields have no behavioral tests.

**SUGGESTION**
- project name rename (finding #2): in-scope, zero code blast radius; document in changelog.
- `engram` is listed in `[brief.mcp_descriptions]` but not in `[mcp.*]`, so it never renders in Runtime MCPs (descriptions only attach to declared servers). The manual brief listed engram; the generated brief omits it. Acceptable (engram is a global MCP, not project-declared) but a minor information loss vs the manual brief — consider a note or a non-MCP "global memory" line if engram visibility matters.

---

## Recommended Fixes (ordered)

1. **(CRITICAL) Fix auto-binding**: in `materialize_recipes`, build the resolved-config bindings from the already-computed catalog-aware `resolved_bindings` (line 484) instead of `build_resolved_config`'s explicit-only map. Keep `build_resolved_config` as the no-catalog fallback for 0-enabled/standalone. Then REMOVE the now-unnecessary explicit `[[bindings]]` from `ai-specs/ai-specs.toml` to re-prove auto-bind, and add a test: single-provider manifest, no `[[bindings]]`, assert board_id/vault_scope present.
2. **(WARNING) Restore TDD gate runner**: set `tdd-flow.test_command` back to `./tests/validate.sh`; surface the focused command via `[brief].useful_commands` (e.g. "Focused tests: ./tests/run.sh") or a dedicated field — do not overload the gate runner for a rendering heuristic.
3. **(WARNING) Remove leaked materialization artifacts** from the change: `git rm --cached` the 25 `.recipe/commands/recipes` files (or revert them from 99c2c74), unless committing them is a separate explicit decision with gitignore policy.
4. **(WARNING) Fix trailing-newline**: regenerate AGENTS.md via sync so the committed file matches a fresh render (with trailing `\n`); avoid hand-trimming.
5. **(WARNING) Add behavioral tests** for R1 partial-brief, R3 no-tracker-omission, R7 subrepo structured-field propagation.

---

## Merge Readiness

**needs-fixes-first.** Blocking: finding #3 (CRITICAL auto-binding gap) makes Option C non-portable to other projects and contradicts the stated convention. Strongly recommended before merge: findings #1 (gate runner) and #4 (leaked artifacts). The feature works for THIS repo only because explicit bindings were hand-added — which is exactly the situation Option C was meant to remove.
