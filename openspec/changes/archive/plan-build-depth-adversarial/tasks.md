# Tasks: plan-build-depth-adversarial

Depth: standard

Authorized: yes — human maintainer authorized #59 explicitly. Decisions D1–D9
and resolved questions R1–R4 are recorded in `proposal.md`; they are settled
inputs, not open items.

## Annotation format (normative)

When a depth conflict was detected, `tasks.md` records exactly this shape —
`Depth:` stays a **standalone lowercase line** and the four annotation labels are
**separate sibling lines**:

```
Depth: full

Requested depth: full
Signal depth: standard
Decided depth: full
Decision source: user
```

`Decided depth` and `Depth` MUST agree. `Decision source` is `user` when a human
chose (including same-turn resolution). Never suffix the `Depth:` line (no
`Depth: full (requested)`): `premerge_guardian.DEPTH_RE` is
`(?im)^\s*Depth:\s*(light|standard|full)\s*$` and any trailing text makes tier
inference silently fall back to `standard`.

Verified against `lib/_internal/premerge_guardian.py` during planning:
`infer_tier` on the block above returns `full` with exactly one `Depth:` match —
the four annotation labels do **not** collide with the regex, because each is
prefixed (`Requested `, `Signal `, `Decided `, `Decision source`). The suffixed
variant `Depth: full (requested)` returns the `standard` fallback, which is the
concrete hazard D6 prevents.

Note for whoever edits this file: `infer_tier` takes the **first** match, so the
illustrative block above MUST stay below this file's own `Depth: standard` line
on line 3. Do not hoist the example to the top.

## Tracker

- card_id: `LOb6pZLj`
- url: https://trello.com/c/LOb6pZLj

## Goal

Make the plan-build-flow depth classifier adversarial: detect explicit
user-depth vs signal conflicts, ask which to use, and annotate the resolution
in `tasks.md` — without touching sibling #60 artifact-minima / verify gates.

## Tasks

### 1. Skill: adversarial classifier loop

- [x] Update `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` §2
      so classification always computes **signal**, detects **explicit user
      depth** when present, compares them, asks on mismatch, then records
      **decided**.
- [x] Document illustrative EN/ES request phrases (at least: full SDD / flujo
      completo; standard / acotado con spec; solo tasks / tasks only / light),
      stating explicitly that this is an illustrative set and **not** an
      exhaustive parser or token whitelist (proposal D8 / R1).
- [x] State that silent adoption of either side on conflict is forbidden, and
      that the ask fires in **both** directions — including when the user asks
      for a deeper tier than the signal suggests (R2). The ask MAY recommend a
      tier; the user decides.
