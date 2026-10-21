# Go 20 / U7-3 — scope the harness env offer to the recipe that asked for it

`recipe add` offers the project-wide env set where `recipe-add.py` scoped collection to the added
recipe id. The scoping lives in `internal/sync.OfferHarnessEnv` and its collection chain, which the
Python parameterizes and the Go port flattened away.

## Specs

Quoted verbatim from `lib/_internal/env_scaffold.py`, `lib/_internal/recipe-add.py` and
`odd/tasks/go-20-u6-recipe.md`.

- **S1 — the parameter and its semantics.** `offer_harness_env` carries
  `recipe_ids: list[str] | None = None`, and the docstring says: "``recipe_ids`` scopes the
  collection/prompted values to those enabled recipes (only prompted values are merged into
  ai-specs.env). Example generation, the root ``.envrc`` managed block, and direnv handling stay
  global/aggregate." `None` keeps the aggregate behavior (every enabled recipe); a list restricts
  collection to those enabled recipe ids; "A selected-but-disabled recipe contributes nothing."
  An **empty** list is not `None`: `selected = None if recipe_ids is None else set(recipe_ids)`
  collects nothing.
- **S2 — the filter, in this order.** `_mcp_env_declarations` returns "per-preset MCP env
  declarations from enabled, in-scope recipes" and applies `if not entry.get("enabled"): continue`
  **then** `if selected is not None and recipe_id not in selected: continue`. The scope never
  resurrects a disabled recipe and never drops an enabled in-scope one.
- **S3 — the whole collection chain is scoped, not just the first call.** `collect_env_vars(
  project_root, recipe_ids=None)`, `collect_env_allowed(project_root, recipe_ids=None)` and
  `prompt_env_vars(project_root, recipe_ids=None, ...)` all take it, and all three reach
  `_mcp_env_declarations(project_root, recipe_ids=recipe_ids)`. A scoped offer must not prompt a
  value it did not collect, nor preselect `env_allowed` values from an out-of-scope recipe.
- **S4 — what stays global.** The env *example* files, the root `.envrc` managed block and the
  `direnv allow` leg are aggregate regardless of scope (S1's docstring; `write_env`,
  `generate_env_example`, `generate_envrc_example`, `ensure_root_envrc` take no `recipe_ids`).
- **S5 — the call sites.** `lib/_internal/recipe-add.py:273` is the project's only scoped caller:
  `env.offer_harness_env(project_root, recipe_ids=[recipe_id])`. The init wizard's tail passes
  nothing and stays aggregate. In Go today `internal/cli/recipe.go:2800` reads
  `sync.OfferHarnessEnv(root, home, &wizardEnvPrompts{out: stdout, keys: keys}, stdout, stderr)` and
  `internal/cli/initwizard.go:403` is the aggregate one — that split is what must survive.
- **S6 — no aggregate behavior change.** `sync`'s non-interactive env step and the offer's own gate
  (`if len(CollectEnvVars(root, home)) == 0 { return }`) keep today's bytes when no scope is given,
  and `recipe add`'s scoped offer still writes only the prompted values into `ai-specs.env`.
- **S7 — the Go shape.** `nil` scope means aggregate. A non-nil empty slice means nothing is in
  scope, matching S1's `set()`. A variadic parameter cannot express that difference, so it is not
  an option. Changing an exported signature in `internal/sync` is allowed; breaking behavior is not.

## Tasks

| id | what | specs | route |
|---|---|---|---|
| T1 | Thread the scope through the `internal/sync` collection chain with tests | S1-S4, S6, S7 | worker |
| T2 | Scope `recipe add`'s tail to the added id; keep the init wizard aggregate | S5 | worker |
| T3 | Differential against the real `env_scaffold.py` for the scoped and aggregate paths | S1-S5 | worker |
| T4 | Record the unit and the program progress, commit slices, open the PR | — | inline |

## Log

- L1 — User request, verbatim: `si reintentar, si, sigue` — continue the Go 20 program after U7-2,
  whose next item is this unit. The finding that owns it, verbatim from
  `odd/tasks/go-20-u6-recipe.md`: "| `R3-EnvOfferScope` | WARNING | **RECORDED, owner U7**: `recipe
  add` offers the project-wide env set where `recipe-add.py` scoped collection to the added id; the
  scoping lives in `internal/sync.OfferHarnessEnv`, outside this slice's edges |".
- L2 — Recon, so no worker repeats it. Python threads one optional parameter through four functions:
  `offer_harness_env` (:572) → `collect_env_vars` (:90) → `_mcp_env_declarations` (:118, the
  `selected` filter at :140) → `collect_env_allowed` (:186) and `prompt_env_vars` (:496). Go's chain
  is `OfferHarnessEnv` (:907) → `CollectEnvVars` (:329) → `collectEnvVarsInOrder` (:305) and
  `collectEnvAllowed` (:340) and `promptEnvVars` (:596) → `mcpEnvDeclarations` (:214); none of them
  takes a scope. Only two call sites exist (S5's pair), so the change is one parameter threaded
  through six functions and one call site.
- L3 — **The worker (`mvhev3x9-2-gcnt`) timed out at 30 minutes inside a `bash` call, again with the code
  written and no evidence** — the same failure mode as U7-2's worker. It left 5 files (+494/-45):
  `internal/sync/envscaffold.go` (57), `envscaffold_test.go` (309), `internal/cli/recipe.go`,
  `initwizard.go` and `recipe_test.go` (147). Nothing it claims is taken on faith: every step below is
  the parent's own, and T3 is the step a worker most likely died on.
- L4 — **T3 differential: 7/7 scenarios identical** against the real `env_scaffold.py`
  (`collect_env_vars` + `collect_env_allowed`), value for value, on a fixture with two enabled recipes
  (`RECIPE_A_TOKEN`, `RECIPE_B_TOKEN`, each with `env_allowed = ["one","two"]`) and one disabled
  (`recipe-c`): aggregate (`recipe_ids=None`), one id, both ids, **empty list** (nothing, matching
  `set()`), a selected-but-disabled id (nothing), and an unknown id (nothing). Scratch trees in `/tmp`
  plus a temporary in-package test to reach the unexported `collectEnvAllowed`; both deleted after the
  run, never committed. The durable pins are the new Go tests, and
  `TestEnvScaffoldMatchesPythonAuthority` (the aggregate Python-vs-Go differential) still passes,
  which is S6's proof that the unscoped path did not move.
- L5 — Slice boundary: **T1** `internal/sync/envscaffold.go` + `envscaffold_test.go`, **T2** the three
  `internal/cli` files, **T4** this document and the program doc. No slice mixes another unit.
