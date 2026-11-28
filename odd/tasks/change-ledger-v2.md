# change-ledger-v2 (ODD)

The flow ledger stops being one store fed by text heuristics and becomes three
capability ledgers inside the CLI (worktree, plan-build, tracker) that talk by
read-only contract, with a composite `changes status|next` that answers what you
can do next and why. It starts by removing what today contradicts Robert's rule:
`plan-build-gate` blocks an OpenSpec production write when no plan exists. It also
gives a name to the worktree that is not a change at all (audit, exploration,
spike), which today has no representation. Worktree `.worktrees/change-ledger-v2`,
base `development@36438150`. Planning phase closed with no commit, push or PR;
implementation runs one unit per branch and per PR (L10).

Design source of truth: `odd/tasks/change-ledger-v2/design.md` (v3, closed).
Evidence base: the read-only audit in `odd/tasks/change-ledger-v2/audit.md`, the
gate reproduction matrix in that design's §1, and the `start --none` inventory in L8.
The full target deltas live under `odd/tasks/change-ledger-v2/specs/` as context;
each unit's own delta lives in its own change folder.

## Specs

- **S1** — No mode may block for a missing plan; the CLI infers the tier.
  Verbatim: «el gate de plan-build-flow NO debe bloquear por falta de archivo de
  plan, ni en ODD ni en SDD/OpenSpec, y no debe esperar autorización: debe INFERIR
  el tier, como ODD (sin plan = none/small; plan = light; plan + delta =
  standard)».
- **S2** — `none` is a refutable claim, and any refusal names the rule and the
  exits. Verbatim: «El CLI no prueba "small": solo lo refuta con hechos objetivos,
  y si lo refuta, el bloqueo nombra la regla y las salidas. El agente puede
  planificar igual».
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
- **S5** — Advise by default; `require` is per project with per-checkpoint detail.
  Verbatim: «Riesgo alto: aconsejar por defecto.» / «require por proyecto con
  detalle por checkpoint: sí».
- **S6** — Card/item reconciliation is `always|ask|off`, generic per provider, with
  `ask` by default once a tracker is enabled. Verbatim: «un modo tipo always/ask
  como en worktree, para que el agente consulte si desea crear la card (Trello) o
  el item (Jira, OpenProject): genérico por provider.» / «Card/item: ask por
  defecto al habilitar un tracker.»
- **S7** — A tracker↔change disagreement is reported and resolved by offered
  decisions, recommended from the facts. Verbatim: «Desacuerdo tracker↔change: se
  informa y se ofrecen decisiones (cómo reconciliar, etc.) según lo que encuentre
  el agente.» / «ante un desacuerdo tracker↔change el CLI recomienda una opción
  según los hechos».
- **S8** — After a merge the CLI only verifies integration and prints the command.
  Verbatim: «Tras el merge el CLI solo verifica la integración y da el comando.»
- **S9** — `changes start --none` is deprecated: inert for one version, then
  removed. Verbatim: «deprecar `changes start --none` (sin efecto una versión,
  luego se elimina)».
- **S10** — Risk thresholds ship as CLI defaults, adjustable per project.
  Verbatim: «umbrales de riesgo: defaults del CLI ajustables por proyecto».
- **S11** — Store v2 with one-version double read; the 14 rows stay as history.
  Verbatim: «store v2 con doble lectura una versión, las 14 filas quedan como
  histórico».
- **S12** — Review receipts are consumed from gentle-ai, never fabricated.
  Verbatim: «receipts de revisión consumidos de gentle-ai».
- **S13** — The invisible Pi advisory is fixed inside this redesign. Verbatim: «el
  aviso invisible del wrapper de Pi se arregla dentro del rediseño».
- **S14** — Everything goes through the CLI entrypoint; the on-disk layout is not a
  contract. Verbatim: «cada ledger con su propio archivo está bien siempre que todo
  pase por el entrypoint del CLI de ai-specs (nadie lee ni escribe esos archivos
  directamente)».
- **S15** — Delivery shape: one PR per unit, under ~1000 lines, each with a
  real-flow test and a RED. Verbatim: «partiendo el rediseño en unidades
  entregables y ordenadas (cada una con su PR, < ~1000 líneas, con su prueba de
  flujo real y su RED)».
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

