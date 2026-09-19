# Go strangler mode resolution

## Objective
Move effective `ledger_mode` and legacy `gate_mode` resolution into the authoritative Go gate so shell hooks remain acquisition/dispatch bridges only, with parity coverage preserving existing precedence and off-path behavior.

## Tracker

- **card_id**: `6aa61f13786f559ac267fa45`
- **shortLink**: `XN5cTJi5`
- **url**: https://trello.com/c/XN5cTJi5/126-adopt-project-level-python%E2%86%92go-strangler-policy
- **note**: Follow-up implementation slice under the existing project-level Python-to-Go strangler policy card.

## Scope

- Define one provider-neutral Go resolver for effective ledger and worktree gate modes.
- Preserve precedence, valid-value sets, legacy aliases, defaulting, and `off` behavior at the CLI boundary.
- Reduce duplicated shell mode policy to thin argument/evidence acquisition and Go invocation.
- Add focused RED/GREEN parity and contract coverage before changing production code.
- Rebuild and verify the committed Go trust root with the canonical toolchain.

## Out of scope

- Converting unrelated Python runtime modules.
- Rewriting provider acquisition or tracker lifecycle semantics.
- Making parity suites mandatory in the default runner; track that as a follow-up slice.
- Changing the worktree gate's user-facing destination policy.

## Tasks

- [x] T1 — Pin current mode-resolution precedence and failure/off-path behavior with failing Go/host contract tests. RED: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveLedgerMode'` fails to compile because `ResolveLedgerMode` does not exist yet.
- [ ] T2 — Implement Go-owned effective mode resolution and route ledger/worktree entrypoints through it.
- [ ] T3 — Remove duplicate policy-bearing shell resolvers while preserving thin acquisition/dispatch bridges and update docs/tests.
- [ ] T4 — Rebuild the four trust-root artifacts, run focused and full validation, and record evidence.

## Acceptance criteria

- Go is the sole authoritative implementation of effective `ledger_mode` and `gate_mode` resolution.
- Shell hooks do not contain a second policy resolver or silently change precedence.
- Existing valid configurations, legacy aliases, invalid-value fallbacks, and `off` semantics remain compatible.
- Focused tests, Go tests, trust-root verification, and `./tests/validate.sh` pass.
- Each task closes with a work-unit commit on this feature branch.

## Progress

- Worktree: `.worktrees/go-strangler-mode-resolution`
- Branch: `feat/go-strangler-mode-resolution`
- Base: `development` at `03b5f2c`
- Status: T1 complete; the contract-only test is committed and production implementation is intentionally absent.

## Evidence

- RED: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveLedgerMode'` → expected build failure: `undefined: ResolveLedgerMode`.
- GREEN: pending; T2 must implement the exact tested contract.
- Full validation: pending.
- Trust-root verification: pending.

## Next step

Implement `ResolveLedgerMode` in Go and wire the ledger CLI to use it.
