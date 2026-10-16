# Go 20 / U1b-2a — The upgrade engine knows both install kinds

Unit of the program `odd/tasks/go-20-python-exit.md` (U1, second slice). Base `cd37c44`. Worktree
`.worktrees/go-20-u1b-install-flip`, branch `change/go-20-u1b-install-flip`.

## Specs
- S1 — Program decisions: A4 (the compiled binary is the entry point, installed at
  `$INSTALL_BIN/ai-specs`), B2 (flip first, delete later), S9 (verified release asset first, local
  `go build` as the fallback).
- S2 — Ordering, recorded because inverting it breaks live installations: this slice lands BEFORE
  `install.sh` stops symlinking. Today `upgrade`'s `Detect` requires `~/.local/bin/ai-specs` to be a
  symlink resolving into `~/.ai-specs` and the running executable to live inside it, so a binary
  install would abort with `~/.local/bin/ai-specs is missing or not a symlink`. The engine must accept
  and classify both layouts first; only then may the installer start producing the second one.
- S3 — The trust model, which keeps this slice small: `$AI_SPECS_HOME` REMAINS a git checkout — it is
  the trust anchor (the committed digest file), the data home, and where the still-unported bash verbs
  live. What changes is only the ENTRY: `$INSTALL_BIN/ai-specs` becomes a real compiled file instead
  of a symlink into that checkout. A pure data home with no git is a later step (U8), after every verb
  is Go and `lib/` is gone.
- S4 — Consequences for the engine, both kinds behind one entry point (the design card [Go 12]
  already recorded: "card 16 only flips detection/installation, not the safety engine"):
  - `Detect` classifies the layout: `legacy` (symlink into the home, executable inside it) or
    `binary` (a real executable in `$INSTALL_BIN`, home as data).
  - `Run` keeps the legacy path byte-identical (git fetch + fast-forward + release report) and gains
    the binary path: refresh the checkout with git (trust root + `VERSION`), then acquire the release
    asset, verify it against the committed trust root BEFORE any replacement, and swap atomically
    (`Acquire` + `SwapBinary` — ported and tested, today called by nothing).
  - The frozen exit codes (0..5) are reused; no new code is invented, and the legacy kind's messages
    do not move.
- S5 — Fail closed: no committed digest for the platform's asset means no install; the current binary
  stays untouched and the message says what is missing. That is what makes the download path safe to
  ship before the release channel exists (U1b-1).
- S6 — No deletion and no `install.sh` change in this slice (B2): `lib/upgrade.sh` keeps serving the
  legacy route until U1c's seam work and U2's caller audit.

## Tasks
- T1 — S4: classify the install kind in `Detect` without moving the legacy messages. Route: inline.
- T2 — S4/S5: the binary upgrade path over `Acquire`/`SwapBinary`, with the trust root read from the
  checkout. Route: inline.
- T3 — S2/S4: pins for both kinds, including the fail-closed case and the untouched binary. Route:
  inline.
- T4 — Evidence: focused packages, `go test ./cmd/... ./internal/...`, parity, `./tests/validate.sh`.

## Log
- L1 — Session request (verbatim): "quiero que este como si se hubiera hecho con go desde 0".
- L2 — Reconnaissance that produced this slice: `install.sh` (156 lines) only clones/updates the
  checkout, narrows it, installs the TUI deps and symlinks the launcher; the release workflow
  (`.github/workflows/release-worktree-gate.yml`) builds a goos/goarch matrix, emits and verifies
  SHA256SUMS against the COMMITTED digests and attaches the assets — for the gate only; there is no
  `ai-specs-<os>-<arch>` asset, and `internal/upgrade.Acquire` is called by nothing.
- L3 — Slicing decision: the engine first (this unit), then `install.sh` (U1b-2b), then the release
  channel (U1b-1). The order is forced by S2, and it is also the safest: at every step the legacy
  install keeps working and nothing new is reachable until the channel exists.
- L4 — Model decision recorded rather than assumed: keeping the checkout as the trust anchor is what
  lets the trust root stay committed (the gate's own discipline: "the network supplies only bytes, the
  repository supplies the expected digest") instead of trusting a manifest fetched at upgrade time.

## Progress
- T1 done, RED then GREEN: RED was the compile failure on the new API (`Install.Kind undefined`,
  `KindLegacy`/`KindBinary`/`Env.ReleaseBase` undefined). GREEN adds `InstallKind` with
  `KindLegacy`/`KindBinary`, `Install.Kind`, and `binaryEntry` — "the running binary IS the installed
  entry": a real executable file at `LocalBin`, not a symlink, resolving to the same path this process
  runs from. `Detect` recognises a binary install FIRST, because all five legacy checks assume the
  legacy shape and would refuse the post-flip layout the installer is about to produce; a binary
  install still has to sit at the expected home, with the existing message. Every legacy failure keeps
  its exact message and exit code, which the package's pre-existing tests verify.
- T2 done: `Env.ReleaseBase` is the test/mirror seam for the download base (empty means the
  repository's own releases for the checked-out version), and `binaryUpgrade` does the two moves: a
  git refresh when the checkout is present (reusing the divergence, fetch and fast-forward messages
  and codes) and then the acquisition — trust root from `<home>/bin/SHA256SUMS`, `Acquire`
  (download, verify BEFORE any replacement, atomic swap), then the version report through the
  changelog the legacy path already uses.
- T3 done, and the pin that matters is the fail-closed one: with no committed digest `Run` exits
  non-zero, the message names the platform asset, the current binary is byte-identical afterwards,
  and the test's HTTP handler fails the run if ANY request is made — the digest check happens before
  the network, which is what makes this path safe to ship before U1b-1 exists.
- A first GREEN attempt failed on my own edit: three comment continuation lines of `binaryEntry` were
  written without `//`, so `vet` reported `expected declaration, found executable` at
  `upgrade.go:188`. Fixed, and recorded because it is the same class of mistake the program's rule
  already covers: nothing gets quoted as evidence until the whole package compiles and passes.

## Evidence
Run from this worktree at base `cd37c44`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go test ./internal/upgrade -run 'TestDetectClassifiesBothInstallKinds\|TestBinaryKind' -count=1` | RED before the implementation (undefined `Install.Kind`, `KindLegacy`, `KindBinary`, `Env.ReleaseBase`) | 1 (RED) |
| `go vet ./internal/upgrade` | clean | 0 |
| `go test ./internal/upgrade -count=1` | `ok` — the WHOLE package, legacy `Detect` message tests included | 0 (GREEN) |
| `go test ./cmd/... ./internal/... -count=1` | every package ok | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; both modes PASS | 0 |
| `./tests/validate.sh` | `Ran 2487 tests in 1682.438s` → `OK` | 0 |

Log: `~/.cache/u1b2a-evidence.log` (chained go test → build-gate → parity → validate), focused runs
in `~/.cache/u1b2a-green*.log`. The Python suite count does not move (2487): this slice adds Go tests
only and deletes nothing.

## Log (continued)
- L5 — Classification order is the design decision worth naming: a binary install is recognised before
  the five legacy checks, and `binaryEntry` demands that the entry be the SAME path the process runs
  from. A fixture whose `$INSTALL_BIN/ai-specs` is a real file while the running executable still
  lives inside the checkout therefore stays legacy — and keeps the legacy refusal message — instead of
  being silently reclassified. That is what keeps every existing `Detect` test meaningful.
- L6 — `--dry-run` for the binary kind prints the current version and the asset and writes nothing:
  no fetch, no download, no swap. The shell's dry-run contract ("no changes will be made") is kept
  even though this path has no shell counterpart.
