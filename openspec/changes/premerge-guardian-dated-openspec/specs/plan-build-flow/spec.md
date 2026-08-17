# Delta for plan-build-flow

This delta aligns the plan-build-flow archive contract with the canonical
OpenSpec provider. It does not alter artifact minima, verification evidence,
tier inference, planning-root resolution, or active-folder enforcement.

## MODIFIED Requirements

### Requirement: Pre-merge archive gate

Archive-tail MUST run on the review branch before merge. The canonical OpenSpec
archive destination is:

```text
openspec/changes/archive/YYYY-MM-DD-<slug>/
```

where the date is a valid ISO calendar date selected by the archive operation
and `<slug>` is the exact change slug. Post-merge archive as the change boundary
MUST remain rejected.

An exact undated destination,
`openspec/changes/archive/<slug>/`, remains a legacy compatibility form for
historical archives. New archive operations MUST use the dated form. Historical
archive directories MUST NOT be renamed, moved, or rewritten to satisfy this
contract.

#### Scenario: OpenSpec archive-tail uses the dated provider path

- **GIVEN** a change is ready to close on the review branch
- **WHEN** archive-tail moves the active change folder
- **THEN** it creates `openspec/changes/archive/YYYY-MM-DD-<slug>/`
- **AND** the date prefix is a valid calendar date
- **AND** the active `openspec/changes/<slug>/` folder is removed on that review branch

#### Scenario: Historical undated archive remains readable

- **GIVEN** an existing historical archive is exactly
  `openspec/changes/archive/<slug>/`
- **WHEN** the pre-merge flow evaluates that change
- **THEN** the exact undated path remains a valid legacy archive destination
- **AND** no historical directory is renamed or rewritten

### Requirement: Pre-merge merge guardian

Before merge, missing tier artifacts, a still-active change folder, an
unresolvable archive, or (for Standard and Full) missing verify evidence per the
staged verify gate is a hard stop. Agents MUST invoke
`$AI_SPECS_HOME/lib/_internal/premerge_guardian.py` with the propagated planning
root as already defined by this requirement.

For the slug under check, the guardian MUST resolve the archive using only
direct child directories of `openspec/changes/archive/` and these exact forms:

1. One dated candidate named `YYYY-MM-DD-<slug>` whose prefix is a valid ISO
   calendar date.
2. One exact undated `<slug>` candidate as legacy compatibility, only when no
   dated candidate exists.

The guardian MUST fail closed when two or more dated candidates exist, when a
dated and undated candidate both exist, when a candidate-shaped directory has
an invalid calendar-date prefix, or when a near-match name would otherwise be
accepted by substring, wildcard, or directory-order matching. It MUST report a
missing-archive blocker when no valid candidate exists. Unrelated archive
directories that are not candidate-shaped for the requested slug remain out of
scope.

Candidate inspection is limited to direct child directories of
`openspec/changes/archive/`. Candidate names and blockers MUST be reported
explicitly when resolution fails; the guardian MUST NOT inspect an archive
through a recursive search or infer a candidate from a partial slug match.

After resolution, the guardian MUST apply the existing tier-minimum and
verification checks without changing their rules. A valid archive MUST NOT
override the active-folder blocker.

#### Scenario: Single dated archive passes the path gate

- **GIVEN** the active change folder is absent
- **AND** exactly one valid `openspec/changes/archive/YYYY-MM-DD-<slug>/`
  directory exists with the required artifacts and evidence
- **WHEN** the pre-merge guardian runs
- **THEN** it evaluates that dated directory
- **AND** it reports OK when the existing artifact and verification gates pass

#### Scenario: Exact undated archive uses legacy fallback

- **GIVEN** no dated candidate exists
- **AND** exactly `openspec/changes/archive/<slug>/` exists with the required
  artifacts and evidence
- **WHEN** the pre-merge guardian runs
- **THEN** it evaluates the exact undated directory as legacy compatibility

#### Scenario: Multiple dated candidates are ambiguous

- **GIVEN** two or more dated directories match the requested slug
- **WHEN** the pre-merge guardian runs
- **THEN** it blocks with an ambiguity error
- **AND** it names the candidate paths
- **AND** it does not select the newest, oldest, or filesystem-first directory

#### Scenario: Dated and undated candidates are ambiguous

- **GIVEN** both `archive/<slug>/` and one valid dated archive for the slug exist
- **WHEN** the pre-merge guardian runs
- **THEN** it blocks with an ambiguity error
- **AND** it does not silently prefer the dated or undated path

#### Scenario: Invalid date and near-match names are rejected

- **GIVEN** an archive directory resembles the dated provider form but has an
  invalid calendar date or does not match the exact `<slug>` name
- **WHEN** the pre-merge guardian runs
- **THEN** it blocks rather than accepting the directory through a broad match
- **AND** the blocker identifies the invalid or near-match archive name

#### Scenario: Active folder still blocks with a valid dated archive

- **GIVEN** `openspec/changes/<slug>/` still exists
- **AND** a valid dated archive also exists
- **WHEN** the pre-merge guardian runs
- **THEN** it reports the existing active-folder blocker
- **AND** archive resolution does not bypass the requirement to run archive-tail

#### Scenario: Existing artifact and verification gates remain unchanged

- **GIVEN** a single archive path is resolved successfully
- **WHEN** the guardian inspects the archive
- **THEN** the existing tier minima and Standard/Full verification checks apply
- **AND** unrelated archived changes are not inspected
