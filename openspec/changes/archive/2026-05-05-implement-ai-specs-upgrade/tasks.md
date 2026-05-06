## 1. Test Infrastructure

- [ ] 1.1 Add upgrade test fixtures/helpers that create isolated temporary global installs (git repo, VERSION, bin/ai-specs, symlink at ~/.local/bin/ai-specs) and dev installs without touching the real workspace.
- [ ] 1.2 Add RED tests for `ai-specs help` listing `upgrade` and `ai-specs upgrade --help` printing usage with `--dry-run` and `--force` flags.
- [ ] 1.3 Add RED tests for detection of a valid global install: `AI_SPECS_HOME` set, `.git` present, symlink valid, command proceeds to pre-flight.
- [ ] 1.4 Add RED tests for detection of a broken/missing install: missing `AI_SPECS_HOME`, missing `.git`, broken symlink, or symlink outside `~/.ai-specs`; assert explicit error, `install.sh` recommendation, and non-zero exit.
- [ ] 1.5 Add RED tests for dev channel protection: resolved binary outside `~/.ai-specs` aborts with explicit message instructing manual `git pull`, no repo mutation, non-zero exit.
- [ ] 1.6 Add RED tests for dirty working tree blocking upgrade by default, and for `--force` permitting it with a warning.
- [ ] 1.7 Add RED tests for successful fast-forward upgrade: temp repo behind `origin/main`, clean tree, assert fetch + ff-only merge, version diff printed, exit 0.
- [ ] 1.8 Add RED tests for non-fast-forward blockage: local branch diverged, assert `--ff-only` not retried, explicit divergence message, `install.sh` recommendation, non-zero exit.
- [ ] 1.9 Add RED tests for dry-run mode: perform read-only detection and pre-flight, print current and target versions, explicitly state no changes made, do not fetch or merge, exit 0.
- [ ] 1.10 Add RED tests for post-upgrade symlink integrity: valid symlink prints confirmation; broken symlink emits warning and recommends `install.sh`, non-zero exit.
- [ ] 1.11 Add RED tests for already-up-to-date installation: version diff reports "already up to date", exit 0.

## 2. CLI Wiring

- [ ] 2.1 Update `bin/ai-specs` subcommand comments, dispatch case, and help text to include `upgrade [--dry-run] [--force]`.
- [ ] 2.2 Create `lib/upgrade.sh` with existing command style, `--help` handling, argument parsing for `--dry-run` and `--force`, and stub functions for detection, pre-flight, pull, and verification.
- [ ] 2.3 Ensure `tests/validate.sh` includes the new shell file through existing `bash -n lib/*.sh` validation.

## 3. Upgrade Logic Implementation

- [ ] 3.1 Implement install detection in `lib/upgrade.sh`: resolve script real path via symlink-walking loop, verify path is inside `~/.ai-specs`, verify `AI_SPECS_HOME` matches, verify `~/.ai-specs/.git` exists, verify `~/.local/bin/ai-specs` symlink resolves into `~/.ai-specs`.
- [ ] 3.2 Implement dev channel guard: if resolved binary lives outside `~/.ai-specs`, abort with explicit message telling the user to pull manually; do not allow `--force` to bypass.
- [ ] 3.3 Implement broken-install guard: if any detection check fails, abort with explicit error describing the failure and recommending `install.sh`.
- [ ] 3.4 Implement pre-flight checks: verify current branch can fast-forward to `origin/main` (using `git merge-base --is-ancestor` or equivalent), verify working tree is clean via `git status --porcelain`; block on dirty tree unless `--force` is passed (warn when forcing).
- [ ] 3.5 Implement dry-run mode: skip fetch and merge, attempt `git show origin/main:VERSION` for target version preview, print current → target diff, explicitly state no changes were made, exit 0.
- [ ] 3.6 Implement fast-forward pull: `git fetch origin main`, `git merge --ff-only origin/main`; capture and surface git stderr on failure; on non-fast-forward abort with actionable guidance recommending manual resolution or `install.sh`.
- [ ] 3.7 Implement post-upgrade version verification: read `VERSION` before and after pull, print old → new diff, print "already up to date" when versions match.
- [ ] 3.8 Implement post-upgrade symlink integrity check: verify `~/.local/bin/ai-specs` still resolves to `~/.ai-specs/bin/ai-specs`; warn and recommend `install.sh` if broken, exiting non-zero.
- [ ] 3.9 Implement structured error messages and exit codes: 0 on success or dry-run, non-zero on detection failure, pre-flight failure, merge failure, or symlink breakage.

## 4. Documentation

- [ ] 4.1 Rewrite README "Updating the CLI" section to document `ai-specs upgrade` as the day-to-day update path, `install.sh` for first-time installs or recovery, and the `ai-specs-dev` local-dev channel.
- [ ] 4.2 Add `upgrade` row to the README CLI command table with description and flag summary.
- [ ] 4.3 Update README "Safe re-install / upgrade" subsection to explain `--dry-run` and `--force` semantics.

## 5. Verification

- [ ] 5.1 Run focused upgrade tests and record RED/GREEN evidence in `apply-progress.md` during apply.
- [ ] 5.2 Run `./tests/run.sh` and confirm the full unittest suite passes.
- [ ] 5.3 Run `./tests/validate.sh` and confirm py_compile, shell syntax, and unit tests pass.
- [ ] 5.4 Manually test the upgrade command against a temporary clone to verify detection, dry-run, and fast-forward paths produce readable output.
