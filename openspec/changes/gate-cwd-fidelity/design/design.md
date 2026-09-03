# Design: gate-cwd-fidelity

- **Change slug**: `gate-cwd-fidelity`
- **Depth**: Full
- **Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/gate-cwd-fidelity`
- **Branch**: `change/gate-cwd-fidelity`
- **Scope**: `catalog/recipes/worktree-flow/gate/` only (shared Go binary for `worktree-gate` and `worktree-gate-shell`)
- **Frozen**: Pi/OMP event cwd remains `process.cwd()` (`openspec/specs/workspace-context`; `tests/test_hooks_render.py::_assert_process_cwd_event`)
- **Out of scope**: `hooks-render.py` cwd semantics, `worktree-gate.sh` launcher contract, `stamped_gate_version` / `doctor.py::_check_worktree_gate`

## 1. Context / problem

The shell gate absolutizes relative write candidates against `Event.Cwd` (`decide.go::Decide` joins a relative candidate with the supplied `cwd`). `ParseEvent` sets that field from the event JSON, falling back to `processCwd()`. Pi/OMP adapters correctly report `cwd: process.cwd()` of the **host session**. For `git -C <worktree> mv <relative>` and `cd <worktree> && …`, the writable directory lives in the **command text**, so the gate block-on-guesses against the protected primary and emits a wrong-exit “create a dedicated worktree” message.

Spec delta (`openspec/changes/gate-cwd-fidelity/specs/worktree-flow/spec.md`):

| ID | Requirement | Gate must |
|----|-------------|-----------|
| R1 | Shell-gate command cwd fidelity | Recover static `git -C <dir>` / `cd <dir>` and use that base **before** absolutizing |
| R2 | Honest degrade when effective cwd is unrecoverable | Relative + no recoverable base → warn/ask, **never** block-on-guess against host `process.cwd()` / `$PWD` |
| R3 | Ask-mode and message parity (modified) | Block stderr names the real command cwd; `/worktree-new` only when that cwd is the protected primary |

The delta also MODIFIED **Internal URI allowlist and event-cwd precedence** so command cwd outranks event cwd, and `$PWD` is not a blocking fallback.

## 2. Architecture

Keep `Decide` as the policy engine. Insert a **cwd-recovery pre-pass** that supplies the `cwd` argument `Decide` already takes. Do not teach `Decide` to parse shell. Do not change adapters.

```text
stdin JSON
    │
    ▼
ParseEvent                    PATH: Event.Cwd = usable event cwd (trusted or not)
    │                         SHELL: splitPOSIX → segments-with-separators
    │                                → recoverCwdWalk (new)
    │                                → extractPass1 per segment (existing)
    │                                → extractPass2 once (existing)
    │
    ▼
main / explainRun loop
    │
    ├─ absolute candidate     → Decide(path, unused-cwd) unchanged
    ├─ relative + recovered   → Decide(path, commandCwd)     block-on-trust
    ├─ relative + trusted     → Decide(path, eventCwd)       block-on-trust
    │   event cwd, no changer
    └─ relative + unrecoverable → degrade (no Decide block)  R2
```

Shared binary: `worktree-gate.sh` already launches the same Go binary for both hook ids. Recovery lives in the binary; the launcher is untouched.

### 2.1 Where the pass sits

| Option | Verdict |
|--------|---------|
| Inside `splitPOSIX` | Reject. Tokenizer stays a shlex clone (`tokenize.go`, design D9 of `worktree-gate-go`). |
| Inside `Decide` | Reject. `Decide` is path/Git policy; shell grammar there would run for PATH events too. |
| New `cwd.go`, called from `ParseEvent` (shell) and consumed in `main.go` / `explainRun` | **Selected.** Reuses `splitPOSIX` tokens. Extends segment splitting so separators survive. |

`extract.go` keeps finding **what** is written. `cwd.go` answers **from which directory** those relative paths are resolved.

### 2.2 Data flow

Today `Event` is `{Mode, Tool, Cwd, Candidates []string}` and `eventCwd` always substitutes `processCwd()` when JSON cwd is unusable. That substitution is exactly the block-on-guess R2 forbids.

Extend the event (names are apply-facing; tasks may shorten):

```text
Event {
  Mode, Tool string
  Cwd        string   // JSON cwd if usable, else process fallback (diagnostics only)
  CwdTrusted bool     // true iff JSON cwd was an absolute existing directory
  Candidates []WriteCandidate
}

