# Epic: Migrate ai-specs-cli to a single Go binary — campaign track

Epic card: https://trello.com/c/qwlHQ7Xa · Branch model: `epic/go-single-binary` (integration) ← `change/<slug>` (cards). Nothing reaches `development` until card 16 promotion. Operating manual: `docs/go-migration-orchestration-handoff.md`. Parity contract: `docs/go-migration-parity-contract.md`.

## State

- 2026-09-27: integration branch rebased onto development `7b1fd75` (absorbed the strangler rank-1..3 actuators + advisory-hardening campaign, 65 commits), conflicts resolved preserving the EPIC CONTRACT lines. Tip `ab3feec`, published (user-side force-with-lease; safety policy blocks orchestrator force-pushes).
- Worker pattern (user-directed, voice-agent style): one Herdr **worktree-bound** session per card under the `ai-specs-cli` parent workspace; worker owns the complete card; orchestrator is reviewer and owns staging/commits/PRs/merges. Worktree-bound sessions require `herdr worktree open --workspace <parent>` — plain `workspace create` produces loose, ungrouped sessions.
- Merge cadence: card PRs → `epic/go-single-binary`, auto-merge authorized (standing decision). `hub.py` (D5×D4) and `skills-list.sh` (D3×D4) collisions serialized by the orchestrator. D1+D5 must land before card 03.
- Rank-4 gate slices (spec_promotion / premerge_guardian into the gate binary) deliberately folded into this epic — cutover deletes the whole Python layer; no double implementation.

## Wave 1 (running, launched 2026-09-27)

| Worker | Card | Branch / worktree | Herdr |
|---|---|---|---|
| go-02-blackbox-tests | [Go 02] POh1vmd6 — coupled suite → black box | `change/go-02-blackbox-tests` | wD |
| fix-manifest-lock-writes | lVB8gEin — D1/D2/D13 | `change/fix-manifest-lock-writes` | wE |
| fix-false-success | sbYplwWF — D5/D3′/D4′/D24 | `change/fix-false-success` | wF |
| fix-deps-layer | DflM3ppP — D16/D17/D7/D8/D18 | `change/fix-deps-layer` | wG |
| fix-docs-drift | uKisnwrK — D26/D25/D27/D28/D30 | `change/fix-docs-drift` | wH |

Card 02 protocol: category 1 autonomous; categories 2+3 collected into ONE list → single user approval → second round applies it.

## Queue after wave 1

1. Card 03 parity harness (after D1+D5, single worker).
2. Tranche 4 — critical path `04 → 05/06 → 07 → 09/10 → 13`; off-path: 08, 11, 12, 14, 15.
3. Card 16 cutover — human's: blocking `base_branch` revert + the single promotion PR to `development`.

## Evidence trail

- Trello epic comment (wave-1 launch record) — 2026-09-27.
- Engram campaign mirror: obs 4150; advisory-hardening closure: obs 4140/4149.
- Baseline unit suite on rebased branch: `/tmp/epic-baseline-run.log` (background run 2026-09-27).