- T1 — Tier inference core: one derivation used by the write gate and the
  guardian, in both modes, with ODD learning the three-value tier (S1, S2).
  route: worker. commit: none.
  - scope: extract the `none|light|standard` derivation out of `internal/gates`
    into a package both callers use; ODD guardian stops reporting `none|odd` and
    computes the three values; no behaviour change yet at the write gate.
  - facts/ledger: L2 P2 only. No store writes.
  - tests: table test `none|light|standard` in ODD and OpenSpec; RED = ODD
    guardian returns `odd` today where the table expects `light`.
  - risks: touching merge-time grading; mitigate by keeping guardian verdicts and
    messages byte-identical for every current fixture (differential on the
    existing suites).
  - deps: none. PR size ~300.
- T2 — The decisive fix: the OpenSpec write gate stops requiring a plan, blocks
  only on a proven refutation, and never blocks a worktree without a change
  (S1, S2, S9, S16). *(R3-R5 moved to T8 by L11; the unit is redesigned around one
  facts→verdict seam by L12.)*
  route: worker. commit: none.
  - scope: `internal/cli/planbuildgate.go` (remove the plan/ceremony requirement
    in OpenSpec; treat a worktree with no product-path divergence and no artifact
    as nothing to grade), new refutations R1–R5 with R3/R4 scored from the diff,
    `internal/planbuild/ceremony.go` + `changes start --none` → deprecation notice
    and no gate effect, message rewrites (no "go ask permission" text).
  - facts/ledger: L2 P1/P2/P3 at `work-start`. No store writes.
  - tests: real-flow scenario in a temp repo over the audit matrix — ODD no plan
    → 0; OpenSpec no plan → 0; audit worktree with no divergence → 0; refutation
    R1 (canonical spec without delta) → 2 naming R1; R4 (contract change) → 2
    naming R4; decl after `start --none` → 0 with a deprecation line. RED = today
    both no-plan cases exit 2.
  - risks: this removes a guard; the mitigation is that every refusal is now
    fact-based and that the merge guardian is untouched. High-risk item 3
    (contracts others consume) → native review due.
  - deps: T1. PR size ~500.
- T3 — The workstream: a worktree that is not a change owes nothing (S16).
  route: worker. commit: none.
  - scope: the fact-based discriminator, in one place and shared by every consumer
    (gate, composite view, ledgers). A worktree is a **change** when it shows
    evidence of product work: product-path divergence from the integration base
    (`git rev-list <base>..HEAD -- <production paths>` or uncommitted product-path
    changes), or an artifact that names a change (plan, delta, change folder).
    Otherwise it is a **workstream** (audit, exploration, spike, review) — including
    the honest case of a branch just created with nothing in it yet.
    Add `ai-specs changes workstream [--reason <text>]` and `--end`: an explicit,
    append-only decision in the plan ledger for the genuinely ambiguous case (a
    worktree with product divergence that is deliberately not a change). The
    declaration is refused when an artifact exists — that is a change with a plan,
    not a workstream — and it never waives a refutation for a change.
    In a workstream: no ledger row, no tracker card, no archive obligation, no
    tier, no refutations; `status` reports the phase `workstream`, the facts it
    observed and the exit ("when product paths change, this becomes a change").
  - facts/ledger: L2 owns the workstream decision; L1 keeps answering identity and
    merge for that worktree (cleanup still applies after a merge); L3 stays silent.
  - tests: real-flow scenario — (a) a fresh worktree with no divergence → `status`
    says `workstream`, exit 0, and no ledger file gains a row, no card is
    requested, no archive is demanded; (b) the same worktree after a product
    commit → phase flips to a change with tier `none`; (c) a declared workstream
    with product commits stays silent until `--end`; (d) a declaration on a
    worktree that has a plan is refused with the rule named. RED = today `status`
    does not exist and OpenSpec mode blocks case (a).
  - risks: a wrong discriminator silently exempts real work from the flow;
    mitigate by making the discriminator conservative (any product-path divergence
    ⇒ change) and by surfacing the observed facts in `--explain`.
  - deps: T2 (same code path), T7 for the reporting half — ships the discriminator
    with a `--json` fact in T3 and the human view in T7. PR size ~350.
