# ODD Feature: go-toml-writer

**Card**: GO-07 (Trello `6ab5b1391e6899b98977a006`, https://trello.com/c/GYp046tF)
**Worktree**: `.worktrees/go-toml-writer` — branch `feat/go-toml-writer` from development `bdd8fec`
**Tracker**: no openspec change; card link recorded here (ODD feature, no proposal.md).

## Goal

Strangler slice 4: migrate `update_recipe_config` (lib/_internal/recipe-config-write.py, ~443 lines; writer at 350-430) to a Go decision+write core with a Python fail-open bridge. Unblocks deleting the Go reconcile-stamps Python writer fallback.

## Tasks

- [x] WU1: Go writer core — recipe-config write decision + TOML emission in a new `gate/recipeconfigwrite.go` + `--write-recipe-config` flag in main.go, with parity tests covering: comment/byte preservation, dotted-vs-whole-table refusal, multiline value detection, inline-comment splitting, quoted recipe ids ("my.recipe"), idempotent no-op when values equal. **Status: done (see Evidence log)**
- [ ] WU2: Python bridge — recipe-config-write.py delegates the write to `worktree-gate`, fail-open with a single warning on bridge failure, preserving exact caller surface and per-recipe warning behavior. Callers: recipe-configure.py:473, config_wizard.py:237, recipe-materialize.py:429/456. **Status: implemented, GREEN; awaiting parent review/commit (see Evidence log)**
- [ ] Full validation green (`./tests/validate.sh`), native RDD review per work-unit candidate. **Status: pending**

## Out of scope

- lock writer + copy actuator (next slice), template actuator, hook/gate actuator (later, not combinable).
- execute_hooks scope decision (queued, strangler-closure question).
- advisory-hardening PR (separate small PR; recipe_toml.go hardening + gate-hang-latency timeout + ~8 informational findings).

## Evidence log

- Worktree + branch created from development bdd8fec. Card GO-07 created in In Progress.
- WU1 (TDD): RED observed — `go test ./...` failed with `undefined: runWriteRecipeConfig/tomlKey/pyJSONString/tomlValue/pyValueEqual/pySplitLines/splitInlineComment/valueIsMultiline` before implementation. GREEN observed — full suite `ok ai-specs.dev/worktree-gate` (+ledger), including 26 new parity tests (pure-unit serialization/line-surgery + end-to-end CLI contract through the python3 tomllib seam). Differential parity vs the live Python reference (`update_recipe_config`): 16/16 fixtures byte-identical files, identical exit codes and refusal strings, including Python numeric-equality edges (30 == 30.0, True == 1, [1,2] == [1.0,2.0]).
- WU1 surface: `gate/recipeconfigwrite.go` (embedded tomllib readers: config read, inline-value parse, post-write validation; line surgery port of recipe-config-write.py), `gate/main.go` (`--write-recipe-config`), `gate/recipeconfigwrite_test.go`. Envelope stdin `{"manifest_path", "recipe_id", "values"}`; stdout `{"applied": bool}` exit 0 / `{"error": "<exact reference refusal>"}` exit 2; infra failures on stderr exit 2. Refusal strings preserved byte-exact: whole+dotted conflict, multiline flat/inline, `cannot update '<path>': table [recipes.<id>.config.<root>] not found`, `cannot update '<root>': its value is not an inline table`, `cannot read the existing value for '<root>': <exc>`, `invalid TOML after config write: <exc>`, `cannot serialize NoneType to TOML`.
- WU1 build: `scripts/build-gate.sh` (canonical go1.24.13) rebuilt the 4-arch assets + `dist/worktree-gate-current` (gitignored release outputs); `SHA256SUMS` regenerated with a go-toml-writer history entry; `scripts/verify-gate-sums.sh` → ok, 4/4 digest entries match. `gofmt -l .` empty. `go vet .` clean.
- WU1 accepted corners (documented in code): int-vs-float equality beyond float64 precision approximated; manifest floats inf/nan make the reader's JSON undecodable → fail-closed exit 2; tomllib datetimes surface as strings in the pending-check (reader `default=str`).
- WU2 (TDD): RED observed — `python3 -m unittest tests.test_recipe_config_write tests.test_recipe_config_write_bridge`: 14 errors + 1 failure (`GO_RECIPE_CONFIG_BRIDGE_FALLBACK`/`go_update_recipe_config`/`_update_recipe_config_python` missing; stub refusal envelope not routed). GREEN observed — same command: 29/29 OK. Regression `tests.test_recipe_configure tests.test_config_wizard tests.test_reconcile_stamps_bridge`: 50/50 OK (two pre-existing prompt_toolkit/PromptSession stderr notices, not failures). Caller files recipe-configure.py, config_wizard.py, recipe-materialize.py byte-identical (git status shows only the two WU2 surfaces).
- WU2 surface: `lib/_internal/recipe-config-write.py` (constants `GO_RECIPE_CONFIG_BRIDGE_FALLBACK` / `GO_RECIPE_CONFIG_BRIDGE_TIMEOUT_SECONDS=60`; lazy cached `_load_gate_binary` via `_load_sibling("gate_binary")`; `_config_write_bridge_home()` module-home pattern `parents[2]` as in the reconcile-stamps/orphans bridges; `go_update_recipe_config` bridge; existing writer intact as `_update_recipe_config_python`; `update_recipe_config` keeps its exact signature as dispatcher), `tests/test_recipe_config_write_bridge.py` (authority, refusal-envelope, and fallback contract tests).
- WU2 routing: fail open (one warning → Python writer) for binary-resolution exception/None, OSError/TimeoutExpired/UnicodeError, exit 2 without a valid stdout error envelope, non-JSON stdout, envelope shape mismatch; fail closed (RecipeConfigWriteError with the exact Go string, NO fallback) only for exit 2 + valid `{"error": "<string>"}` stdout envelope. `json.dumps(values)` TypeError is NOT a bridge failure: silent fallback (no warning) before binary resolution so the original `toml_value` TypeError surfaces unchanged. Empty `values` dict short-circuits to a local no-op before any authority (documented in the docstring, pinned by test).
- WU2 note: in dev/test environments without a verified cache binary (`<home>/cache/bin/worktree-gate/...` + `.verified` receipt, or `$WORKTREE_GATE_BIN`), every `update_recipe_config` call now emits one `GO_RECIPE_CONFIG_BRIDGE_FALLBACK` warning to stderr before the Python writer runs. Existing suites still pass; production installs with the acquired cache binary are warning-free.
