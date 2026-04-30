---
name: openspec-sdd-decision
description: >
  Classify a change before creating SDD artifacts by inspecting existing specs,
  analyzing the diff or change description, and selecting a ceremony level from
  the adaptive contract defined in openspec/config.yaml.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Classifying a change before creating SDD artifacts"
    - "Determining ceremony level for a change"
    - "Deciding how much SDD formality a change requires"
---

# openspec-sdd-decision

This skill guides the agent in classifying a change according to the adaptive
SDD contract before any artifacts are created. The classification determines
which SDD phases and artifacts are required.

## Ceremony Levels

The system defines exactly four ceremony levels:

- `trivial`: typographical errors, copy edits, minor CSS adjustments, internal
  renames without behavior change, or cleanup that does not alter observable
  system behavior.
- `local_fix`: localized bug fixes that restore intended behavior without
  changing system intent, user-facing contracts, or domain rules.
- `behavior_change`: changes that produce observable differences for users or
  downstream consumers, including validation rules, permission checks, state
  transitions, API response shapes, billing calculations, notifications, or small
  domain rules.
- `domain_change`: changes that introduce new capabilities, modify significant
  business rules, span multiple modules, require data model migrations, affect
  security/auth/payment flows, or constitute architectural decisions.

## Mandatory Steps

1. **Read `openspec/config.yaml`**
   - Check for `sdd.decision_matrix`.
   - If it is missing or `sdd.mode` is not `adaptive`, emit a warning and fall
     back to `domain_change` (formal mode).

2. **Inspect existing specs**
   - List files under `openspec/specs/`.
   - Identify any specs that may be related to the proposed change based on
     domain, module, or keyword overlap.
   - Read the most relevant spec files to understand the current contract.

3. **Analyze the change**
   - Review the user request, tracker card, or diff description.
   - Determine which code modules and test modules are likely to be touched.

4. **Select the ceremony level**
   - Apply the criteria above.
   - Prefer the higher level when the change sits on a boundary.

5. **Report structured classification**

   ```yaml
   classification: <trivial | local_fix | behavior_change | domain_change>
   reasoning: >
     <concise paragraph justifying the level>
   specs_touched:
     - <path to spec consulted>
   code_touched:
     - <path to code file expected to change>
   tests_touched:
     - <path to test file expected to change>
   ```

6. **Confirm before proceeding**
   - Do NOT create artifacts or enter `openspec-new-change` until the user
     confirms or corrects the classification.
   - If the classification is below the `sdd.threshold` declared in the active
     recipe, emit a warning describing the mismatch and ask for direction.
