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

- T2.1 — One fact collection and pure rules: the `Facts()`/`Verdict()` seam in
  `internal/gates`, with the CLI reduced to collection plus one print path.
  Specs: S1, S2, S16. route: inline. commit: pending.
  - scope: new `internal/gates/writefacts.go` holding `WriteFacts` (mode, change,
    target, production, diff entries, declared high-risk patterns, plan/delta
    presence, risk trailer, workstream) and `Verdict()` as pure R1/R2 rules over
    it; `internal/cli/planbuildgate.go` keeps only event parsing, the non-graded
    scope (agent config, planning artifacts), the work-start ledger and one print
    path for both the refusal and `--explain`. `WriteRefutations`,
    `HighRiskRefutation` and `CanonicalSpecRefusal` disappear: no rule reads the
    position of a `return` and no caller keeps a second copy of the S16 fact.
  - tests: the four-round tests stay as the RED baseline and keep their
    expectations; two seam-contract tests are added — the same verdict is what
    `--explain` reports and what the refusal prints (no path can print one and
    skip the other), and the workstream fact is computed by the fact layer, so a
    caller cannot pass it in.
  - RED: today no fact collection exists, `WriteRefutations` takes `workstream` as
    a caller-supplied argument, and the explain line is emitted on two separate
    paths of the CLI.
  - risks: this touches the refusal path of a hook. Mitigation: the matrix in
    `planbuildgate_flow_test.go` is unchanged and stays the acceptance test.
- T2.2 — Validate T2 against the released baseline and the real hook flow.
  Specs: S1, S2, S9, S16. route: inline. commit: pending.
- T2.3 — Native review, PR and merge, then `ai-specs sync`.
  Specs: S3. route: inline. commit: pending.

## Verification

| Spec | Evidence |
| --- | --- |
| S1 | Branch binary on temp repos: OpenSpec production write with no plan → exit 0, silent; the released baseline `366049b3` build refuses it (exit 2) with the retired "write …/tasks.md and retry / run changes start --none" text. ODD with no plan → 0 on both. `go test ./...` green. |
| S2 | Seam first: `--explain` on a refused write prints `plan-build-gate: tier none; refutations R1` before the refusal, both read from the one `Verdict` (baseline rejects `--explain` as an unexpected argument, and the pre-seam code refused before reaching its explain line — that was the RED). R1: hand write to `odd/specs/cap/spec.md` with no delta → exit 2 naming R1, `odd/changes/thing/specs/` and `ai-specs specs promote` (baseline refuses with the old wording). R2: `docs/deploy.md` under a declared `high_risk_paths` → exit 2 naming R2, the path and the `Risk: item N` exit (baseline 0). R3–R5 moved to T8 (L4). |
| S9 | Fresh `changes start --none --root .` → exit 0 with one deprecation line and **no** git config write (`git config --get` unset); the baseline build writes `branch.main.aiSpecsCeremony none`. A pre-existing `aiSpecsCeremony = none` → silent, no notice, no behaviour change. |
| S16 | An ODD repo with a documentation-only diff and no artifact → exit 0 on both binaries: the diff-based refutations are skipped for nothing-to-grade while R1 still refuses a hand write. In the branch binary the same worktree answers `--explain` with `tier none; refutations none`, and a worktree that adds a product path is graded again (a worktree that adds `src/` *and* a canonical spec edit is refused as R1 — the discriminator is conservative). Note for T3: the production set is `lib catalog bin src`, so this repository's `internal/` work is not product divergence — the centralised discriminator must own that fact. |
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
- **L6** — Seam implemented as decided in L5, three commits: `T2.1` the factor
  (`internal/gates/writefacts.go` + the CLI reduced to one print path), `T2.2`
  validation evidence, `T2.3` review and merge. The old helper functions
  (`WriteRefutations`, `HighRiskRefutation`, `CanonicalSpecRefusal`, the CLI's
  `planBuildGateWorkstream`/`planBuildGateInferredTier`/`planBuildGateRules`) are
  deleted rather than extended, because each of them held a copy of one of the
  five root causes.

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
- **L7** — Native review `review-00f522ad421aeb6a` (risk + reliability, tier high by
  escalation item 3) returned `correction_required` with four blockers and one
  critical, all answered in one bounded correction (plan: 180 diff lines). R1-001 and
  R3-001 were one defect: the diff-based R1 never read `HasDelta`, so a branch that had
  promoted its rule and committed the delta behind it was refused forever — both
  triggers now share that one exit. R1-002: R2 graded a workstream's diff, so a
  committed high-risk edit a workstream cannot answer trapped every later write; the
  S16 fact now covers both diff-based rules and the pending target stays graded.
  R1-003: the whole odd tree was exempt, so a declared high-risk path inside it was
  invisible — only the planning folders (odd/changes, odd/tasks, openspec/changes) are
  exempt now. R1-004/R3-003: `--explain` was silent on an exempt write — it now reports
  that the target is not graded. R3-002 (non-blocking, recorded): the openspec R2 exit
  now says the plan must be committed, because only a plan this diff contributes counts.
  Evidence: four new tests, `go test ./...` green, and the real-flow matrix is
  byte-identical to the pre-correction run.
