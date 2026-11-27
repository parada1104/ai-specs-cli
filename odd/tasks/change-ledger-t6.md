# change-ledger-t6 (ODD)

Unit T6 of the change-ledger-v2 redesign: the worktree and plan stores extracted
from the tracker store, with the invariants T5 established (S3, S11, S14). Master
plan, design and audit live in `odd/tasks/change-ledger-v2.md` and
`odd/tasks/change-ledger-v2/`.

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

- T6.1 — Map the decisions the single store holds onto the ledger that owns each:
  closes and exemptions to the worktree ledger, plan exemptions/archive/workstream
  decisions to the plan ledger, and everything else staying in the tracker ledger.
  Produce the map in `## Log` with `path:line` evidence before splitting anything.
  Specs: S3. route: worker. commit: 18650df4 (the map, in `## Log`).
- T6.2 — Add `ledgers/worktree.json` and `ledgers/plan.json` as locations with the
  same invariants T5 established (`StoreLocation`, one-release fallback to the
  pre-split file, absorb-and-report, one-shot atomic backup) and route every read and
  write to the ledger that owns the row. Specs: S3, S11. route: worker.
  commit: af587978 (PR-1 half: the worktree ledger; the plan ledger is T6b).
- T6.3 — Delete the monolithic single-store writer, so one row cannot land in the
  wrong ledger, and keep the migration honest: each ledger reads its own share of the
  pre-split file. Specs: S3, S14. route: worker. commit: pending.
- T6.4 — Tests: each ledger's decisions land in its own file; a corrupt ledger leaves
  the others answering; the whole bind/cleanup behavior stays green (differential);
  the S14 contract test still fails on an outside writer. Specs: S3, S11, S14.
  route: worker. commit: pending.
- T6.5 — Validation: focused suites, both Go modules, `ledger_cleanup_flow.sh` before
  and after (the cleanup close path runs on every merge), the Python suites and a real
  flow over a copy of the real 14-row store. Specs: S3, S11, S14. route: worker.
  commit: pending.

## Allowed edit surfaces

- catalog/recipes/worktree-flow/gate/ledger/
- catalog/recipes/worktree-flow/gate/runner/
- tests/scenarios/ledger_cleanup_flow.sh
- internal/cli/ledgerstore_contract_test.go
- tests/

## Verification

| Spec | Evidence |
| --- | --- |
| S3 | (a) a close lands in `ledgers/worktree.json` and nowhere else; (b) a plan exemption and an archive decision land in `ledgers/plan.json`; (c) a binding, an opt-out and an adjudication land in `ledgers/tracker.json`; (d) no row is written to two files. |
| S11 | A v1-only store is read by all three ledgers (each taking its share), the pre-split file stays byte-identical, and an older binary's later rows are absorbed and reported — the T5 invariants, per ledger. |
| S14 | Every read and write happens through the CLI entrypoint; the contract test in `internal/cli` still names only the store package, the gate and the entrypoint. |

## Log

- **L1** — Mandate, verbatim (from `odd/tasks/change-ledger-t5.md` L1, which covers
  this unit): «Sigue con el plan maestro: T4 (aviso de Pi visible) y luego T5,
  T6… en orden, una unidad = un worktree nuevo desde development + un PR, con el
  mismo mandato (validar con flujo real y RED, revisión nativa antes del merge; si
  la revisión se traba en el paso de validación como en T2, me avisas con
  LISTO REVISAR-<unidad> y reviso yo; PR, merge, `ai-specs sync` del repo con su
  PR). Sin release hasta T12, salvo que una unidad lo necesite. Guarda en Engram
  antes de cada merge (el hook retira el worktree). Cuando el contexto pase de
  ~45 %, cierra en Engram y avísame con LISTO RELEVO para abrir sesión nueva.
  Avísame al cerrar cada unidad con una línea LISTO <unidad> (p. ej. LISTO T4).»
  Plus, verbatim, from the closing instruction of the T5 session: «Antes del merge:
  handoff en Engram y worktree de T6 con su rama y plan.»
