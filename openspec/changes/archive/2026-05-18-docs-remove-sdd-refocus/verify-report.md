## Verification Report

**Change**: docs-remove-sdd-refocus
**Version**: N/A (docs-only change)
**Mode**: Standard

---

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 11 |
| Tasks complete | 11 |
| Tasks incomplete | 0 |

All 11 tasks checked off. Task completion verified against actual files:

**Phase 1: Deletions (3/3)**
- ✅ 1.1 — `docs/ai/sdd.md` confirmed deleted
- ✅ 1.2 — `docs/ai/examples/config.yaml` confirmed deleted
- ✅ 1.3 — `catalog/skills/openspec-sdd-conventions/` directory confirmed deleted

**Phase 2: Reference-document modifications (3/3)**
- ✅ 2.1 — `docs/ai-specs-toml.md`: `[sdd]` removed from surface list, field table, manifest sections section, example, and See also. SDD link removed from Compatibility rules.
- ✅ 2.2 — `docs/ai/troubleshooting.md`: "SDD failures" section (3 subsections) deleted. SDD link removed from See also. Subtitle updated.
- ✅ 2.3 — `docs/ai/examples/minimal-manifest.toml`: `[sdd]` block removed.

**Phase 3: Top-level document modifications (2/2)**
- ✅ 3.1 — `README.md`: `ai-specs sdd` CLI row removed. SDD key concept replaced with "Harness engineering". Manifest description updated from "...SDD configuration" to "...and recipes".
- ✅ 3.2 — `AGENTS.md`: OpenSpec/SDD stripped from Purpose, Runtime Flow, Context Sources, Conflict Policy, and Workflow Rules. Replaced with neutral harness/project-tracking language.

**Phase 4: Catalog modifications (3/3)**
- ✅ 4.1 — `catalog/README.md`: `openspec-sdd-conventions` entry removed. `[[deps]]` example removed. "Repeat with" sentence updated.
- ✅ 4.2 — `catalog/recipes/trello-mcp-workflow/README.md`: "ai-specs SDD workflows" → "ai-specs projects".
- ✅ 4.3 — `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md`: All SDD/OpenSpec language removed. Triggers generalized to project-agnostic wording. Phase mappings and label tables removed.

---

### Build & Tests Execution

**Build**: ➖ Not applicable (docs-only change, no build step)

**Tests**: ❌ 5 failed / 4 passed / 0 skipped

```
python3 -m unittest discover -s tests -p 'test_*.py'
...
FAILED (failures=5)
```

**Failures breakdown:**

| Test | Cause | Relation to change |
|------|-------|--------------------|
| `test_manifest_reference_lists_canonical_surface_and_compatibility_rules` | Expects `Omission of [sdd] remains valid...` — removed from doc | 🔴 Caused by this change |
| `test_manifest_reference_marks_out_of_scope_items_as_deferred` | Expects `[memory] (distinct from [sdd].artifact_store = memory)` — removed from doc | 🔴 Caused by this change |
| `test_readme_links...` — `[`docs/ai/sdd.md`](docs/ai/sdd.md)` | SDD section removed from README, link no longer exists | 🔴 Caused by this change |
| `test_readme_links...` — `[`docs/skills-by-agent.md`](docs/skills-by-agent.md)` | Link never existed in README on this branch | ⚪ Pre-existing (not caused by this change) |
| `test_readme_links...` — `[`docs/bundled-merge-rules.md`](docs/bundled-merge-rules.md)` | Link never existed in README on this branch | ⚪ Pre-existing (not caused by this change) |

All failures are stale test expectations, not functional regressions. The documentation content is correct.

**Coverage**: ➖ Not available (Python unittest, no coverage tool configured)

---

### Spec Compliance Matrix

| Requirement | Scenario | Evidence | Result |
|-------------|----------|----------|--------|
| R1: No SDD/OpenSpec in user-facing docs | — | Grep sweep of all target files: zero matches | ✅ COMPLIANT |
| R2: Deleted files gone | Scenario: Deleted files return 404 | `test -f docs/ai/sdd.md` → GONE, `test -f docs/ai/examples/config.yaml` → GONE, `test -f catalog/skills/openspec-sdd-conventions/SKILL.md` → GONE | ✅ COMPLIANT |
| R3: README has accurate primitives | Scenario: README reader sees accurate primitives | README has "Harness engineering", "Manifest", "Agents", "MCP servers", "Skills", "Recipes", "fan-out". No SDD/OpenSpec references. No `ai-specs sdd` CLI row. | ✅ COMPLIANT |
| R4: AGENTS.md internally consistent | Scenario: AGENTS.md remains coherent | AGENTS.md has neutral runtime flow: no OpenSpec/SDD workflow prescriptions. Uses "project's designated spec store", "project's designated workflow". Trello, Vault, Engram context sources intact. | ✅ COMPLIANT |
| R5: No broken cross-links | — | All cross-referenced files exist (`docs/recipe-schema.md`, `docs/mcp-distribution.md`, `docs/skills-by-agent.md`, `docs/bundled-merge-rules.md`). No links to deleted files found. | ✅ COMPLIANT |
| R6: Trello recipe neutral wording | Scenario: Catalog README without SDD skill | `catalog/recipes/trello-mcp-workflow/README.md`: "Automated Trello board integration for ai-specs projects." No SDD references. | ✅ COMPLIANT |

