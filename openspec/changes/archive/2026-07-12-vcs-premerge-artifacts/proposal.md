# Proposal: VCS Pre-merge Artifacts

## Intent

Make the VCS layer archive and record SDD/OpenSpec artifacts **before** merge, so the ceremony stays hidden, deterministic, and consistent across providers.

## Scope

### In Scope
- Add a pre-merge archive requirement to the canonical `vcs-pr-flow` contract.
- Mirror the rule into the provider merge-workflow skills.
- Update any recipe metadata/lock entries needed to keep sync coherent.

### Out of Scope
- New user-facing SDD commands or modes.
- Broader recipe redesign beyond the archive timing rule.
- Provider-specific merge behavior changes unrelated to archive timing.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `vcs-pr-flow`: require archive-pre-merge semantics and keep the runtime brief/skills aligned with the bound provider.

## Approach

Update the canonical spec first, then mirror the same rule into the provider merge-workflow skills and recipe metadata. Keep the change narrow and mechanical so all provider flows stay behaviorally equivalent.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `openspec/specs/vcs-pr-flow/spec.md` | Modified | Add/clarify the pre-merge archive rule. |
| `catalog/recipes/*/skills/*merge-workflow/SKILL.md` | Modified | Mirror the same archive timing contract per provider. |
| `catalog/recipes/*/recipe.toml` | Modified | Sync any recipe metadata needed for the contract. |
| `ai-specs/.ai-specs.lock` | Modified | Refresh lock if skill hashes change. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Skill/spec drift across providers | Medium | Update canonical spec first, then mirror exact wording. |
| Lockfile churn | Low | Keep changes minimal and update only required hashes. |
| Archive timing confusion | Medium | Document the pre-merge boundary explicitly and test sync output. |

## Rollback Plan

Revert the spec, skill, and metadata changes together. If sync breaks, restore the prior canonical `vcs-pr-flow` contract and lockfile state.

## Dependencies

- Existing `vcs-pr-flow` spec and provider merge-workflow skills.

## Success Criteria

- [ ] Canonical spec expresses the pre-merge archive rule.
- [ ] Provider merge-workflow skills match the canonical contract.
- [ ] Sync/validation remains coherent with no orphaned provider behavior.
