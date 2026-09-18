# Tracker Ledger Specification

## Purpose

Provide one durable, Go-authored record of the primary tracked item for the
`tracker` capability: its work identity, its evidence, its lifecycle state, and
its conflicts — graded once by a single authoritative predicate and consumed at
the checkpoints that already exist. The ledger replaces the current state where
"what is the tracked item for this work" is split across three independent
graders and a temp-file binding map, without inventing a generic ledger
framework, a worktree ledger, or provider vocabulary in core.

Decisions D1–D19 (proposal) and A1–A11 (design) are closed inputs; this spec
elaborates them and does not reopen them. Where names appear (flags, JSON keys,
witness `state` values, store path), the design contract is authoritative.

## Requirements

### Requirement: Durable binding witness

The sync/materialization path MUST persist the already-computed `tracker`
binding outcome as a durable witness at `<git-common-dir>/ai-specs/ledger/witness.json`,
written atomically (temp file in the destination directory + atomic replace)
and surviving sync exit. The witness MUST record exactly one state: `bound`
(the capability resolves to exactly one enabled recipe), `ambiguous` (two or
more enabled recipes declare `tracker` with no explicit binding), `unbound`
(no enabled recipe declares `tracker`), or `declared-not-bound` (a tracking
declaration exists but the capability is not bound). The witness MUST NOT be
deleted by the resolved-config exit trap. The ledger binary MUST treat the
witness as read-only input: it MUST NOT re-derive catalog or manifest bindings,
and it MUST treat a missing, unreadable, or unknown-version witness as dormant
with reason `witness-missing`.

#### Scenario: Witness survives sync

- GIVEN a sync that resolves `tracker` to one enabled recipe
- WHEN sync completes and its EXIT trap removes the resolved-config temp file
- THEN the witness file exists at `<git-common-dir>/ai-specs/ledger/witness.json`
  with state `bound` and the bound recipe id

#### Scenario: Ambiguous binding recorded as ambiguous

- GIVEN two enabled recipes both declare the `tracker` capability and no
  explicit binding resolves them
- WHEN sync writes the witness
- THEN the witness state is `ambiguous` with the candidate recipe ids recorded,
  and no provider is guessed or selected

#### Scenario: Missing witness is dormant, never bound

- GIVEN no witness file exists (fresh clone, unsynced checkout, or rollback)
- WHEN the ledger evaluates any checkpoint
- THEN the ledger reports dormant with reason `witness-missing` and never
  synthesizes or guesses a provider

### Requirement: Binding-only activation

The ledger MUST be active for a checkpoint only when the witness state is
`bound`. A declaration alone (a tracking block in project config) is supply,
not activation. Dormant and ambiguous states MUST keep every checkpoint
behaviorally inactive (skip the ledger, no prompt, no block) while remaining
visible as specified below. No state — declared, ambiguous, unbound, or
witness-missing — MAY cause the ledger to infer or synthesize a provider.

#### Scenario: Bound witness activates the ledger

- GIVEN the witness state is `bound`
- WHEN any checkpoint host invokes the ledger verdict
- THEN the ledger is active and returns a lifecycle verdict for the checkpoint

#### Scenario: Declared but not bound stays inactive

- GIVEN a project declares tracking in its config but no enabled recipe
  provides the `tracker` capability
- WHEN a checkpoint host invokes the ledger verdict
- THEN the ledger reports dormant (state `declared-not-bound`), the checkpoint
  proceeds as if the ledger were absent, and the verdict decision is `dormant`

### Requirement: Doctor-only dormancy visibility

Dormancy and ledger infrastructure failure MUST be visible through a `doctor` check
named `tracker-ledger` and through nowhere else (dormancy covers unbound, ambiguous,
declared-not-bound, and missing witness while a tracker-capable recipe is enabled).
The runtime brief and generated agent files MUST NOT gain a static
per-project dormancy line. Severity MUST be: `unbound` → INFO; `ambiguous` →
WARN; `declared-not-bound` → WARN; witness missing while a tracker-capable
recipe is enabled → WARN; infrastructure unevaluable → ERROR; a recorded
unresolved conflict → WARN. A bound, conflict-free ledger MUST render OK.

#### Scenario: Dormant project renders in doctor only

- GIVEN the witness state is `ambiguous`
- WHEN doctor runs
- THEN the `tracker-ledger` check reports WARN with guidance to add an explicit
  binding, and no runtime-brief or generated agent file contains a dormancy
  line for the project

#### Scenario: Bound and healthy renders OK

- GIVEN the witness state is `bound`, an open primary item exists for the
  current identity, and no conflict is recorded
- WHEN doctor runs
- THEN the `tracker-ledger` check reports OK

### Requirement: One primary item per work identity

The ledger MUST represent at most one primary (open) tracked item per work
identity. Work identity MUST be derived from the repository Git common dir and
the current short branch name, optionally enriched by an active change slug.
The ledger store MUST live at `<git-common-dir>/ai-specs/ledger/state.json`,
written atomically; a missing store MUST read as an empty item set and a
corrupt store MUST read as unevaluable (never as a synthesized item). When
identity cannot be derived — detached HEAD, unborn branch, or no Git common
dir — the verdict MUST be `identity_unavailable`, never a heuristic
substitution. A branch rename MUST produce a new identity. The same branch
checked out in two worktrees MUST be one identity (the common dir is shared).

