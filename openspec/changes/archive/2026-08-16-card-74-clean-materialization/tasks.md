# Tasks: card-74-clean-materialization

Depth: standard

Branch / worktree: `change/card-74-clean-materialization` —
`/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card-74-clean-materialization`

Plan refs: `specs/release-materialization/spec.md`

**Stop for human authorization before any production code or tests.**

## Tracker

- **card_id**: `6a7caded57d03bc10d4e5944`
- **shortLink**: `6wq1nrhA`
- **url**: https://trello.com/c/6wq1nrhA/74-story-validate-clean-materialization-before-release
- **list**: In Progress
- **epic**: https://trello.com/c/qxP4SSnS/67-epic-next-release-open-compatibility-runtime-stability

## Tier rationale

Not Light: the card is the last implementation slice of epic #67. It
needs a written contract for what counts as release-candidate
materialization evidence, and a hermetic check that can fail the
candidate.

Not Full: no new user-facing command, no architecture fork, and no
release ritual. The surfaces are known (`init`/`sync`/`doctor`, lock,
adapters, cache, SHA256SUMS).

## Intent

Prove that the current `development` tip (`VERSION` 0.22.0 at
`78a3c30`) can materialize a clean consumer project. #69 already
recorded that this repo's dogfood lock (`cli_version = "0.21.0"`) is
stale by design and must not be treated as release evidence.

If the isolated run finds drift, fix product/catalog/tests in this
change. Do not "pass" by refreshing `ai-specs/.ai-specs.lock` or
other dogfood generated files.

## Out of scope

- Cutting the release (`VERSION` bump, CHANGELOG date, promote, tag)
- Marketplace / provider-ecosystem work
- Card #37 worktree modes (`always` / `ask` / `off`)
- Committing this repository's dogfood sync output

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~150–350 (hermetic test + possible small product fixes + skill pointer) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |

```text
Decision needed before apply: Yes (authorization gate)
Chained PRs recommended: No
400-line budget risk: Low
```

## P0 — Planning gate (this session)

- [x] Classify depth: standard
- [x] Spec delta: `release-materialization`
- [x] `tasks.md` (this file)
- [x] **Human authorization to implement**

## P1 — RED: isolated clean-materialization test

- [x] Add `tests/test_release_materialization.py`
- [x] Isolated temp project; `AI_SPECS_HOME` = candidate repo root
- [x] Representative manifest: foundational recipes (`worktree-flow`,
      `git-pr-flow`, `session-context`, `tdd-flow`, `plan-build-flow`)
      and agents `claude`, `cursor`, `opencode`, `pi`, `omp`
- [x] `init` + `sync` with `AI_SPECS_GATE_OFFLINE=1`
- [x] Assert lock `[meta].cli_version` equals repo `VERSION`
- [x] Assert `doctor` exits 0 with no `ERROR`
- [x] Assert expected generated outputs exist for each enabled agent
- [x] Assert CLI-bundled skills are not copied into
      `ai-specs/skills/`
- [x] Assert `SHA256SUMS` declares the candidate version and four
      platform digests
- [x] Assert the worktree-flow launcher materialized
- [x] Run the new test and record RED (or unexpected GREEN if the
      candidate already satisfies the contract)

## P2 — GREEN: make the gate pass

- [x] Implement the minimum product/catalog/test fix for any RED
      failure (test-contract only: skip `mcp_config_path` when no
      `[mcp.*]` is declared; no product/lib/catalog change)
- [x] Do not refresh or commit this repo's dogfood lock, `AGENTS.md`,
      or generated adapters as the fix
- [x] Re-run the new test and record GREEN

## P3 — Release-flow pointer

- [x] Add one short paragraph to
      `ai-specs/skills/release-flow/SKILL.md` stating that this
      isolated materialization gate must pass before the version bump
      (authored skill; `.claude/skills/` is generated and absent here)
- [x] Do not start the release ritual in this change

## P4 — Validate

- [x] `./tests/validate.sh` — 1672 tests, 116 skipped, exit 0
- [x] Record commands and results in `apply-progress.md`

## P5 — Close

- [x] Verify report
- [x] Archive on the review branch before merge
- [ ] PR to `development`