WriteCandidate {
  Path   string      // scrubbed token from extract
  Base   string      // recovered abs dir, or trusted event cwd, or ""
  Source cwdSource   // command | event | none
}
```

`explainOutput.Cwd` stays the diagnostic event cwd. Add `cwd_source` (and optionally `command_cwd`) so `--explain` can show why a relative path was allowed. `explainRun` **must** call the same effective-base helper as `run`; today it still does `Decide(candidate, event.Cwd)` and would reintroduce the bug on `--explain`.

PATH mode: no command walk. Each candidate `Source=event` when `CwdTrusted`, else `Source=none`. Relative + `none` degrades (R2 / URI scenario *Missing command cwd and event cwd*). Absolute PATH candidates still classify.

### 2.3 Effective base (single helper)

```text
effectiveBase(c WriteCandidate) (base string, degrade bool):
  if filepath.IsAbs(c.Path): return "", false          // Decide ignores cwd for abs paths
  switch c.Source:
    command, event: return c.Base, false
    none:           return "", true
```

`main.go` uses `degrade` to skip `Decide` as a blocking input. `IsClaudeException` must join against `effectiveBase`, not raw `event.Cwd`.

## 3. Cwd recovery grammar (risk 1)

The gate is **not** a shell interpreter. Recovery is a static token walk over POSIX words already produced by `splitPOSIX`. Anything that requires expansion, job control, or a real `cd` builtin falls through to `Source=none`.

### 3.1 Tokenization (unchanged)

`splitPOSIX` already yields space-preserving operands (`git -C "/My WT" mv a b` → `-C` then `/My WT`). Recovery never re-splits.

### 3.2 Segments with separators (small change)

Replace `splitSegments` with a splitter that **keeps** the operator that preceded each segment: `|`, `||`, `&&`, `;`. Today those tokens are discarded, which makes `cd A && foo | tee f` indistinguishable from a pipeline-local `cd`.

```text
type segment struct {
  tokens []string
  sep    string  // "", "&&", "||", "|", ";"
}
```

### 3.3 Sequential shell cwd vs per-invocation overlay

Walk segments left to right with a sequential **shell cwd** `S` (starts as trusted event cwd, or unset).

| Separator / form | Shell cwd `S` | Segment overlay used for that segment’s writes |
|------------------|---------------|------------------------------------------------|
| `&&` or `;` after a recovered `cd <dir>` | `S := resolve(dir, S)` | subsequent segments use new `S` |
| `\|` or `\|\|` | `S` unchanged by a `cd` **inside** a pipe sibling | pipeline segments inherit `S` from **before** the pipeline (so `cd A && foo \| tee f` still resolves `f` against `A`) |
| `git -C <dir>` in a segment | `S` unchanged (`-C` does not `chdir` the shell) | that segment’s git operands use overlay `resolve(dir, S)` chained for multiple `-C` |
| PATH event / no changer | `S` = trusted event cwd | overlay = `S` |

`git -C A && echo x > f`: write is not a git operand; overlay for `echo` is `S` (event cwd), **not** `A`. That is intentional and must be tested.

`cd A && git -C B mv rel dest`: `S=A`, git overlay `B` (relative `B` resolved against `A`), `dest` against `B`.

`git -C A && cd B && echo x > f`: `S` becomes `B`; `f` against `B`.

### 3.4 Recognized forms (closed list)

**`cd`** (first word of the segment is `cd`, optionally with skipped `-L`/`-P` and a `--`):

- Exactly one directory operand after flags.
- Operand is **static**: no `$`, `` ` ``, `$(`, leading `~`, or operand `-` / `-N`.
- Relative operand resolves against current `S` if `S` is set; against trusted event cwd if `S` unset but event cwd trusted; otherwise the `cd` is unrecoverable.
- Resolved directory MUST pass `IsExistingDirectory` (same predicate as event cwd).
- `cd` with no operand (HOME), extra operands, or a non-static operand → **unrecoverable from this point forward** for sequential `S`.

**`git -C`**:

- Word `git` or a path ending in `/git`.
- `-C` as its own token; the **next** token is the directory (static + existing, same rules as `cd`).
- Attached `-C<path>` (no separate token) is also accepted: the remainder after `-C` is the directory.
- Multiple `-C` in one invocation **chain** (Git’s documented behavior): each directory is resolved against the overlay produced by the previous `-C`.
- `-C` without a following operand, or a non-static/non-existing operand → this **segment’s git overlay** is unrecoverable (does not poison later `cd`/`&&` unless the same segment also has a failed `cd`).

No other Git cwd flags (`--git-dir`, `--work-tree`, `GIT_DIR=`) are recovered. They are unrecoverable if they are the only cwd signal and the write is relative.

### 3.5 Stop / unrecoverable (closed list)

Mark sequential `S` or the current overlay unrecoverable (do not guess) when any of these appear in a position that would affect that overlay:

- `cd -`
- operand with `$`, backtick, `$(`, or `~`
- `(` / subshell / process substitution as a cwd changer
- `splitPOSIX` failure (already fail-open: no candidates)
- recovered path that is not an existing directory

Unrecoverable **does not** rewrite history: a prior recovered `cd A &&` still applies to earlier segments. Only the affected overlay and later sequential `S` become `none`.

### 3.6 Pass-2 candidates

`extractPass2` runs on the **full** command string (existing quirk: today’s loop appends it once per segment). Call it **once**. Attach the **final sequential `S`** (last recovered `cd`/`&&` chain), not a `git -C` overlay — pass-2 is interpreter bodies, not git operands. If `S` is unrecoverable and the extracted path is relative, degrade.

## 4. Degrade mechanism (risk 2)

### 4.1 Exact “not recoverable” condition

A relative candidate degrades iff `Source == none`:

1. No static recovered command cwd for that candidate’s segment overlay, **and**
2. No trusted event cwd (`CwdTrusted == false`), **or** the command contained a **cwd changer that failed to recover** affecting this overlay (`cd -`, `cd $x`, failed `git -C`, subshell `cd`).

Interpretation of the R2 scenario vs the kept URI scenario *Relative event-cwd path inside protected repository remains blocked*:

- **No cwd changer** (`echo x > file.txt`): the shell runs at event cwd. If `CwdTrusted`, that is block-on-trust (URI scenario kept). Pi/OMP sending `cwd: process.cwd()` **is** a trusted event cwd for commands that do not change directory.
- **Cwd changer present but not statically recoverable** (`cd - && echo x > f`, `cd "$WT" && …`): the gate **cannot** establish the write lands at event cwd → `Source=none` → degrade even when JSON cwd is the protected primary. This is the R2 scenario’s “cannot establish that the command will actually execute there” clause.
- **Missing/unusable JSON cwd and no command cwd**: degrade; **do not** use `processCwd()` / `$PWD` as `Decide`’s base. `Event.Cwd` may still hold the fallback for `--explain` only.

This preserves today’s block for ordinary relative shell writes on a protected primary, and removes the false positive only where the command actually redirects cwd (or cwd is absent).

PATH mode has no changer: relative + untrusted event cwd degrades; relative + trusted event cwd still blocks.

### 4.2 Routing (no new bypass)

`ResolveGateMode` is unchanged: `off` still exits 0 before evaluation; env `WORKTREE_GATE_MODE` still beats stamp; invalid values still fall back as today.

| `gate_mode` | Honest `Decide` block (`Source` command or event) | Degrade (`Source=none`, relative) |
|-------------|---------------------------------------------------|-----------------------------------|
| `off` | never reached | never reached |
| `always` | `BlockMessage`, **exit 2** | `DegradeMessage`, **do not block this candidate**, continue loop |
| `ask` | `AskMessage`, **exit 2** (existing) | `DegradeMessage` (ask-flavored wording), **do not block this candidate**, continue loop |

Degrade is **fail-open on that candidate**. It is not `Decision{Allow:false}`. It must not call `BlockMessage`. If another candidate later honestly blocks, the process still exits 2.

`DegradeMessage` (new, in `message.go`):

- States that command cwd could not be recovered and the relative write was **not** classified against the host process cwd.
- MUST NOT contain `WORKTREE_GATE_MODE=off`, `to bypass`, or any new env/flag.
- MUST NOT instruct `/worktree-new` (cwd is unknown; that guidance is R3 for honest primary blocks only).
- `always` vs `ask` may differ in wording (`warn:` vs `ask the user if this write is intentional`) but share exit 0 for that candidate.