- T4 — Pi advisory becomes visible: stderr surfaces with exit 0, with a test that
  pins the contract (S13).
  route: worker. commit: none.
  - scope: the generated Pi wrapper (`.pi/extensions/*-tracker-card-gate.ts`
    generator in `internal/sync/hooksrender.go`), plus the same contract for OMP
    and OpenCode adapters if they share the pattern.
  - facts/ledger: L3 reporting only.
  - tests: unit on the renderer (wrapper text contains the unconditional stderr
    relay), plus a scenario step asserting a warning produced with exit 0 reaches
    stderr of the wrapper. RED = today the relay is gated on exit 2.
  - risks: noisy output for agents; mitigate by routing advisories to one line.
  - deps: none (independent, can ship first). PR size ~120.
- T5 — Tracker store extraction with v1→v2 double read (S11, S14).
  route: worker. commit: none.
  - scope: `.git/ai-specs/ledgers/tracker.json` as the first split store, written
    only through the CLI; a migration reader that also reads
    `ledger/state.json` v1; decision routing (bindings, opt-outs, adjudications);
    the 14 historical rows stay untouched.
  - facts/ledger: L3 T1/T4 plus the store layer.
  - tests: v1 fixture is read and re-written as v2 once a decision lands; the 14
    rows survive byte-identical as history; corrupt one store → the others still
    answer; contract test that no package outside `internal/cli` writes those
    paths.
  - risks: silent data loss on migration; mitigate with a one-shot backup path and
    a migration test on a copy of the real 14-row file.
  - deps: none. PR size ~600.
- T6 — Worktree and plan stores extracted, with the same invariants (S3, S11, S14).
  route: worker. commit: none.
  - scope: `.git/ai-specs/ledgers/{worktree,plan}.json`, moving close/exemption
    decisions to worktree and plan exemptions/archive/workstream decisions to
    plan; delete the monolithic writer.
  - facts/ledger: L1 and L2 decision logs.
  - tests: each ledger's decisions land in its own file; every existing
    cleanup/bind scenario stays green (differential); isolation on corruption.
  - risks: the cleanup close path is the one that runs on merges — a mistake there
    leaves merged worktrees behind; mitigate by running the existing
    `ledger_cleanup_flow.sh` before and after.
  - deps: T5. PR size ~450.
- T7 — `changes status`, the read-only composite (S3, S4, S12, S14, S16).
  route: worker. commit: none.
  - scope: facts registry per ledger, the pure `Facts()`/`Verdict()` contract,
    human output, `--json`, `--explain`, `--ledger worktree|plan|tracker`,
    `--rebuild`, exit codes 0/2/3; review receipts consumed from gentle-ai
    (referenced by candidate SHA, `unknown` when absent); the human rendering of the
    `workstream` phase with the facts that decided it. The seam itself is delivered by
    T2 (L12): this unit consumes it and extends it to the three ledgers.
  - facts/ledger: all three, read-only.
  - tests: real-flow scenario that walks a temp repo through `workstream → open →
    planned → merged → cleaned` and asserts the reported phase and `next` line at
    each step; JSON shape stability; exit 3 when a needed fact is `unknown`.
  - risks: a status verb that lies is worse than none; every fact must be derived,
    never defaulted.
  - carried from T5's review (R2): the store notes — rows absorbed from the pre-split
    store, rows that differ in place, an unreadable pre-split store, a backup that
    could not be written — are only visible when `worktree-gate` is invoked directly.
    `tracker-card-gate`, `doctor` and the cleanup path discard them, so a note can be
    lost and can even mask the real error of a bind. This verb is where they surface,
    once and per ledger.
  - deps: T1, T2, T3, T5, T6. PR size ~700.
  - carried from T6b's independent review (Cody, `eb85ee36`, non-blocking):
    (a) `changes workstream --end` and `--reason` print the store note twice on stderr — once
    from the read that decides the refusal and once from the write — so the notes need one
    owner per invocation, which is this verb's "once and per ledger" rule;
    (b) a directory with no git now answers with a new message (the plan port cannot name a
    checkout there), and `status` must render that case deliberately instead of inheriting
    whatever the workstream verb prints;
    (c) reading the plan section can write: the creation token is written into the admin
    directory the first time a checkout is named, even by a read-only call. `status` is
    read-only by contract, so the token must be minted only on a write (or the read must
    tolerate its absence) before this verb ships.
