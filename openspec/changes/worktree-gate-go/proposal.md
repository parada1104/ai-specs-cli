# Proposal: autocontained Go worktree gate

- **Change slug**: `worktree-gate-go`
- **Depth**: Full
- **Baseline**: `development` @ `e080483`
- **Worktree**: `.worktrees/worktree-gate-go` on `change/worktree-gate-go`

## Why

The worktree gate is the one runtime component that stands between an agent and an
unauthorized write to a protected branch, and it is the most fragile code in the product.
Today it is a 541-line Bash script that shells out to `python3` **N+2 times per tool call**
(`catalog/recipes/worktree-flow/hooks/worktree-gate.sh:87`, `:289`, `:365` inside the
per-candidate loop at `:537-539`), and each of those Python invocations re-derives the full
Git topology from scratch — including one `submodule status` per declared submodule for
**every ancestor directory** up to `/` (`:412-477`).

Three costs follow, and all three are already visible in the repo history:

1. **Fragility.** The parser cannot even use `python3 -c` because its regexes contain
   literal single quotes (`:82-86`). The most recent gate-adjacent commit (`192fd4e`)
   exists solely to make a sibling gate parse under macOS bash 3.2.
2. **Latency on the hot path.** Every `Edit`, `Write` and `Bash` tool call in every synced
   project pays for multiple interpreter startups plus a Git storm before the agent can
   proceed.
3. **A contract split across three languages.** Enum resolution and block messages in
   Bash, candidate extraction in one Python program, the decision in another. There is no
   single place to read, test or reason about the gate's behavior.

A single static Go binary collapses all three: one process, one memoized set of Git facts,
one language, no interpreter prerequisite, and a decision core that can be unit-tested
directly instead of only through a shell.

## What changes

1. **New source of truth**: a zero-dependency Go module at
   `catalog/recipes/worktree-flow/gate/` implementing the complete gate contract — event
   parsing, candidate extraction, cwd precedence, URI allowlist, topology classification,
   decision and message emission.
2. **The hook becomes a thin launcher.** `hooks/worktree-gate.sh` keeps its filename and
   its materialized path (`ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`) and
   becomes a bash-3.2-safe, ~90-line resolver that `exec`s the binary. Because every
   renderer in `lib/_internal/hooks-render.py` references only `hook["script_path"]`, all
   five harnesses keep working with **zero renderer changes and zero re-render churn**.
3. **Distribution, verification and cache**: a new `lib/_internal/gate_binary.py` resolves
   the host platform, downloads the matching release asset, verifies it against a
   `SHA256SUMS` file **committed in the repo**, and installs it into
   `$AI_SPECS_HOME/cache/bin/worktree-gate/<version>/<goos>-<goarch>/`.
4. **New config `gate_impl`** (`auto|go|bash`, default `auto`) on the `worktree-flow`
   recipe: the rollback lever and the offline escape hatch.
5. **Frozen legacy implementation** at `hooks/worktree-gate-legacy.sh`, reachable only via
   `gate_impl = "bash"`, for exactly one minor release.
6. **Multi-arch build matrix** plus a reproducible `scripts/build-gate.sh` and a CI release
   job that publishes assets and regenerates the committed digests.
7. **Doctor check** `worktree-gate` reporting the resolved implementation, binary version,
   digest state and any fallback that is silently in effect.
8. **Strict-TDD parity suite**: a frozen corpus plus a differential runner that executes
   the Bash reference and the Go binary over the same events and asserts identical exit
   code, stderr text and candidate list.

## Contract (normative, unchanged semantics)

The Go binary MUST be a drop-in replacement for the current script at the process
boundary. The observable contract:

### Invocation

```
worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected "b1 b2"]
              [--version] [--selftest] [--explain]
```

- Event JSON arrives on **stdin**.
- Exit **0** = allow, exit **2** = block. No other exit code may be produced for a
  well-formed invocation; usage errors also exit **0** (fail-open).
