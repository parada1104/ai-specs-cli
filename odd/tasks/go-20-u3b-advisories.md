# Go 20 / U3b-2 — Advisory remediation from the closed native review

Follow-up of `odd/tasks/go-20-u3b-tui.md`. The native review of U3b-2 (`review-431359c0d529da28`,
candidate `de0eacd8` over base `4e6da9d`) closed **approved** with **0 blocking findings** and 14
advisory ones. This unit addresses them: each finding is either fixed with a pinning test or
declined with a recorded reason. Worktree `.worktrees/go-20-u3b-tui`, branch `change/go-20-u3b-tui`.

## Specs

- S1 — Human, verbatim (2026-10-17): "Y no podemos arlar directamente los advisory en vez de
  dejarlos como follow up. No me gusta dejarlos como follow up." So no advisory leaves this unit as
  a deferred follow-up.

- S2 — The closed review's advisory list, verbatim from the closure envelope. This is the
  reproducible evidence: the reviewer artifacts themselves were burned with the authority on
  acknowledgement, so the ids, lenses and locations below are the record. Every entry is
  `non-blocking`: none opened a correction and no correction transition was offered.
  - `R1-agents-read-error-empty-picker` — risk — `internal/config/agents.go:16-19` — WARNING
  - `R1-array-of-tables-header` — risk — `internal/config/agents.go:110-116` — SUGGESTION
  - `R1-multiline-string-header-swallow` — risk — `internal/config/agents.go:54-66` — WARNING
  - `R2-headline-dup` — readability — `internal/tui/panel.go:32-46` — WARNING
  - `R2-pauseresult-dead-code` — readability — `internal/hub/interactive.go:190-197` — SUGGESTION
  - `R2-record-contradiction` — readability — `odd/tasks/go-20-u3b-tui.md:194-197` — WARNING
  - `R2-tui-tty-claim` — readability — `internal/tui/tui.go:8-10` — WARNING
  - `R2-twochoices-name` — readability — `internal/tui/tui_test.go:37-39` — SUGGESTION
  - `R3-1` — reliability — `internal/tui/terminal.go:108-112` — CRITICAL — **disposition: refuted**
  - `R3-2` — reliability — `internal/tui/raw_linux.go:21-25` — WARNING
  - `R3-3` — reliability — `internal/config/agents.go:53-73` — WARNING
  - `R3-4` — reliability — `internal/tui/tui.go:152-155` — WARNING
  - `R3-5` — reliability — `internal/tui/panel.go:66-75` — SUGGESTION
  - `R4-atomicwrite-readonly-refusal` — resilience — `internal/config/writes.go:148-152` — WARNING

- S3 — Disposition rule. A finding is either `FIX` (with a test that fails before the change and
  pins the fixed behaviour) or `DECLINE` (with the reason recorded in this document). Nothing is
  silently dropped. Behaviour that is parity with `lib/_internal/hub.py` is never changed to satisfy
  a reviewer; the parity contract wins, and the port's own prose is corrected when the prose is what
  is wrong.

- S4 — Multiline strings, reproduced. This is the only advisory that is data corruption, and the
  port's prose claims it cannot happen. Manifest:

  ```
  [project]
  name = "t"
  desc = """
  [agents]
  not a table
  """

  [agents]
  enabled = ["pi"]
  ```

  `SetEnabledAgents(path, []string{"cursor"})` returns **nil error** and leaves:

  ```
  [project]
  name = "t"
  desc = """
  [agents]
  not a table
  """

  enabled = ["cursor"]
  [agents]
  enabled = ["pi"]
  ```

  Three defects in one write: the inserted key landed **inside the string** (string data corrupted),
  it landed **outside any table** (a stray top-level key), and the real `[agents].enabled` still says
  `["pi"]` — the user's selection was silently dropped. The valid→invalid guard does not fire
  because the result is still valid TOML. After the fix, a line inside a multi-line string is never
  structural: the key is written into the real `[agents]` table and the string body is byte-identical.

- S5 — Panel geometry. `RenderPanel` measures rows with `len(string)`, which counts bytes, so a row
  whose content carries a non-ASCII rune makes the box wider than its border. Rows are measured in
  display runes; a test pins a non-ASCII label and a non-ASCII text against the border width.

- S6 — `EnabledAgents` conflates "there is no list" with "the manifest could not be read". A missing
  file, a missing `[agents]` table and a mistyped key all mean an empty picker (parity, keep it), but
  an unreadable or unparsable manifest must not: the hub's Agents picker would then start fully
  unchecked and `SetEnabledAgents` would persist that empty selection over the user's real list. The
  read reports the difference and the hub refuses to open the picker, printing why.

