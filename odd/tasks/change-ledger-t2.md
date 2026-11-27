# change-ledger-t2 (ODD)

Unit T2 of the change-ledger-v2 redesign, the decisive one: the write gate stops
requiring a plan in OpenSpec mode and stops waiting for authorization in both
modes. It infers the tier from artifacts and refuses only on a proven refutation
that names the rule and the exits. Master plan, design, audit and the target
deltas live in `odd/tasks/change-ledger-v2.md` and `odd/tasks/change-ledger-v2/`.

## Specs

- **S1** — No mode blocks for a missing plan; the CLI infers the tier. Verbatim,
  master plan S1: «el gate de plan-build-flow NO debe bloquear por falta de archivo
  de plan, ni en ODD ni en SDD/OpenSpec, y no debe esperar autorización: debe
  INFERIR el tier, como ODD (sin plan = none/small; plan = light; plan + delta =
  standard)».
- **S2** — `none` is refutable only by objective facts, and a refusal names the
  rule and the exits. Verbatim, master plan S2: «El CLI no prueba "small": solo lo
  refuta con hechos objetivos, y si lo refuta, el bloqueo nombra la regla y las
  salidas. El agente puede planificar igual.» This unit ships R1 (a hand write to
  the canonical spec tree without its delta) and R2 (a project-declared high-risk
  target written without its evidence); R3-R5 arrive with T8 (L4).
- **S9** — `changes start --none` is inert for one version, then removed. Verbatim,
  master plan S9: «deprecar `changes start --none` (sin efecto una versión, luego
  se elimina)».
- **S16** — A worktree that is not a change owes nothing, and the CLI tells the two
  apart from facts. Verbatim, master plan S16: «Hay que tener mucho cuidado con las
  tareas que crean un worktree/workstream pero no son efectivamente un change.» For
  this unit that means: no artifact and no product-path divergence is nothing to
  grade, never a blocker.
- **S3** — One unit is one change: its own branch-paired plan, only the deltas it
  implements, promoted and archived before its merge.

## Tasks

- T2 — The write gate infers the tier and refuses only on a proven refutation.
  route: worker. commit: pending.  - scope: `internal/cli/planbuildgate.go` (drop the plan/ceremony requirement in
    OpenSpec mode; a worktree with no artifact and no product-path divergence is
    nothing to grade), the refutations R1 and R2 (R3-R5 belong to T8, L4),
    `internal/planbuild/ceremony.go` + `changes start --none` become inert with a
    deprecation notice, and the messages lose every "go ask permission" line.
  - tests: one real-flow scenario in a temp repo over the audit matrix — ODD with
    no plan → 0; OpenSpec with no plan → 0; audit worktree with no divergence → 0;
    R1 (canonical spec written by hand without its delta) → 2 naming R1; R2 (a
    project-declared high-risk target) → 2 naming R2; a leftover
    `aiSpecsCeremony = none` changes nothing; a fresh `start --none` warns and exits
    0.
  - RED: today the OpenSpec no-plan write exits 2 and the declaration is a
    precondition.
  - risks: this removes a guard. Mitigation: every refusal is now fact-based and
    the merge guardian is untouched.

## Verification

| Spec | Evidence |
| --- | --- |
| S1 | Branch binary on temp repos: OpenSpec production write with no plan → exit 0, silent; the released baseline `366049b3` build refuses it (exit 2) with the retired "write …/tasks.md and retry / run changes start --none" text. ODD with no plan → 0 on both. `go test ./...` green. |
| S2 | R1: hand write to `odd/specs/widget/spec.md` without a delta → exit 2 naming R1, the missing delta path and `ai-specs specs promote` (baseline refuses with the old wording). R4: a removed `case "--old-flag":` arm under a product path → exit 2 naming R4 and the exits (baseline 0); the same removal as a prose mention → exit 0, which is the false refusal the native review found. |
| S9 | Fresh `changes start --none --root .` → exit 0 with one deprecation line and **no** git config write (`git config --get` unset); the baseline build writes `branch.main.aiSpecsCeremony none`. A pre-existing `aiSpecsCeremony = none` → silent, no notice, no behaviour change. |
| S16 | An ODD repo with `max_diff_lines = 1`, a documentation-only diff and no artifact → exit 0 on both binaries: the diff-based refutations are skipped for nothing-to-grade while R1 still refuses a hand write. Note for T3: the production set is `lib catalog bin src`, so this repository's `internal/` work is not product divergence — the centralised discriminator must own that fact. |
| S3 | Plan only (`light`), no delta folder — see L2. Commits `a0427330` (plan), `96c7a8ee` (gate), `57c8a5fb` (review correction); pre-merge guardian green on the branch. |

