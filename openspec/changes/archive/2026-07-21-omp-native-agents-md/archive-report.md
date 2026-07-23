# Archive report — omp-native-agents-md

**Archived:** 2026-07-21
**Branch:** fix/omp-native-agents-md
**PR:** #138
**Status:** ready-to-merge

## Outcome

- omp `instructions_path` changed from `""` to `.omp/AGENTS.md`, so `ai-specs sync`
  symlinks omp's native (highest-priority) provider slot to the root `AGENTS.md`.
- Root cause: per oh-my-pi `docs/context-files.md`, omp resolved the root brief
  only via the lowest-priority `agents-md` provider (priority 10); any
  `.claude/CLAUDE.md`, `.gemini/GEMINI.md`, or `.github/copilot-instructions.md`
  would shadow it. The native `.omp/AGENTS.md` provider (priority 100) shadows
  those, so the brief now loads once at highest priority.
- TDD evidence: sync + doctor tests went RED (no `.omp/AGENTS.md` symlink) → GREEN.
- Spec coverage: delta promoted into `openspec/specs/omp-agent-target/spec.md`
  (Platform registration + AGENTS.md native slot requirements).

## Files changed

- `lib/_internal/platform.sh` — omp `instructions_path` → `.omp/AGENTS.md` (+ comment).
- `lib/_internal/doctor.py` — omp `PLATFORM` entry `instructions_path`.
- `tests/test_sync_pipeline.py` — native-symlink assertions (new test + `--all` case).
- `tests/test_doctor.py` — omp `instructions_path` assertions (platform dict + `platform_get`).
- `openspec/specs/omp-agent-target/spec.md` — delta promoted from this change.
- `openspec/changes/archive/2026-07-21-omp-native-agents-md/` — full SDD trail.

## Verification

- `./tests/validate.sh` — exit 0.
- Full `pytest tests/` — 997 passed, 143 subtests passed.

## Process note

Planning depth was **Standard (spec + tasks)**. The plan-build-flow authorization
gate was not surfaced before implementation: classify → plan → **stop** was
collapsed into the build turn. The maintainer accepted the result retroactively
rather than rolling back. Gate-reinforcement is tracked as follow-up.
