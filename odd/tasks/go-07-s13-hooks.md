# Go 07 S13 PR4a — Recipe hooks runner

## Objective
Give the root single binary a native recipe hooks runner that mirrors the Python authority
`execute_hooks`, so the sync pipeline can run hooks in-process once S14 wires the orchestration.

This is delivery A of S13 PR4. Delivery B (PR4b) is `build_recipe_mcp` plus the provider marker
resolution; it is deferred because `_resolve_provider_markers` depends on
`lib/_internal/provider_install.py` (597 lines) with no native port, and it needs a minimal native
`resolve_provider` (path lookup plus managed-cache verification) of its own.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is presentation and evidence only and is never written
  by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope
- Base: `de50ef5` on `epic/go-single-binary` (S13 PR1, PR2a, PR2b and PR3 merged).
- Worktree: `.worktrees/go-07-s13-hooks`. Branch: `change/go-07-s13-hooks`.
- Add the root native hooks runner under `internal/sync/`, plus its Python differential.
- **No gate change**: this unit has no gate bridge and no shared-core extraction, so `SHA256SUMS`
  must not change. If you conclude it does, STOP and report.

## In-scope unit (`lib/_internal/recipe-materialize.py`)
`execute_hooks(recipe, merged_config, project_root, cli_home)` :3069-3128. It iterates
`recipe.hooks` in declaration order and dispatches on `hook.action`:
- `validate-config` — for every field of `recipe.config_schema.fields`: a required field missing from
  the merged config raises `RuntimeError` with the exact message
  `recipe '<name>': hook 'validate-config' failed: missing required config field '<key>'`; a field not
  present in the merged config is skipped; the value is stringified with Python `str(value)` and, when
  the key is `board_id` and the string is 8 characters of `str.isalnum()`, raises with the exact
  shortLink message; finally, when `field.validation.regex` is set, a `re.fullmatch` mismatch raises
  with the exact pattern message.
- `bootstrap-board` — computes `_load_project_cache().recipe_skills_root(project_root, cli_home)/<recipe id>`,
  creates it with parents, and writes a `bootstrap-ready` file containing exactly three lines:
  `board_id=<merged_config.get("board_id", "")>`, `default_list=<... or "In Progress">` and
  `epic_list=<... or "Epic">`.
- `link-trello-card`, `sync-card-state`, `comment-verification` — print the deferred-to-agent-runtime
  info line and continue.
- anything else — print the unknown-action warning and continue.

Caller: `materialize_recipes` :4054 (after the recipe loop's config validation and before the AGENTS
step), so ordering matters for S14 and must not change.

## Native building blocks that already exist (reuse, do not re-invent)
- `internal/schema`: `Hook` (`recipe.go:135`), `ConfigField` with `Validation map[string]any`
  (`:149-153`), `ConfigSchema` (`:224`) and `Recipe.Hooks`/`ConfigSchema` (`:264-266`).
- `internal/projectcache.RecipeSkillsRoot(projectRoot, cliHome)` (`:272`).
- Python `str()` parity already exists: `internal/config.PyStr` (`config.go:518`, over the
  unexported `pyStr` at `:141`). Reuse it; do not add a fourth repr port.

## Constraints
- Budget: **900 additions plus deletions** (estimate 300-400). If the work is heading past 900, STOP
  and report the measured number with the remaining work.
- Byte contract: the marker file bytes (including the trailing newline of each line and the exact
  defaults), every info/warning line, every error string, the hook iteration order and the exit
  behavior (an exception fails sync) must match the Python authority.
- **Regex semantics**: Python `re.fullmatch` supports constructs RE2 does not (backreferences,
  lookarounds). Use Go's `regexp` as the primary path and REPORT explicitly, with a test, every
  construct where the two engines could diverge; do not silently accept a different match set.
- No production Python change, no CLI wiring (S14 owns the wiring), no dependency change, no gate
  change, no `SHA256SUMS` change.
- Do not modify `development` and do not publish a release.
- Parent owns the commit, push, PR, merge and cleanup.

## Tasks
- [ ] T1: Read the Python unit and its caller, and write down the exact dispatch table, the exact
      strings and the exact marker bytes before coding.
- [ ] T2: Add the root runner under `internal/sync/` with a root-owned boundary request, reusing
      `internal/schema`, `internal/projectcache.RecipeSkillsRoot` and `internal/config.PyStr`. It must
      perform the real marker write (this unit's effect is a file write, unlike the read-only
      resolved-config authority) and must not be wired into `sync.go`.
- [ ] T3: Add the differential `internal/sync/<runner>_test.go` and `internal/sync/testdata/*_ref.py`:
      the driver must import the real `execute_hooks` from `lib/_internal/recipe-materialize.py` (the
      `bindings_ref.py` shape, using a real fixture catalog recipe via `read_recipe`) and compare the
      written marker bytes, the raised error strings and the printed lines.
- [ ] T4: Cover every action, the declaration-order iteration, the unknown action, and each validation
      branch (required-missing, not-present, board_id shortLink, regex match and mismatch).
- [ ] T5: Run the full check set and report real exit codes.

## Acceptance criteria
- Every covered scenario matches the Python authority byte for byte: marker file contents, error
  strings, info/warning lines.
