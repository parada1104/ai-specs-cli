# Sync Report: cross-repo-worktree-artifact-scope

## Status

**SYNCED.**

The Apply phase had already promoted the verified delta into the canonical
`openspec/specs/plan-build-flow/spec.md`; Sync verified that promotion and
performed a minimal reconciliation so the canonical spec exactly matches the
verified change delta for every requirement this change owns. No destructive
delta exists (`## REMOVED Requirements` absent), no sibling change collides with
the domain, and no canonical file other than `plan-build-flow/spec.md` was
touched. The change was **not** archived, and no commit was created.

## Sync decision

- Apply promoted the delta into the canonical spec (git diff shows the promotion
  as `openspec/specs/plan-build-flow/spec.md | +284 -28` before Sync).
- Sync reconciled two residual inconsistencies between the promoted canonical
  text and the verified delta:
  1. **Scenario wording alignment** — the canonical `Central artifact writes are
     narrowly allowed` requirement rendered the action line as
     `- WHEN the hook evaluates the event` in four scenarios; the verified delta
     specifies `- WHEN the hook evaluates the write`. Sync aligned the four
     canonical lines to the delta wording (delta is the record of the verified
     contract). Canonical is now internally consistent: all four scenarios in
     that requirement read `evaluates the write`.
  2. **Markdown separation** — the canonical `### Requirement: Coexistence with
     classic SDD` heading immediately followed the last line of
     `Cross-repository planning has no orchestration side effects` with no blank
     line; Sync restored the blank line to match the delta's block structure and
     the file's own formatting convention.
- Everything else was already aligned; no other canonical edit was necessary.

## Domains synced

| Domain | Canonical file | Action |
|---|---|---|
| `plan-build-flow` | `openspec/specs/plan-build-flow/spec.md` | Reconciliation (4 wording lines + 1 blank line); already promoted |

## Delta requirement reconciliation

| Requirement | Section in delta | Canonical match |
|---|---|---|
| Topology-aware planning artifact root | ADDED | exact (4 scenarios) |
| Robust submodule root discovery | ADDED | exact (3 scenarios) |
| Canonical path normalization and symlink boundaries | ADDED | exact (4 scenarios) |
| Centralized artifact convention | ADDED | exact (3 scenarios) |
| Central artifact writes are narrowly allowed | ADDED | **aligned by sync** (4 scenario lines: `evaluates the event` → `evaluates the write`) |
| Cross-repository planning has no orchestration side effects | ADDED | exact (2 scenarios; blank-line separation restored) |
| Pre-tool-use artifact gate hook | MODIFIED | exact (6 scenarios) |
| Coexistence with classic SDD | MODIFIED | exact (1 scenario) |

- **ADDED requirement names:** Topology-aware planning artifact root; Robust
  submodule root discovery; Canonical path normalization and symlink boundaries;
  Centralized artifact convention; Central artifact writes are narrowly allowed;
  Cross-repository planning has no orchestration side effects.
- **MODIFIED requirement names:** Pre-tool-use artifact gate hook; Coexistence
  with classic SDD.
- **REMOVED requirement names:** none (`## REMOVED Requirements` section absent).
- **RENAMED Requirements:** none (section absent — unsupported delta type not
  triggered).
- After sync, a block-by-block diff of the 8 delta requirements against the
  canonical spec reports **all bodies exactly equal**, modulo canonical-only
  sections that Sync must preserve (`## Acceptance Criteria (test map)` table and
  the 12 pre-existing plan-build requirements, including `Pre-merge merge
  guardian`).

## Active same-domain collisions

None. `openspec/changes/` in the dedicated worktree contains only
`archive/` and this change (`cross-repo-worktree-artifact-scope`); no other
active change touches `specs/plan-build-flow/spec.md`. A sync/archive ordering
decision was therefore not required.

## Destructive sync approvals / blockers

None. No `REMOVED` deltas, no large `MODIFIED` blocks requiring approval, no
legacy flat spec (`openspec/changes/<slug>/spec.md`) present — file-backed
domain specs exist under `specs/plan-build-flow/spec.md`.

## Recipe / source / derived consistency

- **Recipe manifest** — `catalog/recipes/plan-build-flow/recipe.toml` declares
  version `1.4.0`, one bundled skill, zero slash commands, and the
  `pre-tool-use` hook `hooks/plan-build-gate.sh` with
  `matcher = Edit|Write|MultiEdit|NotebookEdit`, `blocking = true`. No new
  configuration surface: the only config field remains the pre-existing
  `artifact_store_default` (enum `openspec|engram|both`).
- **Hook implementation** — `hooks/plan-build-gate.sh` contains the
  topology-aware resolver markers required by the delta: canonical
  normalization (`python3` parse step), `rev-parse --show-toplevel` first
  repository fact, `--git-common-dir` `/modules/` central-root probe with
  `--show-superproject-working-tree` as corroboration only, boundary-aware
  `is_under` helper, fail-open `exit 0` paths for malformed/unrelated events,
  and no `PLAN_BUILD_GATE_MODE` handling (mode is inert / no off switch).
  `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` passes.
