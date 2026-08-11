# Tasks: autocontained Go worktree gate

Depth: full

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~2,600–3,200 |
| 400-line budget risk | Critical |
| Chained PRs recommended | Yes (mandatory) |
| Suggested split | PR 1 Go skeleton + build + CI → PR 2 parity corpus + differential runner (RED) → PR 3 Go implementation (GREEN) → PR 4 launcher + distribution + doctor → PR 5 harness coverage + docs + cutover |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: Critical
```

### Estimated line budget per PR

| PR | Scope | Est. lines |
|----|-------|-----------|
| 1 | Go module skeleton, `scripts/build-gate.sh`, CI matrix, digest file | ~250 |
| 2 | Frozen reference, corpus fixtures, differential + tokenizer runners | ~700 |
| 3 | Full Go implementation + Go unit tests | ~1,300 |
| 4 | Launcher, stamping, `gate_binary.py`, `gate_impl`, doctor, dist tests | ~650 |
| 5 | Harness verification, docs, CHANGELOG, VERSION, cutover | ~250 |

No PR mixes implementation with distribution. PR 2 must land RED-on-Go by design; that is
the strict-TDD evidence, not an incomplete delivery.

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Buildable Go skeleton + reproducible multi-arch build + CI | PR 1 | zero behavior change; nothing wired |
| 2 | Frozen Bash reference + parity corpus + differential runners | PR 2 | corpus is the specification; RED on Go |
| 3 | Go gate implementation to full parity | PR 3 | highest risk: tokenizer, regex, path helpers |
| 4 | Launcher + acquisition + verification + cache + config + doctor | PR 4 | new subsystem; rollback rehearsed by test |
| 5 | Five-harness verification + docs + release | PR 5 | cutover to `gate_impl=auto` |

---

## Phase 0 — Infrastructure (PR 1)

- [x] 0.1 Create `catalog/recipes/worktree-flow/gate/go.mod` — module
      `ai-specs.dev/worktree-gate`, `go 1.22`, **no `require` block** (zero third-party
      dependencies is a locked decision, D8).
- [x] 0.2 Create `gate/main.go` with flag parsing only: `--gate-mode`, `--gate-scope`,
      `--repo-topology`, `--protected`, `--version`, `--selftest`, `--explain`. Unknown flags
      warn on stderr and exit `0`. stdout stays empty except for `--version`/`--selftest`/
      `--explain`.
- [x] 0.3 `--selftest` compiles every regex, checks `git` is invocable, prints `ok`, exits
      `0`; exits `1` on any failure.
- [x] 0.4 Create `scripts/build-gate.sh`: loop the matrix with
      `CGO_ENABLED=0 GOOS=… GOARCH=… go build -trimpath -buildvcs=false
      -ldflags "-s -w -X main.version=$VERSION"` into `dist/worktree-gate-<os>-<arch>`.
      Read `$VERSION` from `VERSION`. Fail loudly if `go` is absent.
- [x] 0.5 Verify reproducibility: build the same target twice and assert identical SHA-256.
- [x] 0.6 Add the CI release workflow: build all four supported targets, run `go vet` and
      `go test ./...`, emit `SHA256SUMS`, attach assets to the release.
- [x] 0.7 Commit `catalog/recipes/worktree-flow/bin/SHA256SUMS` (text only; the trust root,
      D5) plus a `README` note in that directory stating no binaries are ever committed.
- [x] 0.8 Add `.gitignore` entries for `dist/` and any local build output.
- [x] 0.9 Wire Go into the test scripts, guarded: `./tests/run.sh` runs
      `go test ./catalog/recipes/worktree-flow/gate/...` only when `command -v go` succeeds
      and skips loudly otherwise; `./tests/validate.sh` adds `gofmt -l` under the same guard.
- [x] 0.10 Confirm zero behavior change: `./tests/validate.sh` green, no materialized output
      differs, nothing references the new module yet.

## Phase 1 — Freeze the reference and build the parity oracle (PR 2)

- [x] 1.1 Copy the current gate verbatim to
      `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` (from `development` @
      `e080483`, so URI allowlist, event-cwd precedence and the bash-3.2 fix are all inside
      the reference). Byte-identical except the header comment naming it the frozen
      reference.
- [x] 1.2 Add a test asserting the legacy copy is byte-identical to the reference blob
      recorded in the corpus manifest, so it can never drift silently.
- [x] 1.3 Create `tests/fixtures/worktree-gate-corpus/` with entries shaped
      `{name, event, env, stamped, fixture, expect:{exit, stderr, candidates}}`.
- [x] 1.4 Corpus — path mode: every scenario currently in `tests/test_worktree_gate_hook.py`
      (blocked on protected branch, allowed in linked worktree, allowed outside repo,
      `NotebookEdit`, `notebook_path`).
- [x] 1.5 Corpus — shell mode: redirection (`>`, `>>`, glued `2>>f`, `>&2` excluded), `tee`,
      `tee -a`, `sed -i`, `perl -i`, `cp`, `mv`, and every interpreter-write family (Python
      `open`/`Path.write_text`/`write_bytes`, Node five writers, Ruby `File.write`/
      `File.open`).
- [x] 1.6 Corpus — scrub rules: `.`, `-`, `&2`, `/dev/null`, `/dev/stdout`, `/dev/stderr`,
      `/dev/fd/3`; plus order-preserving dedupe of repeated targets.
- [x] 1.7 Corpus — wrapper prefixes: `sudo`, `env`, `nice`, `time`, `nohup`, `xargs`,
      `command`, and `VAR=value` assignment prefixes.
- [x] 1.8 Corpus — command-source precedence: `tool_input.command` → `.script` → `.cmd` →
      top-level `command` → top-level `script`, including the Cursor native top-level shape.
- [x] 1.9 Corpus — enum matrix: valid/invalid `WORKTREE_GATE_MODE`, valid/invalid stamped
      mode, valid/invalid `WORKTREE_GATE_SCOPE`, valid/invalid stamped scope, valid/invalid
      stamped topology — each pinning the **exact stderr warning text**.
- [x] 1.10 Corpus — `gate_mode=off` short-circuit: with an invalid stamped scope, assert
      exit `0` and **no scope warning** (`off` is resolved before scope, `worktree-gate.sh:65`).
- [x] 1.11 Corpus — negative: `WORKTREE_REPO_TOPOLOGY` in env has **no effect** (topology is
      stamped-only, `:67-72`).
- [x] 1.12 Corpus — URIs: all twelve schemes allowed in path mode; `../`-traversal-masked and
      absolute-masked variants blocked; shell mode never allowlisted; unknown schemes
      (`https://`, `file://`, `custom://`) gated normally.
