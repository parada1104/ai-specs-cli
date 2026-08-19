# Tasks: post-merge cleanup sequence

Depth: full

Requested depth: full
Signal depth: full
Decided depth: full
Decision source: user
Explore: completed — existing Go proof, stale branch evidence, ordering, and release trust-root constraints required broad exploration.

## Tracker

- **card_id**: `88`
- **url**: https://trello.com/c/BY26fvb3

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~700 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes; coordinator owns delivery slicing |
| Decision needed before apply | No; human authorized this implementation |

## Implementation

- [x] Add RED tests for cleanup order and final base sync.
- [x] Add RED tests for stale local branch enumeration, absent-path refusal, and structural exact-path iteration.
- [x] Implement stale local branch classification without weakening existing merge proof.
- [x] Implement ordered deletion and final base sync with fail-closed behavior.
- [x] Update Git merge workflow skill to remove `--delete-branch` guidance and encode cleanup sequence.
- [x] Regenerate four published Go asset checksums if the module changes.
- [x] Run focused Go tests and full `./tests/validate.sh`; record RED/GREEN evidence and falsifiability.

## Round-one judgment corrections

- [x] Remove the no-PR path-presence fallback; path existence is not merge evidence.
- [x] Scan every pull request for a head instead of returning on the first without a merge commit.
- [x] Delete the remote branch before the local one so a remote failure stays recoverable.
- [x] Add the falsifying positive test for the NUL-delimited newline path.
- [x] Replace the skill's presence assertions with an order-sensitive one and correct the stale prose.
- [x] Update the spec delta to match the implemented order and evidence rules.
