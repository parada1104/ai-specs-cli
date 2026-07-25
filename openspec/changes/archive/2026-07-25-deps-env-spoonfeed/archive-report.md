# Archive report — deps-env-spoonfeed

**Archived:** 2026-07-25  
**Branch:** `feat/deps-env-spoonfeed`  
**Worktree:** `.worktrees/deps-env-spoonfeed`  
**Status:** verified PASS, judgment-day APPROVED, archived  
**Mode:** hybrid (openspec filesystem + Engram archive-report)

## Outcome

Harness MCP env setup and recipe CLI deps install are spoon-fed:

- Secrets go to `ai-specs/.env` (dotenv); committed `ai-specs/.env.example`;
  project-root `.envrc` gets a merge-safe managed `dotenv_if_exists` block;
  legacy `ai-specs/.envrc` migrates; `direnv allow` on root with soft-fail.
- TTY opt-in brew/apt install for known CLIs (`gh`, `glab`, `jq`, `direnv`,
  `git`); `npx`/`bb` guidance-only; doctor/non-TTY stay check-only.
- Doctor WARNs for missing direnv, missing managed `.envrc` markers, and
  missing/empty harness keys without leaking secrets.

## Specs synced

| Domain | Action | Details |
|--------|--------|---------|
| `harness-env-scaffold` | Created | 5 requirements (secrets `.env`, `.env.example`, root `.envrc` block, legacy migration, direnv allow) |
| `recipe-cli-deps` | Created | 4 requirements (non-destructive checks, TTY opt-in install, constrained argv, direnv offer on env path) |
| `project-doctor` | Updated | 3 ADDED requirements (direnv substrate, managed `.envrc`, harness env key diagnostics) |

No REMOVED/RENAMED deltas. No destructive merge.

## Verification & review gate

- Verify **PASS**: `verify-report.md` — 21/21 scenarios COMPLIANT, 12/12
  requirements evidenced, `./tests/validate.sh` 1086/1086 exit 0,
  evidence_revision `sha256:6838220fdbde00ac563113ebb4dae7990d649f5536bada39f5916638cc2c01fa`
- Judgment Day **APPROVED** (`judgment-ledger.md`): terminal_state `approved`,
  round 2 scoped re-judgment; JD-1..4 fixed; JD-5 suggestion closed by later
  coverage test (`test_dep_gate_offers_install_on_tty`)
- Native Engram `sdd/deps-env-spoonfeed/review/*` topics: **absent** — gate
  satisfied via on-disk judgment ledger + orchestrator archive instruction
  with JUDGMENT APPROVED

## Task completion gate

- Implementation P0–P5: complete (AUTHORIZED + IMPLEMENTED)
- P6 archive checkbox: marked `[x]` at archive time
- P6 commit / PR: intentionally left `[ ]` — parent owns commit and PR; not
  implementation tasks; verify-report treats them as close-out only

## Engram observation IDs (traceability)

| Topic / title | ID |
|---------------|-----|
| `sdd/deps-env-spoonfeed/verify-report` | `#1496` |
| deps-env-spoonfeed change authorized for planning | `#1493` |
| deps-env-spoonfeed planning complete — await auth | `#1494` |
| deps-env-spoonfeed implemented | `#1495` |
| JD round-1 fixes JD-1..4 | `#1497` |
| verify GAP tests closed | `#1498` |
| `sdd/deps-env-spoonfeed/{proposal,spec,design,tasks}` | **missing in Engram** — used openspec files on disk |
| `sdd/deps-env-spoonfeed/review/*` | **missing** — used `judgment-ledger.md` |

## Archive move

- Source: `openspec/changes/deps-env-spoonfeed/`
- Destination: `openspec/changes/archive/2026-07-25-deps-env-spoonfeed/`

### Archive contents

- proposal.md ✅
- design.md ✅
- specs/ ✅ (`harness-env-scaffold`, `recipe-cli-deps`, `project-doctor`)
- tasks.md ✅ (implementation complete; commit/PR parent-owned)
- verify-report.md ✅
- judgment-ledger.md ✅
- archive-report.md ✅ (this file)

## Source of truth updated

- `openspec/specs/harness-env-scaffold/spec.md` (new)
- `openspec/specs/recipe-cli-deps/spec.md` (new)
- `openspec/specs/project-doctor/spec.md` (updated)

## Follow-ups (parent / not this archive step)

1. Commit planning + implementation + archived change + main-spec merges on
   `feat/deps-env-spoonfeed`.
2. Open PR to `development`.

## SDD cycle

Planned, implemented, verified, judgment-approved, and archived.
Ready for parent commit/PR.
