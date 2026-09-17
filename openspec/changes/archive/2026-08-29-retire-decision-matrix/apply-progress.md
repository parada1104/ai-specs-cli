# Apply Progress: retire-decision-matrix

## Scope

Authorized Full-depth removal of the live `sdd-adaptive-contract` ceremony surface and consolidation into `plan-build-flow`.

## Completed

- Removed `openspec/specs/sdd-adaptive-contract/spec.md` from the live tree.
- Removed the obsolete `sdd` configuration surface from `openspec/config.yaml` while preserving the remaining YAML structure.
- Removed the `[sdd]` / `threshold` documentation from `docs/recipe-schema.md`.
- Updated the Trello recipe README and recipes catalog to point ceremony/depth classification at `plan-build-flow`.
- Added the Unreleased changelog entry.
- Updated manifest-contract tests and added retired-surface, migration-mapping, and token-scan coverage.
- Focused GREEN evidence: `python3 -m unittest tests.test_manifest_contract_docs` — 12/12 passed; `python3 -m unittest tests.test_plan_build_flow_recipe` — 25/25 passed.
- Full repository validation GREEN: `./tests/validate.sh` — 1378 tests OK, exit 0 (GNU bash 5.3.9, 2026-08-09).
- Delta spec scenario updated with the explicit test ban-list exemption; tracker block synced in proposal.md/tasks.md (card #66).
- `verify-report.md` written with Full evidence shape (PASS, `ready_for_archive: true`, `## Success-criteria mapping`), reconciling the pre-existing Bash baseline failures with the separate compatibility PR #189.

## TDD Evidence

The affected manifest-contract test was run RED after removing the documented `[sdd]` surface and before updating its assertions. The expected two assertions failed. The test was then updated and the focused suite returned GREEN with 12/12 tests passing.

## Not Yet Completed

- The change folder has not been archived — archive-tail deferred until the PR is created (plan-build pre-merge archive gate; archives preserved).
- Commit, push, and PR against `development` pending (no merge performed).
