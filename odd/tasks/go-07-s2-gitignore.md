# [Go 07.S2] Gitignore renderers — Go port behind the S1 strangler flags

Card: `6abafbb4ee10be6cb8104fc8` — https://trello.com/c/6abafbb4 (parent `6a84e7acca1baf394d9b482b`)
Branch: `change/go-07-s2-gitignore` · Worktree: `.worktrees/go-07-s2-gitignore`
Base: `af2dbdd` · Epic: `go-single-binary` (slice **S2**)
Plan (authoritative, already merged): `odd/tasks/go-07-sync-plan.md` (§(c) strangler
mechanics, §(d) slicing)

## Goal

Port the two `.gitignore` renderers of the sync spine to Go as the first real
forward-strangler steps, selected by the flags S1 already defined:

1. `RenderAiSpecsGitignore` — byte port of `lib/_internal/gitignore-render.py`
   (`GO_SYNC_STEP_GITIGNORE=go`).
2. `RefreshRootGitignore` — byte port of
   `lib/_internal/gitignore-root-refresh.py` (`GO_SYNC_STEP_GITIGNORE_ROOT=go`).

`python` (the default) keeps exec'ing the existing modules, so the Go and Python
legs stay differentially comparable.

## Scope

- `internal/sync/gitignore.go` — the two renderers.
- `internal/sync/gitignore_test.go` + `internal/sync/testdata/gitignore_ref.py`
  — differential test: each case runs the Go port and the REAL Python module
  (via the ref driver, subprocess) in isolated temp dirs and requires byte
  equality of file bytes, stdout, stderr and rc.
- `internal/sync/sync.go` — per-step wiring; refusal kept for unknown modes.
- `tests/parity/parity.py` — `deps-gitignore` fixture (two failing `[[deps]]`,
  gitignore rendered before deps materialization) and `GO_SYNC_STEP_*`
  forwarding into the legs' `BASE_ENV`.
- `odd/tasks/go-07-s2-gitignore.md` — this document.

## Non-goals (binding)

- No other step is ported: `GO_SYNC_STEP_<OTHER>` still has no handler, and any
  value outside {`python`, `go`} for the two ported steps is still refused.
- No Python/Bash module is deleted (Q5: the CLI only ADDS Go).
- No change to the FROZEN deps-materialization failure semantics or step order.
- The "begin marker without matching end marker" path is not parity-gated: the
  Python authority raises an unhandled `ValueError` traceback; the Go port is
  deliberately TOLERANT (one-line error, rc 1).

## Design

### Byte-exact render body

`render()` in `gitignore-render.py` joins `[HEADER, ".internal/", ".deps/", "",
"# Recipe …", "recipes/**", "!recipes/*/", "!recipes/*/overrides/",
"!recipes/*/overrides/**", "", FOOTER]` with `"\n"`. Both `HEADER` and `FOOTER`
carry their own trailing newline, so the emitted file ends with a single `\n`.
The Go port reproduces the join verbatim; the task text's "NO trailing newline"
note was wrong and the differential test pins the actual bytes.

### Universal-newline reads

Python's `Path.read_text()` applies universal-newline translation on read
(`\r\n` and lone `\r` become `\n`). `readTextUniversal` reproduces this for both
the template and the existing `.gitignore`, so CRLF inputs stay byte-identical.

### Wiring

`stepMode("gitignore")` / `stepMode("gitignore-root")` choose the leg per step;
`"python"` execs the module exactly as S1 did. A mode outside {`python`, `go`}
aborts loudly with the unchanged S1 error line.

### Known deviation

The unhandled Python `ValueError` on a begin marker without an end marker is
replaced by a deterministic one-line stderr error and rc 1. This path is not
part of the differential corpus by design.

### Parity env forwarding (added beyond the fixture)

The harness builds each leg's environment from a fixed `BASE_ENV` and therefore
dropped any operator-set `GO_SYNC_STEP_*`, which would have made the `go` run a
silent python-vs-python no-op. `BASE_ENV` now forwards every `GO_SYNC_STEP_*`
from `os.environ`; the legacy leg ignores them entirely, so only the Go leg's
routing changes.

## Tasks

- [x] T1 — Verify worktree/branch/base and clean pre-write baseline.
- [x] T2 — Read `internal/sync/sync.go`, both Python renderers, the parity harness, and the ref-driver patterns.
- [x] T3 — RED: `testdata/gitignore_ref.py` + `gitignore_test.go`; `go test -run Gitignore` fails (undefined functions).
- [x] T4 — GREEN: implement `internal/sync/gitignore.go`; focused differential test green (8/8 subtests).
- [x] T5 — Wire both steps into `internal/sync/sync.go`.
- [x] T6 — Parity fixture `deps-gitignore` in `tests/parity/parity.py`.
- [x] T7 — Validation: `gofmt -l internal/` empty, `go test ./internal/sync/` green, `go build ./...` green, `python3 tests/parity/parity.py --self-test` passes.

## Gate

- `gofmt -l internal/` → empty.
- `go test ./internal/sync/ -count=1` → ok.
- `go build ./...` → ok.
- `python3 tests/parity/parity.py --self-test` → zero deltas (orchestrator runs
  the full corpus in both `GO_SYNC_STEP_*` legs).

## Evidence

### RED

`go test ./internal/sync/ -run Gitignore -count=1` failed to build:

```
internal/sync/gitignore_test.go:148:12: undefined: RenderAiSpecsGitignore
internal/sync/gitignore_test.go:164:12: undefined: RefreshRootGitignore
… (7 references total)
FAIL	ai-specs.dev/ai-specs/internal/sync [build failed]
```

### GREEN

`go test ./internal/sync/ -run Gitignore -count=1 -v` → `PASS`, 8/8 subtests
(2 render, 5 root-refresh, 1 missing-template), `ok …/internal/sync`.

### Parity

- `deps-gitignore` legacy-vs-legacy (self-test mode): **zero deltas**.
- `deps-gitignore` legacy-vs-**Go** with `GO_SYNC_STEP_GITIGNORE=go`
  `GO_SYNC_STEP_GITIGNORE_ROOT=go` (single fixture, gate-absent): **zero deltas**
  — the Go spine's rendered `ai-specs/.gitignore` and root agent block match the
  legacy leg byte-for-byte.