- S7 — Declines, with reasons.
  - `R2-headline-dup`: the `(N error(s))` tail on the Summary row is `hub.py:369-373` verbatim. It is
    the frozen row vocabulary, not a duplicate to remove.
  - `R3-4`: `Confirm` already separates "answered" from "aborted" via its `ok` return. `Pause`
    returns one bool because its only caller implements `hub.py`'s `if not pause(): return 0`, where
    Enter and a closed stdin must reach the same branch. The Ctrl-C-at-pause divergence is already
    recorded in the unit document.
  - `R3-3`: the whole-file CRLF renormalization is the documented `readManifestText` parity rule, and
    stays. What is wrong is the `SetEnabledAgents` prose claiming "every other byte of the manifest,
    comments included, survives" while a CRLF manifest is rewritten to LF throughout; the prose is
    corrected instead.
  - `R3-1`: the native refuter refuted it, and `terminal.go:86` maps both `'\r'` and `'\n'` to
    `KeyEnter`, which is what clearing `ICRNL` makes necessary. No change.

- S8 — `R3-2` states no recoverable claim, so the code at `raw_linux.go:21-25` was read against its
  darwin twin: both clear `ICANON|ECHO|ISIG` and `ICRNL` but leave `IEXTEN` (literal-next) and
  `IXON|IXOFF` (flow control) armed, so Ctrl-V swallows a keystroke and Ctrl-S can freeze a prompt.
  Both twins clear them. Raw mode cannot be unit-tested without a PTY, so this fix carries no test and
  says so in its comment.

- S9 — `R4` states no recoverable claim either. The refusal at `writes.go:148-152` tests the owner
  write bit (`0o200`) while its message says "a manifest nobody can write", so a manifest writable by
  group or other is refused although the user can write it. The test matches the message; a
  group-writable manifest is written and an all-read-only one is still refused byte-identically.

- S10 — `tableName` answers `""` for `[[deps]]` by accident of the bracket scan. `[[agents]]` is an
  array of tables, never the `[agents]` table, and must be recognised as such explicitly.

## Tasks

- T1 (S4) — Multiline-string-aware structural scan in `SetEnabledAgents` + RED/GREEN test. — commit A
- T2 (S6, S10) — `EnabledAgents` read-error distinction + hub refusal; explicit `[[...]]` rejection. — commit A
- T3 (S9) — Read-only refusal matches its message; group-writable still writes. — commit A
- T4 (S7, S3) — Correct the `SetEnabledAgents` and unit-document prose about CRLF and refusal. — commit A
- T5 (S5) — Panel rows measured in runes; non-ASCII test. — commit B
- T6 (S7) — `pauseResult` collapsed (landed with commit A: `internal/hub/interactive.go` is shared with T2);
  `twoChoices` renamed; `tui.go` comment corrected. — commit A for the hub, commit B for the package
- T7 (S8) — Raw-mode flag hardening in both twins. — commit B
- T8 (S2, S3) — Record the disposition table for all 14 findings in this document. — commit B

Commit A = `4e9f5d6` "fix(config): stop the agents writer from corrupting multi-line strings".
Commit B = `95cec6a` "fix(tui): measure the panel in runes and own the terminal's controls".
The unit is two work units and ~250 authored lines; the branch's total against `origin/main` is the
whole epic stacked under it, not this unit.

## Dispositions

Every finding from S2, with its outcome. `FIX` names the test that pins it; `DECLINE` names the
reason, and nothing is left open.

