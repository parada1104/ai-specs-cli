# ODD Feature: go-12-upgrade

## Objective

Port `upgrade`, `version` and `changelog` to the Go single binary: version
resolution/comparison, `--dry-run`/`--force`, dirty-installation detection and
refusal, upgrade notices + changelog rendering, and a NEW verified
download → SHA-256 check → atomic self-replacement of the running binary.

Card: https://trello.com/c/bSCz7c2r — `[Go 12] Port upgrade, version and changelog`.

## Scope and constraints

- Branch `change/go-12-upgrade`, cut from `epic/go-single-binary` tip
  `bdf0fe87cff810faac068d4e02aad0080e86e68f`. PR target is the epic only.
- Zero third-party Go deps, CGO_ENABLED=0, go1.24.13. Module `ai-specs.dev/ai-specs`.
- Byte-identical port discipline for `lib/_internal/changelog.py`,
  `lib/_internal/cli_version.py` and the observable surfaces of
  `lib/upgrade.sh`; differential tests against the Python/Bash reference.
- The self-replacement engine is NEW Go design (no legacy analogue): local
  `httptest` server + temp dirs only. Tests NEVER touch the real installed
  binary or the network.
- Reuse the gate's proven acquisition discipline
  (`lib/_internal/gate_binary.py`): digest before execution, download to a temp
  file in the destination dir, verify, chmod 0755, atomic `os.replace`; a
  mismatched artifact is deleted/quarantined and never executed.
- Do NOT rewire the dispatcher's legacy `upgrade` shim. The native `version`
  verb already exists. Wiring waits for parity/black-box evidence (remaining).
- Out of scope: release artifact building/publishing (Go 15), first-time
  install (Go 16). This card defines the interface card 15 will feed.
- The legacy path keeps working during the strangler phase.

## Design

### Where the pieces live

| Surface | Legacy | Go target |
|---|---|---|
| Changelog parse/render | `lib/_internal/changelog.py` | `internal/changelog` |
| Version resolution/compare/policy | `lib/_internal/cli_version.py` | `internal/cliversion` |
| `version` verb | `lib/version.sh` | `internal/cli/version.go` (already native) |
| `upgrade` flags/dirty/dry-run/notices | `lib/upgrade.sh` | `internal/upgrade` |
| Verified self-replacement | none (new) | `internal/upgrade` (acquire) |

### Release / trust-root interface (coordinated with Go 15)

- Asset name: `ai-specs-<goos>-<goarch>` (mirrors `worktree-gate-<goos>-<goarch>`).
- Trust root: a committed `SHA256SUMS`-format file, default
  `<home>/bin/SHA256SUMS`, parsed exactly like `gate_binary.load_expected_digests`
  (`<64-hex>  <name>`; comments/blank ignored). The path is injectable so Go 15
  can finalize the layout without changing this engine.
- Download URL: `https://github.com/<owner>/<repo>/releases/download/v<version>/<asset>`
  (same shape as the gate). Injectable in tests (httptest).

### Atomic self-replacement design

1. Resolve the running binary path (`os.Executable` with symlink walk, matching
   `resolve_binary` in `upgrade.sh`).
2. Download the target asset to a temp file **in the destination directory**
   (same filesystem, so rename is atomic; a partial download can never land).
3. Compute SHA-256 of the temp file and compare against the trust root BEFORE
   any replacement or execution. Mismatch → delete the temp file (or quarantine
   when a prior candidate exists), record it, never execute.
4. `chmod 0755` the verified temp file.
5. Replace atomically:
   - Unix (darwin/linux): single `rename(tmp, target)`; the running process keeps
     its old inode, the path now serves the new bytes.
   - Windows: a running executable cannot be overwritten; `rename(target, target.old)`
     then `rename(tmp, target)`, restoring the original on failure. The `.old`
     file is left for the next run to clean (a running exe cannot be deleted).
6. Any failure before step 5 leaves the target untouched; any failure during
   step 5 restores the original bytes. A failed or interrupted upgrade therefore
   always leaves a working binary in place.

### Dry-run / force / dirty-install (ported)

- Flags parsed first, before any install detection (matches `upgrade.sh`):
  `--dry-run`, `--force`, `-v|--verbose`, `-h|--help`; unknown arg → usage on
  stderr + exit 2.
- Install detection ported from `upgrade.sh`: `AI_SPECS_HOME` set, resolved
  binary inside `~/.ai-specs`, `AI_SPECS_HOME == ~/.ai-specs`, `~/.ai-specs/.git`
  exists, `~/.local/bin/ai-specs` is a symlink resolving inside `~/.ai-specs`.
- Dirty tree: `git status --porcelain`; mode-only dirt (clean under
  `core.fileMode=false`) is auto-remediated, real content dirt refuses with
  exit 3 unless `--force`.
- **Deliberate deviation from D9** (recorded defect): the mode-only-dirt
  remediation is checked only on the real-upgrade path, so `--dry-run` writes
  nothing (card acceptance). D9 is in the Defects section, not FROZEN, so this
  is allowed; recorded here and reported as an advisory.
- Notices: `print_release_report` replayed through `internal/changelog`
  (summary first, "Action required" notices second).

### Migration path for old installs (card 16 owns execution)

A legacy git-checkout install is detected by the presence of `<home>/.git`.
This engine treats it as the legacy kind (git-based flow) and does not attempt
self-replacement; card 16 replaces the checkout with the binary and then the
binary install kind takes over. The design keeps both kinds behind one
`Upgrade` entry point so card 16 only flips detection/installation, not the
safety engine.

