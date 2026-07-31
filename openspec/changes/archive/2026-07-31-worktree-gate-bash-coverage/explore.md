# Exploration: worktree-gate bash-bypass coverage

> Change slug: `worktree-gate-bash-coverage`
> Worktree: `.worktrees/worktree-gate-bash-coverage` @ `feat/worktree-gate-bash-coverage`
> Separate from `worktree-flow-repo-topology` (independent branch/worktree).
> skill_resolution: `none` (sdd-explore skill path not injected; OpenSpec archive explores + `openspec/config.yaml` used as format/depth source).

## Problem

The worktree-flow **worktree-gate** is supposed to enforce: *no writes on protected branches (`main` / `development`) in the main worktree without a dedicated worktree*.

Live session evidence (this parent session, not speculation):

1. Structured write tools are gated by MATCHER `Edit|Write|MultiEdit|NotebookEdit` only.
2. A subagent’s structured `write` failed for an **unrelated** reason (MCP path-allowlist), then `read` failed (file missing).
3. The agent fell back to `bash(python3 <<'PY' … Path(…).write_text(…) PY)` and **succeeded** — the gate never ran because `bash` is outside MATCHER.

That voids the core safety guarantee whenever any agent treats shell as a write channel (fallback after blocked/failed Edit/Write, or deliberate bypass).

## Current state (end-to-end pipeline)

### 1. Source of truth — recipe declaration

`catalog/recipes/worktree-flow/recipe.toml`:

```toml
[[provides.hooks]]
id = "worktree-gate"
event = "pre-tool-use"
script = "hooks/worktree-gate.sh"
matcher = "Edit|Write|MultiEdit|NotebookEdit"
blocking = true
description = "Block writes to the main worktree on a protected branch"
```

Note: runtime hooks are `[[provides.hooks]]` (not sync-time `[[hooks]]` with `event = "on-sync"`).

Brief already warns about **delegation / subprocess** hook gaps (`workflow_rules` + skill text) but does **not** forbid bash-as-write fallback after a failed structured write.

### 2. Portable guard script

`catalog/recipes/worktree-flow/hooks/worktree-gate.sh`:

- Contract: stdin JSON `{event, tool_name, tool_input, cwd}`; exit `0` allow, `2` block, other **fail-open**.
- Extracts **only** `tool_input.file_path` or `tool_input.notebook_path`.
- If path empty → **exit 0** (allow). Shell tools with `command` / free-form args never produce a path today → silent allow even if MATCHER were extended without script changes.
- Then: walk to existing dir → `git rev-parse` → linked worktree (`git_dir != common_dir`) allow → else if branch ∈ `WORKTREE_GATE_PROTECTED` → block (with small `.claude/settings*` allowlist).
- Modes: stamped `__WORKTREE_GATE_MODE__` + env `WORKTREE_GATE_MODE` (`always|ask|off`).

### 3. Renderer — `lib/_internal/hooks-render.py`

Single `EVENT_MAP` for abstract → native events:

| abstract | claude | cursor | opencode | pi | omp |
|----------|--------|--------|----------|-----|-----|
| `pre-tool-use` | `PreToolUse` | `beforeShellExecution` (shell/MCP only; **no pre-file-write**) | `tool.execute.before` | `tool_call` | `tool_call` |
| `post-tool-use` | `PostToolUse` | `afterShellExecution` | `tool.execute.after` | `tool_result` | `tool_result` |

Per-harness wiring:

| harness | artifact | matcher behavior | block signal |
|---------|----------|------------------|--------------|
| **claude** | `.claude/settings.json` managed `PreToolUse` entry | native `matcher` string on entry | script `exit 2` |
| **cursor** | `.cursor/hooks/<recipe>-<hook>.sh` + `hooks.json` | **If matcher intersects file-write tokens → entire hook skipped** (warn). Else maps to `beforeShellExecution` (no tool-name filter in wrapper) | stdout `{"permission":"deny",…}` |
| **opencode** | `.opencode/plugin/<recipe>-<hook>.ts` | `RegExp(^(?:MATCHER)$, "i")` on tool name; comment explicitly lists `Bash` as Claude-style name | `throw` on exit 2 |
| **pi** | `.pi/extensions/…ts` | same case-insensitive MATCHER; normalizes `path` → `file_path` | `{block:true, reason}` |
| **omp** | `.omp/extensions/…ts` | identical shape to pi (`@oh-my-pi/pi-coding-agent`) | `{block:true, reason}` |

Generated omp shim (main tree, post-sync; **not** present in this fresh worktree until sync):