Existing bypass surface remains exactly: stamped `gate_mode`, env `WORKTREE_GATE_MODE`, and the ask-mode user-override **conversation** already described by `AskMessage` option (3). Degrade does not add a fourth.

Absolute candidates never degrade: `Decide` runs as today (R2 *Absolute candidate path classifies unchanged*).

Recoverable command cwd into the protected primary still blocks (R2 *Recoverable cwd keeps block-on-trust*).

## 5. Block / ask messages (R3)

`Decide` already allows linked worktrees (`gitDir != common` → allow). Honest blocks are therefore **primary checkouts** on a protected branch (including a submodule primary under topology/scope). The wrong-exit bug is the message claiming “create a dedicated worktree” when the **named** cwd is already a worktree the user is targeting; after R1 the original `git -C <linked-wt>` case **allows**, so R3 is still required for:

- `cd <protected-primary> && echo x > f` / `git -C <protected-primary> mv …` → keep `/worktree-new`
- a blocked **other** primary (e.g. proven subrepo primary under `gate_scope=auto`) → name that cwd, **do not** tell the agent to `/worktree-new` as if they were on the session’s main checkout with no destination

Signature change (apply-facing):

```text
BlockMessage(shell bool, toolName, candidate, branch, commandCwd string, createWorktree bool) string
AskMessage(...)  // same extra args; “create a dedicated worktree (recommended)” only if createWorktree
```

`createWorktree` is true iff the resolved base (command cwd if recovered, else trusted event cwd) is the **protected primary that `Decide` just blocked** — i.e. the same root `Decide` used (`repoRoot` where `gitDir == common`). When `createWorktree` is false, stderr MUST include the absolute `commandCwd` and MUST NOT contain `/worktree-new`.

Shell `BlockMessage` still names bash/shell bypass risk (existing sentence). Do **not** add `WORKTREE_GATE_MODE=off` to `AskMessage`; current tests (`TestAskMessagePresentsThreeDestinationsAndNoSelfBypass`) and `message.go` comments forbid advertising that hatch. The delta’s preserved “ask-mode MUST emit WORKTREE_GATE_MODE=off” sentence is **pre-existing spec/code drift** (canonical spec vs Go `AskMessage`). This change does **not** relitigate it: keep current Go behavior; do not introduce the hatch on the **degrade** path either (R2 forbids it).

`TestBlockMessageVerbatim` will change; the primary+shell fixture remains the legacy sentence plus the named cwd as specified.

## 6. Planned file changes

All under `catalog/recipes/worktree-flow/gate/`. No launcher, renderer, or adapter edits.

| File | Change |
|------|--------|
| **`cwd.go`** (new) | `splitSegmentsWithSep`, `staticDirOperand`, `recoverCwdWalk`, `resolveDir(operand, base)`, git `-C` chain, `cd` sequential update. Stdlib only. |
| **`cwd_test.go`** (new) | Grammar tables: see §7. |
| **`event.go`** | `WriteCandidate` / `CwdTrusted`; `ParseEvent` keeps command walk; stop calling `extractPass2` per segment; PATH vs SHELL assignment of `Source`. Keep `splitSegments` as a wrapper or replace call sites. |
| **`event_cwd_test.go`** | Missing JSON cwd → `CwdTrusted=false` (still records fallback in `Cwd` for diagnostics). Do not treat fallback as `Source=event`. |
| **`main.go`** | Loop uses `effectiveBase`; degrade branch; pass cwd into `BlockMessage`/`AskMessage`; `IsClaudeException` join base. |
| **`main.go` `explainRun`** | Same effective base; extra diagnostic fields. |
| **`decide.go`** | **No policy change.** Optional: return `RepoRoot` on a block so messages do not re-query git. Prefer a tiny helper `blockedPrimaryRoot(candidate, cwd)` at the call site if `Decision` stays `{Allow, Branch}`. |
| **`message.go`** | `BlockMessage`/`AskMessage` extra args; new `DegradeMessage`. |
| **`decide_test.go`** | Update verbatim message tests; add R3 cases. |
| **`extract.go`** | Unchanged extraction rules. `git -C wt mv a b` already yields destination `b` via the `mv` token; recovery supplies `wt` as base. |
| **`tokenize.go`** | Unchanged. |
| **`config.go`** | Unchanged. |
| **`uri.go`** | Unchanged (PATH-only allowlist). |