- [x] 1.13 Corpus — `.claude` exceptions: `*/.claude/settings*.json` and `*/.claude/hooks/*`
      allowed, tested against both raw and absolutized candidates.
- [x] 1.14 Corpus — topology: standalone, `monorepo-apps`, initialized submodule (subrepo),
      superrepo with the `openspec/changes` central exception, subrepo under `superrepo`
      scope, nested/ambiguous submodules → unproven.
- [x] 1.15 Corpus — fail-open set: malformed JSON, top-level JSON array, missing fields,
      unbalanced quotes, target outside any repository, nonexistent ancestor chain.
- [x] 1.16 Write `tests/test_worktree_gate_parity.py`: build the fixture, run **both**
      implementations, assert identical `(exit_code, stderr, candidates)` **and** that each
      matches `expect` — so a shared bug cannot pass as parity.
- [x] 1.17 Parity runner skips the Go half loudly when no binary is available; the
      Bash-vs-`expect` half always runs.
- [x] 1.18 Write `tests/test_worktree_gate_tokenizer.py`: generated command corpus fed to
      `shlex.split(posix=True)` and to the Go tokenizer via `--explain`; assert token-for-token
      equality and `ValueError` → empty list.
- [x] 1.19 **RED evidence**: record that the Go half fails for every corpus entry because the
      decision core does not exist yet. This is the strict-TDD baseline.

