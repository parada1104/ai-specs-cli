# /plan — Turn an intent into a reviewable plan

Operationalize the `plan-build-flow` skill for planning. Load that skill and
follow its phase mapping and degradation policies before doing anything else
below.

## Steps

1. **Load the skill.** Read the `plan-build-flow` skill and follow its phase
   mapping (Section 2), orchestrator degradation policy (Section 3), memory
   degradation policy (Section 4), artifact-store default policy (Section 5),
   and change-slug rules (Section 6).

2. **Capture intent and derive the slug.** Read the user's intent for this
   change. Derive a short kebab-case change-slug from it and confirm it with
   the user if it is ambiguous.

3. **Resolve the artifact store.** If an orchestrator preflight already
   resolved a store for this session, use it. Otherwise default to file
   artifacts on disk, per the skill's artifact-store policy.

4. **Run the planning phases.** Run the mapped phases end to end for this
   change — via an external orchestrator if one is present, or inline as one
   continuous conversation if none is present. Never silently skip a phase.

5. **Stop for review.** Produce the reviewable planning artifacts, present
   them to the human, and STOP. Ask the human to review and authorize before
   any building happens. Do not implement, edit production code, or run tests
   during `/plan`.

Never surface internal phase names to the user — speak only in terms of
"plan" and "build".
