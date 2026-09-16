# Ledger remote reconciliation

## Objective and rationale

Make remote reconciliation deterministic and provider-neutral: MCP transports observations, recipes declare provider adaptation, and Go owns validation/comparison. The agent coordinates and presents explicit decisions, not inferred rules. Current snapshots are inert and identity evidence cannot compare provider properties.

## Scope and constraints

Trello is the first integration; richer providers must not require Trello vocabulary in the core. Compare only declared properties. Bind observations to provider, scope and item identity. Distinguish observed-at from persisted-at. Missing, stale, unavailable or invalid data never agree. Expected values derive from declared delivery-event mapping, not local open/closed lifecycle alone.

Drift is reported with a pending explicit decision. Suspend only the disputed action, not independent work. Missing UI never implies consent. No automatic provider writes, network on edit hooks, second grader, new agent loop or Jira/OpenProject implementation. New authority belongs in Go. Preserve prior exploration and unrelated ignored files. No commit, push or merge authorized.

## Accepted direction

User explicitly switched SDD exploration to ODD, selected CLI governance with MCP transport and recipe adaptation, and chose notification plus ask_user for discrepancies instead of blanket blocking.

## Tracker

- **card_id**: `6aaad60a09eb8ba066120ba3`
- **url**: https://trello.com/c/UfTdfS1Y
- Compatibility link: `openspec/changes/ledger-remote-reconciliation/tasks.md`.

## Tasks

- [x] T1 — Implement a minimal Go observation/comparison contract with deterministic results and focused tests; preserve existing identity evidence and grading behavior.
- [x] T2 — Integrate recipe-declared adaptation and Trello acquisition through existing MCP/CLI coordination; keep remote reads off edit hooks and report unmapped events explicitly.
- [x] T3 — Integrate pending-decision notification/response behavior without implicit consent or blocking unrelated work; document boundaries and add contract/parity tests.
- [x] T4 — Build/verify trust-root artifacts, run full validation and independent verification as required, and reconcile documentation and tracker evidence.

## Acceptance and checks

T1: equal/different declared properties; missing fields; identity/scope mismatch; stale/malformed/future timestamps; unavailable observation; deterministic ordering; no changes to legacy identity conflict semantics. No provider-native names in core. Time is supplied for deterministic tests.
T2: provider-specific mapping remains outside core; explicit event expectations; wrong-scope observations rejected; unsupported mapping is pending/unreconciled, never guessed.
T3: drift yields explicit pending decision; headless leaves pending; unrelated work is not blocked; no implicit provider write.
T4: full validation and applicable independent checks pass; failed/skipped checks recorded.

TDD: enabled by project AGENTS.md (red-green-refactor). Focused runner: `go -C catalog/recipes/worktree-flow/gate test ./ledger`; broader Go: `go -C catalog/recipes/worktree-flow/gate test ./...`; full validation: `./tests/validate.sh`. Build verification: `bash scripts/build-gate.sh` and `bash scripts/verify-gate-sums.sh` after inspecting their requirements. RDD is off; assess writer diff to select ordinary independent verification.

## Progress and evidence

T1 implemented in new `reconcile.go` and `reconcile_test.go`; not accepted yet. Writer reports RED/GREEN; parent reproduced focused tests (2.849s); independent verifier reproduced ledger tests (2.773s) and all Go tests (16.068s / 2.596s). Legacy tracked files unchanged. Native assessment unavailable: treat as high risk and use independent verification. Tracker card created in In Progress.

Parent review requires corrections before T1 acceptance: zero Now and non-positive MaxAge must not disable freshness silently; empty expectations and empty/duplicate property names must return non-agreeing invalid-input outcomes in Go. No existence-only or freshness-opt-out mode was authorized. Fix misleading findings comments. These are contract corrections, not new provider scope. T1 remains unchecked; use regression RED/GREEN. Worktree: `.worktrees/ledger-remote-reconciliation`, branch `feat/ledger-remote-reconciliation`, baseline `6321248`.

Live board inspection found Review and Done, but no Published list. Do not create it or invent a mapping; publication integration remains unresolved and does not block T1.

## Next step

T1 accepted as an isolated comparator, not an integrated feature. Corrected writer observed 12 regression failures then focused GREEN (2.567s) and fresh all-Go GREEN (16.118s / 2.617s). Independent re-verification passed (fresh ledger 2.712s, all Go cached). Parent fixed the remaining unbound-findings documentation contradiction and reran focused tests successfully (2.878s).

T2 accepted: comparator integration verified independently across three review rounds (boundaries F1/F4/F5/F6 fixed and verified; conflict-snapshot suppression and observation budget verified; F2 structured config resolution verified). Recipe `[config.reconcile]` is a first-class validated section; sync validates and carries it; `recipe configure --set` can add it (updates to existing dotted-style blocks fail safe — follow-up). Go suites green (`test .` ~28-31s, `./ledger` ~2.5-3s), recipe suites green (57/67/18/11). Remaining known-open: closed-item/post-merge binding (`unbound-identity`), T3, T4.

T3 complete: pending-decision protocol in SKILL (three closed resolutions, no inferred consent, headless leaves pending, unrelated work continues, honesty wording) + contract test pinning decision inputs for all 14 reachable non-agree outcomes. No behavioral Go change needed; contract already held. Parent reproduced test (PASS, 0.909s). Follow-ups recorded: recipe-config-write dotted-style update, closed-item/post-merge binding.

T4 accepted by the parent. Gate trust root
regenerated with the canonical go1.24.13 toolchain: `bash scripts/build-gate.sh`
built the four targets into `dist/` (all four committed digests changed because the
gate source changed in T1-T3), and `bash scripts/verify-gate-sums.sh <generated>
<committed>` reports `ok — 4 digest entries match the committed trust root`. Full
validation green on the frozen candidate: `./tests/validate.sh` exit 0, `Ran 2026
tests`, `OK (skipped=2)`, gofmt clean. Fresh uncached Go suites green:
`go -C catalog/recipes/worktree-flow/gate test -count=1 ./...` (worktree-gate
34.783s, ledger 3.294s). Documentation reconciled: a `CHANGELOG` `[Unreleased] →
Added` entry; `docs/capabilities.md` and `docs/runtime-hooks.md` `remote`-producer
wording corrected from "this slice" to the `--evidence` file, with the explicit
opt-in `--reconcile` comparison named as off-hot-path and never a default. Skill
pending-decision wording checked against behavior: three closed resolutions, no
No provider/network
mutation and no commit/push.

Change implementation complete (T1-T4), review/delivery pending. Parent spot check
ok — regenerated sums from `dist/` match the committed trust root (an initial
mismatch was a stale pre-regeneration /tmp file, not the artifact). Next step:
native review at the deliverable boundary (RDD on, consent before execution), then
the user-owned commit/PR decision.
