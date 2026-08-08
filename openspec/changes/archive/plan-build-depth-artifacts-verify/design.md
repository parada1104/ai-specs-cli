# Design: plan-build-depth-artifacts-verify

## 1. Intent

Tighten `plan-build-flow` depth artifact minima, make Standard explore decisions
inspectable, and add a two-point depth-staged verify gate so archive/merge cannot
silently skip verification for Standard/Full — without touching the adversarial
depth classifier and annotation contract delivered by #59.

## 2. Settled decisions

All questions previously listed for authorization are **closed**. D1–D7 were
confirmed as proposed; D8–D13 record the human decisions taken after #59 reached
final PASS. Nothing in this design is pending an answer.

| # | Decision | Settled value |
|---|---|---|
| D1 | Light minimum | `proposal.md` + `tasks.md` (short proposal allowed) |
| D2 | Standard minimum | `proposal.md` + `tasks.md` + ≥1 `specs/**/*.md` |
| D3 | Full minimum | Unchanged file set; `explore.md` stays first in the chain |
| D4 | Standard explore | Criterion table + mandatory skip line in `tasks.md` |
| D5 | Verify staging | advisory (Light) / enforcement (Standard) / required (Full) |
| D6 | Machine check | Extend `lib/_internal/premerge_guardian.py`; no second helper |
| D7 | Apply order | After #59 lands; rebase onto its merge SHA first (§11) |
| D8 | Standard evidence | Dedicated `verify-report.md` **required**. Auditable command, exit status, date, and commit SHA; overall verdict must not be failing. A section inside `tasks.md` or any other filename does **not** count |
| D9 | Full evidence | Strict global `PASS` **and** `ready_for_archive: true`, mapped to every success criterion of the change |
| D10 | Evidence block shape | One canonical labelled block (§5.2) that the skill documents and the guardian parses; no free-form table scraping |
| D11 | Explore enforcement | Skill-only at **both** Standard and Full. The guardian never blocks on a missing `explore.md` |
| D12 | Verify enforcement points | **Two**: before archive-tail (blocks archiving) and again in the pre-merge guardian (blocks merge). No bypass flag |
| D13 | Grandfathering | Applies only to plans already in flight when this ships. Historical archives are never rewritten; stale PRs are not retro-fixed — the owning agent adds the missing artifacts when it resumes that change |

## 3. Artifact minima

### 3.1 Planning chain and minima (skill §2 + spec *Depth artifact minima*)

| Tier | Chain | Minimum artifacts before build |
|---|---|---|
| **Light** | proposal → tasks | `proposal.md`, `tasks.md` |
| **Standard** | (explore if criteria) → proposal → spec → tasks | `proposal.md`, `tasks.md`, ≥1 `specs/**/*.md`; `explore.md` at plan time when §4.1 criteria fire |
| **Full** | explore → proposal → spec → design → tasks | `tasks.md`, `proposal.md` **or** `design.md`, ≥1 `specs/**/*.md` |

Guardian archive checks mirror the **minimum artifacts** column exactly. Per D11
the guardian never checks `explore.md` — not at Standard, not at Full.

### 3.2 Light proposal shape

Light stays cheap. The skill documents a short proposal:

```markdown
# Proposal: <slug>
## Why
…
## What
…
## Non-goals
…
```

No `design.md` for Light; ≤15 lines is the guidance ceiling.

## 4. Standard explore enforcement criteria

### 4.1 Require `explore.md` when any is true

1. **Multi-approach** — ≥2 plausible approaches with different trade-offs.
2. **Unknown surface** — the concrete files to touch cannot be named at plan start.
3. **Conflict** — docs/skills/prior notes disagree on the approach.
4. **Uncertainty signal** — user language such as "not sure", "figurar",
   "explore options", or equivalent.
5. **Retry** — a prior attempt on the same intent failed or was reverted.

### 4.2 Skip explore only when all are true

1. User (or card) names concrete file path(s) and expected edit/behavior.
2. Single obvious approach in a known module.
3. No conflict / uncertainty / retry signal from §4.1.

### 4.3 Skip record

When skipped, `tasks.md` MUST carry:

```text
Explore: skipped — <short reason matching §4.2>
```

