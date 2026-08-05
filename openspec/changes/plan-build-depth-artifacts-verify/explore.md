# Explore: plan-build-depth-artifacts-verify

> Card #60 `lxv2WQ5g`. Sibling of #59 (adversarial classifier) — this explore
> does **not** redesign depth classification conflict handling.

## Problem restatement

Today `plan-build-flow` (SKILL §2 + `premerge_guardian.py`) enforces:

| Depth | Minimum before build / archive |
|---|---|
| Light | `tasks.md` |
| Standard | `tasks.md` + `specs/**/*.md` |
| Full | those + `proposal.md` **or** `design.md` |

Three gaps:

1. **Light is too thin** — a change can plan "blind" with only a task list and no
   written why/what. Context for reviewers and later sessions is missing.
2. **Standard explore is optional with no criteria** — the skill says "skip
   explore/proposal/design unless they reduce risk" but never says *when* risk
   warrants explore. Agents decide heuristically.
3. **No verify gate** — build sequence is apply → verify → PR gates →
   archive-tail, but only archive/tier-minima are machine-checked
   (`premerge_guardian`). Archive can land without recorded verification.
   Archive-without-verify is the asymmetry this card counters.

## Current surfaces (inspected)

| Surface | Role today |
|---|---|
| `catalog/recipes/plan-build-flow/skills/.../SKILL.md` §2 | Tier minima + chains |
| Same skill §7.2–7.4 | PR artifact gate; pre-merge archive gate; guardian invocation |
| `lib/_internal/premerge_guardian.py` | Hard-stop: active folder, missing archive, tier minima |
| `hooks/plan-build-gate.sh` | Pre-tool-use: block production edits without *any* active `tasks.md` (not tier-aware) |
| Canonical `openspec/specs/plan-build-flow/spec.md` | Classifier + PR/archive/guardian requirements; **no** verify gate |
| `openspec/config.yaml` verify guidance | Soft verify commands; not depth-staged enforcement |

## Proposed directions (to validate — not locked)

From card #60:

1. **Light** → raise minimum to `proposal.md` + `tasks.md`.
2. **Standard** → clearer minima (likely add `proposal.md`) + **forward explore
   enforcement** with explicit when-to-run criteria.
3. **Verify gate (staged)** as archive counterweight:
   - Light → **advisory**
   - Standard → **enforcement**
   - Full → **required**

## Option space

### A. Artifact minima

| Option | Light | Standard | Full | Notes |
|---|---|---|---|---|
| **A1 (card)** | proposal + tasks | tasks + specs + proposal; explore when criteria fire | unchanged (tasks + proposal\|design + specs; explore always in chain) | Matches card; Light gains context |
| **A2** | proposal + tasks | tasks + specs only; explore criteria only (no proposal mandate) | unchanged | Weaker Standard clarity |
| **A3** | tasks + one-line "Why" in tasks.md | unchanged + explore criteria | unchanged | Avoids new Light file; weaker review surface |

**Lean A1** — aligns with card; proposal is the cheapest durable "why" for Light.

### B. Explore enforcement (Standard)

| Option | Mechanism |
|---|---|
| **B1** | Explicit criterion table in skill + delta spec; missing `explore.md` when criteria match → plan-phase blocker (skill) + guardian optional check |
| **B2** | Always require `explore.md` for Standard (collapse toward Full) |
| **B3** | Soft guidance only (status quo with better wording) |

**Lean B1** — preserves Standard lightness when criteria are false; makes the
decision inspectable. Criteria candidates (refine in design):

- Require explore when **any** hold: ≥2 plausible approaches; unknown or
  cross-cutting file set; conflicting docs/skills; user uncertainty ("not sure
  how / where"); prior failed attempt on same area.
- Skip explore when **all** hold: user names concrete files + expected edit;
  single obvious approach; area already mapped in recent change folder / vault
  note for same slug intent.

Record decision in `tasks.md` when skipped: e.g.
`Explore: skipped — concrete files + single approach`.

### C. Verify gate staging

| Mode | Meaning (proposed) |
|---|---|
| **advisory** (Light) | Skill warns if no `verify-report.md` / no validate evidence before archive; does **not** block PR/merge |
| **enforcement** (Standard) | Block **archive-tail** (and thus merge via guardian) unless verify evidence exists: `verify-report.md` with non-failing verdict **or** recorded `./tests/validate.sh` PASS in report |
| **required** (Full) | Same as enforcement **and** `verify-report.md` MUST exist with explicit PASS (or ready_for_archive) mapped to success criteria |

| Option | Where enforced |
|---|---|
| **C1** | Extend `premerge_guardian.py` with depth-staged verify checks + skill text |
| **C2** | New `verify_guardian.py` invoked before archive-tail; premerge stays archive-only |
| **C3** | Skill-only (no machine gate) |

**Lean C1** — one helper agents already call; archive and verify stay coupled as
the card intends ("counterweight to archive gate"). C2 if guardian grows too
many concerns — decide in design if C1 tests stay small.

### D. Out of scope (this card)

- Adversarial depth conflict UX → **#59** only.
- Changing `plan-build-gate.sh` to become tier-aware (still "any tasks.md").
- New recipe schema fields / materializer branches (prefer skill + guardian +
  spec, matching delivery-contracts pattern unless a config knob is proven
  necessary).
- Redesigning classic SDD `openspec/config.yaml` decision_matrix.

## Dependency / sequencing

- Same recipe surface as #59. **Apply serializes after #59 merges** to avoid
  conflicting skill/spec edits. Planning may proceed in parallel.
- Base: `development` @ `12afc3f`.

## Risks surfaced

| Risk | Note |
|---|---|
| Light ceremony creep | proposal-for-Light may push agents to over-plan typos — mitigate with a short proposal template (Why / What / Non-goals ≤15 lines) |
| Explore criteria still fuzzy | Must be binary-ish checks an agent can answer yes/no |
| Verify evidence format drift | Projects vary (`validate.sh` vs `verify-report.md`); Standard should accept either; Full should prefer report |
| Guardian vs skill drift | Minima tables must stay single-sourced in skill + mirrored in guardian tests |
| Grandfathering | In-flight Light changes with only `tasks.md` need a transition note (new minima apply to plans started after this ships) |

## Ready for proposal?

Yes — directions A1 / B1 / C1 are the working hypothesis to validate in
proposal + design, with open questions listed for auth.