## Tasks

- [x] **WU1 — `internal/changelog` byte-identical port.** `ParseSections`,
  `SelectRange`, `CrossedVersions`, `UpgradeNotice`, `SummaryBullets`,
  `RemainingCount`, `CrossedNotices`, emit functions, CLI shim. Differential
  test vs `python3 lib/_internal/changelog.py` over the repo CHANGELOG and
  synthetic corpora; focused unit tests mirroring `tests/test_changelog.py`.
  Evidence: `go test ./internal/changelog/ -count=1` exit 0; differential CLI
  compared 252 invocations (14 fixtures × 9 version pairs × 2 modes) byte-for-byte
  against the Python reference; `gofmt -l` and `go vet` clean. Commit
  `32e8aab905ab4a790be28615369e5277ca239349` (969 changed lines).
  Native review lineage `review-c88527d9eb75ac35`: base tree `b45b2f8` (epic tip
  `bdf0fe8`), candidate tree `fd76c21`, 2 paths, tier high. 4/4 lenses admitted;
  closed `approved`; exact acknowledgement burned (`authority: burned`,
  `consumed_revision sha256:92262f30…`). 10 informational advisories (see the
  report); no correction required.
- [ ] **WU2 — `internal/cliversion` byte-identical port.** `ReadInstalledVersion`,
  `ParseVersionTuple`, `CompareVersions`, `ParseToolPolicy`, `CheckPolicy`,
  `ReadLockMeta`, `EvaluateCLIVersion`, `check-sync` CLI. Differential driver
  under `testdata/` + corpus; focused unit tests.
- [ ] **WU3 — upgrade flags + dirty detection + dry-run planning.** Port flag
  parsing, usage text, exit codes, install detection, dirty/mode-only-dirt
  remediation, dry-run report. No network, no replacement. Tests with temp
  git installs.
- [ ] **WU4 — verified acquisition + atomic self-replacement + failure tests.**
  Trust-root parsing, platform detection, httptest download, digest verify
  before replace, atomic rename, Unix + Windows branches, tamper rejection,
  interrupted-download and failed-rename recovery. Tests never touch the real
  binary or the network.
- [ ] **Close:** `gofmt -l .`, `go vet ./...`, `go test ./...`, then one
  full `./tests/run.sh` with a 3600s timeout; record real exit codes; leave
  dispatcher wiring as remaining.

## Evidence

(recorded as each work unit closes: commit sha, RED/GREEN exit codes, review lineage)

## Findings

- The smoke test already compares `upgrade --bogus` (Go shim == legacy bash);
  legacy exits **2** for unknown args, though the parity contract §2/D30 records
  exit 1 — the contract entry is stale against the current source. FROZEN
  behavior = the checked-out legacy = exit 2.
- `lib/version.sh` is already ported (native `version` verb) and degrades a
  missing VERSION to `unknown` with exit 0.

### Native-review incident (2026-09-29): sticky wide consent vs a narrow candidate

WU1's first `gentle_review` inspect returned the **wide** accumulated candidate
(base `e773514`, ~300 paths) instead of the requested base `bdf0fe8`, and
`start` returned `candidate-target-projection-drift` (`lineage_created:false`).
The facade then treated every subsequent `start` as an answer to an expired
wide consent, re-minting a fresh wide binding each time, so no narrow lineage
could form. Root cause: a pending wide consent envelope shadows later STARTs.
The session was blocked until the provider-issued native START command (from
inspect's `next_transition.execute`, `--consent=relay`) was run directly; it
produced the exact narrow `consent/v3` envelope, the human answered `granted`,
and the narrow lineage was created.

Second trap: `inspect` with **top-level `untrackedScope:"exclude"`** also
returned the wide candidate, whereas inspect *without* it returned the correct
narrow candidate but blocked on an intended-untracked selection for the
untracked ODD doc. Workaround that removed the round trip: add the untracked
ODD doc path to `$(git rev-parse --git-path info/exclude)` so the eligible
untracked inventory is empty. Then inspect (no `untrackedScope`) + START yields
the narrow candidate directly.

Lens reliability: `review-resilience`, `review-reliability` died with
`stopReason: length` on this 969-line candidate. Human-authorized, one-shot
routing changes (applied as single-key tmp+rename edits with verified diffs,
restored byte-identical unconditionally afterwards) admitted them:
`review-resilience` → `opencode-go/mimo-v2.6-pro high`;
`review-reliability` → `opencode-go/deepseek-v4-pro high` (transport-class
failures on `mimo-v2.6-pro high`). Final routing restored to
`sha256:33cd7e97…` on `~/.pi/gentle-ai/models.json`.

## Recommended follow-ups (from the approved 4/4 review advisories)

All non-blocking/informational: `R2-001` render loop readability; `R2-002`
comment accuracy; `R3-negative-limit-panic` (SummaryBullets with a negative
limit); `R3-python-dependency-test` (differential test skips/fails hard when
python3 is absent); `R3-regex-whitespace-parity` (ASCII `\s` vs Python Unicode);
`R3-semver-overflow` (semver int accumulation); `R3-summary-fence-h3`;
`R4-SilentLoadDegrade` (ReadSections swallows all errors);
`R4-TestDepsHardFail`; `R4-WriteErrorsIgnored` (emit ignores write errors).

## What remains

- Dispatcher wiring of the native `upgrade` verb (needs black-box evidence).
- `cli_version.py stamp-meta` (lock write) — belongs with the lock/sync surface.
- Go 15 release assets + committed `<home>/bin/SHA256SUMS`; Go 16 install
  migration.
