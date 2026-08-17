# Tasks: retire-decision-matrix

Depth: full

Branch / worktree: `change/retire-decision-matrix` /
`.worktrees/retire-decision-matrix/`

Plan refs: `explore.md`, `proposal.md`, `design.md`,
`specs/plan-build-flow/spec.md`

**Stop for human authorization before production-code apply.** This file is the
implementation plan only — do not write production code or tests while
authoring it.

---

## Tracker

- **card_id**: `6a78c1e212f9b77a94405085`
- **shortLink**: `oUfVVwde`
- **url**: https://trello.com/c/oUfVVwde/66-retire-decision-matrix-and-consolidate-into-plan-build-flow

The change is tracked in Trello card #66 in the In Progress list. Keep the
card phase aligned with Apply, Verify, and Archive transitions.
***
## Locked delivery decisions (human)

| Decision | Value |
|---|---|
| Retirement scope | **Full removal** of live spec + config section + doc section; test/README updates in the same change; archive preserved byte-identical |
| Migration mapping | `trivial`/`local_fix` → `Light`; `behavior_change` → `Standard`; `domain_change` → `Full` (normative in the plan-build delta spec) |
| Config replacement | **None** — plan-build resolves depth from classifier + topology; no `[sdd]`, no decision matrix, no `artifact_root` |
| Doc/README policy | `docs/recipe-schema.md` loses `[sdd]` metadata; trello README ceremony note points at plan-build; `docs/recipes-catalog.md` verified, not rewritten |
| Test policy | Update `test_manifest_contract_docs.py`; add doc-scan + mapping tests; existing plan-build vocabulary-hygiene tests stay green |
| Archive policy | `openspec/changes/archive/` untouched (immutable audit trail) |
| Tracker | **TBD** — no card; backfill before apply |

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~150–260 (1 dir delete + config block remove + doc section remove + test edits/additions + README note + changelog entry + spec delta) |
| 400-line budget risk | **Low** |
| Chained PRs recommended | No — single reviewable change |
| Suggested split | One PR: docs/config/spec retirement + test cutover |

```text
Decision needed before apply: Yes (authorization gate)
Chained PRs recommended: No
400-line budget risk: Low
```

---

## Planning depth

| Field | Value |
|---|---|
| Depth | `full` |
| Signal | Cross-cutting removal of a canonical spec + live config + docs + tests, replaced by a plan-build contract amendment — multi-area, architectural cleanup |
| Explicit request | None (no user depth request in the assignment) |
| Conflict | No explicit request → signal decides; no conflict annotation required |
| Decision source | signal |

---

## Phase 1: Spec retirement and canonical spec delta

- [x] 1.1 Delete `openspec/specs/sdd-adaptive-contract/` (whole directory,
      including `spec.md`). Verify with `git status` that no file remains
      outside the change folder. Do **not** touch
      `openspec/changes/archive/2026-04-30-definir-sdd-adaptive-contract/`.
- [x] 1.2 Land the delta spec
      `openspec/changes/retire-decision-matrix/specs/plan-build-flow/spec.md`
      ("Legacy ceremony vocabulary retirement" requirement + 6 scenarios) as
      authored in this change folder. (This change's spec delta; the canonical
      sync happens at archive time per the normal plan-build flow.)
- [x] 1.3 Update `openspec/config.yaml`: remove the `sdd:` section (the
      `mode` key, `decision_matrix` block, and both comment lines citing
      `sdd-adaptive-contract`). Leave `tracking:` and all other keys
      byte-identical. Verify `python3 -c "import yaml; yaml.safe_load(open('openspec/config.yaml'))"`
      parses and `grep -n sdd openspec/config.yaml` is empty.

## Phase 2: Documentation removal

- [x] 2.1 `docs/recipe-schema.md`: delete the `## `[sdd]` recipe metadata`
      section — the heading, the intro paragraph, the `### threshold`
      subsection with its field table, the TOML example block, and the
      "Invalid threshold values are rejected when the recipe is parsed." note
      (lines ~479–500). Verify `grep -n "sdd" docs/recipe-schema.md` is empty
      and the surrounding `[init]` section / Reference recipe section remain
      intact.
- [x] 2.2 `catalog/recipes/trello-mcp-workflow/README.md`: replace the
      `## Ceremony vocabulary note` body (lines ~123–129) with a note that
      ceremony/depth classification lives in `plan-build-flow` (`Light` /
      `Standard` / `Full`) and that the legacy `trivial` / `local_fix` /
      `behavior_change` / `domain_change` vocabulary is retired. Do not
      reference `sdd.decision_matrix` or the retired spec by name in the new
      note. Leave the rest of the README untouched.
- [x] 2.3 `docs/recipes-catalog.md`: verify (grep) there is no live
      reference to `sdd`, `decision_matrix`, `threshold`, or
      `sdd-adaptive-contract`. If the `plan-build-flow` section (line ~198+)
      does not already state that it is the depth/ceremony source, add a
      single line under its description noting it replaces the retired
      ceremony contract — only if needed; do not rewrite the section.
