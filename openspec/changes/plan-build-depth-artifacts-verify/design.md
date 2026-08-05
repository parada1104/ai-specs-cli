# Design: plan-build-depth-artifacts-verify

## 1. Intent

Tighten plan-build-flow depth artifact minima and add a depth-staged verify gate
so archive/merge cannot silently skip verification for Standard/Full, without
touching the #59 adversarial classifier.

## 2. Decisions (proposed defaults pending auth)

| # | Decision | Default | Alt if auth rejects |
|---|---|---|---|
| D1 | Light minimum | `proposal.md` + `tasks.md` | Why section inside `tasks.md` only |
| D2 | Standard minimum | `proposal.md` + `tasks.md` + `specs/**/*.md` | Keep tasks+specs; only add explore rules |
| D3 | Full minimum | Unchanged files; explore remains mandatory in chain | — |
| D4 | Explore (Standard) | Criterion table + skip line in `tasks.md` | Always require `explore.md` |
| D5 | Verify staging | advisory / enforcement / required by depth | Skill-only advisory for all |
| D6 | Machine check | Extend `premerge_guardian.py` | Separate verify helper |
| D7 | Apply order | After #59 merge | Cherry-pick / rebase if #59 delayed |

## 3. Artifact minima (target tables)

### 3.1 Planning chain (skill §2)

| Tier | Chain | Minimum artifacts before build |
|---|---|---|
| **Light** | proposal → tasks | `proposal.md`, `tasks.md` |
| **Standard** | (explore if criteria) → proposal → spec → tasks | `proposal.md`, `tasks.md`, ≥1 `specs/**/*.md`; `explore.md` if criteria require it |
| **Full** | explore → proposal → spec → design → tasks | `tasks.md`, `proposal.md` **or** `design.md`, ≥1 `specs/**/*.md` (explore.md expected as chain artifact) |

Guardian archive checks mirror the **minimum artifacts** column (Full keeps
proposal|design OR; Full explore.md presence is skill-enforced, optional in
guardian v1 to avoid over-blocking — call out in auth if guardian should also
require `explore.md` for Full).

### 3.2 Light proposal shape

Keep Light cheap. Skill SHOULD allow a short proposal:

```markdown
# Proposal: <slug>
## Why
…
## What
…
## Non-goals
…
```

No design.md required for Light.

## 4. Standard explore enforcement criteria

### 4.1 Require `explore.md` when any true

1. **Multi-approach** — ≥2 plausible implementation approaches with different
   trade-offs.
2. **Unknown surface** — agent cannot name the concrete files to touch at plan
   start.
3. **Conflict** — docs/skills/prior notes disagree on the approach.
4. **Uncertainty signal** — user language like "not sure", "figurar", "explore
   options", or equivalent.
5. **Retry** — prior attempt on the same intent failed or was reverted.

### 4.2 Skip explore when all true

1. User (or card) names concrete file path(s) and expected edit/behavior.
2. Single obvious approach in a known module.
3. No conflict/uncertainty/retry signals from §4.1.

### 4.3 Skip record

When skipped, `tasks.md` MUST include a line:

```text
Explore: skipped — <short reason matching §4.2>
```

When required, `explore.md` MUST exist before authorization stop.

### 4.4 Boundary with #59

Criteria above decide **whether explore runs**, not which depth wins under
user/classifier conflict. Conflict UX stays in #59.

## 5. Staged verify gate

### 5.1 Modes

| Depth | Mode | Behavior |
|---|---|---|
| Light | **advisory** | Before archive, skill warns if no verify evidence; guardian does **not** block |
| Standard | **enforcement** | Guardian blocks merge if archive lacks verify evidence |
| Full | **required** | Guardian blocks unless `verify-report.md` exists with a passing / ready_for_archive verdict |

### 5.2 Evidence shapes

**Standard (enforcement)** — at least one of:

- `verify-report.md` present under the change folder (active, then archived),
  with verdict not FAIL/BLOCKED; **or**
- `verify-report.md` (or `verify-evidence.md`) that records `./tests/validate.sh`
  (or project verify command) exit 0 with date/SHA.

**Full (required)** — `verify-report.md` MUST exist and MUST state an overall
PASS (or `ready_for_archive` / equivalent success label used in this repo).

**Light (advisory)** — same evidence encouraged; absence → warning only.

### 5.3 Sequence (build)

1. Apply  
2. Verify (produce evidence per depth)  
3. PR artifact gate (updated minima)  
4. Archive-tail  
5. Pre-merge guardian (archive location + minima + verify mode)

Archive WITHOUT prior verify remains the failure mode Standard/Full block.

### 5.4 Guardian API sketch

Extend `check_premerge`:

- After tier minima checks, if resolved tier is `standard` and no verify
  evidence → blocker.
- If tier is `full` and missing/ failing verify-report → blocker.
- If tier is `light` → never add verify blockers (advisory is skill-only).

Optional CLI flag `--skip-verify-check` is **out of scope** (no bypass mode;
matches plan-build-gate philosophy). Human override = explicit auth to amend
guardian later if needed.

## 6. File touch list (apply)

| Path | Change |
|---|---|
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | §2 table, explore criteria, §7 verify gate, guardian blocker list |
| `catalog/recipes/plan-build-flow/README.md` | Depth table + verify note |
| `catalog/recipes/plan-build-flow/recipe.toml` | Brief workflow_rules if minima/verify mentioned |
| `lib/_internal/premerge_guardian.py` | Minima + verify evidence helpers |
| `tests/test_premerge_guardian.py` | New cases |
| `tests/test_plan_build_flow_recipe.py` | Skill/README assertions |
| `openspec/specs/plan-build-flow/spec.md` | Promote delta on apply/sync |

## 7. Testing plan (TDD)

1. RED: Light archive with only `tasks.md` → guardian fails (needs proposal).
2. RED: Standard archive without verify evidence → fails; with stub report → OK.
3. RED: Full without `verify-report.md` → fails; with PASS report → OK.
4. RED: Light without verify evidence → still OK (advisory).
5. GREEN: implement guardian + skill text.
6. Recipe tests: skill contains explore criteria markers + verify mode words.
7. Full `./tests/validate.sh` before verify phase of this change.

## 8. Migration / grandfathering

- New minima apply to changes whose plan phase starts after this ships.
- Document in README: in-flight Light plans that only have `tasks.md` should add
  a short `proposal.md` before PR/archive under the new skill.
- No automatic rewrite of historical archives.

## 9. Non-goals (design)

- Tier-aware `plan-build-gate.sh`.
- Schema field for verify mode overrides.
- Adversarial classifier (#59).

## 10. Open design questions for auth

1. Should Full guardian also require `explore.md` on disk, or skill-only?
2. Exact PASS string regex for verify-report (`PASS`, `ready_for_archive`, …)?
3. Is Standard evidence allowed as a section inside `tasks.md`, or must it be a
   dedicated file? (Design default: dedicated file.)
