# Go 07 S12.2 — Shared template actuator delivery

## Objective
Expose the existing gate template actuator through an importable shared package.
Preserve the gate JSON command contract.
This is delivery A of S12.2. The root authority follows in delivery B.

## Tracker
- **card_id**: `6ac8675b182833f019d256b1`
- **url**: https://trello.com/c/APUQZNIf
- **list**: In Progress
- This card covers both S12.2 deliveries. Close it only after both integrate.

## Scope
- Base: `8b16d1128a9e0df0ea8499beec20a8cd856282fe`, epic/go-single-binary.
- Worktree: `.worktrees/go-07-s12-template-core`.
- Branch: `change/go-07-s12-template-core`.
- Copy only the two extracted gate files from the preserved S12.2 implementation.
- Do not copy the root authority or its tests into this delivery.
- Regenerate only the four canonical gate checksum entries required by the extraction.

## Constraints
- Budget: 1200 additions plus deletions, including both sides of moved code.
- Expected code: approximately 1042 lines. Measure the complete candidate.
- Never remove tests or compress code to meet the budget.
- Keep existing gate tests with the behavior they already verify.
- The extraction is a refactor. Existing behavioral tests provide its checks.
- Do not claim new RED evidence for the extraction.
- No production Python changes, CLI wiring, lock writes, or dependency changes.
- No development changes or releases.
- Parent controls commit, push, PR, merge, and cleanup after checks.

## Tasks
- [x] T1: Isolate the extraction without changing its existing behavior. **Done**. Delivery A copied both gate files byte-exact. Self-checks passed at 1042 code lines.
- [x] T2: Regenerate gate checksums, verify the isolated candidate, and rerun full validation. **Done**; full validation exit 0 with durable logs.
- [ ] T3: Commit the work unit and complete applicable native review. **Commit done as `75231bb`; the native review could not complete: the `review-resilience` slot failed five times in the host relay with no artifact. The human decided to close delivery A with the independent verification instead.**
- [ ] T4: Publish and merge into the epic. Preserve delivery B and close this worktree safely.

## Acceptance criteria
- Gate stdin/stdout envelope fields and JSON order remain unchanged.
- Refusals keep stdout error envelopes and exit 2.
- I/O errors keep stderr diagnostics and exit 2.
- File modes and symlink/containment guards retain existing behavior.
- Existing gate tests pass against the shared implementation.
- Root build and full validation pass without delivery B files.
- The complete authored candidate stays within 1200 lines.

## Checks
- Root: `go test ./cmd/... ./internal/...`.
- Gate: `go test ./... -count=1` from the gate module.
- Formatting and `git diff --check`.
- Full: `./tests/validate.sh`, with durable logs and exit evidence.
- Native review uses the exact committed slice base, not the whole epic.

