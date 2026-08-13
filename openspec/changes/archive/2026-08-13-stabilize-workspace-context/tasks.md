# Tasks: Stabilize explicit workspace context across runtimes

Requested depth: full

Detected signals:

- Cross-cutting change across Python adapter generation, generated TypeScript runtime
  boundaries, Bash launcher resolution, Go event parsing, parity tests, and docs.
- The original plan had unresolved context-boundary decisions and a stale runtime-docs
  statement.
- Runtime behavior needs process-boundary proof; generated-source assertions alone are
  insufficient.
- The implementation is large enough to require two review slices with an explicit
  dependency.

Decided depth: full

Depth: full

This is a planning-only task list. All implementation items remain unchecked until the
approved plan is applied. Production code, generated runtime artifacts, tests, recipes,
and configuration outside this change folder are not modified by this planning
operation.

## Approval decision record

Product and architecture choices are resolved recommendations. Runtime verification is
an implementation proof gate, not a request for another user decision.

Use this concise record format for future plan revisions:

```text
Decision ID: <stable id>
Decision: <chosen behavior>
Status: resolved recommendation
Proof: <test or validation that must demonstrate it>
Non-goal: <behavior explicitly not changed>
```

| Decision ID | Decision | Status | Proof | Non-goal |
|-------------|----------|--------|-------|----------|
| D1 | Keep event cwd, installation root, and process cwd as separate contexts | resolved recommendation | Cross-runtime and launcher tests assert each value independently | Installation root is never an event cwd |
| D2 | OpenCode trims outer whitespace, requires string plus absolute existing directory, uses one value for event and child cwd, and falls back to process cwd for both | resolved recommendation | Node process-boundary harness records event JSON and `spawnSync` options, including invalid and throw cases | No new OpenCode coverage for subagent or MCP calls |
| D3 | OpenCode, Pi, and OMP resolve launcher assets from module location via `import.meta.url` or equivalent | resolved recommendation | Relocated temporary-installation Node process tests run from unrelated cwd | No sync-time machine-specific absolute path and no relative `SCRIPT` |
| D4 | Bash derives physical installation root from `BASH_SOURCE[0]` and resolves `hooks/../bin` | resolved recommendation | Relative invocation, symlink, layout, override, and unrelated-cwd launcher fixtures | `$PWD` is not a project-local asset root |
| D5 | Go and Bash trim outer whitespace only and preserve existing fallback/fail-open policy | resolved recommendation | Direct `ParseEvent` tables and decision-differentiating Bash parity fixtures | No internal path-byte or gate-policy normalization |
| D6 | Review in two dependent slices | resolved recommendation | Each slice has an explicit exit gate and acceptance mapping | No conditional split decision remains |
| D7 | Update `docs/runtime-hooks.md:133-138` and relevant recipe docs | resolved recommendation | Documentation acceptance review against generated behavior | No claim of zero renderer change or unsupported workspace authority |

## Review workload and dependency

| Slice | Estimated lines | Scope | Dependency | Exit gate |
|-------|-----------------|-------|------------|-----------|
| Slice 1: adapters and process boundaries | 220-320 | OpenCode normalization and child cwd, module-location asset paths for OpenCode/Pi/OMP, generated-output regression tests, deterministic Node harness, preserved runtime limitations | Starts first; defines the generated invocation contract consumed by Slice 2 | Process-boundary tests are green and prove real generated-module behavior |
| Slice 2: launcher, Go/Bash parity, and docs | 250-380 | `BASH_SOURCE[0]` launcher root, relative/symlink/layout fixtures, direct Go `ParseEvent` tables, Bash decision-differentiating parity, runtime and recipe docs | Depends on Slice 1's generated invocation contract | Launcher, Go/Bash parity, docs, and full validation are green |
| Total | 470-700 | Cross-cutting change with tests and docs | Fixed two-slice plan | Both slice gates pass |

No alternative split decision remains. Slice 2 may prepare fixtures while Slice 1 is in
review, but its implementation and final acceptance consume Slice 1's absolute module-
location and child-cwd contract.

## Slice 1: adapters and process boundaries

### RED: tests first

- [x] 1.1 Add the deterministic Node harness at
      `tests/fixtures/workspace-context/opencode_process_boundary.mjs`. Load the
      generated module through the supported runtime module form and substitute a
      controlled `spawnSync` test double that records executable path, input JSON,
      options cwd, status, and thrown errors. Do not replace this with regex or source
      substring assertions.
