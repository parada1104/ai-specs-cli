# Verify Report: plan-build-depth-adversarial (#59)

## Verify evidence

- Verdict: PASS
- Command: `sh ./tests/validate.sh` (focused `test_plan_build_flow_recipe.py` also re-run; see Verification notes)
- Exit: 0
- Date: 2026-08-07
- Commit: e4bdac4
- ready_for_archive: true

## Status: PASS (dogfood brief refresh N/A/non-blocking)

Final read-only verification of the amended proposal / tasks / spec against the
implementation in `.worktrees/plan-build-depth-adversarial`
(branch `change/plan-build-depth-adversarial`). No file under `catalog/`, `lib/`,
`hooks/`, `bin/`, `tests/`, `docs/`, or `openspec/specs/` was modified by this
phase; no commit, push, merge, or formatter was run. Only this report was written.

## Status consumed

```yaml
schemaName: spec-driven
changeName: plan-build-depth-adversarial
artifactStore: openspec
changeRoot: openspec/changes/plan-build-depth-adversarial
artifacts:
  proposal: done
  specs: done
  design: not required for Standard depth
  tasks: done (18/18)
  applyProgress: done
  verifyReport: done
  syncReport: missing
taskProgress:
  total: 18
  complete: 18
  remaining: 0
  unchecked: []
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-adversarial
  allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-adversarial]
  warnings: []
dependencies:
  verify: done
  sync: ready
  archive: ready-with-caveat
```

## Task completion

All 18 implementation/verification checkboxes across tasks 1–5 are complete.
The conditional dogfood brief refresh is explicitly **N/A for #59**: recipe
version/docs do not require a generated brief refresh for this product commit.
Any future dogfood sync is verification-only and non-blocking, and must follow
`dogfood-verification-isolation`.

**Assessment: PASS.** The optional eval task is backed by two live scenarios
and the dogfood refresh is not a change blocker.

The optional eval item is checked and backed by two live scenarios that this
phase re-ran independently.

## Spec coverage

Delta `openspec/changes/plan-build-depth-adversarial/specs/plan-build-flow/spec.md`
→ promoted into `openspec/specs/plan-build-flow/spec.md`. All five requirements
present and traced:

| Requirement | Canonical spec | Skill | Brief rule | Focused test | Live eval |
|---|---|---|---|---|---|
| Change depth classifier (MODIFIED) | yes | SKILL §2 | rule 1 | `test_brief_workflow_rules_*` | both |
| Adversarial depth conflict detection | yes | SKILL §2 | rule 1 | `test_skill_describes_adversarial_depth_conflicts` | `ac_depth_conflict_ask` |
| Conflict ask before planning chain | yes | SKILL §2 | rule 1 | same | `ac_depth_conflict_ask` |
| Depth resolution annotation | yes | SKILL §2 | rule 1 | `test_skill_preserves_standalone_depth_annotation_contract` | `ac_depth_conflict_same_turn` |
| Higher decided tier completes its chain | yes | SKILL §2 | — | — | `ac_depth_conflict_same_turn` (`required_path_globs` = proposal+design+spec+tasks) |

`git diff HEAD -- openspec/specs/plan-build-flow/spec.md` is a **single hunk**
(`@@ -40,27 +40,182 @@`) confined to the classifier requirement plus four
appended requirements. Every other requirement in the file is byte-identical.

## Version 1.5.0 integration

| Surface | Value | Location |
|---|---|---|
| `recipe.toml` | `1.5.0` | `catalog/recipes/plan-build-flow/recipe.toml:5` |
| recipe README enable example | `1.5.0` | `catalog/recipes/plan-build-flow/README.md:52` |
| catalog docs enable example | `1.5.0` | `docs/recipes-catalog.md:192` |
| CHANGELOG `[Unreleased] / Changed` | `1.4.0` → `1.5.0` entry added | `CHANGELOG.md:11` |
| version-pinned tests | `1.5.0` | `tests/test_plan_build_flow_recipe.py:290,298` |

Sweep results:

- `git grep -n '1\.4\.0' -- catalog/recipes/plan-build-flow docs/recipes-catalog.md tests/test_plan_build_flow_recipe.py`
  → **one** hit, `docs/recipes-catalog.md:247`, which is the **worktree-flow**
  enable example (verified in context, lines 244-254). Correctly untouched.
