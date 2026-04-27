## Exploration: definir-contrato-ai-specs-toml

### Current State
`ai-specs/ai-specs.toml` already acts as the operational source of truth for the current CLI, but its contract is implicit and spread across template comments, README prose, and tolerant runtime readers.

Today the effective surface is:
- `[project]`
  - `name` is scaffolded and consumed as metadata/documentation.
  - `subrepos` is normalized by `lib/_internal/toml-read.py` and enforced by `lib/_internal/target-resolve.py`.
- `[agents]`
  - `enabled` is read by `lib/_internal/toml-read.py` and consumed by `lib/sync-agent.sh`.
- `[[deps]]`
  - `id` and `source` are effectively required by `lib/add-dep.sh` and `lib/_internal/vendor-skills.py`.
  - `path`, `scope`, `auto_invoke`, `license`, and `vendor_attribution` are optional and documented in the template.
- `[mcp.<name>]`
  - `command`, `args`, `env`, and `timeout` are documented; `lib/_internal/mcp-render.py` also tolerates `environment` and passes through `enabled`.

The current contract is NOT centrally validated. The code mostly relies on TOML parsing plus per-command assumptions:
- `toml-read.py` returns defaults for missing/malformed sections instead of failing.
- `target-resolve.py` validates only `project.subrepos` semantics.
- `vendor-skills.py` fails only when a dep is missing `id` or `source`.
- `mcp-render.py` accepts whatever is under `[mcp.*]` and translates opportunistically.

### Affected Areas
- `templates/ai-specs.toml.tmpl` — strongest current schema-like contract via comments and examples.
- `ai-specs/ai-specs.toml` — live example of the current manifest shape.
- `README.md` — canonical user-facing promises about ownership, root/subrepo behavior, deps, MCP, and generated artifacts.
- `lib/_internal/toml-read.py` — current normalization/default behavior for `project`, `agents`, `deps`, and `mcp`.
- `lib/_internal/target-resolve.py` — only place with real manifest validation today (`project.subrepos`).
- `lib/_internal/vendor-skills.py` — reveals the minimal runtime contract for `[[deps]]`.
- `lib/_internal/mcp-render.py` — reveals the effective MCP input contract and tolerated aliases.
- `lib/init.sh` — encodes the bootstrap/source-of-truth rules and “never overwrite manifest” policy.
- `lib/sync.sh` / `lib/sync-agent.sh` — operationally depend on the manifest without a prior canonical validator.
- `tests/test_sync_pipeline.py` — captures existing compatibility expectations for root/subrepo sync.
- `tests/test_target_resolve.py` — captures current validation expectations for `project.subrepos`.

### Approaches
1. **Document the existing implicit contract as the canonical V1 baseline** — define the minimum validable schema directly from current behavior, then validate against that.
   - Pros: Safest for compatibility; aligns with current code/tests; gives doctor a stable target; avoids inventing future-only fields too early.
   - Cons: Canonical contract will initially reflect some existing looseness/inconsistency; may require explicit “accepted but discouraged” fields.
   - Effort: Medium

2. **Design the future-state manifest now, then backfit compatibility rules** — include precedence/memory-oriented sections in the same contract pass.
   - Pros: More forward-looking; could reduce later documentation churn.
   - Cons: High risk of mixing phases; forces decisions that belong to precedence/doctor/memory cards; increases chance of breaking existing manifests or validating things the runtime cannot yet honor.
   - Effort: High

### Recommendation
Use **Approach 1**.

This change should define a **canonical V1 minimum contract** that is deliberately conservative:
- document the manifest sections that EXIST today and are already consumed (`project`, `agents`, `deps`, `mcp`);
- classify fields as **required**, **optional**, **defaulted**, or **tolerated legacy/alias**;
- explicitly separate **contract** from **implementation strictness**;
- preserve current manifests by treating missing optional sections as valid and by not requiring fields the runtime does not currently require.

The key gap to close is not feature breadth — it is **ownership + validation semantics**. Right now the manifest is source-of-truth in prose, but not yet a single canonical contract with machine-checkable rules.

### Risks
- Tightening validation too early could break manifests that currently work because readers default missing sections.
- Treating template comments as normative without reconciling them to runtime behavior could freeze contradictions into the contract.
- Including precedence or memory semantics now would couple this change to cards that are intentionally sequenced later.
- MCP is especially sensitive: `mcp-render.py` already tolerates `environment` and `enabled`, while template/docs emphasize `env`; the contract must decide whether these are canonical, aliases, or out-of-scope.

### Ready for Proposal
Yes — with a proposal scoped to:
- document the canonical V1 manifest surface already in use;
- define minimal validation rules and compatibility rules for existing manifests;
- identify tolerated aliases/defaults explicitly;
- defer precedence resolution, `[memory]`, and `doctor` command UX/implementation to their own changes.

Recommended out-of-scope for THIS change:
- implementing `ai-specs doctor`;
- introducing precedence merge rules beyond noting they are future work;
- adding `[memory]` or memory type schemas;
- changing sync/runtime behavior beyond what is required to align docs/schema terminology with existing behavior.
