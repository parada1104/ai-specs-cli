# Go 20 / U2c — doctor: native malformed-manifest diagnostic, retire the Python bridge

Unit U2c of `odd/tasks/go-20-u2-launcher-flip.md`. Branch `change/go-20-u2c-doctor` cut from `epic/go-single-binary` @ `9ed25e1` (U2b, PR #419) and rebased onto `epic/go-single-binary` @ `9664909` (PR #420, PTY repair; see L6). PR target: `epic/go-single-binary`, never `development`.
Tracker: epic card https://trello.com/c/qwlHQ7Xa; U2c card #187 https://trello.com/c/DBsYnIgj (board `69ec097f13e2d38ecd89a557`, In Progress). Predecessor card #186 (U2b, Done).

## Specs

- S1 — Parent request, verbatim (2026-10-23): "Continúa migración Go 20 con U2c, doctor bridge." and "El diagnóstico nativo del manifiesto malformado ya fue aceptado como divergencia en S4/U2c: no reabras esa decisión."
- S2 — Plan U2c, verbatim from `go-20-u2-launcher-flip.md`: "the malformed-manifest diagnostic moves to a native parser. Its message is the accepted divergence from S4. Its differential is replaced by a documented row."
- S3 — Plan S5, verbatim: "doctor parses malformed manifests through Python's tomllib (`internal/doctor/bridge.go:63-92`)." After this unit no doctor run spawns `python3`.
- S4 — Accepted divergence (plan S4, decided 2026-10-22, not reopened): the malformed-manifest guidance is the native parser's diagnostic (`internal/toml.Parse` error text), not tomllib's text. The `manifest` check keeps its name, severity and message `<rel> is not parseable`; only the guidance text changes. A read failure reports the read error in the same slot.
- S5 — Retirement: `internal/doctor/bridge.go` and `bridge.py` are removed with their op plumbing. The `doctor-bridge` ERROR check (`internal/doctor/doctor.go:126-130`) can only fire when the bridge cannot run; with no bridge it is dead and is removed with it. Every other check, order, severity, message and exit code stays FROZEN.
- S6 — Contract: `docs/go-migration-parity-contract.md` gets a documented row for the malformed-manifest guidance (accepted divergence, S4) in place of a differential, and any line that still describes doctor's Python bridge is rewritten to what the code does.
- S7 — Out of scope: U2a debt L11/L12 (separate audit, not declared settled here); the tracker ledger hook (still not materialized; no `ai-specs sync` or upgrade run); U2d launcher-driven tests; U2e deletion of `lib/_internal`; historical `odd/tasks/go-08-doctor.md`.
- S8 — Permissions: commits and a PR to `epic/go-single-binary`. Native assess on this candidate runs BEFORE delivery with `baseRef` and `workspaceRoot` of this unit (U2b skipped it; recorded as a deviation). Consent is not inherited from earlier reviews. Full `./tests/validate.sh` run once by the parent.

## Tasks

- **T1** — RED: an unparseable manifest produces the native guidance and no `python3` child. Go test in `internal/doctor` that shadows `python3` on PATH with a logging script and asserts the log is empty and the `manifest` ERROR guidance starts with `toml: line`. Fails today (bridge runs, guidance is `manifest is not valid TOML`). Covers: S3, S4.
- **T2** — GREEN: `checks_core.go` renders `d.manifest.parseErr`; delete `bridge.go`, `bridge.py`, `runBridge`, `bridgeOps`, the `bridge`/`bridgeErr` fields and the `doctor-bridge` check in `doctor.go`; update the package and `Run` comments. Covers: S3, S4, S5.
- **T3** — Replace the bridge-op tests in `checks_brief_native_test.go` (`TestBridgeOpsRequestOnlyTomlError`, the bridge wording in `TestBriefProbesRunWithoutTheBridge`) with tests against the public outcome: no `python3` child, brief checks unchanged. Covers: S5.
- **T4** — Contract rows (S6). Covers: S6.
- **T5** — Gates: `go build ./...`, `gofmt -l`, `go vet ./internal/doctor/`, `go test ./internal/doctor/`. Native assess with `baseRef` = `9664909`, `committedOnly` = true, `workspaceRoot` = this worktree, then PR. Full `./tests/validate.sh` once by the parent.

## Log

- L1 — Parent request verbatim: see S1.
- L2 — Measured before writing (epic `9ed25e1`): `bridge.go` is the only Go→Python call in `internal/doctor` (`bridge.go:92`); the only op left is `toml_error` (`bridge.py`); `checks_core.go:24-29` is its sole consumer; `doctor_test.go:208` asserts only the `is not parseable` suffix. No parity fixture covers a malformed manifest (`tests/parity/parity.py` `doctor-*` fixtures use valid manifests).
- L4 — T1 RED observed: `TestMalformedManifestGuidanceIsNative` failed with `doctor spawned python3 for a malformed manifest: "invoked\n"` (python3 shadow on PATH logged the bridge call). T2-T3 GREEN: the same test passes; `TestDoctorSpawnsNoPython` pins a parseable manifest with brief fragments (no python3, no `doctor-bridge`). T4: contract §2 `doctor` gained the accepted-divergence row. Stale comments naming the deleted `bridge.py` were corrected in `internal/sync/briefview.go` and `internal/sync/testdata/briefref_ref.py`.
- L5 — Gates on the candidate with `PATH=/tmp/pybin:$PATH` (the default `python3` is 3.9 without `tomllib`, which failed the differential tests before the Python leg was moved): `go build ./...` OK, `gofmt -l internal/` clean, `go vet ./internal/doctor/ ./internal/sync/ ./internal/cli/` OK, `go test ./internal/doctor/ ./internal/sync/ ./internal/cli/` OK. Full `./tests/validate.sh`: parent, after commit.
- L6 — Rebased onto epic tip `9664909` (#420) with no conflicts; the unit commit is the single candidate on top of it. Gates on the rebased candidate with `python3` 3.14.7 (`tomllib`) first on PATH: `go build ./...` OK, `gofmt -l internal/` clean, `go vet` OK, `go test ./internal/doctor/ ./internal/sync/ ./internal/cli/ -count=1` OK. Under the default `python3` 3.9 and under 3.11 the differential tests fail on interpreter-dependent texts (`tomllib`, float error wording); the same failures reproduce on epic `9664909` without this unit, so they are environment, not U2c.
- L3 — Decision taken inside permissions: the `doctor-bridge` ERROR check is removed (S5) because no path can produce it once python3 is not spawned. Not a roster change for any reachable state.
