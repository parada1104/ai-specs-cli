# Go 20 / U1b-2b — `install.sh` installs the binary, not a symlink

Unit of the program `odd/tasks/go-20-python-exit.md` (U1, third slice). Base `5b603fb`. Worktree
`.worktrees/go-20-u1b2b-install-flip`, branch `change/go-20-u1b2b-install-flip`.

## Specs
- S1 — Program decisions: A4 (the compiled binary is the entry point, at `$INSTALL_BIN/ai-specs`), B2
  (flip first, delete later), S9 (verified release asset first, local `go build` as the fallback).
- S2 — `install.sh` stops symlinking the launcher and installs the ENTRY as a compiled file:
  download the platform asset and verify it against the **committed** trust root when that is
  possible, otherwise build locally with `go build`; install atomically (temp file in `$INSTALL_BIN`,
  chmod 0755, rename) so a failed install never leaves a broken entry. The checkout is still cloned and
  updated — it stays the data home, the trust anchor and where the still-unported bash verbs live
  (S3 of U1b-2a).
- S3 — Ordering is already satisfied: the engine accepts and classifies the new layout before the
  installer can produce one (U1b-2a), so `ai-specs upgrade` keeps working across the flip.
- S4 — The download path is fail-closed today and that is expected: there is no `ai-specs-<os>-<arch>`
  asset and no committed trust root until U1b-1 lands, so the installer exercises its local-build
  fallback. Nothing may be installed without a matching committed digest.
- S5 — The guard test moves with the contract, not out of the way:
  `tests/test_install_no_lib_chmod.py` asserts "install.sh must still chmod +x bin/ai-specs (the
  entrypoint)". That assertion encodes the OLD entry. Its intent survives — `lib/` must never be
  chmod-ed (mode-only dirt breaks later pulls) and the entry must be executable — but the subject
  changed: the entry is now an installed binary, so the static guard becomes "install.sh must not
  symlink bin/ai-specs as the entry" and the executable bit is asserted where it is observable, in the
  functional test. The existing `ManagedShellFixture` contract note is kept.
- S6 — Test seams, so the whole flow is verifiable offline and no test touches the network:
  `AI_SPECS_RELEASE_BASE` (download base), `AI_SPECS_TRUST_ROOT_FILE` (the committed digests path),
  `AI_SPECS_SKIP_TUI_DEPS` (the still-Python wizard needs `rich`/`questionary`, and pip is not
  available in the test environment). Defaults are the production ones; the seams are documented as
  internal.
- S7 — No deletion here: `bin/ai-specs` (the bash launcher) stays because the parity harness's legacy
  leg still runs it until U1c.

## Tasks
- T1 — S5/S6: the TDD surface — the retargeted static guard plus a functional test that clones a
  fixture repo, serves a fixture asset from a local HTTP server, and asserts the verified bytes land
  as the executable entry; and the fail-closed case (a bad or absent digest installs nothing).
  Route: inline.
- T2 — S2/S4: the flip in `install.sh` (download → verify → atomic install, `go build` fallback).
  Route: inline.
- T3 — Evidence: focused tests, `go test ./cmd/... ./internal/...`, parity, `./tests/validate.sh`.

## Log
- L1 — Session request (verbatim): "quiero que este como si se hubiera hecho con go desde 0".
- L2 — Reconnaissance: `install.sh` is 156 lines and today only clones/updates the checkout, narrows
  it, installs the TUI deps and `ln -sf`s the launcher; the ONLY test that drives it is a static
  contract guard (`tests/test_install_no_lib_chmod.py`), which is why this unit has to build its own
  functional seam instead of extending an existing one.
- L3 — The trust model this unit inherits from U1b-2a: the network supplies only bytes, the checkout
  supplies the expected digest, and no digest means no install. The installer reads the trust root
  from the checkout it just cloned/updated, so the anchor stays in the repository.
- L4 — Transition concern recorded rather than hidden: the TUI-deps step exists only for the
  still-Python wizard (U4 ports it). This unit keeps it in the legacy step, and the seam lets tests
  skip it; when the wizard is Go the step disappears with the rest of the shell layer (U8).

## Progress
- T1 done, RED then GREEN. RED: the retargeted guard failed on the real file (`ln -sf
  "$AI_SPECS_HOME/bin/ai-specs"` still present, `go build` absent). GREEN: 6/6, the retargeted guard
  plus three functional tests.