Change-slug enrichment MUST add the slug only when exactly one change folder
resolves for the identity, using archive-aware resolution (active
`changes/<slug>/`, else the latest dated `archive/YYYY-MM-DD-<slug>/`, else the
legacy undated `archive/<slug>/`). When several active change folders exist,
the identity MUST omit the slug and report a change collision. A stored slug
MUST NOT change when its folder is archived mid-item.

#### Scenario: Identity derived from common dir and branch

- GIVEN a repository with a named branch and zero or one active change folder
- WHEN the ledger derives work identity
- THEN the identity is the realpath of the Git common dir plus the current
  short branch, enriched with the change slug when exactly one folder resolves

#### Scenario: Detached HEAD is unavailable, not guessed

- GIVEN the working tree is in detached HEAD state
- WHEN the ledger derives work identity at a checkpoint
- THEN the verdict is `identity_unavailable` and no item is selected, created,
  or guessed

#### Scenario: Several active changes omit the slug

- GIVEN two active change folders exist for the current work
- WHEN the ledger derives work identity
- THEN the identity omits the change slug and the verdict reports a
  `change-ambiguous` collision

#### Scenario: Archived change keeps its slug

- GIVEN an open ledger item enriched with a change slug whose folder was
  archived mid-item
- WHEN the identity is re-derived
- THEN the stored slug is unchanged and the folder resolves through the
  archive-aware lookup

### Requirement: Human adjudication of conflicts and new primary after branch reuse

The ledger MUST reconcile four evidence sides — local ledger state, remote provider evidence,
code/change artifact state (the `## Tracker` section or `tracker.none` exemption, read as presentation,
not graded), and Git/PR evidence — while this slice produces only the local, code, and Git sides (L3):
the `remote` side remains accepted by the model but has no producer yet. Any disagreement between
produced sides MUST produce a recorded conflict with no default winner: the host MUST present the
conflicting evidence to a human at the terminal, and the human decision MUST be persisted in the ledger
as an append-only decision (kind `adjudicate`, with a choice among the sides or exemption) alongside a
conflict snapshot on the item. The ledger MUST never pick a winning side on its own.

When a branch is reused after its item was closed, the ledger MUST NOT reopen or re-create the item by
heuristic at any grade; a new primary item MUST be opened only through an explicit `open` write and MUST
remain distinct from the closed item. Two open items for one identity MUST be treated as a conflict
requiring human adjudication, never as a silent pick.

#### Scenario: Disagreement presents evidence and persists the human choice

- GIVEN a bound ledger whose local item state disagrees with the code or Git evidence for the identity
- WHEN a checkpoint grades the work
- THEN the verdict records the conflict, the host presents the produced evidence sides (with `remote`
  absent), and only after the human answers is the decision appended to the ledger; the next grade for
  the same input reflects the persisted decision

#### Scenario: Branch reuse after closed work opens a new item only by explicit write

- GIVEN an identity whose primary item is closed and new work begins on the same branch under a new
  change identity
- WHEN an explicit `open` write is issued for the new identity
- THEN a new open primary item is opened for the new identity and the closed item remains closed in the
  store

#### Scenario: Two open items are a conflict, not a pick

- GIVEN the store contains two open items for the same identity
- WHEN the ledger grades a checkpoint
- THEN the verdict is a conflict requiring human adjudication and no item is selected as primary without
  a persisted human decision

#### Scenario: Closed work is not reopened by grade

- GIVEN an identity whose primary item is closed and new work beginning on the same branch
- WHEN checkpoints grade without any write
- THEN no grade reopens or creates an item; a new primary appears only after an explicit `open` write

### Requirement: Executable five-checkpoint lifecycle verdicts

The ledger MUST produce executable verdicts — computed by the same Go
predicate for every host — at exactly five checkpoints: `work-start`,
`apply-start`, `pr-review`, `pre-merge`, and `archive-close`. Work-start MUST
fire before the SDD/OpenSpec proposal phase when that is in play, and before
the first non-read-only production write otherwise; no checkpoint MAY require
a change folder to exist. Path-based hosts MUST never block `openspec/**`
writes. The verdict command MUST exit `0` for `allow`, `ask`, `dormant`, and
reported `unevaluable` outcomes, and exit `2` only when the host must stop (a
`block` decision or a failed decision persist). The verdict output MUST be
machine-readable JSON carrying at minimum the capability, activation state,
checkpoint, mode, decision (`allow` | `block` | `ask` | `dormant` |
`unevaluable`), reason, identity, primary item, conflict, and doctor finding.
The same fixture input MUST yield the same verdict at every host that reaches
the predicate.

#### Scenario: All five checkpoints reach one predicate

- GIVEN a pinned fixture input (identity, evidence, mode, checkpoint)
- WHEN each checkpoint host invokes the ledger (`work-start` via the
  plan-build gate, `apply-start`/`pr-review` via the tracker gate in hook mode,
  `pre-merge` and `archive-close` via that same tracker gate in direct host mode
  `--root <root> --checkpoint pre-merge|archive-close`)