Native review `review-891597c96a64468e` (risk + reliability, tier high by escalation item 3):
approved after one bounded correction that closed its six severe findings — R2 now grades the
pending hook target, R4 requires a removed public declaration, the S16 workstream skips the
diff-based refutations, and `--explain` reports the tier on the allowed canonical-spec path.

## Log

- **L1** — «Decisión de Cody (técnica, dentro del mandato): una unidad = un change
  ODD propio. Cada PR (T1, T2, T3...) lleva su plan emparejado por rama y SOLO los
  deltas que esa unidad implementa; los promueve y archiva antes de su merge, así
  odd/specs nunca declara lo que aún no existe y el guardian pasa en verde.
  odd/tasks/change-ledger-v2.md queda como plan maestro y design/audit van con él
  como contexto (fuera de odd/changes/ si el guardian los gradúa); los deltas de
  T4–T12 se crean con su unidad. No uses Aparcar ni un PR único.»
- **L2** — Delta shape for this unit: `odd/changes/plan-build-odd-companion/` is
  still an active change folder that declares its own
  `specs/plan-build-flow/spec.md`, so `ai-specs specs promote` refuses any
  plan-build-flow delta and `odd/specs/` does not exist yet in this repository.
  Archiving that folder is another change's record and outside T1–T3, so T2 ships
  `light` (plan only) like T1 and the requirement text stays in the master plan's
  target delta until the flow adopts `odd/specs/`.
- **L4** — «Nota de Cody: elegí recortar (decisión técnica mía, no de Robert:
  regístralo así). T2 entrega el tier inferido, R1/R2 y start --none inerte; R3–R5
  pasan a T8 junto a los umbrales. Actualiza el plan maestro con el movimiento y su
  motivo, y pasa la revisión nativa al candidato recortado antes del merge.»
  Motive: the two earlier lineages of this unit found ten severe findings and every
  one of them lived in the new R3–R5 machinery (size threshold, removed public
  contract, plan whose `S#` rows carry no evidence). A refusal whose exit depends on
  thresholds and recorded decisions belongs with the unit that introduces that
  configuration, so those rules ship with T8 and T2 keeps only what is decisive.
- **L5** — «Decisión de Cody (técnica, regístrala así): opción 1, rediseño del seam.
  Recolectar los hechos una vez y expresar R1/R2 como reglas puras sobre ellos, con
  --explain y el rechazo derivados del mismo veredicto; es adelantar a T2 el contrato
  Facts()/Verdict() que el plan pone en T7. Actualiza el plan maestro y el de T2 con esa
  decisión y su motivo (las cuatro rondas y sus cinco causas raíz), conserva la rama
  feat/change-ledger-t2 y lo útil de sus tests como RED, y guarda el cierre en Engram para
  que una sesión nueva retome desde ahí. No implementes en esta sesión.»
  Motive: the four lineages on this unit all closed `escalated`, and every severe finding
  is an instance of five root causes — refutation applicability inferred from the position
  of a `return`; the S16 fact owned by the CLI instead of the refutation layer; "public
  contract" decided by the shape of a line; refusals advertising exits nothing honoured;
  and the explanation emitted at the end of one path. Full record:
  `odd/tasks/change-ledger-v2/t2-review-escalation.md`.

## Redesign shape (L5)

One fact collection per write, then pure rules over it. Facts: mode, change, workstream?,
tier, pending path, production?, declared high-risk, diff entries, plan/delta presence.
Rules: R1 over the pending path and over the diff (skipped for a workstream), R2 over the
pending path wherever it lives (never skipped). The refusal and `--explain` are emitted
from the same verdict, so no path can print one and skip the other.

## Tests as the RED baseline

The 13 commits on `feat/change-ledger-t2` are kept, not squashed: their tests are the RED
for the seam. Keep them and make them pass against the new shape, without re-deriving the
expectations from the old control flow:

- `internal/cli/planbuildgate_flow_test.go` — the whole matrix: ODD and OpenSpec with no
  plan, workstream with no artifact and no divergence, R1 on a hand write to the canonical
  spec, R2 on the canonical-spec path, R2 outside the production scope, R2 inside a
  workstream, a leftover declaration, a fresh declaration, and the workstream that carries
  an inherited canonical-spec edit.
- `internal/cli/planbuildgate_test.go` — the `--explain` tier lines, the ledger block
  ordering, and the declaration cases.
- `internal/gates/refute_test.go` — `TestWriteRefutationsGradeThePendingHighRiskTarget`
  and `TestWriteRefutationsGradeAHighRiskTargetInAWorkstream`.

The code those tests cover is expected to be replaced by the seam, not extended.
