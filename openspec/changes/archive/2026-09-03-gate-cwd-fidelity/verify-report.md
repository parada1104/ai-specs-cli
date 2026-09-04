# Verify report: gate-cwd-fidelity

## Verify evidence

- Verdict: PASS
- Command: ./tests/validate.sh
- Exit: 0
- Date: 2026-09-03
- Commit: e89f783
- ready_for_archive: true

- **Change**: `gate-cwd-fidelity`
- **Workspace**: `.worktrees/gate-cwd-fidelity`
- **Branch**: `fix/gate-cwd-fidelity`
- **HEAD verified**: `e89f783` (`fix(worktree-gate): preserve cwd fidelity`)
- **Artifact store**: openspec
- **RDD**: globally disabled by explicit user decision; no receipt or review approval invented
- **Gate unit suite**: `go test ./...` in `catalog/recipes/worktree-flow/gate/` PASS (`ok ai-specs.dev/worktree-gate`)
- **Regressions**: none. The only prior Python suite failure was an obsolete active manifest-contract check targeting a retired change path; that check was removed. Historical archive artifacts and anti-reintroduction guards remain intact.

## Success-criteria mapping

- Criterion 1: PASS — `recoverCwdWalk` / `effectiveBase` recover static `git -C <dir>` and `cd <dir> && ...` before resolving relative candidates; trusted event-cwd still blocks (`cwd.go`, `event.go`, `cwd_test.go`, `event_cwd_test.go`, `TestRunCwdFidelityMatrix`; `--explain` `git -C <wt> mv rel-a rel-b` with protected event cwd → `decision=allow`, `cwd_source=command`)
- Criterion 2: PASS — relative + unrecoverable cwd degrades (exit 0, `DegradeMessage`, no `$PWD` / `protected-branch` guess); recoverable protected-branch stderr names the actual command cwd (`createWorktree=false` omits `/worktree-new`; primary still uses the legacy create-worktree sentence)