- THEN every host observes the same decision, conflict, and exit code for that
  input

#### Scenario: Work-start without a change folder

- GIVEN no change folder exists and the first non-read-only production write
  is attempted
- WHEN the work-start checkpoint grades the write
- THEN the ledger returns a verdict and the host behavior follows the mode;
  the absence of a change folder alone does not skip or fail the checkpoint

#### Scenario: openspec writes are never blocked

- GIVEN the ledger is active in `always` mode with no primary item
- WHEN a host observes a write under `openspec/**`
- THEN the write is not blocked by the ledger

### Requirement: Modes always, ask, and warn with checkpoint-scoped opt-outs and warn-first adoption

The ledger MUST support exactly three project modes: `always`, `ask`, `warn`.
In `always`: the ledger requires an item before work (a missing or conflicted
item blocks production writes at work-start/apply-start, blocks PR creation at
pr-review, and blocks missing, conflicted, or `identity_unavailable` state at
pre-merge; archive-close blocks a missing or conflicted close decision). In
this slice `always` MUST NOT perform provider create/update calls; it blocks
until a human (or a later slice) supplies the item. In `ask`: the host prompts
at each checkpoint and an explicit opt-out allows that checkpoint to proceed —
including allowing the merge — while the opt-out applies ONLY to the checkpoint
that was answered; the next checkpoint prompts again. In `warn`: the ledger
reports the verdict on stderr and never blocks. In no mode MAY a human decision
be inferred. When no terminal is available to answer an `ask` prompt, `ask`
MUST remain a hard block (L4): the checkpoint MUST NOT proceed and `ask` MUST
NOT be downgraded to report-and-proceed.

A newly bound project MUST adopt `warn` as its first posture; promotion to
`ask` or `always` MUST require an explicit human configuration change. When no
ledger mode is configured, the existing tracker gate vocabulary MUST map
forward: gate `off` → checkpoints skip (doctor still reports the witness), gate
`warn` → `warn`, gate `always` → `always`. The worktree gate's mode MUST NOT be
read as the ledger mode.

#### Scenario: Always blocks missing state at pre-merge

- GIVEN mode `always`, a bound ledger, and no open primary item at pre-merge
- WHEN the pre-merge host grades the work
- THEN the verdict is `block` (exit `2`) with reason `needs-item` and no
  provider write is performed

#### Scenario: Ask opt-out is checkpoint-scoped

- GIVEN mode `ask` and a prompt answered with opt-out at `apply-start`
- WHEN `apply-start` is re-graded, then `pre-merge` is graded
- THEN `apply-start` proceeds under the recorded opt-out and `pre-merge`
  prompts again, because the opt-out covers only the checkpoint that was
  answered

#### Scenario: Ask without a terminal stays a hard block

- GIVEN mode `ask` and a checkpoint host running without a terminal to prompt on
- WHEN the host grades the checkpoint
- THEN the verdict is `block` (exit `2`), the action does not proceed, and no
  report-and-proceed fallback is applied

#### Scenario: Warn never blocks

- GIVEN mode `warn` and a missing or conflicted item at any checkpoint
- WHEN the host grades the checkpoint
- THEN the verdict and evidence are reported on stderr and the action proceeds
  (exit `0`)

#### Scenario: Warn-first adoption

- GIVEN a project whose tracker capability becomes bound with no ledger mode
  configured and tracker gate mode `warn`
- WHEN checkpoints are graded
- THEN the effective mode is `warn`, and switching to `ask` or `always`
  requires an explicit human edit of the ledger mode configuration

### Requirement: Go-authoritative grader with Python bridge only

All ledger decision logic MUST live in the single existing Go product as a `--ledger`
mode, reusing its existing acquisition, digest-verification, cache, release, and
selftest machinery. This covers work-identity derivation, the evidence model, the
conflict predicate, the lifecycle state machine, the write surface and its
invariants, and checkpoint verdicts. No second Go module, second binary, second
release asset, or second trust root MAY be created, and tracker semantics MUST
NOT couple to worktree gate semantics beyond sharing the binary and Git-facts
layer. Python call sites (sync/materialization, the checkpoint hosts, doctor) MUST
remain thin acquisition, witness-writing, and JSON-consumption bridges: no new
Python grader and no duplicated predicate MAY be introduced. The `## Tracker`
validity rule MUST have exactly one authoritative grader — the Go predicate;
existing Python copies MUST delegate to it or be covered by parity tests that
fail on divergence. The `## Tracker` section and `tracker.none` exemption remain
the human-facing authoring surfaces; the ledger is the grading authority. The
binary `--selftest` MUST continue to pass while also exercising the ledger
identity, verdict, and new write-surface invariants without network access. The
write surface MUST live in this same Go binary and MUST be verified by the same
selftest and the same digest-verified release path: no second binary, second
selftest entry point, or second trust root MAY be introduced to carry writes.

#### Scenario: One grader, parity at legacy seams

- GIVEN the `## Tracker` validity predicate graded by the Go ledger
- WHEN each legacy copy (link parser consumers, the tracker hook, doctor) is
  exercised against the same fixture inputs
- THEN each copy either delegates to the Go verdict or matches it exactly, and
  a parity test fails on any divergence

