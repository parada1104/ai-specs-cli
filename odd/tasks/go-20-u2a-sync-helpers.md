# Go 20 / U2a — native sync helpers

Unit U2a of `odd/tasks/go-20-u2-launcher-flip.md`. Branch `change/go-20-u2a-sync-helpers` off
`epic/go-single-binary` @ `7daf7eb`. Each Python helper keeps its script under `lib/_internal/` until
U2e, so every port runs a differential against the Python while it still exists.

## Specs

- S1 — Parent unit, verbatim from the U2 plan: "U2a — native sync helpers: check-sync, gitignore render ×2,
  refresh-bundled call, vendor-skills, brief-render-policy, agents-render, target-resolve (S5)."
- S2 — Inventory, verified on `7daf7eb` (`internal/sync/sync.go`):
  - `check-sync`: `sync.go:211` runs `cli_version.py check-sync`. The policy core is native
    (`internal/cliversion/cliversion.go:1-10`); only the process shim is Python.
  - `gitignore-render`: `sync.go:263`. Native twin in `internal/sync/gitignore.go`; the differential at
    `internal/sync/gitignore_test.go:121-125,343-350` runs the real Python.
  - `gitignore-root-refresh`: `sync.go:271`. Same native twin, same differential.
  - `refresh-bundled`: `sync.go:276`. No native sync path. The top-level verb is a compatibility no-op
    (`internal/cli/refresh_bundled.go:29-32`), so this port is real work.
  - `vendor-skills`: `sync.go:281`, plus the recipe-dep delegation at `materialize_loop.go:615-673`. Only
    part of skill rendering is native (`internal/skills/vendored.go:16-20`). Real work.
  - `brief-render-policy`: `sync.go:391`. Native twin `internal/sync/briefpolicy.go`.
  - `agents-render`: `sync.go:393`. Native twin `internal/sync/agentsrender.go:1332-1414`.
  - `target-resolve`: **not a Python call.** `sync.go:443-460` calls `target.Run` natively. The U2 plan's
    S5 listed it in error; the correction is logged there (L4).
- S3 — Two Python executions outside the S2 list, both must be classified in this unit:
  - `materialize_loop.go:670-673` (`materializeRunPython`): one inline `python3 -c` for recipe-dep delegation.
  - `hooksrender.go:771`: the GENERATED hook script shells out to `python3 -c` to JSON-encode its output.
    This ships to user machines. It is product output, not a Go→Python call, and it is a decision (D4).
- S4 — Ordering, which the port must keep: gitignore before cache and deps; refresh before vendor; brief
  policy and render after recipe materialization (`sync.go:259-281,346-398`). Lock writes are serial:
  refresh, vendor, renderer, then the final version stamp (`sync.go:433-434`). Sync is not transactional:
  earlier writes stand when a later step fails (`sync.go:425-426`). The port keeps all of it.
- S5 — Divergences this unit must decide, not silently keep or silently change:
  - D1 — file mode. Native gitignore writes fixed `0644` (`internal/sync/gitignore.go:71-75,135-152`);
    Python's `write_text` is umask-dependent. The parity harness measures modes (`tests/parity/parity.py:7-16`).
  - D2 — unmatched begin marker in `gitignore-root-refresh`. Native prints one line; Python raises with a
    traceback (`internal/sync/gitignore.go:92-93,122-124`). Both fail; the text differs.
  - D3 — `brief-render-policy` exit is ignored at `sync.go:391`; the caller gates on stdout `true`. A
    missing output can turn into a skipped brief while stderr reports an error. Preserve, or fix?
  - D4 — the generated hook's `python3` dependency (S3). Out of U2's scope to remove silently; it is a
    product decision about what the installed hook may assume.
- S6 — Failure of a refresh leaves a partial cache: destinations are deleted before the copy
  (`refresh-bundled.py:65-75,87-98`). The port keeps the behaviour unless the human decides otherwise.
