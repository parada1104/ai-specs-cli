# Go 20 / U2a debt — AGENTS.md differential pins the verified gate binary

Branch `change/go-20-u2a-agents-gate` cut from `epic/go-single-binary` @ `1e89dae`. PR target: `epic/go-single-binary`, never `development`.
Scope: this is the pre-existing failure found by the full suite of U2b (`change/go-20-u2b-init`, log L10). It is not U2b, and U2b does not declare it settled.

## Specs

- S1 — Failing set, verbatim from the suite: `--- FAIL: TestAgentsRenderStepDifferential` with 7 subtests (`missing_writes`, `managed_stale_writes`, `untracked_preserves`, `untracked_adopts_with_adopt_brief`, `user_modified_preserves`, `user_modified_adopts_with_adopt_brief`, `unknown_vcs_recipe_warns_on_stderr`). The same 8 `--- FAIL` lines reproduce on the base `1e89dae`.
- S2 — Cause, measured: the Python leg prints `GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK: no verified worktree-gate binary; using the temporary Python classification authority` and `GO_LOCK_WRITE_BRIDGE_FALLBACK: ...`, because `runPythonAgentsStep` (`internal/sync/agentsrender_test.go:790`) does not pin `WORKTREE_GATE_BIN`. The Go leg does not print them, so stderr differs.
- S3 — Fix, test-side only: the Python leg of `runPythonAgentsStep` runs with `WORKTREE_GATE_BIN=materializeGateBinary(t)`, the same verified binary the materializer and `briefview_test.go` already use. No product code changes.
- S4 — Acceptance: `go test ./internal/sync/ -run TestAgentsRenderStepDifferential -count=1` exits 0 with the original comparisons unchanged (exit code, both streams, AGENTS.md and lock bytes). No assertion is weakened or deleted.
- S5 — Full suite: `./tests/validate.sh` runs once by the parent, after the commit, against this branch. The PR is not merged until it exits 0.

## Tasks

- **T1** — RED: the existing failing set is the RED (observed on `1e89dae` and on U2b, log L10). Covers: S1.
- **T2** — GREEN: pin `WORKTREE_GATE_BIN` in both Python commands of `runPythonAgentsStep`. Covers: S2, S3.
- **T3** — Gates and full suite. Covers: S4, S5.

## Log

- L1 — Decision: Cody eligió resolver el fallo base antes dentro del encargo técnico (the coordinator answered the `ask_user_choice` question with down+enter; the tool's generic "User selected" text does not identify Robert).
- L2 — Measured on `1e89dae` in a detached worktree (TMPDIR scratch): 8 `--- FAIL` lines, identical to U2b's failing set.