## Phase 2 — Go implementation to parity (PR 3)

- [x] 2.1 `config.go` — resolve `gate_mode`, `gate_scope`, `repo_topology` with exact
      precedence and verbatim warning strings; `off` short-circuits before scope/topology.
- [x] 2.2 `event.go` — parse stdin JSON; non-object → fail open; `file_path` /
      `notebook_path` → path mode; else command-string precedence chain → shell mode; extract
      and validate `cwd` (absolute + existing directory, else process cwd).
- [x] 2.3 `tokenize.go` — POSIX shlex-equivalent state machine returning
      `([]string, bool)`; unterminated quote or trailing backslash → `(nil, false)`; `#` at
      token start starts a comment.
- [x] 2.4 Green the tokenizer differential (1.18) before touching extraction — the tokenizer
      is the input to everything else.
- [x] 2.5 `extract.go` pass 1 — segment on `| || && ;`; redirection detection (standalone
      and glued `\d*>>?target`, `>&` excluded); wrapper/assignment skipping to find the
      command word; `tee`, `sed -i`, `perl -i`, `cp`, `mv` operand rules.
- [x] 2.6 `extract.go` pass 2 — the five interpreter-write families with **Go-side quote
      pairing** replacing every `\1`/`\3` backreference (D10). One positive and one negative
      (mismatched delimiters) test per family.
- [x] 2.7 `extract.go` — scrub and order-preserving dedupe matching
      `worktree-gate.sh:90-110` exactly.
- [x] 2.8 `pathutil.go` — `RealPath` (never errors, resolves the existing prefix and appends
      the remainder), `Inside` (component-wise, `false` for non-absolute inputs),
      `ExistingAncestor` (`""` when the walk reaches the root).
- [x] 2.9 `pathutil` tests on real fixtures: symlinked `/tmp` → `/private/tmp`, nonexistent
      tails, `/repo` vs `/repo-evil` sibling prefixes.
- [x] 2.10 `gitfacts.go` — `git -C <dir> …` wrapper conflating all errors to `""`;
      `gitCommon` with the `--path-format=absolute` fallback for old Git; memoization keyed by
      resolved directory.
- [x] 2.11 `gitfacts` test asserting memoization: a four-candidate event issues strictly
      fewer `git` invocations than the Bash implementation, counted via a `git` shim on
      `PATH`.
- [x] 2.12 `topology.go` — `moduleRecords` (nil on ambiguity, skip uninitialized, verify
      owner and expected `.git/modules/<rel>` common dir) and `classify` (walk all ancestors,
      more than one match → unproven).
- [x] 2.13 `decide.go` — existing-ancestor probe, repo containment, `gitDir == common`
      primary-checkout test, protected-branch test, owner × scope decision with the
      `openspec/changes` exception; any internal error → allow.
- [x] 2.14 `message.go` — block and warning strings byte-identical to
      `worktree-gate.sh:527-533`, including the shell/path variants and the ask-mode hint.
- [x] 2.15 `main.go` — wire the pipeline; first blocking candidate wins; `--explain` emits the
      diagnostic JSON on stdout.
- [x] 2.16 **GREEN evidence**: full parity corpus passes on both sides; tokenizer differential
      passes; Go unit tests pass; `go vet` clean.
- [x] 2.17 Parameterize `tests/test_worktree_gate_hook.py` over both implementations; the Bash
      parameterization must remain green.
- [x] 2.18 Measure per-invocation latency and `git` call counts for both implementations over
      the corpus; record the numbers for the verify report.

## Phase 3 — Distribution and configuration (PR 4)

- [x] 3.1 Rewrite `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` as the thin launcher:
      stamped `__WORKTREE_GATE_MODE__`, `__WORKTREE_GATE_SCOPE__`,
      `__WORKTREE_REPO_TOPOLOGY__`, `__WORKTREE_GATE_IMPL__`, `__WORKTREE_GATE_VERSION__`.