When required, `explore.md` MUST exist before the authorization stop.

### 4.4 Enforcement boundary (D11)

Explore is a **plan-phase** obligation. Nothing machine-blocks PR creation,
archive-tail, or merge for a missing `explore.md`. These criteria decide whether
explore runs; which depth wins under an explicit/signal mismatch stays #59's
*Adversarial depth conflict detection* / *Conflict ask* contract, untouched here.

## 5. Staged verify gate

### 5.1 Modes

| Depth | Mode | Pre-archive-tail | Pre-merge guardian |
|---|---|---|---|
| Light | **advisory** | warn only | never blocks on evidence |
| Standard | **enforcement** | blocks archive without conforming report | blocks merge on the same rule |
| Full | **required** | blocks archive without strict PASS + `ready_for_archive: true` | blocks merge on the same rule |

### 5.2 Canonical evidence block (D8/D9/D10)

`verify-report.md` MUST contain one labelled block. The skill documents it; the
guardian parses it. Labels are case-insensitive and may carry list/table
punctuation:

```markdown
## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-07
- Commit: 604a441
- ready_for_archive: true
```

Accepted label synonyms: `Exit code` / `Exit status` for `Exit`; `SHA` /
`Revision` for `Commit`; `Status` / `Overall` for `Verdict`.

| Field | Standard | Full |
|---|---|---|
| `verify-report.md` present | required | required |
| Verdict | present and not `FAIL` / `BLOCKED` | exactly `PASS` |
| Command | required, non-empty | required, non-empty |
| Exit | required, must be `0` | required, must be `0` |
| Date | required, valid `YYYY-MM-DD` calendar date | required, valid `YYYY-MM-DD` calendar date |
| Commit | required, 7–40 hex | required, 7–40 hex |
| `ready_for_archive: true` | optional | **required** |
| Success-criteria mapping | not required | **required**: exactly one `- Criterion N: PASS` row for every top-level bullet under `## Success Criteria` in `proposal.md` when present, otherwise `design.md` |

The authoritative source is selected by file presence: `proposal.md` wins whenever
it exists, even when its criteria section is missing or empty; `design.md` is
used only when `proposal.md` is absent. Missing, empty, or duplicate `## Success
Criteria` headings in the authoritative source block Full rather than falling
back. The mapping ordinals are 1-based and contiguous. Duplicate, missing,
unknown, or non-PASS rows block Full. The canonical mapping heading is
`## Success-criteria mapping`; nested headings terminate the block.

### 5.3 Build sequence

1. Apply
2. Verify → write `verify-report.md` with the §5.2 block
3. PR artifact gate (tier minima per §3.1)
4. **Pre-archive verify gate** (blocks archive-tail for Standard/Full)
5. Archive-tail
6. **Pre-merge guardian** (archive location + minima + verify re-check)

Archive-without-verify is the failure mode steps 4 and 6 both close.

### 5.4 Guardian API

`lib/_internal/premerge_guardian.py` gains one evidence helper plus one
stage-aware entry point. Existing signatures and the existing CLI keep working.

```python
Stage = Literal["pre-archive", "pre-merge"]

def check_verify_evidence(change_dir: Path, tier: Tier) -> list[str]:
    """Blockers for the staged verify gate. Empty list for tier 'light'."""

def check_prearchive(repo_root, slug, *, tier=None) -> GuardianResult:
    """Inspect the ACTIVE openspec/changes/<slug>/ before archive-tail."""

def check_premerge(repo_root, slug, *, tier=None) -> GuardianResult:
    """Unchanged signature; now also appends check_verify_evidence blockers."""
```

- Tier resolution keeps using `DEPTH_RE` (`^\s*Depth:\s*(light|standard|full)\s*$`)
  against `tasks.md`, and keeps honouring an explicit `--tier`. The regex is
  #59-critical (annotation must never suffix the `Depth:` line) — do not relax it.
- `check_verify_evidence` returns `[]` for `light` unconditionally.
- CLI: `--stage {pre-merge,pre-archive}`, default `pre-merge`, so every existing
  invocation is byte-compatible. `--stage pre-archive` skips the
  active-folder-present and archive-missing blockers and evaluates minima +
  evidence against the active folder.
