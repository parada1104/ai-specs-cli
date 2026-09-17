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

Dormancy (unbound, ambiguous, declared-not-bound, missing witness while a
tracker-capable recipe is enabled) and ledger infrastructure failure MUST be
visible through a `doctor` check named `tracker-ledger` and through nowhere
else. The runtime brief and generated agent files MUST NOT gain a static
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

The ledger MUST reconcile four evidence sides — local ledger state, remote
provider evidence, code/change artifact state (the `## Tracker` section or
`tracker.none` exemption, read as presentation, not graded), and Git/PR
evidence. Any disagreement between sides MUST produce a recorded conflict with
no default winner: the host MUST present the conflicting evidence to a human at
the terminal, and the human decision MUST be persisted in the ledger as an
append-only decision (kind `adjudicate`, with a choice among the sides or
exemption) alongside a conflict snapshot on the item. The ledger MUST never
pick a winning side on its own.

When a branch is reused after its item was closed, the ledger MUST open a NEW
primary item for the new work/change identity; a closed item MUST never be
reopened by heuristic. Two open items for one identity MUST be treated as a
conflict requiring human adjudication, never as a silent pick.

#### Scenario: Disagreement presents evidence and persists the human choice

- GIVEN a bound ledger whose local item state disagrees with the remote or Git
  evidence for the identity
- WHEN a checkpoint grades the work
- THEN the verdict records the conflict, the host prints the four evidence
  sides, and only after the human answers is the decision appended to the
  ledger; the next grade for the same input reflects the persisted decision

#### Scenario: Branch reuse after closed work opens a new item

- GIVEN an identity whose primary item is closed and new work begins on the
  same branch under a new change identity
- WHEN the ledger grades the checkpoint
- THEN a new open primary item is opened for the new identity and the closed
  item remains closed in the store

#### Scenario: Two open items are a conflict, not a pick

- GIVEN the store contains two open items for the same identity
- WHEN the ledger grades a checkpoint
- THEN the verdict is a conflict requiring human adjudication and no item is
  selected as primary without a persisted human decision

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
  plan-build gate, `apply-start`/`pr-review` via the tracker gate, `pre-merge`
  and `archive-close` via the pre-merge guardian stages)
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
be inferred.

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

All ledger decision logic — work-identity derivation, the evidence model, the
conflict predicate, the lifecycle state machine, and checkpoint verdicts —
MUST live in the single existing Go product as a `--ledger` mode, reusing its
existing acquisition, digest-verification, cache, release, and selftest
machinery. No second Go module, second binary, second release asset, or second
trust root MAY be created, and tracker semantics MUST NOT couple to worktree
gate semantics beyond sharing the binary and Git-facts layer. Python call
sites (sync/materialization, hooks, guardian, doctor) MUST remain thin
acquisition, witness-writing, and JSON-consumption bridges: no new Python
grader and no duplicated predicate MAY be introduced. The `## Tracker` validity
rule MUST have exactly one authoritative grader — the Go predicate; existing
Python copies MUST delegate to it or be covered by parity tests that fail on
divergence. The `## Tracker` section and `tracker.none` exemption remain the
human-facing authoring surfaces; the ledger is the grading authority. The
binary `--selftest` MUST continue to pass while also exercising the ledger
identity and verdict invariants without network access.

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
- THEN it prints its existing success marker and additionally exercises
  ledger identity and verdict invariants in-process

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
failed decision persist MUST fail closed (exit `2`).

#### Scenario: Cold cache at pre-merge in always mode

- GIVEN mode `always` at pre-merge and the verified binary cannot be acquired
  (cold cache, no network)
- WHEN the guardian invokes the ledger
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

### Requirement: Scope boundaries

The ledger MUST remain tracker-only. It MUST NOT introduce a generic or
multi-capability ledger framework, a ledger plugin API, a second capability
ledger, or a worktree-specific ledger or worktree-identity model. It MUST NOT
migrate, rewrite, or blanket-revalidate historical archives or in-flight
changes. Provider item vocabulary (provider ids beyond the bound recipe id,
board/list/label/issue-type shapes, provider config fields) MUST stay
provider-private in recipe configuration and in an opaque provider object on
the item that the predicate does not read; core item fields MUST remain
provider-neutral (item id, provider id, native type, URL, state, exemption,
evidence references). The ledger MUST NOT perform any provider create, update,
move, comment, or label call in this slice. Provider configuration MUST live
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

### Requirement: Synthetic provider fixture and parity corpus

Ledger behavior MUST be proven by a synthetic tracker provider fixture recipe
under the internal test-recipe fixtures area (gated by the internal-test-recipe
environment flag) that declares the `tracker` capability — with no MCP, no
network, and no live provider — plus a pinned verdict corpus of
`(identity + evidence + mode + checkpoint) → (decision, conflict, exit)` cases
covering at minimum: no witness (dormant), bound with empty store per mode,
consistent evidence (allow), four-side disagreement without a decision (ask or
block by mode), a persisted decision, a checkpoint-scoped opt-out, branch reuse
after close (new item), two open items (conflict), detached HEAD per mode, and
missing-binary host behavior. Parity tests MUST drive the built binary through
the corpus, and the migrated host tests MUST exercise the bridge rather than a
second grader.

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
