# Go 20 / U2b — init: retire the escape hatch, native refresh step

Unit U2b of `odd/tasks/go-20-u2-launcher-flip.md`. Branch `change/go-20-u2b-init` cut from `epic/go-single-binary` @ `1e89dae`. PR target: `epic/go-single-binary`, never `development`.
Tracker: epic card https://trello.com/c/qwlHQ7Xa. U2b child card: not recovered (no Trello MCP tool exposed in this session; `gate_mode = warn` is advisory and never blocks).

## Specs

- S1 — Parent request, verbatim: "retirar AI_SPECS_INIT_TUI_PY y sustituir llamada Python de init por RefreshBundled Go, documentar contrato D26 fielmente".
- S2 — Decision D26, verbatim from the human: "ah si si, basta con sync". Meaning, fixed by that answer: `refresh-bundled` standalone stays a documented no-op of compatibility (`internal/cli/refresh_bundled.go`); the effective refresh runs inside `sync` (`internal/sync/sync.go`, `RefreshBundled`). No change to the standalone verb.
- S3 — Internal substitution: "No necesitas nueva decisión para la sustitución interna de init." `init`'s refresh step (`internal/cli/init.go:675-677`, `refreshBundledAtInit`) calls the native `sync.RefreshBundled(target, home, stdout, stderr)` instead of `python3 lib/_internal/refresh-bundled.py <target> <home> --init`.
- S4 — Observable contract of the step, measured on the oracle: `--init` is accepted and ignored (`lib/_internal/refresh-bundled.py:136-138`). The header line `▸ refresh-bundled --init` is kept. Exit codes pass through unchanged. `RefreshBundled` is already differential-tested against that script (`internal/sync/refreshbundled_test.go`).
- S5 — Retirement: the env var `AI_SPECS_INIT_TUI_PY` no longer hands the whole verb to `lib/init.sh` (`internal/cli/init.go:119-121`). With the variable set, `init` behaves exactly as without it. The Python tests that exercised the hatch are replaced by Go tests asserting the new behaviour, not deleted silently.
- S6 — Contract: `docs/go-migration-parity-contract.md` rows for `AI_SPECS_INIT_TUI_PY` (line 488), D26 (line 340) and the `refresh-bundled` rows (lines 44, 247) are rewritten to match S2 and S5. The text says what the code does, nothing more.
- S7 — Out of scope, not touched here: U2a debt (L11 seam-table mapping and L12 assertion change in the U2a log, reviewed separately, not declared settled by this unit); `init.go` line numbers outside the step; `doctor/bridge.go` (U2c); `lib/_internal` deletion (U2e).
- S8 — Permissions: commits and a PR to `epic/go-single-binary` with auto-merge under the epic contract. Never `development`. Full suite run once by the parent, not by a worker.

## Tasks

- **T1** — RED: with `AI_SPECS_INIT_TUI_PY` set, `init` does not execute `lib/init.sh` (S5). Go test in `internal/cli`, replacing `TestInitTUIEnvSeamKeepsLegacyDelegation` (`internal/cli/init_test.go:618`). Covers: S5.
- **T2** — RED: the init refresh step runs natively, no `python3` child (S3, S4). Go test that shadows `python3` on PATH, logs invocations, and asserts the step prints `▸ refresh-bundled --init` and the flatten lines, with an empty log. Covers: S3, S4.
- **T3** — GREEN: remove the hatch branch in `init.go:119-121`; replace the body of `refreshBundledAtInit` with `sync.RefreshBundled`. Covers: S3, S5.
- **T4** — Replace the Python hatch tests (`tests/test_init_native.py` lines ~402-434, `tests/test_init_tui.py` lines ~675-692) with the Go tests of T1, and record the mapping here. Covers: S5.
- **T5** — Contract rewrite (S6): D26, `AI_SPECS_INIT_TUI_PY`, `refresh-bundled` rows. Covers: S2, S6.
- **T6** — Gates: `go build ./...`, `gofmt -l`, `go vet ./internal/cli/ ./internal/sync/`, focused `go test`, touched Python suites. Full `./tests/validate.sh` once, by the parent, after the commits.

## Decisions

- D26 → resolved by the human: S2. Recorded here; not reopened by this unit.
- Hatch → retired (S5). Its only purpose was the legacy wizard, which is native since U4b-2.

## Log

- L1 — Parent request, verbatim (2026-10-22 session): "Continúa implementación autorizada de U2b según plan existente: retirar AI_SPECS_INIT_TUI_PY y sustituir llamada Python de init por RefreshBundled Go, documentar contrato D26 fielmente. No necesitas nueva decisión para la sustitución interna de init."
- L2 — D26 answer, verbatim: "ah si si, basta con sync" (recorded in S2 and in the Engram continuity note for this session).
- L4 — T1-T3 delegated to `gentle-ai-worker` (workspace this worktree, allowed surfaces `internal/cli/init.go`, `internal/cli/init_test.go`). RED observed for both new tests (hatch executed the legacy script; python3 spawned for the refresh step), GREEN after the change. Parent re-ran build, gofmt, vet and `go test ./internal/cli/ -run 'Init|Refresh'`: all exit 0.
- L5 — T4: `tests/test_init_native.py` differential `test_init_tui_py_env_seam_keeps_legacy_wizard_delegation` replaced by `test_init_tui_py_env_seam_is_retired_for_the_native_verb` (native run with the variable must equal the run without it). Failing before the replacement: `0 != 1 ... TUI unavailable — falling back to classic init` (the native leg ignored the hatch as S5 requires). `tests/test_init_native.py` now runs 13 tests, OK. `tests/test_init_tui.py::TestInitShellGating` still passes unchanged: it runs `bin/ai-specs`, which still delegates `init` to `lib/init.sh`, the launcher flip of U2e; those cases test the legacy shell hatch, not the native verb.
- L6 — Coverage mapping for the native verb's wizard cases previously exercised through the hatch: cancel → `TestInitWizardCancelThroughTheVerb`; success manifest → `TestInitWizardRunThroughTheVerb`; unavailable → classic fallback → `TestInitForcedTUIWithoutATTYFallsBack` and `TestInitWizardNonTTYExitsThreeBeforeAnyPrompt`.
- L7 — Finding left for U2e, not fixed here: after this unit no route produces `routeShim`, so `runShim` (`internal/cli/shim.go`) is unreachable. Stale comments that claimed a live consumer were corrected (`cli.go`, `shim.go`, `cli_test.go`); deleting the dead exec path belongs to U2e.
- L8 — Contract (`docs/go-migration-parity-contract.md`): rows D26, the `refresh-bundled` claims row, and `AI_SPECS_INIT_TUI_PY` rewritten to S2 and S5. Row 44 (`refresh-bundled` in the legacy surface table) is unchanged: its column semantics were not verified in this unit.
- L9 — U2a debt is not part of this unit: L11's seam-table mapping and L12's assertion change stay open for a separate audit.
- L3 — Measured before writing this document: `init.go:119` (hatch), `init.go:675-677` (python call), `lib/_internal/refresh-bundled.py:136-138` (`--init` ignored). Python tests using the hatch: `tests/test_init_native.py:402-434`, `tests/test_init_tui.py:675-692`.
