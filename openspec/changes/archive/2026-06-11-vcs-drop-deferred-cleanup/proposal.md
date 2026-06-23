# Proposal: VCS Drop Deferred Cleanup

## Intent

Close the 3 deferred items from Trello card #23 and `vcs-drop-provider-config` verify report: VCS fragment leakage, missing GitHub README doc-contract symmetry, and silent custom `vcs-pr-flow` fallback.

## Scope

### In Scope
- Filter VCS `workflow_rules` fragments so only the bound recipe contributes host-specific prose.
- Add the missing `git-pr-flow` README/catalog no-`provider` doc contract test.
- Emit an explicit `⚠ ai-specs:` stderr warning for unknown/custom bound VCS recipe ids and render a generic `VCS PR (custom)` label.
- Preserve strict TDD: every code/doc-contract change starts with a RED test, then GREEN implementation.

### Out of Scope
- Renderer contract redesign from `vcs-drop-provider-config`.
- New VCS recipes, recipe deprecations, or CLI command changes.
- Broader doc rewrites.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `vcs-pr-flow`: require unknown/custom bound recipe ids to warn explicitly and render a safe generic label. Fragment filtering and the `git-pr-flow` doc test are implementation/test gaps already implied by the current spec.

## Approach

Use Trello card #23 as scope authority and the archived verify report as technical context. Add RED coverage for all 3 ACs, then update renderer fragment selection and VCS fallback behavior. Mirror existing GitLab/Bitbucket catalog tests for GitHub.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `lib/_internal/agents-render.py` | Modified | Bound-only VCS fragment filtering and custom recipe label fallback warning. |
| `tests/test_sync_pipeline.py` | Modified | Sync/render tests for 3 VCS recipes with 1 binding and unknown custom fallback. |
| `tests/test_recipes_catalog.py` | Modified | Symmetric `git-pr-flow` README/catalog no-`provider` assertion. |
| `openspec/changes/vcs-drop-deferred-cleanup/specs/vcs-pr-flow/spec.md` | New | Delta for custom fallback warning only. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Silent fallback hides broken custom VCS recipe ids | Med | Warn with `⚠ ai-specs:` and render `VCS PR (custom)`. |
| Filtering removes non-VCS fragments accidentally | Low | Filter only VCS sibling fragments when a `vcs-pr-flow` binding exists. |
| Doc contract misses config-table boundaries | Low | Mirror existing GitLab/Bitbucket section helpers for GitHub. |

## Rollback Plan

Revert this change folder and implementation commits. Renderer behavior returns to all-enabled fragment collection and known-id-only VCS bullets; no data migration.

## Dependencies

- Trello card #23 (`6a2a7d6b596643e2ab4adcfd`) acceptance criteria.
- Archived verify report: `openspec/changes/archive/2026-06-11-vcs-drop-provider-config/verify-report.md`.

## Success Criteria

- [ ] Tests prove only the bound VCS recipe contributes fragments when 3 VCS recipes are enabled.
- [ ] `git-pr-flow` README/catalog no-`provider` contract is symmetric with GitLab/Bitbucket.
- [ ] Unknown custom `vcs-pr-flow` ids warn to stderr and render a generic label.
- [ ] `./tests/run.sh` and `./tests/validate.sh` pass.
- [ ] Re-verify reports all 3 deferred items as ✅ COMPLIANT.