- Human-readable block reason and all warnings go to **stderr**. **stdout stays empty**
  except for `--version`, `--selftest` and `--explain`.

### Precedence (bit-for-bit with the current script)

| Setting | Precedence | Invalid input |
|---------|-----------|---------------|
| `gate_mode` | `WORKTREE_GATE_MODE` env → `--gate-mode` (stamped) → `always` | warn on stderr, fall back one level |
| `gate_scope` | `WORKTREE_GATE_SCOPE` env → `--gate-scope` (stamped) → `auto` | warn on stderr, fall back one level |
| `repo_topology` | `--repo-topology` (stamped) **only — no env override** | warn on stderr, fall back to `auto` |
| protected branches | `WORKTREE_GATE_PROTECTED` env → `--protected` → `main development` | treated as a space-separated list |

`gate_mode = off` MUST exit `0` **before** scope or topology are resolved, so an invalid
stamped scope produces no warning when the gate is disabled.

### Fail-open invariants

The gate MUST exit `0` on: unparseable stdin, non-object JSON, no `file_path` /
`notebook_path` and no command string, an empty candidate set, an unusable event `cwd`
(falling back to process cwd), a candidate outside any repository, a candidate inside a
linked worktree, a non-protected branch, unproven topology under an exception, and **any**
internal error including a missing or unusable binary.

### Message parity

Block messages MUST be byte-identical to the current script (`:527-533`), including the
distinct shell-mode and path-mode wordings and the ask-mode bypass hint.

## Scope

### In scope

- Go module implementing the full gate contract, stdlib-only.
- Thin, stamped, bash-3.2-safe launcher preserving `script_path` and the sync stale-copy
  sentinel.
- Binary acquisition: platform detection, checksum verification, cache layout, optional
  local build, offline behavior.
- `gate_impl` config plus manifest/schema validation and wizard/status surfacing where the
  existing gate settings already appear.
- Multi-arch build script and CI release matrix; committed `SHA256SUMS`.
- Doctor check for gate implementation health.
- Parity corpus + differential runner; Go unit tests; distribution tests.
- Docs: `docs/runtime-hooks.md`, recipe `README.md`, `CHANGELOG.md`, `VERSION`.
- Spec delta under `specs/worktree-flow/`.

### Out of scope

- **Any change to gate *policy*.** No new heuristics, no new URI schemes, no new protected
  scopes, no widening or narrowing of what is blocked. This is a re-implementation, not a
  behavior change.
- **Windows.** The gate is wired through POSIX shell launchers and a bash wrapper for
  Cursor; `windows/amd64` is an explicit non-goal for v1.
- **Migrating other hooks.** `tracker-card-gate.sh` (800 lines) and `plan-build-gate.sh`
  (170 lines) stay Bash. If Go proves out here, they are follow-up changes.
- **Replacing `git` with a Go Git library.** The binary shells out to `git` exactly as the
  Python resolver does.
- **Removing the legacy Bash implementation.** Removal is a named follow-up change
  (`worktree-gate-bash-retire`) after one minor release.

## Go distribution decision

**Selected: Option D — fetch + verify + cache, with opt-in local build and `gate_impl` as
the rollback lever.** Options are compared in full in `explore.md`; the summary:

| Option | Verdict | Deciding reason |
|--------|---------|-----------------|
| A — commit binaries to the repo | rejected | `git clone` is the install channel (`install.sh:88`); binaries never delta-compress, so every release adds ~15-25 MB to history for **all** users, including those who never enable the recipe |
| B — build at install/sync | rejected as default | makes Go a hard prerequisite for a Bash+Git+Python CLI; a failed compile silently degrades a safety gate |
| C — fetch verified release asset | **selected as default** | repo stays small, only the host's ~2-3 MB transfers, and the **trust root stays in git** because the expected digest is committed next to the source |
| D — C + B opt-in + Bash lever | **selected overall** | the only option with no hard toolchain prerequisite, a working offline path, and a one-command retreat |

