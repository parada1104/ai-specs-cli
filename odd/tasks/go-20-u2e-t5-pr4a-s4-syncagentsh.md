# GO-20 U2e T5 PR4a S4 follow-up — sync-agent.sh fixture freeze

Base: epic/go-single-binary fba37a4. Branch: change/go-20-u2e-t5-pr4a-s4-syncagentsh.
Goal: `internal/sync/syncagent_test.go`'s two source-structure pins stop reading `lib/sync-agent.sh` live
and stop skipping when it is absent, and pin the same oracle from a frozen bundle instead. This is the
last live-`lib/` reader found in the S4 final inventory of the platform.sh slice
(`odd/tasks/go-20-u2e-t5-pr4a-s4-platformsh.md`, L11), and it uses the identical pattern (#511
`testdata/platform_sh_golden.json` + `loadPlatformShGolden`, reusing `readGoldenBundle` /
`validateGoldenIdentity` from `agentsrender_test.go`).

## Specs

- S1: `TestSyncAgentCDLineMatchesShellSource` keeps its name and its assertion — the Go constants
  `syncAgentCDLineTarget` (108) and `syncAgentCDLineSource` (110) are compared against the frozen line
  numbers. The oracle becomes the frozen bundle `internal/sync/testdata/sync_agent_sh_golden.json`; there is
  no skip, so a missing golden fails.
- S2: `TestSyncAgentUsageMatchesShellHeredoc` keeps its name and its byte-exact comparison of
  `syncAgentUsage` against the frozen heredoc body (the retired `body[:end+1]` slice, trailing newline
  included).
- S3: **Decision: freeze exactly the facts the two pins consume, not the whole shell file.** Per cd pin that
  is the needle (the statement text, kept so a failing pin names the statement) and its 1-based line; for the
  usage pin the heredoc body verbatim. Reasons: (a) the retired helpers consumed nothing else, so the freeze
  is 1:1 with the removed reads and no editorially selected bytes are introduced; (b) the untested shell
  text (551 lines: parsers, python seams, hooks glue) has no pin consuming it, and freezing it would create a
  second, unasserted oracle nobody can justify; (c) every frozen value is reproduced from the recorded blob.
  Freezing the whole file was rejected as strictly larger for zero added assertion surface.
- S4: `oracle` metadata names `source: lib/sync-agent.sh`, `blob:
  9c6c41337cf523e63707f41e91ec8dae5e81ff3c` (= `git rev-parse fba37a4:lib/sync-agent.sh`), `base`, the
  `driver` (the retired scan: last line whose text contains the needle; heredoc between `<<'EOF'\n` and the
  newline ending the body) and a `note`. The loader refuses a bundle missing any of those keys, refuses a
  `source` other than `lib/sync-agent.sh`, and refuses a payload containing `/Users/`, `/private/` or
  `/var/folders`.
- S5: The hard-coded, independent inventory is `syncAgentShGoldenPinNames` — 3 pins, source order
  (`cd_line_target`, `cd_line_source`, `usage_heredoc`) — checked by `validateGoldenIdentity` (exact count,
  order, uniqueness) before any pin is used. A per-pin shape check refuses a record that does not carry its
  own payload (cd pins: needle + `line > 0` and no text; usage pin: text and no needle/line), so a
  miscaptured record fails instead of silently matching.
- S6: `syncAgentScript` and its `t.Skipf` are deleted; nothing in the file reads `lib/` any more. Every other
  assertion in `syncagent_test.go` — including the platform.sh frozen loader merged in #511 — is preserved,
  and the file's provenance header now names the frozen bundle for the source-structure pins.
- S7: Negative controls: mutating a frozen line number, mutating the frozen heredoc text, deleting a pin and
  duplicating a pin each fail rc=1, with the golden restored byte-identically (`cmp`) afterwards.
- S8: No `lib/` access: both focused tests pass rc=0 (PASS, not SKIP) with `lib/sync-agent.sh` temporarily
  renamed, and `grep 'lib/' syncagent_test.go` shows only provenance text, the frozen-metadata assertion and
  assertion/diagnostic strings (no `os.ReadFile` / exec of the repo `lib/` tree).