- [x] Define same-turn behavior unambiguously: if the same user turn that
      triggers the conflict already states which side wins (e.g. "use full even
      if it looks standard"), adopt that resolution **without** a second ask and
      annotate `Decision source: user`. Ask only when the turn leaves the
      conflict unresolved.
- [x] State that a deeper decided tier MUST complete that tier's **entire**
      planning chain before build authorization counts as satisfied (R4) — an
      upgrade to Full mid-plan requires the Full chain, not a retro-labelled
      Standard set.
- [x] Reproduce the normative annotation block from this file verbatim in the
      skill so agents emit consumer-compatible lines.
- [x] Keep existing tier tables, PR/archive gates, and non-classifier sections
      behaviorally intact except where brief cross-references need alignment.

**Acceptance:** SKILL.md describes detect → ask → annotate; a reviewer can
follow the #58 incident path and see the required stop/ask.

**Evidence:** diff of SKILL.md; checklist review against proposal D1–D3, D5.

### 2. Docs + recipe surface

- [x] Update `catalog/recipes/plan-build-flow/README.md` Planning depth section
      to mention conflict detection, ask, and annotation.
- [x] Bump `catalog/recipes/plan-build-flow/recipe.toml` version `1.4.0` →
      `1.5.0` (line ~5). **#59 owns this bump** from development's 1.4.0
      baseline (proposal D7); no "reconcile later" hedge. Preserve the
      development topology rule as workflow rule 7.
- [x] Sweep every stale `1.4.0` reference invalidated by the bump:
      - `catalog/recipes/plan-build-flow/README.md` (line ~42, enable example)
      - `docs/recipes-catalog.md` (line ~191, enable example)
      - `CHANGELOG.md` — add an `[Unreleased] / Changed` entry for
        `plan-build-flow` `1.4.0` → `1.5.0`; **do not** rewrite the historical
        topology release entry that records `1.3.0` → `1.4.0`.
      - version-pinned tests — see task 4.

**Acceptance:** README + recipe + catalog + changelog all advertise `1.5.0`;
the seven workflow rules retain development's topology rule in position 7; no
`1.4.0` reference remains that describes the current recipe; no new depth-*
config keys in `recipe.toml`.

**Evidence:** recipe.toml / README / catalog / CHANGELOG diff;
`rg 'depth_default|depth_override' catalog/recipes/plan-build-flow` empty;
`rg '1\.4\.0' catalog/recipes/plan-build-flow docs/recipes-catalog.md` empty.

### 3. Canonical spec promotion

- [x] After authorization, promote
      `openspec/changes/plan-build-depth-adversarial/specs/plan-build-flow/spec.md`
      into `openspec/specs/plan-build-flow/spec.md` (merge MODIFIED/ADDED
      requirements; preserve unrelated requirements such as delivery contracts
      and gates).
- [x] Ensure adversarial requirements do not rewrite #60-owned artifact-minimum
      or verify-gate language. Concretely: leave tier minimum artifact sets,
      staged verify gates, and PR/archive guardian requirements byte-identical,
      and do not touch `lib/_internal/premerge_guardian.py`.

**Acceptance:** Canonical spec contains conflict detection, ask, and annotation
scenarios; PR/archive/delivery requirements remain coherent.

**Evidence:** diff of `openspec/specs/plan-build-flow/spec.md`.

### 4. Focused tests (+ optional eval)

- [x] Extend `tests/test_plan_build_flow_recipe.py` (or adjacent focused module)
      so skill text assertions cover conflict detection, the ask, same-turn
      resolution, and the four annotation labels (and the brief classify rule).
- [x] Update the version-pinned assertions in
      `tests/test_plan_build_flow_recipe.py::test_version_and_catalog_documentation_use_current_contract`:
      line ~249 `assertEqual(_recipe_version(), "1.4.0")` → `"1.5.0"`, and line
      ~257 `assertIn("1.4.0", text)` → `"1.5.0"` (that assertion loops over both
      README and `docs/recipes-catalog.md`). These fail immediately on the
      development-baseline bump, so they are in-scope for #59, not incidental
      breakage.
- [x] Add deterministic live runtime coverage under
      `tests/evals/scenarios/plan-build-flow/` for unresolved conflict asks and
      same-turn Full resolution with exact annotation and full-chain artifacts.
- [x] Keep tests from asserting #60 verify-gate or artifact-minima redesigns.

**Acceptance:** Focused recipe tests fail before skill/docs updates and pass
after; no new failures in unrelated plan-build ACs.

**Evidence:** `python3 -m unittest tests.test_plan_build_flow_recipe` (and eval
runner if scenario added).

### 5. Verify / sync hygiene

- [x] Run the focused plan-build recipe tests, then the mandated
      `./tests/validate.sh` validation before final handoff.
- [x] Dogfood brief refresh is **N/A for #59**: recipe version/docs do not require
      generated brief refresh for this product commit; any future dogfood sync is
      verification-only and non-blocking (follow isolation if run).

**Acceptance:** Focused recipe tests and the full mandated validation are green;
dogfood sync (if any) is isolated per skill.

**Evidence:** `python3 -m unittest discover -s .worktrees/plan-build-depth-adversarial/tests -p
'test_plan_build_flow_recipe.py'` — 22 tests passed; `sh
.worktrees/plan-build-depth-adversarial/tests/validate.sh` — 1319 tests passed.

## Non-goals (do not implement)

- Artifact-tier minima changes or staged verify gates → card #60
  (`plan-build-depth-artifacts-verify`), which is **serialized after** #59. If
  #60 needs its own recipe bump it takes `1.5.0` → `1.6.0`; it does not re-claim
  `1.5.0`.
- Recipe config for depth defaults/overrides.
- Changes to `hooks/plan-build-gate.sh`, `premerge_guardian.py`, or any
  PR/archive guardian behavior.
- New Engram observation types or Trello automation beyond normal card sync.

## Review workload (approx.)

- Skill + README + brief: ~80–120 lines
- Spec promotion: moderate merge into existing classifier requirement
- Tests: small focused assertions (+ optional one eval scenario)
