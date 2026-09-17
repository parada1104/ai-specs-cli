# Proposal: Tracker-only, Go-owned Ledger foundation

Change: `tracker-ledger-foundation` · Tier: Full · Explore: `explore.md` (approved)
Proposal question round: closed — answers recorded as D15–D19
Tracker: epic card 125 (context/tracking only — see `## Tracker`)

## Why

Today "what is the tracked item for this work" has no single answer. It is spread
across prose and three independent graders:

- a hand-written `## Tracker` block in a change's `proposal.md` (or `tasks.md`),
  validated by `lib/_internal/trello_link.py`, re-implemented as a copy-pasted
  heredoc inside `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`,
  and graded a third time by `doctor.py::_check_tracker_card_link()`;
- a `tracker.none` exemption file with a one-line reason;
- lifecycle hints in the runtime brief and in skill prose.

There is no durable record, no work identity, no per-item evidence, and no
reconciliation. Consequences we pay for now:

1. **Two rules that can disagree.** The same change can be "linked" to one grader
   and invalid to another. Every new tracker rule risks becoming a fourth grader.
2. **Work state is unverifiable at the moments that matter.** Pre-merge and
   archive decisions are made from the *presence of a string*, not from reconciled
   evidence about local state, remote provider state, code/change state, and
   Git/PR state.
3. **Silent dormancy.** Whether the tracker capability is actually *bound* is
   computed by `resolve_bindings()` and written to a `mktemp` file that
   `lib/sync.sh` deletes on exit. At agent runtime nothing on disk records it, so
   a tracker-aware guard can only guess from a `bootstrap-ready` marker keyed by
   project **realpath** — which means a prescribed worktree reads as unbound and
   the whole capability quietly does nothing exactly where the workflow demands it.
4. **Vendor-shaped rules leak into shared surfaces.** Trello vocabulary
   (`board_id`, list names, labels, `card_id`) is the only shape the current
   guards understand, so a Jira or GitHub-issues provider would need those guards
   rewritten.

The Ledger foundation is the smallest durable answer: **one Go-owned, per-work-
identity record of the primary tracker item, its evidence, its lifecycle state,
and its conflicts — graded once and consumed at the five checkpoints that already
exist.** It is deliberately *tracker-only*: it proves the model on the one
capability whose truth is actually split today, and it does not invent a generic
ledger framework on one data point.

## What Changes

**A new Ledger mode in the one existing Go product.** `catalog/recipes/worktree-flow/gate`
is this repository's only Go module: dependency-free, four-arch, SHA-256-pinned,
release-verified, with a `0`-allow / `2`-block contract plus `--selftest` and a
JSON `--explain` diagnostic. The ledger lands as a mode of that same verified
binary — work-identity derivation, evidence model, conflict predicate, lifecycle
state machine, checkpoint verdicts — reusing the existing launcher, acquisition,
digest, receipt, cache, release job, and parity corpus verbatim. No second release
asset, no second trust root, no third-party Go dependency. (Repo precedent:
`2026-08-18-worktree-cleanup-go` and `card-46-asset-freshness` both chose one
binary plus a command mode for exactly this reason.)

**A durable activation witness.** Sync persists the *already computed* bound
`tracker` fact into a durable location instead of letting it die in a temp file.
Python keeps producing it (that is the sanctioned bridge role); Go only reads it.
Ambiguous and unbound states are recorded as what they are and surface through
`doctor` — nowhere else (D15).

**Existing hosts invoke; they stop deciding.** `doctor`, the two tracker/worktree
hooks, and `lib/_internal/premerge_guardian.py` call the Go verdict and consume
JSON. The three surviving copies of the `## Tracker` validity rule collapse to one
authoritative Go grader; the legacy copies delegate or become parity-tested
shadows. This is the merged project-level Python→Go strangler policy applied at
the seam this change actually touches (`ai-specs/ai-specs.toml`
`[brief].workflow_rules`).

**A synthetic tracker provider, no writes.** A `test-*` fixture recipe under
`tests/fixtures/recipes/` provides `tracker`, plus a pinned JSON corpus of
`(work identity + evidence) → (expected verdict, expected conflict)` — the shape
already proven by `tests/fixtures/worktree-gate-corpus/`. The first slice
records and reconciles evidence; it performs **no** MCP/API create or update.

**Unchanged on purpose:** provider item vocabulary and artifact text stay
provider-private behind `recipe.toml` configuration; `openspec/**` writes are
never blocked; the `## Tracker` block stays the human-facing artifact section; no
historical archive is migrated or rewritten.