- T8 — `changes next` plus postures: `advise` by default, `require` per project and
  checkpoint (S5, S10).
  route: worker. commit: none.
  - scope: transition→posture map, project config for `require` (with
    per-checkpoint list) and for the R3/R4 thresholds, exit 2 only when opted in,
    plus the refutations R3-R5 themselves (L11): the size and module threshold, the
    removed public contract, and the plan whose `S#` rows carry no evidence, each
    with the exit it honours.
  - facts/ledger: composition layer only.
  - tests: default project never blocks even with high risk; a project with
    `require = ["merge"]` exits 2 with rule and exits; thresholds override the
    defaults; a workstream is never subject to a posture.
  - risks: config surface creep; keep it to two keys.
  - carried from T5's review (R1): a close written by an older binary on a row the
    split store already has stays shadowed — the split store's version answers — and
    its notice repeats on every invocation. Report it once, name the ledger that owns
    the row, and say which file wins.
  - deps: T7. PR size ~400.
  - carried from T6b's independent review (Cody, `eb85ee36`, non-blocking):
    (a) the gate pays roughly 20-60 ms more per call since the plan section is read through
    the full ledger load (rev-parse calls for the checkout identity plus the file read);
    measure it on the hook path before adding the posture checks, and memoize the identity
    per invocation if the budget is tight;
    (b) `.ai-specs/` is ignored only in this repository's own `.gitignore`; a consumer
    project still sees the plan predecessor and its one-shot copies as untracked files until
    the managed ignore block that `sync` renders covers them (T11/T12 when the docs and the
    release land).
