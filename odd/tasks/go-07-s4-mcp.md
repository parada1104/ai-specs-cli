# [Go 07.S4] MCP config rendering — byte-exact Go port of `mcp-render.py`

Card: `[Go 07.S4]` — https://trello.com/c/roaRjGzu (parent epic `go-single-binary`)
Branch: `change/go-07-s4-mcp` · Worktree: `.worktrees/go-07-s4-mcp`
Base: `7e5b8ac` (S3 merge) · Plan: `odd/tasks/go-07-sync-plan.md` (S4 row)

## Goal

Port `lib/_internal/mcp-render.py` (315 lines) to Go in `internal/sync` as a
byte-exact library, proven by a module differential against the REAL Python
module (plan gate 2), plus the `mcp-per-agent` parity fixture.

## Scope decision (S2 precedent)

`mcp-render.py` has exactly one caller: `lib/sync-agent.sh:486`. The Go spine
(`internal/sync/sync.go`) still execs `sync-agent.sh` unchanged until S15, so
there is no Go call site to put behind a `GO_SYNC_STEP_*` flag in this slice.
S4 therefore ships the Go library + differential only; `mcp-render.py` stays
alive and the native wiring lands with the `sync-agent` fan-out in S15 (same
treatment `gitignore-render.py` received in S2). No reverse bridge from Bash.

## Tasks

- [x] T1 — Go port of `mcp-render.py` (`load_mcp` incl. recipe-mcp merge,
  env-var translation, generic/opencode translators, slim-write,
  `merge_into_json` with key-order preservation, `$schema`-first for opencode,
  `json.dumps(indent=2)` byte format incl. `ensure_ascii` and float repr,
  `merge_into_toml` section strip/append via `toml.TOMLValue`, `--dry-run`
  output, missing-manifest rc 1, no-`[mcp.*]` skip notice) + module
  differential test against the real Python over every agent in the
  `platform.sh` matrix (claude, cursor, opencode, codex, gemini, pi, omp; copilot
  has no MCP) and the edge cases (existing file preserved keys, invalid/non-object
  JSON, duplicate keys, non-ASCII, prior `[mcp_servers.*]` blocks, empty servers).
- [x] T2 — `mcp-per-agent` parity fixture in `tests/parity/parity.py` (every
  `mcp_key`/path pair incl. opencode `mcp` and codex `mcp_servers`), zero deltas
  × both gate modes.
- [x] T3 — Evidence: `go test ./...`, parity corpus, and this document updated
  with commit ids.

## Evidence

### T1 — byte-exact Go port + module differential

Strict TDD (RED first):

- RED: `go test ./internal/sync/ -run TestMCPRenderDifferential -count=1`
  → `internal/sync/mcprender_test.go:148:8: undefined: RenderMCPFile` /
  `undefined: RenderMCPOptions` — `[build failed]` (the port did not exist yet).
- GREEN: `go test ./internal/sync/ -run TestMCPRenderDifferential -count=1`
  → `ok ai-specs.dev/ai-specs/internal/sync` — 25 differential cases pass:
  7-agent matrix (claude/cursor/opencode/codex/gemini/pi/omp) + existing-key
  order, duplicate keys, invalid/non-object JSON, `$schema` handling, recipe
  merge/override/missing/ non-object, TOML strip + user tables + CRLF, missing
  manifest, no-`[mcp.*]`, dry-run (json + toml), and the env-ref regex quirks.
  Every case asserts byte equality of the written file, stdout, stderr and rc.

Validation:

- `go build ./... && go vet ./internal/sync/ && gofmt -l internal/sync` → empty (clean).
- `go test ./internal/sync/ -count=1` → `ok` (4.2s).
- `go test ./... -count=1` → `ok` for every package.

### T2 — `mcp-per-agent` parity fixture

- `python3 tests/parity/run.py` (builds Go + gate, runs the corpus in both
  gate modes) → `fixtures: 17, failing: 0` for `gate-absent` AND
  `gate-present`; `parity summary: gate-absent failing=0, gate-present
  failing=0 — PASS`. The new `mcp-per-agent` fixture reports zero deltas in
  both modes.
