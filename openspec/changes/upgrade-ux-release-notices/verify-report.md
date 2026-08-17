# Verify report: upgrade-experience

Depth: full. Discipline: red-green-refactor, RED evidence recorded per work unit.

## Suite

| Check | Result |
|---|---|
| `./tests/validate.sh` (change worktree) | **exit 0 — 1772 tests, 0 failures** |
| Same on `development` before the change | 1684 tests |
| `bash -n` — `install.sh`, `lib/upgrade.sh`, `lib/_internal/narrow-checkout.sh` | clean |
| Upgrade-related suites (`test_upgrade*.py`) | 70 tests, all pass |

## RED → GREEN per work unit

| WU | RED evidence | GREEN |
|---|---|---|
| WU1 parser | `FileNotFoundError: lib/_internal/changelog.py`, 0 tests collected | 33 tests pass |
| WU2 output | 5 failures of 11 (`hides_git_output`, `verbose_restores`, `one_line_per_step`, `short_verbose_flag`, `help_documents_verbose`) | 11 pass |
| WU3 summary + notices | 9 failures of 35 | 35 pass |
| WU3b bullets | 8 errors of 28 | 28 pass |
| WU3c truncation | 3 failures of 33 | 33 pass |
| WU4 narrowing | 7 failures of 12 | 12 pass |

## Requirement coverage

| Requirement | Covered by |
|---|---|
| Compact upgrade output by default | `test_successful_upgrade_hides_git_output`, `test_compact_mode_prints_one_line_per_step`, `test_already_up_to_date_is_quiet` |
| Verbose restores full detail | `test_verbose_restores_git_output`, `test_short_verbose_flag_is_accepted` |
| Failing step prints everything, exit code preserved | `test_fetch_failure_dumps_output_and_keeps_exit_code` (asserts exit 4 **and** that git's own error survives) |
| Safety behavior unchanged | `test_dirty_tree_still_blocks_with_exit_3`, `test_dry_run_output_is_unchanged`, plus all 15 pre-existing `test_upgrade.py` tests |
| Version crossing summary | `test_summary_lists_the_crossed_version`, `test_summary_covers_every_crossed_version`, `test_summary_is_ordered_newest_first`, `test_no_summary_when_already_up_to_date` |
| Changelog unreadable degrades | `test_unreadable_changelog_still_upgrades`, `test_malformed_changelog_degrades_to_plain_line`, `test_missing_parser_does_not_break_the_upgrade` |
| Notices replay oldest first | `test_notices_replay_oldest_first`, `test_notice_is_printed_for_a_crossed_version` |
| Notice bounded to its subsection | `test_notice_does_not_bleed_into_the_next_subsection`, `test_notice_stops_at_the_next_subsection` |
| Notices survive compact mode | `test_notice_survives_compact_mode` |
| Notice displayed, never executed | `test_notice_command_is_displayed_not_executed` |
| Excluded subtrees leave the tree | `test_excluded_subtrees_leave_the_working_tree` |
| Runtime paths survive | `test_runtime_subtrees_survive`, `test_root_files_survive` |
| History and ancestry intact | `test_history_and_ancestry_are_intact` |
| Narrowing idempotent | `test_second_run_is_a_noop`, `test_second_run_reports_nothing_to_do` |
| Unsupported git falls back | `test_git_without_sparse_checkout_falls_back`, `test_fallback_warns_rather_than_failing_silently` |
| Failure never blocks | `test_missing_target_is_not_fatal`, `test_non_git_target_is_not_fatal`, `test_dirty_excluded_path_is_left_alone` |

## Manual verification

Both modes rendered against the **real** `CHANGELOG.md` and the real 0.22.0
notice, via an isolated fake install.

Compact, `0.20.0 → 0.22.0`: **~250 lines → 21**, every line carrying
information — three condensed bullets per version, an explicit `and 23 more`,
and the Action required block.

Verbose: the report stays on stdout, raw git detail returns on stderr, exit 0.

Narrowed checkout driven end-to-end — `--version`, `help`, `init`, `sync`,
`doctor` (18 OK / 0 ERROR). See `apply-progress.md`.

## Deliberately not done

| Excluded | Reason |
|---|---|
| Shallow clone | Breaks `git merge-base --is-ancestor`, the divergence guard (design D4) |
| Conditional or executable notices | `upgrade` has no consumer project in scope and would be guessing (design D2) |
| Changes to safety checks or exit codes | Out of scope; the upgrade logic was never the defect |
| Runtime-brief / `AGENTS.md` ownership | Card #81 |

## Defect found while reviewing the limitations (fixed)

Re-reading the "old git fallback is only proven with a shim" note surfaced a
real bug in the capability probe, not just a missing test.

The probe was `git sparse-checkout --help`. That routes through `git help`,
which honors `help.format`. A user with `help.format = web` and a browser
configured would have **a browser launched during install or upgrade** by a
check that is supposed to observe and change nothing.

Reproduced by `test_capability_probe_has_no_side_effects`, which configures
`help.format=web` with a browser command that touches a sentinel file. The
sentinel was created — RED — before the fix.

The probe is now `git sparse-checkout -h`, which prints short usage and never
reaches man or a browser. Exit codes turned out to be unusable for this check
(**129** for a known subcommand, **1** for an unknown one, not comparable
across versions), so the probe matches on the `is not a git command` message
git prints for a missing subcommand. That is also what the old-git shim emits,
so both paths agree.

Two measurement errors were made while chasing this, both worth recording:

1. `timeout 10 git …` returned 127 and looked like a git failure. macOS ships
   no `timeout`; the 127 was the missing wrapper. Re-measured directly:
   `--help` returns 0 in ~167ms.
2. `git … | head -1; echo $?` reports `head`'s exit code, not git's.

## Honest limitations

- **`install.sh` clone path is not covered by an automated test.** The
  two-line `--filter=blob:none` fallback is not exercised by a network clone in
  CI; the helper it then calls is fully tested. Its one dangerous assumption
  *was* verified by hand: a failed `git clone` removes the destination
  directory, so the retry cannot hit "destination path already exists".
  Confirmed for both a bad ref and an unreachable remote.
- **`test_upgrade_notices.py` inherits `UpgradeOutputTests`,** so 11 output
  tests re-run under a notice-bearing changelog — roughly 35s of the 557s
  suite. Kept deliberately: it proves compact mode still holds when a notice is
  present. The cost is real and the coupling is a maintenance smell.
- **Narrowing has not been exercised against a git older than 2.25** on real
  hardware; the fallback is proven with a PATH shim. The probe is now
  message-based rather than exit-code-based, which makes it more robust across
  versions than it was.