- A hook action is dispatched in declaration order and an unknown action is skipped with a warning.
- The sync pipeline is untouched (`internal/sync/sync.go` still execs Python) and `SHA256SUMS` is
  unchanged.
- The complete authored candidate stays within the 900-line budget.

## Checks
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l`, `git diff --check`.
- Regression (Python path unchanged): `python3 -m unittest tests.test_recipe_materialize` and, if
  present, the hooks-related module in `tests/`.
- Gate (regression only): `go -C catalog/recipes/worktree-flow/gate test ./... -count=1`.
- Full: `./tests/validate.sh` with durable logs and captured exit evidence (run by the parent, with
  the gate binary built so the bridge classes are not skipped).
- Environment: the real toolchains with the Python 3.14.7 oracle prefixed on PATH
  (`~/.local/share/mise/installs/{go/1.24.13,python/3.14.7}/bin`), never the mise shims.
- Native review uses the exact committed slice base `de50ef5`, not the whole epic.

## Progress
- Slice prepared: worktree `.worktrees/go-07-s13-hooks` and branch `change/go-07-s13-hooks` created at
  base `de50ef5`. The scope was confirmed by the human after the provider-installer dependency was
  measured: hooks now, recipe-mcp plus the minimal provider port next.
- Implemented by a delegated writer: `internal/sync/hooksrunner.go` (235), `internal/sync/hooksrunner_test.go`
  (527 after the repair round) and `internal/sync/testdata/hooksrunner_ref.py` (62). Measured 824 authored
  lines against the 900 budget. Nothing is wired into the pipeline; `SHA256SUMS`, the gate,
  `internal/sync/sync.go`, `internal/schema`, `internal/projectcache` and `internal/config` are untouched.
- Two deliberate deviations, both justified: the runner takes a recipe PATH and re-parses `[config]`
  through `internal/toml` because `schema.ConfigSchema.Fields` is a map and Python iterates declaration
  order (the technique `mergeconfig.go` already uses); and it special-cases a top-level `nil` to `"None"`
  before delegating to `internal/config.PyStr`, because Python `str(None)` is `"None"` while the port's
  `nil` is `""`.
- Independent verification: PASS on the dispatch fidelity, the marker bytes, the field order, the scope
  and the honest differential, plus 13 independently authored scenarios matching the oracle; the
  verifier also proved the `str.isalnum()` approximation is EXACT by brute force over the whole
  code-point space, and that the marker path and the symlink behavior are identical to Python.
- Three real findings, repaired test-side: the regex divergence was pinned in only the fail-closed
  direction while RE2 is also **fail-open** in places (`\p{L}` and a POSIX-class spelling that Python
  treats as a literal set, and the ASCII-only `\W`/`\S`/`\D`/`\B` where Python is Unicode-aware, so the
  native runner can validate a pattern Python would have failed sync on); several Python-only constructs
  (`\uXXXX`, `\Z`, possessive quantifiers, atomic and comment groups, conditionals) and the `$`-before-
  trailing-newline asymmetry of the fullmatch emulation were unlisted; and the reused `internal/config.PyStr`
  renders nested values differently from Python `str()` (`map[a:1]` vs `{'a': 1}`, and a nested `nil` as
  empty). All three are now pinned on both sides with the real Python engine, in tests that state the
  direction and never claim parity. The writer also corrected one wrong claim in the brief: Python 3.14.7
  does not reject `[[:alpha:]]`, it compiles it with a `FutureWarning` as a literal set, so the test pins
  the measured behavior instead.
- Full validation with the gate binary present: `./tests/validate.sh` exit 0 with
  `Ran 2482 tests in 1612.154s` then `OK`, and zero `FAIL:`/`ERROR:` lines. The one-shot gate-suite
  failure the verifier observed once did not reproduce in this run, consistent with its own two clean
  re-runs of the untouched gate module. Evidence: `~/.cache/s13-pr4a-validate.log` and
  `~/.cache/s13-pr4a-validate.exit`.

## Follow-ups (registered here, not fixed in this delivery)
- **Regex engine differences are a recorded divergence, not parity.** RE2 cannot express every Python
  `re` construct, and the native path is fail-open for RE2-only syntax and for ASCII-only character
  classes. A truly faithful implementation needs a Python-compatible regex engine or an explicit
  refusal policy for those constructs; both are out of scope here. The divergence is pinned on both
  sides by `TestRecipeHooksRegexEngineDivergence` and `TestRecipeHooksRegexPythonOnlyConstructs`.
- **`internal/config.PyStr` nested-value gap** (pre-existing, owned by the repr port, not by this
  delivery): `map[string]any` renders as `map[a:1]` and a nested `nil` renders as empty where Python
  `str()` prints `{'a': 1}` and `None`. Pinned by `TestRecipeHooksNestedValueReprDivergence`.
- **`recipe.id` has no charset validation** in either authority, so a `../`-bearing id escapes the
  `.recipe/` marker directory in both (pre-existing, identical on both sides).
- PR4b backlog: `build_recipe_mcp` plus `_resolve_provider_markers`, which needs a minimal native
  `resolve_provider` (path lookup and managed-cache verification) ported from the 597-line
  `lib/_internal/provider_install.py`.

## Rollback
Revert this delivery's root runner, its differential and this document together. PR4b depends on the
shared surface, not on this file.
