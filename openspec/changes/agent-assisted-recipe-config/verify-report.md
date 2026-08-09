# Verify Report: agent-assisted-recipe-config

## Verdict

**PASS — deterministic implementation validation and both required real-runtime
scenarios passed; the multi-runtime evidence gate is complete.** Both runtime
records are preserved below exactly, including
`helper_report_present: false`; the scenario transcript assertions passed and
that field is not upgraded to `true` by inference.

## Deterministic verification

| Check | Result |
|---|---|
| `python3 -m unittest discover -s tests -p 'test_recipe_config_write.py'` | 13 passed |
| `python3 -m unittest discover -s tests -p 'test_recipe_configure.py'` | 14 passed |
| `python3 -m unittest discover -s tests -p 'test_harness_cli_literacy.py'` | 13 passed |
| `bash tests/evals/run.sh` | 46 passed, 17 skipped |
| `bash tests/run.sh` | passed |
| `bash tests/validate.sh` | passed |
| `git diff --check` | passed |

The assisted-configure live client import smoke also passed with
`EVALS_LIVE=1`; its bounded no-runtime smoke selected no runtime and reported
five skipped tests, `OK`. This is harness/client validation, not live evidence.

## Live evidence

The canonical wrapper was executed from the #62 worktree with one trial, a
16-turn cap, and a 420-second timeout for each runtime. Each run selected only
`ac_apply_sync_verify_report`.

### Claude / Opus

```text
EVALS_LIVE=1 EVALS_RUNTIMES=claude \
EVALS_SCENARIOS=ac_apply_sync_verify_report EVALS_TRIALS=1 \
EVALS_MAX_TURNS=16 EVALS_TIMEOUT_SEC=420 \
./tests/evals/run-live-assisted-configure.sh

Ran 5 tests in 103.673s
OK
EXIT:0
```

```json
{
  "cli_version": "0.21.0",
  "exit": 0,
  "helper_report_present": false,
  "model": "opus",
  "runtime": "claude",
  "scenario": "ac_apply_sync_verify_report",
  "timed_out": false,
  "trial": 1,
  "worktree_sha": "4193277981c8052b5ac132f41685458f6e103131"
}
```

**Interpretation:** PASS. Claude/Opus completed the approved Trello/MCP
configuration scenario; canonical config application and transcript assertions
for sync/verify/report language passed.

### cursor-agent / Composer 2.5

```text
EVALS_LIVE=1 EVALS_RUNTIMES=cursor-agent \
EVALS_SCENARIOS=ac_apply_sync_verify_report EVALS_TRIALS=1 \
EVALS_MAX_TURNS=16 EVALS_TIMEOUT_SEC=420 \
./tests/evals/run-live-assisted-configure.sh

Ran 5 tests in 318.436s
OK
EXIT:0
```

```json
{
  "cli_version": "0.21.0",
  "exit": 0,
  "helper_report_present": false,
  "model": "composer-2.5",
  "runtime": "cursor-agent",
  "scenario": "ac_apply_sync_verify_report",
  "timed_out": false,
  "trial": 1,
  "worktree_sha": "36b68b9b6ab5d56318ef90454425ac5d1dfbc781"
}
```

**Interpretation:** PASS. Cursor-agent/Composer 2.5 completed the approved
Trello/MCP configuration scenario; canonical config application and transcript
assertions for sync/verify/report language passed.

The two SHA values above identify disposable fixture baseline commits, not the
source worktree branch tip.

## Isolation evidence

The Claude run's source-worktree status before and after was identical:
15 modified tracked files and 7 untracked files, all pre-existing #62 branch
artifacts. `git diff -- AGENTS.md` was empty, and the fixture/temp state was
cleaned. The cursor-agent target worktree had clean `git status --short` after
the run; it also produced no source-worktree modifications or publication.
The fixture's temporary project and transient `RECIPE_MCP_TEMP` files were
cleaned by the harness.

## Task disposition

- **7.3 — COMPLETE:** Claude/Opus and cursor-agent/Composer 2.5 both produced
  trustworthy PASS records for `ac_apply_sync_verify_report`.
- **7.4 — COMPLETE:** post-run isolation and cleanup were confirmed.
- **7.5 — COMPLETE:** exact evidence is transcribed per runtime, including
  `helper_report_present: false` and disposable fixture SHAs.

No canonical eval scenario contract, fixture isolation model, assertion rule, or
#59/#60 inherited surface was intentionally changed by the live runs.