`packages/coding-agent` is not in scope; this repository’s gate is the worktree-flow recipe binary.

## 7. Tests to add

Go unit tests in `catalog/recipes/worktree-flow/gate/`. Use `gitFixture` + `git worktree add` like `TestDecideLinkedWorktreeAllows`. Drive `ParseEvent` + `run()` / `effectiveBase` / `Decide`, not a shell interpreter.

| Fixture | Expect |
|---------|--------|
| `git -C <wt> mv rel-a rel-b` with event cwd = protected primary | recover `<wt>`, `Decide` allow (linked worktree) |
| `git -C <wt> mv rel-a rel-b` with `--explain` | `decision=allow`, `cwd_source=command` (not `protected-branch`) |
| `cd <wt> && echo x > rel` | allow against `<wt>` |
| `cd <wt> && git -C <primary> mv a b` (or `git -C <primary>` after `cd <wt>`) | relative dest against primary → **block** (block-on-trust) |
| `git -C A && echo x > rel` (A is worktree, event cwd primary) | `rel` against **event cwd**, not A → block if event is protected primary |
| `git -C A -C B mv rel dest` | overlay chains A then B |
| Nested `cd A && cd B && echo x > rel` | last `cd` wins |
| Quoted spaces: `git -C "/tmp/My WT" mv a b` | one operand token, existing dir |
| `cd - && echo x > rel` | degrade, exit 0 in `always`, no `protected-branch` |
| `cd "$WT" && echo x > rel` / `cd $(pwd) && …` | degrade |
| `echo x > rel` (no changer, trusted event cwd = primary) | still **exit 2** |
| Relative path, **omitted** event cwd, process cwd = primary | degrade, exit 0 (not `$PWD` block) |
| Absolute path inside primary, any cwd state | exit 2 unchanged |
| `gate_mode=ask` + degrade | exit 0, stderr has `DegradeMessage`, **no** `WORKTREE_GATE_MODE=off` |
| `gate_mode=off` | exit 0 before recovery |
| `gate_mode=always` + recoverable primary write | exit 2, `BlockMessage` includes cwd, includes `/worktree-new` |
| Block on a non-session primary (subrepo fixture if cheap) | message names that cwd, **no** `/worktree-new` |
| PATH `file_path` relative + trusted event cwd | unchanged block/allow |
| Same shell JSON through `run()` (covers both hook ids; launcher not duplicated) | one test is enough |

Keep existing `event_cwd_test.go` trim/accept cases for **trusted** JSON cwd. Add the `CwdTrusted` distinction rather than deleting fallback storage.

## 8. Architectural decisions

### ADR-1 — Recover in the Go gate, not in the adapter

Adapters cannot name the command’s write cwd without parsing the command (Pi/OMP especially: event cwd is defined as `process.cwd()`). Parsing in every generated extension would fork the grammar per harness. The binary is already the single policy point for both hook surfaces. **Decision: gate-only.**

### ADR-2 — Do not touch workspace-context

Changing Pi/OMP to claim a workspace root would fail `_assert_process_cwd_event` and still would not decode `git -C`. Event cwd remains host process cwd. Command cwd is a **gate-internal** base that outranks event cwd when recovered.

### ADR-3 — `Decide` stays cwd-agnostic

Passing an already-chosen absolute base into `Decide` reuses topology, scope, linked-worktree, and central `openspec/changes` exception without a second policy path.

### ADR-4 — Degrade is fail-open, not ask-exit-2

Using `AskMessage` + exit 2 for unrecoverable cwd would keep false-positive blocks (`cd -`, variables). Explore required zero false positives on the untrustworthy path. Ask-mode degrade only changes stderr tone.

### ADR-5 — Minimal grammar, fail to degrade

No `eval`, no arithmetic `cd`, no `pushd`. Every extension of the grammar is a new false-negative risk (missed recovery) or false-positive risk (wrong overlay). Prefer degrade over a clever parse.