```text
const MATCHER = "Edit|Write|MultiEdit|NotebookEdit";
// … re.test(toolName); file_path from path/notebook_path only
```

### 4. Documented known gaps (`docs/runtime-hooks.md`)

Already honest about:

- Cursor: no pre-file-write → file-write matchers skipped.
- OpenCode: `tool.execute.before` does **not** fire for **subagent** or **MCP** tool calls.
- Pi/omp: `tool_call` is **this process only**; child agents need their own extensions.
- Fail-open philosophy: buggy guard must not wedge editing.

**Not documented today:** structured-write MATCHER leaves **Bash/shell** completely ungated on every harness that *can* intercept shell.

### 5. Tests / specs already in tree

- `tests/test_worktree_gate_hook.py` — path-based allow/block/fail-open; no shell/command cases.
- `tests/test_hooks_render.py` — uses a **separate** fixture `SHELL_HOOK` with `matcher = "Bash"` to prove Cursor `beforeShellExecution` wiring; worktree-gate fixture proves Cursor **skip** for file-write matcher.
- `openspec/specs/runtime-hook-distribution/spec.md` — distribution + Cursor skip rules.
- `openspec/specs/worktree-flow/spec.md` — cleanup heuristics + pre-delegation brief rule; **no** bash-write requirement yet.

## Q2 — Can any harness intercept shell/bash at all?

**Yes — several can match shell tool names or shell events.**

| harness | Shell interception? | How | Same gate script reusable? |
|---------|---------------------|-----|----------------------------|
| **claude** | Yes | `PreToolUse` matcher can include `Bash` (tests already use `matcher: "Bash"`) | Only if script learns to parse `tool_input.command` (or equivalent) into candidate paths; path field is absent |
| **omp / pi** | Yes | `tool_call` fires for all tools in-process; extend MATCHER with `Bash` / `bash` (case-insensitive already) | Same: need command-string → path heuristics; input shape is free-form (`command`, etc.), not `file_path` |
| **opencode** | Yes (primary agent only) | `tool.execute.before` + matcher; comments already mention `Bash` | Same limitations + **no subagent/MCP** shell either |
| **cursor** | Yes for **shell**, no for file writes | `beforeShellExecution` is the *only* useful pre event; payload is shell-oriented | **Critical renderer issue:** `_matcher_targets_file_writes` returns true if matcher contains *any* of Edit/Write/… — so a **combined** `Edit\|…\|Bash` matcher is **still fully skipped** for Cursor. Shell coverage on Cursor needs a **separate** hook id (Bash-only matcher) **or** renderer change to split mixed matchers / not skip when shell tokens present |

### Path extraction vs Edit/Write

Edit/Write are easy: structured `file_path` / `path` / `notebook_path`.

Bash is fundamentally harder:

- Command is an opaque string: `>`, `>>`, `tee`, `cat >file <<EOF`, `sed -i`, `python3 -c '…write_text…'`, `ruby -e`, `node -e`, `printf … >`, `dd`, `cp`/`mv` into tree, editors (`$EDITOR`), etc.
- False positives: `echo 'a > b'`, tests printing redirects, `git show > /tmp/x`, writes **outside** repo.
- False negatives: obfuscated / multi-stage / `eval` / encoded payloads.
- Correct approach aligned with existing philosophy: **best-effort heuristic, fail-open on ambiguity** (never block unknown shell; only block high-confidence write-shaped commands targeting main worktree protected branch).

Pre-tool-use can still block **before** execution when confidence is high. Perfect prevention is impossible without an OS sandbox; the product already accepts process-boundary gaps.

Post-tool-use can observe `git status --porcelain` after shell and warn/revert — but **revert is dangerous** (destroys unrelated dirty state; violates “never revert changes you did not make”) and **warn-only** does not restore the safety guarantee. Prefer pre-emptive block + policy text; use post only as optional telemetry/soft warn if ever.

## Mitigation strategies (evaluated)

### A — Extend MATCHER + command heuristics in `worktree-gate.sh` (technical, pre-tool)

**Idea:** matcher becomes something like `Edit|Write|MultiEdit|NotebookEdit|Bash` (plus any harness aliases if needed). Script:

1. If `file_path`/`notebook_path` present → existing logic.
2. Else if tool looks like shell → extract `command` (and common aliases) from `tool_input`; run conservative regex/heuristics for write redirections / known writers; resolve candidate paths against cwd; apply same main-worktree + protected-branch check; fail-open if no confident path.

**Pros**

