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
- **L5** — Baseline before any change, on `306691d8`: the gate module
  (`catalog/recipes/worktree-flow/gate`) builds, vets and tests clean and the root
  module builds and vets clean (`go test ./ledger/... ./runner/... -count=1`: both ok,
  3.8 s and 44 s). Log: `/private/tmp/change-ledger-v2/t6b/baseline.log`.
- **L6** — The plan section's shape and the identity it is keyed by. `PlanSection`
  stops being `[]json.RawMessage` (T6a shipped it shape-only, L15.1) and becomes
  `{items:[{identity, decisions:[{at, kind, checkpoint, choice, note, reason}]}]}`,
  with `kind` one of `workstream` / `workstream-end` for this unit (a later unit adds
  `plan-exempt` and `archive`). One refinement of L8, decided here and stated for the
  reviewer: the row carries the **existing `ItemIdentity`** (`common_dir`, `branch`,
  `change`) instead of L8's bare `change` slug, because the ledger file is shared by
  every worktree of the clone (`<git-common-dir>/ai-specs/ledger/state.json`) while a
  workstream declaration is per-checkout (T3 chose a per-checkout file for exactly
  that reason: «a worktree carries its own declaration and its siblings cannot inherit
  it»). Keying by identity keeps that property in one shared file and reuses the
  identity type the other two rows already use; a workstream row is written with an
  empty `change`, because a workstream by definition has no change. No new key, no new
  identity scheme.
- **L7** — Seeding, and why the JSONL becomes a predecessor instead of a second
  writer. `.ai-specs/plan-ledger.jsonl` is read **on load** by `StoreLocation`, through
  a new `PlanSource{Path, Key}` field that names the checkout's JSONL and the work
  identity that owns it; every event it holds becomes one `PlanDecision` for that
  identity, in file order, so no event is lost. It contributes **only when the plan
  section has no row for that identity**, which is the same rule T6a settled for the
  tracker and for the same reason (L17 ALTO-1: a newer file wins over an older binary's
  append; the alternative is `close,open`-style resurrection of a decision that was
  already superseded). A one-shot copy of the JSONL is kept by the same
  `backupPredecessor` discipline before the first write that supersedes it, and an
  unreadable or unparseable JSONL is reported through `StoreReport.LegacyUnreadable`
  without disabling the ledger (L17 MEDIO-3, now applied to the third source). Reads
  fold last-event-wins exactly as `planledger.Workstream` does today, so the
  declaration's meaning does not change; unknown event names are still skipped.
- **L8** — T6b.2, decided. «Whatever still writes a whole store without going through
  a capability section» is `StoreLocation.Save(LedgerFile)`: the only export that
  writes all three sections at once, and the reason a caller could put a row in a
  ledger it does not own. It is not deleted but made incapable of that: `Save` takes
the sections the caller owns (`Save(file, SectionWorktree, SectionTracker)`) and
  re-reads every section it was not given from disk before writing, so an unowned
  section cannot be overwritten even by a hand-built `LedgerFile`. Deleting it
  outright was the alternative and was rejected on evidence: four cross-package
  fixtures (`internal/cli`, `gate/runner`) seed a file the production mutators cannot
  construct (a pinned item id, a collided pair, an adjudication history), so the
  export would have to be re-added as a seeding helper with a different name; the
  ownership contract gives the published invariant instead, and it is testable. Plus
  the two dead writers and their now-unreachable helper: `PersistConflict`
  (`decide.go:123`), `AppendDecisionToPrimary` (`store.go:592`) and `setConflict`.
- **L9** — T6b.4, decided: the contract test stops matching a string list and starts
  naming the API. Detection becomes (a) the existing forbidden literals and (b) any
  shipped source outside the owned prefixes that imports `worktree-gate/ledger` or
  `worktree-gate/ledger/...`, plus the store entrypoints that have no string form
  (`LedgerAt(`, `.Load()`, `.Save(`). `internal/planledger/` joins the owned prefixes,
  because it is now the CLI's port to the plan section — the one place outside the
  gate that may reach the ledger, and it is reachable only from `internal/cli`. RED is
  a new file under a non-owned package calling `ledger.LedgerAt(...).Load()`, which the
  test must name; the same file must be deleted afterwards, so the RED proof is
  recorded as the failure text in this Log.