- The functional seam is the substance of this unit, because nothing drove `install.sh` before beyond a
  static guard: a fixture git repo stands in for the source and CARRIES the committed digest, a local
  HTTP server serves the platform asset, and three seams make the flow offline and deterministic
  (`AI_SPECS_RELEASE_BASE`, `AI_SPECS_TRUST_ROOT_FILE`, `AI_SPECS_SKIP_TUI_DEPS`). The positive test
  asserts the entry exists, is NOT a symlink, its bytes are exactly the served payload and it is
  executable — that is the flip proven end to end, not described. The mismatch test asserts the
  installer exits non-zero and installs NOTHING when the digest disagrees (its fixture has no `cmd/`,
  so the build fallback cannot rescue it either). A third test asserts no temp file is left in the
  install directory.
- T2 done: `install.sh`'s `[3/3]` installs the compiled binary — verified release asset first, local
  `go build` as the fallback, `chmod 0755` and rename so the install is atomic and a failure never
  leaves a broken or unverified entry. The `chmod +x "$AI_SPECS_HOME/bin/ai-specs"` line is gone: the
  entry is the installed binary now, and chmod-ing a tracked 100644 file is the very mode-only-dirt
  source the existing guard protects against.
- T3 done: see Evidence.
- Second evidence cycle: the FIRST chain failed on two tests of
  `tests/test_upgrade_mode_dirt.py` (`InstallModeOnlyDirtTests`), which also drive `install.sh` and whose
  fixture has no Go module and no release asset — so the new `[3/3]` step exited 1 after the git work
  that class is actually about. A third test in the same class (`content dirt still blocks`) was
  PASSING for the wrong reason: it expects a non-zero exit, and it got one from the entry step rather
  than from the dirt gate. The fixture was retargeted to look like a real installation — a dummy
  release asset OUTSIDE the checkout, a committed `bin/SHA256SUMS`, and the `file://` release base
  plus the TUI-deps skip — and the whole file is green now (9/9), with every assertion failing for its
  own reason again.

## Evidence
Run from this worktree at base `5b603fb`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `bash -n install.sh` | syntax ok | 0 |
| `python3 -m unittest tests.test_install_no_lib_chmod -v` | RED before the flip (the entrypoint guard still found the legacy symlink), `ok` after | 0 (GREEN) |
| `python3 -m unittest tests.test_install_binary_entry -v` | `Ran 3 tests` → `OK` (verified asset installed as the entry; digest mismatch installs nothing; no leftovers) | 0 |
| the FIRST chain | **FAILED** on two `InstallModeOnlyDirtTests` in `tests/test_upgrade_mode_dirt.py`, which also drive `install.sh` from the Python tree and whose fixture had no Go module and no asset; a third test in that class was passing for the wrong reason (non-zero exit from the entry step, not from the dirt gate) | 1 |
| `python3 -m unittest tests.test_upgrade_mode_dirt -v` after retargeting the fixture | `Ran 9 tests` → `OK`, every assertion failing for its own reason again | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; both modes PASS | 0 |
| `./tests/validate.sh` | `Ran 2490 tests in 1628.252s` → `OK` | 0 |

Log: `~/.cache/u1b2b-evidence2.log` (the passing chain) and `~/.cache/u1b2b-evidence.log` (the
first, failing one, kept because it is the evidence for L7). The Python suite moves 2487 → **2490**:
the three functional tests are the only addition.

## Log (continued)
- L5 — The download path is fail-closed TODAY by design and the unit says so instead of pretending the
  release channel exists: there is no `ai-specs-<os>-<arch>` asset and no committed trust root until
  U1b-1, so a real install takes the `go build` fallback. What is proven here is the ORDER (verify
  before installing, nothing installed on a mismatch) and the atomicity, both with fixtures.
- L6 — The retargeted guard keeps its intent: `lib/` is never chmod-ed (the mode-only-dirt lesson) and
  the entry must be executable — the latter asserted where it is observable (the functional test)
  instead of by grepping for a `chmod` line whose subject no longer exists.
