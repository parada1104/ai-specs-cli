# Delta for Plan-Build-Flow

> **Baseline.** This delta applies to the canonical `openspec/specs/plan-build-flow/spec.md`
> **after** `plan-build-depth-adversarial` (#59) has landed — recipe
> `plan-build-flow` `1.5.0`. The four requirements #59 added
> (*Adversarial depth conflict detection*, *Conflict ask before planning chain*,
> *Depth resolution annotation*, *Higher decided tier completes its chain*) are
> **not** modified here and MUST survive this promotion byte-identical, together
> with the six topology requirements already on `development`.
>
> The *Change depth classifier* block below reproduces #59's final text verbatim,
> including its `signal` / explicit-request / standalone-`Depth:`-line paragraphs
> and its post-promotion bold scenario formatting. Exactly three edits differ,
> and no others: the Standard and Light chain bullets, one added pointer
> sentence, and the Light scenario's `THEN` line. See
> `design.md` §11 for the line-level merge procedure and its verification
> commands.

## MODIFIED Requirements

### Requirement: Change depth classifier

The bundled skill SHALL classify each substantial request into exactly one
planning depth before production edits:

- **Full** — explore → proposal → spec → design → tasks
- **Standard** — conditional explore → proposal → spec → tasks
- **Light** — proposal → tasks

Minimum artifacts per depth are normative in *Depth artifact minima*; this
requirement names the chain order only and MUST NOT restate the minima.

Classification SHALL compute a **signal** tier from size/scope heuristics AND
separately detect an **explicit user depth request** when the user names a tier
or clearly equivalent planning depth (including common English and Spanish
phrasings such as "full SDD", "flujo completo", "solo tasks", "tasks only").

The **decided** depth MUST be recorded in `tasks.md` as a standalone lowercase
line of the exact form `Depth: <light|standard|full>`, with no trailing text on
that line, so existing tier-inference consumers keep matching it. Direct
implementation verbs on a request with no existing change folder MUST NOT skip
planning.

#### Scenario: Full depth for ambiguous scope

- **GIVEN** a request for a new cross-cutting capability with unclear boundaries
- **AND** the user did not state a conflicting explicit depth
- **WHEN** planning starts
- **THEN** the full planning chain runs
- **AND** tier minimum artifacts exist before build

#### Scenario: Light depth for scoped fix

- **GIVEN** a one-file bug fix with an explicit file and expected edit
- **AND** the user did not state a conflicting explicit depth
- **WHEN** planning starts
- **THEN** `proposal.md` and `tasks.md` are required, and nothing else
- **AND** no production code is modified during planning

#### Scenario: Direct implement still plans first

- **GIVEN** the user says "implement X" with no `openspec/changes/<slug>/` folder
- **WHEN** the skill evaluates the request
- **THEN** it classifies depth and runs the plan phase before build
- **AND** stops for authorization unless the tier is trivially light and inline build is allowed

### Requirement: PR artifact gate

The skill and generated brief fragments SHALL block PR/MR creation until the
matching `openspec/changes/<slug>/` folder on the review branch contains the
tier minimum planning files defined by *Depth artifact minima* and those files
are committed.

#### Scenario: PR blocked without change folder

- GIVEN implementation is complete but no change folder exists on the branch
- WHEN an agent attempts to open a PR
- THEN the skill stops with a blocker to complete planning first

#### Scenario: PR allowed with tier minimum files

- GIVEN a standard-tier change with `proposal.md`, `tasks.md`, and spec deltas
  under `specs/`
- WHEN the artifact gate is evaluated before PR creation
- THEN PR creation may proceed

#### Scenario: PR blocked for Light without proposal

- GIVEN a light-tier change whose committed folder holds only `tasks.md`
- WHEN the artifact gate is evaluated before PR creation
- THEN the skill stops with a blocker naming `proposal.md`

### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts, a still-active (non-archived) change
folder, or (for Standard and Full) missing verify evidence per the staged verify
gate is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` (defaulting
`AI_SPECS_HOME` to `$HOME/.ai-specs` when unset). Sync MUST NOT materialize a
per-project copy under `ai-specs/bin/`.

Hard blockers (do **not** merge):

1. `openspec/changes/<slug>/` still exists (active, not archived).
2. `openspec/changes/archive/<slug>/` is missing.
3. Archived folder lacks the tier minimum files from *Depth artifact minima*:
   - Light: `tasks.md` and `proposal.md`
   - Standard: `tasks.md`, `proposal.md`, and at least one `specs/**/*.md`
   - Full: `tasks.md`, `proposal.md` or `design.md`, and at least one
     `specs/**/*.md`
4. Standard: archived folder lacks a `verify-report.md` that satisfies the
   Standard evidence shape in *Staged verify gate*.
5. Full: archived folder lacks a `verify-report.md` that satisfies the Full
   evidence shape in *Staged verify gate*.

The guardian MUST NOT add a blocker for a missing `explore.md` at any depth, and
MUST NOT add a verify blocker at Light. The guardian SHALL evaluate only the
slug under check; it MUST NOT inspect or require changes to other archived
changes.

#### Scenario: Merge blocked when change folder still active

- GIVEN `openspec/changes/<slug>/` still exists (not archived)
- WHEN an agent attempts to merge the PR/MR
- THEN the skill stops with a plain-language blocker requiring archive-tail first

#### Scenario: Guardian path is CLI-home

- GIVEN `plan-build-flow` (or a VCS merge skill) is enabled
- WHEN an agent runs the pre-merge guardian
- THEN it uses `${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py`
- AND the recipe does not target `ai-specs/bin/premerge_guardian.py`

#### Scenario: Light archive requires proposal

- GIVEN Depth light and the archive contains `tasks.md` but not `proposal.md`
- WHEN the pre-merge guardian runs
- THEN it fails with a tier-minima blocker naming `proposal.md`

#### Scenario: Standard archive requires proposal and spec

- GIVEN Depth standard and the archive lacks `proposal.md` or any `specs/**/*.md`
- WHEN the pre-merge guardian runs
- THEN it fails with a tier-minima blocker

#### Scenario: Missing explore is never a guardian blocker

- GIVEN Depth full and an archive with tier minima and a passing `verify-report.md`
- AND no `explore.md` in the archived folder
- WHEN the pre-merge guardian runs
- THEN it reports OK
- AND explore enforcement remains a plan-phase skill responsibility

#### Scenario: Guardian ignores unrelated archived changes

- GIVEN other folders under `openspec/changes/archive/` predate this contract and
  lack `proposal.md` or verify evidence
- WHEN the pre-merge guardian runs for the slug under merge
- THEN only that slug is evaluated
- AND no blocker mentions the older archived changes

## ADDED Requirements

### Requirement: Depth artifact minima

The bundled skill SHALL require these minimum planning artifacts before build,
and the pre-merge guardian SHALL enforce the same sets against the archived
folder before merge:

- **Light** — `proposal.md` and `tasks.md`. A short proposal (Why / What /
  Non-goals) satisfies Light; `design.md` is not required.
- **Standard** — `proposal.md`, `tasks.md`, and at least one spec delta under
  `specs/`. `explore.md` is additionally required at plan time when the Standard
  explore criteria match.
- **Full** — `tasks.md`, plus `proposal.md` or `design.md`, plus at least one
  spec delta under `specs/`. `explore.md` is expected as the first chain
  artifact and is enforced by the skill, not by the guardian.

These minima define the "tier minimum planning files" referenced by the PR
artifact gate and the pre-merge merge guardian. They do not change how a depth
is decided, and they do not alter adversarial depth conflict handling.

#### Scenario: Light requires proposal and tasks

- GIVEN a one-file bug fix classified as Light
- WHEN planning completes
- THEN both `proposal.md` and `tasks.md` exist under the change folder
- AND no production code was modified during planning

#### Scenario: Standard requires proposal, tasks, and spec

- GIVEN a scoped multi-file feature classified as Standard
- WHEN planning completes
- THEN `proposal.md`, `tasks.md`, and at least one `specs/**/*.md` exist
- AND either `explore.md` exists or `tasks.md` contains `Explore: skipped —`

#### Scenario: Full minima unchanged by this contract

- GIVEN a change classified as Full
- WHEN planning completes
- THEN `tasks.md`, `proposal.md` or `design.md`, and at least one
  `specs/**/*.md` exist
- AND the guardian does not require `explore.md` on disk

#### Scenario: Decided deeper tier uses the deeper minima

- GIVEN a depth conflict resolved in favour of a deeper tier
- WHEN planning continues
- THEN the minima of the decided tier apply
- AND artifacts already written for the shallower tier are not relabelled as
  satisfying the deeper tier

### Requirement: Standard explore enforcement criteria

For Standard depth, the skill SHALL require `explore.md` when any of the
following hold at plan start:

1. Two or more plausible approaches with material trade-offs.
2. Concrete files to edit cannot yet be named.
3. Project docs or skills conflict on the approach.
4. The user signals uncertainty about approach or location.
5. A prior attempt at the same intent failed or was reverted.

The skill SHALL skip `explore.md` only when all of the following hold:

1. Concrete file path(s) and expected behavior are known.
2. A single obvious approach exists in a known area.
3. None of the require-explore signals above apply.

When explore is skipped, `tasks.md` MUST include a one-line
`Explore: skipped — <reason>` record before authorization.

Explore enforcement — at Standard and at Full — is a plan-phase skill
responsibility. No machine gate SHALL block PR creation, archive-tail, or merge
for a missing `explore.md`. These criteria decide whether explore runs; they do
not decide which depth wins when an explicit request conflicts with the signal
tier.

#### Scenario: Explore required for multi-approach Standard change

- GIVEN Standard depth and two plausible approaches
- WHEN planning runs
- THEN `explore.md` is written before authorization
- AND the plan does not rely on an Explore skipped line alone

#### Scenario: Explore skipped with recorded reason

- GIVEN Standard depth, named files, and a single obvious approach
- WHEN planning runs
- THEN `explore.md` MAY be omitted
- AND `tasks.md` contains `Explore: skipped —` with a short reason

#### Scenario: Explore stays skill-enforced at Full

- GIVEN Full depth and no `explore.md`
- WHEN the skill runs the plan phase
- THEN it treats the missing explore as a plan-phase gap to fix
- AND no machine gate blocks PR, archive, or merge for it

### Requirement: Staged verify gate

After apply and before archive-tail, and again before merge, the verification
evidence for a change SHALL be evaluated according to its decided depth:

- **Light — advisory**: warn when verify evidence is missing; MUST NOT block
  archive-tail or merge solely for missing verify evidence.
- **Standard — enforcement**: a dedicated `verify-report.md` MUST exist in the
  change folder and MUST satisfy the Standard evidence shape below.
- **Full — required**: a dedicated `verify-report.md` MUST exist and MUST
  satisfy the Full evidence shape below.

**Standard evidence shape.** `verify-report.md` MUST record auditable evidence:
the verify command that was run, its exit status, a valid calendar date in
`YYYY-MM-DD` form, and the commit SHA it was run against; and its overall
verdict MUST NOT be a failing verdict. Recording the evidence as a section
inside `tasks.md`, or in any file other than `verify-report.md`, does NOT
satisfy Standard.

**Full evidence shape.** `verify-report.md` MUST state a strict global `PASS`
verdict, MUST carry an explicit `ready_for_archive: true` marker, and MUST
contain a `## Success-criteria mapping` block. The authoritative source is
`proposal.md` when present, otherwise `design.md`. An existing proposal with a
missing or empty `## Success Criteria` section MUST block rather than fall back
to design. The authoritative source MUST contain exactly one non-empty heading;
duplicate `## Success Criteria` headings MUST be rejected. Each top-level bullet
is assigned a 1-based ordinal, and the report MUST contain exactly one
`- Criterion N: PASS` row for every ordinal. Duplicate, missing, unknown, or
non-PASS mapping rows fail Full.

Enforcement applies at two points: the skill MUST NOT complete archive-tail for
a Standard or Full change whose evidence does not satisfy its shape, and the
pre-merge guardian MUST re-check the archived folder and block the merge on the
same rule. Light MUST NOT be blocked at either point for missing evidence.
There is no bypass flag.

#### Scenario: Light archive without verify evidence is allowed

- GIVEN Depth light and minima (`proposal.md`, `tasks.md`) are present
- AND no `verify-report.md` exists
- WHEN archive-tail runs and the pre-merge guardian runs
- THEN neither step adds a verify-evidence blocker
- AND the skill may still warn that evidence is missing

#### Scenario: Standard archive-tail blocked without a verify report

- GIVEN Depth standard, tier minima present, and no `verify-report.md`
- WHEN the agent attempts archive-tail on the review branch
- THEN it stops with a plain-language verify-evidence blocker before archiving

#### Scenario: Standard merge blocked without a verify report

- GIVEN Depth standard and archived tier minima are present
- AND no `verify-report.md` is archived, or its verdict is failing
- WHEN the pre-merge guardian runs
- THEN it fails with a plain-language verify-evidence blocker

#### Scenario: Standard evidence in tasks.md is rejected

- GIVEN Depth standard and a verify evidence section written inside `tasks.md`
- AND no `verify-report.md` exists
- WHEN the staged verify gate is evaluated
- THEN the change is treated as missing verify evidence

#### Scenario: Standard report needs command, exit, date, and SHA

- GIVEN Depth standard and a `verify-report.md` that claims success without
  naming a command, exit status, date, and commit SHA
- WHEN the staged verify gate is evaluated
- THEN it fails with a blocker naming the missing auditable fields

#### Scenario: Full merge requires strict PASS, ready marker, and complete mapping

- GIVEN Depth full and archived tier minima are present
- AND `verify-report.md` is missing, or its global verdict is not `PASS`, or it
  lacks `ready_for_archive: true`, or omits any success-criteria mapping row
- WHEN the pre-merge guardian runs
- THEN it fails until a conforming `verify-report.md` is archived

#### Scenario: Build sequence keeps verify before archive

- GIVEN an authorized Standard or Full change
- WHEN build runs
- THEN verify evidence is produced before archive-tail on the review branch
- AND the pre-merge guardian re-checks the same evidence after archive
