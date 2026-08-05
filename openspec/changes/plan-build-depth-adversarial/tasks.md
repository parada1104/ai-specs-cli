# Tasks: plan-build-depth-adversarial

Depth: standard

## Tracker

- card_id: `LOb6pZLj`
- url: https://trello.com/c/LOb6pZLj

## Goal

Make the plan-build-flow depth classifier adversarial: detect explicit
user-depth vs signal conflicts, ask which to use, and annotate the resolution
in `tasks.md` — without touching sibling #60 artifact-minima / verify gates.

## Tasks

### 1. Skill: adversarial classifier loop

- [ ] Update `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` §2
      so classification always computes **signal**, detects **explicit user
      depth** when present, compares them, asks on mismatch, then records
      **decided**.
- [ ] Document illustrative EN/ES request phrases (at least: full SDD / flujo
      completo; standard / acotado con spec; solo tasks / tasks only / light)
      without claiming an exhaustive parser.
- [ ] State that silent adoption of either side on conflict is forbidden.
- [ ] State that a deeper decided tier MUST complete that tier's planning chain
      before build.
- [ ] Keep existing tier tables, PR/archive gates, and non-classifier sections
      behaviorally intact except where brief cross-references need alignment.

**Acceptance:** SKILL.md describes detect → ask → annotate; a reviewer can
follow the #58 incident path and see the required stop/ask.

**Evidence:** diff of SKILL.md; checklist review against proposal D1–D3, D5.

### 2. Docs + recipe surface

- [ ] Update `catalog/recipes/plan-build-flow/README.md` Planning depth section
      to mention conflict detection, ask, and annotation.
- [ ] Bump `catalog/recipes/plan-build-flow/recipe.toml` version
      (`1.3.0` → `1.4.0` unless a parallel change already claims that bump —
      reconcile at apply).
- [ ] If the classify brief `workflow_rules` entry would otherwise omit conflict
      handling, extend it in one concise sentence (no new config fields).
- [ ] Do **not** add `depth_default` / `depth_override` (or similar) config
      schema in this change (proposal D4).

**Acceptance:** README + recipe version reflect adversarial policy; no new
depth-* config keys in `recipe.toml`.

**Evidence:** recipe.toml / README diff; `rg 'depth_default|depth_override'
catalog/recipes/plan-build-flow` empty.

### 3. Canonical spec promotion

- [ ] After authorization, promote
      `openspec/changes/plan-build-depth-adversarial/specs/plan-build-flow/spec.md`
      into `openspec/specs/plan-build-flow/spec.md` (merge MODIFIED/ADDED
      requirements; preserve unrelated requirements such as delivery contracts
      and gates).
- [ ] Ensure adversarial requirements do not rewrite #60-owned artifact-minimum
      or verify-gate language.

**Acceptance:** Canonical spec contains conflict detection, ask, and annotation
scenarios; PR/archive/delivery requirements remain coherent.

**Evidence:** diff of `openspec/specs/plan-build-flow/spec.md`.

### 4. Focused tests (+ optional eval)

- [ ] Extend `tests/test_plan_build_flow_recipe.py` (or adjacent focused module)
      so skill text assertions cover conflict / ask / annotation (and brief rule
      if updated).
- [ ] Optionally add
      `tests/evals/scenarios/plan-build-flow/ac_depth_conflict_ask_annotate/`
      with a prompt where user requests Full and signals look Standard; expect
      ask + annotation guidance (eval harness conventions as existing ACs).
- [ ] Keep tests from asserting #60 verify-gate or artifact-minima redesigns.

**Acceptance:** Focused recipe tests fail before skill/docs updates and pass
after; no new failures in unrelated plan-build ACs.

**Evidence:** `python3 -m unittest tests.test_plan_build_flow_recipe` (and eval
runner if scenario added).

### 5. Verify / sync hygiene

- [ ] Run focused plan-build recipe tests, then `./tests/run.sh` before commit
      of implementation.
- [ ] If recipe version/docs require generated brief refresh for this repo's
      dogfood project, follow `dogfood-verification-isolation` — do not mix
      dogfood sync churn into the product commit without isolation.
- [ ] Record RED/GREEN evidence in apply notes when implementing (tdd-flow).

**Acceptance:** Implementation commit leaves focused + full unit suite green;
dogfood sync (if any) is isolated per skill.

**Evidence:** test command output captured in apply/verify notes.

## Non-goals (do not implement)

- Artifact-tier minima changes or staged verify gates → card #60
  (`plan-build-depth-artifacts-verify`).
- Recipe config for depth defaults/overrides.
- Changes to `hooks/plan-build-gate.sh` or `premerge_guardian.py`.
- New Engram observation types or Trello automation beyond normal card sync.

## Review workload (approx.)

- Skill + README + brief: ~80–120 lines
- Spec promotion: moderate merge into existing classifier requirement
- Tests: small focused assertions (+ optional one eval scenario)