## Progress
- Parent verified a clean worktree at the stated base and branch.
- The original S12.2 worktree remains untouched with all implementation and tests.
- Previous verifier timed out without a usable verdict. No result was accepted.
- Delivery A copied `gate/templateactuator.go` and `gate/shared/templateactuator.go` byte-exact.
- Self-checks passed: 1042 code lines, root tests, gate tests, gofmt, and `git diff --check`.
- Parent measured 1106 changed lines: 546 in the gate wrapper, 496 in shared code, and 64 in this document.
- Full-file counts are not the delivery metric. Both sides of relocated code remain counted.
- No new RED cycle applies to this behavior-preserving extraction; existing tests passed.
- Native ASSESS measured 1110 changed lines after evidence updates and classified process-boundary risk as high.
- Verifier mv0h9cqy-6-w724 returned PARTIAL: source comparison PASS, no semantic gate delta; root/gate tests and gate vet pass.
- Full validation ran once and exited 1: Python 2377 tests, 19 failures, 1 error, 164 skips.
- Parity passed: 29 fixtures in each gate mode, zero deltas. This does not override the failed Python phase.
- Durable artifacts: /Users/robert/.cache/s12-verify-validate.log, .exit, and .pid. Recorded processes exited.
- Raw candidate: 1112 changed lines; source and staged hashes stayed unchanged during validation.
- Failure clusters include gate acquisition, release checksums, topology, cwd, verbosity, ledger witness, and stamps.
- Explorer mv0j5tin-a-4nu3 found a built binary rejected for mismatch with the committed SHA256SUMS trust root; quarantine caused apparent absence.
- No packaging file was omitted. A new shared package can change compiled bytes without changing behavior.
- Parent prepared a clean detached base at .worktrees/go-07-s12-baseline, exact commit 8b16d11.
- Verifier mv0jylkp-c-woug observed base PASS and A FAIL for matrix digests, cache acquisition, and isolated release materialization.
- Actual toolchain was go1.24.13 in both trees. Base digests equal committed entries; all four A digests differ.
- Candidate-caused stale trust metadata is confirmed. Regenerate only the four SHA256SUMS entries with scripts/build-gate.sh and go1.24.13.
- Metadata correction applied: exactly four digest lines replaced in SHA256SUMS; comments, header, order, and names unchanged.
- Rebuilt all four matrix assets from this root with GOTOOLCHAIN=go1.24.13 (actual go version go1.24.13) and hashed the real outputs; no placeholder values.
- RED: the three discriminating tests already failed on candidate A before this metadata edit (verifier mv0jylkp-c-woug, base PASS / A FAIL). Existing failing tests are the RED evidence; no new RED cycle was authored.
- GREEN: all three tests now pass locally with canonical Go and matched fixture controls, explicit rc=0 (logs /Users/robert/.cache/s12tpl-t{1,2,3}.log).
- Source gate hashes are unchanged by the metadata edit: templateactuator.go d863f416..., shared/templateactuator.go 1e27c70d....
- Parent measured 1139 changed lines after the metadata evidence update: 643 additions and 496 deletions, within the 1200 budget.
- No trust weakening, VERSION bump, release, or publication. Full validation, native review, commit, and delivery remain pending.
- Environment was rebuilt after an incomplete iCloud backup destroyed most tracked files in every worktree: 2133 paths in the main checkout and 2027-2028 in each linked worktree were restored from the index, and the missing toolchains were reinstalled (go1.24.13 and python3.14.7 via mise, bash 5.3.20 and direnv 2.38.1 via Homebrew).
- The suite is interpreter-pinned: its differential tests need the Python 3.14 oracle (`internal/lock`, `internal/schema`, `internal/projectcache`) and the legacy shell needs bash >= 4.4 for empty-array expansion under `set -u`.
- Checksum truth re-verified: `scripts/build-gate.sh` under go1.24.13 rebuilt the four matrix assets and all four digests MATCH this candidate's `bin/SHA256SUMS` entries; that file's staged diff is exactly four digest lines with header, comments, names and order unchanged.
- Focused checks on the candidate: `gofmt -l` clean at the root and in the gate module; `git diff --check` and `git diff --cached --check` clean; root and gate `go vet ./...` clean; root `go test ./cmd/... ./internal/...` ok for all 17 packages; gate `go test ./... -count=1` ok (worktree-gate 43.8s, ledger, and the new shared package).
- Differential parity harness: 29 fixtures, 0 failing, in both gate modes.
- Full validation passed in a matched environment: `./tests/validate.sh` exit 0, `Ran 2380 tests in 1575.058s` -> `OK (skipped=164)`. Durable evidence: /Users/robert/.cache/s12-A-validate6.log and .exit.
- Earlier runs in the pre-restoration environment failed only on environment defects (bash 3.2 empty-array expansion in `lib/skills-list.sh`, and the absent direnv desynchronizing the init TUI prompt); both reproduced on the pristine base tree at the same commit and disappeared once the tools were reinstalled.
- T3 and T4 remain pending: commit, review, and delivery.
- Work unit committed as `75231bb` — `refactor(gate): extract the template actuator into a shared package` — 4 files, 649 additions and 496 deletions, inside the 1200 budget; working tree clean.
- Native review started on the exact slice base `8b16d11` (lineage `review-e84515b5410c08c0`, tier high, four lenses required, correction budget 200). It reviewed the slice's four paths, not the epic stack.
- `review-risk` was captured and admitted (`rart1_6c0c528d…`, sha256 `6c61da8a…`, admission completed). `review-resilience` failed five times in the host relay: one `terminated` inside the four-lens group and four `reviewer-empty-output` with the reviewer stop reason `length` (430-607s elapsed), every attempt reporting `mutation_performed: false` and zero submissions.
- Lowering the session reasoning level did not change the outcome, and the pinned gentle-pi bundle exposes no relay model or effort knob; the failure is reproducible, not transient. The remaining two lenses were never reached.
- Human decision: stop retrying and close delivery A with the independent verification already recorded (2380 Python tests with exit 0, root and gate Go suites, parity 29 fixtures with zero deltas in both gate modes, and the four rebuilt trust-root digests).
- No acknowledgement was issued and no native input was invented, so this candidate has no reviewed authority; the review switch stays a human decision, and delivery follows ordinary repository policy.
- T4 remains: publish and merge into the epic.

## Rollback
Revert this delivery's gate extraction, its four checksum entries, and its task document together.
Delivery B must not integrate until this shared API exists.
