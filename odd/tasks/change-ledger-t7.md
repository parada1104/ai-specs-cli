# change-ledger-t7 (ODD)

Unit T7 of the change-ledger-v2 redesign: `ai-specs changes status` becomes the
read-only composite that answers what this worktree is, what it owes and what it can
do next, composing the three capability ledgers by contract and never fabricating a
fact. It consumes the `Facts()`/`Verdict()` seam T2 delivered and extends it across the
worktree, plan and tracker sections that T6 finished. Master plan, design and audit live
in `odd/tasks/change-ledger-v2.md` and `odd/tasks/change-ledger-v2/`; T6b's own plan is
the immediate predecessor and must be read before touching the ledger.

## Specs

- **S3** — One ledger per capability, in the CLI, conversing without mixing state.
  Verbatim: «no es un ledger único. Hay un ledger por capacidad, todos en el CLI,
  que conversan pero no se mezclan: el de worktree (identidad, merge, limpieza,
  integración), el de plan-build-flow (la capa del change y su flujo: si
  plan-build-flow está activo el checkpoint tiene más pasos) y el del tracker
  (conciliación entre el tracker y los changes)».
- **S4** — The composite view lives in `changes` and composes the ledgers.
  Verbatim: «`ai-specs changes` es nuestra familia (archive, start --none), no el
  binario de OpenSpec, así que encaja que `changes status/next` sea la vista del
  change que compone los otros ledgers».
- **S12** — Review receipts are consumed from gentle-ai, never fabricated.
  Verbatim: «receipts de revisión consumidos de gentle-ai».
- **S14** — Everything goes through the CLI entrypoint; the on-disk layout is not a
  contract. Verbatim: «cada ledger con su propio archivo está bien siempre que todo
  pase por el entrypoint del CLI de ai-specs (nadie lee ni escribe esos archivos
  directamente)».
- **S16** — A worktree that is not a change owes nothing, and the CLI tells the two
  apart from facts, not from text. Verbatim: «`changes start --none` hoy también se
  usa para worktrees que NO son un change (por ejemplo, una auditoría o una
  exploración en su propio worktree). Hay que tener mucho cuidado con las tareas
  que crean un worktree/workstream pero no son efectivamente un change.» — the plan
  must resolve «cómo distingue el CLI un worktree sin change de un change sin plan
  (inferido por hechos, no por texto), qué responde status/next ahí, y que ningún
  ledger abra filas, pida card ni exija archive para ese worktree. Si eso exige
  conservar alguna forma de declaración, propónla con su motivo.»

## Tasks

- T7.1 — The facts registry: one derived fact set per ledger, read-only, with every
  field naming where it came from. The worktree facts (identity, base, divergence,
  merge state), the plan facts (tier, plan and delta, the standing `workstream`
  decision from the plan section) and the tracker facts (open/closed row, card,
  exemption, conflict, opt-outs) are collected once and never defaulted; a fact the
  environment cannot answer is `unknown` with its reason, never `false`. Specs: S3,
  S16. route: worker. commit: pending.
- T7.2 — `changes status`: the human rendering, the `--json` shape and `--explain`,
  with `--ledger worktree|plan|tracker` narrowing to one capability. Exit 0 for a
  resolved phase, 2 when the facts refuse the requested step, 3 when a fact the
  answer needs is `unknown`. The `workstream` phase renders the facts that decided it
  and the exit («when product paths change, this becomes a change»). Specs: S3, S4,
  S16. route: worker. commit: pending.
- T7.3 — The review receipt: consumed from gentle-ai for the candidate SHA, with
  `unknown` when no receipt exists, and never inferred from a commit message or a
  file on disk. Specs: S12. route: worker. commit: pending.
- T7.4 — The store notes surface here, once and per ledger: rows absorbed from a
  pre-split predecessor, rows that differ in place, an unreadable predecessor, a
  backup that could not be written. Today only a direct `worktree-gate` invocation
  prints them; `tracker-card-gate`, `doctor` and the cleanup path discard them, so a
  note can be lost and can mask the real error of a bind. Specs: S3, S14. route:
  worker. commit: pending.
