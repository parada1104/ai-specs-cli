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
- **L12** — Second batch from the same verification. L11's six blockers stand; these are
  additional and some are worse, so the correction covers both.
  1. **A failed two-file write persists half the mutation.** Baseline `bind` fails
     without saving a row; the candidate fails *after* saving 1 worktree / 0 tracker
     rows, and the retry dies with «no open item for identity». A lock plus two atomic
     renames is not a transaction, and it contradicts the "every failure persists
     nothing" the package documents. Fix: a multi-file save snapshots the pre-write
     bytes of every file it will touch and restores them if a later save fails,
     reporting the original error (and any restore failure); the caller still exits
     non-zero. A snapshot-bearing close gets the same treatment, so the close cannot
     survive while its snapshot does not.
  2. **A close must not dirty the tracker and must not vanish from stdout.** A bare
     explicit close from a valid v1-only store created both files (tracker marked dirty
     with no snapshot change) and its stdout omitted the close decision baseline
     returned, because the reported item is built from the tracker record. Fix: the
     tracker is dirty only when `state`/`provider` actually change; the reported item's
     `decisions` are the tracker's decisions joined with the worktree row's close
     decisions in stored order, and the close decision keeps its place in the stdout
     contract.
  3. **One lock per directory, not per file.** Two distinct `--store` overrides in one
     directory share a lifecycle file but take different locks, so they do not exclude
     each other. Fix: the lock file is canonical for the directory, so every pair in it
     serializes.
  4. **`--store` resolves by canonical name.** An override naming `worktree.json`,
     `tracker.json` or `state.json` maps to the ledger that owns that name and derives
     the others; it never aliases two ledgers onto one file, and it never rewrites the
     pre-split file. Any other name keeps T5's meaning (the tracker ledger's file) with
     the worktree sibling beside it.
  5. **Scenario step 7 cannot pass.** In a `state.json`-only layout `$TRACKER` and
     `$LEDGER` are the same file, so the step demands the same close count be both
     positive and zero. Fix the fallback so each layout asserts one coherent condition,
     and keep proving the close in the file that owns it.
  6. **Tests were weakened and must be restored, not adapted.** The historical-row byte
     comparison became a decoded comparison judged by the production projection (its own
     oracle): restore a byte-level assertion that does not use the code under test as
     the judge. The failure, collision, idempotence and cleanup dry-run byte checks
     snapshot only the tracker file: they must cover the lifecycle file too. The
     temp-residue test still globs `state.json.tmp.*` instead of the names actually
     written. The backup-once test does not change the source bytes between attempts, so
     an overwrite with the same bytes would pass unnoticed. No test may be deleted or
     loosened to make this pass; the verifier already supplied two ready regression
     tests for the T5 upgrade and the failed-bind orphan, and both must be in.
  7. **Not pinned and must now be pinned:** the T5-produced upgrade state; a second-write
     failure and its retry; alias independence of two overrides; unchanged close stdout
     decision history; bare-close tracker non-creation; and the migration/backup warnings
     the cleanup path currently discards.
