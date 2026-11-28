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
  facts, with the same exits (0/2/3) and `--json`. Specs: S5, S16. route: pending.
  commit: pending.
- T8.2 — Postures: the transition→posture map; `advise` by default; `require` opt-in
  per project with a per-checkpoint list in `ai-specs/ai-specs.toml`; exit 2 only when
  opted in. Keep the config to two keys. Specs: S5. route: pending. commit: pending.
- T8.3 — Refutations R3-R5 (L11 of the master plan): the size and module threshold,
  the removed public contract, the plan whose `S#` rows carry no evidence; thresholds
  as CLI defaults overridable per project. Specs: S10. route: pending. commit: pending.
- T8.4 — Carried reservations (L3): resolve or re-carry each one with a reason.
  route: pending. commit: pending.
- T8.5 — Validation: real flow (default project never blocks even with high risk;
  `require = ["merge"]` exits 2 naming the rule; thresholds override defaults; a
  workstream is never subject to a posture), RED, both Go modules,
  `ledger_cleanup_flow.sh`, the full Python suite. route: pending. commit: pending.

## Allowed edit surfaces

To be confirmed when T8 starts (expected: `internal/cli/`, `internal/gates/`,
`internal/planledger/`, `catalog/recipes/plan-build-flow/`,
`catalog/recipes/worktree-flow/gate/ledger/`, `catalog/recipes/worktree-flow/gate/runner/`,
`tests/`, `odd/tasks/change-ledger-t8.md`).

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