#### Scenario: Single verified binary serves both modes

- GIVEN the released Go product with its existing digest manifest
- WHEN the ledger mode is added and shipped
- THEN the same four-target digest set, cache, and verification path are used —
  no second binary, release asset, or trust root appears

#### Scenario: Selftest covers ledger invariants

- GIVEN the built binary
- WHEN `--selftest` runs offline
- THEN it prints its existing success marker and additionally exercises ledger
  identity, verdict, and write-surface invariants in-process

### Requirement: Unevaluable, outage, and unsupported behavior

The ledger MUST degrade along the existing fail-open blast radius and never
become a new way to break a worktree. A missing or unverified binary, an
acquisition failure, or a failed selftest MUST fail open at every checkpoint
in every mode, with a `doctor` ERROR (an infrastructure exemption — this MUST
NOT turn a cold cache into a merge-eligibility change, including in `always`
mode at pre-merge). A dormant witness MUST skip the ledger at all checkpoints.
`identity_unavailable` MUST block only in `always` mode where the mode table
says so (production writes; pre-merge), and MUST be reported without blocking
in `warn` and `ask`. A corrupt store or state IO error MUST fail open with a
`doctor` ERROR and MUST NOT synthesize items. A host reaching an older binary
without ledger support MUST degrade to the existing one-line offline stderr
behavior and exit `0`. Flag-parse errors on verdict calls MUST fail open; a
failed decision persist MUST fail closed (exit `2`). The same fail-closed
posture MUST cover the write surface (L6): a write rejected by payload
validation, a write that cannot acquire the store lock before the bounded
timeout, an ambiguous-identity refusal, and a store IO error during a write
MUST each persist nothing, leave the store byte-identical, and exit `2`. Grade
paths MUST keep failing open — a grade-path lock timeout, unreadable evidence,
or a malformed verdict call never blocks.

#### Scenario: Cold cache at pre-merge in always mode

- GIVEN mode `always` at pre-merge and the verified binary cannot be acquired
  (cold cache, no network)
- WHEN a checkpoint host invokes the ledger
- THEN the checkpoint fails open, the merge is not blocked by the ledger, and
  doctor records an ERROR for the infrastructure failure

#### Scenario: Corrupt store never synthesizes

- GIVEN the ledger store file exists but contains invalid JSON
- WHEN a checkpoint grades the work
- THEN the verdict is `unevaluable` (reason `store-corrupt`), behavior follows
  the fail-open rule for the mode, and no item is invented or repaired silently

#### Scenario: Older binary without ledger support

- GIVEN a host resolves a binary that predates the `--ledger` mode
- WHEN the host invokes the ledger verdict
- THEN the host prints the one-line offline degradation to stderr, proceeds
  (exit `0`), and doctor can surface the unsupported state

#### Scenario: Write-path failures persist nothing and exit 2

- GIVEN an invalid write payload, or a held store lock, or an ambiguous
  identity, or a store IO error on the write path
- WHEN the write executes
- THEN each case persists nothing, leaves the store byte-identical, and exits
  `2`, while the corresponding grade path still fails open

### Requirement: Scope boundaries

The ledger MUST remain tracker-only. It MUST NOT introduce a multi-capability
ledger framework, a second capability ledger, a per-provider grader or any
provider code in the core, a ledger plugin API beyond the single Tracker-domain
port specified below, or a worktree-specific ledger or worktree-identity model.
It MUST NOT
migrate, rewrite, or blanket-revalidate historical archives or in-flight
changes. Provider item vocabulary (provider ids beyond the bound recipe id,
board/list/label/issue-type shapes, provider config fields) MUST stay
provider-private in recipe configuration and in an opaque provider object on
the item that the predicate does not read; core item fields MUST remain
provider-neutral (item id, provider id, native type, URL, state, exemption,
evidence references). The ledger MUST NOT perform any provider create, update,
move, comment, or label call in this slice; the generic `bind` writer and the
VCS-boundary archive-close writer are local lifecycle writers and perform no such
call and no network access. Provider configuration MUST live
in recipe configuration, never in ledger core.

#### Scenario: No generic capability ledger

- GIVEN the ledger implementation
- WHEN a second, non-tracker capability is considered
- THEN no generic ledger framework, capability parameterization, or second
  ledger exists; a future capability would require its own explicit change

#### Scenario: Provider vocabulary stays out of core

- GIVEN a tracker recipe with provider-specific configuration (board, list, or
  issue-type shapes)
- WHEN the ledger stores and grades items
- THEN the predicate reads only core neutral fields and the opaque provider
  object, and no provider-specific field appears in ledger core, the artifact
  section contract, or the exemption file contract

#### Scenario: History is untouched

- GIVEN archived changes and in-flight changes recorded before the ledger
- WHEN the ledger ships
- THEN no archive is migrated, rewritten, or revalidated; forward work only

### Requirement: Tracker domain port with declarative provider adapters

