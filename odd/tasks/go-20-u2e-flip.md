# Go 20 / U2e — flip, parity corpus snapshot, Python deletion (feature)

Program: `odd/tasks/go-20-python-exit.md`, item U2e. Plan: `odd/tasks/go-20-u2-launcher-flip.md` (U2e section).
Branch `change/go-20-u2e-flip-1`, base `epic/go-single-binary` @ `5e98caf` (U2d closed, PR #431).
Slice 1 = documentation and measured inventory only. No source or test change in this slice.

## Specs

- S1 — Program rule, verbatim from the program doc: "zero Python under the product tree; bash only as
  wrappers."
- S2 — U2e scope, verbatim from `odd/tasks/go-20-u2-launcher-flip.md` (U2e): "the flip and the deletion:
  `bin/ai-specs` execs the compiled binary for every verb; the parity corpus is snapshotted and extended per
  S7; `lib/_internal` and the five differentials that run Python (`config`, `deps`, `gatebinary`, `schema`,
  `target`) are deleted; the contract (§7, D1, D9, D13, D14, D26) is rewritten with the accepted divergences
  of S4. `validate.sh` runs once on the result."
- S3 — Parity gap rule, verbatim from U2 S7: "U2e must either extend it to those verbs or record each gap
  explicitly. Silent gaps are not acceptable." Gaps: hub, upgrade, skills, add-dep, configure-recipes,
  standalone refresh-bundled, recipe remove, recipe configure.
- S4 — Accepted divergences, verbatim from U2 S4 ("aceptar ambas como contrato"), joined to the contract in
  U2e: (1) gate binary receipt without a committed digest: Go refuses, Python accepts; (2) TOML non-bare keys:
  Go quotes, Python writes raw; (3) datetimes: Go refuses, Python corrupts to strings. The doctor malformed
  manifest message is the native one (U2c).
- S5 — Trello card for U2e: none exists on board `69ec097f13e2d38ecd89a557` (lists Backlog, Ready, In Progress,
  Review, Testing interno, Done checked with filters "U2" and "Go 20"). Human decision, verbatim: "Seguir sin
  card". No card is created here; Trello comments and readback are pending until the human creates or links it.
- S6 — Escalation rule, verbatim from the parent request: "Escalame si una regeneración de sums tocara el trust
  root de release." Applies to the gate digest regeneration owned by D-U2d-9, not to this slice.
- S7 — Out of scope for this slice: the flip itself, corpus extension, deletions, contract rewrite. D-U2d-9
  stays registered in `odd/tasks/go-20-u2d-tests.md` and Engram 245, outside this feature.

## Tasks

- **T1** — Measure the U2e inventory (S2, S3). Route: inline (read-only measurement). Commit: this slice.
- **T2** — Flip: `bin/ai-specs` execs the native binary for every verb (13 `bash "$LIB_DIR/…"` dispatch lines,
  `bin/ai-specs:38-50`). Blocked until T3 and the named test rows are native. Commit: pending.
- **T3** — Retarget the U2e test rows to the native binary, no assertion removed: `test_init_brief_route`
  (LEGACY_CLI), `AgentsSubmenuManifestTests` (`lib/_internal` loader), `test_init_native` (oracle), `test_upgrade_output`
  (oracle `lib/_internal/changelog.py`), `_materialize_process` (`lib/_internal/recipe-materialize.py`). Commit: pending.
- **T4** — Extend the parity corpus to the S3 gaps or record each gap (`tests/parity/parity.py`). Commit: pending.
- **T5** — Replace the five Python differentials (`config`, `deps`, `gatebinary`, `schema`, `target`) with native
  checks that keep their assertions; then delete `lib/_internal`. Commit: pending.
- **T6** — Rewrite the contract (§7, D1, D9, D13, D14, D26) with S4. Commit: pending.
- **T7** — Decide the generated-hook `python3` (see L4). Human decision. Commit: pending.
- **T8** — `validate.sh` once on the result, native assess per candidate, PR with auto-merge. Commit: pending.

## Log

- L1 — Parent request, verbatim (2026-10-24): "Continúa la migración Python→Go: U2d quedó cerrada (PR #431 →
  5e98caf, card #189 Done, Engram 245, doc L18-L20 + deuda D-U2d-9). Recupera de Engram 241/245 y
  odd/tasks/go-20-u2d-tests.md + el programa de U2 el alcance de U2e, y ejecuta su primer slice con el flujo
  habitual: worktree propio, TDD, TMPDIR/scratch/Python fijados, assess por candidato, validate.sh una vez, PR
  al épico con auto-merge autorizado, nunca development. Límites: no crees nuevas cards en Trello — la decisión
  de Robert sigue pendiente (mi pregunta expiró sin respuesta, no la tomes ni por aprobada ni por rechazada);
  D-U2d-9 sigue registrada en doc y Engram, fuera de este slice. Si U2e carece de card existente para
  comentarios/readback, repórtamelo en cuanto lo compruebes en vez de crearla. Escalame si una regeneración de
  sums tocara el trust root de release. Comienza por recuperar estado y confirmarme el alcance y la card de U2e
  antes de implementar."
- L2 — Human answers in the same session (verbatim): slice = "Doc + inventario (Recomendado)"; card = "Seguir
  sin card".
- L3 — Measured inventory (epic 5e98caf, worktree `change/go-20-u2e-flip-1`):
  - 49 test modules touch `lib/_internal` (list in scratch `u2e/internal-users.txt`).
  - 51 test modules touch the launcher or `lib/*.sh` (scratch `u2e/launcher-users.txt`).
  - `tests/parity/parity.py` defines 29 fixtures. Verbs covered: init, sync, sync-agent, doctor, rules-audit,
    help, version, recipe list/init/add, `bogus-verb`. Not covered (S3 gap): hub, upgrade, skills, add-dep,
    configure-recipes, refresh-bundled standalone, recipe remove, recipe configure.
- L4 — Python execution on the product path, measured. No direct `exec` of `python3` remains in active non-test
  Go code. Findings:
  - `internal/sync/hooksrender.go:771` emits `python3 -c` inside the generated Cursor hook script. This is product
    output, a runtime dependency on the user's `python3`, not a Go-side exec. Decision needed (T7).
  - `internal/cli/recipe.go:2205` (`configureFrontDoor`) and `internal/hub/interactive.go:205` (hub `delegate`)
    run `<home>/bin/ai-specs`. After the flip the launcher execs the binary, so these hops stay valid but add
    one process each. Recorded as T2 dependencies.
  - `internal/cli/shim.go:36` runs `bash`. Dead code since U2b (L7 in `go-20-u2b-init.md`); deletion is U2e.
  - `internal/sync/sync.go:132-137`: `stepMode` defaults to `"python"` and its comment says the default "still
    execs the remaining modules". Not yet verified whether any step still reaches Python. Open for T2/T5.
  - `internal/target/resolve.go:35` keeps `programName = "lib/_internal/target-resolve.py"` only for the usage
    string on stderr. Deleting `lib/_internal` requires changing it (parity row).
- L5 — Trello: no card for U2e (S5). Engram 245 open item 1 (no card for D-U2d-9) also stands.
