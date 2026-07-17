# Apply progress: tui-hub

Status: complete (all phases green)

## Completed
- [x] P1.1 util.py + test_util.py
- [x] P1.2 pre-vendor rich+questionary (`lib/_vendor/`, `scripts/vendor-deps.sh`)
- [x] P1.3 init_tui.py delegates to util (patchable wrappers preserved)
- [x] P2.1 hub.py core + test_hub.py
- [x] P2.2 lib/hub.sh (+ bash uninit/no-TTY guard)
- [x] P2.3 bin/ai-specs bare → hub
- [x] P3.1–P3.5 CommandMenu, DelegateRunner, StatusPanel, offer-init, PTY E2E
- [x] P4.1 README hub section
- [x] P4.2 CHANGELOG + `./tests/run.sh` + `./tests/validate.sh` green

## Collateral fixes required for green suite
- Restored missing `git merge --ff-only origin/main` in `lib/upgrade.sh` (regressed when TUI deps block was added).
- Fake-install helper in `test_upgrade_mode_dirt.py` now includes `.gitignore` with `lib/_vendor`.
- Bumped stale `git-pr-flow` pin `1.2.1` → `1.2.2` in `test_sync_pipeline.py`.

## Verification
- `./tests/run.sh` → 829 OK
- `./tests/validate.sh` → OK
