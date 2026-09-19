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
- [x] T2 — Implement Go-owned effective mode resolution and route ledger/worktree entrypoints through it. GREEN: focused resolver/ledger tests passed; Go now resolves raw config/env/hint inputs when `--ledger-mode` is omitted.
- [x] T3 — Remove duplicate policy-bearing shell resolvers while preserving thin acquisition/dispatch bridges and update docs/tests. GREEN: focused hook/config suite passed 105 tests with 4 expected skips; all three hooks pass `bash -n`.
- [x] T4 — Rebuild the four trust-root artifacts, run focused and full validation, and record evidence. GREEN: rebuilt the Go trust root with go1.24.13 and verified it with `bash scripts/verify-gate-sums.sh <temp-generated-sums> catalog/recipes/worktree-flow/bin/SHA256SUMS` (4 digest entries); Go tests, `go vet`, `gofmt`, full validation, parity suites, and `git diff --check` all clean. NOTE: the first direct-host `off` run failed because the host test double did not mirror the shell contract; the test double was corrected and the suite has passed since. Commit: `a7686cb`.

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
- Status: T1–T4 complete; feature work-unit commits are 539aca8, e75c7a3, 3047d99, a7686cb; candidate native-reviewed and acknowledged; PR/delivery remains user-owned.

## Evidence

- RED: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveLedgerMode'` → expected build failure: `undefined: ResolveLedgerMode`.
- GREEN: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveLedgerMode|TestLedger.*Mode|TestLedger'` → PASS; `gofmt -l` and `git diff --check` clean.
- Trust-root verification (T4): go1.24.13 build; `bash scripts/verify-gate-sums.sh <temp-generated-sums> catalog/recipes/worktree-flow/bin/SHA256SUMS` → PASS with 4 digest entries.
- Focused Go validation (T4): `go -C catalog/recipes/worktree-flow/gate test -count=1 ./...` → PASS for both packages; `go vet ./...` → PASS; `gofmt -l` → clean.
- Full validation (T4): `./tests/validate.sh` → PASS with 2190 tests and 2 opt-in Jinna release-smoke skips; parity suites ran with no binary-absence skips.
- Diff hygiene (T4): `git diff --check` → clean.
- T2 review: independent read-only verifier PASS; shell/Python remained untouched.
- T3 review: independent read-only verifier PASS; focused suite `Ran 105 tests ... OK (skipped=4)`.
- Native review: lineage `review-a83d97a0d458fc64`, target `sha256:b6738deef16fedb472a9676451356a9d539e36e47dbd0015ad1863730c6187a7`, 3 changed paths / 35 lines, all four lenses submitted; review approved and acknowledgement burned successfully. No correction required.
- Advisory findings were non-blocking/informational: trust-root unverifiable in reviewer context and test-stub contract/off-path suggestions; independent trust-root/full-suite evidence already recorded above.

## Next step

Open the PR when the user authorizes delivery; do not push or merge automatically.