- S9: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, `go test ./internal/sync/ -count=1`
  rc=0, `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.
- S10: The bundle is read-only — loaded through `readGoldenBundle` and never rewritten by a test, so
  re-capturing needs a human decision (recorded in the `note`).

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `syncAgentScript` + `TestSyncAgentCDLineMatchesShellSource` (locates each cd statement in `lib/sync-agent.sh`, `t.Skipf` when absent) | FREEZE | the consumed facts are the two statements and the lines bash reports (108/110) in a file deleted in PR4b; no host-dependent value, so needle + line freeze verbatim. RETIRE was rejected: the cd line numbers are observable CLI output (`syncagent.go` prints them in its failing-cd diagnostic), and no other bundle carries them. RETARGET has no candidate. |
| `TestSyncAgentUsageMatchesShellHeredoc` (slices the `<<'EOF'` heredoc out of `lib/sync-agent.sh`, `t.Skipf` when absent) | FREEZE | the heredoc body is the byte-exact `syncAgentUsage` constant printed by `--help`; it is a literal with no substitution, so it freezes verbatim. RETIRE was rejected: `syncAgentUsage` would lose its only oracle. RETARGET (`cli` golden usage surfaces) does not carry this text. |

## Frozen bundle

`internal/sync/testdata/sync_agent_sh_golden.json` — 2504 B, `oracle` (5 keys) + 3 `pins`
(`cd_line_target`, `cd_line_source` → `{name, needle, line}`; `usage_heredoc` → `{name, text}`). The only
reviewable content is the three values.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S8 | inline | |
| T2 | S9–S10 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order; this follow-up was found in the S4 final inventory of
  PR #511, where `syncagent_test.go:31` (`syncAgentScript`) was classified as the last live `lib/` read with
  a skip.
- L2: Baseline before any edit: both pins rc=0 (`logs/00-baseline.log`, captured from the live
  `lib/sync-agent.sh`); the worktree's copy is byte-identical to `fba37a4:lib/sync-agent.sh` (`cmp` rc=0).
- L3: RED after the restructure with no golden: `read frozen oracle: open
  testdata/sync_agent_sh_golden.json: no such file or directory` at `syncagent_test.go:111` and `:129`,
  rc=1, zero SKIP lines (`logs/01-red-missing-golden.log`).
- L4: The capture harness was throwaway, outside the worktree
  (`~/.hermes/.../go20-pr4a-s4-syncagentsh/cap/capture.py`): it reproduces the retired extraction verbatim
  (last line containing the needle; `body[:end+1]`) and emits the loader's own struct shape, so the frozen
  payload is the legacy scan output, not a projection of the port.
- L5: Captured twice and `cmp` byte-identical (`logs/02-capture1.log`, `logs/02-capture2.log`,
  `logs/03-capture-cmp.log` cmp_rc=0): 2504 B, 3 pins in source order, `blob`
  `9c6c41337cf523e63707f41e91ec8dae5e81ff3c` = `git rev-parse fba37a4:lib/sync-agent.sh`. The leak grep over
  the committed golden for `/Users/`, `/private/`, `/var/folders` is 0.
- L6: GREEN: both focused pins rc=0 with the frozen bundle (`logs/04-green.log`).
- L7: Negative controls, each rc=1, golden restored from the pristine capture and `cmp`-verified
  byte-identical after each (`logs/05-nc-mutate-line.log` — `cd_line_target: pinned line 108, but the frozen
  lib/sync-agent.sh:109 carries TARGET_PATH="$(cd ... )"`; `logs/06-nc-mutate-heredoc.log` — `syncAgentUsage
  drifts from the frozen lib/sync-agent.sh heredoc`; `logs/07-nc-delete-pin.log` — `2 records, want 3`;
  `logs/08-nc-duplicate-pin.log` — `4 records, want 3`).
- L8: No-`lib/` proof: with `lib/sync-agent.sh` renamed for the duration of the run, both pins were rc=0
  with `--- PASS` and zero `SKIP` lines (`logs/09-lib-renamed.log`); the file was restored immediately,
  `git status --short lib/` was empty and the restored file is byte-identical to the base blob.
- L9: `grep 'lib/' syncagent_test.go` (`logs/10-librefs.log`) now shows only provenance comments, the
  frozen-metadata assertion and assertion/diagnostic strings — no read or exec of the repo `lib/` tree.
- L10: GREEN: `gofmt -l internal/sync` empty (`logs/11-gofmt.log`), `go vet ./internal/sync/` rc=0
  (`logs/12-vet.log`), `go test ./internal/sync/ -count=1` rc=0 in 11.86s (`logs/13-package-tests.log`).
- L11: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/15-build.log`) and `./tests/validate.sh` rc=0
  (`logs/16-validate.log`). The worktree carries only the three intended paths; no scratch, cache or capture
  artifact is inside it, and the change is left unstaged for the orchestrator.
