# change-ledger-t6b (ODD)

Unit T6b of the change-ledger-v2 redesign: the plan build-flow ledger becomes a real
section, the monolithic single-store writer and its dead writers go, corruption stays
contained to the file it belongs to, and the S14 contract names the entrypoint instead
of a set of forbidden strings. It completes T6 after T6a delivered the per-capability
one-file layout. Master plan, design and audit live in `odd/tasks/change-ledger-v2.md`
and `odd/tasks/change-ledger-v2/`; T6a's own plan and its Log (L5-L20) are the
immediate predecessor and must be read before touching the ledger.

## Specs

- **S3** — One ledger per capability, in the CLI, conversing without mixing state.
  Verbatim: «no es un ledger único. Hay un ledger por capacidad, todos en el CLI,
  que conversan pero no se mezclan: el de worktree (identidad, merge, limpieza,
  integración), el de plan-build-flow (la capa del change y su flujo: si
  plan-build-flow está activo el checkpoint tiene más pasos) y el del tracker
  (conciliación entre el tracker y los changes)».
- **S11** — Store v2 with one-version double read; the 14 rows stay as history.
  Verbatim: «store v2 con doble lectura una versión, las 14 filas quedan como
  histórico».
- **S14** — Everything goes through the CLI entrypoint; the on-disk layout is not a
  contract. Verbatim: «cada ledger con su propio archivo está bien siempre que todo
  pase por el entrypoint del CLI de ai-specs (nadie lee ni escribe esos archivos
  directamente)».

## Tasks

- T6b.1 — The plan ledger owns the plan section. The `workstream` and `workstream-end`
  events that today live in `.ai-specs/plan-ledger.jsonl` (`internal/planledger`,
  appended from `internal/cli/changes.go`, read by `internal/gates/writefacts.go`)
  move into `LedgerFile.Plan`, seeded on read from the existing JSONL without losing
  an event, with the same one-shot backup discipline T6a established. Specs: S3, S11.
  route: worker. commit: pending.
- T6b.2 — Delete the monolithic writer and the dead writers: whatever still writes a
  whole store without going through a capability section, plus `PersistConflict` and
  `AppendDecisionToPrimary` (no production caller; carried from T5 L9, deferred here by
  the master plan). Specs: S3, S14. route: worker. commit: pending.
- T6b.3 — Corruption stays contained: a corrupt file is unevaluable and every write
  fails closed with the file byte-identical (the L15 replacement for the old
  "a corrupt ledger leaves the others answering", which has no subject in one file),
  and an unreadable migration source is reported without disabling the ledger. Specs:
  S11. route: worker. commit: pending.
- T6b.4 — The S14 contract test names the entrypoint: it must fail on a new API-only
  caller such as `ledger.LedgerAt(root).Load()` from outside the gate entrypoint, not
  just on `ledgers/` literals. Specs: S14. route: worker. commit: pending.
- T6b.5 — Validation: focused suites, both Go modules, `ledger_cleanup_flow.sh` before
  and after, the full Python suite, and the real-store probes T6a used (v1 only; v1 +
  T5 tracker; T5 single open row; the 0.28.0 -> T5 upgrade; v1 with a lifecycle
  opt-out). Specs: S3, S11, S14. route: worker. commit: pending.

## Allowed edit surfaces

- catalog/recipes/worktree-flow/gate/ledger/
- catalog/recipes/worktree-flow/gate/runner/
- internal/planledger/
- internal/cli/
- internal/gates/
- tests/scenarios/ledger_cleanup_flow.sh
- tests/
- odd/tasks/change-ledger-t6b.md

## Verification

| Spec | Evidence |
| --- | --- |
| S3 | A workstream decision lands in the plan section and nowhere else; the worktree and tracker sections are untouched by it; no row is written to two sections. |
| S11 | The JSONL events survive migration byte-identically (or with a one-shot backup), the v1 ledger still migrates, and no historical row is lost. |
| S14 | The contract test fails on an outside caller using the new API, and every read and write still happens through the CLI entrypoint. |

## Log