- **L10** — T6b.1 and T6b.2 landed with their tests first. RED for the plan section, on
  `306691d8` with the tests written and the API absent: `go vet ./ledger/` →
  `ledger/plan_test.go:31:46: ledger.LedgerAt(ident.CommonDir).WithPlanSource undefined
  (type ledger.StoreLocation has no field or method WithPlanSource)`
  (`/private/tmp/change-ledger-v2/t6b/red-t6b1.log`). GREEN: `go test ./ledger/ -count=1`
  passes with nine new plan tests — the decision lands only in the plan section, every
  JSONL event is absorbed in order, the file answers over a stale append, the first
  write absorbs the predecessor and keeps its one-shot copy, a sibling checkout inherits
  nothing, an unreadable predecessor is reported and not fatal, unparseable lines stay
  skipped, and the writer refuses an identity it cannot name.
  Deleted: `PersistConflict`, `AppendDecisionToPrimary` and their now-unreachable
  `setConflict`; `TestPersistConflictFailsClosedForCollision` and
  `TestAppendDecisionToPrimaryFailsClosed` went with them, and
  `TestConcurrentAppendDoesNotLoseDecisions` was **retargeted to a live writer**
  (`PersistDecision`) rather than deleted, so the atomic-replace-and-lock pin survives.
  `Save` now names its sections: a write that names none is refused, and every section
  it was not given is re-read from disk, pinned by
  `TestSaveCannotWriteASectionItDoesNotOwn` (a tracker-only write that hands over a
  stolen lifecycle row and a stolen plan row writes neither).
- **L11** — A real defect the new tests found, and its fix. An unreadable predecessor
  (a directory where a ledger file belongs) made **every** write fail closed: `Save`
  treats a predecessor it cannot copy as a failed backup, which is right for a regular
  file with unreadable bytes and wrong for a path that holds no predecessor bytes at
  all. Reproduced by two new tests (`TestUnreadableMigrationSourceDoesNotDisableWrites`,
  `TestUnreadablePlanSourceDoesNotDisableWrites`) failing with «refusing to write …
  without a predecessor backup: read …: is a directory». Fixed in `location.go` with
  `predecessorBytes`, which reports an absent or non-regular path as «nothing to copy»
  while a regular file whose bytes cannot be read still fails the save closed (the
  existing `TestWriteFailsClosedWhenTheBackupCannotBeWritten` keeps passing, unweakened).
- **L12** — T6b.4 delivered, with one prefix decision taken here. The contract test now
  detects the API, not only its string forms: `LedgerAt(`, `.Load()` and `.Save(` joined
  the forbidden entrypoints, and an import of `ai-specs.dev/worktree-gate/ledger` (or a
  subpackage) outside the owned prefixes is a finding on its own. `internal/planledger/`
  joined the prefixes, and so did `catalog/recipes/worktree-flow/gate/shared/` — the one
  change to L9's list, taken on evidence the worker escalated rather than silently
  fixing: `witness.go` there imports the ledger package for the witness schema and
  `WitnessPath`, never for a store call, and it is compiled into the gate the CLI runs
  in-process. Widening to the whole gate module (the alternative) was rejected because
  it would license any future gate subpackage to reach the store. Parent-verified RED,
  with a compiling probe at `internal/tier/zz_contract_red.go` that names no path and
  only calls the API:
  «`internal/tier/zz_contract_red.go imports ai-specs.dev/worktree-gate/ledger` /
  `internal/tier/zz_contract_red.go names LedgerAt(` / `… names .Load()`» — three
  independent detections for one caller, green again once the probe was deleted
  (`/private/tmp/change-ledger-v2/t6b/red-t6b4.log`).
- **L13** — Real-flow probe over copies of the real 14-row store, built binary, no
  fabricated rows (`/private/tmp/change-ledger-v2/t6b/probe.sh`, log beside it): **PASS**.
  With a three-event JSONL predecessor beside the real v1 file, a read absorbs the
  declaration and leaves both files byte-identical; `--reason` writes v2 with 14
  worktree and 14 tracker rows, one plan row carrying
  `[workstream, workstream-end, workstream, workstream]` with each reason preserved, ids
  joined across sections, `status` only in the worktree section and no `close` in the
  tracker, and both predecessors keep a one-shot `.v1-backup` and their own bytes;
  `--end` appends to the same plan row; the same rows as T5 left them (`ledgers/tracker.json`
  only) migrate to v2 with the plan decision beside them and T5's file byte-identical; a
  lifecycle opt-out written into the real rows survives into `tracker.opt_outs`; and a
  corrupt ledger refuses the write with the file byte-identical.
