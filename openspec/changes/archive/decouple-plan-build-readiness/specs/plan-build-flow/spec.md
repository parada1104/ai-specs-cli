# Delta for plan-build-flow

## MODIFIED Requirements

### Requirement: Artifact store degradation and default

The recipe SHALL separate persistence from readiness. Plan-build readiness
SHALL be proven exclusively by file-backed artifacts under the canonical
`openspec/changes/<slug>/` tree — `tasks.md`, committed tier-minimum planning
files, and `verify-report.md` where the staged verify gate requires it. The
preflight-resolved store (`openspec|engram|both`) MUST NOT be consulted for
readiness and MUST NOT alter any classifier, PR/archive gate, staged verify
gate, or pre-merge guardian decision.

The store SHALL act only as an external-session persistence preference. When
Engram is unavailable, the skill SHALL fall back to file artifacts. When Engram
is present but no preflight resolved a store, the default SHALL be file
artifacts under `openspec/changes/<slug>/`. Engram MAY mirror artifacts but MUST
NOT replace them; a memory-only presence MUST NOT satisfy any readiness check.
(Previously: readiness conflated with the persistence preference, so `engram`
could be read as a memory-only readiness source.)

#### Scenario: Default store with Engram but no preflight

- GIVEN Engram is available and no artifact-store preflight ran
- WHEN planning starts producing artifacts
- THEN artifacts are written as files, not memory-only

#### Scenario: Store selection never changes readiness

- GIVEN a store of `openspec`, `engram`, or `both` is resolved
- AND the same change folder state exists in each case
- WHEN the PR artifact gate or pre-merge guardian runs
- THEN the decision is identical across all three selections

#### Scenario: Openspec store keeps file-backed enforcement

- GIVEN the store resolves to `openspec`
- WHEN planning and gates run
- THEN artifacts are written under `openspec/changes/<slug>/`
- AND gate decisions follow the file-backed readiness invariant

#### Scenario: Engram memory-only cannot satisfy tier minima

- GIVEN the store resolves to `engram`
- AND `openspec/changes/<slug>/` lacks the tier minimum files while an Engram
  mirror holds them
- WHEN the PR artifact gate or pre-merge guardian runs
- THEN it blocks on the missing repository artifacts
- AND the Engram mirror does not change the decision

#### Scenario: Engram mirror cannot satisfy verify evidence

- GIVEN the store resolves to `engram`
- AND `verify-report.md` exists only in Engram, not in the change folder
- WHEN the staged verify gate or pre-merge guardian runs for a Standard or Full change
- THEN the verify evidence is treated as missing
- AND the change is blocked until the file exists

#### Scenario: Both store mirrors but never replaces canonical files

- GIVEN the store resolves to `both`
- WHEN planning produces artifacts
- THEN `openspec/changes/<slug>/` remains the canonical readiness source
- AND the Engram mirror never substitutes for a missing artifact

#### Scenario: No preflight and no Engram fall back to files

- GIVEN Engram is unavailable and no preflight resolved a store
- WHEN planning starts producing artifacts
- THEN file artifacts under `openspec/changes/<slug>/` are used
- AND no readiness check is skipped or relaxed
