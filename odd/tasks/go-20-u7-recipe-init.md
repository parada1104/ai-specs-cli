# Go 20 / U7-2 — `recipe init` in Go

Port `lib/_internal/recipe-init.py` (272 lines) so `recipe init` stops routing to
`lib/recipe-init.sh` and `runShim` loses one of its two surviving consumers.
Read-only verb: it prints a brief and writes nothing.

## Specs

Quoted verbatim from `lib/_internal/recipe-init.py` and `tests/test_recipe_init.py`.

- **S1 — banner and shape.** Stdout begins `# Recipe Init Brief` and contains, in this order,
  `## Recipe` (`- ID: <id>`, `- Name:`, `- Version:`, `- Description:`, `- Install state:`),
  `## Init Workflow` (`- Prompt:`, `- Description:`, `- Needs manifest: true|false`,
  `- Needs MCP: <comma list>|(none)`), `## Prompt Content` (the prompt file's text `rstrip()`ed),
  `## Manifest Context` (`- Project:`, `- Enabled agents:`, `- Active bindings: <n>`),
  `## Config Guidance`, `## MCP Discovery`, `## Template/Override Preview`,
  `## Reviewable Next Actions`. Missing values render as `(unnamed)`, `(none)`.
- **S2 — install state.** Not in the manifest → `available (not installed)`; present with
  `enabled` truthy → `installed`; otherwise → `disabled`.
- **S3 — config guidance.** First line of `## Config Guidance` after the blank is
  `Existing config keys: ` + sorted keys joined by `, `, or `(none)`.
  When the recipe is absent from the manifest, emit `Reviewable manifest addition:` plus a fenced
  block exactly `[recipes.<id>]` / `enabled = true`. With fields, emit
  `Schema-aligned config targets:` and one line per field, sorted by key:
  present → ``- Update existing key `k` if needed; do not append a duplicate `k` key.``;
  required → ``- Add required `k` under `[recipes.<id>.config]`; sync still validates it later.``;
  optional with default → ``- `k` is optional and defaults to `<default>`.``; else ``- `k` is optional.``
  No fields → `Recipe declares no config schema.`
  Keys outside the schema → `Unknown config keys: <sorted, comma>` + `; sync still validates recipe config later.`
  **The brief must never print an actual config assignment** (test asserts `board_id =` is absent).
- **S4 — MCP discovery.** Header line `Reminder: project manifest values take precedence during sync-time MCP merge.`
  (tests match the substring `project manifest values take precedence`); with nothing needed →
  `No MCP servers declared by this init workflow or recipe presets.` Otherwise, for the ordered
  union `init.needs_mcp` + preset ids: `- <id>: configured` or `- <id>: missing`, appending
  `; recipe preset available` when a preset exists; for a configured id emit `  project manifest:`
  and the sorted mapping at 4-space indent; for a preset emit `  recipe preset:` and its mapping.
- **S5 — redaction.** A string value matching `^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$` renders as
  `${NAME}`; any value whose KEY matches `(key|token|secret|password|api|auth)` (case-insensitive)
  renders as `***`, recursively through dicts and lists. Tests assert `${TRELLO_TOKEN}`,
  `literal_secret: ***`, `Authorization: ***`, `API_TOKEN: ***` and the absence of `super-secret`,
  `literal-auth`, `recipe-secret`.
- **S6 — template preview.** Header `## Template/Override Preview`; none →
  `No template/override targets declared.` Otherwise one line per target:
  `- <target>: exists|missing; condition `<condition>`; action: review update/skip/diff|review create.`
  An existing target is **never overwritten** (test compares file bytes before/after).
- **S7 — read-only.** `No files were changed. Init does not run sync or materialize primitives.`
  Tests assert the project tree is unchanged, and that neither `ai-specs/.recipe` nor
  `ai-specs/.tmp/recipe-mcp.json` appears.
- **S8 — errors.** Uninitialized project (no `ai-specs/ai-specs.toml`) → stderr
  `Project not initialized. Run: ai-specs init`, exit **1**, project left untouched.
  Unknown id → `Recipe '<id>' not found in catalog/recipes/`, exit 1.
  No init workflow → `Recipe '<id>' has no init workflow`, exit 1.
  Internal test recipe → `util.internal_test_recipe_message` text containing `internal test fixture`, exit 1.
  Missing id → exit **2** with `missing recipe id` in stderr and the usage text.
- **S9 — catalog resolution.** `$AI_SPECS_HOME/catalog/recipes` wins when that directory exists;
  otherwise the CLI's own `catalog/recipes`. A project-local `catalog/recipes` is **ignored**
  (test rewrites the local recipe to `Local Tracker` / `9.9` and asserts the brief still shows
  `Tracker` / `1.0`). An isolated home with an empty catalog plus one recipe resolves that recipe
  from `$AI_SPECS_HOME`; a project with no local catalog resolves `trello-mcp-workflow` from the
  CLI catalog and prints its `# Recipe Init Contract` prompt text.
- **S10 — the wrapper's own grammar** (from `lib/recipe-init.sh`, which the Go verb replaces):
  `--help`/`-h` anywhere before positionals' exhaustion prints the usage block and exits **0**;
  `--` makes everything after it a positional; the first positional is the id, the second the path,
  a third → stderr `ERROR: unexpected positional argument: <arg>` exit 2; an unknown `-<flag>` →
  stderr `ERROR: unknown flag: <flag>` then `Run 'ai-specs recipe init --help' for usage.`, exit 2;
  no id → stderr `ERROR: missing recipe id` followed by the usage block, exit 2. The path defaults
  to the current directory and is resolved absolute.

## Tasks

| id | what | specs | route |
|---|---|---|---|
| T1 | Port the brief to Go with unit tests for every spec | S1-S8 | worker |
| T2 | Wire `recipe init` to the Go verb and retire the shim route | S7, S8 | worker |
| T3 | Differential against the real Python for the reachable cases | S1-S6, S8, S9 | worker |
| T4 | Record the unit and the program progress | — | inline |

## Log

- L1 — User request, verbatim: `ok continuemos` — continuing `odd/tasks/go-20-python-exit.md`'s
  `## Next` item 2, quoted verbatim: "**`recipe-init.py`** — it belonged to **no unit row** (a gap in
  the U6 slicing, found by a worker); `recipe init` routes straight to `lib/recipe-init.sh` today. It is
  a read-only brief (134 lines plus its wrapper), the cheapest remaining verb."