- **No** `--skip-verify-check` and no environment bypass (D12). A human override
  means amending this contract, not flipping a flag.

### 5.5 Full enforcement

Strict `PASS`, `ready_for_archive: true`, valid calendar dates, and the
success-criteria mapping are machine-checked. The guardian derives the
expected criterion ordinals from the non-empty `## Success Criteria` section in
`proposal.md` when present, otherwise `design.md`. A missing or empty section,
or duplicate exact headings, blocks Full; no fallback is permitted when
`proposal.md` exists. It requires exactly one strict-PASS mapping row per
top-level bullet. This keeps the contract deterministic at both enforcement
points.

## 6. Ownership boundary with #59

#60 owns: artifact minima, Standard/Full explore guidance, the staged verify
gate, the guardian and its tests, and the matching docs/spec fragments.

#60 does **not** own and MUST NOT alter: the signal/explicit-request classifier
computation, the conflict ask, the four annotation labels
(`Requested depth`, `Signal depth`, `Decided depth`, `Decision source`), the
standalone-`Depth:`-line contract, "higher decided tier completes its chain",
brief rule 1, or brief rule 7 (submodule topology).

## 7. File touch list (apply)

| Path | Change |
|---|---|
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | §2 tier table (chain + minimum columns), Light micro-plan row, explore criteria block, verify gate section, guardian blocker list |
| `catalog/recipes/plan-build-flow/README.md` | Depth/minima table, verify staging note, grandfathering paragraph, version example → `1.6.0` |
| `catalog/recipes/plan-build-flow/recipe.toml` | `version = "1.6.0"`; extend brief rules 3 and 5 only |
| `docs/recipes-catalog.md` | Enable example version → `1.6.0` |
| `CHANGELOG.md` | New `[Unreleased]` entry `1.5.0` → `1.6.0`; #59's entry stays untouched |
| `lib/_internal/premerge_guardian.py` | `check_verify_evidence`, `check_prearchive`, minima update, `--stage` |
| `tests/test_premerge_guardian.py` | Minima + staged verify cases |
| `tests/test_plan_build_flow_recipe.py` | Pinned brief rules 3/5 updated (rule 1 text kept verbatim from #59), version pins `1.5.0` → `1.6.0`, new skill-marker assertions |
| `openspec/specs/plan-build-flow/spec.md` | Promote this delta (§11 merge procedure) |

Out of scope: `hooks/plan-build-gate.sh`, recipe schema/materializer, classic
`openspec/config.yaml` decision matrix.

## 8. Version ownership

- `plan-build-flow` `1.5.0` belongs to **#59**. #60 MUST NOT re-claim it and MUST
  NOT edit #59's CHANGELOG entry.
- #60 changes SKILL/README/brief content, so it bumps `1.5.0` → **`1.6.0`** and
  updates every pinned surface in one commit: `recipe.toml:5`,
  `catalog/recipes/plan-build-flow/README.md:52`, `docs/recipes-catalog.md:192`,
  and the `1.5.0` assertions in `tests/test_plan_build_flow_recipe.py`
  (`test_version_and_catalog_documentation_use_current_contract`).
- The seven brief `workflow_rules` stay seven, in order, with rule 6 keeping
  exactly one `{config.artifact_store_default}` placeholder and rule 7 remaining
  the submodule-topology rule.

## 9. Testing plan (TDD)

1. RED: Light archive with only `tasks.md` → guardian blocks (needs `proposal.md`).
2. RED: Standard archive without `verify-report.md` → blocks; with a conforming
   report → OK.
3. RED: Standard with evidence only inside `tasks.md` → still blocks (D8).
4. RED: Standard report missing `Exit`/`Date`/`Commit`, or `Exit: 1` → blocks.
5. RED: Full without `PASS`, or without `ready_for_archive: true` → blocks; with
   both → OK.
6. RED: Light without any evidence → OK (advisory, never a blocker).
7. RED: Full without `explore.md` but otherwise conforming → OK (D11).
8. RED: `--stage pre-archive` on an active folder → minima + evidence enforced
   without the "still active" blocker; `--stage pre-merge` unchanged.
9. RED: guardian run for one slug ignores non-conforming sibling archives.
10. GREEN: implement guardian changes, then skill/README/brief text.
11. Recipe tests: skill markers for minima, explore criteria, and the three verify
    modes; brief rules 3/5 pins; version pins at `1.6.0`.
12. Preservation tests stay green untouched: `test_skill_describes_adversarial_depth_conflicts`,
    `test_skill_preserves_standalone_depth_annotation_contract`,
    `test_brief_describes_adversarial_depth_conflicts`,
    `test_depth_conflict_scenarios_cover_runtime_contract`.
13. `./tests/validate.sh` before this change's own verify report.

## 10. Migration / grandfathering (D13)

- The new minima and the verify gate apply to changes whose plan phase starts
  after this ships.
- Plans already in flight at ship time: add the missing `proposal.md` (Light) and
  the §5.2 evidence block before their PR/archive; no replan, no restart.
- `openspec/changes/archive/**` is never rewritten. The guardian evaluates only
  the slug under check, so older archives cannot fail a new merge.
- Stale PRs are not retro-fixed by this change. When their owning agent resumes
  them, they follow the in-flight rule above.
- README carries one short paragraph stating exactly this.

## 11. Manual merge / rebase strategy for shared surfaces

### 11.1 Baseline state (verified 2026-08-07)

- `change/plan-build-depth-artifacts-verify` is at `f248433`, based on
  `development` @ `12afc3f`: **1 ahead, 9 behind** current `development`
  (`604a441`).
- `f248433` touches only
  `openspec/changes/plan-build-depth-artifacts-verify/**` (5 files, 670
  insertions), so rebasing the planning commit itself cannot conflict.
- Between `12afc3f` and `604a441` the canonical `plan-build-flow` spec grew from
  14 to 20 requirements (six topology/central-artifact requirements). The
  requirements #60 modifies — *Change depth classifier*, *PR artifact gate*,
  *Pre-merge merge guardian* — are byte-identical across that range, so the
  delta's MODIFIED bases stay valid.
- #59's implementation lands `1.5.0`, rewrites the classifier requirement, adds
  four requirements, and rewrites brief rule 1. **That** is the real merge
  surface, and it exists only after #59 merges.

### 11.2 Order of operations

1. Wait for #59 to merge; record its merge SHA in `tasks.md` task 0.2.
2. `git fetch origin && git rebase <#59-merge-SHA>` on this branch. Expect zero
   conflicts (planning-only commit, disjoint paths).
3. Re-read the three shared files **from the rebased tree** before editing. Never
   edit from the pre-#59 copies captured during planning.
4. Apply the per-surface rules in §11.3, then run §11.4.

### 11.3 Per-surface rules

**`openspec/specs/plan-build-flow/spec.md`** — apply the delta by hand:

- Inside *Change depth classifier*, make exactly three edits: rewrite the
  Standard and Light chain bullets (the Full bullet is unchanged), insert the
  one-sentence pointer to *Depth artifact minima*, and rewrite the
  `THEN only tasks.md is required` line of *Scenario: Light depth for scoped fix*.
  Every other line — signal/explicit-request paragraph,
  standalone-`Depth:`-line paragraph, the other two scenarios — stays
  byte-identical, bold bullets included.
- Do not touch *Adversarial depth conflict detection*, *Conflict ask before
  planning chain*, *Depth resolution annotation*, *Higher decided tier completes
  its chain*, or the six topology requirements.
- Reconcile *PR artifact gate*: its *PR allowed with tier minimum files* scenario
  still says "tasks.md and spec deltas", which is stale under D2; it becomes
  `proposal.md`, `tasks.md`, spec deltas, and gains a Light-without-proposal
  scenario. This is a minima statement, so it is #60-owned; ownership does not
  move.
- Replace the *Pre-merge merge guardian* blocker list, append the three new
  requirements.

**`catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`** — edit in
place; never regenerate §2 from the planning-time copy:

- Post-#59 the adversarial block sits at roughly lines 44–86 (explicit-depth
  phrasings, conflict ask, the four annotation labels, deeper-tier rule). Preserve
  it verbatim.