Trust model: the network supplies **bytes**, the repository supplies the **expected
digest**. A downloaded asset whose SHA-256 does not match
`catalog/recipes/worktree-flow/bin/SHA256SUMS` is deleted and never executed.

## Multi-arch matrix

| GOOS | GOARCH | Tier | Notes |
|------|--------|------|-------|
| darwin | arm64 | supported | primary development platform; ad-hoc signature verified on real hardware before release |
| darwin | amd64 | supported | Intel Macs; also serves Apple Silicon under a Rosetta-translated shell, where `uname -m` reports `x86_64` |
| linux | amd64 | supported | `CGO_ENABLED=0` → glibc/musl agnostic |
| linux | arm64 | supported | ARM servers and containers |
| windows | amd64 | **not built (v1)** | non-goal; the launcher and Cursor wrapper are POSIX shell |
| linux | 386, arm/v6, arm/v7 | not built | no demand; unsupported platforms fall back to `gate_impl=bash` or a local build |

Build invariants: `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`,
`-ldflags "-s -w -X main.version=<CLI version>"`. Same inputs → same bytes, so the
committed digests are verifiable by any reviewer with a Go toolchain.

## Compatibility launcher

`hooks/worktree-gate.sh` keeps its name and path and does exactly four things:

1. Carries the stamped values (`gate_mode`, `gate_scope`, `repo_topology`, expected binary
   version) **and the sync stale-copy sentinel** so
   `recipe-materialize.py:494-508` keeps working.
2. Detects the platform with `uname -s` / `uname -m`, bash 3.2 only.
3. Resolves the binary in a fixed order: `$WORKTREE_GATE_BIN` → project-local pin →
   version-keyed cache → legacy Bash implementation (only when `gate_impl` allows) → warn
   once on stderr and exit `0`.
4. `exec`s the binary so stdin passes through untouched and the exit code needs no
   translation.

`exec` matters: `spawnSync(SCRIPT, …)` in the opencode/pi/omp renderers invokes the path
directly with no shell, and Claude/Cursor rely on the exit code. `exec` preserves both with
no wrapper process.

## Migration phases

Each phase is independently reviewable and ends in a green suite. Phase boundaries are the
proposed PR boundaries.

| Phase | Goal | Ends when |
|-------|------|-----------|
| 0 | Go module skeleton, build script, CI matrix, digest file — no wiring, no behavior change | `./tests/validate.sh` green; `go build` produces all four targets |
| 1 | Freeze the Bash reference; build the parity corpus and the differential runner | runner executes both implementations and **fails RED** because the Go decision core is absent |
| 2 | Implement the Go gate to full parity | entire corpus GREEN; Go unit tests cover tokenizer, regex families, path helpers, decision core |
| 3 | Distribution: launcher, stamping, acquisition, verification, cache, `gate_impl`, doctor | a fresh `ai-specs sync` in a scratch project yields a working Go gate; offline and digest-mismatch paths tested |
| 4 | Runtime coverage across all five harnesses + docs + cutover to `gate_impl=auto` | render tests assert `script_path` unchanged; live smoke on at least one harness; docs and CHANGELOG updated |

## Strict TDD and parity plan

`strict_tdd: true` (`openspec/config.yaml:9`), and the reference implementation still
exists, so parity is mechanically provable rather than argued.

- **The corpus is the specification.** A frozen JSON corpus of gate events, derived from
  every scenario already in `tests/test_worktree_gate_hook.py` plus the enum/warning matrix
  plus adversarial tokenizer inputs. Each entry pins expected exit code, stderr text and
  extracted candidate list.
- **Differential runner** (`tests/test_worktree_gate_parity.py`): for every corpus entry,
  run the frozen Bash gate and the Go binary against the same Git fixture and assert
  identical `(exit_code, stderr, candidates)`. `--explain` exposes the candidate list so
  extraction is compared directly, not only through the final verdict.
