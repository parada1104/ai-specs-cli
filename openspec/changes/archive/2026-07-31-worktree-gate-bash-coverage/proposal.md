# Proposal: worktree-gate bash-bypass coverage

## Intent

The worktree-flow **worktree-gate** is meant to enforce: *no file writes on a
protected branch (`main` / `development`) in the main worktree without a
dedicated worktree*. Today that guarantee only covers structured file tools
(`Edit|Write|MultiEdit|NotebookEdit`). Shell is completely outside the MATCHER.

Live session evidence (not speculation): a subagent’s structured write failed
for an **unrelated** MCP path-allowlist error; the agent then fell back to
`bash(python3 <<'PY' … Path(…).write_text(…) PY)` and **succeeded** — the gate
never ran because `bash` is not matched. That voids the core safety contract
whenever any agent treats shell as a write channel (fallback after a blocked or
failed Edit/Write, or a deliberate bypass).

This change closes the dominant in-process bash-write hole with a **hybrid**:

1. **Technical (pre-tool):** teach `worktree-gate.sh` best-effort command-string
   write detection and wire shell tools into harnesses that can intercept them.
2. **Policy:** reinforce SKILL + always-on brief so agents never treat
   bash/shell as a legitimate alternate I/O path when structured writes fail.

Coverage is intentionally **best-effort and uneven by harness** — document that
honestly rather than claim a uniform sandbox.

## Scope

### In scope

1. **Dual-input gate script** — extend
   `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` so that:
   - Existing path-based path stays unchanged:
     `tool_input.file_path` / `notebook_path` (and shim-normalized `path`) →
     current main-worktree + protected-branch logic.
   - When no structured path is present, accept a shell **command string** from
     `tool_input` (`command`, and any common aliases design confirms) and run
     **heuristic** write-redirection / writer detection (see Approach).
   - On a **plausible match** whose candidate path resolves inside the protected
     main worktree → `exit 2` with a message that names the bash-bypass risk and
     points to `/worktree-new`.
   - On **ambiguous / unparseable / no confident write target** → `exit 0`
     (fail-open). Preserve today’s philosophy exactly: a buggy or incomplete
     guard must never wedge all shell use.
   - `gate_mode` (`always` / `ask` / `off`) and
     `WORKTREE_GATE_PROTECTED` / `WORKTREE_GATE_MODE` stay authoritative for
     shell blocks the same way they do for path blocks (`off` still early-exits;
     `ask` adds the documented one-shot bypass hint).