- [x] 1.2 Extend `tests/test_hooks_render.py` to render and execute the generated
      OpenCode plugin through the Node harness. Cover valid directory with outer
      whitespace; absent, non-string, whitespace-only, relative, nonexistent, and
      non-directory values; unrelated process cwd; status `2`; child error; and child
      throw. The RED run must prove the current relative `SCRIPT`/missing child-cwd
      behavior rather than a fixture or loader failure.
- [x] 1.3 Extend the same process-boundary test strategy for Pi and OMP launcher-path
      resolution. Assert that relocated generated extensions find the launcher while
      their emitted event cwd remains `process.cwd()` and no workspace root is claimed.
- [x] 1.4 Preserve existing Claude/Cursor and target-propagation regression coverage in
      `tests/test_hooks_render.py` and `tests/test_worktree_root_propagation.py`, adding
      only assertions needed to prove the adapter change does not replace project-root
      propagation or Cursor's file-write limitation.
- [x] 1.5 Run the Slice 1 RED checks once and record failing test names and reasons. A
      loader/setup failure is not valid RED evidence; fix the harness fixture before
      implementation if it cannot observe the generated process boundary.

### GREEN: minimal implementation

- [x] 1.6 Update `render_opencode` in `lib/_internal/hooks-render.py` to normalize
      `directory` by outer trim, require string plus absolute existing directory, and use
      the same normalized value for event `cwd` and `spawnSync` `cwd`. Invalid input uses
      process cwd for both. Catch child-process errors and throws and preserve status `2`
      as the only block result.
- [x] 1.7 Replace the relative OpenCode `SCRIPT` with a runtime-supported absolute
      module-location derivation, preferring `import.meta.url` or an equivalent. The
      generated output must not embed a sync-time machine-specific absolute path.
- [x] 1.8 Apply the same module-location launcher-path stabilization to `render_pi` and
      `render_omp` where their relative `SCRIPT` has the same defect. Preserve
      `cwd: process.cwd()` in both event shapes and make no workspace-authority claim.
- [x] 1.9 Keep `render_claude` and `render_cursor` project-directory variables and
      existing event coverage unchanged.
- [x] 1.10 Run Slice 1 tests again and record GREEN evidence for each RED case before
       refactoring.

### REFACTOR: under green

- [x] 1.11 Refactor only after GREEN to keep directory normalization and module-location
       path derivation readable without merging event cwd and installation root into one
       helper. Re-run Slice 1 tests after each meaningful refactor.
- [x] 1.12 Freeze the generated invocation contract for Slice 2: absolute launcher path,
       explicit OpenCode child cwd when valid, process-cwd fallback when invalid, and
       fail-open child errors/throws.

## Slice 2: launcher, Go/Bash parity, and docs

### RED: tests and decision fixtures first

- [x] 2.1 Extend `tests/test_worktree_gate_hook.py` with real temporary installations
      and an executable marker implementation. Invoke the Bash launcher through a
      relative path, a symlink, and an unrelated process cwd. Assert physical
      `BASH_SOURCE[0]` root resolution, `hooks/../bin`, project-local selection, and
      legacy fallback. Add cases for `WORKTREE_GATE_BIN`, cache precedence, and missing
      root without allowing `$PWD` to become a project root.
- [x] 2.2 Add the direct Go table tests in the deliberately new file
      `catalog/recipes/worktree-flow/gate/event_cwd_test.go`. There is no existing
      `event_test.go` assumption. `TestParseEventCwdNormalization` must call
      `ParseEvent` directly and cover path and shell events with trimmed valid cwd,
      whitespace-only, relative, nonexistent, non-directory, non-string, and process-cwd
      fallback cases.
- [x] 2.3 Extend `tests/test_worktree_gate_parity.py` with Bash decision-differentiating
      fixtures: process cwd is an allowing feature or linked-worktree context while the
      event cwd is outer-whitespace-wrapped protected main-checkout path. Assert both
      Bash and Go block after trimming. Add invalid-cwd fallback cases and compare path
      and shell events.
- [x] 2.4 Add a documentation regression checklist for `docs/runtime-hooks.md:133-138`
      and the relevant worktree-flow recipe documentation so the stale zero-renderer-
      change claim cannot survive the implementation.
- [x] 2.5 Run Slice 2 RED checks and confirm each failure is behavioral: launcher root,
      direct `ParseEvent` normalization, Bash/Go decision parity, or stale documentation.

### GREEN: minimal implementation

- [x] 2.6 Update `_resolve_binary` and the legacy fallback in
      `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` to derive the physical
      launcher path from `BASH_SOURCE[0]`, handle relative invocation, follow symlinks,
      compute `recipe_root = launcher_dir/..`, and resolve `hooks/../bin`. Keep Bash 3.2
      syntax and the existing explicit override, cache, legacy, stamped-value, `exec`,
      stdin, exit-code, and fail-open behavior.