- L7 — The U1a lesson was not sharp enough, and this unit paid for it: "run the WHOLE package before
  quoting evidence" covers a Go package, but `install.sh` is driven from the PYTHON tree by tests that
  nothing in the Go package mentions. Sharpened rule for the rest of the program: before running the
  evidence chain, `grep -rn <the file you changed> tests/ internal/` and run EVERY test that
  references it — the failures above were two grep-able references, not a subtle interaction.
- L8 — A passing test is not automatically a truthful one: `content dirt still blocks` asserted only
  "non-zero exit", which the broken entry step satisfied for the wrong reason. When a change adds a new
  failure mode to a command, every test that asserts "this fails" needs a re-read to check it still
  fails for its own reason.

## Native review (high tier, four lenses)
- Lineage `review-ab188aa2eb895f53`, target `sha256:48b74d24bb19bdeed076d7bd06bfa688e27f573091002aa8678612c86bde2bab`,
  candidate `5b603fb487040030b425b83bb8ebd9aa36538821..b9ccf0c` (committed-only), 5 paths / 466
  changed lines, correction budget 200. Risk `high` (`process_boundary` **and** `shell_source` on
  `install.sh`). Consent was resolved by the host.
- Group capture over `pi_host_relay`: 4/4 reviewers submitted, closure `approved` on the last admitted
  event (no correction offered), authority burned (`gentle-ai.review-acknowledged/v1`).
- 9 advisory findings, all informational and non-blocking: `R2-1-misleading-digest-warning`
  (`install.sh:199-210`), `R2-2-removed-chmod-missing-invariant` (`:122-125`),
  `R2-3-inconsistent-platform-helpers` (`tests/test_upgrade_mode_dirt.py:24-32`),
  `R2-4-silent-download-path-fallthrough` (`install.sh:199`); `R3-curl-no-timeout` (`:201`),
  `R3-mismatch-test-reason` (`tests/test_install_binary_entry.py:153-155`),
  `R3-no-cleanup-trap` (`install.sh:192-193`), `R3-sums-binary-marker` (`:200`);
  `R4-temp-entry-no-cleanup-trap` (`:193`).

## Independent verification
The high-risk plan requires a separate verifier; the delegated `gentle-ai-verify` subagent is
reported **unavailable** (the same 30-minute bash stall as before, not retried). The obligation was
discharged inline by a method different from the unit's tests — aimed at the ADVISORIES themselves, to
confirm or refute them rather than re-run the suite:

| Check | Result |
| --- | --- |
| Does the digest check really run? (`R3-mismatch-test-reason`) | The suspicion is **refuted as a defect and confirmed as a missing assertion**: with a correct committed digest and tampered served bytes the installer exits 1, installs nothing, and its stderr carries `did not match the committed digest`. The code reaches verification — but the unit's test only asserts "non-zero exit + no entry", so it could pass through another failure path. Follow-up: assert the warning. |
| A binary-mode trust root (`<digest> *<asset>`, as `sha256sum -b` writes it) (`R3-sums-binary-marker`) | **Confirmed as a real gap**: `awk '$2 == asset'` never matches, so the download is skipped in silence and the installer falls back to building. Follow-up: accept the `*` marker. |
| `curl` timeout (`R3-curl-no-timeout`) | **Confirmed**: neither `--max-time` nor `--connect-timeout` appears anywhere, so a mirror that accepts and never answers hangs the install. |
| Cleanup trap for the temp entry (`R3-no-cleanup-trap`, `R4-temp-entry-no-cleanup-trap`) | **Confirmed — and the first check of it was WRONG**: `"trap " in install.sh` reported True because "Boos**trap** a project: " contains that substring; the line grep matched is the script's final `echo`. `install.sh` has no trap, so an interrupted install leaves `.ai-specs.install.XXXXXX` behind. |

## Follow-ups (recorded, not done here)
- The four confirmed gaps above: assert the mismatch warning in the functional test; accept the
  binary-mode `*` marker in the digest lookup; give `curl` a connect/max timeout; and remove the temp
  entry on interrupt (the success path is already covered by the no-leftovers test).
- The remaining advisories are informational: the digest-warning wording, the comment carrying the
  removed-chmod invariant, the two different platform helpers (the test file's and the script's), the
  silent fall-through when the download path is skipped, and the install-time message for a skipped
  download.
- U1b-1 (the release channel) is what turns the download path from fail-closed into the default, and
  it is also where the `*` marker question becomes practical: the workflow that emits the digests
  decides their format.
