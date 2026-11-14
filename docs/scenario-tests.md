# Scenario tests

A scenario is a plain bash script under `tests/scenarios/` that exercises a real
end-to-end path. It is hermetic (no network, no writes outside the `mktemp`
scratch dir) and backend-agnostic: a local runner, CI, or a remote Space runs
the same file.

`upgrade_from_0_24.sh` installs the real `v0.24.0` release from a local bare
clone, promotes this tree's committed content to `main`, runs the installed old
`ai-specs upgrade`, then asserts the first verb afterwards self-heals
`dist/ai-specs` (#534), sync still works, and `doctor` is clean except the
documented tracker-ledger fail-open.

## Run it
```bash
bash tests/scenarios/upgrade_from_0_24.sh               # direct: PASS/FAIL per check
cd tests && python3 -m unittest test_upgrade_scenario  # thin runner
```

`tests/run.sh` (hence `validate.sh`) finds the runner via `unittest discover`;
it skips on rc 77 and on `AI_SPECS_SKIP_SCENARIOS=1`.

## Env parameters
| Variable | Default | Meaning |
| --- | --- | --- |
| `FROM_TAG` | `v0.24.0` | old release installed by its own `install.sh` |
| `REPO` | this script's repo root | repository cloned into the scratch dir |
| `TO_REF` | `HEAD` | ref whose committed tree is promoted to `origin/main` |
| `KEEP` | `0` | `1` keeps the scratch dir instead of deleting it |
| `BREAK_SELF_HEAL` | `0` | `1` pins `AI_SPECS_GO_BIN` to a missing file in (d), which must fail — proof this scenario catches #534 |

## Exit codes
- `0` passed. `1` an assertion failed; the `FAIL` line names the check and prints
  log tails. `77` skipped: a prerequisite is missing (git/curl/go/tar/python3.11+,
  or the tag is absent because the clone is shallow) — one line on stderr says why.

## Space backend (not implemented yet)
Copy the repository into a Space (Linux container, or a macOS VM whose `HOME` is
symlinked), install git/curl/go/python3, then run `bash
tests/scenarios/upgrade_from_0_24.sh`. A shallow clone exits 77 and is skipped.