- S7 — Gates per task: `go build ./...`, `gofmt -l`, `go vet` on touched packages, focused tests in
  `internal/sync/`, the parity runner for the full-sync fixtures (`tests/parity/parity.py:1033-1036`), and
  `./tests/validate.sh` once by the parent. Python legs under `PATH=/tmp/pybin:$PATH`.

## Tasks

- **T1** — `check-sync`: route sync to the native check. Differential against `cli_version.py check-sync`,
  exit codes 0 / 1 / warning on stderr (`sync.go:215-216` collapses nonzero to 1). Tests: the existing
  sync-gate cases in `tests/test_sync_pipeline.py:3436-3438` stay green.
- **T2** — `gitignore-render` and `gitignore-root-refresh`: call the native functions. Re-run the existing
  differential. D1 and D2 are decided here; the decision is written in the unit log before the commit.
- **T3** — `brief-render-policy` and `agents-render`: call the native functions. D3 is decided here. The
  existing differentials (`internal/sync/agentsrender_test.go:714-717`) run against the Python first, then
  against the native call.
- **T4** — `refresh-bundled`: native cache rebuild, legacy cleanup and lock normalization. Differential
  against `refresh-bundled.py`, including the partial-cache behaviour of S6.
- **T5** — `vendor-skills`: native root `[[deps]]` path and the recipe-dep delegation. Differential against
  `vendor-skills.py` with `AI_SPECS_VENDOR_FIXTURE_ROOT`, the same fixture the Python uses.
- **T6** — `materializeRunPython` (S3): replace the inline `python3 -c` with the native path it wraps.
  D4 is decided before the hook generator is touched.
- **T7** — remove the Go→Python executions in `internal/sync/sync.go` once T1–T6 are green. Acceptance:
  `grep -n python3 internal/sync/*.go` returns only tests and generated hook content (the latter per D4).

## Decisions this unit needs from the human

Written here so they are not discovered mid-task. Each one is taken before its task starts.

- D1 — gitignore file mode: keep `0644` (native) or match the Python's umask-dependent mode.
- D2 — unmatched begin marker: accept the native one-line wording as contract, or match the traceback.
- D3 — ignored `brief-render-policy` exit: preserve the current skip-on-missing-output, or fail loudly.
- D4 — the generated hook's `python3` dependency: keep it (installed hooks may assume python3), or
  replace the JSON encoding with a shell-only form.

Decided by the human on 2026-10-22, taking the recommendation in each case:

- D1 → **match the umask.** The native gitignore writes with the umask-dependent mode, like Python.
  The fixed `0644` is an accident, not a hardening. T2 changes `internal/sync/gitignore.go:71-75,135-152`.
- D2 → **accept the native wording** as contract. Both paths fail with a nonzero exit; only the text
  differs. Recorded as an accepted divergence in U2e.
- D3 → **preserve** in U2a. A port is not a behaviour change. The silent skip is a separate unit
  to be scheduled after U2.
- D4 → **keep the hook's python3** for now. Installed hooks may depend on it, and changing a shipped
  artifact is a separate decision. T6 leaves the generated hook untouched.

## Log

- L1 — Parent request, verbatim: "ok" (2026-10-22), approving the merge of #407 and the drafting of this
  unit document, with no code in this step.
- L2 — The inventory in S2 was measured by read-only exploration on `7daf7eb` and checked by grep before
  this document was written.
- L3 — S3 surfaced two Python executions the U2 plan had not listed. They are added here, not hidden.
- L4 — Correction to `go-20-u2-launcher-flip.md` S5 (the U2 plan): `target-resolve` is not executed by
  Python from sync. Measured: `sync.go:443-460` calls `target.Run`. The U2 plan's list is corrected here, and
  the U2 plan's own text is left as written for its history.

- L5 — The four decisions D1–D4 were taken by the human on 2026-10-22, each with the recommended
  option. They are recorded in the Decisions section above, before any task starts.