- [x] 2.7 Ensure an unresolved `BASH_SOURCE[0]` installation root skips project-local
      and legacy lookup instead of guessing from `$PWD`; continue through preserved
      explicit/cache candidates or exit `0` with a diagnostic.
- [x] 2.8 Update `eventCwd` in `catalog/recipes/worktree-flow/gate/event.go` to trim
      outer whitespace only before the absolute-existing-directory check. Preserve
      internal path bytes, process-cwd fallback, path/shell event behavior, and existing
      gate policy.
- [x] 2.9 Pin the legacy Bash outer trim and make a Bash source edit only if the RED
      parity fixtures prove it is required. If existing behavior is already conformant,
      record that no Bash normalization source change was needed.
- [x] 2.10 Update `docs/runtime-hooks.md:133-138` with the three-context model, module-
       derived adapter paths, Bash launcher-root behavior, OpenCode validation/fallback,
       and unchanged Cursor/OpenCode-subagent-MCP/Pi/OMP limitations. Update the relevant
       `catalog/recipes/worktree-flow/README.md` launcher guidance.
- [x] 2.11 Run Slice 2 tests again and record GREEN evidence before refactoring.

### REFACTOR: under green

- [x] 2.12 Refactor only after GREEN to keep launcher root derivation, event normalization,
       and parity fixture setup explicit. Re-run Slice 2 tests after each meaningful
       refactor and preserve the fixed precedence table.
- [x] 2.13 Review the complete generated output for all five runtimes and confirm no
       generated runtime artifact is hand-edited as a source of truth.

## Cross-slice validation and acceptance

- [x] 3.1 Run the Slice 1 focused tests, Slice 2 focused tests, and record exact RED/GREEN
      evidence in the implementation verification record.
- [x] 3.2 Run Go tests with the repository's actual module command:
      `go -C catalog/recipes/worktree-flow/gate test ./...`.
- [x] 3.3 Run syntax checks for changed boundaries, including
      `python3 -m py_compile lib/_internal/hooks-render.py` and
      `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh` plus the legacy
      script if it changes.
- [x] 3.4 Run the repository smoke validation command `./tests/validate.sh` from the
      worktree root. Do not claim success unless the command is actually run.
- [x] 3.5 Confirm existing target propagation remains green and that the detached Gentle
      AI candidate-view worktree and unrelated main-worktree `ai-specs/ai-specs.toml`
      edit were not modified.
- [x] 3.6 Review the final implementation diff for scope: only approved Slice 1 and
      Slice 2 implementation paths, tests, and docs may change after authorization; this
      planning package remains under `openspec/changes/stabilize-workspace-context/`.

## Acceptance mapping

| Acceptance ID | Spec requirement | Slice | Evidence and dependencies |
|---------------|------------------|-------|---------------------------|
| A1 | Launcher assets are independent of process cwd | 2 | Tasks 2.1, 2.6-2.7; relative, symlink, `hooks/../bin`, override, cache, and legacy fixtures |
| A2 | Generated adapters use runtime-supported absolute asset paths | 1 | Tasks 1.1-1.3, 1.6-1.8; Node process harness observes executable path from module location |
| A3 | OpenCode preserves explicit directory execution context | 1 | Tasks 1.1-1.2, 1.6-1.7; one normalized value observed in event JSON and `spawnSync` options |
| A4 | Go and Bash normalize event cwd identically | 2 | Tasks 2.2-2.3, 2.8-2.9; direct `ParseEvent` tables and decision-differentiating parity fixtures |
| A5 | Resolution and context failures remain fail-open | 1 and 2 | Tasks 1.2, 1.6, 2.1, 2.6-2.7; child throw/error, missing root, missing asset, and ambiguity cases |
| A6 | Real harness limitations remain explicit | 1 and 2 | Tasks 1.3-1.4, 1.8-1.9, 2.10; Pi/OMP process cwd, Cursor gap, OpenCode subagent/MCP gap |
| A7 | Existing target propagation and launcher compatibility remain stable | 1 and 2 | Tasks 1.4, 1.9, 3.5; existing propagation, filename, stdin, exit, and stamped-value assertions |
| A8 | Runtime docs reflect final behavior | 2 | Tasks 2.4, 2.10; explicit `docs/runtime-hooks.md:133-138` acceptance review |

## Validation command summary

Commands are listed for implementation and verification only; none are run as part of
this planning-only revision:

```text
go -C catalog/recipes/worktree-flow/gate test ./...
python3 -m py_compile lib/_internal/hooks-render.py
bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh
./tests/validate.sh
```