### ADR-6 — Canonical spec literalism for write-bypass (risk 3)

See §9.3. Tasks must add a **MODIFIED** of *Shell Command Write-Bypass Detection* so archive does not leave two contradictory MUSTs.

## 9. Closure of the three spec risks

### 9.1 Exact recovery semantics

Closed in §3: per-segment walk, separators preserved, `cd` sequential only across `&&` / `;`, `git -C` overlay does not `chdir` the shell, multiple `-C` chain, quoted spaces via `splitPOSIX`, `cd -` / expansions / subshell → unrecoverable. Parser stops at the closed token forms; it does not interpret the rest of the command.

### 9.2 Degrade does not create a bypass

Closed in §4: degrade never sets mode to `off`, never prints `WORKTREE_GATE_MODE=off`, never skips `ResolveGateMode`, and never converts an honest recovered-primary write into allow. `off` remains the only disable. Absolute and recovered-cwd paths keep exit 2.

### 9.3 Canonical “Shell Command Write-Bypass Detection” vs this delta

The ADDED requirement and the MODIFIED URI/event-cwd requirement **do** define precedence for this change’s implementation: command cwd > usable event cwd > degrade (no `$PWD` block).

They are **not** sufficient for the **canonical** spec after archive. *Shell Command Write-Bypass Detection* still MUST-resolves relative candidates “against the event `cwd`” with no MODIFIED. After merge, that sentence and R1 would both be live MUSTs.

**Decision: require a second delta MODIFIED** (spec hygiene, not extra Go work) in `openspec/changes/gate-cwd-fidelity/specs/worktree-flow/spec.md`:

- Keep every existing write-bypass scenario (redirect, tee, sed, fail-open, linked worktree, …).
- Replace the single resolution sentence with: relative candidates resolve against the recovered command cwd when the command determines one; otherwise against a usable event cwd; otherwise degrade per *Honest degrade…*; `$PWD` is not a blocking fallback.
- Do **not** rewrite heuristics, URI rules, or hook registration.

Tasks phase owns that spec edit before apply. Implementation still lands only in the Go gate.

## 10. Coordination with codegraph-worktree

`.codegraph/` is git-ignored and is **not** copied by `git worktree add`. This worktree currently has no local index; queries in this session resolved against the main checkout index (acceptable for design: gate sources are unchanged on the branch).

| Phase | Action |
|-------|--------|
| **design** (this phase) | No index required. Prose-only under `openspec/changes/gate-cwd-fidelity/`. |
| **tasks** | Same; no symbol edits. |
| **apply** | Before any `codegraph` explore/query on gate symbols, run `codegraph init` from this worktree root (`git rev-parse --show-toplevel` must be `.worktrees/gate-cwd-fidelity`). After adding `cwd.go` / new symbols, re-init or rely on the watcher; corroborate callers of `ParseEvent`, `Decide`, `BlockMessage` with `grep` (skill freshness rule). |
| **verify** | Same worktree index; do not query the main checkout graph for branch-only files. |
| **Never** | Commit `.codegraph/`; init in `$HOME`. |

CodeGraph does not index `.sh` or this markdown; launcher confirmation stays `grep`.

## 11. Rollout

1. Tasks: include spec MODIFIED in §9.3; list Go fixtures in §7; no launcher/doctor tasks.
2. Apply: red-green on `cwd_test.go` + message tests, then `cwd.go` + `event.go` + `main.go` + `message.go`.
3. Validate: `go test` in `catalog/recipes/worktree-flow/gate/` and `./tests/run.sh` / `./tests/validate.sh` as the repo already does for gate changes.
4. Sync/doctor: no stamp-format change; existing binary rebuild/release path of worktree-flow applies when this ships. Not part of the design’s runtime contract.

## 12. Risks left for tasks

- Exact `DegradeMessage` copy (always vs ask) — behavior is fixed; wording is not.
- Whether `Decision` grows a `RepoRoot` field vs a call-site git memo for `createWorktree` — both satisfy R3.
- Subrepo-primary message fixture may need a topology fixture; if too expensive, unit-test `BlockMessage(..., createWorktree=false)` plus one `Decide` block on a standalone extra primary.
- Pass-2 + `cd` interaction is rare; if tasks drop it, document as residual and still degrade when `S` is `none`.