- L2 — The 134-line figure is wrong: `lib/_internal/recipe-init.py` is **272 lines** plus a 66-line
  wrapper. Recorded so the next estimate is not built on it.
- L3 — `tests/test_recipe_init.py` has **two duplicated methods**: `test_trello_recipe_init_uses_cli_catalog_when_project_has_no_local_catalog`
  is defined twice (the second copy renamed `_v2`) and `test_init_ignores_project_local_catalog_in_favor_of_cli_catalog`
  appears twice with identical bodies. Harmless — unittest keeps the last — but the count of distinct
  cases is 13, not 16.
- L4 — **Writer dispatch refused twice**, reason: "Writers run only for a task of an existing ODD feature
  document (`Spec: odd/tasks/<feature>.md, T<n>` in the task)". The `Spec:` reference WAS in the task text;
  what failed is the document's visibility: the dispatching parent session's cwd is the MAIN worktree
  (`/Users/robert/proyectos/nnodes/ai-specs-cli`, on `development`), where this unit document does not
  exist, so the dispatcher could not see it. Writing the doc into the main worktree was not an option —
  that tree is on the protected branch. The parent then started the port inline, which the human flagged
  as wrong for a unit this size and stopped. **To resume: launch the session with the worktree as cwd**
  (or otherwise make this document visible to the dispatcher), then hand T1-T3 to `gentle-ai-worker` with
  `Spec: odd/tasks/go-20-u7-recipe-init.md, T1-T3`.
- L5 — Recon already done before the stop, so the worker does not repeat it:
  `internal/recipe/list.go` already provides `CatalogDir`, `CatalogHome(home, installRoot)` (S9 exactly)
  and the `enabledRecipes` pattern over `config.LoadManifest`; `internal/schema/recipe.go` has every field
  (`ConfigField{Required,Type,Default,Validation,Enum,HelpText}`, `McpPreset{ID,Config}`, `TemplateRef`,
  `InitWorkflow`); `toml.Table` exposes `Keys/Get/String/Int64/Float64/Bool/Array/Table/Tables/Any`;
  the internal-test guard is `strings.HasPrefix(id, "test-")` at `internal/cli/recipe.go:310`; and
  `lib/_internal/toml-read.py` gives the exact shapes: `read_recipes` → `{enabled: bool|False,
  version: str, config: dict}`, `read_mcp` → `_normalize_mcp_server` emitting command/args/env/timeout/
  enabled plus extras, `read_bindings` → top-level `[[bindings]]` filtered to string capability+recipe.
