# Feature: go-04-module-bootstrap

Card: [Go 04] Go module bootstrap + command skeleton (Trello 6a84e7818039d13bb9d5e78e)
Branch: change/go-04-module-bootstrap (from epic/go-single-binary @ 1ab48a2)

## Tasks

- [x] 1. Scaffold root module: go.mod (go1.24.13, module ai-specs.dev/ai-specs, zero deps), cmd/ai-specs, internal/cli, internal/home
- [x] 2. RED: failing tests first (routing, version, help golden, home resolution, unknown command)
- [x] 3. GREEN: AI_SPECS_HOME resolution with symlink walk (internal/home)
- [x] 4. GREEN: dispatch table (14 verbs), unknown exit 2, native version ("unknown" fallback) + byte-exact help
- [x] 5. GREEN: passthrough shim (exec lib/*.sh, args/env/stdout/stderr/exit/TTY)
- [x] 6. Differential smoke test: built binary vs bin/ai-specs (usage surfaces + one safe real invocation per family)
- [x] 7. ADR docs/go-adr/0001-cli-framework.md (stdlib flag vs cobra/urfave); wire root module into tests/run.sh
- [x] 8. Full verification: ./tests/run.sh, go build ./..., go test ./..., smoke tests

## Evidence

- RED: `go test ./...` exit 1 (undefined Route/ResolveHome — build-failed packages) before implementation.
- GREEN: `go test ./... -count=1` exit 0 (internal/cli ok, internal/home ok).
- go build ./... / go vet ./... / CGO_ENABLED=0 go build: all exit 0.
- TestDifferentialSmoke: 12/12 subtests pass (byte-identical stdout/stderr + exit vs bin/ai-specs).
- ./tests/run.sh: exit 0 — vault-fs-mcp ok, gate Go tests ok, root module ok, Python `Ran 2410 tests ... OK (skipped=168)`.
- Independent gentle-ai-verify pass: all checks exit 0; git status clean (only tests/run.sh +2 lines tracked-modified; no repo mutation by tests).
- Commit identities: recorded by the orchestrator at staging (worker/this session do not commit per card boundary).

- RED: `go test ./...` exit 1 — undefined: Route/routeKind/ResolveHome (build failure of test-first files).
- GREEN: `go test ./...` exit 0 — internal/cli + internal/home ok, incl. 12-case differential smoke (2 help cases pinned to embedded bytes; legacy help heredoc is unquoted and command-substitutes `\`ai-specs\`` at runtime, so the launcher's help output is PATH-dependent).
- Verification: go build 0, go vet 0, CGO_ENABLED=0 go build 0, go test -count=1 0, ./tests/run.sh 0 (bash vault-mcp + gate module + root module + 2410 python tests).

## Notes

- Trello MCP down at session start; card scope taken from the task brief (declared authoritative inline).
- version behavior unified on "unknown" fallback, exit 0 (post docs-drift card) — mirror lib/version.sh current behavior, NOT parity-contract D32.