- **Tokenizer differential**: a generated corpus (including unbalanced quotes, escapes,
  `$'…'`, comments, embedded newlines) asserting the Go tokenizer's output equals
  `shlex.split(cmd, posix=True)` token-for-token, and that a Python `ValueError` maps to an
  empty token list.
- **Regex-family assertions**: one explicit test per interpreter-write family proving it
  still fires after backreferences are replaced by Go-side quote pairing, plus negative
  tests for mismatched quotes.
- **Path-helper tests** on real symlink fixtures (`/tmp` → `/private/tmp`), nonexistent
  tails, and sibling-prefix directories (`/repo` vs `/repo-evil`).
- **Existing suite is the regression floor.** `tests/test_worktree_gate_hook.py` is
  parameterized over both implementations; the Bash parameterization must stay green
  throughout, and the Go parameterization goes RED→GREEN in Phase 2.
- **Go tests are conditional.** `./tests/run.sh` runs `go test ./...` only when `go` is on
  `PATH` and skips loudly otherwise, because the CLI must not acquire a hard Go
  prerequisite.

## Affected areas

| Area | Impact |
|------|--------|
| `catalog/recipes/worktree-flow/` | new `gate/` module, new `bin/SHA256SUMS`, launcher rewrite, frozen legacy script, `recipe.toml` version + `gate_impl` |
| `lib/_internal/recipe-materialize.py` | launcher stamping, `gate_impl` selection, sentinel, acquisition call |
| `lib/_internal/gate_binary.py` | new |
| `lib/_internal/doctor.py` | new `worktree-gate` check |
| `lib/_internal/hooks-render.py` | **no change expected** — asserted by test, not assumed |
| `tests/` | parity runner, distribution tests, parameterized gate tests, Go-conditional runner |
| `docs/`, `README`, `CHANGELOG`, `VERSION` | release documentation |
| CI | new release matrix workflow |

## Risks

| # | Risk | Severity | Mitigation |
|---|------|----------|------------|
| 1 | RE2 has no backreferences; five write-detection regexes rely on `\1`/`\3`. A careless port silently loses or widens detection | **High** | quote pairing becomes an explicit Go-side equality check; one positive and one negative test per family; differential corpus |
| 2 | Go has no `shlex`; tokenizer divergence changes the candidate set | **High** | in-tree tokenizer plus a generated differential corpus against Python `shlex`; any divergence is treated as a defect, and unparseable input maps to fail-open |
| 3 | `filepath.EvalSymlinks` errors where Python `realpath` succeeds → false allow or false block on macOS `/tmp`, `/var` | **High** | dedicated `realpath`-equivalent helper (resolve longest existing prefix, append remainder) with symlink fixtures |
| 4 | Binary missing / unverifiable → gate silently inert | **High** | warn once on stderr per invocation, `doctor` reports ERROR, `gate_impl` fallback to Bash, and fail-open is preserved because a wedged editor is worse |
| 5 | Supply chain: an executable arrives over the network | **High** | digest committed in git is the trust root; mismatch → delete, never execute, fall back; `--selftest` before first use |
| 6 | Reviewer overload — the honest estimate is far above 400 changed lines | **High** | five phases, chained PRs, no phase mixing implementation with distribution |
| 7 | Materialized launcher rejected as "stale" by `recipe-materialize.py:494-508`, leaving old gates in place | Medium | launcher keeps a sentinel; a test asserts an existing pre-Go materialized gate is upgraded, not skipped |
| 8 | `repo_topology` accidentally gains an env override during the port | Medium | explicit negative parity test: `WORKTREE_REPO_TOPOLOGY` in env must have no effect |
| 9 | darwin/arm64 ad-hoc signing from a cross-compile fails to execute | Medium | release gate: run the CI-produced darwin/arm64 asset on Apple Silicon; if it fails, build darwin targets on macOS runners or `codesign -s -` |
| 10 | Per-invocation cost regresses instead of improving | Low | Phase 2 measures both implementations over the corpus; the spec sets a single-process, memoized-Git-facts budget |
| 11 | Cache growth across CLI versions | Low | version-keyed cache paths; `doctor` reports size; pruning is a documented manual `rm -rf` |

