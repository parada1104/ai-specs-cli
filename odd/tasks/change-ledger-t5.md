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
  `feat/change-ledger-t5`, based on `development` at `2204f43f` (the T4 merge), implemented and corrected in L5-L8.
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
  - New `gate/ledger/location_test.go`: 8 tests covering the split path, the legacy
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
- **L8 — Native review and the bounded correction.** Lineage `review-3eeec8276ce92655`
  (four lenses plus the provider refuter, tier high by `escalate` item 1) returned
  `correction_required` with two findings of the same defect, both `introduced` and
  deterministic: R1-001 (BLOCKER) and R3-001 (CRITICAL). The legacy fallback was
  inferred from the path, so **any** store named inside a directory called
  `ledgers` was treated as the default split store: an explicit
  `--store <dir>/ledgers/<name>.json` read and copied the sibling pre-split rows
  into the requested file, and a corrupt sibling could break an otherwise valid
  explicit-store operation. Correction in `f5d48ae9`: `StoreLocation{Path, Legacy}`
  carried by the caller, with `TrackerLocation(common)` (split file plus the
  one-release fallback) and `StoreAt(path)` (explicit file: no fallback, no
  backup); `Load`/`Save` moved onto the location, the one-shot backup is written
  atomically, and `legacyStorePathFor`/`LoadTrackerStore`/`SaveTrackerStore` are
  deleted, so no path can grant itself a fallback. The mutators, the runner
  resolution (which now uses `StoreAt` only when `--store` is given), the
  reconcile read and the cleanup close all take the location; the S14 contract
  test also guards `TrackerLocation(`/`StoreAt(`; two docs that had been left
  naming a nonexistent `ledgers/state.json` now name `ledgers/tracker.json`; the
  vacuous `.backup` extension assertion became the exact `state.json.v1-backup`
  name, and `TestExplicitStoreUnderALedgersDirKeepsNoLegacyFallback` is the RED
  probe (`location_test.go:292` "items = 14: pre-split rows were copied into the
  explicit store" under the restored inference, GREEN after reverting it). The one
  call site outside the worker's surfaces,
  `internal/cli/trackercardgate_test.go:111`, was fixed by the parent as
  `ledger.TrackerLocation(common).Load()`. Evidence after the correction: `go vet`
  and `go test ./... -count=1` green in both modules, and the real flow re-run with
  16 checks green, including the explicit `--store` case that no longer inherits
  the pre-split rows.
- **L9 — Where T5 stopped (needs the reviewer).** The targeted-validation slot
  (`review.capture-validation`, `provider_targeted_validator`) refuses the
  correction: «actual correction is 313 changed lines, exceeding the frozen budget
  of 200» — the correction's diff against the original candidate is larger than
  the frozen logical correction budget, so the bounded correction cannot be
  validated inside this lineage, and a fresh STATUS reoffers the same slot. That is
  the validation step stuck exactly as in T2, so nothing was pushed, no PR was
  opened and nothing was merged. Also true of this environment: `gentle-ai-verify`
  cannot run in a linked worktree (`probesRun: 0`, "unsupported repository:
  ordinary single-root Git directory required"), so the independent verification is
  the parent's; and `./tests/validate.sh` fails only in its parity leg (28/35
  fixtures, the `/private` TMPDIR normalization signature T3 already recorded as
  pre-existing, identical on a clean base copy). Non-blocking review findings
  recorded for later work: R3-002 (component-wise path construction escapes the
  contract guard), R4-rollback-divergence (a rolled-back binary reads only the
  pre-split store and misses post-handoff decisions — the accepted fix-forward-only
  limitation of a one-version double read), R2-contract-test-broad-patterns
  (substring matching), and the dead `PersistConflict`/`AppendDecisionToPrimary`
  writers T6 removes.
- **L10 — The Python suites and the whole-suite picture.** Two suites pinned the
  pre-split layout and failed after the split, so they were moved to the
  authoritative file without weakening a single assertion: the parity corpus
  (`tests/test_tracker_ledger_parity.py`) still seeds `ledger/state.json` — that is
  what exercises the fallback — and now pins from `ledgers/tracker.json`, reading
  the seeded file only when no split store exists (the CLI's own double read), with
  residue allow-lists for both directories (`state.json.v1-backup` included, since
  the one-shot backup is by design); `tests/test_worktree_cleanup.py` reads the
  closed item from the split store and adds the assertion it lacked — after cleanup
  closes it, the pre-split `state.json` is byte-identical to what the fixture
  seeded. RED before the edit: 12 parity failures and 1 cleanup failure; GREEN
  after: `test_tracker*.py` 70 ok, `test_worktree_cleanup*.py` 43 ok,
  `test_ledger_mode_config.py` 18 ok, `test_doctor_tracker_card.py` 21 ok.
  Whole picture: `go vet` + `go test -count=1` green in both Go modules;
  `python3 -m unittest discover -s tests -p 'test_*.py'` = 1933 tests, 1 failure,
  1 skipped, 2 expected failures, where the only failure
  (`test_git_pr_flow_recipe…test_skill_never_recommends_delete_branch`, a skill-text
  phrase) fails **identically on the untouched base tree** and this branch has zero
  diff lines in that skill and that test — pre-existing. `./tests/run.sh` cannot
  complete because `tests/parity/run.py` fails 28/35 fixtures with the
  `/private<TEMP>` normalization; that too is identical on the base tree
  (`git archive 2204f43f` → `base/`, `python3 tests/parity/run.py` → «fixtures: 35,
  failing: 28»), so the runner aborts before its unittest leg and the suites above
  were run directly.
- **L11 — Cody's independent review and the three reservations, fixed.** His review of
  `a5ec02fd` (`/Users/robert/.orca/bots/cody-2/scripts/t5-review-cody.md`) is APPROVE
  WITH RESERVATIONS, and each reservation was a real gap:
  - **H1 (HIGH).** `tests/scenarios/ledger_cleanup_flow.sh:256` still read the pre-split
    file by a fixed path, so the scenario was 0/7 on this branch. It now reads whichever
    store the binary under test wrote (split store first, pre-split as the fallback),
    with the reason and the follow-up in a comment: the file remains a test seam because
    no CLI read verb exists until `changes status` (T7). Back to **7/7**.
  - **H2 (MEDIUM).** The released CLI 0.28.0 that the hooks still run writes the
    pre-split store with its own lock, and after the handoff the split store never saw
    those rows — silently. The double read now **absorbs** the rows the split store does
    not have (items by id, opt-outs by record) and **reports** every divergence through
    `StoreReport` on stderr: rows absorbed, rows both files have with different content
    (`Shadowed`: an older ai-specs decided on a row this build already wrote — the split
    store's version answers and the difference is stated), and an unreadable pre-split
    store. **Why absorb instead of only detecting:** those rows are real decisions — a
    human opt-out, an item already opened — and ignoring them can open a second item for
    an identity that already has one or drop a human answer, which is exactly the loss
    S11 exists to prevent. The merge is idempotent and the next write carries the
    absorbed rows into the split store, so the divergence heals instead of recurring; a
    same-id disagreement is deliberately **not** rewritten in place, because a row this
    build wrote must not be silently replaced by an older binary's version. A corrupt
    pre-split file never takes the split rows down.
  - **LOW.** A backup that cannot be written is reported (`BackupFailed`) instead of
    swallowed; the split write still succeeds, and the pre-split store is untouched
    either way.
  Tests: four new unit tests (absorption with the pre-split store gaining a row and an
  opt-out after the handoff, a row changed in place, an unreadable pre-split store, a
  failed backup) plus the report's one-line-per-problem contract;
  `TestSplitRowsWinAndLegacyOnlyRowsAreAdded` replaces the older "prefers the split
  file" test, because the intended read is now split-plus-absorbed and the old
  expectation was the defect.
  Validation after the fixes: `go vet` + `go test ./... -count=1` green in both modules;
  `tests/scenarios/ledger_cleanup_flow.sh` **7/7**; the real flow is now **19 checks
  FLOW OK**, including a repository where the *baseline* binary keeps writing the
  pre-split store after the handoff (its row is reported, the next write carries it into
  the split store, and the pre-split file it owns is never rewritten); the Python suites
  are 70 + 43 ok; `./tests/validate.sh` exits 1 only in its parity leg, and the failing
  fixture set is **byte-identical on the untouched base tree** (28/35, the
  `/private<TEMP>` normalisation: the two lists of failing fixture names diff empty).
