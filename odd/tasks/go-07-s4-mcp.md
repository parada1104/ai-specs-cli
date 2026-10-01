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
- Known limitation (not exercised by the corpus): an integer JSON literal wider
  than int64 is parsed as float64 (Python keeps arbitrary precision).
- `RenderMCPFile` mirrors `main()`'s body, not its argv parsing; the
  `--dry-run` / `--recipe-mcp` flags and the usage/rc-2 path stay in the Bash
  caller until S15 wiring.
