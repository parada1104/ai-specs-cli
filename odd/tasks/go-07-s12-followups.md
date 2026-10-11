# Go 07 S12 follow-ups — advisories, tracker gap and repo debt

## Purpose
Durable register of the non-blocking findings and environmental gaps left behind by the S12
slices (S12.1 copy, S12.2 templates and docs, S12.3 gate hooks). None of these items reopens
or invalidates an approved review: every review that produced them is approved, its receipt
stands, and the findings are separate later work.

## 1. Advisory findings from the approved native reviews
| Id | Severity | Location | Source | What it asks for |
|---|---|---|---|---|
| `R3-copydoc-status-unchecked` | WARNING | `internal/sync/templatedocs.go:249-256` | S12.2 B (`review-ce29b16976038921`) | The doc copy path does not check the status it relies on before deciding; confirm the pre-existence policy reads the same status the Python path does. |
| `R3-boundary-assertion-narrow` | SUGGESTION | `internal/sync/templatedocs_test.go:372-380` | S12.2 B | The boundary assertion covers only one side of the condition; widen it so both directions are pinned. |
| `R3-nil-seam-guard-untested` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/hookactuator.go:222` | S12.3 A (`review-354feed30fce771e`) | The new nil refresh-seam guard has no test; it cannot fire on an existing path today, but it should be pinned if it stays. |
| `R3-clihome-config-mapping-unproved` | WARNING | `internal/sync/hookactuator.go:75` | S12.3 B (`review-c1534147f00b9cb0`) | The `cli_home` to config mapping in the request built by the native authority is not proved against the Python caller; confirm which field the Python dispatcher actually passes. |
| `R3-escape-guard-doc-mismatch` | SUGGESTION | `odd/tasks/go-07-s12-hooks.md:38` | S12.3 B | The task document describes the rel-path escape guard as differential while it is asserted natively only; correct the wording. |
| `R3-partial-backup-repair-untested` | SUGGESTION | `internal/sync/hookactuator_test.go:222` | S12.3 B | The partial-backup repair branch (`hookBackupComplete`) is not exercised; add a scenario with a truncated backup. |

Disposition for all six: informational, non-blocking, later work. None opened a correction and
none reopens its review.

## 2. Tracker gap (availability, not a missing link)
- S12.2 and S12.3 were implemented while the Trello API key was missing or expired, so no card
  was created or linked and no `tracker.none` was written (the host never writes it; it is
  presentation and evidence only).
- The in-repo grader ran at each slice base and reported
  `tracker-card-gate: tracker-ledger: no verified gate binary resolved; failing open (run ai-specs sync / ai-specs doctor)`
  with exit 0. The tracker host is advisory by design, and its `identity_unavailable` rule is to
  report and proceed without recording a decision.
- Close-out once the tracker is reachable: create or link the S12.2 and S12.3 cards, record
  `card_id` and `url` in the `## Tracker` sections of `odd/tasks/go-07-s12-template-core.md`,
  `odd/tasks/go-07-s12-templates.md`, `odd/tasks/go-07-s12-hook-core.md` and
  `odd/tasks/go-07-s12-hooks.md`, and close them with a progress comment.

## 3. Native review left open (S12.2 delivery A)
- S12.2 delivery A shipped under the user-authorized fallback with independent verification,
  because its `review-resilience` lens failed six times in the host relay. The lineage
  `review-e84515b5410c08c0` is still open in `reviewing` with uncollected lenses, and its frozen
  candidate view remains at `.git/gentle-ai/candidate-views/339f233d-cb1a-4d2d-a568-c3fccd71839b`.
- Root cause found later: the `review-resilience` subagent profile pinned `nan/glm5.3-flash`, and
  child agents do not load the gentle-pi extension that registers the `nan` provider
  (gentle-shell#1587). Declaring that provider in `~/.gentle-shell/agent/models.json` fixed the
  spawn path, so the lens is expected to work now.
- Decision still open: leave the lineage open, or disable RDD for this clone
  (`gentle-ai review mode disable --scope clone`) so ordinary policy governs delivery. A fresh
  review of the merged commit is also possible and would produce a real approval.

## 4. Repository and environment debt
- `lib/skills-list.sh:224` expands an empty array under `set -u`, which fails on bash 3.2 (the
  macOS default). The suite needs bash >= 4.4 and Python >= 3.11 for `tomllib`, and the
  differential oracle is pinned to Python 3.14.7 by `internal/projectcache`, `internal/lock` and
  `internal/schema`. Nothing in the repository declares those requirements.
- The development environment was rebuilt after an incomplete iCloud backup deleted most tracked
  files in every worktree. They were restored from the index, and the toolchains were reinstalled:
  go1.24.13 and python3.14.7 through mise, bash 5.3.20 and direnv 2.38.1 through Homebrew. The
  mise `go` shim rebuilds the child PATH and strips `mise/installs/*`, so the real Go binary and
  the Python 3.14.7 bin directory must be prefixed for the suite to run.
- Child agents need `~/.gentle-shell/agent/models.json` for the `nan` provider; removing that file
  breaks every subagent profile pinned to `nan/*` again.

## 5. Where the S12 evidence lives
- S12.2 A: `odd/tasks/go-07-s12-template-core.md`, logs `~/.cache/s12-A-validate6.{log,exit}`.
- S12.2 B: `odd/tasks/go-07-s12-templates.md`, logs `~/.cache/s12-B-validate.{log,exit}`.
- S12.3 A: `odd/tasks/go-07-s12-hook-core.md`, logs `~/.cache/s12-hookA-validate.{log,exit}`.
- S12.3 B: `odd/tasks/go-07-s12-hooks.md`, logs `~/.cache/s12-hookB-validate.{log,exit}`.
- Merged commits: `ffdbbb6` (S12.2 A, PR #336), `8efcc7a` (S12.2 B, PR #337), `a183ec9`
  (S12.3 A, PR #338), `50d680b` (S12.3 B, PR #339).