The ledger core MUST be an autonomous grader with no provider vocabulary: its item fields, lifecycle state
machine, conflict predicate, and checkpoint verdicts MUST be provider-neutral. Tracker is a **domain port**
of that core, not a provider. A provider recipe (Trello today; Jira/Linear later) extends the port by
declaring a **declarative adapter mapping** — native state values bound to neutral properties, e.g.
`[config.reconcile]` with `scope_field`, `max_age_seconds`, and per-event `expectations` — that the Go
core reads from the project manifest. Recipe configuration is an adapter input only: it MUST NOT enable,
disable, or change ledger behavior or grading, and `ledger_mode` / `gate_mode` stay Tracker-domain policy
separate from the mapping surface. An absent or unbound adapter MUST stay dormant or report an explicit
`unconfigured` comparison, never a default, and no provider may be guessed. Adding a provider MUST add
only mapping data: it MUST NOT add provider-specific Go, a second grader, or a core change.

#### Scenario: One core comparator, declarative adapters

- GIVEN the Tracker-domain core and any bound provider recipe
- WHEN a checkpoint grades or reconciles an item
- THEN the comparison runs in the one Go predicate against neutral expectations
- AND the provider contributes only its declarative mapping

#### Scenario: Mapping never conditions ledger behavior

- GIVEN a provider recipe that declares an adapter mapping
- WHEN its `[config.reconcile]` table is inspected
- THEN it declares only native-value-to-neutral-property mapping keys
- AND `ledger_mode` / `gate_mode` are absent from it

#### Scenario: Absent adapter is dormant, never guessed

- GIVEN no bound adapter, or an item whose native state is outside the declared mapping
- WHEN a comparison is requested
- THEN the outcome is dormant or an explicit `unconfigured`
- AND no provider or default state is synthesized

### Requirement: Synthetic provider fixture and parity corpus

Ledger behavior MUST be proven by a synthetic tracker provider fixture recipe
under the internal test-recipe fixtures area (gated by the internal-test-recipe
environment flag) that declares the `tracker` capability — with no MCP, no
network, and no live provider — plus a pinned verdict corpus of
`(identity + evidence + mode + checkpoint) → (decision, conflict, exit)` cases
covering at minimum: no witness (dormant), bound with empty store per mode,
consistent evidence (allow), four-side disagreement without a decision (ask or
block by mode), a persisted decision, a checkpoint-scoped opt-out, branch reuse
after close (new item), two open items (conflict), detached HEAD per mode,
missing-binary host behavior, a code-vs-ledger conflict, a `tracker.none`
exemption, an idempotent reopen, a link mismatch, a same-second item-id
collision, a `change-ambiguous` refusal, and a write-failure case. Parity tests
MUST drive the built binary through the corpus, and the migrated host tests
MUST exercise the bridge rather than a second grader.

#### Scenario: Corpus pins verdict behavior

- GIVEN the pinned corpus fixtures
- WHEN the parity test drives the built binary with each fixture
- THEN every fixture produces exactly its pinned decision, conflict, and exit
  code

#### Scenario: Hermetic fixture provider

- GIVEN the synthetic tracker provider fixture recipe
- WHEN the test suite runs in a clean environment without MCP or network access
- THEN the fixture activates the ledger through the witness path and all
  ledger tests pass with no external provider contacted

---

### Requirement: Machine write surface for open, bind, link, close, and exempt

The ledger CLI MUST expose a machine write surface on the existing verdict flow — one JSON-payload write
verb plus the existing `--checkpoint`, following the proven `--decide` pattern: parse → validate → locked
read-modify-write → re-grade → print verdict. It MUST support exactly the kinds `open`, `bind`, `link`,
`close`, and `exempt`, and MUST NOT add a new subcommand tree, second binary, second Go module, or new
release asset.
After this change every declared decision kind and core item field — `DecisionOpen`, `DecisionClose`,
`DecisionLink`, and `Item.{ID, URL, State, NativeType, Provider, Exemption}` — MUST have at least one
production writer; any declared kind still unreachable from production MUST be named as deliberately
deferred or deleted, not left silent. The `open`, `link`, and `close` verbs MUST append a decision of the
corresponding machine-written kind to the item's append-only decision log. The `link` verb MUST record the
provider's native id, URL, native type, and state on the provider-neutral core fields plus an opaque
provider object the predicate does not read. The `exempt` verb MUST set the item's exemption with a
persisted reason.

The `bind` verb is the **generic, workflow-agnostic lifecycle write**: in one locked transaction it opens
the primary item if the identity has none and records the supplied native fields, appending the existing
`open` and `link` decisions (never a new decision kind). It MUST be usable by SDD, ODD, and no-flow callers
alike, MUST require **no** repository tracker artifact (`## Tracker` section, `tracker.none`, or `openspec/`
tree), and MUST perform no provider or network call. A `bind` that omits `change` MUST be a deliberate
**branch-level** binding: it matches the single open row for the same common dir and branch regardless of
the stored change slug, MUST refuse (fail closed, nothing persisted) when several open rows exist, and
otherwise MUST open a new branch-only item. A `bind` whose payload names the change MUST keep the
slug-keyed identity. A retried `bind` for an already-linked item MUST report the idempotent `unchanged`
outcome; and a closed row MUST NOT be reopened — a later explicit `bind` opens a distinct new primary (D17).

#### Scenario: Bind seeds open and link without a repository artifact

