# change-ledger-t1 (ODD)

Unit T1 of the change-ledger-v2 redesign: the change tier becomes one derivation
that both execution modes share, so the ODD guardian stops reporting its own
`none|odd` pair and computes the same three values as OpenSpec. Master plan,
design, audit and the full target deltas live in `odd/tasks/change-ledger-v2.md`
and `odd/tasks/change-ledger-v2/`.

## Specs

- **S1** — The tier is inferred from artifacts, never declared, and both modes
  infer the same values. From the master plan S1, verbatim: «el gate de
  plan-build-flow NO debe bloquear por falta de archivo de plan, ni en ODD ni en
  SDD/OpenSpec, y no debe esperar autorización: debe INFERIR el tier, como ODD (sin
  plan = none/small; plan = light; plan + delta = standard)».
- **S2** — Only objective facts refute a tier, and a refusal names the rule and
  the exits. From the master plan S2, verbatim: «El CLI no prueba "small": solo lo
  refuta con hechos objetivos, y si lo refuta, el bloqueo nombra la regla y las
  salidas. El agente puede planificar igual.»
- **S3** — One unit is one change: its own branch-paired plan, only the deltas it
  implements, promoted and archived before its merge (L1). A unit that changes no
  contract carries no delta: it ships `light`, with the plan alone.

## Tasks

- T1 — One shared tier derivation, used by the guardian in both modes; ODD stops
  reporting `none|odd`. route: worker. commit: pending.
  - scope: new `internal/tier` (the vocabulary and `Derive(hasPlan, hasDelta)`);
    `internal/gates/tiers.go` aliases it; `CheckPremergeOdd` derives the tier from
    the artifacts it already validates; the ODD-only `TierOdd` constant is deleted.
  - no behaviour change at the write gate: it derives no tier at all today, so
    inference stays out of this unit and lands with the gate rewrite (T2).
  - tests: RED-first table over ODD and OpenSpec across `none|light|standard`
    (`internal/gates/tier_table_test.go`); six tier-token expectations moved; the
    whole suite green.
  - real flow: the branch binary grades an ODD plan-only change `light` and a
    plan+promoted-delta change `standard`, where the base binary said `odd`, with
    blocker and note text byte-identical.

## Verification

| Spec | Evidence |
| --- | --- |
| S1 | `internal/gates/tier_table_test.go` (both modes, three values); real-flow run with the branch binary: `premerge-guardian: OK (light, change demo)` and `(standard, change demo)` where `36438150` printed `(odd, change demo)`. |
| S2 | Diff of the same run: only the tier token differs; every blocker and note line is byte-identical between the base and the branch binary. |
| S3 | Plan only, tier `light`, no delta folder — see L3: the tier requirement ships with T2's delta because another active change already declares `specs/plan-build-flow/spec.md` and blocks the promotion. Pre-merge guardian: `OK (odd, change change-ledger-t1)` — that `odd` token is the released 0.27.1 binary that ships today, and it is exactly the label this unit retires; the branch binary prints `(light, change change-ledger-t1)` for the same tree (S1 row). |

## Log

- **L1** — «Decisión de Cody (técnica, dentro del mandato): una unidad = un change
  ODD propio. Cada PR (T1, T2, T3...) lleva su plan emparejado por rama y SOLO los
  deltas que esa unidad implementa; los promueve y archiva antes de su merge, así
  odd/specs nunca declara lo que aún no existe y el guardian pasa en verde.
  odd/tasks/change-ledger-v2.md queda como plan maestro y design/audit van con él
  como contexto (fuera de odd/changes/ si el guardian los gradúa); los deltas de
  T4–T12 se crean con su unidad. No uses Aparcar ni un PR único.»
- **L2** — Robert's per-unit mandate: implement → validate (focused tests +
  real-flow proof with the branch binary + RED) → native review before the merge →
  PR to `development` → merge → `ai-specs sync` with its own PR. No worktree or
  branch is retired by hand.
- **L3** — T1 ships `light` (plan only) instead of carrying a delta, for a fact
  found while promoting: `odd/changes/plan-build-odd-companion/` is still an active
  change folder in this repository and declares its own
  `specs/plan-build-flow/spec.md`, so `ai-specs specs promote` refuses the
  collision. That folder is outside T1–T3 and is left untouched. The requirement
  this unit implements (one tier derivation, both modes) is drafted at
  `odd/tasks/change-ledger-v2/specs/plan-build-flow/t1-tier-derivation.md` and is
  promoted with T2's delta, which owns the same domain and the same requirement
  family. Nothing in `odd/specs/` claims behaviour that does not exist.
