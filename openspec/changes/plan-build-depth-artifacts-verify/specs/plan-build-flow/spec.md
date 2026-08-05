# Delta for Plan-Build-Flow

## MODIFIED Requirements

### Requirement: Change depth classifier

The bundled skill SHALL classify each substantial request into exactly one
planning depth before production edits and SHALL require these **minimum
planning artifacts** before build (and, when archived, before merge via the
pre-merge guardian):

- **Full** — planning chain: explore → proposal → spec → design → tasks.
  Minimum files: `tasks.md`, plus `proposal.md` or `design.md`, plus at least
  one spec delta under `specs/`.
- **Standard** — planning chain: conditional explore → proposal → spec → tasks.
  Minimum files: `proposal.md`, `tasks.md`, plus at least one spec delta under
  `specs/`. When Standard explore criteria (below) match, `explore.md` is also
  required before authorization. When criteria do not match, `tasks.md` MUST
  record an `Explore: skipped — …` line.
- **Light** — planning chain: proposal → tasks.
  Minimum files: `proposal.md` and `tasks.md`.

The chosen depth MUST be recorded in `tasks.md`. Direct implementation verbs on
a request with no existing change folder MUST NOT skip planning.

This requirement does **not** define adversarial handling when an explicit user
depth conflicts with classifier signals; that behavior is owned by a separate
change.

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

#### Scenario: Full depth for ambiguous scope

- GIVEN a request for a new cross-cutting capability with unclear boundaries
- WHEN planning starts
- THEN the full planning chain runs
- AND tier minimum artifacts exist before build

#### Scenario: Direct implement still plans first

- GIVEN the user says "implement X" with no `openspec/changes/<slug>/` folder
- WHEN the skill evaluates the request
- THEN it classifies depth and runs the plan phase before build
- AND stops for authorization unless the tier is trivially light and inline build is allowed

## ADDED Requirements

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

### Requirement: Staged verify gate

After apply and before archive-tail / merge, the skill SHALL treat verification
evidence according to depth:

- **Light — advisory**: warn when verify evidence is missing; MUST NOT block
  archive or merge solely for missing verify evidence.
- **Standard — enforcement**: MUST NOT complete archive-tail intended for merge
  without verify evidence in the change folder (a `verify-report.md` whose
  overall verdict is not failing, or an equivalent verify evidence file that
  records a passing project verify command such as `./tests/validate.sh`).
- **Full — required**: MUST NOT complete archive-tail intended for merge without
  `verify-report.md` present with an overall passing / ready-for-archive verdict.

The pre-merge guardian SHALL machine-enforce Standard enforcement and Full
required modes when checking an archived change, and SHALL NOT block Light
solely for missing verify evidence.

#### Scenario: Light archive without verify evidence is allowed

- GIVEN Depth light and archived minima (`proposal.md`, `tasks.md`) are present
- AND no `verify-report.md` exists
- WHEN the pre-merge guardian runs
- THEN it does not add a verify-evidence blocker

#### Scenario: Standard merge blocked without verify evidence

- GIVEN Depth standard and archived tier minima are present
- AND neither `verify-report.md` nor equivalent passing verify evidence exists
- WHEN the pre-merge guardian runs
- THEN it fails with a plain-language verify-evidence blocker

#### Scenario: Full merge requires passing verify-report

- GIVEN Depth full and archived tier minima are present
- AND `verify-report.md` is missing or marks an overall failure
- WHEN the pre-merge guardian runs
- THEN it fails until a passing `verify-report.md` is archived

#### Scenario: Build sequence keeps verify before archive

- GIVEN an authorized Standard or Full change
- WHEN build runs
- THEN verify evidence is produced before archive-tail on the review branch

## MODIFIED Requirements

### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts, a still-active (non-archived) change
folder, or (for Standard/Full) missing verify evidence per the staged verify
gate is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` (defaulting
`AI_SPECS_HOME` to `$HOME/.ai-specs` when unset). Sync MUST NOT materialize a
per-project copy under `ai-specs/bin/`.

Hard blockers (do **not** merge):

1. `openspec/changes/<slug>/` still exists (active, not archived).
2. `openspec/changes/archive/<slug>/` is missing.
3. Archived folder lacks the tier minimum files for the resolved depth:
   - Light: `tasks.md` and `proposal.md`
   - Standard: `tasks.md`, `proposal.md`, and at least one `specs/**/*.md`
   - Full: `tasks.md`, `proposal.md` or `design.md`, and at least one
     `specs/**/*.md`
4. For Standard: archived folder lacks verify evidence as defined by the staged
   verify gate.
5. For Full: archived folder lacks a passing `verify-report.md`.

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