- Historical topology release entry (`1.3.0` → `1.4.0`) not rewritten — confirmed;
  the CHANGELOG diff is a pure 3-line insertion under `[Unreleased]`.
- `depth_default` / `depth_override`: **absent** from `catalog/recipes/plan-build-flow`
  (D4 honored; no new config keys in `recipe.toml`).

## Seven-rule preservation

`workflow_rules` contains exactly **7** entries in the original order. Only rule 1
changed text (classify → compute signal / compare requested / ask on conflicts /
annotate). Rules 2–7 are byte-identical to development.

- Rule 6 retains `{config.artifact_store_default}` exactly once.
- Rule 7 is development's submodule-topology rule, still in position 7.
- Asserted by `test_brief_workflow_rules_*`: `[fragment.key …] == [None] * 7`,
  first five texts pinned, `rules[5].count("{config.artifact_store_default}") == 1`,
  `"topology"` and `"superproject"` in `rules[6]`.

Note (non-blocking): rule 1 replaced "…and stop for authorization" with
"…before authorization". Semantics preserved; the stop is still normative in
SKILL §2 and in the canonical spec.

## Rebase / branch state

```
git rev-list --left-right --count development...change/plan-build-depth-adversarial
0	1
```

0 behind, 1 ahead of `development` (`604a441`, current). Rebase confirmed.
Committed content on the branch is planning-only (proposal/specs/tasks, 385
insertions); the implementation is present as uncommitted working-tree changes
plus 5 untracked files — expected, since committing was out of scope.

## Verification commands (all re-run in this phase)

| Command | Result |
|---|---|
| `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'` | **Ran 22 tests — OK** |
| `python3 -m unittest tests.evals.eval_harness_smoke` | **Ran 28 tests — OK** |
| `python3 -m unittest discover -s tests/evals -p 'eval_harness_smoke.py' -t .` | **Ran 28 tests — OK** |
| `python3 -m py_compile tests/evals/eval_plan_build_flow_live.py tests/evals/eval_harness_smoke.py` | **OK** |
| `sh ./tests/validate.sh` | **Ran 1319 tests in 382.6s — OK, exit 0** |
| `EVALS_LIVE=1 EVALS_RUNTIMES=claude EVALS_SCENARIOS=ac_depth_conflict_ask,ac_depth_conflict_same_turn EVALS_TIMEOUT_SEC=420 EVALS_MAX_TURNS=16 ./tests/evals/run-live.sh` | **Ran 9 tests in 197.1s — OK (skipped=6)**; `test_ac_depth_conflict_ask` → `ok`, `test_ac_depth_conflict_same_turn` → `ok` |

The `validate.sh` run above is a **fresh** execution taken *after* the eval
additions, closing the handoff gap (the previously reported 1319-pass run
predated `tests/evals/**` changes). Same 1319/OK result.

## Live eval scenarios — independent reproduction

Both scenarios were re-run live against `claude` in this phase and passed. Each
runs in a `tempfile.TemporaryDirectory()` sandbox with a materialized project,
wired runtime hooks, and a git baseline commit — no repository mutation.

**`ac_depth_conflict_ask`** (AC9, tier standard)
- `forbidden_path_globs = ["src/**","lib/**","catalog/**","openspec/**"]` — the
  agent wrote **no** planning artifacts, proving the stop-before-artifacts contract.
- `required_transcript_all = ["full","standard"]` — both tiers surfaced.
- `required_transcript_one_of = ["which","choose","elegir","elegí","qué"]` — an
  ask marker was present.
- Enforcement path verified in `eval_plan_build_flow_live.py:222-236`: the soft
  `break` only fires when `required_content` exists; this scenario has none, so
  both transcript assertions are hard-enforced.

**`ac_depth_conflict_same_turn`** (AC10, tier full, slug `signup-validation`)
- `required_content` on `tasks.md`: `Depth: full`, `Requested depth: full`,
  `Signal depth: standard`, `Decided depth: full`, `Decision source: user` — all
  five present, i.e. the exact five annotation lines.
- `required_path_globs`: `proposal.md`, `design.md`, `specs/**/*.md`, `tasks.md`
  — the complete Full chain.
- `forbidden_path_globs = ["src/**","lib/**","catalog/**"]` — no production edits.

## Scope / #60 non-invasion

