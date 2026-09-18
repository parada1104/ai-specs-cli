# Flow-agnostic tracker ledger lifecycle

## Objective
Make Tracker Ledger lifecycle recording independent of SDD, ODD, or any named agent workflow. A generic runtime/VCS surface must create and link a local ledger item when a valid human-authored tracker binding exists, and close it after a VCS merge boundary without provider writes.

## Root cause
The shipped ledger intentionally chose agent-only `open|link|close` writes (L2/DW1). Hooks only grade, the project remains `warn`, and no generic caller issues writes. The witness is bound, but every new identity therefore grades `needs-item` and continues silently.

## Product decisions
- A generic binding command seeds open+link for a valid external Tracker binding under `.git/ai-specs/ledger`; no cardless item is invented.
- The binding command is usable before any apply boundary and requires no SDD/ODD/OpenSpec artifact.
- The local lifecycle writer is generic and provider-write-free; it does not require SDD, ODD, OpenSpec, or a particular agent.
- VCS `post-merge` is the automatic close trigger. The direct lifecycle host remains an explicit fallback for environments without the installed Git hook.
- Go remains the sole durable-state writer and grader; shell/Python remain acquisition/transport bridges.
- Preserve D17: closed rows never become Primary; opening/linking is idempotent and collision-safe.

## Tracker

- **card_id**: `AImzsLWw`
- **url**: https://trello.com/c/AImzsLWw/37-feat-worktree-flow-recipe-modes-always-ask-off
- **note**: Follow-up to the tracker-ledger and worktree-flow dogfood; reuse existing related card because Trello MCP is unavailable.

## Scope
- Amend the canonical tracker-ledger contract and recipe/runtime documentation to supersede agent-only opening while keeping grading pure and provider writes forbidden.
- Add a generic external binding command and an atomic Go lifecycle write for valid bindings (open+link), preserving explicit write compatibility.
- Make the generic tracker host grade the seeded branch binding at apply-start, with safe fail-closed write errors and fail-open grade/infrastructure behavior as appropriate.
- Install/distribute a VCS `post-merge` hook that closes the matching local item without coupling to SDD/ODD.
- Refresh dogfood materialization and explicitly exercise the flow with tests and ledger state evidence.

## Tasks

- [x] T1 — Write RED contract tests for flow-agnostic open/link and VCS close behavior.
- [x] T2 — Implement the Go atomic lifecycle writer and host bridge.
- [x] T3 — Install/distribute the generic VCS post-merge close hook and update integration tests.
- [x] T4 — Reconcile canonical spec, recipe/agent docs, and stale generated assets.
- [x] T5 — Run focused/full validation, dogfood sync/doctor, and record ledger evidence.

## Acceptance criteria
- A valid external branch binding can be seeded once through the generic ledger command and produces one idempotent local open+link, independent of SDD/ODD/no-flow execution.
- Missing/invalid tracker binding never invents a local or provider item; mode behavior remains observable.
- A VCS post-merge trigger closes the matching local item idempotently and preserves closed-row/D17 semantics.
- No provider API/MCP write is performed by lifecycle recording.
- Hooks work from fresh materialization; no stale retired Python host remains.
- Focused tests and `./tests/validate.sh` pass.

## Non-goals
- Creating, moving, commenting, labeling, or otherwise mutating provider cards.
- Inferring delivery from OpenSpec archive state or from a named agent workflow.
- Rewriting historical ledger rows or retroactively fabricating records for merged branches.
- Releasing `0.23.1` in this change; release follows after the lifecycle fix is merged and audited.

## Progress

- Worktree: `.worktrees/tracker-ledger-flow-agnostic`
- Branch: `feat/tracker-ledger-flow-agnostic`
- Base: `development` at `90c19a3`
- State: T1–T4 implemented and verified in the feature worktree; no commit yet.
- Evidence: focused suites green; Go gate rebuilt with go1.24.13; trust-root verification passed; `./tests/validate.sh` ran 2139 tests with 2 skips.
- Ledger evidence: generic branch-level `bind` created one open item for this branch in the Git-common-dir store; `ai-specs doctor` reported tracker-ledger OK, with work-start unhosted INFO.
- Dogfood: in-progress `bash bin/ai-specs sync .` and `bash bin/ai-specs doctor .` completed successfully; generated AGENTS/lock/manifest changes were restored per dogfood isolation, while the refreshed ignored hook/config state was retained only as verification output.
- Delivery: commits `2cf55f5`, `a8ccc82`, and `f076233`; PR #264 — https://github.com/parada1104/ai-specs-cli/pull/264 — labeled `type:feature`.
- Native review: attempted, but the controller selected an accumulated historical/generated baseline and then hit a native revision race while abandoning it; no reviewer capture was submitted. Independent verification is the delivery evidence.
- Known note: repository-wide `openspec validate --specs` still reports unrelated pre-existing spec failures; tracker-ledger and runtime-hook specs validate.