- Confirmed the fixture exercises every pair: after `sync-agent --all` the
  tree holds `.mcp.json` (claude + pi), `.cursor/mcp.json`, `opencode.json`
  (`$schema` promoted first, `mcp` key), `.codex/config.toml`
  (`mcp_servers` tables before the trailing foreign `[user_table]`),
  `.gemini/settings.json` and `.omp/mcp.json`; every foreign key survived.

Today both legs run the Python renderer through `sync-agent.sh` (a shim until
S15); this fixture becomes the Go gate once the fan-out is ported.

### T3 — Evidence summary

- `go test ./... -count=1` → `ok` for every package.
- `python3 tests/parity/run.py` → zero deltas, both gate modes (above).
- Commits: T1 = `3695349`, T2 = `1228c25`, T3 = this document commit.

## Deviations/Quirks

- Reproduced (frozen byte port, do NOT "fix" in Go): Python's `re` `$` anchor
  also matches immediately before ONE trailing newline. So `_ENV_VAR_RE`
  matches `"$VAR\n"`, and `re.sub` replaces only the matched span, leaving the
  newline: result `"${VAR}\n"`. The Go port returns the matched name plus the
  preserved suffix; pinned by the `env ref quirks generic` / `env ref quirks
  opencode local` differential cases.
- Reproduced: the optional braces in `^\$\{?([A-Z_][A-Z0-9_]*)\}?$` are
  independent, so `${VAR`, `$VAR}` and `$VAR` all normalize (to `${VAR}` for the
  generic translator, `{env:VAR}` for opencode).
- Reproduced: opencode URL values are NOT env-substituted — `_translate_opencode`
  copies `url` verbatim for the remote branch; pinned by
  `opencode remote url stays literal`.
- `RenderMCPFile` mirrors `main()`'s body, not its argv parsing; the
  `--dry-run` / `--recipe-mcp` flags and the usage/rc-2 path stay in the Bash
  caller until S15 wiring.

## Native review correction (R3-json-int-overflow)

Lineage `review-9d29cb088f632a95` (CRITICAL, reliability): integer literals
wider than int64 degraded to an approximate float, and an overflow float
literal (`1e400`) errored and discarded the whole existing target file to `{}`.
Fix: `mcpBigInt` keeps over-range integers as canonical decimal digits emitted
verbatim by the JSON and TOML writers; `ParseFloat` `ErrRange` overflow now
yields `±Inf` (Python `float()`), rendered `Infinity`/`-Infinity`/`inf`.
TDD: `go test ./internal/sync/ -run TestMCPRenderDifferential -count=1` failed
on the 3 new cases (RED) and passes with the fix (GREEN).

## Follow-ups (branch `fix/go-07-s4-followups`, base `183f2d8`)

Policy: the next slice (S5) does not start until these are closed. Triage of
the 7 informational advisories of `review-9d29cb088f632a95`, each checked
against the real Python:

- [x] F1 — Unreadable / invalid-UTF-8 existing target (JSON and TOML paths):
  Python `read_text()` raises → rc 1, file untouched; Go swallowed the error and
  rewrote the user's config (advisories R4-SilentReadFailureRewrite, R1-001).
- [x] F2 — Unreadable / invalid-UTF-8 `--recipe-mcp` file: Python raises → rc 1,
  nothing written; Go skipped it silently (R4-RecipeInputSilentlySkipped).
- [x] F3 — Manifest read error reported as "not found"
  (R4-MisattributedManifestReadError).
- [x] F4 — JSON literals `NaN` / `Infinity` / `-Infinity` and lone-surrogate
  escapes (`\ud800`): Python `json.loads` accepts and round-trips them; Go's
  decoder rejected the file and discarded it to `{}` (R3-json-nan-infinity-parse).
- [x] F5 — A lone surrogate that reaches the rendered content raw (a raw TOML
  table name `[mcp_servers.<name>]` or inline-table key from a `--recipe-mcp`
  JSON key like `"\ud800"`): Python `write_text()` re-encodes and raises
  UnicodeEncodeError → rc 1, and the target is left created/truncated empty
  (write_text opens before it encodes); `--dry-run`'s `print()` raises the same
  on a real UTF-8 stdout. Go wrote the raw WTF-8 bytes (rc 0). JSON and TOML
  *values* escape the surrogate via `json.dumps`, so only raw keys leak it.