- GIVEN a bound identity, an empty store, and no `openspec/` tree or `## Tracker` section
- WHEN a single `bind` write supplies the native id, URL, native type, state, and provider payload
- THEN exactly one open primary item exists with those core fields populated, the open and link decisions
  are appended, the `write` sidecar reports `applied: true`, and no provider or network call was made

#### Scenario: Bind is branch-level without an artifact

- GIVEN several active change folders (a `change-ambiguous` identity) and either no open row or exactly one
  existing open row for the common dir and branch
- WHEN a `bind` write omits `change`
- THEN the single existing open row is linked regardless of its stored slug, or a new branch-only item is
  opened when none exists, and no `openspec/` artifact or provider call is required

#### Scenario: Branch-level bind refuses a collision

- GIVEN two open rows for one common dir and branch
- WHEN a `bind` write omits `change`
- THEN the bind is refused, nothing is persisted, and the command exits `2`

#### Scenario: Repeated bind is idempotent

- GIVEN an item already linked through `bind`
- WHEN the identical `bind` write is issued again
- THEN no second item is created, no duplicate link decision is appended, and the outcome is `unchanged`

#### Scenario: Open creates exactly one primary item

- GIVEN a bound identity and an empty store
- WHEN an `open` write is issued
- THEN exactly one open primary item exists for the identity, the `open` decision is appended, and the
  post-write verdict is re-graded and printed

#### Scenario: Link records native fields

- GIVEN an open primary item
- WHEN a `link` write supplies native id, URL, native type, state, and provider payload
- THEN the core fields are populated, the provider payload stays opaque to the predicate, and a `link`
  decision is appended

#### Scenario: Close closes without reopening

- GIVEN an open primary item
- WHEN a `close` write is issued
- THEN the item is closed, the `close` decision is appended, and the item remains in the store

#### Scenario: Exempt persists a reason

- GIVEN a human-authored `tracker.none` for the change
- WHEN an `exempt` write is issued
- THEN `Item.Exemption` is set with the persisted reason and is visible in `state.json`

---

### Requirement: Explicit item opening by authorized local lifecycle writers

An item MUST be opened only by a deliberate write. Grading MUST be pure: no grade of any checkpoint, in
any mode, MAY create, mutate, or delete store state. Hosts MUST NOT auto-open an item because a
`## Tracker` section parses or because a checkpoint grades `needs-item`. The archived scenario in which
the grade opens the item is superseded: the opening verb lives on the write surface.

The archived **agent-only** L2/DW1 wording is superseded. Opening is no longer agent-only: exactly two
writers are authorized, and both are **local lifecycle writers** that perform no provider or network call.

1. The generic machine write surface (the `open` and `bind` verbs, plus `link`, `close`, and `exempt`),
   invoked deliberately by a host, a human, or a workflow-agnostic caller. It is usable before any apply
   boundary and requires no SDD/ODD/OpenSpec artifact.
2. The VCS-boundary archive-close writer, invoked by the verified worktree cleanup at a merge boundary
   (the `post-merge` hook and its direct cleanup fallback). It closes the matching open primary item for
   the repository common dir and branch at `archive-close`, ignoring any stored change slug.

Neither writer creates, mutates, moves, comments on, or labels a provider item, and neither may be reached
by a grade path. A closed row MUST NOT be reopened by either writer, and a reused branch opens a new
primary only through an explicit write (D17).

#### Scenario: Grading never writes

- GIVEN mode `always` and no open item
- WHEN `work-start` grades the checkpoint
- THEN the verdict is `block` with reason `needs-item` and the store is byte-identical afterward

#### Scenario: Parsing never opens

- GIVEN a `## Tracker` section with a valid `card_id` and no write issued
- WHEN any checkpoint grades
- THEN no item is created and no write surface call is made by the parse alone

#### Scenario: Authorized writers are local and provider-free

- GIVEN a bound ledger and a workflow-agnostic caller (no SDD, ODD, or OpenSpec in play)
- WHEN the generic `bind` writer or the VCS archive-close writer records the lifecycle
- THEN the only durable change is the local store under `<git-common-dir>/ai-specs/ledger/`, no provider
  or network call is performed, and no grade path mutated state

---

### Requirement: VCS-boundary archive-close writer

The lifecycle MUST close automatically at the VCS merge boundary without coupling to SDD, ODD, OpenSpec, or
any agent. A managed `post-merge` trigger MUST invoke the verified worktree cleanup, which closes the
matching item **before** any destructive removal. Cleanup matches the open primary by the repository's Git
common dir and the candidate branch, ignoring any stored change slug, and records the `archive-close`
checkpoint. A missing or already-closed matching item is an idempotent no-op, and a collision of two open
rows for the pair fails closed rather than guessing (D17). The direct tracker host
(`tracker-card-gate.sh --root <root> --checkpoint archive-close`) remains the fallback when the Git hook is
absent.

The close MUST be reached only from the merge boundary, and it MUST NOT invent or reopen a row: a branch
whose merge is not proven is preserved. The `post-merge` hook MUST be fail-open for the sealed merge
outcome (it reports failures to stderr and exits `0`), while the destructive cleanup it triggers MUST fail
closed: when the ledger close cannot be persisted, the candidate worktree/branch is preserved rather than
removed, so no work is destroyed without its lifecycle record.