Verified clean on every #60-owned surface:

- `git status --porcelain -- lib/ hooks/ bin/` → **empty**. `lib/_internal/premerge_guardian.py`
  untouched.
- Tier minimum artifact sets, staged verify gates, and PR/archive guardian
  requirements in `openspec/specs/plan-build-flow/spec.md` are byte-identical
  (single-hunk diff proof above).
- Changed-file set is exactly the 12 modified + 5 untracked paths named in
  `apply-progress.md`; no extras.
- `.worktrees/plan-build-depth-artifacts-verify` is **clean** at `f248433`
  (planning-only commit) and was not touched by this change.
- No new depth-* config keys, no new hook, no Engram/MCP schema change.

## Strict TDD compliance

`apply-progress.md` contains a `TDD Cycle Evidence` table. Cross-referenced:

- Test file `tests/test_plan_build_flow_recipe.py` exists and contains the three
  new tests plus the two updated version pins.
- Baseline 19 → RED 4 failures → GREEN 22 (post-rebase). GREEN state
  independently reproduced: **22 passed**.
- Runtime TRIANGULATE evidence (two live scenarios) independently reproduced.

Assertion-quality audit of the new/changed tests — **no** tautologies, ghost
loops, type-only assertions, or CSS/implementation-detail assertions:

- `test_skill_describes_adversarial_depth_conflicts` — 9 documented-policy
  phrases, each traceable to a spec requirement (D8/R1/R2 terms included).
  Keyword-presence style matches the existing convention in this module for a
  skill-text contract.
- `test_skill_preserves_standalone_depth_annotation_contract` — the strongest
  assertion in the set: `assertRegex(r"(?m)^Depth: full$")` plus a negative
  `assertNotRegex(r"(?m)^Depth: (?:light|standard|full) \(")` that directly
  defends the `premerge_guardian.DEPTH_RE` hazard behind D6. Fails on the
  plausible bug (suffixed `Depth:` line).
- `test_brief_describes_adversarial_depth_conflicts` — pins the brief rule text
  and re-asserts the single `{config.artifact_store_default}` placeholder.
- `test_depth_conflict_scenarios_cover_runtime_contract` — metadata contract for
  both scenarios; would fail if a scenario dropped a forbidden glob, an
  annotation label, or a Full-chain artifact requirement.
- Version pins `1.5.0` fail immediately on any partial bump.

## Review workload / PR boundary

`tasks.md` "Review workload (approx.)" forecast: skill + README + brief
~80–120 lines, moderate spec merge, small focused tests + optional one eval.
Actual: 559 insertions / 107 deletions across 12 modified + 5 untracked files,
of which 385 lines are planning artifacts and ~183 are the mechanical spec
promotion. Production-surface delta (SKILL/README/recipe.toml/CHANGELOG/docs) is
~72 lines. **Within forecast**; no chained PR, no `size:exception` needed, and
none was claimed. No scope creep beyond the assigned tasks was found.

## Findings (all non-blocking)

**N1 — stale dogfood brief (the one unchecked task).**
`AGENTS.md:72` in this worktree still carries the pre-#59 rule text:
`"…before writing production code; record depth in tasks.md and stop for authorization."`
The file is tracked and *unmodified* (stale-but-committed), so it lags the new
`recipe.toml` rule 1. This is precisely the conditional item task 5 defers to
`dogfood-verification-isolation`. Not spec-required, not asserted by any test,
and `validate.sh` is green with it. Recommend refreshing it in an isolated
dogfood sync, separate from the product commit.

**N2 — handoff evidence off by one.** The handoff reported "focused smoke 29
passed". Observed count is **28** via both `python3 -m unittest tests.evals.eval_harness_smoke`
and `discover -p 'eval_harness_smoke.py'`. Suite is green either way; only the
number in the narrative is wrong.

**N3 — eval prompt coaching narrows what `ac_depth_conflict_ask` proves.**
`prompt.txt` ends with *"preguntame cuál usar antes de crear cualquier archivo o
tocar producción"*, so a pass demonstrates instruction-following plus verified
no-write compliance, rather than the skill detecting the conflict and asking
unprompted. This is disclosed as a deliberate deviation in `apply-progress.md`.
The same-turn scenario's prompt likewise names the four artifacts to produce, so
`required_path_globs` is partly instruction-driven — but its five annotation
label/value assertions are **not** stated in the prompt and are therefore genuine
skill-derived behavioral evidence for the core #59 contract.