- T8b — Integration target per worktree, in the worktree ledger (approved by Robert,
  relayed by Cody at T7's close). Verbatim: «destino de integración por worktree en el
  ledger de worktree (inferido de la rama de origen, p. ej. las ramas que salen de una
  épica integran en epic/x; override explícito por CLI; el toml del proyecto queda
  como default y no se toca; status/next, prueba de merge y cleanup lo usan).»
  route: worker. commit: none.
  - scope: an integration-target fact per worktree row, inferred from the branch it
    was cut from (a branch cut from `epic/x` integrates into `epic/x`); an explicit
    CLI override recorded as a worktree-ledger decision; the project's toml
    (`base_branch`/`integration_branch`) stays the default and is never rewritten;
    `changes status`/`next` (the `integration` phase and the merge proof), the local
    merge proof and `worktree-cleanup` read the per-worktree target instead of the
    project default.
  - facts/ledger: L1 (worktree) owns the target; L2/L3 read it.
  - tests: a branch cut from `epic/x` reports `epic/x` as its target and is proven
    merged only into it; the override wins over the inference; a project without
    either keeps the toml default; cleanup removes a worktree merged into its own
    target and keeps one merged only into the project default.
  - risks: cleanup is destructive — a wrong target removes unmerged work; the target
    must be a recorded fact, never guessed at removal time.
  - deps: T8. PR size ~400.
- T9 — Tracker reconciliation: `always|ask|off` per provider and the decision menu
  on disagreement (S6, S7).
  route: worker. commit: none.
  - scope: tracker mode config, `ask` prompt reusing the worktree mechanics (no TTY
    ⇒ inform and continue, recording the answer), provider-generic adapter verbs
    (Trello card, Jira issue, OpenProject work package), and the offered
    decisions with a fact-based recommendation.
  - facts/ledger: L3 T1–T4.
  - tests: `ask` with no TTY informs and continues without blocking; a
    disagreement offers the six decisions and recommends one from the facts; the
    chosen decision is appended once and not re-asked; a workstream is never
    asked for an item.
  - risks: provider calls from the CLI; keep them behind the adapter and
    idempotent.
  - deps: T5, T7. PR size ~500.
- T10 — Removals cleanup: the pieces the redesign deletes, with the use-case
  migration spelled out (S1, S9, S16).
  route: worker. commit: none.
  - scope: `PersistConflict` and `AppendDecisionToPrimary` (no production caller),
    the `archive-close` duplicate checkpoint, the folder matcher as a flow
    trigger, `## Tracker` as a bind requirement, the retired `worktree-gate.sh`
    launcher and its README promise, and `changes start --none` itself once its
    deprecation window has passed.
  - migration of the two uses of `start --none`, named explicitly so the removal
    loses nobody: (a) "a change that owes no plan" → nothing to declare, the tier
    is inferred (T1/T2); (b) "a worktree that is not a change" → `changes
    workstream` (T3). The inventory in L8 is the checklist; each of its 12 call
    sites is either deleted, rewritten to the new verb, or kept as a frozen-golden
    normalization that no longer refers to a live verb.
  - facts/ledger: all three (deletion only).
  - tests: the suites that referenced them move to the new contract; a grep-based
    guard proves the retired symbols are gone.
  - risks: deleting something still reachable — verify by searching for callers
    before each removal.
  - deps: T2, T3, T6, T9. PR size ~350.
- T11 — Real-flow coverage and docs for the composed flow and the workstream
  (S15, S16).
  route: worker. commit: none.
  - scope: extend `tests/scenarios/ledger_cleanup_flow.sh` and add
    `tests/scenarios/change_status_flow.sh` (including the workstream case); update
    `docs/{recipes-catalog,runtime-hooks}.md`, the recipe READMEs, and the
    `plan-build-flow` SKILL section that still documents the plan requirement.
  - facts/ledger: none (documentation and harness).
  - tests: the scenarios themselves are the test.
  - risks: docs drift is what caused this audit; tie each doc edit to an S#.
  - deps: T7, T8, T9. PR size ~400.
- T12 — Repo dogfood sync and CHANGELOG after the release (S15).
  route: inline. commit: none.
  - scope: `ai-specs sync` of this repository with the released CLI, lock-only PR
    (the #605/#609 shape), CHANGELOG entry and upgrade note.
  - facts/ledger: F7 integration evidence.
  - tests: `doctor` 0 ERROR; `changes status` on a real merged branch and on a real
    workstream.
  - risks: mixing product and project state — isolate in its own branch.
  - deps: T11 merged and released. PR size ~150.

## Log

- **L1** — «exploración + propuesta de rediseño del ledger. SIN código: no edites el
  repo, sin commit/PR/sync.» «Visión de Robert: el ledger es la fuente de verdad
  del flujo, determinista y resuelta por el CLI, para decirle a la persona/agente
  qué se hizo, qué falta, qué puede y qué no puede hacer en cada caso, y por qué.
  Modelo: el binario de OpenSpec … Como RDD: no obliga a nada por defecto; evalúa
  el caso/riesgo y recién entonces exige (como los lentes). Un texto en un md (p.
  ej. ## Tracker) puede ser un recordatorio, nunca el contrato. Ejemplo: si la rama
  ya se mergeó, el siguiente paso obvio es la limpieza y actualizar development.»
- **L2** — «1) Riesgo alto: aconsejar por defecto. 2) Naming y arquitectura: no es
  un ledger único. Hay un ledger por capacidad, todos en el CLI, que conversan
  pero no se mezclan … 3) Card/item: un modo tipo always/ask como en worktree …
  genérico por provider. 4) Tras el merge el CLI solo verifica la integración y da
  el comando.» Decisiones técnicas: «store v2 con doble lectura una versión, las 14
  filas quedan como histórico, receipts de revisión consumidos de gentle-ai, y el
  aviso invisible del wrapper de Pi se arregla dentro del rediseño.»
- **L3** — «ÉNFASIS: el gate de plan-build-flow NO debe bloquear por falta de
  archivo de plan, ni en ODD ni en SDD/OpenSpec, y no debe esperar autorización:
  debe INFERIR el tier, como ODD (sin plan = none/small; plan = light; plan + delta
  = standard). El CLI no prueba "small": solo lo refuta con hechos objetivos, y si
  lo refuta, el bloqueo nombra la regla y las salidas. El agente puede planificar
  igual.» «require por proyecto con detalle por checkpoint: sí.» «Card/item: ask por
  defecto al habilitar un tracker.» «Desacuerdo tracker↔change: se informa y se
  ofrecen decisiones (cómo reconciliar, etc.) según lo que encuentre el agente.»
  «cada ledger con su propio archivo está bien siempre que todo pase por el
  entrypoint del CLI de ai-specs (nadie lee ni escribe esos archivos
  directamente).»