- T7.5 — Validation: the real-flow scenario that walks a temp repo through
  `workstream → open → planned → merged → cleaned`, asserting the reported phase, the
  `next` line and the exit code at each step; `--json` shape stability; exit 3 with an
  `unknown` fact; both Go modules; `ledger_cleanup_flow.sh`; the full Python suite.
  Specs: S3, S4, S12, S14, S16. route: worker. commit: pending.

## Allowed edit surfaces

- catalog/recipes/worktree-flow/gate/ledger/
- catalog/recipes/worktree-flow/gate/runner/
- internal/planledger/
- internal/cli/
- internal/gates/
- internal/receipts/   (new package for T7.3; its final name is confirmed in T7.1)
- tests/scenarios/
- tests/
- odd/tasks/change-ledger-t7.md

## Verification

| Spec | Evidence |
| --- | --- |
| S3 | Each capability's facts come from its own section, and the composite reads them without writing any of them. |
| S4 | `changes status` answers the whole change from the three ledgers; no other verb is needed to learn what the worktree is or owes. |
| S12 | A candidate without a receipt reports `unknown`; a candidate with one reports exactly what gentle-ai recorded. |
| S14 | Every read goes through the CLI; the contract test still fails on an outside caller. |
| S16 | A worktree with no change and no artifact reports `workstream` with exit 0, and no ledger gains a row, no card is requested and no archive is demanded. |

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
  Plus, verbatim, from Robert at the close of T6b: «Antes del merge: handoff en
  Engram y worktree de T7 desde development con su rama y plan.»
- **L2** — Master plan entry, verbatim (`odd/tasks/change-ledger-v2.md`, T7):
  «T7 — `changes status`, the read-only composite (S3, S4, S12, S14, S16).» with its
  scope (the facts registry, the pure contract, human output, `--json`, `--explain`,
  `--ledger worktree|plan|tracker`, `--rebuild`, exit codes 0/2/3, review receipts
  referenced by candidate SHA, the workstream rendering), its deps (T1, T2, T3, T5,
  T6) and its size (~700 lines). Its risks, verbatim: «a status verb that lies is
  worse than none; every fact must be derived, never defaulted» and, carried from
  T5's review, «the store notes … are only visible when `worktree-gate` is invoked
  directly … This verb is where they surface, once and per ledger.»
- **L3** — Inherited state, not to be re-derived. T1 shipped the tier derivation in
  `internal/tier`; T2 the facts→verdict seam in `internal/gates/writefacts.go`
  (`WriteFactsFor`, `WriteFacts.Verdict()`, `Refutation`, with `--explain` and the
  refusal derived from one verdict); T3 the workstream discriminator and
  `changes workstream [--reason|--end]`; T5 the tracker store extraction; T6a the
  one-file per-capability ledger (`<git-common-dir>/ai-specs/ledger/state.json`, v2,
  sections `worktree`, `tracker`, `plan`) and T6b the plan section: the
  `workstream`/`workstream-end` decisions live in `LedgerFile.Plan`, keyed by the work
  identity (`common_dir` + `branch`), seeded on read from the retired
  `.ai-specs/plan-ledger.jsonl` predecessor, with `Save(file, Section...)` refusing a
  write that names no section. `internal/planledger` is the CLI's port to that
  section and the one place outside the gate allowed to reach it; the S14 contract
  test (`internal/cli/ledgerstore_contract_test.go`) detects the store API and the
  package import, not only path literals.
  Traps carried over, each of which cost real time: the pi-lens LSP binds the MAIN
  worktree, so its `undefined:` reports are stale in a linked one — only `go build`,
  `go vet`, `go test` count; the gate is its own Go module under
  `catalog/recipes/worktree-flow/gate`; `tests/run.sh` aborts at its parity leg, so
  run `python3 -m unittest discover -s tests -p 'test_*.py'` directly (about ten
  minutes; the base has exactly one pre-existing failure,
  `test_git_pr_flow_recipe.GitPrFlowGoldenContentTests.test_skill_never_recommends_delete_branch`);
  a merge retires the worktree and the cwd; native review cannot run above roughly
  3300 changed lines (`lens_context_budget_exceeded` with every lens set), so such a
  candidate goes to Robert with `LISTO REVISAR-<unit>`; and never leave a build under
  a scratch probe file — a temporary file in a package the tree compiles will break
  every concurrent build, which is how one Python parity leg reported a false
  `go build failed` during T6b.
