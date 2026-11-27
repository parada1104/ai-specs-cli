# change-ledger-t5 (ODD)

Unit T5 of the change-ledger-v2 redesign: the Tracker store extraction with a
one-version v1→v2 double read (S11, S14). Master plan, design and audit live in
`odd/tasks/change-ledger-v2.md` and `odd/tasks/change-ledger-v2/`.

## Specs

- **S11** — Store v2 with one-version double read; the 14 rows stay as history.
  Verbatim: «store v2 con doble lectura una versión, las 14 filas quedan como
  histórico».
- **S14** — Everything goes through the CLI entrypoint; the on-disk layout is not
  a contract. Verbatim: «cada ledger con su propio archivo está bien siempre que
  todo pase por el entrypoint del CLI de ai-specs (nadie lee ni escribe esos
  archivos directamente)».

## Tasks

- T5.1 — Map the v1 store and the 14 rows before touching anything: every reader
  and writer of `<git-common-dir>/ai-specs/ledger/state.json`, the shape of the
  tracker rows, and which decisions already live there. Produce the map in
  `## Log`, with `path:line` evidence. Specs: S11, S14. route: worker.
  commit: cd55bc04.
- T5.2 — Extract the tracker store as the first split store
  (`<git-common-dir>/ai-specs/ledgers/tracker.json`), written only through the
  CLI, keeping the remaining decisions wherever they live today. Specs: S11,
  S14. route: worker. commit: cd55bc04.
- T5.3 — The one-version double read: a migration reader that reads both v2 and
  `ledger/state.json` v1, prefers v2 when it exists, and hands a v1-only store
  forward on the first decision that lands. The 14 historical rows stay
  byte-identical. Specs: S11. route: worker. commit: cd55bc04.
- T5.4 — Tests: a v1 fixture is read and re-written as v2 once a decision lands;
  the 14 rows survive byte-identical as history; corrupting one store leaves the
  others answering; a contract test that no package outside `internal/cli`
  writes those paths. Specs: S11, S14. route: worker. commit: cd55bc04.
- T5.5 — Validation: focused suites, `./tests/run.sh` and `./tests/validate.sh`,
  and a real flow over a copy of the real 14-row file (never the original).
  Specs: S11, S14. route: worker. commit: cd55bc04.

## Verification

| Spec | Evidence |
| --- | --- |
| S11 | (a) a v1-only store still answers, and the first decision that lands writes a v2 tracker store while leaving v1 readable for this version; (b) the 14 historical rows survive byte-identical in the v2 store, as history and not as live decisions; (c) a corrupt v2 tracker store does not take the other ledgers down, and a corrupt v1 fallback is reported, never silently treated as empty. |
| S14 | Every read and write of the ledger paths happens inside `internal/cli` (or the packages it calls for the store, with no other caller); nothing outside the entrypoint opens them. Proven by a contract test that fails when a package outside the entrypoint touches those paths, not by a comment. |

## Log

- **L1** — Mandate, verbatim (from `odd/tasks/change-ledger-t4.md` L1, which names
  this unit): «Sigue con el plan maestro: T4 (aviso de Pi visible) y luego T5,
  T6… en orden, una unidad = un worktree nuevo desde development + un PR, con el
  mismo mandato (validar con flujo real y RED, revisión nativa antes del merge; si
  la revisión se traba en el paso de validación como en T2, me avisas con
  LISTO REVISAR-<unidad> y reviso yo; PR, merge, `ai-specs sync` del repo con su
  PR). Sin release hasta T12, salvo que una unidad lo necesite. Guarda en Engram
  antes de cada merge (el hook retira el worktree). Cuando el contexto pase de
  ~45 %, cierra en Engram y avísame con LISTO RELEVO para abrir sesión nueva.
  Avísame al cerrar cada unidad con una línea LISTO <unidad> (p. ej. LISTO T4).»
  Plus, verbatim, from the same message: «crea el worktree de la siguiente unidad
  (T5) desde development con su rama y plan» and «No retires worktrees ni ramas a
  mano».