- L6 — User correction, verbatim: `esperame guarda estado voy a reiniciar gentle-shell porque estas
  haciendo cosas inline que segun yo no deberian ir inline`, then `ok continuemos con lo que nos
  quedamos`. T1-T3 route to `gentle-ai-worker`, not inline. The worker runs in this worktree
  (`.worktrees/go-20-u7-recipe-init`) so the unit document is visible to the dispatcher, which is the
  L4 blocker.
- L7 — **The worker timed out after 30 minutes still inside a `bash` call**, but it had already written
  `internal/recipe/init.go` (696 lines), `internal/recipe/init_test.go` (468) and the four CLI edits. It
  returned no evidence, so nothing it claims was taken on faith: the parent rebuilt every step —
  `go build ./...`, `go vet`, `go test ./internal/recipe`, `go test ./internal/cli`, then the T3
  differential against the real Python, which is where the worker most likely died.
- L8 — **T3 differential: 16/16 byte-identical** (stdout, stderr and exit code) between
  `lib/recipe-init.sh` + `recipe-init.py` and the built binary, on a scratch install
  (`lib/_internal` + `lib/recipe-init.sh` + `catalog/recipes` copied to a temp home, `go build` into
  `<home>/bin/ai-specs`, `AI_SPECS_HOME` pinned for both, one isolated project tree per case):
  installed recipe with config + bindings + redaction, absent recipe (reviewable addition), path
  defaulting to cwd, unknown id, no init workflow, internal `test-` fixture, uninitialized project,
  `--help`, `-h` after a positional, `--` making the rest positional, a lone `-`, an unknown flag, a
  third positional, the missing id, an empty positional after `--`, and a project-local
  `catalog/recipes` being ignored. The script is scratch (`/tmp`), never committed: the durable
  byte pins are the Go tests plus `TestRecipeDispatcherDifferentialAgainstShell`, which already drives
  the shell wrapper for the missing-id arm.
- L9 — **Two divergences the differential could not catch were found by reading, one of them fixed.**
  (a) `RunInit` treated a lone `-` as a positional; `lib/recipe-init.sh` matches `-*`, so `-` is
  `ERROR: unknown flag: -` (exit 2). Fixed, pinned by a subtest that was observed RED first. (b) The
  wrapper's `TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"` also refused a path that does not exist or is
  not a directory, with bash's own message on stderr (exit 1). The port resolves the path with
  `filepath.Abs` and never enters it, so a bogus path now reads `Project not initialized. Run:
  ai-specs init` (still exit 1, still nothing written). Recorded as a deviation rather than reproduced:
  the `cd` existed only to resolve the path, and S8 does not specify a message for it.
- L10 — Also recorded: the `activeBinding` counter reads `[[bindings]]` through the *array-of-tables*
  shape (`[]*toml.Table`), which is what this parser emits; the original `Array`-first branch was
  dead. Collapsed to one helper, and a binding whose `recipe` key is missing is proven not to count.
- L11 — **Slice boundary, recorded because the commit hook asks for it.** The hook counts the whole
  branch against `origin/main` (105 commits, ~150k lines: the entire epic) and therefore always fires on
  `epic/go-single-binary`. This unit's own slices are: **T1** `internal/recipe/init.go` +
  `init_test.go` (1164 lines, one artifact — the brief and its grammar — with the review map in the
  commit body), **T2** the four `internal/cli` files (~105 lines), **T4** this document and the program
  doc. No slice mixes another unit's behaviour.
- L12 — **`./tests/validate.sh` green on the final tree** (exit 0, 32m21s): 31 Go packages ok, 0 FAIL; the
  differential parity harness PASS (gate-absent and gate-present, 0 failing); 2405 Python tests OK with
  165 skipped. `PATH` prefixed with the python3.14 bridge as the program requires.
- L13 — **The user asked for a native review of the T1 candidate and the consent gate declined it.** The
  candidate was frozen as the real slice (`ff5e81b..d5c4dd4`, 2 files, 1164 lines, workspace projection,
  base `ff5e81b` detached checkout) with lenses `risk`, `readability`, `reliability`; START returned
  `consent-declined-this-candidate` with `lineage_created: false` and `mutation_performed: false`, so
  nothing was created, nothing was burned, and no verdict exists. A decline is candidate-scoped, not the
  clone's switch. The tier path is unchanged and holds on its own: the differential (16/16), the Go
  tests, and `validate.sh` are the evidence, and `assess` asked for no independent verifier.