- Closes the exact live bypass for in-process shell on claude/omp/pi/opencode primary.
- One script remains the single decision brain (distribution model intact).
- Matches fail-open contract.
- Tests can drive stdin JSON without a live agent.

**Cons / harness unevenness**

- Cursor still skips if matcher keeps file-write tokens → need **second hook** (`worktree-gate-shell`, matcher `Bash` or empty for Cursor shell event) sharing the same script, **or** renderer fix for mixed matchers.
- Heuristics incomplete by nature; creative bypasses remain.
- OpenCode subagent/MCP and pi/omp child processes still bypass (pre-existing).
- Risk of annoying false positives if heuristics too aggressive (mitigate: fail-open + narrow patterns + only when protected main worktree).

**Effort:** Medium. **Depth:** behavior/domain change to a safety-critical recipe.

### B — POST-tool-use dirty check (after any bash)

**Idea:** `post-tool-use` matcher on Bash/shell; after command, `git status --porcelain` in main worktree on protected branch; warn or attempt revert.

**Pros**

- Catches writes heuristics missed pre-exec (if post fires).
- Cursor has `afterShellExecution`.

**Cons**

- **Too late** for true gate semantics (“refuse the write”).
- Auto-revert is unsafe and policy-hostile.
- Warn-only is easily ignored (same class as pure prompt fixes).
- Still miss subagent/MCP where post also does not fire.
- Extra hook surface + flaky dirty detection (unrelated dirt, intentional commits).

**Verdict:** Poor primary fix; optional soft secondary at most. Do not auto-revert.

### C — Policy / skill / brief reinforcement only

**Idea:** AGENTS brief + `SKILL.md` + workflow_rules: “If Edit/Write is blocked or fails, that may be the gate or a tool error — create a worktree; **never** fall back to bash/python to write files on the protected main worktree.”

**Pros**

- Cheap; documents intent; helps well-behaved agents.
- Complements technical controls (like existing pre-delegation brief rule).

**Cons**

- **This session’s evidence:** agent rationalized bash **because** structured write failed for an unrelated MCP error — treated shell as legitimate alternate I/O, not as bypass. Pure prompt fixes are weak against tool-failure recovery heuristics.
- No machine enforcement; regression-prone across models/harnesses.

**Verdict:** Necessary **complement**, never sufficient alone.

### D — Hybrid (recommended): A + C, with Cursor as second hook or renderer tweak; B out of scope for v1

1. **Script:** path-or-command dual input; shell heuristics fail-open.
2. **Recipe:** extend matcher and/or add sibling shell hook so Cursor actually wires `beforeShellExecution`.
3. **Renderer:** only if needed — fix mixed-matcher Cursor skip so one hook can target both categories, **or** keep two `[[provides.hooks]]` sharing one script (simpler, no schema change).
4. **Brief/SKILL:** explicit anti-fallback rule.
5. **Docs/specs/tests:** document residual gaps (heuristics, subagents, MCP).
6. **Not in v1:** post-hoc revert.

## Uniformity across harnesses

