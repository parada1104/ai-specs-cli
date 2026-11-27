# Delta for change-ledger

Splits the single ledger into three capability ledgers inside the CLI, adds the
read-only composite view, and fixes the reporting contract so an advisory is
visible regardless of the exit code.

## ADDED Requirements

### Requirement: Three capability ledgers, each owning its facts

The CLI SHALL keep three ledgers, and no others:

- **worktree** — tree identity, proof of merge, cleanup and integration;
- **plan-build** — the change layer: artifact, inferred tier, refutations,
  archive;
- **tracker** — reconciliation between a tracker item and the change.

Each ledger SHALL own its facts, its checkpoints and its decision log. A ledger
MUST NOT read or write another ledger's store, and MUST NOT derive a fact owned by
another. Ledgers SHALL converse only through read-only queries that return values
and never share mutable state.

#### Scenario: A worktree fact is never recomputed by the tracker ledger

- **WHEN** the tracker ledger needs to report a change as merged
- **THEN** it asks the worktree ledger and reports the answer, including `unknown`,
  instead of computing a merge itself

#### Scenario: One corrupt store does not silence the others

- **WHEN** one ledger's store is unreadable
- **THEN** that part of the composite answer is `unknown`, the other parts keep
  answering, and the CLI names the corrupt store

### Requirement: Storage is an implementation detail reached only through the CLI

Ledger state SHALL be reachable only through the `ai-specs` CLI. No agent, hook,
script or external tool may read or write those files directly, and the on-disk
layout SHALL NOT be treated as a contract: it may change without a migration for
consumers. The store SHALL remain append-only for decisions, atomic (lock, temp
file, fsync, rename) and located under the Git common directory.

The existing single store SHALL migrate to the split layout with a double-read
window of one version. The historical rows present at migration SHALL be preserved
byte-identically as history and MUST NOT be rewritten or discarded.

#### Scenario: Old rows survive the migration untouched

- **WHEN** the CLI first writes a decision after the migration
- **THEN** the pre-existing rows remain readable and unmodified, and the new
  decision lands in the ledger that owns it

#### Scenario: Direct file access is not supported surface

- **WHEN** the layout changes in a later release
- **THEN** no migration is owed to consumers, because only the CLI shape
  (`changes status --json`) is contractual

### Requirement: A composite read-only view answers what you can do next and why

`ai-specs changes status` SHALL report, from the three ledgers and consumed review
receipts: the change identity, its phase, the plan tier and refutations, the
tracker state, the review state, the recommended next step, the reason, and
whatever is blocked. It MUST NOT write anything.

`ai-specs changes next` SHALL print only the actionable command. Both SHALL accept
`--json`, `--explain` (the evidence behind each fact), `--ledger
worktree|plan|tracker` (one ledger, uncomposed) and `--rebuild` (recompute a
discarded cache). A change SHALL be resolved by branch when no slug is given.

Exit codes SHALL distinguish three cases: `0` no blocker, `2` a transition blocked
by an explicit project `require`, and `3` a fact the answer depends on is
`unknown`. "I do not know" MUST NOT be reported as "you may proceed".

#### Scenario: A merged branch says to clean up

- **WHEN** the worktree ledger proves the merge by any strategy and the tree is
  still present
- **THEN** `changes status` reports the phase as merged with cleanup pending, and
  `changes next` prints the cleanup command

#### Scenario: After cleanup the next step is integration, verified not executed

- **WHEN** the worktree, the remote branch and the local branch are all proven gone
- **THEN** the CLI verifies whether the integration branch already contains the
  merge, prints the command to update it when it does not, and never runs it

#### Scenario: An unknown fact is not a green light

- **WHEN** no merge evidence can be obtained because the remote is unreachable
- **THEN** the command exits `3` and states which fact is unknown

### Requirement: Postures advise by default and require only on request

Every transition SHALL carry a posture: `observe`, `advise` or `require`.
`advise` SHALL be the default for every risk level, including high risk, and SHALL
never block. `require` SHALL be opt-in per project, with per-checkpoint detail, and
SHALL be the only source of exit `2` from the composite view.

Risk SHALL rank the recommendation, not act as a switch. Risk thresholds SHALL
ship as CLI defaults and SHALL be adjustable per project.

#### Scenario: High risk advises by default

