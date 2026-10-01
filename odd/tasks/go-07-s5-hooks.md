# [Go 07.S5] Runtime hooks rendering — byte-exact Go port of `hooks-render.py`

Card: `[Go 07.S5]` — https://trello.com/c/M6S6dwdj (parent epic `go-single-binary`)
Branch: `change/go-07-s5-hooks` · Worktree: `.worktrees/go-07-s5-hooks`
Base: `80e05c6` (S4 follow-ups merge) · Plan: `odd/tasks/go-07-sync-plan.md` (S5 row)

## Goal

Port `lib/_internal/hooks-render.py` (481 lines; claude, cursor, opencode, pi,
omp writers) to Go in `internal/sync` as a byte-exact library, proven by a
module differential against the REAL Python module, plus the
`hooks-five-runtimes` parity fixture.

## Scope decision (S2/S4 precedent)

`hooks-render.py` has exactly one caller: `lib/sync-agent.sh:538`. The Go spine
still execs `sync-agent.sh` until S15, so S5 ships the Go library + differential
only; no `GO_SYNC_STEP_*` flag, no wiring, `hooks-render.py` stays alive.

## Lessons carried from S4 (apply up front)

- Error paths are part of parity: match exactly which exceptions the Python
  catches (`_load_json_file` swallows `JSONDecodeError`/`OSError` → `{}`; an
  uncaught `UnicodeDecodeError` is rc 1 with nothing further written) and the
  side effects that already happened before the failure.
- `json.loads` literal parity (`NaN`/`Infinity`, lone surrogates, big ints,
  depth) — reuse the S4 decoder/encoder in `internal/sync/mcprender.go`
  instead of writing another one.
- Traceback stderr is not byte-reproducible: pin rc, stdout, written bytes, and
  the exception class (S4 traceback mode).

## Tasks

- [x] T1 — Go port + module differential (all 5 writers, EVENT_MAP, managed-key
  merge/replace, cursor file-write skip warning, TS adapters for opencode/pi/omp,
  env assignments, warnings on stderr, argv/exit-2 contract of `main()` as
  library entry semantics).
- [x] T2 — `hooks-five-runtimes` parity fixture, zero deltas × both gate modes.
- [x] T3 — Evidence + commit ids in this doc.

## Design

- **Library entry**: `RenderHooks(resolvedHooksPath, agent, projectRoot string,
  stdout, stderr io.Writer) int` mirrors `render()` + `main()`'s body. `stdout`
  is always empty (the script only warns on stderr). `main()`'s argv check
  (`Usage:` + exit 2 on `argc != 3`) is not expressible in a library entry and
  stays in Bash until S15 — the caller already has the three resolved values.
- **JSON**: reuses the S4 `mcpParseJSONOrdered` decoder (NaN/Infinity, lone
  surrogates, big ints, depth bound) and `mcpWriteJSONCompact`/`mcpJSONString`/
  `mcpWriteJSONBlock` writers. Only the `sort_keys=True` variant
  (`hooksJSONDumpsSorted`) and a composite-aware compact writer
  (`hooksJSONDumpsCompact`, for non-str TS `MATCHER` literals) are new — both
  callers of the existing writers, not a second encoder.
