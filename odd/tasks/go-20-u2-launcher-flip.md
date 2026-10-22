# Go 20 / U2 — launcher flip, parity snapshot, Python deletion (plan)

Program: `odd/tasks/go-20-python-exit.md`, item U2. This document plans U2 and splits it into five units.
Each unit gets its own `odd/tasks/go-20-u2<x>-<name>.md` when it starts. Branch `change/go-20-u2-plan`,
base `epic/go-single-binary` @ `9d54962`.

## Specs

- S1 — Program rule, verbatim from the program doc: "zero Python under the product tree; bash only as
  wrappers."
- S2 — Program S9, verbatim from `## Next`: "those files leave only when `bin/ai-specs` stops calling them,
  which is U2's flip, not U7's." Retiring a module means the binary stops using it; deleting it is U2's job.
- S3 — Partition, approved by the human on 2026-10-22 after the proposal: five units, U2a → U2e, each with
  its own PR to the epic. The human's answer, verbatim: "Aprobar la partición (recomendado)".
- S4 — Accepted divergences, decided 2026-10-22, verbatim: "aceptar ambas como contrato". They join the
  parity contract (`docs/go-migration-parity-contract.md` §7 and its rows) during U2e:
  1. gate binary receipt without a committed digest: Go refuses, Python accepts (`internal/gatebinary/gatebinary.go:273-274`);
  2. TOML keys: Go quotes non-bare inline-table keys, Python writes them raw (`internal/config/recipeconfig.go:62-64`);
  3. datetimes: Go refuses, Python corrupts them to strings (`TestUpdateRecipeConfigRefusesADatetimeTheGateCopyCorrupts`).
  And the doctor diagnostic for a malformed manifest: the native message is accepted, not tomllib's text (U2c).
- S5 — Blockers, measured on epic `9d54962`:
  - the launcher still dispatches to bash: 13 `bash "$LIB_DIR/…"` lines in `bin/ai-specs`;
  - Go still executes Python in `sync`: `internal/sync/sync.go:211` (cli_version check-sync), `:263`
    (gitignore-render), `:271` (gitignore-root-refresh), `:276` (refresh-bundled), `:281` (vendor-skills),
    `:391` (brief-render-policy), `:393` (agents-render); plus target-resolve at `:443`;
  - doctor parses malformed manifests through Python's tomllib (`internal/doctor/bridge.go:63-92`);
  - init keeps the `AI_SPECS_INIT_TUI_PY` hatch (`internal/cli/init.go:118-120`) and runs `refresh-bundled.py`.
  Deleting `lib/_internal` before these are closed breaks the native binary. That is why U2 is five units.
- S6 — Tests that drive the launcher: 20 modules invoke `bin/ai-specs` (`_blackbox.invoke` targets it,
  `tests/_blackbox.py:18,117-150`), 5 of them test legacy scripts directly (recipe remove, skills
  add/list/remove, upgrade). Two read the launcher as shell text: `tests/test_upgrade.py:25-28` and
  `tests/test_harness_cli_literacy.py:90-103`.
- S7 — Parity corpus: `tests/parity/parity.py` defines 29 fixtures (init, sync, sync-agent, doctor,
  rules-audit, dispatcher help/version, some recipe paths). It does NOT cover hub, upgrade, skills,
  add-dep, configure-recipes, standalone refresh-bundled, recipe remove, or recipe configure. U2e must
  either extend it to those verbs or record each gap explicitly. Silent gaps are not acceptable.
- S8 — Stale or contradicted contract rows, to settle in U2e: D1, D13, D14 are flagged stale in the
  program log; D9 says `upgrade --dry-run` can write (Go fixes it, the legacy `lib/upgrade.sh:264,278-280`
  still runs `git checkout --`); D26 says refresh-bundled, doctor and hub claim read-only, while the direct
  Go refresh route is a compatibility no-op (`internal/cli/refresh_bundled.go:12-30`).
- S9 — Gates per unit: `go build ./...`, `gofmt -l`, `go vet` on touched packages, focused tests, the
  parity runner (`tests/parity/run.py`), and `./tests/validate.sh` run once by the parent. Python legs under
  `PATH=/tmp/pybin:$PATH`.

## Tasks

- **U2a** — native sync helpers: check-sync, gitignore render ×2, refresh-bundled call, vendor-skills,
  brief-render-policy, agents-render, target-resolve (S5). The Python still exists during U2a, so each
  port gets a differential against it. Commits per helper group.
- **U2b** — init: retire the `AI_SPECS_INIT_TUI_PY` hatch (`internal/cli/init.go:118-120`) and the
  `refresh-bundled.py` call. Decide D26 for the standalone refresh verb: a real refresh, or a documented
  no-op with its contract row rewritten. This decision is the human's, taken when U2b starts.
- **U2c** — doctor: the malformed-manifest diagnostic moves to a native parser. Its message is the
  accepted divergence from S4. Its differential is replaced by a documented row.
- **U2d** — retarget the 20 launcher-driven modules to `_go_cli.cli()` and rewrite the 5 legacy-only
  modules to drive the binary. Adjust `test_upgrade.py` and `test_harness_cli_literacy.py`, which read
  the launcher as text. No test is deleted without a replacement that asserts the same behaviour.
- **U2e** — the flip and the deletion: `bin/ai-specs` execs the compiled binary for every verb; the parity
  corpus is snapshotted and extended per S7; `lib/_internal` and the five differentials that run Python
  are deleted (`config`, `deps`, `gatebinary`, `schema`, `target`); the contract (§7, D1, D9, D13, D14,
  D26) is rewritten with the accepted divergences of S4. `validate.sh` runs once on the result.

## Log

- L1 — Session request, verbatim (2026-10-22): "ok, vamos", answering "¿Arranco U7-6 ya?". The
  remaining decisions followed in the same session (S3, S4, U2c).
- L2 — U7-6 was dropped: its modules have native twins or die with U2 (exploration of the Python callers,
  checked against `bin/ai-specs:33-40`, `internal/cli/init.go:118-120`, and the five Go differentials that
  run Python).
- L3 — U2 was re-scoped from one flip to five units after the blockers in S5 were measured. The first
  plan named one flip; that premise was checked against the code before any deletion was scheduled.