## Confirmed decisions

Carried from the approved explore plus the user-confirmed handoff. Specs and
design elaborate these; they do not reopen them. The answered proposal question
round is folded in as D15–D19.

| # | Decision |
|---|---|
| D1 | Tracker-only Ledger foundation. No generic multi-capability ledger framework. |
| D2 | No worktree ledger in this slice. Reconsider worktree identity only after the pending ask-mode release and CWD-fix release produce evidence. |
| D3 | The Ledger and all authoritative lifecycle / reconciliation / predicate / state-machine logic are Go-owned, per the merged project-level Python→Go strangler policy in `ai-specs/ai-specs.toml` `[brief].workflow_rules`. |
| D4 | Reuse the existing verified Go product, distribution, and trust path by **adding a Ledger mode** to the existing Go product. Tracker semantics must not couple to worktree semantics. A second release asset is not created unless later design evidence proves it unavoidable. |
| D5 | Existing Python call sites may remain only as thin acquisition / compatibility / JSON bridges. No new Python grader and no duplicated decision logic. |
| D6 | Activation happens only when `resolve_bindings` yields a bound `tracker`. A declaration alone is *supply*. Ambiguous or unbound stays **dormant but visible** and never guesses a provider. |
| D7 | One primary native item per work identity. Identity is repository Git common-dir + current branch, enriched by active change identity. Detached HEAD, branch reuse, and collisions require explicit human reconciliation — never heuristic selection. |
| D8 | The ledger reconciles local ledger state, remote provider evidence, code/change state, and Git/PR evidence. Conflicts are resolved by a human, not by a default winner. |
| D9 | Five checkpoints: `work-start`, `apply-start`, `pr-review`, `pre-merge`, `archive-close`. Work-start is before proposal when SDD/OpenSpec is in play, and before the first non-read-only write when it is not. No checkpoint requires a change folder. |
| D10 | Modes `always`, `ask`, `warn`. `always`: asks native type, creates/links before work, syncs lifecycle, and blocks missing or conflicted state pre-merge. `ask`: prompts at checkpoints and records an explicit opt-out while still allowing merge. `warn`: reports only. No human decision is ever inferred. |
| D11 | The first slice records and reconciles evidence but performs no MCP/API create or update. Provider item vocabulary and artifact text remain provider-private. A synthetic provider fixture is required; the current providers need not become fully functional. |
| D12 | The binding witness must become durable, because today's `resolve_bindings` output is temporary. The exact witness and storage shape is a design/spec decision. Avoid a generic universal field schema. |
| D13 | Forward active work only. No historical archive migration or rewrite, and no generic artifact revalidation. |
| D14 | Providers are incidental. Provider configuration stays in `recipe.toml`; Trello-shaped fields are not promoted into ledger core or into a universal tracker schema. |
| D15 | Dormancy visibility is **`doctor` only** — no static `AGENTS.md` / runtime-brief dormancy line. Checkpoint behavior stays governed by the lifecycle protocol, not by brief prose. |
| D16 | Conflicts are adjudicated by a **human at the terminal**: the ledger presents the local / remote / code / Git evidence, the human decides, and that decision is **persisted in the ledger**. |
| D17 | Branch reuse after closed work starts a **new primary item** for the new work / change identity. A closed item is never reopened by heuristic. |
| D18 | First adoption mode is **`warn`**: a project observes verdicts before it ever sees a prompt or a block. Promotion to `ask` or `always` is an explicit human configuration choice. |
| D19 | An `ask`-mode opt-out is **checkpoint-scoped**: it covers the checkpoint that was answered, and the next checkpoint prompts again. No opt-out carries forward implicitly. |

## Scope

### In

- **Go ledger core** (new files in the existing module, stdlib only): work-identity
  derivation (Git common-dir + current branch, change-identity enrichment), the
  evidence model, the conflict predicate, the lifecycle state machine, the five
  checkpoint verdicts, JSON diagnostic output, and `--selftest` coverage of the
  new invariants.
- **Durable activation witness** written by the existing sync/materialization path
  from the already-computed resolved bindings, using the repo's atomic-write
  convention, and read by the Go mode.
- **Host invocation at the existing checkpoints**: `plan-build-gate.sh`,
  `tracker-card-gate.sh` (path + shell kinds), `premerge_guardian.py`
  (`pre-archive` and `pre-merge`), and `doctor.py` consume the Go verdict instead
  of re-deciding.