- Edit the tier table (post-#59 ≈ lines 93–95): chain column and minimum column
  only. Update the "trivial one-line fix" row (≈ line 122) so Light means
  proposal + tasks. Update the PR-gate line (≈ line 158) and the guardian blocker
  list (≈ lines 198–199). Add the explore-criteria block and the verify-gate
  subsection.
- Line numbers are orientation only; anchor on headings and table text.

**`catalog/recipes/plan-build-flow/recipe.toml`**

- Bump `version` to `1.6.0`.
- Extend brief rule 3 (PR tier minimum) and rule 5 (archive before merge) only.
  Rule 1 (classifier, #59) and rule 7 (topology) are untouched; keep the array at
  seven entries and keep the phrase `tasks-only` present somewhere in the rules
  (`test_brief_mentions_depth_and_pr_gate` asserts it).

**`tests/test_plan_build_flow_recipe.py`**

- `test_recipe_brief_rules_preserve_store_and_add_topology_guidance` pins
  `rules[:5]` verbatim. Update only the rule 3 and rule 5 strings; copy rule 1's
  text from the **rebased** `recipe.toml`, never from the pre-#59 baseline.
- Update the `1.5.0` assertions to `1.6.0`.

**`lib/_internal/premerge_guardian.py`, `tests/test_premerge_guardian.py`** —
#59 provably did not touch these (`git status --porcelain -- lib/ hooks/ bin/`
was empty in its verify report), so they carry no merge risk.

**Conflict policy.** If git does report a conflict in SKILL.md, the spec, or
`recipe.toml`, resolve it manually with #59's side as the base and re-apply #60's
edits on top. Never use `-X ours` / `-X theirs`, and never accept a whole-hunk
replacement that drops #59 text.

### 11.4 Post-rebase preservation checks (must pass before apply is called done)

```sh
# #59 classifier + annotation contract intact in the canonical spec
grep -c 'Requested depth\|Signal depth\|Decided depth\|Decision source' \
  openspec/specs/plan-build-flow/spec.md          # expect > 0, all four present
grep -n '^### Requirement: Adversarial depth conflict detection' \
  openspec/specs/plan-build-flow/spec.md
grep -n '^### Requirement: Depth resolution annotation' \
  openspec/specs/plan-build-flow/spec.md

# brief still has exactly seven rules with topology last
python3 - <<'PY'
import tomllib, pathlib
r = tomllib.loads(pathlib.Path("catalog/recipes/plan-build-flow/recipe.toml").read_text())
rules = r["provides"]["brief"]["workflow_rules"]
assert len(rules) == 7, len(rules)
assert "topology" in rules[6].lower()
assert rules[5].count("{config.artifact_store_default}") == 1
PY

# classifier requirement differs from #59's landed text in EXACTLY 3 edits
python3 - <<'PY'
import pathlib, difflib
def block(p, name):
    t = pathlib.Path(p).read_text().splitlines()
    s = next(i for i, l in enumerate(t) if l.strip() == "### Requirement: " + name)
    e = next((i for i in range(s + 1, len(t)) if t[i].startswith("### Requirement")), len(t))
    return t[s:e]
NAME = "Change depth classifier"
landed = block("/tmp/plan-build-flow-spec-at-59.md", NAME)   # git show <#59-SHA>:… > this
now = block("openspec/specs/plan-build-flow/spec.md", NAME)
diff = [l for l in difflib.unified_diff(landed, now, lineterm="", n=0)
        if l[:1] in "+-" and not l.startswith(("---", "+++"))]
print("\n".join(diff))
assert sum(1 for l in diff if l.startswith("-")) == 3, "unexpected deletions in #59 text"
PY

# #59 regression tests still green, untouched
python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'
```

Plus: no `1.5.0` left in `catalog/recipes/plan-build-flow`,
`docs/recipes-catalog.md`, or `tests/test_plan_build_flow_recipe.py`, while
`docs/recipes-catalog.md`'s `worktree-flow` example keeps its own version.

## 12. Non-goals (design)

- Tier-aware `hooks/plan-build-gate.sh`.
- Recipe schema field or config knob for verify mode overrides.
- Any change to the adversarial classifier, conflict ask, or annotation contract
  (#59).
- Rewriting historical archives or existing PRs.