- **L4** — «Robert cerró el diseño v3: (1) deprecar `changes start --none` (sin
  efecto una versión, luego se elimina); (2) umbrales de riesgo: defaults del CLI
  ajustables por proyecto; (3) ante un desacuerdo tracker↔change el CLI recomienda
  una opción según los hechos; (4) diseño cerrado.»
- **L5** — «prepara el plan de implementación por partes, todavía sin código de
  producto: en un worktree nuevo desde development, escribe el plan ODD
  (odd/tasks/<change>.md y los deltas de specs que correspondan) partiendo el
  rediseño en unidades entregables y ordenadas (cada una con su PR, < ~1000 líneas,
  con su prueba de flujo real y su RED), empezando por lo que hoy contradice la
  regla de Robert: el bloqueo por falta de plan en OpenSpec.» «Sin
  commit/push/PR.» Nota de Robert: limpió los worktrees obsoletos
  (`plan-build-odd-companion`, `plan-build-openspec-odd-align`,
  `review-slice2/tiers-close` y sus ramas), con el contenido íntegro en
  `development`.
- **L6** — Evidence for S1/S2 recorded during planning: today `plan-build-gate`
  exits **0** for an ODD production write with no plan, **2** for an OpenSpec
  production write with no `tasks.md` (offering `changes start --none`), and **2**
  for a hand write to `odd/specs/` without a delta. The Nov-24 decision
  (`e931c4f4`) already implemented inference for ODD only; the approved ODD log
  chose the local declaration for OpenSpec
  (`odd/tasks/plan-build-openspec-odd-align.md:319–324`), so L3 supersedes that
  choice. Tests that pin the current OpenSpec refusal and will change expectation:
  `internal/cli/planbuildgate_test.go:119,637,720`.
- **L7** — «Matiz de Robert para el plan: `changes start --none` hoy también se usa
  para worktrees que NO son un change (por ejemplo, una auditoría o una exploración
  en su propio worktree). Hay que tener mucho cuidado con las tareas que crean un
  worktree/workstream pero no son efectivamente un change. Antes de deprecarlo,
  inventaria con evidencia quién lo usa y para qué (skills, recetas, hooks, worker
  specs), y que el plan resuelva explícitamente ese caso: cómo distingue el CLI un
  worktree sin change de un change sin plan (inferido por hechos, no por texto),
  qué responde status/next ahí, y que ningún ledger abra filas, pida card ni exija
  archive para ese worktree. Si eso exige conservar alguna forma de declaración,
  propónla con su motivo. Inclúyelo en el reporte.»
- **L8** — Inventory of `changes start --none` (grep over `*.md/*.toml/*.sh/*.go/*.ts/*.py`,
  excluding `tmp/`, `dist/`, `node_modules`), run at planning time:
  **docs and product text**: `catalog/recipes/plan-build-flow/README.md:101–107`,
  `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md:248–260` and
  `:255–260`, `catalog/recipes/plan-build-flow/recipe.toml:42` (session context)
  and `:54` (hook description), `internal/cli/help.txt`.
  **code**: `internal/cli/changes.go` (verb + config write), `cli.go` (dispatch),
  `internal/cli/planbuildgate.go:141–143,224–227,240–252` (the waiver text and the
  `git config` read), `internal/planbuild/ceremony.go:23–50` (the value).
  **tests**: `internal/cli/{planbuildgate,changes}_test.go`,
  `tests/test_plan_build_flow_recipe.py`, `tests/parity/parity.py:241–242,540–542`
  (a frozen-golden normalization whose justification names the verb),
  `tests/evals/lib/harness.py`, `tests/evals/eval_plan_build_flow_live.py`,
  `tests/evals/scenarios/plan-build-flow/ac_ceremony_none_openspec/scenario.toml`
  (`required_git_config = { "branch.<branch>.aiSpecsCeremony" = "none" }`).
  **Finding**: every live use is the "this change owes no plan" case. **No skill,
  recipe, hook or worker spec anywhere uses it for a worktree that is not a
  change** — that case has no representation at all today, which is exactly why
  S16 needs a fact-based discriminator plus an explicit `workstream` decision, and
  why removing the verb without it would leave audit/exploration worktrees with no
  legal path in OpenSpec mode.