- **Python type/side-effect parity**: every field access is routed through a
  helper that reproduces the exact Python outcome at the exact access point —
  `hooksRequirePyStr` (f-string/`str()` context → `KeyError` on absence),
  `hooksRequireStr` (`str + x` → `KeyError`/`TypeError`),
  `hooksMatcherTargetsFileWrites` (falsy, or `.split` `AttributeError`),
  `hooksEnvDict` (dict-shape `.items()` `AttributeError`),
  `hooksEnvAssignments`/`hooksEnvAssignmentsList` (`sorted(env)`/`env[k]` list
  emulation), `hooksWriteText` (path/content lone-surrogate `UnicodeEncodeError`
  with Python's open-then-encode side effects).
- **Reused helpers** (no duplication): `pyStr`, `pyStrip`, `mcpJSONTruthy`,
  `mcpAsList`, `mcpIsFile`, `mcpParseJSONOrdered`, `errMCPJSONTooDeep`,
  `mcpJSONString`, `mcpWriteJSONBlock`, `mcpWriteJSONCompact`, `mcpHasLoneSurrogate`,
  `formatPyFloat`.

## Evidence

### RED (Strict TDD — test first)

Command (before `hooksrender.go` existed):

```
go test ./internal/sync/ -run 'TestHooksJSONDumpsSorted|TestHooksRenderDifferential' -count=1
```

Observed failure (intended):

```
internal/sync/hooksrender_test.go:225:8: undefined: RenderHooks
FAIL	ai-specs.dev/ai-specs/internal/sync [build failed]
FAIL
```

### RED — divergence-closing round (KeyError/TypeError/AttributeError)

The pre-fix port coerced malformed hook fields instead of raising. Reinstating
that tolerant behavior for one run (then restoring) failed exactly the new cases:

```
go test ./internal/sync/ -run 'TestHooksRenderDifferential' -count=1
--- FAIL: TestHooksRenderDifferential (4.33s)
    --- FAIL: .../claude_missing_recipe_keyerror
    --- FAIL: .../opencode_missing_id_keyerror
    --- FAIL: .../pi_missing_script_path_keyerror
    --- FAIL: .../omp_missing_recipe_keyerror
    --- FAIL: .../cursor_file-write_skip_missing_recipe_keyerror
    --- FAIL: .../claude_script_path_number_typeerror
    --- FAIL: .../cursor_script_path_number_typeerror
    --- FAIL: .../cursor_matcher_number_attributeerror
    --- FAIL: .../claude_env_number_attributeerror
    --- FAIL: .../opencode_env_list_attributeerror
    --- FAIL: .../pi_env_string_attributeerror
    --- FAIL: .../omp_env_number_attributeerror
    --- FAIL: .../claude_event_list_typeerror
    --- FAIL: .../opencode_event_object_typeerror
    --- FAIL: .../unhashable_event_after_a_written_hook
FAIL   16 subtests
```

The lone-surrogate write path gave a second RED round (the four content cases;
the path cases are also rejected by macOS `open()`, so they only diverge on
Linux and the explicit check keeps them deterministic):

```
    --- FAIL: .../pi_script_path_surrogate_content_encode_error
    --- FAIL: .../omp_script_path_surrogate_content_encode_error
    --- FAIL: .../cursor_script_path_surrogate_content_encode_error
    --- FAIL: .../cursor_env_value_surrogate_content_encode_error
```

### GREEN

```
go test ./internal/sync/ -run 'TestHooksJSONDumpsSorted|TestHooksRenderDifferential' -count=1
  → ok  ai-specs.dev/ai-specs/internal/sync  5.119s
    (79 differential subtests + the sorted-dumps differential, all green)
```

### TRIANGULATE

The differential matrix drives the negative/alternate cases the task required
(unmapped events per harness, cursor file-write skip, every JSON failure mode,
partial writes before an uncaught exception, missing/non-string fields, non-dict
env, unhashable events, lone surrogates, stale adapters, non-dict hook entries).
Independent of the Go unit differential, `python3 -m unittest
tests.test_hooks_render` (the 551-line black-box suite that exercises the real
`sync` → `hooks-render.py` path) stays green.

### Verification (this worktree)

```
go build ./... && go vet ./internal/sync/ && gofmt -l internal/sync
  → build+vet clean, gofmt:[] (empty)
go test ./internal/sync/ -count=1
  → ok  ai-specs.dev/ai-specs/internal/sync  11.015s
go test ./... -count=1
  → ok  all packages (sync 11.344s)
python3 tests/parity/run.py
  → fixtures: 18, failing: 0
    parity summary: gate-absent failing=0, gate-present failing=0 — PASS
python3 -m unittest tests.test_hooks_render
  → Ran 25 tests ... OK (57.4s)
```

Note: `sync` (step 1) is what renders runtime hooks (it materializes the
resolved-hooks temp file and passes `--resolved-hooks` to `sync-agent`);
`sync-agent --all` re-runs the fan-out without a resolved-hooks file. The
fixture therefore measures the rendered artifacts in step 1 and idempotence in
step 2, and both legs are byte-identical in both steps.

### Differential coverage (79 cases)

- All 4 abstract events + an unknown event, run for claude/cursor/opencode/pi/
  omp (unmapped combos: opencode `session-start`/`stop` → warn+skip; unknown
  event → warn+skip).
- Cursor file-write matcher (warn+skip) vs shell matcher; matcher token padding
  vs substring non-match; claude file-write kept.
- `blocking` true/false (data only — it is not consumed by the renderers, but is
  carried in every blob).
- Env with quotes, backslash, non-string values (int/float/bool/null), unicode,
  empty value, escapes, empty map, absent map — per writer family.
- Existing claude settings / cursor hooks.json: foreign keys, previous managed
  entry replaced (not duplicated), user entry preserved, insertion ordering,
  multiple hooks per agent, duplicate JSON keys, `NaN`/`Infinity`/big int/lone
  surrogate round-trips through the sorted writer.
- Failure paths: invalid JSON, non-object JSON, invalid UTF-8 (traceback mode),
  unreadable file (swallowed → overwrite), partial writes before an uncaught
  `UnicodeDecodeError` (cursor wrapper written, then hooks.json fails).
- **Missing keys** (`recipe`/`id`/`script_path`) → `KeyError` rc 1, including the
  cursor skip-warning f-string.
- **Non-string fields**: `script_path` non-str (claude/cursor `TypeError`, TS
  `str()` tolerated), matcher non-str (claude/TS `json.dumps` it, cursor
  `AttributeError`), `recipe`/`id` non-str (`str()` tolerated), non-str matcher
  under a non-pre-tool event (short-circuited in cursor).
- **Non-dict env**: claude/TS `.items()` `AttributeError`, cursor `sorted(env)`/
  `env[k]` (`TypeError`/`IndexError`, or the tolerated `[0,1]` and `[]` shapes).
- **Unhashable event** (list/object) → `TypeError` in `EVENT_MAP.get`, including
  the partial-write case where an earlier hook already wrote settings.json;
  hashable non-string events (`123`, `true`) are merely unknown → warn+skip.
- **Lone surrogates**: `\uXXXX` escapes in raw-interpolated fields →
  `UnicodeEncodeError` (path open fails, or content write leaves an empty file),
  while json.dumps-escaped values (claude settings, TS env lines) are tolerated.
- Stale TS adapters preserved for opencode/pi/omp; stale cursor wrapper kept.
- Missing resolved file, resolved path is a directory, resolved hooks not a
  list, empty hook list, agent without a renderer, non-dict hook entries.

## Quirks reproduced

1. `_load_json_file` exception semantics: `JSONDecodeError` and `OSError`
   (including `PermissionError`, `IsADirectoryError`) → `{}`; a non-dict JSON
   value → `{}`; a missing path (`is_file()` false) → `{}` without reading.
2. `UnicodeDecodeError` is a `ValueError`, NOT an `OSError`, so invalid UTF-8
   propagates uncaught → rc 1, after whatever was already written (the cursor
   wrapper lands before `hooks.json` is read).
3. Depth overflow (`RecursionError`) also propagates uncaught → rc 1; it must
   not degrade to `{}`.
4. `write_text` truncates in place and does NOT change an existing file's mode;
   the cursor wrapper is then `chmod 0o755` (mode bits are compared in the
   differential).
5. `json.dumps(..., indent=2, sort_keys=True)`: recursive key sorting, `{}`/`[]`
   for empty containers, `ensure_ascii=True` (lone surrogates re-emitted as
   `\uXXXX`), `NaN`/`Infinity`/`-Infinity`, big-int literals kept verbatim.
6. Managed-entry replacement only drops entries where
   `isinstance(e, dict)` AND `e.get("_ai_specs_managed") == managed_id`; user and
   foreign entries keep their array positions, and the fresh entry is appended
   last.
7. `hook.get("matcher")` truthiness: empty string, `null`, `0` all omit the
   claude `matcher` key and serialize the TS `MATCHER` as `""`.
8. `_matcher_targets_file_writes` splits on `|`, `str.strip()`s each token, and
   set-intersects `{Edit, Write, MultiEdit, NotebookEdit}` (padding tolerated,
   substrings do not match).
9. `_env_assignments` sorts keys, `str()`s values (`None`→`None`, `True`→`True`),
   and escapes `"` as `\"`; keys are inserted raw.
10. TS adapters stringify env values with `str()` then `json.dumps` each
    (claude instead keeps `{k: str(v)}` and lets `json.dumps` sort it).
11. `_module_script_decl` injects `script_path` raw into the TS `new URL(...)`
    (no JSON escaping) and always resolves `../../` from the module location.
12. Warning text is exact (`  ! ` prefix on stderr, em dash, `'None'` for absent
    recipe/id read through `str()`), and warning order matches hook order.
13. cursor's pre-file-write warning uses `hook.get('matcher','')` (absent → `''`)
    while its outer guard uses `hook.get("matcher","")` truthiness.
14. **`hook['recipe']` / `hook['id']` / `hook['script_path']` / `hook['event']`
    raise `KeyError` when absent** (rc 1). In f-string / `str()` contexts
    (`_managed_id`, `_shim_basename`, `_module_script_decl`, the TS headers, the
    cursor skip message) any present value is `str()`-coerced, but in
    `"$CLAUDE_PROJECT_DIR/" + hook["script_path"]` and
    `"$CURSOR_PROJECT_DIR/" + hook["script_path"]` a present non-str is a
    `TypeError`. Access order is per writer (claude: settings load → recipe/id →
    script_path → env; cursor: event → matcher → recipe/id → script_path → env;
    TS: recipe/id → env → event → script_path), so the first bad field decides
    which exception and how much was written.
15. **A truthy non-str matcher** reaches `matcher.split("|")` only in cursor and
    raises `AttributeError`; claude stores the raw JSON value and the TS
    adapters `json.dumps` it (`1.5` → `1.5`, `["Write"]` → `["Write"]`,
    `true` → `true`). A non-`pre-tool-use` event short-circuits before the
    matcher is touched.
16. **Env typed as a non-dict**: claude/opencode/pi/omp call `env.items()` →
    `AttributeError`; cursor runs `for k in sorted(env): env[k]`, so a number or
    string is a `TypeError`, a list of strings is a `TypeError`, a list with an
    out-of-range int index is an `IndexError`, and a list of valid int indices
    (e.g. `[0, 1]`) is tolerated and rendered as `0="0" 1="1"`. An empty list or
    empty dict is falsy and skipped.
17. **An unhashable event** (JSON list or object) makes `EVENT_MAP.get(event)`
    raise `TypeError` (rc 1) at that hook, after any earlier hook's writes; a
    hashable non-string event (`123`, `true`, `1.5`, `null`) is simply unknown
    and warn-and-skips with its `str()` in the message.
18. **A lone surrogate** (`\uD800`…) in a raw-interpolated field is a
    `UnicodeEncodeError`: in the *path* (`recipe`/`id` → wrapper/TS filename) the
    `open()` fails and no file is created; in the *content* (`script_path` in a
    wrapper/TS header, a cursor env value) `write_text` opens/truncates first, so
    the target is left as an empty file, then rc 1. json.dumps-escaped values
    (claude `settings.json`, TS `ENV` lines) tolerate the same surrogate.
19. **`mcpBigInt` (JSON integers wider than int64)** in the cursor env-list path:
    as a list index it is `IndexError: cannot fit 'int' into an index-sized
    integer` (CPython fits the index into an ssize_t *before* the range check,
    so it wins over "list index out of range"); as a sort operand it compares
    numerically and exactly against int/bool/float (`big.Rat`, since float64
    rounding would flip `10**20+1` vs `1e20`) and against another `mcpBigInt`,
    but against a str it is `TypeError` naming the int type. That message also
    names the compared pair in REVERSE input order (Python's sort compares
    `x1 < x0`: `[1,'a']` → `'str' and 'int'`), and NaN compares equal so the sort
    preserves input order while `+Inf`/`-Inf` compare exactly.

## Divergences / residual risk

**No known divergence from the frozen Python oracle remains.** Every malformed
field named in the closing task is reproduced at the same access point with the
same rc, the same already-written tree, and the same exception class (traceback
mode); every tolerated input produces the same bytes. The only deliberate
boundary is scope, not behavior:

- Review advisories **R3-hooks-bigint-envlist** (see quirk 19),
  **R3-mcpbigint-error-order** (TypeError operands now in Python's reverse-input
  order on the big-int path) and **R3-mcpbigint-naninf-coverage** (NaN/±Inf sort
  pins) are fixed.
- Accepted boundary: with MULTIPLE mismatched pairs in one list, which pair the
  sort compares first — and therefore which TypeError/IndexError is raised —
  depends on the sort algorithm's comparison order, which Go's
  `sort.SliceStable` does not replicate; the exception class and rc still match.
- `main()`'s argv/usage/exit-2 contract stays in Bash until S15 (a library entry
  receives the three already-resolved arguments), so there is nothing left to
  reproduce for that leg.
- Size: T1 is ~1863 lines (port 971 + tests 759 + ref driver 133), above the
  ~1000 aim, because the two generated TS adapters are ~110 lines of exact
  template each and the required matrix now spans 88 differential cases. T1 and
  T2 stay separable (T2 is only the 44-line parity fixture), so the parent can
  make the two planned commits.

## Review focus

- `internal/sync/hooksrender.go` — the five writers' exact bytes, the access
  order per writer, and the `KeyError`/`TypeError`/`AttributeError`/
  `UnicodeEncodeError` mapping.
- `internal/sync/hooksrender_test.go` + `testdata/hooksrender_ref.py` — the
  differential oracle (whole-tree bytes + modes, rc, stdout, stderr).
- `tests/parity/parity.py` — the new `hooks-five-runtimes` fixture references
  the existing catalog `worktree-flow` recipe (no new fixture files).