#### Scenario: Post-merge closes the matching item before removal

- GIVEN a merged, clean worktree whose branch has one open ledger item carrying a change slug
- WHEN the managed `post-merge` trigger runs the verified cleanup
- THEN the matching item is closed with an `archive-close` decision before the worktree is removed, and the
  close matched on the branch despite the stored slug

#### Scenario: Post-merge close is idempotent and never reopens

- GIVEN the matching item for a merged branch is already closed
- WHEN the `post-merge` trigger runs cleanup again
- THEN no row is created, rewritten, or reopened, and cleanup reports a successful no-op

#### Scenario: Failed close preserves the candidate (fail closed)

- GIVEN cleanup has proven a worktree merged but the ledger store write fails
- WHEN cleanup runs
- THEN the candidate worktree and branch are preserved and reported as failed, no destructive action is
  taken, and the merge outcome observed by Git is unchanged

#### Scenario: Hook absence falls back to the direct host

- GIVEN a project with no managed `post-merge` hook installed
- WHEN the merge boundary passes
- THEN closing the item remains available through the direct tracker host at `archive-close`, and nothing
  infers a close from an OpenSpec archive

---

### Requirement: Idempotent, collision-safe writes under a bounded lock

Every write MUST mutate the store under the same exclusive store lock the existing append primitive
uses, with a bounded non-blocking acquisition attempt plus short retry (exact parameters are a design
decision). An `open` write MUST be open-if-absent: a repeated or retried open for an identity that
already has an open primary item MUST NOT create a second item and MUST NOT manufacture a
multiple-open conflict. Item ids MUST be unique: the writer MUST guard the second-precision id
derivation so two opens for one identity in the same second cannot yield two items sharing one id. A
`link` write identical to the item's current link state MUST NOT append a duplicate decision. A `close`
of an already-closed item MUST NOT create anything and MUST be explicitly signaled rather than silently
duplicated. A write for a `change-ambiguous` identity MUST be refused (fail closed) unless the caller
supplies an explicit change slug. The one exception is a branch-level `bind` that omits `change`: it is
matched by common dir and branch, as defined by the machine write surface, instead of being refused. On
lock timeout, grade paths MUST fail open and write paths MUST fail
closed with exit `2` and no store change.

#### Scenario: Retried open stays idempotent

- GIVEN an open primary item for the identity
- WHEN the same `open` write is issued again
- THEN the store still holds exactly one open item for the identity and no multiple-open conflict exists

#### Scenario: Same-second opens do not collide

- GIVEN two `open` writes for the same identity within the same second
- WHEN both are processed under the lock
- THEN the store ends with exactly one open item and all stored item ids are unique

#### Scenario: Repeated link does not duplicate

- GIVEN a `link` write identical to the item's current link state
- WHEN it is issued a second time
- THEN no duplicate `link` decision is appended

#### Scenario: Close after close is explicit

- GIVEN a closed primary item
- WHEN a `close` write is issued again
- THEN the outcome is explicitly signaled (per the design representation) and no new item or duplicate
  `close` decision is silently created

#### Scenario: Change-ambiguous write is refused

- GIVEN an identity whose change collision is `change-ambiguous`
- WHEN a write other than a branch-level `bind` is attempted without an explicit slug
- THEN the write is refused, nothing is persisted, and the command exits `2`

#### Scenario: Lock contention has bounded postures

- GIVEN the store lock is held by another writer
- WHEN a grade path acquires the lock, then a write path attempts it until timeout
- THEN the grade fails open (exit `0`, checkpoint proceeds) and the write fails closed (exit `2`, store
  byte-identical)

---

### Requirement: Local, code, and Git evidence with remote deferred

The ledger MUST retain the four-side evidence model while this slice produces only three sides (L3). A
thin Python bridge — acquisition only — MUST build the evidence file from local facts: `local` = the
ledger store snapshot; `code` = the change's `## Tracker` `card_id` and recorded `pr:` parsed by the
existing pure parser, or the presence of `tracker.none`; `git` = locally derivable branch/HEAD/commit
plus the recorded PR URL. The bridge MUST NOT grade, MUST NOT introduce a new Python predicate, and MUST
NOT call `gh`, MCP, or the network. The tracker gate host — in hook mode and in its direct
`--root <root> --checkpoint pre-merge|archive-close` mode — MUST pass the
bridge-built evidence at its checkpoints (`apply-start`, `pr-review`, `pre-merge`, `archive-close`);
`work-start` keeps its existing host behavior. Unreadable or malformed evidence MUST fail open (empty
evidence side), as today. The `remote` side MUST have no producer in this slice — no tracker MCP read —
and the deliberate gap MUST be stated in the docs. The bridge and evidence passing MUST keep working from
a cold CLI install with no project cache.

#### Scenario: Code-vs-ledger mismatch conflicts in production

- GIVEN an open item whose id differs from the `## Tracker` `card_id`
- WHEN the apply-start and pre-merge hosts grade with bridge-built evidence
- THEN the verdict is a conflict requiring adjudication per mode, with `remote` absent

#### Scenario: Malformed artifact fails open

- GIVEN a missing or malformed `## Tracker` section
- WHEN the bridge builds evidence and the checkpoint grades
- THEN the code side is empty, no conflict is invented, and the checkpoint proceeds (fail open)

