# Design: agent-assisted recipe configuration

## Problem restated

Close the loop from natural-language recipe-setup intent to a verified project
state, without requiring the user to already know topology, manifest layout,
recipe contracts, or sync semantics — while preserving existing config and
overrides and staying clear of sibling override-lock work (#63).

## Technical approach (outcomes-first)

Treat assisted configuration as a **four-phase agent workflow** backed by
existing harness primitives:

```text
NL intent
  → INSPECT  (repo + manifest + recipe schema + grounding signals)
  → RECOMMEND (reviewable proposal: keys, rationale, assumptions)
  → APPLY    (idempotent surgical write to [recipes.<id>.config]; optional add)
  → VERIFY   (sync + doctor/lock checks) → REPORT
```

The skill playbook is the primary UX. CLI/helpers exist only to make INSPECT /
APPLY **deterministic and testable** when skill prose alone cannot guarantee
idempotence or preservation.

Do **not** over-build: prefer composing `recipe init` (read-only brief),
topology resolution (when the recipe uses it), `update_recipe_config`,
`ai-specs sync`, and `ai-specs doctor` before inventing new subsystems.

## Architecture decisions

| Decision | Choice | Alternatives | Rationale |
|---|---|---|---|
| Orchestration | Agent skill playbook (extend harness-recipes; cross-link lifecycle) | New catalog recipe; MCP tool | Literacy already ships always-on; chicken/egg if gated on a recipe |
| Grounding | Assemble recommendation from inspectable project signals | Pure LLM guess from catalog README | Acceptance requires grounded recommendation |
| Apply | Surgical `[recipes.<id>.config]` update preserving comments/unmentioned keys | Rewrite whole toml; interactive wizard only | `update_recipe_config` already matches idempotent + preserve |
| Sync policy | Assisted **APPLY** (after user approval) MAY/MUST run sync+verify; keep `recipe init` read-only and init.md propose-only unless amended | Always auto-sync on any init mention; never sync | Card requires sync; init-contract forbids silent sync — split phases |
| Overrides | Preserve all override files; report suspected stale/drift only | Force-refresh managed overrides | #63 owns lock provenance |
| Secrets | Redact / `${env:VAR}` only | Prompt for raw tokens in transcripts | Existing recipe-init + MCP conventions |
| Topology | Use existing `resolve_repo_topology` (or equivalent) as a grounding input when configuring topology-aware recipes | Hardcode Melón paths | Scenario is evidence, not product hardcoding |
| CLI expansion | Defer concrete flags until apply; allow thin non-interactive apply/inspect if tests need it | Ship large new subcommand surface now | Follow-up card standard: avoid over-prescribing before evidence |

## Phase contracts

### 1. INSPECT

Collect at least:

- Project initialized? (`ai-specs/ai-specs.toml`)
- Target recipe id(s) from NL (disambiguate via `recipe list` if unclear)
- Recipe enabled? Schema fields / required keys / enums / defaults
- Existing `[recipes.<id>.config]` values
- Relevant grounding: topology resolution when `repo_topology` (or similar) applies; MCP blocks if recipe needs MCP; CLI dep presence when declared
- Sync/version signals: `.ai-specs.lock` `[meta].cli_version` vs running CLI when available; doctor WARN count if cheap to run

Do not mutate during INSPECT.

### 2. RECOMMEND

Emit a reviewable recommendation including:

- Proposed config key/value set (schema-valid)
- Keys intentionally left unchanged (preserve)
- Assumptions (e.g. "treat as monorepo-submodules via auto detection")
- Risks / questions still needing the user
- Planned post-apply steps (sync, doctor)

Stop for user confirmation before APPLY unless the user already authorized apply in the same turn with an explicit approval verb **and** the recommendation was shown.

### 3. APPLY

- Write only recommended keys via surgical update (idempotent).
- Do not delete unspecified keys.
- Do not modify `ai-specs/recipes/**/overrides/**` contents in this change.
- May `recipe add` when the NL intent clearly enables a missing catalog recipe (still followed by configure + sync).
- Do not write secret literals.

### 4. VERIFY + REPORT

- Run `ai-specs sync` on the project path; treat non-zero as failure of the flow.
- Run verification via `ai-specs doctor` and/or sync success criteria already used by the project; surface failures plainly.
- Closing report MUST include: applied keys summary, preserved-untouched note, unresolved assumptions, drift/version gaps observed, sync/doctor outcome.

## Sync vs init-contract reconciliation

| Flow | Mutate manifest? | Invoke sync? |
|---|---|---|
| `ai-specs recipe init` / propose-only init.md | Agent may propose; init binary stays read-only | No (keep current init-contract unless separately amended) |
| Assisted configure APPLY (this capability) | Yes, after recommendation + approval | Yes — run and verify |

If specs need a formal amendment, prefer **adding** assisted-configure requirements rather than silently weakening init's propose-only posture.

## Boundary with #63 (override ownership)

This design may:

- Detect and **report** that an override differs from catalog / looks stale (using existing WARN patterns if present).
- Instruct the agent not to clobber overrides.

This design must not:

- Extend `.ai-specs.lock` with per-override hashes.
- Force-update user-modified overrides.
- Define per-artifact governance categories for hooks/templates (owned by #63).

## Validation approach

- Prefer focused unit tests for any new helper (inspect/recommend serialization, apply idempotence, preserve-unmentioned-keys).
- Skill content checks (commands named exist; playbook mentions sync + preserve + report).
- No requirement for live Melón checkout in CI — use fixtures that mimic submodule topology if topology grounding is tested.
- Full suite: `./tests/run.sh` and `./tests/validate.sh` before merge (apply phase).

## File-level sketch (indicative — not an apply checklist)

Exact paths are **authorization-flexible**; apply should confirm after RED tests:

- `bundled-skills/harness-recipes/SKILL.md` (+ maybe lifecycle cross-link) — playbook
- Optional: small helper under `lib/_internal/` or non-interactive branch in configure path
- Spec delta under this change → promote to `openspec/specs/` on apply
- Docs pointer (recipes catalog or troubleshooting)
- Tests under `tests/`

Avoid editing override-lock / materialize force-update paths reserved for #63.

## Risks

| Risk | Mitigation |
|---|---|
| Agents skip verify | Spec + skill checklist + tests that playbook text / helper enforces order |
| Skill-only too soft | Auth may approve thin CLI apply helper |
| Scope creep into #63 | Hard non-goal; report-only for overrides |
| init-contract contradiction | Explicit phase split in specs |
| Over-fitting to Alquimia | Fixture-general topology signals only |
