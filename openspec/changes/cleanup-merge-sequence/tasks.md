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

- [ ] Add RED tests for cleanup order and final base sync.
- [ ] Add RED tests for stale local branch enumeration, no-PR path presence, absent-path refusal, and structural exact-path iteration.
- [ ] Implement stale local branch classification without weakening existing merge proof.
- [ ] Implement ordered deletion and final base sync with fail-closed behavior.
- [ ] Update Git merge workflow skill to remove `--delete-branch` guidance and encode cleanup sequence.
- [ ] Regenerate four published Go asset checksums if the module changes.
- [ ] Run focused Go tests and full `./tests/validate.sh`; record RED/GREEN evidence and falsifiability.