2. **Harness wiring (honest, not uniform)** — keep one decision brain
   (`worktree-gate.sh`); change only how MATCHER / hook ids reach each renderer:
   - **Preferred shape:** keep the existing file-write hook
     (`id = "worktree-gate"`, matcher `Edit|Write|MultiEdit|NotebookEdit`) **and**
     add a sibling `[[provides.hooks]]` entry (e.g. `id = "worktree-gate-shell"`)
     that shares the **same script** with a shell-oriented matcher
     (`Bash` plus confirmed aliases). Rationale grounded in generator code:
     `hooks-render.py` `_matcher_targets_file_writes` returns true if **any**
     matcher token is in `{Edit, Write, MultiEdit, NotebookEdit}`, and
     `render_cursor` then **skips the entire hook**. A combined
     `Edit|…|Bash` matcher is therefore still skipped on Cursor. A Bash-only
     second hook is the path that actually emits
     `.cursor/hooks/worktree-flow-worktree-gate-shell.sh` +
     `beforeShellExecution` without a renderer schema change.
   - **claude:** second (or extended) `PreToolUse` managed entry; native matcher
     already proven with fixture `matcher = "Bash"` in `tests/test_hooks_render.py`.
   - **omp / pi:** generated extensions already case-insensitive-match tool
     names and spread `rawInput` into `tool_input` (so `command` passes through
     even while they also normalize `path` → `file_path`). Extending/adding the
     shell matcher is sufficient for in-process Bash/shell tool calls; no hand
     edits to generated `.omp/extensions/*.ts` / `.pi/extensions/*.ts`.
   - **opencode:** same matcher fan-out on primary-agent `tool.execute.before`
     (comments already mention Claude-style `Bash`). Subagent (#5894) and MCP
     (#2319) still do not fire — pre-existing, document only.
   - **cursor:** shell hook only (file-write gate remains impossible — no
     pre-file-write API). Wrapper still maps script `exit 2` →
     `{"permission":"deny",…}`. Design MUST confirm Cursor’s native
     `beforeShellExecution` payload shape vs the normalized
     `{tool_name, tool_input.command, cwd}` contract (today’s Cursor wrapper
     pipes stdin through unchanged) and either teach the script to accept both
     shapes or normalize in the wrapper — without weakening fail-open.
   - **Renderer change** to mixed-matcher Cursor behavior is **not required** if
     the dual-hook approach lands; only reopen if design finds dual-hook drift
     unacceptable.

3. **Anti-fallback policy** — add an explicit rule to
   `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` and
   `[provides.brief].workflow_rules` in `recipe.toml`:
   - If a structured Edit/Write/MultiEdit/NotebookEdit call is **blocked by the
     gate** **or errors for any unrelated reason** while on a protected branch in
     the main worktree, the correct response is to **create a worktree**
     (`/worktree-new`) and continue there.
   - It is **never** license to retry the write via bash/shell/python/node/ruby
     one-liners, heredocs, or redirections.

4. **Docs + specs + tests**
   - `docs/runtime-hooks.md`: document shell gating pattern, dual-hook Cursor
     reason, residual heuristic/process gaps (do not overclaim).
   - Recipe README + `docs/recipes-catalog.md` worktree-flow blurb if they imply
     gate scope is file-tools-only or absolute.
   - Delta specs under this change for `worktree-flow` (shell best-effort
     pre-tool + anti-fallback brief/skill). Touch
     `runtime-hook-distribution` **only if** renderer behavior changes.
   - TDD: extend `tests/test_worktree_gate_hook.py` with command-string
     block/allow/fail-open cases; extend `tests/test_hooks_render.py` /
     sync-pipeline fixtures for the shell hook id and Cursor wiring.
   - Recipe version bump on `worktree-flow`.

### Out of scope

- **Post-hoc revert / dirty-tree auto-rollback** after shell runs — conflicts
  with “never revert changes you did not make” and is too late for gate
  semantics. Optional soft warn/telemetry is not v1.
- **Fully general shell-command parser or OS sandbox** — heuristics only;
  documented false-negative residual risk is accepted.
- **`worktree-flow-repo-topology`** and any other independent change branch /
  worktree — separate scope; do not couple.
- **plan-build-gate** same MATCHER hole — track as a follow-up unless design
  explicitly expands (same script pattern, different recipe).
- Closing **OpenCode subagent/MCP** or **pi/omp child-process** hook gaps —
  platform limits already documented; brief pre-delegation rule remains the
  mitigation.
- Inventing a new abstract event type — `pre-tool-use` already maps to Cursor
  `beforeShellExecution` for non-file-write matchers.

## Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| `worktree-flow` / `worktree-isolation` | **Modified** | Gate contract expands from structured file tools only to best-effort in-process shell write detection + anti-fallback policy text |
| `runtime-hook-distribution` | **Unchanged (default)** | Dual `[[provides.hooks]]` already supported; Cursor skip rules stay. Delta only if design changes `_matcher_targets_file_writes` / mixed matchers |

## Approach

### 1. Single script, two input modes

Keep the normalized stdin contract and exit codes (`0` allow, `2` block, other
fail-open). Parsing order:

1. Resolve `gate_mode`; `off` → exit 0 (unchanged).
2. Parse JSON; on parse failure → exit 0.
3. If `file_path` / `notebook_path` (after shim normalization) is non-empty →
   **existing** path gate path (unchanged semantics, including
   `.claude/settings*` allowlist).
4. Else extract shell command string from `tool_input` (primary key `command`;
   design locks any harness-specific aliases / Cursor top-level fields).
5. If no command → exit 0 (allow; preserves empty-path fail-open).
6. Run **v1 high-precision heuristics** against the command text + `cwd` to
   collect candidate write paths. If none confident → exit 0.
7. For each confident candidate, resolve against `cwd`, then apply the **same**
   git main-worktree + protected-branch check used for structured paths.
8. On hit → stderr guidance (bash-bypass + `/worktree-new`; `ask` bypass hint)
   and exit 2.

### 2. Locked v1 heuristic pattern set (best-effort)

Match only patterns with high write intent; do not aim for completeness:

| Pattern class | Examples (illustrative) | Notes |
|---------------|-------------------------|--------|
| Shell redirection | `>`, `>>` targeting a path token | Ignore obvious non-paths / pure `/dev/null` if easy; fail-open if path token unclear |
| `tee` | `tee path`, `tee -a path` | |
| In-place editors | `sed -i`, `perl -i` | |
| Copy/move into tree | `cp` / `mv` with a destination path under the repo | Source-only reads allow |
| Interpreter write APIs in `-c` / heredoc bodies | `python3 -c`, `python3 <<…` containing `write_text` / `open(..., "w"|"a"|"x")` (and close variants); analogous obvious `node -e` / `ruby -e` only if cheap and high-precision | The live exploit class |

**Explicit non-goals for v1 heuristics:** `eval`-obfuscated payloads, multi-stage
pipelines that hide the path, encoded blobs, arbitrary `$EDITOR` sessions, `dd`,
and creative indirection. Those remain fail-open false negatives by design.

**False-positive control:** only block when a candidate path resolves to the
**main** worktree **and** branch ∈ protected set. Linked worktrees always allow.
Quotes that look like `echo 'a > b'` should fail-open when path extraction is
not confident.

### 3. Recipe dual-hook distribution

```toml
[[provides.hooks]]
id = "worktree-gate"
# unchanged matcher — file tools (Cursor still skips this one)

[[provides.hooks]]
id = "worktree-gate-shell"
event = "pre-tool-use"
script = "hooks/worktree-gate.sh"   # same script
matcher = "Bash"                    # + aliases design confirms (bash|shell|Shell|…)
blocking = true
description = "Best-effort block of shell writes to the main worktree on a protected branch"
```

Both hooks share env stamping (`WORKTREE_GATE_PROTECTED`, gate_mode stamp).
Sync materializes one script file; two harness shims/entries point at it.

### 4. Per-harness reality (do not overclaim)

| Harness | Structured Edit/Write gate | Shell write gate after this change | Residual gaps |
|---------|----------------------------|------------------------------------|---------------|
| **claude** | Yes (`PreToolUse`) | Yes (`PreToolUse` + Bash matcher) | Heuristic false negatives |
| **omp** | Yes (`tool_call`, this process) | Yes (`tool_call` + shell matcher, this process) | Child/subagent processes; heuristics |
| **pi** | Yes (this process) | Yes (this process) | Same as omp |
| **opencode** | Yes (primary agent) | Yes (primary agent shell tools) | Subagent + MCP tools never fire pre-hooks; heuristics |
| **cursor** | **No** (no pre-file-write API; file-write matcher skipped) | **Yes** via separate Bash-only hook → `beforeShellExecution` | File writes still ungated at hook layer; payload normalization; heuristics |

Policy/SKILL/brief reinforcement applies on **all** harnesses regardless of hook
fidelity.

### 5. Anti-fallback policy text

SKILL + `workflow_rules` gain one crisp rule (wording polished in apply):

> If Edit/Write/MultiEdit is blocked **or fails for any reason** on a protected
> branch in the main worktree, create a dedicated worktree and continue there.
> Never retry the write through bash/shell/python/heredoc/redirection.

This complements the existing pre-delegation brief rule; it does not replace
hooks.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Modified | Dual path/command input; shell heuristics; shell-specific stderr |
| `catalog/recipes/worktree-flow/recipe.toml` | Modified | Sibling shell hook; brief anti-fallback rule; version bump |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Modified | Anti-fallback + shell coverage honesty |
| `catalog/recipes/worktree-flow/README.md` | Modified | Shell coverage table + residual gaps |
| `docs/runtime-hooks.md` | Modified | Shell gating pattern; dual-hook Cursor rationale; heuristic limits |
| `docs/recipes-catalog.md` | Modified | worktree-flow blurb if gate scope is described |
| `openspec/specs/worktree-flow/spec.md` | Delta (via change) | Shell best-effort pre-tool + anti-fallback requirements |
| `openspec/specs/runtime-hook-distribution/spec.md` | Delta only if needed | Only if renderer mixed-matcher / Cursor normalize behavior changes |
| `lib/_internal/hooks-render.py` | Unchanged (default) | Dual-hook approach avoids mixed-matcher fix; reopen only if design requires Cursor payload normalize in wrapper |
| `tests/test_worktree_gate_hook.py` | Modified | Command-string block / allow / fail-open cases |
| `tests/test_hooks_render.py` | Modified | Shell hook id wires Cursor `beforeShellExecution`; file-write hook still skipped on Cursor |
| `tests/test_worktree_flow_recipe.py`, `test_sync_pipeline.py`, `test_recipes_catalog.py` | Modified as needed | Matcher/hook-id/docs expectations |
| Generated `.omp/extensions/…`, `.pi/…`, `.opencode/…`, `.claude/settings.json`, `.cursor/hooks/*` | Via `ai-specs sync` only | Never hand-edit |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Docs/agents overclaim “bash is fully gated” → false security | Medium | Explicit residual-gap table in README + `runtime-hooks.md`; success criteria require honest language |
| Heuristic false negatives (obfuscated writers) | High (by design) | Fail-open + anti-fallback policy; document intentional misses; do not promise sandbox |
| Heuristic false positives block legitimate protected-branch shell | Low–Medium | High-precision pattern list; require resolvable path in main+protected; fail-open on ambiguity; `gate_mode=off` / env override remain |
| Cursor payload shape ≠ normalized event → shell hook no-ops or mis-parses | Medium | Design/apply must verify with real Cursor payload or fixture; normalize or dual-parse; tests cover both shapes if both accepted |
| Dual-hook matcher drift (file vs shell env/stamp diverge) | Low | Same script path + same env keys; single materialization; tests assert both hook ids share script |
| Matcher-only change without script command parsing → **zero effect** | Medium if rushed | Success criteria require command-string tests that block the live exploit class |
| Extending primary matcher only (no second hook) → Cursor still skipped | High if dual-hook skipped | Lock dual-hook (or equivalent renderer split) in design/tasks |

## Rollback Plan

1. Revert recipe.toml shell hook entry and brief rule; restore prior matcher-only
   file-write hook and SKILL text.
2. Revert `worktree-gate.sh` to path-only extraction (or ship a version bump that
   drops command heuristics).
3. Revert docs/spec deltas and new tests.
4. Run `ai-specs sync` so generated shims drop the shell hook managed ids.
5. No data migration; fail-open means partial deploy never wedges editors.
   Projects that never re-sync keep old behavior until sync.

## Dependencies

- Existing `worktree-flow` recipe (gate_mode, protected branches, runtime hook
  distribution pipeline).
- `lib/_internal/hooks-render.py` EVENT_MAP and per-harness adapters (claude /
  cursor / opencode / pi / omp) as of current tree — dual `[[provides.hooks]]`
  already supported; Cursor skip for file-write tokens unchanged.
- Host agents surface hook stderr / block reasons (already required by the
  existing gate).
- Does **not** depend on `worktree-flow-repo-topology` landing first.

## Success Criteria

- [ ] Structured Edit/Write/MultiEdit/NotebookEdit gating behaves **exactly** as
      today for path-based events (including `gate_mode`, allowlists, linked
      worktree allow, fail-open on bad JSON).
- [ ] Shell command stdin events with high-confidence write patterns targeting a
      path in the protected main worktree → `exit 2` and a message that mentions
      bash-bypass risk and `/worktree-new` (or equivalent worktree creation
      guidance).
- [ ] Ambiguous / non-write / unparseable shell commands → `exit 0` (fail-open).
- [ ] Non-write shell on protected main worktree (e.g. `git status`, `ls`) →
      allow.
- [ ] Write-shaped shell inside a **linked** worktree → allow.
- [ ] `gate_mode=off` still disables path **and** shell gating; `ask` surfaces
      the same class of bypass hint on shell blocks.
- [ ] Recipe declares a shell-oriented hook (or equivalent) such that after sync:
      - **claude / omp / pi / opencode** invoke the gate for in-process shell
        tool names matched by the shell matcher;
      - **cursor** emits a `beforeShellExecution` wrapper for the shell hook and
        still skips the file-write hook.
- [ ] SKILL.md and brief `workflow_rules` contain the anti-fallback rule
      (blocked **or** unrelated structured-write error → worktree, never bash
      retry).
- [ ] Docs state residual gaps: heuristics incomplete; OpenCode subagent/MCP;
      pi/omp child processes; Cursor still has no pre-file-write gate.
- [ ] Unit/integration tests cover the live exploit class
      (`python3` heredoc / `-c` `write_text` or equivalent) plus fail-open and
      non-write allow cases; renderer/sync tests cover dual-hook Cursor wiring.
- [ ] No post-hoc revert logic ships.
- [ ] `./tests/validate.sh` passes.

## Proposal assumptions (locked for design)

1. Hybrid **A + C** from explore: script heuristics + matcher/hook wiring +
   policy text; **B (post-hoc revert) rejected**.
2. **Dual hook sharing one script** is the default distribution shape (Cursor-
   correct without renderer mixed-matcher changes).
3. Heuristics are **best-effort, fail-open**, high-precision pattern list above.
4. plan-build-gate bash hole is a **follow-up**, not this change.
5. Generated harness shims are sync-only artifacts.

## Proposal question round (optional product check)

These do not block writing design if parent keeps the locks above; surface only
if product wants a second pass:

1. Should v1 shell matcher include only `Bash`, or also `shell` / `Shell` /
   `Execute` / harness-specific names observed in 2026 tool catalogs?
2. Is empty-matcher Cursor `beforeShellExecution` (all shell) acceptable, or
   must the shell hook stay name-filtered on harnesses that support matchers
   (Cursor event model ignores tool-name matcher in the wrapper today)?
3. Any customer workflow that **must** shell-write on protected main (e.g.
   release scripts) beyond `gate_mode=off` / env override?
4. Should `ask` mode copy for shell blocks differ from path blocks (same
   bypass env vs stronger “create worktree” only)?

## Artifact path

`openspec/changes/worktree-gate-bash-coverage/proposal.md`
