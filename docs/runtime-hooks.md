# Runtime hooks

Recipes can ship **agent-runtime lifecycle hooks** via `[[provides.hooks]]`.
`ai-specs sync` distributes one hook to every enabled harness in its native
runtime-hook format. A hook is either a script the product materializes or, for
the recipes the CLI serves, a command bound to the installed CLI. Either way the
author writes the decision once and the product owns the per-harness adapters.

> Distinct from the sync-time `[[hooks]]` table (`event = "on-sync"`), which runs
> *during* `ai-specs sync` materialization. Runtime hooks fire while the coding
> agent runs (per tool call, session, stop).

## Normalized script contract

A hook script receives a normalized JSON event on **stdin** and communicates its
decision through its **exit code**:

```json
{ "event": "pre-tool-use", "tool_name": "Edit",
  "tool_input": { "file_path": "..." }, "cwd": "..." }
```

| exit code | meaning |
|-----------|---------|
| `0` | allow the action |
| `2` | block the action (stderr is surfaced to the agent) |
| any other | non-blocking error → **fail-open** (allow) |

One contract for all harnesses. A buggy guard must never wedge all work.

## Abstract → native event map (v1)

The product owns a fixed set of abstract events mapped to each harness's native
event. This map is the single source of truth used by every renderer
(`internal/sync/hooksrender.go`).

| abstract | claude | cursor | opencode | pi | omp |
|----------|--------|--------|----------|-----|-----|
| `pre-tool-use` | `PreToolUse` | `beforeShellExecution` (shell/MCP only; **no pre-file-write hook**) | `tool.execute.before` | `tool_call` | `tool_call` |
| `post-tool-use` | `PostToolUse` | `afterShellExecution` | `tool.execute.after` | `tool_result` | `tool_result` |
| `session-start` | `SessionStart` | `sessionStart` | — (observe-only) | `session_start` | `session_start` |
| `stop` | `Stop` | `stop` | — (no equivalent) | `agent_end` | `agent_end` |

Unsupported `(event, harness)` pairs (—) are **warned and skipped** — sync never
emits broken wiring, and continues rendering the hook for harnesses that support
it.

## Per-harness distribution

| harness | wiring | block signal |
|---------|--------|--------------|
| **claude** | the hook command wired directly into a managed block in `.claude/settings.json` | `exit 2` (native) |
| **cursor** | generated shell wrapper in `.cursor/hooks/<recipe>-<hook>.sh`, referenced by `.cursor/hooks.json` | wrapper emits `{"permission":"deny", ...}` (snake_case) on `exit 2` |
| **opencode** | generated TS plugin `.opencode/plugin/<recipe>-<hook>.ts` | `throw` on `exit 2` |
| **pi** | generated TS extension `.pi/extensions/<recipe>-<hook>.ts` (imports `@earendil-works/pi-coding-agent`) | `return { block: true, reason }` on `exit 2` |
| **omp** | generated TS extension `.omp/extensions/<recipe>-<hook>.ts` (imports `@oh-my-pi/pi-coding-agent`) | `return { block: true, reason }` on `exit 2` |

Only **Claude** natively honors the normalized contract, so it gets the command
wired directly. Every other harness decides through a different channel, so sync
generates a thin **adapter** that runs that command with the normalized event on
stdin and translates `exit 2` into that harness's native decision.

### Known gaps

- **Cursor** has no generic "pre tool" event and **no pre-file-write hook**. A
  `pre-tool-use` hook whose matcher targets file writes (`Edit`/`Write`/
  `MultiEdit`/`NotebookEdit`) has no Cursor target → warn-and-skip.