- Accepted, no change: R4-InPlaceConfigOverwrite (Python `write_text` is the
  same non-atomic in-place write; an atomic rename would change file
  mode/inode semantics vs the frozen oracle) and R4-DifferentialGateSilentSkip
  (same `python3`-absent skip as every differential in the package; CI always
  has python3).

Accepted: Python `read_text` uses the locale encoding; Go assumes UTF-8
(harness runs `PYTHONUTF8=1`).

For F1–F3 Python's stderr is a traceback, which is not byte-reproducible: Go
emits one `error: …` line; the differential pins rc 1, empty stdout, target
bytes unchanged/absent, and Python stderr containing the exception class.

### Evidence (Strict TDD)

- RED: `go test ./internal/sync/ -run TestMCPRenderDifferential -count=1`
  against base `183f2d8`'s `mcprender.go` (16 new cases, fix reverted) fails
  on exactly the 12 F1–F4 cases: `target_unreadable_json` (go rc 0 + rewritten
  file vs ref rc 1 + `{"keep": 1}` untouched), `target_unreadable_toml`,
  `target_invalid_utf8_json`, `target_invalid_utf8_toml`, `recipe_mcp_unreadable`,
  `recipe_mcp_invalid_utf8`, `manifest_unreadable` (go `error: … not found` vs
  PermissionError), `json_nan_infinity_literals`, `json_lone_surrogate_escape`
  (go `\ufffd` vs ref `\ud800`), `recipe_json_nan_json_agent`,
  `recipe_json_nan_codex_toml`, `recipe_lone_surrogate_codex_toml`.
- GREEN: the same command passes — 49 differential cases (29 pre-existing + 16
  F1–F4 + 4 F5). Four new cases already matched the base and stay as regression pins:
  `manifest_invalid_utf8` (invalid UTF-8 already rejected), `target_is_a_directory`
  (Python `IsADirectoryError`; go rc 1, one error line),
  `json_raw_control_char_rejected` (`json.loads(strict=True)` → JSONDecodeError
  → `{}`), `recipe_mcp_invalid_json_ignored` (JSONDecodeError caught).
- Note: the unreadable cases chmod `0200` (read-denied, write-allowed), not
  `000`; under `000` the rewrite also failed and the base's silent-read bug was
  masked. Skipped when euid 0.
- F5 RED: with only the five new cases/tests added (fix reverted) the focused
  command fails: `surrogate server name codex toml truncates existing target`,
  `... creates empty target` and `surrogate env key codex toml ...` all report
  `rc: go=0 ref=1`, `stdout differs`, `target bytes differ`; `TestMCPRenderDryRunSurrogateFails`
  reports `rc: got 0 want 1`. The JSON case passes (already escaped).
- F5 GREEN: the same command passes. Fix: one `mcpHasLoneSurrogate(content)`
  check before print/write; on a non-dry-run hit the port mimics Python's
  open-before-encode by creating/truncating the target empty, then emits one
  `error: …` line and rc 1.

Fixes: `mcpReadTextFile` (read + strict UTF-8) now surfaces read errors from the
manifest (F3, `error: <reason>`), the `--recipe-mcp` input (F2) and both target
writers (F1) instead of swallowing them. F4 replaces the `encoding/json` token
loop with a small recursive-descent decoder: bare `NaN`/`Infinity`/`-Infinity`,
`\uXXXX` lone surrogates kept as WTF-8 so `mcpJSONString` re-emits them verbatim
(valid pairs still combine), raw control characters rejected, Python's exact
whitespace set and duplicate-key/int64/`mcpBigInt`/±Inf semantics preserved.

Known divergence closed by F5: a TOML object *key* holding a lone surrogate
(raw table name or inline key) made Python `write_text` raise
UnicodeEncodeError (rc 1, target created/truncated empty), while Go wrote the
raw WTF-8 bytes (rc 0). Reachable with the codex TOML target and a
`--recipe-mcp` JSON whose key is `"\ud800"`; `mcpHasLoneSurrogate` now fails it
rc 1 like the script.
