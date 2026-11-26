# The flow ledger becomes three capability ledgers with a composite view

Today one ledger file tries to represent three different things — the working tree,
the change and its planning, and the reconciliation with a tracker — and its only
automatic trigger is a heuristic over folders and markdown. Evidence: the store
holds 14 rows, all closed, last written 2026-10-08, while two shipped units
registered nothing; and `plan-build-gate` still refuses an OpenSpec production
write when no plan exists, which contradicts the owner's rule that the CLI infers
the tier instead of asking for authorization.

This change splits the ledger by capability (worktree, plan-build, tracker), each
owning its facts and its decision log and conversing only through read-only
queries, and adds `ai-specs changes status|next` as the composite view that answers
what you did, what is missing, what you may do and what you may not, with the
reason and the command. Nothing blocks by default: postures advise, `require` is
opt-in per project and checkpoint, and every refusal names the rule and the exits.

Delivery is staged so that the contradiction is fixed first, and every unit ships
as its own PR under ~1000 lines with a real-flow test and a RED.

## Tracker

- **Card**: none linked. No tracker capability is required for this change; the
  plan records the absence explicitly instead of inventing an item. If a tracker
  is enabled before implementation starts, bind it at `apply-start` as usual.

## Decision recorded

- **Inference over authorization** (owner, 2026-11-26): the pack infers
  `none|light|standard` and only refutes `none` with objective facts; `changes
  start --none` is deprecated, inert for one version, then removed.
- **Three ledgers, one entrypoint** (owner): the on-disk layout is an
  implementation detail; only the CLI shape is contractual.
- **Advise by default, require on request** (owner): high risk never blocks unless
  the project opts in, with per-checkpoint detail.
- **Supersedes** the archived `2026-11-24-plan-build-openspec-odd-align` choice to
  keep a local no-plan declaration for OpenSpec mode. The Nov-24 commit `e931c4f4`
  already implemented inference for ODD only; this change extends it to both modes.

## Order and shape

1. Tier inference core (both modes, guardian parity).
2. The decisive fix: the OpenSpec write gate stops requiring a plan; refutations
   R1–R5 name the rule and the exits; the ceremony becomes inert.
3. The workstream: a worktree that is not a change owes nothing, decided from
   facts, with `changes workstream` for the deliberate case.
4. Advisory visibility in the generated runtime wrappers.
5. Tracker store extraction with the one-version double read.
6. Worktree and plan stores, same invariants.
7. `changes status`, the read-only composite.
8. `changes next`, postures and per-project thresholds.
9. Tracker reconciliation: modes, decision menu, recommendation.
10. Removals: dead code, duplicate checkpoint, retired launcher, the matcher as a
    trigger, `## Tracker` as a requirement, `start --none` — with both of its uses
    re-homed explicitly.
11. Scenario coverage and docs.
12. Repo dogfood sync after the release.

## The non-change worktree

`changes start --none` is documented only for "this change owes no plan". The other
use — an audit, an exploration, a spike or a review done in its own worktree — has
no representation today: in ODD mode it triggers nothing, and in OpenSpec mode its
only legal path is a declaration that says something untrue about it. Removing the
verb without answering that would leave those worktrees unserved.

The plan answers it from facts instead of text: a worktree is a **change** when it
shows evidence of product work (production-path divergence, committed or not, or an
artifact naming a change), and a **workstream** otherwise. A workstream opens no
ledger row, asks for no item and owes no archive; `changes status` says so and
names the exit that turns it into a change. For the genuinely ambiguous case (a
worktree with product divergence that is deliberately not a change) the plan keeps
one declaration — `changes workstream --reason` — recorded as a plan-ledger
decision, refused when an artifact exists, reversible with `--end`. That is the
only declaration the design keeps, and its motive is that facts can prove work but
cannot prove intent.
