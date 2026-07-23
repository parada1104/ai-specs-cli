# Archive report — materialization-followup-guidance

**Archived:** 2026-07-23
**Branch:** `change/materialization-followup-guidance`
**Trello:** #49 https://trello.com/c/AfRD6P6O
**Status:** ready-to-merge (pre-merge archive gate)

## Outcome

- AGENTS harness literacy pointer no longer claims skills live under `ai-specs/skills/`.
- `harness-lifecycle` documents cache flatten (no `.new` sidecars).
- After leftover disk removal, sync/refresh prints `git rm -r --cached` guidance;
  doctor WARNs `tracked-bundled-leftover` — CLI never mutates the git index.
- Dogfood: untracked `skill-creator`/`skill-sync`; refreshed `[brief].context_sources`.

## Specs synced

| Domain | Action |
|--------|--------|
| `runtime-brief-rendering` | Updated — harness pointer wording |
| `project-doctor` | Added — tracked bundled leftover WARN |
| `harness-cli-literacy` | Created/updated — refresh-bundled cache flatten |

## Verification

- `./tests/validate.sh` — 1023 tests, EXIT 0
- Verify report: PASS

## Archive move

- Source: `openspec/changes/2026-07-23-materialization-followup-guidance/`
- Destination: `openspec/changes/archive/2026-07-23-materialization-followup-guidance/`