- **L2** — Master plan entry for this unit, verbatim
  (`odd/tasks/change-ledger-v2.md`, T5): «T5 — Tracker store extraction with
  v1→v2 double read (S11, S14). route: worker. commit: none.» with its scope
  (`.git/ai-specs/ledgers/tracker.json` as the first split store, written only
  through the CLI; a migration reader that also reads `ledger/state.json` v1;
  decision routing (bindings, opt-outs, adjudications); the 14 historical rows
  stay untouched), its tests, its risks (silent data loss on migration; mitigate
  with a one-shot backup path and a migration test on a copy of the real 14-row
  file), and its size (~600 lines).
- **L3** — Located before starting, with evidence, so T5.1 has a starting point
  instead of a guess:
  - the durable store is `<git-common-dir>/ai-specs/ledger/state.json`
    (`catalog/recipes/worktree-flow/gate/ledger/store.go:183-185`, "StorePath"),
    and it is written atomically through a `state.json.tmp.*` file
    (`store.go:209`, "SaveStore");
  - the ledger package already separates concerns the split can follow:
    `decide.go`, `identity.go`, `reconcile.go`, `store.go`, `verdict.go`,
    `witness.go`, `worktree.go` plus their tests and `testdata/`;
  - a second ledger package already exists as precedent to imitate:
    `internal/planledger/planledger.go`.
- **L4** — State: the worktree `change-ledger-t5` exists on branch
  `feat/change-ledger-t5`, based on `development` at `2204f43f` (the T4 merge).
  Nothing implemented, nothing committed. T5.1 is the first task and it is
  exploration, not code: the map must be written down before the extraction
  starts, because a wrong map here loses the 14 rows.

- **L5 — T5.1, the map (write it before touching anything).** The durable store is
  one record kind, not several: `Store{V, Items, OptOuts}` (`gate/ledger/store.go:176-180`)
  with `Item{id, identity{common_dir,branch,change}, status, item_id, provider_id,
  native_type, url, state, provider, exemption, conflict, decisions[]}`
  (`store.go:139-151`), `Decision{at, checkpoint, scope, kind, choice, note}`
  (`store.go:116-123`) and `ScopedOptOut` (`store.go:130-136`). Decision kinds are
  `adjudicate|opt-out|open|close|link` (`store.go:30-34`). **No binding, worktree
  identity or stamp lives here**: the binding is `ledger/witness.json`
  (`ledger/witness.go:42-48`), read by `ReadBinding` (`witness.go:61`), written by
  `shared/witness.go:142`. Path: `StorePath` = `<common>/ai-specs/ledger/state.json`
  (`store.go:184`), read by `LoadStore` (`store.go:191`), written atomically by
  `SaveStore` (`store.go:211-263`: temp `state.json.tmp.*`, fsync, rename) under
  `withStoreLock` (`store.go:878`, flock + 5×20 ms → `ErrLockTimeout`). A missing
  file is an empty store; a corrupt or unknown-version file is `*StoreCorruptError`
  and is never repaired (`store.go:191-206`).
  Readers/writers, all inside the worktree-gate module: `ApplyWrite` (`store.go:333`),
  `CloseBranchItem` (`store.go:538`), `PersistDecision` (`decide.go:58`), the dead
  `PersistConflict` (`decide.go:120`) and `AppendDecisionToPrimary` (`store.go:855`)
  — no caller for the last two, design `design.md:91`; `runner/ledger_cmd.go` reads
  at `:100,104,131,156` and writes at `:367,390`; `runner/ledger_reconcile.go:222`
  reads; `runner/cleanup.go:324` closes. Nothing outside the gate module touches the
  file in non-test code: `internal/doctor/checks_tracker.go:47,78` reads the witness
  only, and `:258` shells out to `worktree-gate --ledger`. The seam into the
  entrypoint is an in-process import (`go.mod:18-20`, `replace ./catalog/recipes/
  worktree-flow/gate`): `internal/cli/cli.go:283` (`runner.Run`),
  `trackercardgate.go:475,577,609`, `planbuildgate.go:226,286`,
  `worktreecleanup.go:129`. **The 14 rows are distinguishable only by
  `status=="closed"`; nothing marks a row as historical.** The real file is not in
  the tree: `<main checkout>/.git/ai-specs/ledger/state.json` (15,243 bytes, v1,
  14 items, all closed, 0 opt-outs) and design `audit.md:23` records the same. The
  v2 layout is dictated at `odd/tasks/change-ledger-v2/design.md:226-232`
  (`.git/ai-specs/ledgers/{worktree,plan,tracker}.json`, same flock+temp+fsync+rename,
  "v1→v2 con doble lectura una versión; las 14 filas quedan como histórico"), and
  `open question 6` (`design.md:281`) asks whether those rows are frozen or
  annotated — T5 freezes them as-is, which is what S11 requires.