- **OpenCode** `tool.execute.before` does **not** fire for **subagent** tool
  calls (opencode#5894) or **MCP** tool calls (opencode#2319).
- **Pi / omp** `tool_call` handlers apply to tool calls in **that agent
  process**. Typical subagent/task delegation spawns a **separate process**;
  the parent's extension handlers do not see the child's `write`/`edit` calls.
  Child enforcement only happens if that child also loads the same project
  extensions — not part of the ai-specs distribution contract. Do not treat
  worktree/plan-build gates as the sole guard for delegation-heavy workflows.
- **OpenCode** `session-start`/`stop` are observe-only; there is no blocking
  session hook.

## Where the hook logic lives

Two layouts, decided by the recipe:

- **CLI-served** (every recipe that ships a gate today: `worktree-flow`'s
  `worktree-gate`, `trello-mcp-workflow`'s `tracker-card-gate` and
  `plan-build-flow`'s `plan-build-gate`). Nothing is materialized. Every
  harness's wiring carries a shell **command** that resolves the CLI — the
  install that ran sync at `dist/ai-specs` first, then `ai-specs` on `PATH` —
  and `exec`s the verb. A missing CLI exits `0` (fail open) with one stderr
  warning, and doctor reports the install as an ERROR.
- **Materialized script** (any other recipe). The script is copied once to a
  harness-neutral path under the project, made executable, and every harness's
  wiring references that single copy:

  ```
  ai-specs/recipes/<recipe-id>/hooks/<script>
  ```

A project synced from a custom CLI home is pinned to that install: the rendered
command names the absolute `dist/ai-specs` path of the install that ran sync, and
only a later sync (after the CLI is reinstalled) rewrites it.

## Config flow

Tunable values declared in the recipe `[config.*]` (overridable per-project in
`[recipes.<id>.config]`) reach the rendered hook as **environment variables**.
For example, `worktree-flow` exposes `WORKTREE_GATE_PROTECTED` (space-separated
protected branch names) to its `worktree-gate` hook as a rendered env entry.

A CLI-served hook stamps nothing into the wiring: the verb reads the resolved
values from the **per-project sync snapshot**
(`cache/projects/<key>/runtime.toml`), which is the run-time config source, never
`ai-specs.toml` (S2). That is why a `gate_mode` edit takes effect on the next
sync and why a hook cannot disagree with the manifest it was synced from.

- `worktree-flow`: the snapshot's `[worktree_gate]` table carries `gate_mode`,
  `gate_scope`, `repo_topology`, `protected`, `worktrees_dir` and
  `integration_branch`; `WORKTREE_GATE_MODE` remains a one-shot process override.
- `trello-mcp-workflow`: `[tracker_gate].gate_mode` carries the legacy hint
  (default `warn`), and `TRACKER_CARD_GATE_MODE` is the one-shot env override.
  The ledger `ledger_mode` is not stamped: the checkpoint hosts read it from
  `ai-specs/ai-specs.toml` at runtime (falling back to `gate_mode`), and
  `TRACKER_LEDGER_MODE` is the one-shot override.
- `plan-build-flow`: reads no policy from the snapshot; its rule is the change
  folder itself.

## Gate implementation (worktree-flow)

There is no launcher. `worktree-flow`'s hooks are CLI-served: the rendered
command `exec`s `ai-specs worktree-gate --hook <id>`, and the verb resolves the
event exactly as the retired thin launcher did, so stdin and the exit code pass
through untouched. `gate_impl` accepts `auto | go` only; both use the same
`worktree-gate` route in the installed CLI and fail open if that route is
unavailable (`ai-specs doctor` reports ERROR).

Resolution order (first hit wins):

1. An executable `WORKTREE_GATE_BIN` override; with `WORKTREE_GATE_VERIFY=1`,
   an override that fails `--selftest` is ignored.
2. The installed CLI that ran sync (`dist/ai-specs`), then `ai-specs` on `PATH`.
3. If no CLI is available, warn on stderr and exit `0` (fail open).
   Use `ai-specs doctor` to diagnose the install; upgrade an old or failing
   CLI with `ai-specs upgrade`, or reinstall via `install.sh` if it is missing
   or not executable.

The gate ships inside the CLI release; the project-local pin and version-keyed
cache are ignored, and there are no separate gate assets or checksum trust root.

**Recovery (no Bash rollback path):** per invocation, set
`WORKTREE_GATE_MODE=off` or `WORKTREE_GATE_BIN=<path>`. Full revert is install
the previous CLI release and run `ai-specs sync`. `gate_impl = auto` with an
unavailable CLI route is a doctor ERROR, not a silent fallback.

Because every renderer emits the same CLI command, all five harnesses keep
working without re-render churn, and the Cursor wrapper maps exit `2` →
`{"permission":"deny"}` exactly as before (the CLI's empty stdout does not
degrade the deny decision — the gate message travels on stderr).

### Workspace context model

The hook keeps three distinct contexts, and no adapter or launcher conflates
them:

| Context | Producer | Meaning | Fallback |
|---------|----------|---------|----------|
| Event cwd | Runtime adapter + normalized event | Directory whose repository/worktree context the gate evaluates | Gate process cwd when the event value is unusable |
| Project root for the CLI hook | Generated adapter | Root exported to the hook as `AI_SPECS_PROJECT_ROOT` so the verb reads this project's snapshot | No process-cwd substitution |
| Process cwd | Actual hook child process | Fallback context when the event cwd is unusable | The OS/runtime process cwd |

- **Adapters carry a command, not a path.** The generated OpenCode plugin and
  Pi/OMP extensions export `AI_SPECS_PROJECT_ROOT` from `import.meta.url`
  (`../../` from `.opencode/plugin`, `.pi/extensions`, or `.omp/extensions` up to
  the project root) and spawn the hook command through `sh -c`. No generated
  adapter emits a relative `SCRIPT`, a script path, or a hardcoded project path.
- **OpenCode validates the explicit directory.** `directory` is outer-trimmed
  and accepted only as a string, absolute, existing directory; the one
  normalized value drives both the event `cwd` and the child `spawnSync` cwd.
  Absent, non-string, whitespace-only, relative, nonexistent, or non-directory
  values fall back to the process cwd for both. Child `spawnSync` errors and
  thrown child-process exceptions fail open; status `2` remains the only block.
- **Pi/OMP keep process-cwd-only events** and claim no workspace root; Cursor's
  missing pre-file-write hook and OpenCode's subagent/MCP coverage gap are
  unchanged (see below).

**Unchanged pre-existing coverage gaps** (not introduced by the Go cutover):

- **Cursor** has no generic "pre tool" event and **no pre-file-write hook**;
  a file-write-matcher `pre-tool-use` hook has no Cursor target → warn-and-skip.
  The shell matcher still renders as a genuine `beforeShellExecution` hook.
- **OpenCode** `tool.execute.before` does **not** fire for **subagent** tool
  calls (opencode#5894) or **MCP** tool calls (opencode#2319).
- **Pi / omp** `tool_call` handlers apply to tool calls in **that agent
  process**; subagent/task delegation spawns a separate process whose
  `write`/`edit` calls the parent's handlers do not see.

## Idempotency

All generated wiring lives in a managed block keyed by
`ai-specs:hooks:<recipe>:<hook>` (a `_ai_specs_managed` tag in JSON; a wholly
generated file for TS shims and the Cursor wrapper). A second `sync` with no
manifest change produces no diff, and user-authored hooks outside the managed
block are preserved.

## Status

| harness | `pre-tool-use` blocking | notes |
|---------|-------------------------|-------|
| claude | ✅ | native exit-2 |
| pi | ✅ (this process) | not a cross-process subagent guarantee |
| omp | ✅ (this process) | same handler shape as pi |
| opencode | ✅ (primary agent) | not subagent/MCP tool calls |
| cursor | ⚠️ | no pre-file-write hook; shell/MCP gates only |

## Dual-hook gates (worktree-flow + trello-mcp-workflow)

| Recipe | Path hook id | Shell hook id | Shell heuristic |
|--------|--------------|---------------|-----------------|
| `worktree-flow` | `worktree-gate` | `worktree-gate-shell` | shell writes into protected main |
| `trello-mcp-workflow` | `tracker-card-gate` | `tracker-card-gate-shell` | `gh pr create` (archive-close is graded by the verb's direct mode) |

Both share one script per recipe with two `[[provides.hooks]]` ids so
Cursor's file-write skip does not swallow shell coverage. Neither gate
intercepts MCP tool calls.

## Ledger checkpoints (tracker lifecycle)

The `ai-specs plan-build-gate` and `ai-specs tracker-card-gate` verbs, plus the
Tracker gate's direct host mode, are the hosts of one Go predicate: each calls
`worktree-gate --ledger --checkpoint <name>` in process inside the installed
CLI, not through the project-local pin, the cache, or `WORKTREE_GATE_BIN`. Both
rendered hook commands pin the sync-time install
(`<sync-home>/dist/ai-specs <verb>`) instead of re-reading the agent's
environment, and Go resolves the effective ledger mode. Neither host compiles or
downloads a binary. Tracker verdicts are advisory and exit `0`: a rendered
tracker hook whose CLI is missing exits `0` without grading, and the verb itself
only exits `2` for a direct-mode usage error. Plan Build blocks: it exits `2` for
a work-start block verdict and for its own missing-plan refusal.

The domain is Tracker, not a provider. One pure Go grader compares neutral
expectations against neutral observations; a provider recipe extends that domain
by declaring a `[config.reconcile]` **adapter** mapping in the project manifest,
while `ledger_mode` / `gate_mode` stay Tracker-domain policy. The tracker
lifecycle host is `ai-specs tracker-card-gate` invoked directly
(`--root <root> --checkpoint pre-merge|archive-close`); `ai-specs gates premerge`
is Plan Build's artifact-only guardian and never invokes the ledger.

| Checkpoint | Host |
|---|---|
| `work-start` | `plan-build-flow` `ai-specs plan-build-gate` hook, path kind (before the SDD/proposal phase and before the first production write; no change folder required; resolves the witness-bound recipe id in process) |
| `apply-start` | `trello-mcp-workflow` `tracker-card-gate` hook, path kind |
| `pr-review` | `trello-mcp-workflow` `tracker-card-gate` hook, shell `gh pr create` |
| `pre-merge` | `ai-specs tracker-card-gate --root <root> --checkpoint pre-merge` |
| `archive-close` | `ai-specs tracker-card-gate --root <root> --checkpoint archive-close` |

The `pre-merge` and `archive-close` lifecycle is Plan Build-independent: the host
needs **no `openspec/` tree**, and `--stage pre-merge|pre-archive` remains a
compatibility alias for `--checkpoint pre-merge|archive-close`. `archive-close` is
**tracker item closure**, not an OpenSpec archive — the host never reads archive
state to infer a close, and it is acquisition-only (evidence + JSON bridge, no
provider write, no network, no second grader).

**Write surface.** Items are opened, linked, closed, and exempted only by explicit
writes on the same dispatcher:

```bash
ai-specs worktree-gate --ledger --checkpoint apply-start --ledger-mode warn \
  --project-root . --write '{"kind":"open"}'
ai-specs worktree-gate --ledger --checkpoint apply-start --ledger-mode warn --project-root . \
  --write '{"kind":"link","item_id":"<native-id>","url":"<url>","native_type":"card","state":"in-progress"}'
# Generic, workflow-agnostic: open-if-absent + link in one locked transaction.
ai-specs worktree-gate --ledger --checkpoint apply-start --ledger-mode warn --project-root . \
  --write '{"kind":"bind","item_id":"<native-id>","url":"<url>","native_type":"card","state":"in-progress"}'
```

`bind` is the one verb that combines open and link, so SDD, ODD, and no-flow callers
can seed the branch item with no `## Tracker` / `tracker.none` / `openspec/` artifact
and no provider or network call. `--write` and `--decide` are mutually exclusive
(both → exit `2`, nothing persisted). Success adds
`"write":{"kind":…,"applied":…,"reason":…}` to the verdict JSON, with
`already-open` / `unchanged` / `already-closed` for the idempotent no-ops. A failed
write prints `worktree-gate: ledger --write failed: …` on stderr, persists nothing,
and exits `2` with **no** stdout JSON.

**Pure grade paths vs. authorized lifecycle writes.** Every *grade* path is pure: no
checkpoint, in any mode, opens or mutates an item because a `## Tracker` section
parses. Exactly two writers record lifecycle locally: the generic machine write
surface above, and the VCS-boundary `archive-close` writer run by the verified
worktree cleanup. Both write only `<git-common-dir>/ai-specs/ledger/state.json`,
perform no provider MCP/API call and no network I/O, and are never reached from a
grade path. Opening is therefore **not** agent-only.

**Evidence sides.** `ai-specs tracker-card-gate` (pre-tool-use hook and direct
host mode) builds its
`--evidence` file through the native `ai-specs tracker evidence` verb — acquisition only
(`## Tracker` / `tracker.none` / local Git facts; no `gh`, no MCP, no network).
`local` is the ledger's own store snapshot, `code` is the change's `card_id`, `git`
is that id when a `pr:` is recorded, and `remote` has **no producer in this
`--evidence` file** — a deliberate 3-of-4 reconciliation there. Missing, malformed,
or unreadable artifacts yield empty sides (fail open). Branch names and PR URLs are
never used as evidence sides. Remote reconciliation is a separate, explicit
`--reconcile` comparison that stays off the hooks and off the grade path and is
opt-in per project (a project without the `[config.reconcile]` mapping gets an
explicit `unconfigured`, never a default); it writes neither the store nor the
provider and leaves the graded exit code unchanged.

**`tracker.none`.** Where a host finds `openspec/changes/<slug>/tracker.none` it treats
it as evidence only (blank `code` side) and grades; it never records the exemption
itself (R1). The file is the human act and is never created, modified, or deleted by a
host. Recording the exemption is the explicit human/agent
`--write '{"kind":"exempt","reason":"<one line>"}'`. A recorded `Item.Exemption` is
honored at every checkpoint as allow/exempt and is not a conflict; removing the file
does not revoke it (a human adjudicates with `--decide`).

Sync stamps the CLI home that served it into the rendered hook command, which
execs `<sync-home>/dist/ai-specs tracker-card-gate`; the verb does its evidence
acquisition (`tracker slug|evidence`) and stdin classification in process, so
the hook carries no python3. `ai-specs` on PATH is the fallback only when that
install's binary is gone, and a missing binary skips the evidence file and
exits `0`. The legacy `__TRACKER_LIB_INTERNAL__` stamp remains a materializer
placeholder that no gate reads any more: both verbs resolve the witness-bound
recipe id in process.

Without the CLI the tracker gate also cannot classify the hook event, so it exits 0
without grading `apply-start` or `pr-review`. This is the accepted U-F2b behavior
change (python3 did not need the CLI); it is fail open, never a block.

The verdict is computed from the durable binding witness at
`<git-common-dir>/ai-specs/ledger/witness.json` and the per-identity store at
`<git-common-dir>/ai-specs/ledger/state.json`. Only a `bound` witness activates the
ledger; missing, unreadable, or unknown-version witnesses stay dormant
(`witness-missing`) and never guess a provider. Mode comes from project config at
`recipes.<witness recipe_id>.config`: the tracker `ledger_mode`
(`always | ask | warn`, default `warn`) wins, and the legacy `gate_mode` maps
forward (`off` → skip checkpoints, `warn` → `warn`, `always` → `always`). The recipe
id is read from the durable witness (the legacy literal is only the bridge's
fallback), so a non-legacy provider recipe's own config drives the checkpoint. The
worktree gate's own mode is never read as the ledger mode. `TRACKER_LEDGER_MODE` is
the one-shot override.

Dormancy is visible through `doctor` only — a `tracker-ledger` check (unbound INFO,
ambiguous / declared-not-bound / missing witness / recorded conflict WARN,
infrastructure failure ERROR), plus one INFO when a bound witness has no
`plan-build-flow` recipe enabled: `work-start is unhosted`, with the other four
checkpoints still grading. The runtime brief and generated agent files gain no
dormancy line. No host performs a provider MCP/API create or update; `always`
blocks until a human supplies the item. Path hosts never block `openspec/**`.

**VCS-boundary close (`post-merge` and `post-checkout`).** Independent of the
tracker gate hook, `worktree-flow` installs a managed `.git/hooks/post-merge`
trigger: a short `sh`
script that proves the installed CLI serves `worktree-cleanup` (`--help`) and then
`exec`s the verb, which sync ALWAYS regenerates (it is not an
override, so a hand-written hook is replaced). At a merge boundary the verb
closes the matching open item at `archive-close` — matched on the Git common dir
+ branch, ignoring any stored change slug — **before** removing a provably
merged worktree or local branch. The close is
idempotent and never reopens a closed row (D17). The hook is fail-open for the
already-sealed merge outcome (failures go to stderr and it exits `0`), while the
destructive cleanup fails closed: a close that cannot be persisted preserves the
candidate. The one skew case the hook reports itself is an installed CLI older
than the verb: it removes nothing and prints a single actionable line
(`post-merge: … does not serve 'worktree-cleanup'; reinstall a current ai-specs,
then run: ai-specs sync`) on stderr, then exits `0`. A second managed trigger, `.git/hooks/post-checkout`, covers the merge that never
happens locally: a pull request squashed or merged on the hosting provider lands
as `git checkout <integration>` (or `git switch`) after a fetch, which
`post-merge` never sees. It runs the same verb with `--if-integration`, which
compares the primary checkout's branch with the snapshot's `integration_branch`:
on a match it runs the pass, otherwise it prints one line
(`worktree-cleanup: current branch is X, not the integration branch Y; skipping
the cleanup pass.`) and exits `0`. The comparison lives in Go, so the hook still
carries no policy. `ai-specs sync` deliberately runs no cleanup: it runs
unattended in every project and must never delete worktrees. The trigger is workflow-agnostic (no SDD/ODD/OpenSpec and no provider
call), and the direct tracker host
(`ai-specs tracker-card-gate --root <root> --checkpoint archive-close`) remains
the fallback when the hook is absent. The hook carries **no** config: the verb
reads `worktrees_dir`, `integration_branch` and `repo_topology` from the
per-project sync snapshot, so cleanup never silently falls back to
`.worktrees`/current HEAD. The recorded topology is the CLI-resolved
`[project].repo_topology` value (the deprecated
`recipes.worktree-flow.config.repo_topology` alias is read only when the project
field is absent).

## Shell write-bypass coverage (worktree-flow)

`worktree-flow`'s gate also registers a second matcher
(`worktree-gate-shell`, matcher `Bash|Shell|Execute|Terminal`) that
heuristically detects shell commands (redirects, `tee`, `sed -i`/`perl -i`,
`cp`/`mv`, interpreter `-c`/heredoc bodies calling `write_text`/`open(...,'w')`
and language-specific writers) that would write into the protected main
worktree — closing the gap where an agent falls back to `bash` after a
structured Edit/Write call is blocked or errors. The heuristic is
**best-effort and fails open** on any ambiguous, unparseable, or
out-of-scope command; it is not a general shell-command sandbox.

Coverage is **not uniform** across harnesses, by structural necessity (not
an oversight):

| harness | shell write-bypass detection | notes |
|---------|-------------------------------|-------|
| claude | ✅ | shell/Bash matcher intersects no file-write token; renders as a normal second `PreToolUse` entry |
| pi | ✅ (this process) | same `tool_call` handler shape as the path-write hook; not a cross-process subagent guarantee |
| omp | ✅ (this process) | same as pi |
| opencode | ✅ (primary agent) | not subagent/MCP tool calls, same limitation as the path-write hook |
| cursor | ✅ | registers as a genuinely separate `beforeShellExecution` hook (its own managed id), since `_matcher_targets_file_writes` only skips matchers that target Edit/Write/MultiEdit/NotebookEdit — the shell matcher never matches that set |

Where shell interception is structurally unavailable or unreliable (e.g. any
harness's subagent/MCP tool-call boundary), the mitigation is policy-level:
`worktree-flow`'s brief `workflow_rules` and `SKILL.md` explicitly instruct
agents that a blocked or errored Edit/Write on a protected branch is never
grounds to retry the write via bash/shell — the correct response is to create
a worktree first.

`tracker-card-gate` does **not** intercept Trello MCP and does **not**
claim uniform full prevention across harnesses — see Residual platform
gaps in the trello-mcp-workflow README.