- [x] 3.2 Launcher keeps the literal sentinel `stamped_gate_scope="` so
      `recipe-materialize.py:494-508` upgrades existing projects instead of preserving the old
      gate.
- [x] 3.3 Launcher platform detection with `uname -s` / `uname -m`, bash 3.2 only, mapping
      `aarch64`→`arm64` and `x86_64|amd64`→`amd64`; unknown platform → empty target.
- [x] 3.4 Launcher resolution order: `$WORKTREE_GATE_BIN` → project-local
      `ai-specs/recipes/worktree-flow/bin/worktree-gate` → version-keyed cache → legacy Bash
      (when `gate_impl` allows) → warn once on stderr and exit `0`.
- [x] 3.5 Launcher hands off with `exec` (stdin untouched, exit code untranslated); the legacy
      path uses `exec bash "$legacy"`.
- [x] 3.6 `bash -n` clean, and a test asserting the launcher parses under bash 3.2 semantics
      (no `mapfile`, no associative arrays, no `${v,,}`).
- [x] 3.7 Add `[config.gate_impl]` to `recipe.toml` — `enum = ["auto","go","bash"]`,
      `default = "auto"`, help text; bump the recipe version.
- [x] 3.8 Extend `lib/_internal/recipe-materialize.py`: stamp the two new placeholders and
      validate `gate_impl` at sync exactly like `gate_scope` (invalid → `RuntimeError`).
- [x] 3.9 Materialize the legacy script alongside the launcher so `gate_impl=bash` works with
      no network and no binary.
- [x] 3.10 New `lib/_internal/gate_binary.py`: platform detection, cache path
      `$AI_SPECS_HOME/cache/bin/worktree-gate/<version>/<goos>-<goarch>/`, download to a temp
      file in the destination directory, SHA-256 verify against the committed `SHA256SUMS`,
      `chmod 0755`, atomic `os.replace`, then `--selftest`.
- [x] 3.11 Digest mismatch → delete the temp file, warn, **never execute**, record the
      mismatch for `doctor`.
- [x] 3.12 Opt-in local build: `AI_SPECS_GATE_BUILD=1`, or offline with `go` present, builds
      into the same cache layout.
- [x] 3.13 Acquisition never fails `ai-specs sync`: every failure warns and degrades.
- [x] 3.14 Add the `worktree-gate` doctor check with the severity table from design §6.5
      (OK / INFO / WARN / ERROR), including the "gate is silently failing open" ERROR.
- [x] 3.15 `tests/test_gate_binary_dist.py`: `uname` mapping incl. Rosetta `x86_64` on Apple
      Silicon; cache path construction; digest match → install; mismatch → no install and no
      execution; partial download never installed.
- [x] 3.16 Distribution tests for degradation: offline + `gate_impl=auto` → legacy + WARN;
      offline + `gate_impl=go` → ERROR + gate fails open; unsupported platform → WARN.
- [x] 3.17 **Rollback rehearsal test**: set `gate_impl=bash`, sync, and assert the legacy
      implementation answers the full parity corpus.
- [x] 3.18 Sentinel-upgrade test: a project with a pre-Go materialized `worktree-gate.sh` is
      **upgraded** to the launcher, not skipped as stale.
- [x] 3.19 Invalid `gate_impl` test: sync raises with the enum listed in the message.
- [x] 3.20 Smoke: fresh `ai-specs sync` in a scratch project on darwin/arm64 yields a working
      Go gate — blocked write on a protected branch, allowed write inside a linked worktree.

## Phase 4 — Harness coverage, docs and cutover (PR 5)

- [x] 4.1 Test asserting `hooks-render.py` output is **byte-identical** to the pre-change
      output for claude, cursor, opencode, pi and omp (proves `script_path` stability rather
      than assuming it).
- [x] 4.2 Verify the Cursor wrapper still maps exit `2` → `{"permission":"deny"}` with the
      launcher, and that the binary's empty stdout does not degrade the deny message.