## Rollback plan

Four independent levers, cheapest first:

1. **Per invocation** — `WORKTREE_GATE_MODE=off` disables the gate (existing behavior,
   `worktree-gate.sh:65`); `WORKTREE_GATE_BIN=/path/to/binary` pins an arbitrary
   implementation.
2. **Per project** — set `gate_impl = "bash"` in `ai-specs/ai-specs.toml` and run
   `ai-specs sync`: the launcher `exec`s the frozen legacy Bash implementation. No CLI
   downgrade, no network, no binary.
3. **Per CLI install** — `rm -rf $AI_SPECS_HOME/cache/bin/worktree-gate` forces
   re-acquisition; combined with lever 2 it removes the binary from the picture entirely.
4. **Full revert** — revert the change's commits and re-run `ai-specs sync`. Because the
   materialized filename never changed, the previous CLI version re-materializes the Bash
   gate over the launcher with no manual cleanup. If a stale copy is preserved by the
   sentinel guard, `rm ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh && ai-specs
   sync` restores it.

Rollback is verified as a test, not documented as a hope: Phase 3 includes a case that
sets `gate_impl=bash`, syncs, and asserts the legacy implementation answers the corpus.

## Dependencies

- `git >= 2.20.0` — already declared (`recipe.toml:20-26`); unchanged.
- `curl` — required only for binary acquisition, at sync time, not at gate runtime.
- `go >= 1.22` — **contributors and opt-in local builds only**. Never a runtime or install
  prerequisite for users.
- `python3` — still required by the CLI itself; the *gate runtime* stops depending on it.

## Success criteria

1. The full parity corpus produces identical `(exit code, stderr, candidates)` for the Bash
   reference and the Go binary.
2. `tests/test_worktree_gate_hook.py` passes for both parameterizations.
3. `./tests/validate.sh` green; `go test ./...` green where `go` exists, cleanly skipped
   where it does not.
4. A fresh `ai-specs sync` in a scratch project produces a working Go gate on
   darwin/arm64, and a live tool call is blocked on a protected branch and allowed inside a
   linked worktree.
5. `hooks-render.py` output is byte-identical to the pre-change output for all five
   harnesses — asserted by test.
6. Offline sync with no cached binary and no `go` degrades to `gate_impl=bash` with a loud
   warning and a `doctor` ERROR, never to a silently open gate.
7. A digest mismatch never results in execution.
8. `gate_impl=bash` restores the legacy implementation in one sync.
9. Measured per-invocation latency for a four-candidate shell event improves; the figure is
   recorded in the verify report.

## Tracker

- **card_id**: TBD (no card linked yet; `gate_mode = warn` in
  `openspec/config.yaml:88-91` makes this an INFO nudge, not a validity block)
- **url**: TBD

## Plan

1. Phase 0 — Go module skeleton, reproducible build script, CI matrix, digest file.
2. Phase 1 — freeze the Bash reference, build the parity corpus and differential runner
   (RED).
3. Phase 2 — implement the Go gate to full parity (GREEN) with Go unit tests.
4. Phase 3 — launcher, stamping, acquisition, verification, cache, `gate_impl`, doctor.
5. Phase 4 — five-harness coverage, docs, CHANGELOG, VERSION, cutover.

## Artifact path

`openspec/changes/worktree-gate-go/proposal.md`

## Corpus correction

Parity evidence must run against real temporary Git fixtures. Synthetic paths
such as `/repo/src.py` are not acceptable oracle inputs because the current gate
intentionally fails open outside a repository. Corpus entries therefore encode
fixture intent and relative targets; the runner creates the repository,
protected branch, external directory, or linked worktree required by each case.
***