- **WHEN** a change touches authentication paths and the project declares no
  `require`
- **THEN** the composite view recommends a review and the exits, and never blocks

#### Scenario: A project opts in for one checkpoint

- **WHEN** a project requires the merge checkpoint
- **THEN** only that transition exits `2`, with the rule and the exit that
  resolves it, while the other transitions keep advising

### Requirement: Tracker reconciliation is explicit, provider-generic and never silent

The tracker ledger SHALL support three modes: `off`, `ask` and `always`, and SHALL
default to `ask` when a tracker capability is enabled. In `ask`, the CLI SHALL
offer to create or link the item; with no terminal it SHALL inform and continue
without blocking, recording the answer so the same change is not asked twice.
Adapters SHALL be provider-generic (Trello card, Jira issue, OpenProject work
package). With no tracker enabled, the tracker ledger SHALL report
`not applicable`, never a missing item.

When the tracker item and the change disagree, the CLI SHALL report the
disagreement and offer explicit reconciliation decisions, recommending one from
the facts it observed. It MUST NOT apply a reconciliation on its own.

#### Scenario: No tracker means not applicable

- **WHEN** no tracker capability is enabled in the project
- **THEN** the tracker part of the report is `not applicable` and no item is
  requested

#### Scenario: A disagreement offers a recommended decision

- **WHEN** the change is merged while the tracker item is still open
- **THEN** the CLI reports the disagreement, lists the reconciliation decisions
  with one recommended from the facts, and applies nothing until one is chosen

### Requirement: Review receipts are consumed, never fabricated

The composite view SHALL report the review state of the exact candidate from
receipts produced by the review authority (gentle-ai), referenced by candidate
identity. When no receipt is available the state SHALL be `unknown`; the CLI MUST
NOT infer approval, and MUST NOT copy review authority into its own store.

#### Scenario: No receipt is reported as unknown

- **WHEN** the candidate has no review receipt
- **THEN** the report says `unknown` and, when risk is high, recommends obtaining
  one

### Requirement: A worktree that is not a change owes nothing

The CLI SHALL distinguish a worktree that is not a change (audit, exploration,
spike, review) from a change without a plan, and it SHALL decide from facts, never
from text in a document.

A worktree SHALL be classified as a **change** when it shows evidence of product
work: divergence from the integration base on production paths (committed or
uncommitted), or an artifact that names a change (plan, delta, change folder).
Otherwise it SHALL be classified as a **workstream**, including a freshly created
branch with no divergence yet.

In a workstream the CLI MUST NOT open a ledger row, MUST NOT request a tracker
item, MUST NOT require an archive, MUST NOT assign a tier and MUST NOT apply
tier refutations. `changes status` SHALL report the phase `workstream` with the
facts that decided it and the exit that turns it into a change.

Where facts cannot separate intent from work — a worktree with product-path
divergence that is deliberately not a change — the CLI SHALL offer an explicit
`changes workstream [--reason]` decision, recorded append-only in the plan ledger.
That declaration SHALL be refused when an artifact naming a change exists, MUST NOT
waive any refutation for a change, and SHALL be reversible with `--end`.

#### Scenario: An audit worktree owes nothing

- **WHEN** a worktree has no product-path divergence and no artifact
- **THEN** `changes status` reports `workstream`, exits `0`, and no ledger gains a
  row, no item is requested and no archive is demanded

#### Scenario: The same worktree becomes a change

- **WHEN** a product path is modified or committed in that worktree
- **THEN** the next evaluation reports a change with the inferred tier, because the
  classification follows the facts and not a declaration

#### Scenario: A deliberate non-change work must be declared

- **WHEN** a worktree has product-path divergence that the owner does not consider a
  change
- **THEN** `changes workstream --reason <text>` records that decision, the CLI
  reports `workstream` with the reason, and `--end` returns it to change
  classification

### Requirement: Advisories are visible regardless of the exit code

A ledger advisory SHALL reach the agent or human whenever it is emitted, including
when the command exits `0`. The exit code SHALL express blocking, never
visibility. Generated runtime wrappers SHALL relay standard error unconditionally.

#### Scenario: A warning survives a zero exit

- **WHEN** a gate emits an advisory and exits `0`
- **THEN** the generated wrapper surfaces that advisory on standard error
