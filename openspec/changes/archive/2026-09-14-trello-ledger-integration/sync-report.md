# Sync Report: trello-ledger-integration

- **status**: synced
- **change**: `trello-ledger-integration`
- **artifactStore**: openspec (+ Engram, artifactStore both)
- **domain**: `tracker-ledger`
- **merged via**: native `lib/openspec-deltas.ts::applyDeltaSpec` (run with `bun` against the installed helper; no hand-edit)
- **change kept active**: yes — NOT moved to archive

## Domains synced

- `tracker-ledger` → `openspec/specs/tracker-ledger/spec.md`

## Canonical file updated

- `openspec/specs/tracker-ledger/spec.md` (357 changed lines: 311 insertions, 46 deletions; 19 requirement blocks total after merge — 11 canonical requirements preserved/replaced + 8 ADDED appended)

## Delta operation (ADDED / MODIFIED / REMOVED)

**ADDED (8)** — appended at the end of the `## Requirements` section:
1. Machine write surface for open, link, close, and exempt
2. Explicit item opening
3. Idempotent, collision-safe writes under a bounded lock
4. Local, code, and Git evidence with remote deferred
5. Persistent tracker.none exemption
6. Witness-derived provider configuration lookup
7. Checkpoint ownership stays put
8. Write-path failure postures fail closed

**MODIFIED (5)** — replaced by exact name:
1. Human adjudication of conflicts and new primary after branch reuse
2. Modes always, ask, and warn with checkpoint-scoped opt-outs and warn-first adoption
3. Go-authoritative grader with Python bridge only
4. Unevaluable, outage, and unsupported behavior
5. Synthetic provider fixture and parity corpus

**REMOVED (0)** — none.

**RENAMED (0)** — none present; not required by the helper.

## Destructive delta analysis (guardrails)

- `defineDeltaDestructiveness`: `destructive: true`, `removedRequirements: []`.
- `largeModifiedRequirements` (≥ 40 lines): Human adjudication (44), Modes always/ask/warn (60), Go-authoritative grader (43), Unevaluable/outage (51).
- **Unrelated canonical requirements preserved:** `Durable binding witness`, `Binding-only activation`, `Doctor-only dormancy visibility`, `One primary item per work identity`, `Executable five-checkpoint lifecycle verdicts`, `Scope boundaries` (all byte-preserved).
- **Destructive approval:** the parent prompt explicitly approved "any documented destructive-merge check required by the sync contract". No REMOVED requirement existed; the large-MODIFIED blocks are the spec-correcting replacements (correcting the "grade opens the item" wording per L2 and the 3-of-4 evidence reconciliation per L3) and are documented as approved, not silently skipped.

## Collisions / guardrails

- **Active same-domain collisions:** none — `openspec/changes/` contains only `archive` and `trello-ledger-integration`; no other active change touches `openspec/specs/tracker-ledger/spec.md`.
- **Legacy flat spec:** none — the change uses domain specs (`openspec/changes/trello-ledger-integration/specs/tracker-ledger/spec.md`); no flat `spec.md`.
- **`## RENAMED Requirements`:** none; no correction required.

## Validation / checks performed

1. Read the verify-report: **verdict PASS**, `blockers: 0`, `critical_findings: 0`, `requirements: 13/13`, `scenarios: 39/39`, test/build exit codes 0 (native SDD status currently marks `sync: blocked` only because verify report resolution is pending in the status ledger; the report itself is clean passing with zero blockers).
2. `parseDeltaSpec` — 8 ADDED / 5 MODIFIED / 0 REMOVED / 0 RENAMED, no duplicate deltas.
3. `parseDeltaSpec` applied via `applyDeltaSpec`; merged canonical re-parsed with `parseRequirementBlocks` → **19 requirements, zero duplicates**.
4. Spot-verified all 5 MODIFIED blocks carry the delta's corrected wording (new scenarios present, superseded "grade opens the item" wording removed).
5. Confirmed `git diff` touches only the canonical spec path within the worktree; no production code, tests, config, or archive files modified.
6. `openspec/config.yaml` consulted — `rules.sync` no override; test/build tooling absent for sync validation, so spec-structure validation is the definitive check.

## Structured status and actionContext findings

- Native `gentle-ai.sdd-status` (v2) read fresh: `change trello-ledger-integration`, `artifactStore openspec`, `applyState all_done`, tasks 24/24, `verify: ready`, `sync: blocked` (blocked only because the verified sync gate requires verification resolution; the verify-report is clean PASS with zero blockers), `nextRecommended sdd-verify`, `blockedReasons: []`.
- `actionContext.mode`: repo-local; `workspaceRoot` and `allowedEditRoots`: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/trello-ledger-integration`; no `actionContext` warnings.
- Allowed edit surfaces honored: only `openspec/specs/tracker-ledger/spec.md` and `openspec/changes/trello-ledger-integration/sync-report.md` written.

## Next recommended phase

- `sdd-archive` — verification clean, sync completed, zero unchecked implementation tasks; archive readiness is now met from the sync side.

## Notes

- No commit, no push, no subagent launch, no archive move. Change folder remains active.
- The same sync report is persisted to Engram topic `sdd/trello-ledger-integration/sync-report`.