- **Recipe docs** — README and SKILL describe the topology-derived central
  superproject tree, the `openspec/changes/<slug>/` canonical location, and the
  no-duplication/no-orchestration boundary; brief fragments contain no
  forbidden vocabulary (`SDD`, `OpenSpec`, `spec-driven`, `/plan`, `/build`) —
  the vocabulary guard in `test_brief_and_readme_vocabulary_clean` enforces the
  brief surface.
- **Version pins** — `1.4.0` is consistent at the four pinned locations:
  `recipe.toml`, catalog `README.md` enablement snippet,
  `docs/recipes-catalog.md` plan-build section, and the recipe test
  expectation (`test_plan_build_flow_recipe.py`); unrelated recipes remain at
  `1.3.0` untouched.
- **Derived outputs (not hand-edited)** — `ai-specs/recipes/plan-build-flow/`
  is gitignored (`ai-specs/.gitignore: recipes/**`). The materialized
  `README.md` and `hooks/plan-build-gate.sh` are byte-identical to their catalog
  sources; the materialized skill under the worktree's cache
  (`cache/projects/c8909dfd9f52-cross-repo-worktree-artifact-scope/.recipe/
  plan-build-flow/skills/plan-build-flow/SKILL.md`) is byte-identical to the
  catalog SKILL. No hand edits were made to derived output.
- **Absence of removed configuration concepts** — grep of all changed tracked
  files finds `[sdd]`, `artifact_root`, and decision-matrix references **only as
  negative MUST-NOT assertions inside the canonical spec text**; no recipe file
  introduces a `[sdd]` section, an `artifact_root` selector, a decision matrix,
  or a per-subrepository store. `openspec/config.yaml` is unchanged.

## Validation performed (this sync)

1. Block-by-block comparison script (Python): all 8 delta requirement bodies
   exactly match canonical after reconciliation; canonical-only sections
   preserved.
2. `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'`
   → `Ran 19 tests ... OK`.
3. `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'`
   → `Ran 26 tests ... OK`.
4. `git diff --check` → clean (no whitespace errors).
5. `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` → exit 0.
6. `git status`/`git diff --stat` → changed tracked scope remains the approved
   8 files; `openspec/specs/plan-build-flow/spec.md` shows `+285 -28` (Apply
   promotion `+284 -28` plus this sync's +1 net line: 4 wording substitutions
   and 1 blank line). Full tracked diff: `8 files +682 -108 = 790 changed
   lines` (789 from Apply/verify record + 1 blank line added by this sync).
7. Full-suite evidence is inherited from `verify-report.md`: parent-observed
   `./tests/run.sh` and `./tests/validate.sh` each `Ran 1266 tests ... OK`,
   exit 0 (343.11s / 350.09s).

## Structured status and action context

```yaml
schemaName: spec-driven
changeName: cross-repo-worktree-artifact-scope
artifactStore: both
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  changesDir: openspec/changes
changeRoot: openspec/changes/cross-repo-worktree-artifact-scope
artifactPaths:
  proposal:
    - openspec/changes/cross-repo-worktree-artifact-scope/proposal.md
  specs:
    - openspec/changes/cross-repo-worktree-artifact-scope/specs/plan-build-flow/spec.md
    - openspec/specs/plan-build-flow/spec.md
  design:
    - openspec/changes/cross-repo-worktree-artifact-scope/design.md
  tasks:
    - openspec/changes/cross-repo-worktree-artifact-scope/tasks.md
  applyProgress:
    - openspec/changes/cross-repo-worktree-artifact-scope/apply-progress.md
  verifyReport:
    - openspec/changes/cross-repo-worktree-artifact-scope/verify-report.md
  syncReport:
    - openspec/changes/cross-repo-worktree-artifact-scope/sync-report.md
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
  verifyReport: done
  syncReport: done
taskProgress:
  total: 10
  complete: 10
  remaining: 0
  unchecked: []
applyState: all_done
dependencies:
  apply: all_done
  verify: all_done
  sync: done
  archive: ready
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  allowedEditRoots:
    - /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  warnings:
    - Apply promoted the canonical spec with two residual wording/formatting
      divergences from the verified delta; Sync reconciled them (4 scenario
      lines + 1 blank line). All other deltas were already exactly promoted.
```

## Notes and non-blocking observations

- The verify record's `1266 tests` (parent 600-second runs) and apply record's
  `1283 tests` are preserved as distinct historical observations; both are
  green and neither blocks sync.
- Existing non-blocking materializer notices (session-context/worktree-flow tag
  overlap, intentionally un-refreshed customized worktree-flow override) were
  re-confirmed unrelated to this change.
- `rules.sync` is not present in `openspec/config.yaml`; no additional sync
  rules applied.

## Next recommended phase

**sdd-archive** — sync is clean and complete. The change may proceed to the
archive phase (archive verification then move of
`openspec/changes/cross-repo-worktree-artifact-scope/` to
`openspec/changes/archive/`). Archive was deliberately **not** performed by
this sync phase.
