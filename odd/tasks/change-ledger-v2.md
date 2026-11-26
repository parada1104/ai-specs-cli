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
  (S1, S2, S9, S16).
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
    `workstream` phase with the facts that decided it.
  - facts/ledger: all three, read-only.
  - tests: real-flow scenario that walks a temp repo through `workstream → open →
    planned → merged → cleaned` and asserts the reported phase and `next` line at
    each step; JSON shape stability; exit 3 when a needed fact is `unknown`.
  - risks: a status verb that lies is worse than none; every fact must be derived,
    never defaulted.
  - deps: T1, T2, T3, T5, T6. PR size ~700.
- T8 — `changes next` plus postures: `advise` by default, `require` per project and
  checkpoint (S5, S10).
  route: worker. commit: none.
  - scope: transition→posture map, project config for `require` (with
    per-checkpoint list) and for the R3/R4 thresholds, exit 2 only when opted in.
  - facts/ledger: composition layer only.
  - tests: default project never blocks even with high risk; a project with
    `require = ["merge"]` exits 2 with rule and exits; thresholds override the
    defaults; a workstream is never subject to a posture.
  - risks: config surface creep; keep it to two keys.
  - deps: T7. PR size ~400.
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