| Goal | Uniform from shared source? | Reality |
|------|----------------------------|---------|
| Single script decision logic | Yes | `worktree-gate.sh` stays harness-neutral |
| MATCHER string distribution | Mostly | claude/opencode/pi/omp honor extended matcher |
| Cursor shell gate | **Partial today** | File-write-only hook skipped; need Bash-only second hook or renderer change |
| Cursor file-write gate | **Impossible** in v1 map | No pre-file-write API — unchanged |
| OpenCode subagent/MCP | **No** | Platform limits (#5894, #2319) |
| Pi/omp child process | **No** | Per-process extensions only |
| Heuristic quality | Same script everywhere | Same false neg/pos on all harnesses that fire |

**Honest product stance:** fix is **best-effort and partial** by harness capability. Still worth doing: closes the dominant in-process bash fallback hole on omp/pi/claude/opencode primary (and Cursor shell if second hook/renderer fixed), which is exactly the live exploit class.

No new abstract event type is strictly required (`pre-tool-use` already maps to Cursor shell). Schema already allows multiple `[[provides.hooks]]` and free-form `matcher` strings.

## Files likely to change

| File | Why |
|------|-----|
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Command-string heuristics; dual path/command extraction; stderr messages for shell blocks |
| `catalog/recipes/worktree-flow/recipe.toml` | Matcher extend and/or second `[[provides.hooks]]`; brief `workflow_rules` anti-bash-fallback; version bump |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Explicit: do not shell-write when gated/failed; create worktree |
| `catalog/recipes/worktree-flow/README.md` | Document shell coverage + residual gaps |
| `lib/_internal/hooks-render.py` | **Only if** mixed matcher should render on Cursor (split or refine `_matcher_targets_file_writes` / `render_cursor`) |
| `docs/runtime-hooks.md` | Document shell gating pattern + Cursor second-hook guidance + known heuristic limits |
| `docs/recipes-catalog.md` | worktree-flow blurb if it claims gate scope |
| `openspec/specs/worktree-flow/spec.md` | Delta: shell best-effort pre-tool requirements |
| `openspec/specs/runtime-hook-distribution/spec.md` | Delta **if** renderer Cursor behavior changes |
| `tests/test_worktree_gate_hook.py` | RED/GREEN: bash write patterns block/allow/fail-open; non-write shell allow |
| `tests/test_hooks_render.py` | Matcher/Cursor wiring for extended or dual hook |
| `tests/test_worktree_flow_recipe.py` / `test_sync_pipeline.py` / `test_recipes_catalog.py` | Fixture expectations if matcher/docs change |
| Generated `.omp/extensions/…`, `.pi/…`, `.opencode/…`, `.claude/settings.json` | Via `ai-specs sync` only — never hand-edit |

Unlikely needed: recipe schema event enum (already has pre/post-tool-use); plan-build-gate may want a **follow-up** for the same bash hole (out of scope unless design expands).

## Planning depth recommendation

| Field | Value |
|-------|--------|
| **Change slug** | `worktree-gate-bash-coverage` |
| **Decision-matrix class** | **`domain_change`** (not `behavior_change`) |
| **Artifacts** | `proposal.md`, `design.md`, `tasks.md` (+ delta specs under `openspec/changes/…/specs/`) |
| **worktree_required** | true (already on dedicated worktree) |
| **strict_tdd** | true (`openspec/config.yaml`) |

### Justification

From `openspec/config.yaml` decision_matrix:

- `behavior_change` → tasks only, no proposal/design.
- `domain_change` → proposal + design + tasks.

This is **domain_change** because:

1. It changes the **safety contract** of a foundational capability (`worktree-isolation`), not a one-line bugfix.
2. Cross-cutting **multi-harness** tradeoffs (Cursor skip vs dual hook vs renderer; heuristic fail-open policy; residual subagent gaps) need an explicit design ADR.
3. Spec deltas touch `worktree-flow` and possibly `runtime-hook-distribution`.
4. Sibling discipline (`worktree-flow-repo-topology`) used full SDD; same risk class (hook honesty / worktree-flow correctness).
5. Wrong heuristics or auto-revert designs are high blast-radius; design must lock “fail-open, no revert, best-effort patterns list.”

Not `trivial`/`local_fix`: live exploit + multi-file product surface.

## Open questions for proposal/design

1. **One hook vs two:** extend matcher only (claude/omp/pi/opencode) + separate Cursor shell hook sharing script, vs renderer mixed-matcher support.
2. **Heuristic v1 pattern set:** minimal high-precision list (`>`, `>>`, `tee`, `sed -i`, `cp`/`mv` into tree, `python* -c`/`<<` with write APIs) vs broader; document intentional misses.
3. **Tool name aliases:** `Bash`, `bash`, `shell`, `Shell`, `Execute` — which appear in supported harnesses in 2026? Confirm against live tool catalogs during apply.
4. **plan-build-gate** same MATCHER hole — same change or tracked follow-up?
5. **ask mode** messaging for shell blocks (parity with file path message).
6. Whether Cursor shell hook should run with **empty matcher** (all beforeShellExecution) vs Bash-named (N/A on Cursor event model).

## Risks

- Heuristic arms race / false sense of security if docs overclaim “bash is gated.”
- False positive blocks on legitimate protected-branch shell (rare if only main+protected and high-precision patterns).
- Dual-hook drift if two matchers diverge.
- Skipping design and shipping matcher-only without script changes → **zero effect** (empty file_path fail-open).

## Ready for Proposal

**Yes.** Problem grounded in code + live bypass; three strategies compared; recommended direction is hybrid **A+C** with explicit partial harness coverage; depth **domain_change** under slug `worktree-gate-bash-coverage`.

### Recommended direction (concise)

Close the bash bypass by teaching `worktree-gate.sh` best-effort command-string write detection (fail-open), wiring shell into pre-tool matchers (and a Cursor-capable shell hook or renderer fix), reinforcing brief/SKILL anti-fallback policy, and documenting residual process/heuristic gaps — full proposal → design → delta specs → tasks → TDD apply.

## Artifact path

`openspec/changes/worktree-gate-bash-coverage/explore.md`