- **L2** — Master plan entry for this unit, verbatim
  (`odd/tasks/change-ledger-v2.md`, T6): «T6 — Worktree and plan stores extracted,
  with the same invariants (S3, S11, S14).» with its scope
  (`.git/ai-specs/ledgers/{worktree,plan}.json`, moving close/exemption decisions to
  worktree and plan exemptions/archive/workstream decisions to plan; delete the
  monolithic writer), its tests (each ledger's decisions land in its own file; every
  existing cleanup/bind scenario stays green, differential; isolation on corruption),
  its risks (the cleanup close path is the one that runs on merges — a mistake there
  leaves merged worktrees behind; mitigate by running the existing
  `ledger_cleanup_flow.sh` before and after) and its size (~450 lines).
- **L3** — Inherited state, so T6 does not re-derive it. T5 (`b6e160dc`, PR #618)
  left the tracker ledger split with `StoreLocation{Path, Legacy}`
  (`catalog/recipes/worktree-flow/gate/ledger/location.go`):
  `TrackerLocation(common)` is the split file plus the pre-split
  `ai-specs/ledger/state.json` as a one-release fallback; `StoreAt(path)` is an
  explicit file with no fallback and no backup; `Load() (Store, StoreReport, error)`
  prefers the split file, reports a corrupt one and never falls back, absorbs the rows
  the split file lacks, counts the rows that differ in place, and reports an unreadable
  pre-split store; `Save(Store) (StoreReport, error)` writes the split file plus a
  one-shot atomic `state.json.v1-backup`; `StoreReport.Note` prints one line per
  problem and nothing on a clean path. The mutators (`ApplyWrite`,
  `CloseBranchItem`, `PersistDecision`) take the location, and
  `runner/ledger_cmd.go` resolves it once, prints the report and attempts the backup
  before any write. The S14 contract test lives in
  `internal/cli/ledgerstore_contract_test.go`. T5's own plan carries the traps in its
  Log (L9-L11); the ones that will bite T6: the pi-lens LSP binds the main worktree so
  its `undefined:` reports are stale (only `go build`/`go vet`/`go test` count); the
  gate is its own Go module under `catalog/recipes/worktree-flow/gate`; `tests/run.sh`
  aborts at its parity leg (28/35, `/private` TMPDIR, pre-existing) so the Python
  suites are run directly; a no-op write (`already-open`) carries no absorbed rows
  over, so probes need a real decision; and a merge retires the worktree, so continue
  from the main checkout.
- **L4** — State: the worktree `change-ledger-t6` exists on branch
  `feat/change-ledger-t6`, based on `development` at `b6e160dc` (the T5 squash merge of
  PR #618). Nothing implemented, nothing committed. T6.1 is exploration, not code: the
  map decides which ledger owns each decision, and a wrong map writes a row into the
  wrong file.
- **L5** — T6.1 map, with the evidence that decides the split
  (`catalog/recipes/worktree-flow/gate/`). One `Store{V, Items, OptOuts}`
  (`ledger/store.go:177`) where each `Item` mixes both capabilities: `status`,
  `Decision{close}` and the provider snapshot (worktree) share the row with
  `item_id`, `url`, `provider_id`, `native_type`, `state`, `exemption`, opt-outs and
  adjudications (tracker). Mutators: `applyWriteToStore` (`store.go:385`) for
  `open`/`bind`/`exempt`, `applyBranchBindToStore` (`:490`), `applyLinkToStore`
  (`:510`), `CloseItem` (`:440`), `CloseBranchItem` (`:538`), `PersistDecision`
  (`ledger/decide.go:58`), and the dead writers `PersistConflict`
  (`decide.go:122`) and `AppendDecisionToPrimary` (`store.go:855`). Cross-capability
  reads that the split must keep answering through a read-only contract:
  `corroboratedClosedItemID` matches **closed** rows by `item_id`
  (`runner/ledger_reconcile.go:222-243`), and the close report reads
  `store.LatestClosed` (`runner/ledger_cmd.go:172`). The plan ledger has **no row in
  v1**: plan state is `.ai-specs/plan-ledger.jsonl` with `workstream` and
  `workstream-end` events (`internal/planledger/planledger.go:18,23-24`, appended at
  `internal/cli/changes.go:282-286`, read at `internal/gates/writefacts.go:255`).
- **L6** — Decision: **separate by fields** (`Item` split into a worktree record and a
  tracker record joined by identity, plus a plan record; the two cross-capability
  reads become read-only contract calls). Chosen by Cody on 2026-11-27 as a technical
  decision inside the architecture Robert fixed (one ledger per capability, conversing
  by contract, never mixing state), because S3(a) "a close lands in
  `ledgers/worktree.json` and nowhere else" is unreachable while one row carries both
  capabilities. The same decision resolves the plan's internal contradiction in favour
  of the master plan: the files are `ledgers/{worktree,plan,tracker}.json`, so T3's
  note «T7 adds the other two ledgers over the same file» is superseded and the plan
  ledger file is T6's, seeded from the existing JSONL events. If the diff passes ~700
  lines, T6 ships as two ordered PRs.
- **L7** — Baseline before any change (evidence for the differential): at `b6e160dc`
  the gate module builds, vets and tests clean, the root module builds and vets clean,
  and `tests/scenarios/ledger_cleanup_flow.sh` passes 7/7 (4 merge strategies, the
  automatic post-merge hook, provider-side squash via post-checkout, local squash then
  pull). Full log: `/private/tmp/change-ledger-v2/t6/baseline.log`.
  The Python suites (`python3 -m unittest discover -s tests -p 'test_*.py'`) run 1933
  tests on the same untouched tree with exactly one failure,
  `test_git_pr_flow_recipe.GitPrFlowGoldenContentTests.test_skill_never_recommends_delete_branch`,
  because that skill's sentence is wrapped across a newline; it is pre-existing and
  must not be chased in T6.5.
- **L8** — Design fixed for both PRs (do not re-decide it; it is L6 applied to the
  map in L5):
  - **Join key**: the existing `Item.ID`. No new key, no new identity scheme.
  - `ledgers/worktree.json` = `{"v":1,"items":[{id, identity, status, decisions:[{at,
    checkpoint, kind:"close"}]}]}` — the worktree lifecycle and nothing else.
  - `ledgers/tracker.json` = today's `Store` shape **minus `status` and the close
    decisions**: `{v, items:[{id, identity, item_id, provider_id, native_type, url,
    state, provider, exemption, conflict, decisions:[open|link|opt-out|adjudicate]}],
    opt_outs:[…]}`.
  - `ledgers/plan.json` = `{v, items:[{change, decisions:[{at, kind, checkpoint,
    choice, note}]}]}` where kind is one of plan-exempt, archive, workstream,
    workstream-end; it is seeded from the existing `workstream`/`workstream-end`
    events of `.ai-specs/plan-ledger.jsonl` (T3), so no plan row is invented.
  - **One verb, one row**: `close` mutates only `worktree.json`; when the close
    carries a provider payload the snapshot fields (`state`, `provider`) are updated on
    the tracker row that already exists — the close row itself never lands in a second
    file. `bind`/`link`/`exempt`/`opt-out`/`adjudicate` mutate only `tracker.json`.
  - **Cross-ledger reads are read-only contract**: a composed `View{Worktree, Tracker,
    Plan}` exposes `Primary(key)`, `OpenItemsForBranch`, `LatestClosed`,
    `HasScopedOptOut`, `ProviderBacked`, opt-out and adjudication lookups; `Grade` takes
    the `View`, and `corroboratedClosedItemID` joins closed worktree ids with tracker
    rows by `id`. No mutator may write a ledger it does not own.
  - **Migration honesty**: each ledger reads only its own share of the pre-split file
    (`worktree`: id, identity, status, close decisions; `tracker`: everything else;
    `plan`: nothing, since v1 has no plan row). `StoreReport` is reused per ledger and a
    disagreement in a field a ledger does not own is not reported as a shadow. The
    pre-split file stays byte-identical and keeps its one-shot `state.json.v1-backup`.
  - **Two ordered PRs** (guarda de Cody, ~700 líneas): **PR-1** extracts the worktree
    ledger (three locations, the worktree store, close/cleanup routing, the `View`, the
    differential and per-ledger landing tests; T6.2 + the worktree half of T6.4);
    **PR-2** adds the plan store, deletes the monolithic single-store writer, removes the
    dead writers (`PersistConflict`, `AppendDecisionToPrimary`), finishes corruption
    isolation, the S14 contract and T6.5 (T6.3 + the rest of T6.4/T6.5).
- **L9** — PR-1 shape and the size exception. PR-1 (worktree ledger extracted) measures
  ~987 production lines added / 438 deleted including the two new files, plus ~1 020
  lines of test adaptation, because a Go shape change does not compile in halves: the
  fixtures that seed the old single store must move in the same change or nothing
  builds. Cody's ~700-line guard is honoured by the two-PR order (PR-1 the split, PR-2
  the plan store, the deletion of the monolithic writer, the dead writers and full
  isolation), not by halving the split itself; the review workload is named in the PR
  body instead of hidden. Two clarifications of L8, settled by the parent and recorded
  here so the reviewer sees the reasoning: (1) `open` — and a bind or exempt that has to
  open — writes one row per ledger, because identity and status are the worktree's
  columns while the provider record and the open decision are the tracker's; S3(d) is
  about the row that owns a decision, and a close never touches a tracker decision.
  (2) A close carrying a provider payload updates `state`/`provider` on the tracker row
  that already exists; it never adds a tracker row, so the close itself stays
  worktree-only. Also approved as a parent technical call, inside this unit: the two
  `internal/cli` fixtures (`cli_test.go`, `trackercardgate_test.go`) are adapted to seed
  both ledgers, since they call the removed `Store.OpenItem`/`Items` API and the root
  module cannot `go vet` without them; no production code under `internal/` changes.
- **L10** — Delivery split, decided before the first merge. PR-1 (`af587978`, the
  worktree ledger) and T6b (PR-2: the plan store, deleting the monolithic writer and
  the dead writers, full corruption isolation, the S14 contract, T6.5) are two ordered
  PRs, so the second one is built in its own worktree from `development` once PR-1 has
  merged and the cleanup hook has retired this one. That keeps one unit per worktree
  and per PR, and avoids a stacked branch nobody asked for. Before each merge: handoff
  in Engram. Before the last one: the T7 worktree from `development` with its branch
  and plan. PR-1's evidence: gate module and root `go build`/`go vet` clean,
  `ledger_cleanup_flow.sh` 7/7 (identical to L7's baseline), `-race` clean on the
  ledger package, `gofmt` clean, and the real-flow probe over a copy of the real 14-row
  store showing close decisions 15 in `worktree.json` / 0 in `tracker.json`, link
  decisions 0/15, `status` only in the worktree ledger and `item_id` only in the
  tracker, 14/14 historical rows still answering by identity, and the pre-split file
  byte-identical (`63535f2c…`). Log: `/private/tmp/change-ledger-v2/t6/`.
- **L11** — Confirmed blockers on `f946d882` (independent verification; all real). The
  root cause is one missing migration step: T5's `ledgers/tracker.json` **is** the
  immediate predecessor of the field split, and it still carries `status` and the close
  decisions. T6 treated only `state.json` as its legacy source, so a repository upgraded
  straight from T5 loses its lifecycle to the candidate:
  1. A baseline T5 `bind` writes the mixed `tracker.json`; the candidate then grades
     `needs-item` and an explicit close exits 2 «ledger: no open item for identity»,
     because only `state.json` is a worktree fallback and nothing reads the T5 row's
     status.
  2. T5's close decisions survive inside the tracker file, so a close lives in
     `tracker.json` and the worktree ledger never sees it (S3(a) violated on upgrade).
  3. An immediate candidate rebind duplicated the item id: `uniqueItemID` consults only
     the worktree file, which is empty in exactly that upgrade state.
  4. `--store …/ledgers/worktree.json` aliases both ledgers onto one file, so a
     successful bind silently loses the lifecycle.
  5. The `legacyOnly` downgrade turns a corrupt v1-only store from «unevaluable, bind
     exits 2» into a successful synthetic bind — it masks corruption.
  6. The shell scenario's fallback now requires *zero* closes in the single legacy file
     where it previously required a close, so it can no longer detect a regression.
  The fix, decided here so the correction does not invent a second design: **the
  worktree ledger absorbs its share from both legacy sources — `state.json` and T5's
  `tracker.json`** — and a close is never dropped from `tracker.json` until
  `worktree.json` demonstrably holds it. Concretely: the tracker ledger reads its own
  file normalized (status and close decisions filtered in memory); the worktree ledger
  absorbs the status/close share of both legacy files; and a write that would persist
  the tracker rows without their close decisions first persists the worktree rows that
  carry them, and refuses (fail closed) rather than dropping a close it cannot place.
  Ids are unique across **both** ledgers. `--store` names one ledger and derives the
  other canonical file without ever aliasing one file onto itself. The `legacyOnly`
  downgrade is removed so a corrupt store stays unevaluable and fails closed. The
  scenario's fallback keeps its assertion strength. Validation must include the
  upgrade the probe missed: a store pair written by the T5 shape (mixed `tracker.json`
  plus `state.json`), driven through bind, close and grade by the candidate.
