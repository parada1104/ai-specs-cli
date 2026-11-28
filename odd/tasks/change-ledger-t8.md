# change-ledger-t8 (ODD)

Unit T8 of the change-ledger-v2 redesign: `ai-specs changes next` and the postures —
`advise` by default, `require` per project and checkpoint — plus the refutations R3-R5
and their thresholds. It composes on `changes status` (T7): the same facts registry,
phases and exits. Master plan: `odd/tasks/change-ledger-v2.md`; T7's plan
(`odd/tasks/change-ledger-t7.md`, L5-L9) is the immediate predecessor and must be read
first.

## Specs

- **S5** — Advise by default; `require` is per project with per-checkpoint detail.
  Verbatim: «Riesgo alto: aconsejar por defecto.» / «require por proyecto con
  detalle por checkpoint: sí».
- **S10** — Risk thresholds ship as CLI defaults, adjustable per project.
  Verbatim: «umbrales de riesgo: defaults del CLI ajustables por proyecto».
- **S16** — A worktree that is not a change owes nothing (a workstream is never
  subject to a posture). Verbatim in the master plan.

## Tasks

- T8.1 — `changes next`: the one next step derived from `changes status`' phase and
  facts, with the same exits (0/2/3) and `--json`. Specs: S5, S16. route: worker.
  commit: pending.
- T8.2 — Postures: the transition→posture map; `advise` by default; `require` opt-in
  per project with a per-checkpoint list in `ai-specs/ai-specs.toml`; exit 2 only when
  opted in. Keep the config to two keys. Specs: S5. route: worker. commit: pending.
- T8.3 — Refutations R3-R5 (L11 of the master plan): the size and module threshold,
  the removed public contract, the plan whose `S#` rows carry no evidence; thresholds
  as CLI defaults overridable per project. Specs: S10. route: worker. commit: pending.
- T8.4 — Carried reservations (L3): resolve or re-carry each one with a reason (L7).
  route: worker + inline. commit: pending.
- T8.5 — Validation: real flow (default project never blocks even with high risk;
  `require = ["merge"]` exits 2 naming the rule; thresholds override defaults; a
  workstream is never subject to a posture), RED, both Go modules,
  `ledger_cleanup_flow.sh`, the full Python suite. route: inline. commit: pending.

## Allowed edit surfaces

Confirmed at start: `internal/cli/`, `internal/gates/`, `internal/planbuild/`,
`catalog/recipes/plan-build-flow/`, `catalog/recipes/worktree-flow/gate/ledger/`,
`tests/parity/parity.py`, `odd/tasks/change-ledger-t8.md`.

## Log

- **L1** — Mandate, verbatim (Robert, carried from T6/T7): «Sigue con el plan maestro:
  … una unidad = un worktree nuevo desde development + un PR, con el mismo mandato
  (validar con flujo real y RED, revisión nativa antes del merge; si la revisión se
  traba en el paso de validación como en T2, me avisas con LISTO REVISAR-<unidad> y
  reviso yo; PR, merge, `ai-specs sync` del repo con su PR). Sin release hasta T12,
  salvo que una unidad lo necesite. Guarda en Engram antes de cada merge (el hook
  retira el worktree).» Plus, at T7's close (Cody, relaying Robert): «Antes del merge:
  handoff en Engram y worktree de T8 con su plan.»
- **L2** — Master plan entry: T8 in `odd/tasks/change-ledger-v2.md` (scope, tests,
  risks, deps T7, ~400 lines) with its carried items: T5 R1 (a shadowed close repeats
  its notice on every invocation — report it once, name the owning ledger and which
  file wins) and T6b (a) gate latency, (b) `.ai-specs/` ignore in consumer projects.
