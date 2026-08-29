# Exploration: replace the Bash+Python worktree gate with a Go binary

- **Change slug**: `worktree-gate-go`
- **Depth recommendation**: Full
- **Primary target**: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` and its
  distribution pipeline
- **Baseline**: `development` @ `e080483` (`Retire sdd-adaptive-contract and consolidate
  ceremony into plan-build-flow (#190)`)

## Problem

The worktree gate is the safety-critical runtime hook of the `worktree-flow` recipe: it
enforces "exploration ends at the first write" by blocking `Edit|Write|MultiEdit|
NotebookEdit` and best-effort `Bash|Shell|Execute|Terminal` writes that land in the
canonical main worktree while it sits on a protected branch.

It is implemented as a **541-line Bash script wrapping three separately-spawned embedded
Python programs**:

| Block | Lines | Job |
|-------|-------|-----|
| Shell/JSON parser | `87-281` (~195 lines of Python) | `shlex` tokenization, segment split, redirection / `tee` / `sed -i` / `perl -i` / `cp` / `mv` extraction, interpreter-write regex pass (Python/Node/Ruby) |
| Event cwd extractor | `289-307` | validate `event.cwd` is absolute + existing dir |
| Topology + decision resolver | `365-522` (~157 lines of Python) | `git` facts, submodule proof, super/sub-repo classification, `allow` / `block:<branch>` |

Structural consequences, all observable in the current source:

1. **Process storm per tool call.** The parser and the cwd extractor each spawn `python3`
   once (`worktree-gate.sh:87`, `:289`). Then `resolve_and_check` (`:329-535`) spawns
   `python3` **once per candidate path** (`:365`, driven by the loop at `:537-539`). A
   shell command with four write candidates costs **six Python interpreter startups**
   before the gate answers.
2. **Git storm per candidate.** Each resolver invocation redoes the entire topology proof
   from scratch: `rev-parse --show-toplevel`, `rev-parse --absolute-git-dir`,
   `rev-parse --git-common-dir` (twice, with a fallback for old Git), `symbolic-ref`, plus
   — inside `classify` / `module_records` (`:412-477`) — one
   `config --file .gitmodules --get-regexp` and one `submodule status` **per declared
   submodule**, repeated for **every ancestor directory** up to `/`. Nothing is memoized
   across candidates.
3. **Quoting fragility already paid for in blood.** The parser cannot use `python3 -c`
   because the regexes contain literal single quotes (documented at `:82-86`), so the
   program is fed through a quote-delimited heredoc with the event JSON passed as
   `argv[1]`. The most recent gate commit before this baseline (`192fd4e`) exists purely
   to make a sibling gate parse under **macOS bash 3.2**.
4. **One contract spread across three languages.** Enum resolution and warning text live
   in Bash (`_resolve_gate_mode` `:35-47`, `_resolve_gate_scope` `:50-62`,
   `_resolve_repo_topology` `:67-72`); candidate extraction lives in Python; the decision
   lives in a second Python program; the operator-facing block messages live back in Bash
   (`:527-529`). Any behavioral change touches at least two languages.
5. **Undeclared runtime prerequisite.** `python3` is a hard dependency of a *hook* that
   must be maximally robust, yet the recipe declares only `git` in `[[deps.cli]]`
   (`catalog/recipes/worktree-flow/recipe.toml:20-26`).

## Current state — end-to-end pipeline

### 1. Declaration (source of truth)

`catalog/recipes/worktree-flow/recipe.toml:94-108` declares **two** hooks pointing at the
**same** script:

```toml
[[provides.hooks]]
id = "worktree-gate"        # matcher = "Edit|Write|MultiEdit|NotebookEdit"
script = "hooks/worktree-gate.sh"
[[provides.hooks]]
id = "worktree-gate-shell"  # matcher = "Bash|Shell|Execute|Terminal"
script = "hooks/worktree-gate.sh"
```

Tunables: `gate_mode` (`always|ask|off`), `gate_scope` (`auto|superrepo|subrepo`),
`repo_topology` (`auto|standalone|monorepo-apps|monorepo-submodules`), and
`WORKTREE_GATE_PROTECTED` (exported as env because the key is UPPER_SNAKE_CASE).

### 2. Materialization (stamping)

`lib/_internal/recipe-materialize.py`:

- `hook_script_rel_path` (`:428-430`) → **one** copy per basename:
  `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`. Both hook ids collapse onto
  this single file.
- `materialize_hook_script` (`:447-512`) reads the catalog source and replaces
  `__WORKTREE_GATE_MODE__`, `__WORKTREE_GATE_SCOPE__` and `__WORKTREE_REPO_TOPOLOGY__`
  from merged config, validating the enums (`:477-490`), then `chmod 0o755`.
- Stale-copy guard (`:494-508`): if a materialized `worktree-gate.sh` lacks the literal
  sentinel `stamped_gate_scope="`, sync **preserves the existing bytes** and warns. Any
  replacement launcher must keep an equivalent sentinel or that guard must change in
  lockstep, otherwise every already-synced project silently keeps the old gate.

### 3. Runtime wiring

`lib/_internal/hooks-render.py` renders one native artifact per harness, and **every
renderer references `hook["script_path"]`** — the project-relative materialized path:

| Harness | Artifact | Invocation |
|---------|----------|-----------|
| claude | `.claude/settings.json` managed entry | `$CLAUDE_PROJECT_DIR/<script_path>`, exit-code native (`:142-149`) |
| cursor | generated wrapper in `.cursor/hooks/` + `.cursor/hooks.json` | wrapper pipes stdin, maps exit `2` → `{"permission":"deny"}` (`:173-186`) |
| opencode | `.opencode/plugin/*.ts` | `spawnSync(SCRIPT, {input, env})`, `status === 2` → `throw` (`:250-257`) |
| pi | `.pi/extensions/*.ts` | `spawnSync(SCRIPT, …)` → `{block:true}` |
| omp | omp extension, import `@oh-my-pi/pi-coding-agent` | `spawnSync(SCRIPT, …)` → `{block:true}` |

Cursor skips the file-write matcher entirely — no pre-file-write hook exists (`:161-167`)
— and wires only the shell hook.

**Key leverage:** `spawnSync(SCRIPT, …)` executes the path directly with **no shell**, so
`script_path` must stay an executable file with a shebang. Nothing in the renderers cares
*what language* is behind it. Keeping the filename
`ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` therefore means **zero renderer
changes and zero re-render churn** in already-synced projects.

### 4. Distribution channel (the constraint that decides everything)

- `install.sh:88` → `git clone --branch "$AI_SPECS_REF" "$AI_SPECS_REPO" "$AI_SPECS_HOME"`
  (default `~/.ai-specs`).
- `lib/upgrade.sh:206-216` → `git fetch origin main`, ancestor check, merge — with a
  dirty-tree guard that runs `git checkout -- .` when only file **modes** differ
  (`:171-176`).

The CLI ships as a **git checkout**, not an npm tarball or a release archive. There is no
`package.json`, no release-asset pipeline and no CI publishing step today.

### 5. Cache conventions that already exist

`lib/_internal/project-cache.py:4,31,49-57` → cache root is
`$AI_SPECS_HOME/cache/projects/<key>/`, honouring `AI_SPECS_HOME` from env. This is the
precedented home for a binary cache.

### 6. Tests and specs already in tree

- `tests/test_worktree_gate_hook.py` (757 lines) drives `bash <GATE>` with JSON on stdin
  and asserts the exit-code contract; `_stamped_gate(mode)` (`:47-51`) stamps the mode
  placeholder into a temp copy; `_run` / `_run_in` (`:53-85`) control env and process cwd.
  **This harness is already implementation-agnostic in shape** — it executes a path with
  JSON on stdin — which makes it the natural parity driver.
- `openspec/specs/worktree-flow/spec.md` (718 lines) already pins gate behavior:
  *Topology-aware `gate_scope` worktree protection* (`:152`), *Shell Command Write-Bypass
  Detection* (`:415`), *Dual Hook Registration for Shell Matchers* (`:535`), *Ask-mode and
  message parity for shell blocks* (`:576`), *Internal URI allowlist and event-cwd
  precedence* (`:648`).
- Runner `./tests/run.sh` (unittest); validation `./tests/validate.sh` (`py_compile` +
  `bash -n`). **No Go toolchain is referenced anywhere in the repo.**

## The behavior that must be preserved bit-for-bit

This is the parity surface. Every item is observable and therefore testable.

1. **Exit codes**: `0` allow, `2` block. Nothing else may be produced.
2. **Fail-open everywhere.** Parse failure (`:282`, `:284`), empty candidate list
   (`:324`), unusable event cwd, canonicalization/topology exception (`:519-521`), resolver
   crash (`:523`) — all allow.
3. **Enum precedence**, exactly as coded:
   - `gate_mode`: `WORKTREE_GATE_MODE` → stamped → `always`; invalid env warns and falls
     back to stamped; invalid stamped warns and falls back to `always`.
   - `gate_scope`: `WORKTREE_GATE_SCOPE` → stamped → `auto`, same warning shape.
   - `repo_topology`: **stamped only — there is no env override** (`:67-72`). Trivially
     "improved" by accident during a port; that would be a spec violation.
   - `gate_mode = off` short-circuits **before** scope and topology are resolved (`:65`),
     so an invalid stamped scope must stay silent when the gate is off.
4. **Warning text on stderr** for every invalid-enum path (`:40`, `:42`, `:55`, `:60`,
   `:70`).
5. **Candidate extraction set**: redirections (standalone and glued `2>>file`, with `>&`
   excluded), `tee`, `sed -i` / `perl -i`, `cp` / `mv` last non-flag operand, and the
   interpreter-write regex families for Python `open(...,'w|a|x')` /
   `Path(...).write_text|write_bytes`, Node `writeFileSync|appendFileSync|writeFile|
   appendFile|createWriteStream`, Ruby `File.write` / `File.open(...,'w|a|x')`.
6. **Scrubbing / dedupe** (`:90-110`): drop empty, `.`, `-`, `&…`, `/dev/null`,
   `/dev/stdout`, `/dev/stderr`, `/dev/fd/*`; order-preserving dedupe.
7. **Wrapper skipping** for the command word: `sudo env nice time nohup xargs command`
   plus `VAR=value` prefixes (`:112-127`).
8. **Command-string precedence**: `tool_input.command` → `.script` → `.cmd` → top-level
   `command` → top-level `script` (`:256-269`).
9. **Internal URI allowlist**, PATH mode only, defeated by `../` traversal or an absolute
   path after the scheme (`:339-352`).
10. **Local agent-config exception**: `*/.claude/settings*.json`, `*/.claude/hooks/*`
    allowed (`:358-363`), checked against **both** the raw candidate and the absolutized
    form.
11. **Decision core** (`:479-521`): existing-ancestor probe, repo-root containment,
    `git_dir == git_common_dir` (i.e. *this is the primary checkout, not a linked
    worktree*), branch ∈ protected, then owner classification against `gate_scope`, with
    the `openspec/changes` central-planning exception for `superrepo`.
12. **Message text**, verbatim, including the shell-vs-path variants and the ask-mode
    bypass hint (`:527-533`). Message parity is already spec-pinned at
    `openspec/specs/worktree-flow/spec.md:576`.

## Language choice

**Go is a product given for this change**, not relitigated here. Recording why it fits the
constraints and what it costs:

- Single static binary with `CGO_ENABLED=0` → no runtime interpreter, no shared libs.
- Startup ~1-3 ms versus ~25-40 ms per `python3` spawn, and the gate spawns Python `N+2`
  times today. `[INFERENCE]` — order-of-magnitude figures, to be measured in Phase 2.
- Cross-compilation is a first-class single-command operation → the multi-arch matrix is
  cheap.
- Stdlib covers everything needed (`encoding/json`, `os/exec`, `path/filepath`, `regexp`,
  `strings`) → a **zero-dependency** module is realistic.
- **Cost**: a compiled artifact must reach the user's machine through a channel that is
  currently `git clone`, and this repo has never shipped a binary.

## Go distribution options (the real decision)

### Option A — commit prebuilt binaries into the CLI git repo

Track `catalog/recipes/worktree-flow/bin/worktree-gate-<goos>-<goarch>` as repo files.

- **Pros**: zero network at sync; works offline and air-gapped; reuses the existing
  clone/upgrade channel with no new infrastructure; trust root is the repo itself.
- **Cons**: binaries do not delta-compress, so **every release adds a full copy of every
  target to history forever** (~2.5-4 MB × 4-6 targets ≈ 15-25 MB *per release*); clone
  time regresses for every user, including those who never enable `worktree-flow`;
  `lib/upgrade.sh:171-176` already special-cases mode-only dirt and calls
  `git checkout -- .`, exactly the interaction executables invite; build outputs make
  review diffs meaningless.
- **Verdict**: rejected. The cost is permanent and paid by everyone.

### Option B — build from source during install/sync

- **Pros**: nothing binary in git; arch-perfect; trivially auditable.
- **Cons**: makes **Go a hard prerequisite** for a CLI that today needs only Bash, Git and
  Python; first sync becomes a compile; CI images and agent sandboxes frequently lack Go;
  a failed compile means a silently degraded safety gate.
- **Verdict**: rejected as the *default*; retained as an explicit **opt-in / offline
  escape hatch**.

### Option C — fetch a checksum-verified release asset into a cache

Publish per-target binaries as release assets; commit only a small text `SHA256SUMS` to
the repo; sync downloads the single asset for the host platform into
`$AI_SPECS_HOME/cache/bin/worktree-gate/<version>/<goos>-<goarch>/`.

- **Pros**: repo stays small; only the host's ~2-3 MB is transferred; version pinning falls
  out of the CLI version; **the trust root stays in git**, because the expected digest is
  committed next to the CLI source rather than fetched from the network; the
  `$AI_SPECS_HOME/cache/` convention already exists.
- **Cons**: needs network on first sync; needs a release matrix that does not exist today;
  adds failure modes (proxy, rate limit, 403, partial download).
- **Verdict**: **recommended default**.

### Option D — hybrid: C default + B opt-in + Bash as the rollback lever

- C is the default acquisition path.
- `AI_SPECS_GATE_BUILD=1` (or "no network but `go` present") builds from the in-repo source
  into the same cache layout.
- A `gate_impl = auto|go|bash` recipe config keeps the **existing Bash implementation
  reachable for exactly one minor release** as the documented rollback lever for a
  safety-critical component.
- **Verdict**: **recommended**. The only option that keeps the clone small, adds no hard
  toolchain prerequisite, works offline for users who care, and leaves a one-command
  retreat if Go parity misbehaves in the field.

## Concrete portability hazards found by reading the code

Not speculative — each is a specific place where a naive port silently changes behavior.

1. **RE2 has no backreferences.** Five interpreter-write regexes rely on `\1` (one also on
   `\3`) to require matching quote delimiters — `:202`, `:210`, `:216`, `:222`, `:228`.
   Go's `regexp` refuses to compile them. The naive "fix" of replacing `\1` with `["']`
   silently accepts `open("path', 'w')` and changes the matched span. Quote pairing must
   move to an explicit equality check in Go code.
2. **No `shlex` in the Go stdlib.** `pass1` depends on `shlex.split(cmd, posix=True)`
   semantics *including* its `ValueError` on unbalanced quotes → `return []` (`:130-133`),
   which is a fail-open path. Third-party shlex ports differ in detail.
3. **`os.path.realpath` vs `filepath.EvalSymlinks`.** Python's `realpath` resolves as far
   as it can and never fails on a nonexistent tail; `EvalSymlinks` returns an error. The
   resolver calls `realpath` on **targets that may not exist yet** (`:480`, `:430`,
   `:443`). On macOS this is not academic: `/tmp` → `/private/tmp`, `/var` →
   `/private/var`, and the fixtures use `tempfile`.
4. **`os.path.commonpath` vs Go containment.** `inside()` (`:384-388`) returns `False` on
   `ValueError` (mixed absolute/relative). A `strings.HasPrefix` port would additionally
   match `/repo-evil` against `/repo`.
5. **`--path-format=absolute` fallback.** `git_common` (`:400-410`) handles old Git by
   joining a relative result onto the probed root. Must carry over verbatim; the recipe's
   `min_version` is only `2.20.0`.
6. **Empty-vs-error conflation.** `git()` (`:375-382`) swallows `OSError` and
   `CalledProcessError` into `""`, and callers branch on emptiness. Go must reproduce
   *that* conflation rather than improving on it.
7. **bash 3.2 for the launcher.** No `mapfile`, no associative arrays, no `${v,,}`.
   `192fd4e` exists because this was violated once already.
8. **darwin/arm64 executables must carry at least an ad-hoc signature.** Cross-compiled Go
   binaries are ad-hoc signed by the linker, but this must be *verified on real hardware*
   before the release is trusted. `[INFERENCE]` — asserted from Go toolchain behavior, not
   measured in this repo.

## Files likely to change

| Path | Change |
|------|--------|
| `catalog/recipes/worktree-flow/gate/**` (new) | Go module: new source of truth for gate logic |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | becomes the thin, stamped launcher |
| `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` (new) | current implementation, frozen, reachable via `gate_impl=bash` |
| `catalog/recipes/worktree-flow/bin/SHA256SUMS` (new) | committed digests = trust root |
| `catalog/recipes/worktree-flow/recipe.toml` | `gate_impl` config, dep note, version bump |
| `lib/_internal/recipe-materialize.py` | stamp launcher fields, `gate_impl` selection, stale sentinel, acquisition call |
| `lib/_internal/gate_binary.py` (new) | platform detection, cache layout, download, digest verify, optional local build |
| `lib/_internal/doctor.py` | `worktree-gate` health check |
| `scripts/build-gate.sh` (new) | reproducible multi-arch build |
| CI release workflow (new) | build matrix + digest publication |
| `tests/test_worktree_gate_hook.py` | parameterized over both implementations |
| `tests/test_worktree_gate_parity.py` (new) | differential corpus runner |
| `tests/test_gate_binary_dist.py` (new) | platform / cache / verify / fallback |
| `tests/run.sh`, `tests/validate.sh` | run Go tests when `go` is present, skip otherwise |
| `docs/runtime-hooks.md`, `catalog/recipes/worktree-flow/README.md`, `CHANGELOG.md`, `VERSION` | docs + release |
| `openspec/specs/worktree-flow/spec.md` | delta merged at archive |

## Planning depth recommendation

**Full.** Per `openspec/specs/plan-build-flow/spec.md:586-599`, Full minima are `tasks.md`
plus (`proposal.md` or `design.md`) plus at least one spec delta, with `explore.md`
expected as the first chain artifact. Justification:

- Replaces the implementation language of a **safety-critical** runtime component.
- Introduces a **new artifact class** (compiled binary) and therefore a distribution,
  verification and cache subsystem the product has never had.
- Adds a new config surface (`gate_impl`) and a new doctor check.
- Touches all five harness runtimes' assumptions, even where their rendered bytes do not
  change.
- `strict_tdd: true` (`openspec/config.yaml:9`), so parity must be proven RED→GREEN
  against a frozen reference implementation.

## Sequencing note

Two gate-adjacent changes are already in the tree:

- `openspec/changes/worktree-gate-internal-uris/` — URI allowlist + event-cwd precedence;
  the implementation is already merged into the gate source (`59e2ffa`), the spec delta is
  not yet archived.
- `openspec/changes/worktree-gate-bash-coverage/` — directory present with `specs/` only;
  the implementation is archived under
  `openspec/changes/archive/2026-07-31-worktree-gate-bash-coverage/`.

The parity corpus **must be frozen from the post-merge Bash gate**, i.e. from `development`
at or after `e080483`, so URI-allowlist, event-cwd and bash-3.2 behavior are all inside the
reference. Freezing earlier would encode a regression as "parity".

## Open questions for proposal/design

1. Where do release assets live, and is a CI release matrix acceptable as new
   infrastructure? (Proposal assumes **yes**.)
2. Digest verification at acquisition only, or on every invocation? (Design: acquisition
   only; per-invocation is opt-in, because hashing ~3 MB on every tool call defeats the
   performance rationale.)
3. Is Windows in scope? (Design: **no** for v1 — the gate is wired through POSIX shell
   launchers and bash wrappers; recorded as an explicit non-goal.)
4. How long does `gate_impl=bash` survive? (Design: one minor release, with a named
   follow-up change for removal.)

## Risks (summary — expanded in proposal and design)

- Silent heuristic loss from RE2 / shlex / realpath divergence → gate holes that look like
  passing tests.
- Binary unavailable → gate silently inert. Must be loud in `doctor` and on stderr.
- Supply-chain exposure of a downloaded executable.
- Reviewer overload: the honest estimate is well over 400 changed lines, so this must be a
  chained-PR delivery.

## Ready for proposal

**Yes.** The problem is grounded in the current source; the preserved-behavior surface is
enumerated and testable; four distribution options are compared against the real
`git clone` install channel; the recommended direction is **Option D** (fetch + verify +
cache, opt-in local build, `gate_impl` as the rollback lever) at depth **Full** under slug
`worktree-gate-go`.

## Artifact path

`openspec/changes/worktree-gate-go/explore.md`
