# Tasks: Recipe Init Prompts

## 1. Tests First (Strict TDD)

- [x] 1.1 Record pre-apply baseline by running `./tests/run.sh`; document whether the known sync-related failures are still present and distinguish them from new #66 failures.
- [x] 1.2 Add schema/parser RED tests for recipes without `[init]`, valid `[init]`, missing `[init].prompt`, unknown init fields, invalid `needs_mcp`, absolute prompt paths, parent traversal, directory prompt paths, and missing prompt files.
- [x] 1.3 Add recipe-read RED tests asserting init metadata appears in JSON for recipes with `[init]` and absent-init behavior is stable for recipes without `[init]`.
- [x] 1.4 Add CLI RED tests for `ai-specs recipe init <id> [path]` covering success for installed recipes, available-but-not-installed recipes, missing recipes, recipes without init workflows, uninitialized projects, missing recipe IDs, extra arguments, and path semantics matching existing recipe commands.
- [x] 1.5 Add init brief RED tests covering recipe identity, install state, existing recipe config state, init prompt content or path, relevant manifest/config context, reviewable next actions, and exit code behavior.
- [x] 1.6 Add idempotency RED tests covering existing `[recipes.<id>]`, existing `[recipes.<id>.config]`, existing config keys, missing config table added once in proposed guidance, no duplicate recipe table proposals, and no duplicate key proposals.
- [x] 1.7 Add config proposal RED tests covering schema-aligned `[recipes.<id>.config]` proposals, unknown or out-of-schema config warning/rejection behavior, and confirmation that init does not claim to satisfy sync-time required-field validation.
- [x] 1.8 Add MCP discovery and redaction RED tests covering configured `needs_mcp`, missing `needs_mcp`, recipe MCP preset guidance, manifest-precedence wording, env-backed secret redaction, literal token/secret/password/key redaction, nested redaction, and no process environment secret resolution.
- [x] 1.9 Add template/override preview RED tests covering missing targets, existing targets, reviewable create/update/skip/diff guidance, and no silent overwrite.
- [x] 1.10 Add no-sync/no-materialization RED tests proving init does not run `ai-specs sync`, does not call recipe materialization, does not copy bundled primitives, and does not update generated agent configs or registries.
- [x] 1.11 Run the new focused tests before implementation and record RED evidence showing failures are caused by missing recipe init behavior, not by unrelated baseline sync failures.

## 2. Schema And Parser

- [x] 2.1 Add an `InitWorkflow` data shape in `lib/_internal/recipe_schema.py` with `prompt`, optional `description`, optional `needs_manifest`, and optional `needs_mcp`.
- [x] 2.2 Extend recipe parsing so `[init]` is optional and recipes without `[init]` preserve existing add, read, and sync behavior.
- [x] 2.3 Validate `[init].prompt` is required when `[init]` exists and is a non-empty string.
- [x] 2.4 Reject unsupported `[init]` fields with explicit field names in validation errors.
- [x] 2.5 Validate `needs_manifest` as an optional boolean and `needs_mcp` as an optional array of non-empty strings.
- [x] 2.6 Validate prompt paths against the recipe directory: relative only, inside the recipe root, not empty, not a directory, and existing as a file.
- [x] 2.7 Extend `lib/_internal/recipe-read.py` so recipe JSON exposes init metadata, using the design-selected stable absent-init representation.
- [x] 2.8 Run schema and recipe-read focused tests and record GREEN evidence for the parser/reader layer.

## 3. CLI And Init Helper

- [x] 3.1 Add recipe command dispatch for `init <id> [path]` in `lib/recipe.sh`, preserving existing `list` and `add` behavior.
- [x] 3.2 Add a dedicated init helper, likely `lib/_internal/recipe-init.py`, and a shell wrapper if consistent with current recipe command structure.
- [x] 3.3 Resolve the project root using existing recipe command path semantics and require `ai-specs/ai-specs.toml` for initialized-project checks.
- [x] 3.4 Resolve catalog recipes, load validated recipe metadata, and fail clearly for missing recipes, invalid init declarations, and recipes without init workflows without mutating files.
- [x] 3.5 Load manifest project, agents, recipes, bindings, recipe config, and MCP context through existing manifest readers where feasible.
- [x] 3.6 Generate an agent-readable initialization brief with recipe identity, init metadata, prompt content or path, install state, existing config state, relevant manifest context, MCP context, template/override preview, warnings, and reviewable next actions.
- [x] 3.7 Detect installed and available-but-not-installed recipes and present appropriate reviewable manifest guidance without appending duplicate `[recipes.<id>]` tables.
- [x] 3.8 Detect existing `[recipes.<id>.config]` and config keys, propose updates instead of duplicate keys, and surface missing required config fields as setup targets without bypassing sync validation.
- [x] 3.9 Implement conservative recursive MCP output redaction for env references and secret-like key names, including recipe MCP presets and project manifest MCP declarations.
- [x] 3.10 Include MCP guidance for configured servers, missing servers, recipe-provided presets, and project-manifest precedence during sync.
- [x] 3.11 Implement template/override preview guidance that reports source, target, target existence, condition, and review action without copying files.
- [x] 3.12 Ensure init is read-only by default: no manifest writes, no template writes, no sync, no materialization, no generated agent config updates, and no registry updates.
- [x] 3.13 Update top-level or recipe help text as needed so users can discover `ai-specs recipe init <id> [path]`.
- [x] 3.14 Run CLI/helper focused tests and record GREEN evidence for init command behavior.

## 4. Docs

- [x] 4.1 Update `docs/recipe-schema.md` with the optional `[init]` table, supported fields, prompt path validation rules, and an example.
- [x] 4.2 Document the relationship between init output and durable `[recipes.<id>.config]` storage.
- [x] 4.3 Document that init is reviewable/read-only by default and separate from sync materialization.
- [x] 4.4 Document MCP discovery and redaction expectations, including guidance-only behavior and manifest-precedence sync semantics.
- [x] 4.5 Document idempotency expectations for existing recipe declarations, existing config keys, and existing template/override targets.
- [x] 4.6 Update top-level help docs or command help text references if the repository documents recipe subcommands outside `docs/recipe-schema.md`.
- [x] 4.7 Add or update documentation tests if the repo has existing docs consistency coverage for recipe schema/help output.

## 5. Validation

- [x] 5.1 Re-run the focused test subset for changed areas and record GREEN evidence, including schema/parser, recipe-read JSON, CLI init, MCP redaction, idempotency, template preview, and no-sync/materialization tests.
- [x] 5.2 Run `openspec validate motor-recipe-init-prompts --strict` and fix only issues related to this change.
- [x] 5.3 Run `./tests/run.sh`; record whether the known pre-existing sync-related baseline failures remain and whether any new #66 failures exist.
- [x] 5.4 Run `./tests/validate.sh` when feasible; if it fails because of pre-existing sync-related baseline issues, document the exact distinction and do not make unrelated fixes without explicit approval.
- [x] 5.5 Update apply-progress evidence during apply with RED/GREEN entries for strict TDD, including command names, exit codes, and concise failure/pass summaries.
- [x] 5.6 Before verify, rerun focused tests, `openspec validate motor-recipe-init-prompts --strict`, `./tests/run.sh`, and `./tests/validate.sh` as feasible, preserving the baseline-vs-#66 failure distinction.