- **L6 — T5.2/T5.3, the split and the double read (design).** A new
  `gate/ledger/location.go` owns the layout: `TrackerStorePath(common)` =
  `<common>/ai-specs/ledgers/tracker.json`; `legacyStorePathFor` maps any path under
  a `ledgers/` directory back to `<common>/ai-specs/ledger/state.json` (one shared
  v1 fallback for every split ledger, and no fallback for an explicit `--store`
  path); `LoadTrackerStore` prefers the split file, reports a corrupt split file
  instead of falling back to the older one (history must never come back as live
  state) and reports a corrupt fallback instead of reading it as empty;
  `SaveTrackerStore` writes the split file and keeps a one-shot
  `state.json.v1-backup` of the pre-split rows before the first handoff. The schema
  version is unchanged (`StoreVersion` stays 1: the shape did not change, only the
  file), so no existing store or fixture gains a new meaning. The six store calls
  in `ApplyWrite`/`CloseBranchItem`/`PersistDecision` and the four resolution points
  (`runner/ledger_cmd.go:100,104,131,156`, `runner/ledger_reconcile.go:222`,
  `runner/cleanup.go:324`) now go through that pair. S14 holds because the path is
  constructed only in the store package: `internal/cli/ledgerstore_contract_test.go`
  fails when any other package names the store API or the on-disk names (RED proven
  by adding `ledger.TrackerStorePath("probe")` in `internal/doctor`, which the test
  named as two offenders, and removing it again).
- **L7 — Validation evidence (T5.5).**
  - `go vet ./...` and `go test ./... -count=1` green in both modules
    (`ai-specs.dev/worktree-gate/{ledger,runner,shared}` and the root module,
    including `internal/cli` and the four updated fixture helpers).
  - New `gate/ledger/location_test.go`: 6 tests covering the split path, the legacy
    read before the first decision, the handoff with byte-identical historical rows
    (all 14 still closed, the opt-out only in v2), the split-file preference, a
    corrupt split file that never resurrects v1, a corrupt fallback reported as
    corrupt, and an explicit `--store` path with no fallback and no backup.
  - Real flow `/private/tmp/change-ledger-v2/t5/flow.sh` over a **copy** of the real
    14-row store, baseline binary (base `2204f43f`) versus branch binary: RED — the
    baseline creates no `ledgers/tracker.json` and keeps writing the pre-split file;
    GREEN — the first decision lands in `ledgers/tracker.json`, the pre-split file
    stays byte-identical to the original, the one-shot backup holds the original
    rows, the 14 rows are a byte-identical prefix of the split store and all still
    closed, a later close keeps writing only the split store, a corrupt split store
    fails the write closed (exit 2, path named) and fails the grade open
    (`store-corrupt`), the plan ledger keeps answering meanwhile, a corrupt
    pre-split fallback is named instead of read as empty, and the real repository
    store is byte-identical at the end. `FLOW OK`.
  - Docs that state where the store lives were updated (`README.md`,
    `docs/capabilities.md`, `docs/runtime-hooks.md`, the two recipe READMEs,
    `commands/trello-workflow.md`, `skills/trello-mcp-workflow/SKILL.md`), keeping
    the witness at `ledger/witness.json`. Not touched on purpose: the canonical
    `openspec/specs/tracker-ledger/spec.md:113` (the promoter owns canonical specs;
    this change's delta carries the new requirement) and `CHANGELOG.md` history.