- **L4** — Robert's decision on T7.3, verbatim: «autorizo leer SOLO el paquete
  gentle-pi (código, solo lectura) para encontrar el comando nativo soportado de
  lectura de recibos por candidato; NO leas /Users/robert/.gentle-ai (puede tener
  estado o credenciales). Si encuentras un comando soportado y estable, consúmelo
  desde Go invocando el binario; si no, review queda unknown con su motivo y T7.3
  pasa a otra unidad como reserva en el plan maestro. No dependas de formatos
  privados del disco.» Found (gentle-pi `lib/review-risk-assessment.ts:1-21`,
  `lib/native-review-cli.ts:2640`): the documented read-only verb
  `gentle-ai review assess --cwd <repo> --base-ref <full sha> --committed-only --json`,
  schema `gentle-ai.review-assessment/v1`, with `candidate.consumed` (absent on older
  binaries, «never defaulted»), `review_due`, `review_due_reason` (`already_reviewed`,
  `high_risk`, …) and `risk`. Observed on this worktree: `consumed:false`,
  `review_due:false`, `review_due_reason:passive`. gentle-pi ships the binary inside
  its own package; ai-specs does not reach into it.
- **L5** — Design decisions for T7 (parent-owned, binding on the workers):
  1. Phases, first match wins: `cleaned` (no open worktree row for the work key and a
     closed one exists) → `merged` (the local merge proof the cleanup already owns holds
     against a base candidate, AND the work is real: a plan or delta in the tree, or an
     open worktree row; a fresh branch whose tip is the base tip is not merged) →
     `planned` (plan or delta) → `open` (a change: not a workstream) → `workstream`.
     A squash merge the local proof cannot see stays `planned`/`open`; the next line
     says so instead of guessing.
  2. Exits: 0 a resolved phase; 2 the facts refuse the requested step (the write
     verdict carries blockers) — usage errors keep the family's 2 and print no
     report; 3 a fact the phase needs is `unknown` (no git checkout, no base, diff or
     untracked listing git could not answer, a ledger section that cannot be read).
     The review receipt never decides the exit: it is reported, not graded.
  3. `--ledger worktree|plan|tracker` narrows the human and `--json` report to that
     capability's facts; the phase and the exit stay the composite's.
  4. `--explain` prints every fact with its source on stderr.
  5. `--rebuild` (master-plan scope) is not in this unit's tasks and stays out: a
     read-only verb that rebuilds is a write, and goes to T8 as a reservation.
  6. Read-only by contract: status writes no ledger file, no backup and no creation
     token. The plan identity gets a read form that reads the token and never mints
     it (T6b note c); only `RecordPlanDecision`'s path mints.
  7. Receipt (T7.3): `internal/receipts` runs `$GENTLE_AI_BIN`, else `gentle-ai` on
     PATH, with the L4 argv against the resolved base's full SHA, bounded by a
     timeout. It relays `consumed`, `review_due`, `review_due_reason` and `risk`
     verbatim; an absent `consumed`, a missing binary, a non-zero exit, a timeout or
     an undecodable envelope is `unknown` with its reason. Nothing is inferred from a
     commit message or a file.
  8. Every fact carries its source; a fact git cannot answer is `unknown` with its
     reason, never `false`. The untracked listing gains `UntrackedKnown` (T3 R2).
  9. Store notes (T7.4): status prints the ledger's `StoreReport` once; `changes
     workstream --end/--reason` prints it once (the write's report, not the read's
     too — T6b note a); `tracker-card-gate` relays the note and reports the real
     error instead of the first stderr line. Outside a git checkout, status reports
     the checkout as `unknown` and exits 3 (T6b note b).