- **One grader for `## Tracker` validity**: `trello_link.py`, the
  `tracker-card-gate.sh` heredoc twin, and `doctor._check_tracker_card_link()`
  delegate to the Go predicate or are held by parity tests.
- **Build/verify plumbing in the same work unit**: `tests/run.sh`,
  `tests/validate.sh`, `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, and
  the release workflow learn the ledger surface together with the code, so the new
  logic cannot ship uncompiled, unformatted, or unverifiable.
- **Synthetic tracker provider recipe** in `tests/fixtures/recipes/` (gated by
  `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES`) plus a pinned verdict corpus.
- **Docs and brief text** where the contract becomes user-visible:
  `docs/capabilities.md`, hook/runtime docs, the tracker recipe README, and
  CHANGELOG.

### Out (non-goals)

- A generic or multi-capability ledger framework, ledger "plugin" API, or a
  second ledger for another capability (D1).
- A worktree ledger or worktree-specific identity work (D2).
- A second Go module, second binary, second `SHA256SUMS`, or second release asset
  (D4).
- MCP/API create, update, move, comment, or label calls from the ledger (D11).
- Promoting provider fields (`board_id`, `default_list`, `epic_list`,
  `board_isolation`, Trello list/label names) into ledger core item fields or into
  the `## Tracker` contract (D14).
- New Python graders, duplicated predicates, or a broad Python→Go migration
  started here (D3, D5).
- Migration, rewrite, or blanket revalidation of historical archives and
  in-flight changes (D13).
- UI, dashboards, reporting, analytics, or retention/compaction policy beyond
  what the first slice needs to stay reviewable.
- Making Trello (or any real provider) fully functional through the ledger in
  this slice.

## Affected areas

