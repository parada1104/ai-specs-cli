# Archive Report: agents-md-render-opt-out

**Archived**: 2026-06-09  
**Verify verdict**: PASS — 0 CRITICAL, 0 WARNING, 0 SUGGESTION  
**Test suite**: 565/565 GREEN, exit 0  
**Branch / worktree**: `feat/agents-md-render-opt-out` at `.worktrees/agents-md-render-opt-out/`  
**Apply commit**: `6faf783` — `feat(brief): opt out of managed AGENTS.md via [brief].render = false`

---

## Specs Merged into Main

### 1. `openspec/specs/recipe-manifest-contract/spec.md`

**Delta source**: `openspec/changes/agents-md-render-opt-out/specs/recipe-manifest-contract/spec.md`  
**Action**: ADDED (4 new requirements appended after existing requirements)

Requirements added:

| Requirement | Scenarios added |
|-------------|----------------|
| `[brief].render controls managed AGENTS.md generation` | render omitted defaults to enabled; render false disables managed output; render true behaves as today |
| `[brief].render validation` | Lowercase boolean accepted; Invalid boolean rejected; Capitalized True rejected at parse time |
| `render false propagates to subrepo sync targets` | Root render false applies to subrepo fan-out |
| `Doctor guidance for render disabled configurations` | Doctor ERROR when render false and AGENTS.md missing; Doctor WARN when recipe fragments unused; Doctor INFO when render disabled with AGENTS.md present |

No existing requirements were modified or removed.

---

### 2. `openspec/specs/runtime-brief-rendering/spec.md`

**Delta source**: `openspec/changes/agents-md-render-opt-out/specs/runtime-brief-rendering/spec.md`  
**Action**: MODIFIED (4 existing requirements updated) + ADDED (3 new requirements)

#### Modified requirements

| Requirement | Change summary |
|-------------|---------------|
| `init renders a non-empty AGENTS.md immediately` | Added `[brief].render = false` guard clause; added 2 new scenarios (placeholder on fresh init, preserve existing on init with render disabled); updated "Fresh init" scenario with render guard condition; added render guard to "Init render failure" and "Baseline brief" scenarios |
| `init→sync idempotency` | Updated requirement text to mention render=false; added render guard to "Second render after init" scenario; added new scenario "Sync with render disabled leaves AGENTS.md unchanged"; added render guard to "User-authored marker prevents re-render" scenario |
| `Subrepos receive enriched output` | Updated requirement to condition enriched output on render enabled; added 2 new scenarios (subrepo render skipped when root render disabled; subrepo missing AGENTS.md fails clearly); added render guard to existing "Subrepo AGENTS.md contains structured fields" scenario |
| `--preserve-if-runtime-brief escape hatch preserved` | Added clarifying note that callers MUST NOT invoke renderer when render=false; added render guard condition to both existing scenarios |
| `Idempotent output` | Updated requirement text to scope to render-enabled case; added render guard to "Second sync produces no diff" scenario; added new scenario "Two syncs with render disabled produce no diff" |

#### Added requirements

| Requirement | Scenarios added |
|-------------|----------------|
| `[brief].render manifest opt-out disables AGENTS.md writes` | Sync skips render when render is false; Default render true preserves current behavior; Render false does not block other sync artifacts |
| `Precedence of render flag over marker and per-section modes` | Render false skips even without marker; Render false with marker present is redundant but valid; Render true with marker still preserves file |
| `Observability when render is disabled` | Sync stdout names skip reason; Init stderr guides manual brief authoring |

No existing requirements were removed.

---

## Folder Move

**Source**: `openspec/changes/agents-md-render-opt-out/`  
**Destination**: `openspec/changes/archive/2026-06-09-agents-md-render-opt-out/`  
**Method**: Files written to destination (git mv to be staged in working tree before commit)

Archive contents:
- `exploration.md`
- `proposal.md`
- `design.md`
- `tasks.md` (with archive-time task checked off — see below)
- `apply-progress.md`
- `verify-report.md`
- `specs/recipe-manifest-contract/spec.md` (delta — preserved for audit trail)
- `specs/runtime-brief-rendering/spec.md` (delta — preserved for audit trail)
- `archive-report.md` (this file)

---

## Final Task State

**tasks.md**: 28/29 tasks `[x]`. 1 open task remains:

- `[x]` Merge delta specs into `openspec/specs/` at archive time — **DONE** (this archive)
- `[ ]` Trello #18 → In Progress during apply, Review after PR — **LEFT OPEN** (post-PR tracker hygiene, not an archive responsibility)

All B1–B7 implementation tasks (44 items) are `[x]`.

---

## Verify Verdict Reference

**Verdict**: PASS (2026-06-09, final)  
**Test count**: 565/565 GREEN  
**CRITICAL issues**: 0  
**WARNING issues**: 0  
**SUGGESTION issues**: 0  

All spec scenarios for both `recipe-manifest-contract` and `runtime-brief-rendering` have automated test coverage. The verify report superseded an earlier `PASS-WITH-WARNINGS` (2026-06-08) after W1, S1, and S2 follow-up items were resolved under strict TDD.

---

## SDD Cycle Status

**CLOSED** — planning → implementation → verification → archive complete.

The `[brief].render = false` opt-out is implemented, tested, documented, and spec-merged. The feature is ready for PR review (Trello #18 → Review).