- **L9** — *(superseded by L10; kept as history.)* Implementation phase opened with
  Robert's per-unit mandate: T1–T3 in
  order, one unit = one PR — implement, validate (focused tests + real-flow proof
  with the branch binary + RED), native review before the merge, PR to
  `development`, merge, then `ai-specs sync` with its own PR; at the close of T3,
  release with the bump validated before the tag, update the global install and
  verify `version`/`doctor` without `AI_SPECS_HOME`. Branches are
  `feat/change-ledger-t<N>` and the guardian is graded with
  `--change change-ledger-v2`, because the branch suffix is the unit and not the
  change. Delivery shape decided during T1: each unit PR carries its code plus
  this plan; `odd/changes/change-ledger-v2/` (design, audit, deltas) stays out of
  the unit PRs until the deltas are promoted and the folder is archived, because
  the pre-merge guardian owes an archive for any active change folder and this
  change spans twelve units, not one. Commit field below records the merged
  squash commit as each unit closes.
- **L10** — L9's delivery shape is superseded by Cody's decision (same message as
  change-ledger-t1 L1): one unit is its own ODD change. Every unit PR carries a
  branch-paired plan (`odd/tasks/<unit>.md`) plus only the deltas that unit
  implements, promoted and archived before its merge; this master plan, its design,
  audit and the full target deltas live under `odd/tasks/change-ledger-v2/` — the
  change folder moved out of `odd/changes/` on purpose, so the guardian never owes
  an archive for the redesign as a whole. The deltas for T4–T12 are drafted here
  and each is created with its unit.
- **L11** — «Nota de Cody: elegí recortar (decisión técnica mía, no de Robert:
  regístralo así). T2 entrega el tier inferido, R1/R2 y start --none inerte; R3–R5
  pasan a T8 junto a los umbrales. Actualiza el plan maestro con el movimiento y su
  motivo, y pasa la revisión nativa al candidato recortado antes del merge.»
  Movement: T2 keeps S1/S9/S16 and the R1/R2 half of S2; T8 gains the refutations R3
  (configured size and module threshold), R4 (a removed public contract) and R5 (a
  plan whose `S#` rows carry no evidence), beside the per-project thresholds and the
  postures that give them their exit. Motive: all ten severe findings of T2's two
  review lineages lived in R3–R5, and a refusal whose exit depends on thresholds and
  recorded decisions belongs to the unit that introduces that configuration.
- **L12** — «Decisión de Cody (técnica, regístrala así): opción 1, rediseño del seam.
  Recolectar los hechos una vez y expresar R1/R2 como reglas puras sobre ellos, con
  --explain y el rechazo derivados del mismo veredicto; es adelantar a T2 el contrato
  Facts()/Verdict() que el plan pone en T7.» Movement: T2 stops being a patch series
  over the existing early-return chain and becomes the seam — one fact collection (mode,
  change, workstream?, tier, pending path, production?, declared high-risk, diff entries,
  plan/delta presence) and pure rules over it, with the refusal and `--explain` both
  derived from the same verdict. T7 consumes and extends that seam to the three ledgers
  instead of introducing it. R3-R5 stay in T8 with the thresholds and decisions that
  make their exits resolvable. Motive, on record: four review lineages on T2 all closed
  `escalated` and every severe finding is an instance of five root causes — (1)
  refutation applicability inferred from the position of a `return` instead of the rule's
  own preconditions; (2) the S16 "nothing to grade" fact owned by the CLI, so each rule
  re-implements or forgets it; (3) "public contract" decided by the shape of a line; (4)
  refusals advertising exits that nothing honoured; (5) the explanation emitted at the end
  of one path instead of by the decision function. Full record of the four rounds:
  `odd/tasks/change-ledger-v2/t2-review-escalation.md`.
