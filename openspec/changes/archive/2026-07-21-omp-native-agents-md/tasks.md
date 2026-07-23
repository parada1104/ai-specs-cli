# Tasks: omp native `.omp/AGENTS.md` slot

## Planning depth

- **Classification**: spec + tasks (modifies an existing platform-contract spec,
  `omp-agent-target`; behavior-visible but small and mechanical).
- **Authorization**: approved by maintainer (session request 2026-07-21).

## Implementation (red-green-refactor)

- [x] RED: add `tests/test_sync_pipeline.py` case — with `omp` enabled, sync
      creates `.omp/AGENTS.md` as a symlink resolving to the root `AGENTS.md`.
- [x] RED: extend `tests/test_doctor.py` to assert
      `instructions_path == ".omp/AGENTS.md"` (platform dict + `platform_get`).
- [x] GREEN: set omp `instructions_path` to `.omp/AGENTS.md` in
      `lib/_internal/platform.sh` (update the accompanying comment).
- [x] GREEN: set omp `instructions_path` to `.omp/AGENTS.md` in
      `lib/_internal/doctor.py`.
- [x] Update `openspec/specs/omp-agent-target/spec.md` via the change delta.

## Validation

- [x] `./tests/validate.sh` passes (exit 0); full `pytest tests/` green
      (997 passed, 143 subtests).