- **L3** — Inherited from T7 (its L5-L9), not to be re-derived:
  - `changes status` phases `integration` → `cleaned` → `merged` → `planned` → `open`
    → `workstream`; exits 0 resolved, 2 verdict blockers, 3 unknown fact; `--ledger`
    narrows output only; reads never mint `ai-specs-created`.
  - One product predicate (`gates.ProductScope`, `internal/gates/product.go`): graded,
    outside `odd/` and `openspec/`, minus `[recipes.plan-build-flow.config]
    product_exclude`; `PLAN_BUILD_GATE_PATHS` is retired.
  - Review receipt: `gentle-ai review assess … --json` via `$GENTLE_AI_BIN` or PATH,
    never decides the exit.
  - Reservations carried to T8: `--rebuild` (a rebuild is a write, so it is not
    `status`'s); store notes relayed per gate invocation, so a grade followed by a
    bind prints a non-empty note twice; `tracker-card-gate` recognizes a note by
    `StoreReport.Note`'s message tails; the three differential oracles still frozen at
    0.28.0 (`TestDifferentialSmoke`, `TestRecipeConfigureDifferentialAgainstScript`,
    `TestSkillsAddDifferentialAgainstShell`); `product_exclude` is not declared in
    `recipe.toml` (the schema oracle models scalars only); `changes workstream` does
    not print scope notes. To T11: the retired shell host ignores `product_exclude`;
    the canonical spec text still names the folder list (promotion).
  - Traps: pi-lens LSP binds the MAIN worktree (only `go build`/`vet`/`test` count);
    the gate is its own Go module; run the Python suite directly
    (`python3 -m unittest discover -s tests -p 'test_*.py'`, ~10 min, one pre-existing
    failure `test_skill_never_recommends_delete_branch`); fixture repos must commit
    `ai-specs/ai-specs.toml` in their base, since it is now a product path; the
    explorer subagent cannot read outside cwd; two writers with overlapping surfaces
    run sequentially; a merge retires the worktree and the cwd.
- **L4** — This branch was cut from `development@37414b9d` and rebased onto
  `development@1daa0408` after T7 merged (#628 squash `2200656f`, CHANGELOG #629
  `1daa0408`); the master-plan conflict with T7's carried notes was resolved by keeping
  both the T6b notes under T8 and the new T8b entry.
- **L5** — Design decisions taken at start (technical, the parent's; the T2 escalation
  record `odd/tasks/change-ledger-v2/t2-review-escalation.md` causes 1-5 are what each
  one avoids):
  1. *Where R3-R5 live.* A new pure method beside R1/R2 in `internal/gates`, fed by a
     separate collection `MergeFactsFor(root, WriteFacts)` (changed lines per product
     path from the new `planbuild.DiffLines`, `git diff --numstat -z --no-renames`
     over the same merge base; the plan's verification gaps). They are NOT appended to
     `WriteFacts.Verdict().Blockers`: the write gate (work-start, the hook path) keeps
     R1/R2 only and pays no extra git call (T6b note a). Applicability is each rule's
     own precondition on the facts: never for a workstream or an unpaired branch (S16).
  2. *R3 — size and module threshold.* Tier `none` only (no plan, no delta). Counts
     product paths only (`Scope.Path`), so a documentation or planning worktree never
     trips it (T2 r1). Lines = added+deleted over product paths; modules = distinct
     first path segment of product paths (`.` for a root file). Fires when lines >
     `lines` or modules > `modules`. CLI defaults: `lines = 1000` (the master plan's
     per-PR cap), `modules = 5`. Exits it honours: write the plan (tier light — R3 no
     longer applies) or raise the project threshold. An unanswered numstat is a note,
     never a pass and never a block.
  3. *R4 — removed public contract.* From a declaration, never from the shape of a line
     (T2 cause 3): a diff entry with status `D` whose path matches a pattern the
     project already declares in `[recipes.plan-build-flow.config] high_risk_paths`
     (read at base, as R2 does). Tier `none` only. Exit: the plan that records the
     removal (tier light). No new key.
  4. *R5 — plan whose `S#` rows carry no evidence.* Only when the change carries a plan
     that declares at least one `S#`; graded by the guardian's own
     `checkPlanVerification` (one grader per behaviour). Exit: one `## Verification` row
     per `S#` with its command, or `N/A` with its reason.
  5. *Config, two keys* under `[recipes.plan-build-flow.config]` of
     `ai-specs/ai-specs.toml`: `require = ["merge"]` (checkpoints whose posture is
     `require`) and `risk_thresholds = { lines = N, modules = M }` (either key may be
     omitted; a non-positive or non-integer value is a note and the default stays).
     A malformed `require` or an unknown checkpoint name is a note naming the accepted
     list, never silently applied.
  6. *Transition→posture map.* The checkpoint a phase is heading to: `open`/`planned` →
     `merge`; every other phase has no posture-governed transition (`merged`,
     `integration` → cleanup carries no R3-R5; `cleaned`, `workstream`, `unknown` →
     none). Accepted `require` names: `merge` (the only checkpoint with posture rules;
     the list stays the per-checkpoint mechanism of S5). Default posture `advise`:
     R3-R5 are printed as advice and never change the exit. With `merge` in `require`,
     they join the blockers and exit 2 naming the rule and its exits. A workstream has
     no checkpoint, so it is never subject to a posture (S16).
  7. *Where postures bite.* `changes status` and the new `changes next` share one
     collection (`collectStatus`); both report `checkpoint`, `posture` and `advice`.
     R1/R2 keep blocking as today in both postures. `gates premerge` is not changed in
     this unit (its grader is the artifact guardian of a change folder); re-carried.
  8. *`changes next`.* `ai-specs changes next --root <repo-root> [--json]`: the one
     next step, the checkpoint and posture, then each blocker and each advice line;
     JSON `{phase, checkpoint, posture, next, blockers, advice, unknown}`; exits
     0/2/3 exactly as `changes status`.
- **L6** — Implementation notes (worker, reviewed by the parent). Deviations from L5,
  accepted: notes come from a sibling pure method `MergeNotes(MergeFacts)`; under
  `require` a rule goes to the blockers only, never also to the advice, so one rule is
  never printed twice; `next` prints `posture: none` when the phase has no checkpoint,
  so S16 is observable; `StoreReport` gained `Path` so the shadowed note can name the
  file that wins. `changes status` renders the posture with its ten-column labels
  (`posture:  <p> at the <cp> checkpoint`, `advice:   …`); `next` keeps short labels.
  Evidence (scratch `/private/tmp/change-ledger-v2/t8`): `flow.sh` → `flow2.log`
  FLOW OK, 12 checks (workstream with `require` exits 0 with posture none; a default
  project advises R3 and exits 0 in `next` and `status`; `require = ["merge"]` exits 2
  naming R3; a raised threshold passes; R4 on a removed declared contract with R2
  satisfied by its trailer; R5 on a plan with `S1` and no verification row; the same
  plan with its row exits 0; a lowered threshold fires on a 4-line change). RED: the
  base binary answers `unknown subcommand 'next'` and its `status` exits 0 with no R3
  under `require` and a lowered threshold. The worker's differential check: the write
  gate (`plan-build-gate`, `--explain`) and `changes workstream` are byte-identical to
  base; `changes status` differs only by the requested posture line and JSON fields.
- **L7** — T8.4, the carried reservations, one disposition each:
  - T5 R1 (shadowed close repeats): *resolved in part* — the note now names the file
    that answers (`StoreReport.Path`) and the predecessor it differs from, and the
    tracker gate's tail follows it. "Once across invocations" is *re-carried to T10*:
    silencing it needs a recorded acknowledgement, which is a write, and the reads that
    print it (`status`, `next`) are read-only by contract.
  - T6b (a) gate latency: *resolved* — T8 adds no call to the write gate; R3-R5 are
    collected only by `status`/`next` (`latency.log` records base vs branch).
  - T6b (b) `.ai-specs/` ignore in consumer projects: *re-carried to T11/T12* (the
    managed ignore block lands with the docs and the release).
  - T7 `--rebuild`: *re-carried to T10* — a rebuild is a write and no derived store
    exists yet to rebuild; `status` still rejects the flag.
  - T7 store notes relayed per gate invocation (grade then bind prints twice):
    *re-carried to T10*, which owns the gate's removals and its single note owner.
  - T7 tail recognition in `tracker-card-gate`: *kept* — the tails are now pinned by
    `location_test.go` on both sides of the module seam.
  - T7 differential oracles frozen at 0.28.0: *re-carried to T12* (the release refreshes
    them; they fail identically on base).
  - T7 `product_exclude` (and now `require`, `risk_thresholds`) not declared in
    `recipe.toml`: *re-carried to T11* — the schema oracle models scalars only.
  - T7 `changes workstream` prints no scope notes: *re-carried to T11*.
  - L5 item 7, `gates premerge` does not apply the posture: *re-carried to T8b*, which
    touches the merge proof and the premerge path anyway.