- **L13** — Round-2 blockers, and the reason there are three rounds of them: L11/L12
  specified *patches* to a migration that was still conditional, so each layout or entry
  point had its own hole. Reproduced: (a) a default candidate `bind` on a T5 mixed
  `tracker.json` holding only an open row normalises its status away without creating
  `worktree.json`, and the next close exits 2; (b) `--store tracker.json` drops real T5
  closes; (c) `Worktree.json` (case difference) aliases the two ledgers and `STATE.JSON`
  rewrites the pre-split file; (d) naming `.ledger.lock` makes a fresh override fail or
  renames the held lock inode. The fix is one invariant plus a matrix, not four patches.
  **Invariant M (migration completeness).** Before this build writes either ledger file,
  everything the sources hold is represented across the pair: every status and every
  close decision present in any readable source is present in `worktree.json`. If that
  cannot be guaranteed — typically because `worktree.json` cannot be written — the write
  fails closed and nothing is persisted (L12.1's rollback). A tracker-only mutation still
  materialises the worktree file when a source holds lifecycle state it does not.
  **Source symmetry.** The worktree ledger reads its share of `worktree.json`, of the
  tracker file (whatever shape it has, including T5's mixed one) and of the pre-split
  `state.json`; the tracker ledger reads `tracker.json` and, for repository-derived or
  pre-split-named locations, `state.json`. Symmetry is what makes the entry point
  irrelevant: `--store tracker.json` and a repository-derived run must see the same
  sources, so (a) and (b) cannot differ.
  **Naming.** Canonical names match case-insensitively (APFS/HFS+ make `Worktree.json`
  the same file), so a case variant resolves to the ledger that owns it instead of
  aliasing; the lock file and `*.tmp.*` names are never accepted as a store and are never
  renamed or used as a restore target, so the held inode stays the lock.
  **Proof: a matrix, not an example.** One table-driven test over every layout (only
  `state.json`; only a T5 tracker file with closes; only a T5 tracker file with a single
  open row; the pair; the pair plus `state.json`) x every entry point (repository-derived,
  `--store tracker.json`, `--store worktree.json`, `--store state.json`, `--store <other
  name>`) x every operation (grade, open, bind, close), asserting invariant M and that the
  pre-split file is byte-identical. That matrix is the deliverable: it is what makes the
  next review cheap, and its absence is why two verification rounds still found holes.
- **L14** — Decision: **logical split in one file; physical separation later.** Chosen by
  Cody on 2026-11-27 as a technical decision inside the architecture Robert fixed
  (S14 says the layout is not a contract: «cada ledger con su propio archivo está bien
  siempre que todo pase por el entrypoint del CLI»; Robert's rule — one ledger per
  capability, no mixed state, everything through the CLI entrypoint — holds with
  per-capability sections and separated writers), after three verification rounds kept
  producing new defects of one class: the combinatorial surface of layouts x entry points
  x operations x odd filesystems on a two-file migration.
  **Layout.** One file, `<common>/ai-specs/ledger/state.json`, `{"v":2,"worktree":{"items":
  […]}, "tracker":{"items":[…],"opt_outs":[…]}, "plan":{"items":[…]}}`. Each capability
  writes only its own section, so no capability can write another's row.
  **Migration, on read, never two halves.** T5 already ships `ledgers/tracker.json` in
  `development` with no release behind it, so both predecessor sources are absorbed
  **unconditionally**, not as a one-release window: a v1 file at the v2 path is normalized
  into sections in memory (worktree share and tracker share, the same functions that already
  exist), T5's `ledgers/tracker.json` contributes the rows the v2 file lacks, a one-shot
  atomic backup of every source is kept, and the first write rewrites the v2 file. No row
  may be dropped and no source is ever rewritten. Only `fs.ErrNotExist` means absent: any
  other read error is corruption, the ledger is unevaluable, and writes fail closed — a
  symlink loop or a permission error can never become «empty and writable».
  **Deleted with the pair machinery**: `Inherit`, the two-location `Ledgers`, the pair save
  with its best-effort rollback, the per-directory lock, and `--store`'s derived sibling.
  `--store` names THE file; one atomic rename per write; the lock is `<store>.lock`, the
  name T5 already uses, so two versions of the binary exclude each other.
  **Decision history** is the tracker's decisions in stored order followed by the worktree
  section's close decisions in stored order — the close terminates the row, so it is last
  by construction and no timestamp sorting is involved (that sorting is what printed
  `close,open`).
  Ids are unique across sections. Kept and reused: `WorktreeItem`, `WorktreeStore`, the
  share functions, `View`, the per-capability routing, the L11 tests (adapted) and the
  scenario's per-capability assertions. `ledgers/{worktree,plan}.json` becomes its own
  future unit with an explicit `ai-specs ledger migrate` verb and preconditions; nothing in
  T6a may create or read those files as primary state.
- **L15** — L14 applied, with three consequences settled here so they are not
  re-litigated. (1) The `plan` section is emitted as `{"items":[]}` — shape only, no
  location, no writer, no migration — because L14 declares the layout and T6b fills it;
  the key must not be omitted and later invented. (2) The pair-specific regressions of L11
  (refusing to drop a close «it cannot place», alias independence, the per-directory lock,
  the two-file rollback) become vacuous on one file: a single atomic rename cannot place a
  row in a file it does not own. Their **intent** is preserved as one-file assertions (no
  close decision in the tracker section, ids unique across sections, a failed write
  persists nothing) and the mechanisms are dropped rather than kept as dead tests
  asserting a deleted design. (3) The verification row that read «a corrupt ledger leaves
  the others answering» no longer has a subject: with one file there is no «others». It is
  replaced by «a corrupt file is unevaluable and every write fails closed with the file
  left byte-identical» — the neighbouring contract, pinned by test.
  Delivery: the whole T6a change exceeds the reviewable size in one commit, so it ships as
  an ordered pair of PRs — the first carries the v2 single-file production change plus the
  full gate-suite adaptation (green), the second carries the scenario's per-section
  assertions, the `internal/cli` seeds and the three real-store probes as its evidence.
- **L16** — T6a delivered as `abf4e50e` (net against `b6e160dc`: 22 files, +2941
  /-1548). Delivery shape decision: **one PR, not two**. A two-PR split was considered
  and rejected on evidence, not preference: a Go API change does not compile in halves, so
  the runner and `internal/cli` fixtures (the root module's `go vet`) and the scenario's
  per-capability assertions must ride with the shape change or nothing builds. What is
  splittable is capability, and capability here is one change. The size exception is
  therefore stated in the PR body instead of hidden by a fake slice.
  Evidence at delivery: gate module 394 tests with `-count=1`, root `go build`/`go vet`
  clean, `internal/cli` ledger and tracker-card tests, `ledger_cleanup_flow.sh` 7/7
  identical to the base, `-race` and `gofmt` clean, and three parent-run probes over
  copies of the real 14-row store (v1 only; v1 plus T5's mixed tracker file; a T5 file
  holding a single open row) proving the no-row-lost, close-in-the-worktree-section,
  byte-identical-predecessor and backup properties. Native review remains impossible at
  this candidate size (`lens_context_budget_exceeded` with every lens set tried), so it
  goes to Robert with `LISTO REVISAR-T6`. Remaining known gaps, named rather than buried:
  the shadow advisory repeats on every invocation while a predecessor file exists (the
  master plan assigns the report-once behaviour to T7/T8), and a concurrent older binary
  is excluded by the shared `<store>.lock` name but was not exercised.
- **L17** — Cody's independent review of T6a: **no aprobar todavía**. Two blockers and
  three mediums, all reproduced with real binaries over copies of the 14 rows. Fix list,
  in the reviewer's own terms:
  - **ALTO-1 (blocking) — the newest form must win.** With `Path` v1 and T5's
    `ledgers/tracker.json` present, the stale v1 was taken as «the file's version» and T5's
    newer file became a shadow, so a row T5 re-linked and closed came back `open` with the
    old card, the link was lost, and the next close duplicated (`[open,link,close,close]`).
    T5 never writes `state.json`; its tracker file is the newest superset. Fix: when `Path`
    is v1 and a migration source exists, the base is the **normalised tracker file** (T5
    semantics) and v1 only contributes the rows it lacks; when `Path` is already v2, the
    current order stands. The exact scenario 0.28.0 `bind` card-A -> T5 relink card-B and
    `close` -> T6a must be a test: grade sees `closed`/card-B with
    `[open,link,link,close]`, and a further close must not duplicate.
  - **ALTO-2 (blocking) — an absorbed close closes the row.** A close is terminal (D17),
    so any row carrying one is `closed`, whatever the source order; today the close is
    added and the status left behind, which is the same «allow on an already closed item»
    symptom.
  - **ALTO-3 (blocking) — the Python suites are red.** `tests/test_tracker_ledger_parity.py`
    and `tests/test_worktree_cleanup.py` read T5's layout directly; base 49/49, branch
    4 FAIL + 1 ERROR. They must read through the CLI or the new layout, and the full
    Python suite runs before the commit — the L16 evidence line claimed them without
    running them, which is my error, not the reviewer's.
  - **MEDIO-2 — a stale backup and a write that proceeds without one.** The one-shot
    backup is skipped because T5 already made one (14 rows) while v1 had grown to 15, and
    `Save` writes even when the backup failed while the message claims «untouched». Fix:
    back up the bytes actually being replaced (fresh content or a name of its own) and fail
    closed when the backup cannot be written; the message must not claim anything false.
  - **MEDIO-3 — a corrupt migration source must not invalidate the ledger.** A corrupt
    `tracker.json` today makes the whole ledger unevaluable, so cleanup's close fails and
    merged worktrees stay, unrecoverable because T6a never rewrites that file. Restore
    T5's semantics: report the unreadable source and let the v2 file answer.
  - **LOW, fix now because they are cheap:** a row differing in both sections is counted
    twice (reported «2 differ» for one row); the lock comment claims T5 exclusion when the
    real exclusion is with 0.28.0 (`state.json.lock`) and T5 needs none because it never
    writes the same file; `WorktreeDecision` drops `choice`/`note`/`scope` from a close.
  - **Do not touch MEDIO-1.** v2 overwriting `state.json` means an installed 0.28.0 can no
    longer read it: that is Robert's compatibility decision, not mine. Current state stays
    and the options are described to him.
- **L18** — L17 answered in `860a0c77`. Every finding fixed with its own RED test against
  `161c0d70`, and MEDIO-1 left untouched by instruction. Parent-verified evidence, not
  only the writer's: gate module 401 tests with `-count=1`; root `go build`/`go vet` clean;
  `internal/cli` ledger and tracker-card tests; `ledger_cleanup_flow.sh` 7/7; `gofmt` and
  `-race` clean; the **full** Python suite at 1933 tests with one failure, one skip and two
  expected failures — identical to the base, and that failure is the pre-existing
  `test_skill_never_recommends_delete_branch`; plus a parent-run probe of Cody's exact
  ALTO-1 scenario (v1 open card-A, T5 closed card-B with `[open,link,link,close]`) where
  grade finds no primary, the latest closed row carries card-B with that history, the
  extra close reports `already-closed`, and the T5 file stays byte-identical.
  Deliberate consequence of «the newest form wins», stated for the reviewer rather than
  discovered later: a row 0.28.0 closed that T5 left open stays open, because T5 wrote
  later. **MEDIO-1 remains open and is Robert's**: v2 lives at `ledger/state.json`, so an
  installed 0.28.0 stops reading the ledger after the first v2 write — `bind` exits 2
  without damage, `grade` falls to `unevaluable` with exit 0 under its inherited
  fail-open, and `worktree-cleanup` stops removing merged worktrees. Two options:
  (a) keep it, accepting that a repository still running the old binary loses automatic
  cleanup until it upgrades, or (b) let v2 live at a new path (e.g. `ledger/ledger.json`)
  and treat `state.json` as a read-only predecessor, exactly as T5 did, which preserves
  the «never rewrite an older binary's file» rule at the cost of carrying a retired file
  as a read-only source until it is dropped.