| finding | lens | outcome | pin / reason |
| --- | --- | --- | --- |
| `R1-agents-read-error-empty-picker` | risk | FIX | `TestEnabledAgentsReportsUnreadableManifest` (config) + `TestAgentsRefusesUnreadableManifest` (hub) |
| `R1-array-of-tables-header` | risk | FIX | `TestTableNameRejectsArrayOfTables` |
| `R1-multiline-string-header-swallow` | risk | FIX | `TestSetEnabledAgentsIgnoresStructuralLinesInsideMultilineStrings` |
| `R2-headline-dup` | readability | DECLINE | the `(N error(s))` tail is `hub.py:369-373` verbatim: frozen row vocabulary, not a duplicate |
| `R2-pauseresult-dead-code` | readability | FIX | body collapsed to `return 0, !s.pause()`; the hub suite pins every submenu's exit |
| `R2-record-contradiction` | readability | FIX | the unit document now distinguishes what is refused, what is handled, and what is recorded |
| `R2-tui-tty-claim` | readability | FIX | the package comment no longer claims `internal/tty` owns the raw-mode primitive |
| `R2-twochoices-name` | readability | FIX | renamed `threeChoices`, with the fixture's shape documented |
| `R3-1` | reliability | DECLINE | refuted by the native refuter; `terminal.go:86` maps both `'\r'` and `'\n'` to `KeyEnter`, which clearing `ICRNL` makes necessary |
| `R3-2` | reliability | FIX | `IEXTEN`/`IXON`/`IXOFF` cleared in both twins (S8); the unit document had already recorded "Ctrl-S freezing output" as a known defect and now records it fixed |
| `R3-3` | reliability | DECLINE | whole-file CRLF renormalization is `readManifestText`'s documented parity rule; only the prose claiming "every other byte survives" was wrong, and it is corrected (S7) |
| `R3-4` | reliability | DECLINE | `Confirm` already separates answered from aborted via `ok`; `Pause`'s single caller implements `hub.py`'s `if not pause(): return 0`, where Enter and a closed stdin must reach the same branch |
| `R3-5` | reliability | FIX | `TestRenderPanelAlignsNonASCIIRows`, which also caught a defect nobody reported: the top border was 3 runes shorter than the body for ASCII too, and now matches |
| `R4-atomicwrite-readonly-refusal` | resilience | FIX | `TestSetEnabledAgentsWritesGroupWritableManifest`; the existing read-only refusal test still passes |

Totals: 10 fixed, 4 declined with a recorded reason, 0 left open.

Verification of this unit, all run from `.worktrees/go-20-u3b-tui`:

- RED observed before each fix, separately: the multiline/tableName/group-writable probes failed with the
  exact bytes recorded in S4 and S9, and the panel probe failed on both the non-ASCII row and the
  pre-existing border mismatch.
- Differential against the candidate commit `b302d3f` in a throwaway worktree: `go test ./internal/config/`
  gave the same 14 failures before and after (none caused by this unit); the panel dump differs only in
  the top border width; `go test ./...` gives the identical 26-failure set on both sides.
- The interpreter is half of this suite's environment. Pointed at the mise SHIM, `python3` resolves back
  to the system 3.9 under the tests' sanitized HOME, which has no `tomllib`, and that — not the hub —
  turned 34 of the 47 hub/PTY tests red. The real `3.14.7` binary (`~/.local/share/mise/installs/python/
  3.14.7/bin/python3.14`) is the interpreter these tests want: with it the hub + PTY pair is 47/47 `OK`.
- `go test ./...` is green with that interpreter when it runs ALONE. Its differential pairs and
  `internal/gatebinary` disagree when a second suite runs against the same worktree and `TMPDIR` at the
  same time (observed: two concurrent runs reported failures in `sync`, `target`, `toml` and
  `gatebinary` that a solo run does not reproduce). Treat concurrent runs of this suite as unsupported
  rather than as evidence about the change.
- The throwaway base worktree cannot carry the `test_topology_*` tests: topology is resolved from the
  repository's own shape, and a detached temp worktree of the same clone resolves it differently. Those
  three failures belong to the artifact, not to `b302d3f`, and this unit touches no topology code.
- The PTY suite is the only place raw mode and the panel are exercised end to end (it spawns the real
  binary on a pty), so it is the gate for T7 and T5, not the Go unit tests.

## Log

- L1 — Human, verbatim (2026-10-17): "Y no podemos arlar directamente los advisory en vez de dejarlos
  como follow up. No me gusta dejarlos como follow up."
- L2 — Evidence (2026-10-17): the reviewer artifacts were burned with the authority at
  `acknowledge-approved`, so per-finding prose is unrecoverable. Findings with a semantic id are
  judged from that id plus the code at its location; `R3-1`..`R3-5` carry no text, so those five were
  judged by reading the code myself. `R3-1` was already refuted by the refuter. `R3-2` and `R4` have
  no recoverable claim: the code at both locations was reviewed on its own merits (S8, S9).
- L3 — Probe (2026-10-17), commitless, via a throwaway `zz_probe_test.go` in `internal/config`:
  the S4 corruption is real (`SetEnabledAgents` returns nil and writes the key inside the string), and
  a CRLF manifest is rewritten to LF end to end (S7, `R3-3`).