- **L8** — The corrected candidate is a fresh target, so the fix was reviewed as
  `review-601dd5b074549a01` (the provider reported the corrected tree `unrelated` and
  offered `fresh_target_ready`). It returned three more blockers, all of the same
  family: the graded scope still had two owners. Fixed in one bounded correction
  (plan: 180 diff lines): `gates.GradedPath(rel)` is now the one scope predicate for
  both the pending target and the diff, so an ungraded plan file can neither answer
  nor require a refutation; the work-start checkpoint moved ahead of the planning
  exemption and is keyed on the production scope alone, so it fires for a planning
  write inside `PLAN_BUILD_GATE_PATHS` in either mode; and R2's openspec exit names an
  unpaired branch instead of printing an empty plan path. Its two non-blocking notes:
  the unpaired-branch message covers the second, and the first — the workstream
  discriminator does not read *uncommitted* product divergence — is T3's own scope
  ("product-path divergence from the integration base, committed or uncommitted"), so
  it ships there with the shared discriminator.
- **L9** — **Blocker, needs a decision from Robert.** Two review rounds on T2 each
  returned `correction_required`, and every finding is answered in the tree:
  `review-00f522ad421aeb6a` (5 findings, all fixed) and `review-601dd5b074549a01`
  (3 findings, all fixed). Evidence at this point: `go test ./...` green, the
  real-flow matrix byte-identical to its pre-correction run, pre-merge guardian green.
  The correction's *verification* slot cannot be captured: it is a
  `review.capture-validation` input for `provider_targeted_validator`
  (`domain: artifact_path_or_stdin`, `--input={{value}}`), and the Pi facade rejects
  the exact provider binding — `gentle_review_capture` with "collectBinding is missing
  or stale for current STATUS", `gentle_review_capture_group` with "current STATUS does
  not offer an exclusively materialize reviewer capture group", and
  `gentle_review_run` resume with "capture-route-registration-rejected:
  selector-mismatch". No `approved` receipt is reachable through the facade, and T2
  does not merge without one. Nothing is lost: `feat/change-ledger-t2` holds the unit,
  this plan holds its evidence, and both lineages keep their findings on record.
- **L10** — Cody's independent review of `54e300f0` (`/private/tmp/t2-review-cody.md`) said NO
  APROBAR: one blocker and five lesser points, and answered the open question — **«decidí yo
  que una escritura de planificación nunca pasa por work-start»** (a planning artifact is not
  product work). Fixes in one commit: (1) *blocker* — R2 in openspec was unsatisfiable because
  `WriteFactsFor` filtered the diff through `GradedPath`, which drops `openspec/changes/**`,
  and `planInDiff` then looked for a plan it had just discarded; plan provenance is now its
  own fact (`PlanInDiff`, read from the whole diff) while the graded filter only decides which
  paths may fire a rule, with a positive openspec test whose RED reproduced the contradiction
  (`tier light` next to "carries no plan"); (2) an unread base ref is a reported fact
  (`Verdict.Notes`, printed on stderr) instead of silent silence, and an unborn HEAD stays
  silent because there is nothing to compare against; (3) the deprecation notice no longer
  names `changes workstream`, which does not exist until T3; (4) `--explain` reports the
  verdict before the work-start block, so no refusal path explains nothing; (5) the dead
  `WriteRefutations` and `HighRiskRefutation` are gone; (6) the workstream that skips the
  diff-based R1 stays noted for T3, with the uncommitted-divergence gap. Evidence: full suite
  green, the real-flow matrix keeps every earlier case and gains the openspec-committed-plan
  and the no-base cases, pre-merge guardian green. No native review: Cody reviews it himself.
- **Preexisting, out of T2** (Cody's report): `ODD/Specs/…` with different capitalisation is not
  caught by `isCanonicalSpec` — the baseline behaves the same, so it is not a T2 regression and
  belongs to the unit that owns canonical-path matching.