- [x] 2.4 `CHANGELOG.md`: add a new entry under Unreleased/Changed (above the
      existing 0.19.0-era entries): "removed the retired `sdd-adaptive-contract`
      ceremony contract: deleted the canonical spec, the `sdd.decision_matrix`
      section in `openspec/config.yaml`, and the `[sdd]` recipe metadata in
      `docs/recipe-schema.md`; `plan-build-flow` (Light/Standard/Full) is now
      the sole ceremony contract." Leave the historical line 64 entry
      untouched.

## Phase 3: Test cutover (strict TDD — RED first)

- [x] 3.1 **RED**: run `tests/test_manifest_contract_docs.py` after Phase 2 —
      `test_recipe_reference_covers_current_v2_contract_and_boundaries` must
      fail on the removed `## `[sdd]` recipe metadata` / `threshold` rows.
      Record the failure output as RED evidence in `verify-report.md` during
      apply.
- [x] 3.2 Update `test_recipe_reference_covers_current_v2_contract_and_boundaries`
      in `tests/test_manifest_contract_docs.py`: remove
      `"## `[sdd]` recipe metadata"` and
      `"| `threshold` | string | no | Optional ceremony level: `trivial`, `local_fix`, `behavior_change`, or `domain_change` |"`
      from the `assertContainsAll` list. Keep every other assertion.
- [x] 3.3 Add `test_recipe_doc_has_no_retired_sdd_surface` in
      `tests/test_manifest_contract_docs.py`: `assertNotIn` for
      `"## `[sdd]` recipe metadata"` and `"threshold"` in `self.recipe_doc`.
- [x] 3.4 Add `test_legacy_ceremony_mapping_table` in
      `tests/test_manifest_contract_docs.py` (or the closest doc-contract
      module): read the change delta
      `openspec/changes/retire-decision-matrix/specs/plan-build-flow/spec.md`
      and assert all four mapping rows are present:
      `trivial` and `local_fix` → `Light`, `behavior_change` → `Standard`,
      `domain_change` → `Full`.
- [x] 3.5 Add a retired-token doc scan (same module or
      `tests/test_plan_build_flow_recipe.py`): scan
      `openspec/config.yaml`, `docs/recipe-schema.md`,
      `docs/recipes-catalog.md`, and
      `catalog/recipes/trello-mcp-workflow/README.md` and assert the tokens
      `sdd-adaptive-contract`, `openspec-sdd-decision`, `sdd.decision_matrix`,
      and `sdd.threshold` are absent. Explicitly exclude
      `openspec/changes/archive/` and `CHANGELOG.md` from the scan.
- [x] 3.6 **GREEN**: re-run `tests/test_manifest_contract_docs.py` and
      `tests/test_plan_build_flow_recipe.py` — all pass. Record GREEN
      evidence.

## Phase 4: Verification

- [x] 4.1 Run `./tests/validate.sh` (py_compile + bash -n + full
      `./tests/run.sh`); capture pass/fail counts. No regressions outside the
      doc-contract modules are expected. — GREEN with GNU bash 5.3.9: 1378
      tests OK, exit 0 (2026-08-09).
- [x] 4.2 Verify `openspec/config.yaml` YAML-parseable and `sdd:`-free
      (Ruby Psych parse OK; `grep -n sdd` empty).
- [x] 4.3 Verify archive immutability: `git status -- openspec/changes/archive/`
      clean.
- [x] 4.4 Verify the live-tree scan from task 3.5 against the committed tree
      (same exclusions) — enforced by
      `test_no_retired_ceremony_tokens_in_live_surfaces`.
- [x] 4.5 Write `verify-report.md` for this Full change: strict global `PASS`
      verdict, `ready_for_archive: true`, `## Success-criteria mapping` block
      with one `- Criterion N: PASS` row per proposal success criterion, the
      verify command, exit status, `YYYY-MM-DD` date, and commit SHA (Full
      evidence shape per the plan-build staged verify gate).
- [x] 4.6 Backfill the `## Tracker` section with the real card_id / shortLink /
      url — done; card #66 exists and the block is present in proposal.md and
      tasks.md.

## Phase 5: Archive and cleanup

- [ ] 5.1 Archive-tail on the review branch (pre-merge, per plan-build): move
      `openspec/changes/retire-decision-matrix/` to
      `openspec/changes/archive/retire-decision-matrix/` only after verify
      evidence passes and the merge gate is green. — deferred: PR must be
      created first (no archive move before PR unless plan-build policy
      requires it; archives preserved).
- [ ] 5.2 Confirm no orphaned references: `grep -rn "sdd-adaptive-contract"`
      across the live tree returns only `openspec/changes/archive/`,
      `CHANGELOG.md`, and this change folder.
- [ ] 5.3 Update `docs/recipes-catalog.md`/`README.md` only if a user-facing
      surface was missed by the Phase 2 sweep (unlikely — verified by 2.3).
