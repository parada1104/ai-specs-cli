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

---

# [Go 07.S3b] Write-governance state machine + brief-render-policy + native AGENTS step

Base: `c979a55` (S3a rendering core already ported) · Same branch/worktree.

## Goal

Port the governance half of `lib/_internal/agents-render.py` (L615-881) and
`lib/_internal/brief-render-policy.py` (whole file) to Go, and route the
`sync` AGENTS.md step (a-c strangler flags) through the native path behind
`GO_SYNC_STEP_AGENTS_RENDER=go`.

## Scope of this commit (S3b)

- `classify_brief`, `brief_ownership_state`, `_brief_preserve`,
  `_brief_is_our_output`, `brief_effective_state`, `_brief_decision`,
  `_brief_lock_path` / `_brief_lock_key` — mapped onto `internal/lock`
  (`LoadLock` / `Sha256Bytes` / `SetBriefBaseline` / `WriteLock`).
- The AI_SPECS marker `<!-- ai-specs:runtime-brief -->` and the
  non-strict `Path.resolve()` lock key, byte-exact.
- `RenderAgentsFile` as the `render()` entry: mkdir parents, `write_bytes`
  on the write states, preserve on the others, the unknown-VCS stderr
  warning, and the preserve notice.
- `brief-render-policy.py` → `EvaluateBriefRender` (fail-safe enabled default
  for non-boolean) + `ValidateBriefRender` (`--validate` semantics).
- `internal/sync/sync.go` step 7: `GO_SYNC_STEP_AGENTS_RENDER=go` runs the
  two-step native path (policy gate then renderer); `python` keeps the exec;
  unknown values are refused before any write.
- Differential driver `testdata/briefgov_ref.py` + 13 governance cases.
- Parity fixtures `sync-adopt-brief`, `sync-brief-render-false`.

## Tasks

- [x] T1 — Verify worktree/branch/base (`change/go-07-s3-agents`) and clean baseline.
- [x] T2 — Read the S3a core, `agents-render.py` L615-881, `brief-render-policy.py`,
  `util._python_classify_managed_override`, `lock.py`/`internal/lock`, the S3a
  differential pattern, and the parity `Fixture` pattern.
- [x] T3 — RED: `testdata/briefgov_ref.py` + governance/policy tests;
  `go test ./internal/sync/` fails to build (undefined `RenderAgentsFile`,
  `EvaluateBriefRender`, `classifyBriefState`, …).
- [x] T4 — GREEN: implement `briefpolicy.go` + the S3b section of
  `agentsrender.go`; 13/13 governance cases + 8 policy cases pass.
- [x] T5 — Wire `sync.go` step 7 + `renderAgentsStep`; refusal/env test green.
- [x] T6 — Parity fixtures + legacy-vs-Go `GO_SYNC_STEP_AGENTS_RENDER=go`, zero deltas.
- [x] T7 — This document.

## Gate (this commit)

