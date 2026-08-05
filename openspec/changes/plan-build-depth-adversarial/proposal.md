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

## Proposed decisions (for authorization)

| ID | Decision | Consequence |
|----|----------|-------------|
| **D1** | Explicit user depth beats silence: if the user names a tier (or clearly equivalent phrase) and the signal differs, treat as conflict and ask. | Stops silent under/over-planning from the #58 failure mode. |
| **D2** | Signal still runs always; user request does not skip classification — it is compared. | Keeps adversarial data (requested vs signal) even when the user wins. |
| **D3** | Annotation lives in `tasks.md` next to `Depth: …` (structured lines or a short block). | Searchable in-repo trail without new config schema. |
| **D4** | No recipe.toml depth config in this change. | Keeps #59 thin; thresholds can be tuned from annotations first. |
| **D5** | Keep PR/archive/verify gates untouched (owned by #60 / existing contracts). | Parallel planning with #60 stays non-overlapping. |

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
| `catalog/recipes/plan-build-flow/README.md` | Modified — document conflict/ask/annotation |
| `catalog/recipes/plan-build-flow/recipe.toml` | Likely version bump + optional brief rule tweak |
| `openspec/specs/plan-build-flow/spec.md` | Promoted from delta at apply/sync |
| `tests/test_plan_build_flow_recipe.py` (+ optional eval scenario) | Modified / added |

## Risks / open questions for human auth

1. How strict should phrase detection be? Prefer a small illustrative phrase set
   in the skill (EN/ES) vs. requiring exact tokens `full|standard|light` only.
2. If the user says "full SDD" but then lists exact files and a one-line edit,
   should the ask still fire (yes under D1) or may the agent recommend Light in
   the ask text?
3. Confirm D4: leave `depth_default` / `depth_override` out of this change.
4. When conflict is resolved toward a higher tier mid-plan, is re-running the
   missing planning chain mandatory before build? (Recommended: yes.)

## Ready for authorization

Planning artifacts for Standard depth are complete. No production code in this
change yet — stop here for human review.
