# [Go 07.S3a] Agents-render pure core — byte-exact Go port

Card: `[Go 07.S3]` — https://trello.com/c/jYphZBhb (parent epic `go-single-binary`)
Branch: `change/go-07-s3-agents` · Worktree: `.worktrees/go-07-s3-agents`
Base: `14b80f6` · Slice: **S3a** (pure rendering core)

Plan (authoritative, already merged): `odd/tasks/go-07-sync-plan.md`.

## Goal

Port the PURE rendering core of `lib/_internal/agents-render.py` (881 lines) to
Go as a byte-exact differential port: the recipe-fragment helpers, every
`_section_*` renderer, `_render_lines`, and the exact byte assembly `render()`
performs before its governance decision.

`RenderAgentsMarkdown` returns exactly the bytes `render()` would pass to
`output_path.write_bytes(...)` when the governance decision is `"write"`:

```
"\n".join(_render_lines(manifest, resolved)).encode("utf-8")
```

## Scope of this commit (S3a)

- `substitute_config` (`{config.KEY}`, `{{ }}` escapes, unknown/bare keys
  verbatim, lone-brace tolerance).
- `collect_recipe_brief_fragments` (enabled-order iteration, `{config.KEY}`
  substitution per recipe namespace, key-dedup then exact-text dedup).
- `_validate_brief_modes` (byte-exact `ValueError` message in Go).
- `_redact_env_value`.
- `_section_intro` / `_section_project` / `_section_mcp` /
  `_section_runtime_flow` / `_section_trello` / `_section_context_sources` /
  `_section_conflict_policy` / `_section_workflow_rules` /
  `_section_useful_commands`.
- `_render_lines` and the `"\n"`-join byte assembly.
- `internal/sync/testdata/agentsrender_ref.py` + `internal/sync/agentsrender_test.go`:
  12-case byte-for-byte differential against the REAL Python module.

## S3b remainder (out of scope here)

- `_load_util` / `_load_lock` and everything that depends on them:
  `classify_brief`, `brief_ownership_state`, `brief_effective_state`,
  `_brief_decision`, `_brief_preserve`, `_brief_is_our_output`, and the lock
  baseline read/write (`load_lock` / `set_brief_baseline` / `write_lock`).
- `lib/_internal/brief-render-policy.py` port and `worktree-isolation` /
  `worktree-flow` gate-mode interplay.
- Native wiring in `internal/sync/sync.go` + `sync-agent`: reading the TOML and
  resolved-config JSON, computing `project_root`, the write-governance state
  machine, and the unknown-VCS **stderr warning** (a writer side effect with no
  place in the pure byte function).
- Fixtures: `adopt-brief`, `brief-render-false`, `marker-preserved`,
  `user-modified-preserved`.

## Design

### Manifest representation: `*toml.Table`, not `map[string]any`

The card sketched `RenderAgentsMarkdown(manifestData map[string]any, ...)`.
TOML **document order is observable output** here — `[mcp.*]` servers render in
document order and the effective `mcp_descriptions` dict preserves insertion
order (recipe fragments first, then `[brief].mcp_descriptions` overriding in
place). A Go `map[string]any` cannot preserve that: the "full" fixture renders
`trello` then `engram`, which a sorted map would emit as `engram` then
`trello`. The signature therefore takes `*toml.Table`, the parsed-manifest type
`internal/config` and `internal/target` already use, which also lets
`_section_project` call `target.ProjectRepoTopology` directly instead of
re-deriving topology.

`resolved` (the resolved-config JSON) stays `map[string]any`; nothing that
reads it iterates its object keys (recipes are reached via the `enabled`
array), so order loss is not observable there.

### Python formatting mirrors

`pyStr` / `pyRepr` / `pyReprString` / `formatPyFloat` / `pySplitLines` /
`pyStrip` reproduce Python semantics for both TOML and JSON-decoded values.
`pyReprString` and `formatPyFloat` are copied verbatim from the unexported
`internal/target/json.go` helpers (same package boundary, no import possible).
`pySplitLines` matches `str.splitlines()` boundaries; `pyStrip` matches
`str.isspace()`.

### Validation-error contract

`_validate_brief_modes` raising is a **human-visible sync failure**. The pure
`RenderAgentsMarkdown(...) []byte` returns `nil` on invalid modes; callers must
validate first with the exported `ValidateBriefModes(brief *toml.Table) error`,
whose message is byte-identical to the Python `ValueError`, and must not write
on `nil`. The differential pins both.

## Tasks

- [x] T1 — Verify worktree/branch/base (`change/go-07-s3-agents`, base `14b80f6`) and clean baseline.
- [x] T2 — Read `agents-render.py`, `util.py` topology, `internal/toml`, `internal/target`, the gitignore differential pattern, the parity fixtures, and `test_agents_render_brief_fragments.py`.
- [x] T3 — RED: `testdata/agentsrender_ref.py` + `agentsrender_test.go`; `go test -run AgentsRender` fails to build (`undefined: RenderAgentsMarkdown`, `undefined: ValidateBriefModes`).
- [x] T4 — GREEN: implement `internal/sync/agentsrender.go`; 12/12 differential subtests byte-equal.
- [x] T5 — This feature document.

## Gate (this commit)

- `gofmt -l internal/` → empty.
- `go test ./internal/sync/ -count=1` → ok.
- `go build ./...` → ok.

## Evidence

### RED

`go test ./internal/sync/ -run AgentsRender -count=1` failed to build:

```
internal/sync/agentsrender_test.go:98:11: undefined: RenderAgentsMarkdown
internal/sync/agentsrender_test.go:111:13: undefined: ValidateBriefModes
FAIL	ai-specs.dev/ai-specs/internal/sync [build failed]
```

### GREEN

`go test ./internal/sync/ -run TestAgentsRenderDifferential -count=1 -v` → PASS,
12/12 subtests, byte equality + SHA-256 agreement on every case:

| case | covers |
| --- | --- |
| minimal manifest project only | (a) H1 + Project + pointer-only Useful Commands |
| full manifest with every brief section | (b) intro, project, MCP table + global desc, runtime flow + VCS bullet, trello, context, conflict, workflow, useful commands |
| deps and recipe brief fragments | (c) `[[deps]]` + fragments in resolved-config |
| config substitution edge cases | (d) known/missing/bare/escape/lone-brace, bool/int/list `str()` |
| mcp env redaction | (e) `$VAR`, `${VAR}`, literal secret, padded, array env |
| missing optional sections | (f) project only |
| blank optional sections | (f) whitespace-only intro/purpose |
| repo topology from project field | `[project].repo_topology` via-config line |
| unknown vcs recipe uses generic label | custom VCS fallback label |
| vcs sibling filtering keeps only the bound provider | `_section_workflow_rules` filter |
| replace modes suppress recipe fragments per section | `*_mode = 'replace'` |
| invalid brief mode raises | (g) Go error == Python `ValueError` message |

### Artifact sizes

- `internal/sync/agentsrender.go` — 1041 lines
- `internal/sync/agentsrender_test.go` — 451 lines
- `internal/sync/testdata/agentsrender_ref.py` — 106 lines

### Not ported

None for the pure core. The only deferred behavior is the unknown-VCS stderr
warning (a writer side effect of `_section_runtime_flow`) plus the entire
governance/lock layer, both explicitly assigned to S3b above.