#### Scenario: Evidence acquisition is offline

- GIVEN a hermetic environment with no network, MCP, or `gh`
- WHEN hosts grade with bridge-built evidence
- THEN the evidence path completes without any external call

---

### Requirement: Persistent tracker.none exemption

The ledger MUST record a `tracker.none` file in a change folder — the human-authored act of exempting
that change — as a persistent, auditable, change-scoped exemption stored as `Item.Exemption` with a
persisted reason, via the explicit `exempt` write. The exemption MUST be honored by the existing
allow/exempt branch at EVERY checkpoint while it stands; it MUST NOT be mapped onto the
checkpoint-scoped opt-out path and MUST NOT re-prompt per checkpoint. The exemption MUST NOT register as
an evidence conflict. Removing the file MUST NOT auto-revoke the store record — revoking an exemption is
a human act (remove the file and adjudicate in the ledger); no grade MAY auto-revoke it.

#### Scenario: Exemption honored at every checkpoint

- GIVEN `Item.Exemption` recorded for the change's item
- WHEN each of the five checkpoints grades
- THEN every checkpoint resolves allow/exempt without prompting and without blocking

#### Scenario: Exemption does not conflict

- GIVEN `tracker.none` present and the code evidence side carrying no card id
- WHEN a checkpoint grades
- THEN the verdict is exempt/allow and no evidence conflict is raised

---

### Requirement: Witness-derived provider configuration lookup

The ledger mode / gate mode configuration lookup MUST resolve the bound recipe id from the durable
binding witness (`Binding.RecipeID`) at every host layer — the plan-build work-start gate, the tracker gate (including its direct `--root <root> --checkpoint pre-merge|archive-close` host mode), and doctor. Reading the witness is acquisition, not grading. When the
witness is missing or unreadable, the lookup MUST fall back to the legacy literal so behavior is
identical to today. No host MAY resolve its config section from a hardcoded provider or recipe literal:
the one literal MUST live in the bridge's fallback, and each host MUST read
`recipes.<witness recipe id>.config`. The witness → recipe id → provider-config seam is the only
provider extension point; no provider-specific behavior enters ledger core.

#### Scenario: Witness recipe id drives mode lookup

- GIVEN a witness bound to a non-legacy fixture recipe id with its own ledger mode config
- WHEN a host resolves the effective ledger mode
- THEN the mode is read from that recipe's config, not from the legacy literal

#### Scenario: Missing witness falls back identically

- GIVEN the witness is missing or unreadable
- WHEN a host resolves the effective ledger mode
- THEN the legacy fallback is used and the effective behavior is unchanged from today

---

### Requirement: Plan Build-independent tracker checkpoint hosts

The five checkpoints MUST keep their tracker-domain hosts: `work-start` → plan-build-flow gate;
`apply-start` and `pr-review` → tracker card gate; `pre-merge` and `archive-close` →
the `tracker-card-gate.sh` shell bridge in direct host mode
(`--root <root> --checkpoint pre-merge|archive-close`). The `pre-merge` and
`archive-close` checkpoints MUST be executable with no `openspec/` tree, and tracker item closure MUST
stay independent of OpenSpec archive: archive state MUST NOT select, infer, or substitute for a tracker
checkpoint, and the Plan Build artifact guardian MUST NOT host, grade, or write tracker state. Plan Build
owns its own verify → promotion → read-only guardian → archive tail; the `tracker-card-gate.sh` direct
checkpoint host owns only tracker lifecycle grading. Where `work-start`'s host is not enabled (a tracker-bound project without
plan-build-flow), that limitation MUST be visible through the doctor check or documentation while the
other four checkpoints keep grading.

#### Scenario: Unhosted work-start is visible

- GIVEN a tracker-bound project without plan-build-flow enabled
- WHEN doctor runs
- THEN the unhosted `work-start` checkpoint is reported or documented as a limitation, and the other
  four checkpoints still grade

#### Scenario: Tracker closure without an OpenSpec archive

- GIVEN a tracker-bound change with no `openspec/` tree
- WHEN `archive-close` is graded through the `tracker-card-gate.sh` direct checkpoint host
- THEN the checkpoint reaches the same Go predicate and returns a verdict
- AND no OpenSpec archive is required, read, or inferred

#### Scenario: The artifact guardian never hosts a tracker checkpoint

- GIVEN the Plan Build artifact guardian evaluates a change
- WHEN it reports its verdict
- THEN no tracker checkpoint is graded by it
- AND tracker state is never written by it

---

### Requirement: Write-path failure postures fail closed

A failed write MUST persist nothing and exit `2`, like a failed `--decide` persist (L6): validation
failure, lock timeout, ambiguous-identity refusal, or store IO error MUST leave the store byte-identical
and the output honest about the failure. Grade paths keep the existing fail-open blast radius unchanged
(missing/unverified binary, acquisition failure, flag-parse error on verdict calls, cold cache).

#### Scenario: Failed write exits 2 with no store change

- GIVEN a write with an invalid payload (or a lock timeout, or an ambiguous identity)
- WHEN the write executes
- THEN the command exits `2`, the store is byte-identical, and no item or decision was created
