# Proposal: plan-build-depth-adversarial

## Tracker

- card_id: `LOb6pZLj`
- full_id: `6a6f9c527b87ecb7c9fbce92`
- url: https://trello.com/c/LOb6pZLj
- title: [Follow-up] plan-build-flow: clasificador de depth adversarial (conflicto pedido vs señal + pregunta + anotación)

## Intent

The `plan-build-flow` depth classifier currently picks a tier from size/scope
signals alone. When a user already stated a depth (for example "flujo completo
SDD" / full), the skill can still record a different tier and proceed. The
mismatch only surfaces later, after planning has started or finished, and the
human must force a correction by hand.

This change makes the classifier adversarial: detect an explicit user depth
request vs the signal-derived tier, ask which to use when they differ, and
annotate requested / signal / decided so thresholds can be tuned later.

## Goal

1. **Detect conflict** when an explicit user-stated depth differs from the
   classifier's size/scope signal.
2. **Ask** which depth to use before writing the planning chain for the decided
   tier (do not silently pick one).
3. **Annotate** requested vs signal vs decided in `tasks.md` whenever a conflict
   occurred (and when the user stated a depth that matched, optionally record
   that the request confirmed the signal).

## Problem (incident)

During `plan-build-delivery-contracts` (#58), the user asked for full SDD while
the classifier recorded `standard` because the change looked bounded. The
disagreement was noticed post-hoc; the full chain ran only after a manual
override, and `tasks.md` stayed inconsistent until corrected. Follow-up was
explicitly deferred to this card (#59).

## Scope

### In scope

1. **Skill classifier policy** — extend `plan-build-flow` SKILL §2 so agents:
   - extract an explicit user depth request when present (natural language;
     English and Spanish examples from the incident);
   - compute a signal tier from existing size/scope heuristics;
   - treat mismatch as a conflict, not a silent override either way.
2. **Conflict ask** — when conflict exists and the user has not already answered
   which tier wins in the same turn, stop and ask before writing tier artifacts.
3. **Annotation contract** — require a durable record in `tasks.md` of
   requested, signal, and decided values (plus who decided: user vs matching
   signal) whenever conflict resolution ran.
4. **Canonical / recipe surfaces** — update the canonical plan-build-flow spec
   (via delta), recipe README classifier section, and any brief workflow rule
   text that describes classification without mentioning conflict handling.
5. **Regression coverage** — focused tests and/or eval scenario(s) that assert
   the skill (and brief, if updated) describe conflict detection, ask, and
   annotation.

### Out of scope / NON-goals

- **Sibling #60** (`plan-build-depth-artifacts-verify`): do not change tier
  minimum artifact sets, staged verify gates, or PR/archive guardian behavior.
- **Recipe config knobs** (`depth_default`, `depth_override`, auto-prefer-user,
  etc.): annotation is the tuning feed; project-level defaults are a later
  follow-up unless authorization expands scope.
- **`plan-build-gate.sh` / shell gate**: adversarial depth is skill policy, not
  a pre-tool-use filesystem check.
- **Orchestrator or Engram schema**: no new MCP tools or memory observation
  types required; file annotation in `tasks.md` is enough.
- **Rewriting unrelated classifier signal tables** beyond what conflict
  detection needs.
- **Forcing Full ceremony for this change itself** beyond Standard tier minima.

## Authorization

**Status: AUTHORIZED** by the human maintainer for planning completion and a
later apply. The decisions below are no longer proposals — they are settled
inputs for the apply phase. Every open question from the original draft is
resolved under "Resolved questions".

## Authorized decisions

| ID | Decision | Consequence |
|----|----------|-------------|
| **D1** | Explicit user depth beats silence: if the user names a tier (or clearly equivalent phrase) and the signal differs, treat as conflict and ask. | Stops silent under/over-planning from the #58 failure mode. |
| **D2** | Signal still runs always; user request does not skip classification — it is compared. | Keeps adversarial data (requested vs signal) even when the user wins. |
| **D3** | Annotation lives in `tasks.md` next to `Depth: …` (structured lines or a short block). | Searchable in-repo trail without new config schema. |
| **D4** | No recipe.toml depth config in this change. | Keeps #59 thin; thresholds can be tuned from annotations first. |
| **D5** | Keep PR/archive/verify gates untouched (owned by #60 / existing contracts). | Parallel planning with #60 stays non-overlapping. |
| **D6** | Annotation uses fixed labels on their own lines: `Depth: <tier>` stays a standalone lowercase line, and `Requested depth:`, `Signal depth:`, `Decided depth:`, `Decision source:` are separate sibling lines. | `premerge_guardian.DEPTH_RE` (`(?im)^\s*Depth:\s*(light\|standard\|full)\s*$`) keeps matching; suffixing the `Depth:` line would silently break tier inference. |
| **D7** | **#59 owns** the `plan-build-flow` recipe bump `1.4.0` → `1.5.0` and every stale `1.4.0` reference that the bump invalidates; preserve development's seventh workflow rule for submodule topology in position 7. | No "reconcile at apply" ambiguity; apply is not green until source, docs, catalog, changelog, rule-count/order assertions, and version-pinned tests agree. |
| **D8** | Phrase detection is an illustrative EN/ES example set in the skill, explicitly **not** an exhaustive parser or a fixed token whitelist. | Agents keep judgment for paraphrases; tests assert the documented policy, not a grammar. |
| **D9** | #60 (`plan-build-depth-artifacts-verify`) stays serialized **after** #59 and is not redesigned here. | Single writer per surface; #59 lands the classifier text, #60 then layers minima/verify gates on top. |

## Approach (high level)

1. Spec the conflict / ask / annotation requirements as a delta on
   `Change depth classifier`.
2. Teach the bundled skill the compare → ask → annotate loop in plain language
   (no slash commands).
3. Mirror the policy in README (and brief rule if the current classify rule
   would otherwise contradict).
4. Lock behavior with focused skill/brief assertions and an eval prompt that
   presents an explicit-depth vs signal mismatch.

## Affected areas (expected at apply)

| Area | Impact |
|------|--------|
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Modified — adversarial classifier section |
| `catalog/recipes/plan-build-flow/README.md` | Modified — conflict/ask/annotation policy **and** `version = "1.4.0"` → `"1.5.0"` (line ~42) |
| `catalog/recipes/plan-build-flow/recipe.toml` | Modified — `version` `1.4.0` → `1.5.0` (line ~5) and the classify `workflow_rules` entry (line ~29) which currently omits conflict handling; preserve the development topology rule as rule 7 |
| `docs/recipes-catalog.md` | Modified — plan-build-flow enable example `version = "1.4.0"` → `"1.5.0"` (line ~191) |
| `CHANGELOG.md` | Modified — `[Unreleased] / Changed` entry for `plan-build-flow` `1.4.0` → `1.5.0`; the historical topology release entry is **not** rewritten |
| `openspec/specs/plan-build-flow/spec.md` | Promoted from delta at apply/sync |
| `tests/test_plan_build_flow_recipe.py` | Modified — conflict/ask/annotation assertions, seven-rule count/order assertions, and hard-pinned `1.4.0` assertions updated to `1.5.0` |
| `tests/evals/scenarios/plan-build-flow/ac_depth_conflict_ask_annotate/` | Optional — added eval scenario |
| `openspec/changes/plan-build-depth-adversarial/{proposal.md,tasks.md}` | Modified — version ownership and #60 handoff retargeted |

## Resolved questions

| # | Question | Resolution |
|---|----------|------------|
| **R1** | How strict should phrase detection be? | Illustrative EN/ES phrase set in the skill (D8). No exhaustive parser, no `full\|standard\|light` token whitelist. |
| **R2** | User says "full SDD" but describes a one-line edit — does the ask still fire? | **Yes.** D1 holds: mismatch is a conflict regardless of direction. The ask MAY recommend the signal tier in its text, but the user (or a same-turn resolution) decides. |
| **R3** | Confirm D4 — leave `depth_default` / `depth_override` out? | **Confirmed.** No depth-* config schema in this change. Annotations are the tuning feed. |
| **R4** | If resolution picks a deeper tier mid-plan, must the missing planning chain run before build? | **Yes, mandatory.** The decided tier's full planning chain MUST be complete before build authorization counts as satisfied. |

## Conflict boundary with #60

#59 and #60 both touch `plan-build-flow`, so the boundary is explicit:

- **#59 owns:** SKILL §2 classifier text (signal + explicit-request compare, ask,
  annotation), the classify brief rule, the recipe `1.5.0` bump from development's
  `1.4.0` baseline and its stale `1.4.0` references, and the `Change depth
  classifier` spec requirement plus the four adversarial requirements in this delta.
- **#60 owns:** tier minimum artifact sets, staged verify gates, and PR/archive
  guardian behavior. #59 MUST NOT edit those, and MUST NOT alter
  `lib/_internal/premerge_guardian.py`.
- **Shared file, serialized:** #60 starts only after #59 lands. If #60 also needs a
  recipe bump, it moves `1.5.0` → `1.6.0`; it does not re-claim `1.5.0`.

## Authorization status

Planning artifacts for Standard depth are complete and the human authorization
above is recorded. No production code has been written in this change. Apply may
proceed under the authorized decisions D1–D9; production edits remain gated on
the maintainer explicitly starting the apply phase.