| Area | Change |
|---|---|
| `catalog/recipes/worktree-flow/gate/` (single Go module) | New ledger mode: identity, evidence, conflict predicate, lifecycle state machine, checkpoint verdicts, JSON output, extended `--selftest`. Worktree gate behavior and exit-code contract unchanged. |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`, `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | Reach the ledger through the existing verified-binary resolution; never compile, never download on the hot path; preserve the one-line offline degradation. |
| `lib/_internal/recipe-materialize.py` (`resolve_bindings`, `build_resolved_config`) | Produce the durable bound-`tracker` witness in addition to the temp resolved config. Stays the single binding producer. |
| `lib/sync.sh` | Witness survives the `RESOLVED_CONFIG_TEMP` exit trap. |
| `lib/_internal/gate_binary.py` | Reused verbatim as the sanctioned acquisition/verification bridge (digest → receipt → cache → fail-open warn). |
| `lib/_internal/premerge_guardian.py` | Invokes the Go ledger verdict at both stages; its own tier-minima and verify-evidence logic is untouched. Must tolerate a cold cache / unknown `AI_SPECS_HOME` (it ships in `~/.ai-specs`). |
| `lib/_internal/doctor.py` | Reports ledger state and dormant/ambiguous states; stops being a second `## Tracker` grader. |
| `lib/_internal/trello_link.py` | Delegates validity or remains a parity-tested legacy alias. |
| `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` | Work-start checkpoint reaches the same predicate. |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | Provider-private configuration home (mode/ledger wiring stays here, not in ledger core). See compatibility for the `off`/`ask` vocabulary question. |
| `catalog/recipes/*/skills/**`, `commands/pr-create.md` | Checkpoint wording points at the ledger verdict rather than restating the validity rule. |
| `docs/capabilities.md`, `docs/runtime-hooks.md` (or equivalent), README, `CHANGELOG.md` | Ledger contract, dormancy visibility, provider-adapter boundary. |
| `tests/run.sh`, `tests/validate.sh` | Learn the ledger test/format surface in the same commit (both hardcode the existing Go path today). |
| `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, `.github/workflows/release-worktree-gate.yml`, `catalog/recipes/worktree-flow/bin/SHA256SUMS` | Build/vet/test/verify the ledger mode on the same four-arch matrix and trust root. |
| `tests/fixtures/recipes/` (new synthetic tracker provider), `tests/fixtures/…corpus/` (new pinned corpus) | Hermetic proof of predicates and conflicts; mirrors `worktree-gate-corpus`. |
| `tests/test_worktree_gate_parity.py`, `tests/test_tracker_card_gate_hook.py`, `tests/test_premerge_guardian.py` | Parity at each migrated seam; no behavior regressions at existing hosts. |
| `openspec/specs/…` (next phase) | Spec deltas for the ledger lifecycle, activation witness, doctor/brief surfaces. |

## Compatibility

- **One binary, one trust root.** Same four targets, same `SHA256SUMS`, same
  version-keyed cache layout, same verified-before-executed rule. Nothing about
  the worktree gate's digests, tests, or behavior is edited here beyond adding a
  mode.
- **Existing gate semantics preserved.** `0` allow / `2` block, fail-open on
  parse/git/IO/missing-binary errors, and "`openspec/**` is never blocked" all
  stay; a ledger that cannot be evaluated must not become a new way to break a
  worktree.
- **One grader, no semantic drift.** Where a Python copy is kept for
  distribution, it must produce the same verdict as Go or the parity test fails;
  a legacy copy may not stay authoritative.
- **Config vocabulary is an explicit question.** The tracker gate today is
  `off | warn | always` while the ledger modes are `always | ask | warn`. The
  mapping (including what happens to a project — like this one — that sets
  `gate_mode = "warn"`) is a spec/design decision, not a license to change this
  project's dogfood posture silently. Existing `ai-specs/ai-specs.toml`
  (`gate_mode = "warn"`) and `openspec/config.yaml` (`tracking.gate_mode: warn`)
  stay warn unless a human decides otherwise.
- **Artifact contract unchanged.** The `## Tracker` block (`card_id` + `url`,
  `tracker.none` exemption) keeps its current shape for authors and for
  `openspec/config.yaml` consumers; the ledger may make it *presentation* of a
  graded record, but does not require existing changes to rewrite it.
- **Dormant projects are unaffected.** No bound `tracker` means no ledger
  behavior, and the reason surfaces through `doctor` only (D15); the runtime brief
  gains no per-project dormancy line.
- **Adoption is observational first.** A newly bound project's first posture is
  `warn` (D18), so verdicts can be read before any checkpoint prompts or blocks,
  and an `ask` opt-out covers only the checkpoint answered (D19), so silence is
  never carried across the lifecycle.
- **`premerge_guardian.py` lives in the CLI install**, so the witness and verdict
  path must work from both a project checkout and a cold `~/.ai-specs` install.

## Risks

| Risk | Mitigation |
|---|---|
| **Silent dormancy in worktrees** — the cache key is `sha256(realpath)`, so an unsynced worktree reads as unbound. Highest practical risk, and it argues for a project-local primary witness with the cache path as fallback. | Decide the witness location in design with this failure mode as the named test; `doctor` must render the dormant reason. |
| **A fourth `## Tracker` grader appears** during implementation (the policy's exact failure mode). | One Go authority; each legacy copy delegates or is parity-tested; no new Python predicate. |
| **Layering smell**: tracker logic physically inside the `worktree-flow` recipe tree. | Keep tracker and worktree semantics decoupled (D4). Physical module home is architecture question A1, decided on its own merits — not smuggled in. |
| **Release-asset freshness / unverifiable binary** if any new seam bypasses the digest path. | No new asset; build, verify-sums, and release CI updated in the same commit as the mode. |
| **Hot-path cost** — hooks run on every edit; re-deriving catalog + TOML bindings per invocation is wasteful. | Durable witness read, not re-derived. |
| **Strict pre-merge vs today's fail-open** discipline: making pre-merge strict turns a cold cache into a merge-eligibility change. | Write the exemption down explicitly in the spec (what "cannot evaluate" means at pre-merge per mode). |
| **Synthetic-provider realism gap**: a fixture proves predicates but not that real provider evidence maps cleanly. | Adapter boundary (A7) kept narrow and provider-private; the gap is recorded, not papered over with a live integration. |
| **Ledger growth in a committed path** collides with archive-tail's dated move and guardian folder minima, and D17 means each new work identity adds an item rather than reopening a closed one. | Store shape is a design decision (A3) with retention/size named; careless placement inside `openspec/changes/**` is rejected up front. |
| **Scope creep toward "while we're here" Python migration.** | Strangler policy is touch-driven: migrate only the seams this change touches. |

## Rollback

Fully additive and reversible by revert:

1. Revert the change branch / PR. The ledger mode disappears from the Go binary;
   the existing worktree gate path and its digests are untouched, so no asset
   churn is needed.
2. Remove the durable-writer step in sync. A missing witness yields the documented
   dormant state — hosts must treat "no witness" as *inactive plus visible*, never
   as `bound`, so an abandoned ledger cannot resurrect as a false block.
3. Legacy `## Tracker` graders stay in place as delegating or shadowed copies, so
   rollback restores today's behavior without recreating deleted logic.
4. No data migration exists in either direction; nothing recorded historically is
   rewritten.

## Success Criteria

1. Ledger decisions come from a mode of the single existing Go product: no second
   Go module, no second release asset, no new third-party Go dependency, and the
   existing four-target `SHA256SUMS` trust root remains the only verified
   acquisition path.
2. Activation is derived from a durable bound-`tracker` witness: declaration-only,
   ambiguous, and unbound projects are dormant **and** explain themselves through a
   `doctor` finding (D15), with no new static `AGENTS.md` / runtime-brief dormancy
   line, and no state causes the ledger to guess or synthesize a provider.
3. Exactly one primary native item is representable per work identity (Git
   common-dir + current branch, enriched by active change identity): detached HEAD,
   unborn branch, and collision inputs resolve to an explicit human-reconciliation
   outcome rather than a heuristic choice, and a branch reused after an item closed
   yields a new primary item instead of reopening it (D17).
4. All five checkpoints (`work-start`, `apply-start`, `pr-review`, `pre-merge`,
   `archive-close`) reach the same Go predicate through their existing hosts and
   return the same verdict for the same fixture input; work-start fires before
   proposal with SDD/OpenSpec and before the first non-read-only write without it,
   and no checkpoint requires a change folder.
5. Mode behavior matches D10 and D18/D19 exactly: `always` asks the native type,
   creates or links before work, syncs lifecycle, and blocks missing or conflicted
   state at pre-merge; `ask` prompts at each checkpoint and records an explicit
   opt-out **scoped to that checkpoint** while still allowing the merge, so the next
   checkpoint prompts again; `warn` reports only; the first posture for a newly
   bound project is `warn`, and promotion to `ask` / `always` requires an explicit
   human configuration change; and no human decision is inferred in any mode.
6. Evidence reconciliation covers local ledger state, remote provider evidence,
   code/change state, and Git/PR evidence; every disagreement is presented to a
   human at the terminal with that evidence, and the resulting decision is persisted
   in the ledger (D16); and the slice performs no MCP/API create or update call.
7. `## Tracker` validity has one authoritative grader: each legacy copy in
   `trello_link.py`, `tracker-card-gate.sh`, and `doctor.py` either delegates to
   the Go predicate or is covered by a parity test that fails on divergence.
8. A synthetic tracker provider recipe under `tests/fixtures/recipes/` and a
   pinned `(identity + evidence) → (verdict, conflict)` corpus exercise the
   predicates, conflicts, and dormancy states with no MCP, network, or live board.
9. Every architecture question in `## Architecture questions` is answered in
   specs or design (or explicitly deferred with a reason), and the confirmed
   decisions D1–D19 are neither widened nor reopened by the spec set.
10. The ledger's build, format, test, digest, and release plumbing is updated in
    the same change: `./tests/validate.sh` passes, Go vet/test run in CI for the
    ledger surface, and `scripts/verify-gate-sums.sh` remains green.
11. Existing behavior is preserved where this change does not intentionally
    touch it: worktree-gate verdicts, gate exit-code contract, fail-open
    blast radius, `openspec/**` non-blocking writes, this project's `warn`
    dogfood posture (consistent with `warn`-first adoption, D18), and all archived
    changes.

## Architecture questions

Product decisions are locked above; these are vehicle/design decisions for the
spec and design phases (labels map to explore Unknowns U1–U11).

- **A1 — Physical home of the ledger Go code** (U1): a mode inside
  `catalog/recipes/worktree-flow/gate`, or relocation of the single module to a
  CLI-owned neutral path. Relocation touches the trust root and must be judged on
  its own merits; either way D4 (one binary, one asset) holds.
- **A2 — Canonical identity string and non-derivable cases** (U2): exact
  common-dir + branch representation, and the outcome when identity cannot be
  derived at each checkpoint.
- **A3 — Where the ledger record lives** (U3, U10): project-committed file,
  project-local gitignored file, CLI cache keyed by project, or memory. Highest-
  leverage open question; must satisfy reviewability, worktree reality
  (realpath-keyed cache risk), and the archive-tail/guardian folder rules, plus a
  stated size/retention ceiling. Engram is not final authority under the project's
  conflict policy.
- **A4 — Witness shape and durability** (U12/D12): what sync persists, where, in
  what atomic form, and how dormant/ambiguous/declared-not-bound states are
  encoded — without a universal field schema.
- **A5 — "One primary item" over time** (U4) — *product answer: D17 (new primary
  item on branch reuse; never a heuristic reopen)*. Remaining vehicle question: how
  a superseded item is represented in storage and what the retention ceiling is
  (feeds A3).
- **A6 — Conflict representation and resolution surface** (U5) — *product answer:
  D16 (human at the terminal, evidence presented, decision persisted in the
  ledger)*. Remaining vehicle question: append-only event vs resolved field, and the
  exact evidence rendering per checkpoint.
- **A7 — Provider adapter boundary** (U8): core fields vs opaque provider
  payload, and the rule that keeps provider vocabulary out of the artifact
  section and the `## Tracker` contract.
- **A8 — Ledger vs the `## Tracker` artifact section** (U9): which is
  authoritative and how the other becomes presentation or a parity shadow
  (only one grader may be authoritative).
- **A9 — Mode granularity and escape hatches** (U6) — *product answers: D18
  (`warn` first) and D19 (checkpoint-scoped opt-out)*. Remaining vehicle question:
  one mode for the ledger vs per-checkpoint modes, the exact strictness at
  pre-merge, how a genuinely unevaluable ledger is reported there, and how today's
  `off | warn | always` tracker enum reconciles with `always | ask | warn`.
- **A10 — Dormant-but-visible rendering** (U7) — *product answer: D15 (`doctor`
  only; no brief line)*. Remaining vehicle question: `doctor` severity and wording
  for declared-not-bound vs ambiguous vs unbound.
- **A11 — Change-identity enrichment rules** (U11): several active change
  folders, a change archived mid-item (`changes/<slug>/` →
  `changes/archive/YYYY-MM-DD-<slug>/`), and the archive-aware resolver precedent.

## Assumptions carried forward

1. "Native type" in the `always` flow means the provider's own item type as
   configured on the recipe surface (for Trello today, its card template type);
   ledger core stays type-agnostic.
2. "Blocks missing/conflicted pre-merge" in `always` refers to the ledger verdict
   reaching the existing pre-merge host, not to a new merge path or a new
   guardian.
3. "First non-read-only write" keeps the definition used by today's path-kind
   gates (production paths; `openspec/**` exempt).
4. The synthetic provider fixture, not live Trello, is the proof surface for
   specs and tests in this slice.
5. Delivery strategy (single PR vs chained PRs), changed-line budget, and
   worktree placement are tasks-phase decisions; explore notes this already
   exceeds the 400-line review budget.
6. The proposal question round is answered and closed; no product question is
   carried into specs or design.

## Proposal question round (closed)

Asked before finalizing the bridge; answered by the user, so the round is closed
and these answers are confirmed decisions, not open framing.

| Question | Answer | Recorded as |
|---|---|---|
| Where must dormancy (declared-not-bound / ambiguous / unbound) surface? | `doctor` only; no static `AGENTS.md`/brief line; checkpoints keep following the lifecycle protocol. | D15 |
| Who adjudicates a local / remote / code / Git disagreement, and where? | A human at the terminal, shown the evidence; the decision is persisted in the ledger. | D16 |
| What happens when a branch is reused after work closed? | A new primary item for the new work / change identity; never a heuristic reopen. | D17 |
| What is the first adoption posture for a bound project? | `warn` first — observe verdicts before prompts or blocking; promotion is a human choice. | D18 |
| How far does an `ask` opt-out reach? | Checkpoint-scoped only; the next checkpoint prompts again. | D19 |

## Planning depth

**Full.** Chain: explore (done) → proposal (this file) → specs → design → tasks.
Requested depth: Full. Signal depth: Full (new capability, cross-cutting
Go/Python/recipe seam, trust-root-adjacent, ambiguous storage). Decided depth:
Full — no depth conflict, so no annotation beyond this line is required in
`tasks.md`. `Explore: written` (see `explore.md`).

## Tracker

- **card_id**: `6aa60e32aee4c220de4b1887`
- **url**: https://trello.com/c/0Tv0HZ6Q/125-epic-tracker-ledger-foundation
- **list**: Backlog (epic)

This card is **context and tracking only**. It is the epic that groups the
tracker-ledger slice; it is not this change's primary ledger binding and it is
**not a provider vocabulary contract**. Its Trello-specific shape — board, list
names, labels, card type, comments — must not be read as ledger core fields,
item-state vocabulary, or a `## Tracker` schema requirement (D11, D14). Provider
configuration remains in `recipe.toml` / `[recipes.trello-mcp-workflow.config]`.