**N4 — cosmetic: spec formatting drift at promotion.** The promotion bolded
`**GIVEN**/**WHEN**/**THEN**/**AND**` in the classifier requirement's three
pre-existing scenarios and in all new scenarios. `openspec/specs/plan-build-flow/spec.md`
now has 14 bold vs 43 plain scenario bullets, and the source delta uses plain.
No validation impact.

**N5 — cosmetic: SKILL.md annotation block is unfenced.** The normative block in
`tasks.md` is inside a fenced code block; the reproduction in SKILL §2 is bare
lines, which collapse into one paragraph under Markdown rendering. Raw-text
consumers (and the `(?m)^Depth: full$` assertion) are unaffected.

**N6 — minor internal inconsistency in the TDD table.** The GREEN column says
"20 tests passing" while the Test Summary says 22; the summary's rebase note
explains the delta. Cosmetic.

## Blockers

**None.**

## #60 readiness

**#60 (`plan-build-depth-artifacts-verify`) may proceed to rebase and evaluation
now.** Justification:

- Its worktree is clean at `f248433` (planning-only) on base `604a441`, which is
  current `development` — the rebase is a no-op today and carries no conflict.
- #59 invaded none of #60's surfaces: artifact minima, staged verify gates,
  PR/archive guardian requirements, and `lib/_internal/premerge_guardian.py` are
  all provably untouched.

Two constraints carry forward from D7/D9:

1. **Serialization holds for apply, not for planning.** #59's implementation is
   still uncommitted in its own worktree, so a #60 rebase onto `development`
   today will *not* contain the 1.5.0 classifier text. #60 may plan and evaluate
   now, but must re-sync onto #59's landed state before it applies changes to the
   shared `catalog/recipes/plan-build-flow` surface.
2. **Version:** if #60 needs a recipe bump it takes `1.5.0` → `1.6.0`. It must not
   re-claim `1.5.0`, and it must preserve the seven workflow rules with the
   topology rule in position 7.

## Verification notes (archive-tail normalization, 2026-08-07)

This report was normalized at archive-tail on the review branch
(`change/plan-build-depth-adversarial`) to satisfy the Standard evidence shape
of the *Staged verify gate* requirement (final #60 contract): the canonical
`## Verify evidence` block above records auditable evidence — verify command,
exit status, calendar date, and commit SHA — with a non-failing verdict.

Fresh evidence re-run on the consolidated snapshot `e4bdac4` (the #60 merge
commit that carries #60's implementation on top of #59's landed `e2774c4`):

- `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'`
  → **Ran 25 tests — OK**, exit 0.
- `sh ./tests/validate.sh` (py_compile + bash -n + `./tests/run.sh`)
  → **Ran 1344 tests in 390.4s — OK**, exit 0.

Canonical sync: the #59 delta was promoted into
`openspec/specs/plan-build-flow/spec.md` at apply (`e2774c4`, task 3) and
re-verified at archive-tail (see `sync-report.md`): the four ADDED requirements
are present in canonical (byte-equal modulo the bold `**GIVEN**/**WHEN**/**THEN**`
formatting drift recorded as N4), and `Change depth classifier` carries #60's
superseding text per its design contract (3 deletions / 6 additions). No
archive-time sync change was required.

Pre-archive guardian:
`python3 lib/_internal/premerge_guardian.py plan-build-depth-adversarial
--root . --tier standard --stage pre-archive` → **OK (standard)**, exit 0.

Archive-tail executed: active folder moved to
`openspec/changes/archive/plan-build-depth-adversarial/` (undated
`archive/<slug>/` per SKILL §7.3 step 4 and the *Pre-merge merge guardian*
requirement — the guardian resolves exactly `openspec/changes/archive/<slug>/`).

Post-archive pre-merge guardian:
`python3 lib/_internal/premerge_guardian.py plan-build-depth-adversarial
--root . --tier standard --stage pre-merge` → **OK (standard)**.

The report's `Commit` field identifies the consolidated snapshot the evidence
was run against (`e4bdac4`); the archive-tail delivery commit is reported
separately in the archive run evidence. The branch was committed and pushed so
PR #184 reflects the archived state; no merge was performed (#62 untouched).