- **L1** — Mandate, verbatim (from `odd/tasks/change-ledger-t6.md` L1, which covers
  this unit): «Sigue con el plan maestro: T4 (aviso de Pi visible) y luego T5,
  T6… en orden, una unidad = un worktree nuevo desde development + un PR, con el
  mismo mandato (validar con flujo real y RED, revisión nativa antes del merge; si
  la revisión se traba en el paso de validación como en T2, me avisas con
  LISTO REVISAR-<unidad> y reviso yo; PR, merge, `ai-specs sync` del repo con su
  PR). Sin release hasta T12, salvo que una unidad lo necesite. Guarda en Engram
  antes de cada merge (el hook retira el worktree). Cuando el contexto pase de
  ~45 %, cierra en Engram y avísame con LISTO RELEVO para abrir sesión nueva.
  Avísame al cerrar cada unidad con una línea LISTO <unidad> (p. ej. LISTO T4).»
  Plus, verbatim, from Robert at the close of T6a: «Antes del merge: handoff en
  Engram y worktree de T6b.»
- **L2** — Master plan entry, verbatim (`odd/tasks/change-ledger-v2.md`, T6):
  «T6 — Worktree and plan stores extracted, with the same invariants (S3, S11, S14).»
  T6a delivered the worktree half; this unit delivers the plan half plus the deletion
  of the monolithic writer.
- **L3** — Inherited state, not to be re-derived. T6a (`5317fe47`, PR #620) made the
  ledger one versioned file with a `worktree`, a `tracker` and a shape-only `plan`
  section, one atomic rename per write, the lock `<store>.lock`, `--store` naming THE
  file, and a read-time migration absorbing both predecessors (the v1 file and T5's
  `ledgers/tracker.json`) with the newest form winning. `StoreLocation{Path,
  Migration}`, `LedgerFile`, `LedgerAt`, `StoreAt`, `Load`, `Save`, `Backup`,
  `StoreReport`, `View` and the share functions are its API. Still present and owed to
  this unit: `PersistConflict`, `AppendDecisionToPrimary`, the plan section with no
  writer, and the `.ai-specs/plan-ledger.jsonl` events outside it.
  Traps carried over, each of which cost real time: the pi-lens LSP binds the MAIN
  worktree (its `undefined:` reports are stale in a linked one — only `go build`,
  `go vet`, `go test` count); the gate is its own Go module under
  `catalog/recipes/worktree-flow/gate`; `tests/run.sh` aborts at its parity leg, so run
  `python3 -m unittest discover -s tests -p 'test_*.py'` directly (about ten minutes;
  the base has exactly one pre-existing failure,
  `test_git_pr_flow_recipe.GitPrFlowGoldenContentTests.test_skill_never_recommends_delete_branch`);
  a merge retires the worktree and the cwd; native review cannot run on candidates of
  this size (`lens_context_budget_exceeded` from ~3300 changed lines with every lens
  set), so a candidate of that size goes to Robert with `LISTO REVISAR-<unit>`.
  Lesson worth carrying: the two defects that survived T6a's first probes were the ones
  whose fixtures differed from reality only in a detail (the real rows carry no
  opt-out; an older binary can relink and close a row instead of only adding one).
  Probe the queries that read, not just the writes that arrive.
- **L4** — Release findings from shipping 0.29.0, recorded for their own unit (not fixed
  here, and not this unit's scope):
  1. **`scripts/build-cli.sh` builds the four assets but never writes
     `bin/SHA256SUMS`**, although its own header says the digests must be regenerated
     after any CLI change. A version bump that changes the binary therefore ships a
     trust manifest holding the previous release's digests, and every install of the new
     asset would be refused as a mismatch and silently fall back to a local Go build.
     Reproduced: the 0.29.0 binaries hashed `21c88263…`, `d7aa2ae4…`, `8d611720…`,
     `aaeae58d…` while the committed manifest still held 0.28.0's digests; the manifest
     had to be regenerated by hand. Fix direction: make the script write the manifest
     (or fail when it is stale) so the release cannot publish a mismatched asset.
  2. **`ai-specs upgrade` reports «Already up to date» by comparing `VERSION` and can
     leave a stale launcher binary.** Reproduced: the home checkout fast-forwarded to
     0.29.0 while `~/.ai-specs/dist/ai-specs` stayed the 0.28.0 asset (`acc2cff7…`
     instead of `d7aa2ae4…`) and `dist/VERSION` kept the old stamp; `ai-specs version`
     still answered 0.29.0 because it reads the checkout's `VERSION`, so the mismatch was
     invisible from the command users run. Only `install.sh` noticed and printed
     «verified release asset». Fix direction: compare the installed binary's digest
     against the trust root, not the version string, and refresh the launcher when they
     differ.
  Both matter for T12 (release) and for any unit that ships a binary change.