---

### Correctness (Static — Structural Evidence)

| # | Requirement | Status | Notes |
|---|-------------|--------|-------|
| R1 | No SDD/OpenSpec in user-facing docs | ✅ Implemented | All 8 modified files + 3 deleted files verified clean |
| R2 | Deleted files gone | ✅ Implemented | 3 files/directories confirmed absent |
| R3 | README describes core primitives | ✅ Implemented | Harness engineering paragraph present; fan-out, skills, MCP, recipes described |
| R4 | AGENTS.md internally consistent | ✅ Implemented | Neutral language throughout; no SDD/OpenSpec traces |
| R5 | No broken cross-links | ✅ Implemented | All referenced docs exist; no links to deleted files |
| R6 | Trello recipe neutral wording | ✅ Implemented | README and SKILL.md use "ai-specs projects" or equivalent |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| Phase 1: Delete 3 files | ✅ Yes | All confirmed absent |
| Phase 2: Modify 3 reference docs | ✅ Yes | `[sdd]` and SDD refs removed from all 3 |
| Phase 3: Modify README + AGENTS.md | ✅ Yes | SDD replaced with harness engineering; OpenSpec/SDD stripped |
| Phase 4: Modify catalog files | ✅ Yes | README + SKILL.md cleaned; openspec-sdd-conventions entry removed |

---

### Remaining SDD/OpenSpec References (Outside Scope)

The following files still contain SDD/OpenSpec references but were explicitly **not in scope** per the task list:

| File | Reference | Task list status |
|------|-----------|------------------|
| `docs/recipe-schema.md` | `[sdd]` recipe metadata section, lines 236-241 | N3: "Do NOT modify" |
| `catalog/recipes/trello-mcp-workflow/templates/card-feature.md` | "## SDD Checklist" heading | Not in task list |
| `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md` | "On OpenSpec change creation", "On SDD phase transitions", "SDD Phase" tables | Not in task list |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | "ai-specs SDD workflows" description | Not in task list |
| `catalog/skills/testing-foundation/SKILL.md` | SDD workflow references in guidance | Not in task list |
| `catalog/skills/context-precedence/SKILL.md` | OpenSpec references in precedence rules | Not in task list |

---

### Non-Goal Compliance

| Non-Goal | Status | Notes |
|----------|--------|-------|
| N1: Don't modify `ai-specs/ai-specs.toml` | ⚠️ Minor violation | Commented-out `[sdd]` block removed (4 lines of comments). Zero functional impact. |
| N2: Don't modify `openspec/specs/` | ✅ Respected | No changes to internal specs |
| N3: Don't modify protected docs | ✅ Respected | `recipe-schema.md`, `mcp-distribution.md`, `skills-by-agent.md`, `bundled-merge-rules.md` unmodified |
| N4: Don't change code in `lib/` or `src/` | ⚠️ Violated by broader commit | `lib/_internal/*.py`, `lib/sdd.sh`, `bin/ai-specs` changed — but this is part of the SDD product code removal commit, not specific to the doc change scope |
| N5: Don't add redirects | ✅ Respected | No redirects or replacement pages created |

---

### Issues Found

**CRITICAL** (must fix before archive):
- None

**WARNING** (should fix):
- **Test expectations stale**: 3 test assertions need updating to match new doc content:
  1. `test_manifest_reference_lists_canonical_surface_and_compatibility_rules` — remove `Omission of [sdd] remains valid...` needle from expected content
  2. `test_manifest_reference_marks_out_of_scope_items_as_deferred` — remove `[memory] (distinct from [sdd].artifact_store = memory)` needle
  3. `test_readme_links_to_dedicated_manifest_and_recipe_references` — remove `[`docs/ai/sdd.md`](docs/ai/sdd.md)` needle
- **N1 violation**: `ai-specs/ai-specs.toml` had commented-out `[sdd]` block removed. Minor — no functional impact — but deviates from spec non-goal.
- **Remaining SDD/OpenSpec references** in `docs/recipe-schema.md` (N3 protected), Trello recipe templates/commands/recipe.toml (outside task scope), and other catalog skills (outside scope). Consider a follow-up change to clean these if R1 ("All user-facing documentation") is interpreted broadly.

**SUGGESTION** (nice to have):
- **Pre-existing test failures**: 2 test assertions (`skills-by-agent.md` link and `bundled-merge-rules.md` link in README) were failing before this change. Consider fixing these in the same test-update pass.
- The `catalog/recipes/trello-mcp-workflow/recipe.toml` description still says "ai-specs SDD workflows" — trivial one-line fix.

---

### Verdict

**PASS WITH WARNINGS**

All 11 tasks are complete. All spec requirements (R1-R6) are satisfied. All target files are clean of SDD/OpenSpec references. Deleted files are absent. No broken cross-links. The 3 test failures directly caused by this change are stale expectations that need updating to match the correct documentation changes — not functional regressions.