- `gofmt -l internal/` → empty.
- `go vet ./internal/sync/` → clean.
- `go test ./internal/sync/ -count=1` → ok (S3a's 12 cases + new).
- `go build ./...` → ok.
- `GO_SYNC_STEP_AGENTS_RENDER=go` legacy-vs-Go on both new parity fixtures → zero deltas.

## Evidence

### RED

`go test ./internal/sync/ -run 'Governance|BriefRenderPolicy|PolicyGate' -count=1`
failed to build:

```
internal/sync/agentsrender_test.go:670:22: undefined: EvaluateBriefRender
internal/sync/agentsrender_test.go:673:17: undefined: ValidateBriefRender
internal/sync/agentsrender_test.go:729:31: undefined: loadAgentsRenderInputs
internal/sync/agentsrender_test.go:734:14: undefined: classifyBriefState
internal/sync/agentsrender_test.go:737:14: undefined: briefEffectiveState
internal/sync/agentsrender_test.go:742:25: undefined: RenderAgentsFile
internal/sync/agentsrender_test.go:742:61: undefined: RenderAgentsOptions
internal/sync/agentsrender_test.go:786:11: undefined: renderAgentsStep
internal/sync/agentsrender_test.go:801:11: undefined: renderAgentsStep
internal/sync/briefpolicy_test.go:81:20: undefined: EvaluateBriefRender
FAIL	ai-specs.dev/ai-specs/internal/sync [build failed]
```

### GREEN

`go test ./internal/sync/ -run 'TestAgentsRenderDifferential|TestAgentsRenderGovernanceDifferential|TestBriefRenderPolicy|TestRenderAgentsStepPolicyGate' -count=1 -v` → PASS.

Per-state table (both legs agree byte-for-byte on `classify_state`,
`effective_state`, `action`, `wrote`, stdout/stderr, and AGENTS.md/lock sha256):

| case | classify / effective | action | wrote | AGENTS.md | lock |
| --- | --- | --- | --- | --- | --- |
| missing | missing | written | yes | rendered | baseline recorded |
| managed_stale | managed_stale | written | yes | re-rendered | baseline rewritten |
| marker | marker | preserved | no | untouched | untouched / none |
| marker + `--preserve-if-runtime-brief` | marker | preserved | no | untouched | untouched / none |
| untracked | untracked | preserved | no | untouched | none |
| untracked + `--adopt-brief` | untracked | adopted | no | untouched (user bytes) | baseline recorded |
| user_modified | user_modified | preserved | no | untouched | untouched |
| user_modified + `--adopt-brief` | user_modified | adopted | no | untouched (user bytes) | baseline recorded |
| corrupt lock | undetermined | preserved | no | untouched | corrupt bytes untouched |
| `[brief].render = false` | policy `false` | — | no | never rendered (wiring test) | — |
| non-boolean render, `--validate` | policy error rc 1 | — | — | — | — |
| unknown VCS recipe | missing | written | yes | rendered | baseline recorded (+ one stderr warning) |

### Artifact sizes / lines

- `internal/sync/agentsrender.go` — S3b section at L1049-1400
  (`runtimeBriefMarker` L1053, `briefLockPath` L1076, `briefLockKey` L1083,
  `classifyManagedOverride` L1118, `classifyBriefState` L1147,
  `briefIsOurOutput` L1173, `briefEffectiveState` L1186, `brief_ownership`
  L1199, `briefDecision` L1206, `loadResolvedConfig` L1242,
  `loadAgentsRenderInputs` L1278, `warnUnknownVCS` L1297,
  `RenderAgentsFile` L1320, `renderAgentsStep` L1388).
- `internal/sync/briefpolicy.go` — 89 lines (`EvaluateBriefRender` L41,
  `ValidateBriefRender` L57).
- `internal/sync/sync.go` — refusal loop L253, native branch L376-378.
- `internal/sync/testdata/briefgov_ref.py` — differential driver.
- `internal/sync/agentsrender_test.go` — `TestAgentsRenderGovernanceDifferential`,
  `TestRenderAgentsStepPolicyGate`.
- `internal/sync/briefpolicy_test.go` — policy semantics.
- `tests/parity/parity.py` — `_setup_adopt_brief`, `_setup_brief_render_false`
  and the two CORPUS fixtures.

### Parity

`GO_SYNC_STEP_AGENTS_RENDER=go` legacy-vs-Go on `sync-adopt-brief` and
`sync-brief-render-false` → **zero deltas** (tree, stdout, stderr, exit code).
`GO_SYNC_STEP_AGENTS_RENDER=bogus` → `ERROR: GO_SYNC_STEP_AGENTS_RENDER=bogus
is not implemented in this slice`, exit 1, no writes.

## Deviations from the task text

1. **`main()` prints nothing to stdout.** In this revision of
   `agents-render.py`, `main()` (L847-881) only parses flags and calls
   `render()`, discarding its return value; the only writer side effects are
   the `_brief_preserve` notice and the unknown-VCS warning, both on stderr.
   `RenderAgentsFile` therefore writes nothing to stdout, and the differential
   pins `render_stdout == ""` for every case.
2. **Reference driver pins the documented Python fallbacks.** The real
   `util.classify_managed_override` and `lock.write_lock` delegate to the
   worktree-gate binary when present, each emitting a degraded-run warning
   into stderr. The driver redirects both to their documented Python fallbacks
   (`_python_classify_managed_override`, `_write_lock_python`) so this slice
   pins the historical Python decision the Go state machine reproduces, and so
   the stderr comparison is clean. This is the code path util.py/lock.py keep
   as the temporary authority.
3. **`sync-adopt-brief` uses a marker-free user file.** A file carrying the
   marker classifies as `marker` and is preserved even under `--adopt-brief`
   (design D5), so it cannot exercise adoption. The fixture uses a plain
   hand-written AGENTS.md (no marker, no baseline) to drive the `adopted`
   path; marker inertness is pinned by the differential's two marker cases.
4. **`managed_current` is not a fixture state.** It is not reachable
   deterministically without knowing the rendered bytes at case-construction
   time (the lock baseline must equal the rendering). It is still exercised
   indirectly: `briefEffectiveState` promotes a self-identical untracked file.
5. **Missing / unparseable manifest.** Python raises out of `render()` with a
   traceback; the Go port prints the Go parse error and returns rc 1. The
   wired sync path never reaches the renderer with a bad manifest (the policy
   gate reads the same file first and skips), so this divergence is not
   differentially pinned.
