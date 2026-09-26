# Tracker ledger provider-backed advisory lifecycle

## Objective
Make the generic Tracker ledger represent the real provider-backed lifecycle while keeping Tracker recipes advisory and non-blocking for source changes.

## Product decisions
- `## Tracker` remains artifact sugar only; it never opens or links a ledger item.
- One ledger item represents a change/session identity (`common_dir + branch + change`), not each PR in a split delivery.
- `always` requires a provider item, an explicit local bind, and a fresh remote observation before reporting lifecycle compliance; it does not block source edits directly.
- `ask` asks once at cycle start whether to create/link the provider item; a decline is remembered for the change lifecycle and suppresses later prompts unless explicitly requested.
- `warn` reports only.
- `ledger_mode` is the canonical Tracker policy. Trello `gate_mode` is deprecated/removed; Worktree `gate_mode` remains a separate destination policy.
- The initial `tracker` capability is singleton. Multi-tracker witness/routing semantics are a later change.

## Scope
- Close the card-linking → local bind → remote observation seam.
- Reject local-only Tracker success in the advisory contract.
- Normalize mode/help/docs and remove dead Tracker blocking semantics.
- Move repository topology ownership to the CLI project layer with a compatibility fallback from worktree-flow.

## Non-goals
- OpenProject tracker adapter implementation.
- Multi-tracker witness/ledger semantics.
- Changing worktree-flow `gate_mode` behavior.
- Provider writes from the Go ledger or shell hooks.

## Tracker

- **card_id**: `6aadf3cb030174f40aa532b6`
- **url**: https://trello.com/c/uOj0htNV/138-feature-tracker-ledger-provider-backed-advisory-lifecycle
- **list**: In Progress

## Tasks

- [x] T1 — Define the advisory provider-backed state machine and mode normalization with RED/GREEN contract tests.
- [x] T2 — Wire explicit bind and fresh remote observation after provider item creation/linking; preserve provider-neutral core boundaries.
- [x] T3 — Remove Tracker blocking hooks/legacy gate_mode semantics while retaining Worktree gate_mode.
- [x] T4 — Move repo_topology to `[project]` with legacy recipe fallback and stamped compatibility.
- [x] T5 — Run focused/full validation, update evidence, and prepare review.

## Progress and evidence

- T1 implementation slice is partially complete: the pure Go predicate now rejects local-only rows as `needs-item` (warn remains non-blocking but doctor reports WARN), and new opt-outs are lifecycle-scoped while legacy checkpoint-scoped records remain compatible.
- T1 RED/GREEN: focused ledger tests failed before the new Scope/provider-backed fields existed, then the 9 focused cases passed; the package suite and `go vet ./...` pass.
- Expected integration fallout remains in `ledger_cmd_test.go`: four existing CLI tests still encode local-only open success or checkpoint-scoped opt-out. T2/T3 must rebase those contracts while making tracker hosts advisory.

## Progress and evidence

- T1 complete. The pure Go predicate now marks local-only rows as `needs-item` and doctor WARN (warn remains non-blocking), requires both provider item id and provider id for compliance, and new opt-outs are lifecycle-scoped while scope-less legacy opt-outs remain checkpoint-scoped.
- T1 RED/GREEN: the focused 9-case selection failed before Scope/provider-backed fields existed; ledger package tests then passed. CLI integration contracts were rebased from local `open` to provider-backed `bind` where compliance is expected, and branch-level bind/close recovery now preserves an intentional empty stored slug under ambiguous planning trees.
- T1 verification: `go test ./ledger/ -count=1`, `go test ./... -count=1`, `go vet ./...`, `gofmt -l`, and `git diff --check` all pass.
- T2 complete. Trello skill and command now document the provider-card → local `bind` write and the MCP observation → `--reconcile` continuation; `## Tracker` remains artifact sugar and only `agree` counts as provider-backed compliance.
- T2 RED/GREEN: the new bind/observation/ask-path contract tests were added against existing surfaces and the Trello recipe suite passed 25 tests; ledger parity/witness/host/mode suites passed 70 tests with 7 existing skips; diff check passed.
- T3 complete. Tracker hook and direct host verdicts are advisory/non-blocking; Trello hook metadata is `blocking = false`; `ledger_mode` is canonical and Trello `gate_mode` is documented as deprecated compatibility. Worktree and Plan Build gate semantics remain unchanged; merge-host docs reflect advisory Tracker behavior.
- T3 RED/GREEN: Tracker/Ledger/Trello suite passed 90 tests; Plan Build/Worktree suite passed 175 tests with 90 existing Go-binary skips; diff check passed.
- T4 complete. `[project].repo_topology` is the CLI-owned source; the legacy worktree-flow recipe key remains a fallback with deprecation metadata; planning, stamping, brief, doctor, hub, configure, TUI, and sync consumers use the shared accessor while stamped gate tokens remain compatible.
- T4 verification: topology/config/materialization/TUI/doctor/hub/target-resolve/manifest suites passed 425 tests; `git diff --check` passed.
- T5 complete. The Go trust root was regenerated with canonical go1.24.13; parity fixtures and design-row needles were updated for provider-backed local-row/lifecycle-opt-out semantics. Final `./tests/validate.sh` passed with exit 0; Go tests, vet, gofmt, sums verification, and diff check passed.
- Final T5 scope: `catalog/recipes/worktree-flow/bin/SHA256SUMS`, tracker-ledger parity corpus 02/03/04/09/10/21/22/25/26/29/31/32, and `tests/test_tracker_ledger_parity.py`.

## Acceptance criteria

- A Tracker item cannot be compliant through a local-only `open` row.
- `always`/`ask`/`warn` expose advisory actions and provider-backed state without blocking source edits.
- A declined `ask` decision is change-scoped and does not re-prompt on every checkpoint.
- Trello card creation/linking has a generic local bind and remote-observation continuation.
- `gate_mode` remains meaningful only for Worktree destination policy; Tracker uses `ledger_mode`.
- Topology is CLI/project-owned and worktree-flow consumes one resolved value.
- All focused and full checks pass; the current Jinna product branch remains independent.

## Delivery

- Worktree: `.worktrees/tracker-ledger-advisory`
- Branch: `feat/tracker-ledger-advisory`
- Test runner: `./tests/validate.sh`
- Strategy: reviewable work-unit commits; no provider writes from ledger core.