- [x] 4.3 Verify `spawnSync(SCRIPT, …)` works with the launcher on opencode/pi/omp (executed
      directly with no shell → shebang and mode 0755 required).
- [x] 4.4 Live smoke on at least one real harness: a blocked write on a protected branch and
      an allowed write inside a linked worktree.
- [x] 4.5 Update `docs/runtime-hooks.md`: the Go implementation, the launcher, `gate_impl`,
      the cache layout, the build matrix, and the unchanged pre-existing coverage gaps
      (Cursor has no pre-file-write hook; opencode `tool.execute.before` misses subagent and
      MCP calls).
- [x] 4.6 Update `catalog/recipes/worktree-flow/README.md`: `gate_impl`, offline behavior,
      rollback levers, `ai-specs doctor` guidance.
- [x] 4.7 Document the contributor build path (`scripts/build-gate.sh`, `go >= 1.22` for
      contributors only, never a user prerequisite).
- [x] 4.8 Release gate: run the CI-produced darwin/arm64 asset on real Apple Silicon to
      confirm the ad-hoc signature; if it fails, build darwin targets on macOS runners or
      `codesign -s -`.
- [x] 4.9 Regenerate and commit `SHA256SUMS` for the release; assert the digests match the
      published assets.
- [x] 4.10 Cut over: default `gate_impl = "auto"`, so a synced project prefers the Go binary.
- [x] 4.11 Update `CHANGELOG.md` and `VERSION`.
- [x] 4.12 Record the follow-up change slug `worktree-gate-bash-retire` with its entry
      criterion (one minor release with Go as default and no field regression).

## Verification

- [ ] V.1 `./tests/run.sh` green.
- [ ] V.2 `./tests/validate.sh` green (`py_compile`, `bash -n`, and `gofmt -l` where Go
      exists).
- [ ] V.3 `go vet ./...` and `go test ./...` green where Go exists; cleanly skipped where it
      does not.
- [ ] V.4 Full parity corpus identical across both implementations.
- [ ] V.5 `tests/test_worktree_gate_hook.py` green for both parameterizations.
- [ ] V.6 Every spec-delta scenario mapped to a passing test, listed explicitly in the verify
      report.
- [ ] V.7 State unavailable quality signals explicitly (no coverage tool, no linter, no type
      checker, no formatter for Python/Bash per `openspec/config.yaml:27-39`).
- [ ] V.8 Record measured latency and `git` call counts for both implementations.
- [ ] V.9 Confirm no production file outside the declared affected areas changed.

## Fixture governance and edit authority

All Phase 1/2 Git fixtures are test resources owned by this change and MUST be
created beneath the authorized worktree (or its test-managed temporary children)
using `TemporaryDirectory()`/`t.TempDir()` during execution. They are not source
repositories, edit roots, or delivery targets. The test runner MUST remove them
when the test exits and MUST NOT create, modify, or inspect the main repository
root as a fixture. The only persistent edits permitted by this change are files
under `.worktrees/worktree-gate-go`.

The fixture builder may create temporary Git repositories, protected branches,
external directories, and linked worktrees, but those resources are ephemeral
inputs to tests. Before apply, verify the actual edit root with
`git rev-parse --show-toplevel`; it MUST resolve to the authorized worktree.

## Tracker

- **card_id**: TBD (no card linked yet)
- **url**: TBD

## Artifact path

`openspec/changes/worktree-gate-go/tasks.md`


## Phase 1 correction — hermetic corpus

- [x] 1.20 Replace illustrative `/repo` paths with fixture metadata and relative
      targets; no corpus case may rely on nonexistent-path fail-open behavior.
- [x] 1.21 Add a temporary Git fixture builder for protected branches, feature
      branches, external paths, and linked worktrees.
- [x] 1.22 Implement the Bash reference runner against hermetic fixtures and
      assert expected exit/stderr; loudly skip Go comparison until the binary
      exists.
- [x] 1.23 Add negative coverage proving a malformed fixture cannot silently
      pass as an outside-repository allow.
***