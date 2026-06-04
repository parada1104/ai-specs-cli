# Proposal: `/rules-audit` migration-plan command

## Intent

Projects arriving at ai-specs from a Cursor/`AGENTS.md` legacy have rules scattered across `.cursor/rules/*.mdc`, `.cursorrules`, and monolithic `AGENTS.md`. There is no batch tool to inventory and classify them into the ai-specs target model (runtime-brief `AGENTS.md`, `auto_invoke` Skills, `.atl/skill-registry.md`). `/skills-as-rules` only handles one rule at a time. `/rules-audit` produces a READ-ONLY migration plan classifying every legacy rule item.

## Scope

### In Scope
- `bundled-commands/rules-audit.md` — new slash command (auto-distributed by existing fan-out, no pipeline change).
- `lib/_internal/rules-inventory.py` — read-only scanner of `.cursor/rules/**/*.mdc`, `.cursorrules`, `AGENTS.md` status, manifest, resolved skills, recipe catalog, `.atl/skill-registry.md`; emits JSON. MUST NOT write files.
- `lib/rules-audit.sh` + `bin/ai-specs` `rules-audit` case (parallels `doctor`).
- `tests/test_rules_audit.py` — unittest (strict_tdd true).
- Fix `bundled-commands/skills-as-rules.md`: remove stale "AGENTS.md auto-invoke table" claim; align to runtime-brief reality; link `/rules-audit`.

### Out of Scope
- Heavy Mode B (greenfield) logic — kept lightweight (see Decision 1).
- Auto-applying migrations or running `ai-specs sync`. Plan is advisory only.
- Per-harness or multi-file plan output (single dated file per run).

## Capabilities

### New Capabilities
- `rules-audit`: read-only inventory + classification of legacy rule files into the ai-specs target model, emitting an advisory migration plan.

### Modified Capabilities
- None (no spec-level requirement changes to existing capabilities; the `skills-as-rules.md` edit is a doc correction, not a behavior contract).

## Approach (Decisions)

- **D1 — Mode scope.** v1 focuses on **Mode A** (legacy inventory + classification, the core value). **Mode B** ships as a LIGHTWEIGHT branch: when no legacy rules are detected (greenfield), recommend `ai-specs init` + default recipes by detected stack + a `[brief]` draft. No heavy logic.
- **D2 — Taxonomy (7 buckets).** `keep_in_brief` / `enable_recipe` / `use_catalog_dep` / `create_local_skill` / `merge_into_skill` / `already_in_atl` / `deprecate_rule_file`. Classifications are suggestions, not directives.
- **D3 — `.cursorrules`.** Monolithic file included in inventory scope alongside `.cursor/rules/*.mdc` (cheap to add).
- **D4 — Doc fix.** Remove the stale auto-invoke-table claim in `skills-as-rules.md` AND lightly update to reflect runtime-brief reality + link `/rules-audit`.
- **D5 — Read-only guarantee.** `rules-inventory.py` MUST NOT write any file; only the agent (via the bundled command) writes the plan to `ai-specs/plans/rules-migration-<date>.md`.

Python emits a deterministic JSON inventory; the agent applies nuanced classification and writes the plan. Reuses `skill_contract.py` frontmatter parsing, `skill-resolution.py` `collect_skills()`, and the `doctor.py` Check pattern.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `bundled-commands/rules-audit.md` | New | Slash command; auto-distributed |
| `bundled-commands/skills-as-rules.md` | Modified | Remove stale claim; runtime-brief alignment |
| `lib/_internal/rules-inventory.py` | New | Read-only scanner → JSON |
| `lib/rules-audit.sh` | New | CLI wrapper |
| `bin/ai-specs` | Modified | Add `rules-audit` case |
| `tests/test_rules_audit.py` | New | unittest for inventory |
| `docs/capabilities.md` / README | Modified | Note new migration utility |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Heuristic mis-classifies a rule | Med | Plan presents buckets as suggestions; agent reviews |
| Accidental write by inventory | Low | Pure-read Python; test asserts no fs writes |
| Multi-harness command drift | Low | Markdown-only; no harness-specific APIs |
| Stale doc reintroduced | Low | Doc fix shipped in same change |

## Rollback Plan

Change is mostly additive. Rollback = remove the new files (`bundled-commands/rules-audit.md`, `lib/_internal/rules-inventory.py`, `lib/rules-audit.sh`, `tests/test_rules_audit.py`), revert the `bin/ai-specs` `rules-audit` case and the `skills-as-rules.md` doc edit, then re-run `refresh-bundled.py` + `sync-agent.sh` to drop the distributed command copies. No state migration or data is touched.

## Dependencies

- Existing fan-out pipeline (`refresh-bundled.py`, `sync-agent.sh`) — unchanged.
- Existing `lib/_internal/skill_contract.py`, `skill-resolution.py`, `doctor.py` for reuse.

## Success Criteria

- [ ] `ai-specs rules-audit [path]` runs read-only and emits valid JSON inventory.
- [ ] `/rules-audit` produces a plan in `ai-specs/plans/rules-migration-<date>.md` classifying items into the 7 buckets.
- [ ] Greenfield project yields the lightweight Mode B recommendation.
- [ ] No files written by the Python helper (asserted by test).
- [ ] `skills-as-rules.md` no longer claims an AGENTS.md auto-invoke table.
- [ ] `./tests/validate.sh` passes.
