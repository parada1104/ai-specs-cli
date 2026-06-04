# rules-audit Specification

## Purpose

Read-only inventory and classification of legacy rule files into the ai-specs target model. Produces an advisory migration plan. No files are written by the Python helper; only the agent writes the plan.

## Requirements

### Requirement: CLI command availability

The system MUST expose `ai-specs rules-audit [path]` as a read-only command.

#### Scenario: Help lists rules-audit

- GIVEN the CLI is installed from this repository
- WHEN a user runs `ai-specs help`
- THEN the help output MUST list `rules-audit` as an available command

#### Scenario: Accepts optional path argument

- GIVEN an ai-specs project path is provided as an argument
- WHEN a user runs `ai-specs rules-audit <path>`
- THEN the command MUST inspect that path instead of the current working directory

#### Scenario: Exits non-zero on missing path

- GIVEN a path argument that does not exist on the filesystem
- WHEN the user runs `ai-specs rules-audit <path>`
- THEN the command MUST exit with a non-zero code
- AND MUST print a human-readable error message

---

### Requirement: Inventory scope

The Python helper (`lib/_internal/rules-inventory.py`) MUST scan the following sources from the target project root:

- `.cursor/rules/**/*.mdc`
- `.cursorrules`
- `AGENTS.md` (presence/absence flag only)
- `ai-specs/ai-specs.toml` (manifest)
- Resolved skills (via `collect_skills()`)
- Recipe catalog entries
- `.atl/skill-registry.md`

#### Scenario: Full-scope scan emits JSON

- GIVEN a project with at least one `.cursor/rules/*.mdc` file, `.cursorrules`, and `ai-specs/ai-specs.toml`
- WHEN `ai-specs rules-audit` runs
- THEN stdout MUST include a valid JSON object with keys for each inventory source
- AND the JSON MUST list each discovered rule item with at minimum its source path and content summary

#### Scenario: Missing sources are represented

- GIVEN a project where `.cursorrules` does not exist
- WHEN `ai-specs rules-audit` runs
- THEN the JSON output MUST include an entry for `.cursorrules` with status `absent`
- AND MUST NOT raise an unhandled exception

---

### Requirement: Read-only invariant

The Python helper MUST NOT write, create, modify, or delete any file on the filesystem.

#### Scenario: No file written after scan

- GIVEN any project directory
- WHEN `lib/_internal/rules-inventory.py` is executed directly
- THEN no file MUST be created or modified in the filesystem
- AND the helper MUST exit without error

#### Scenario: Failure exits non-destructively

- GIVEN an unexpected error during scanning (e.g. unreadable file, malformed TOML)
- WHEN the helper encounters the error
- THEN it MUST exit with a non-zero code
- AND MUST NOT write partial output to any file

---

### Requirement: Mode detection

The command MUST detect whether the project is a legacy project (Mode A) or a greenfield project (Mode B).

| Condition | Mode |
|-----------|------|
| `.cursor/rules/` entries OR `.cursorrules` present | Mode A |
| Neither present AND `AGENTS.md` absent | Mode B |

#### Scenario: Mode A — legacy rules detected

- GIVEN a project with at least one `.mdc` rule file or a `.cursorrules` file
- WHEN `ai-specs rules-audit` runs
- THEN the JSON output MUST include `"mode": "A"`
- AND MUST include a `rule_items` array with one entry per discovered rule

#### Scenario: Mode B — greenfield

- GIVEN a project with no `.mdc` files, no `.cursorrules`, and no `AGENTS.md`
- WHEN `ai-specs rules-audit` runs
- THEN the JSON output MUST include `"mode": "B"`
- AND the output MUST include lightweight recommendations: `ai-specs init`, default recipes for detected stack, and a `[brief]` draft hint

---

### Requirement: Classification taxonomy

Each discovered rule item MUST be assigned to exactly one of the 7 classification buckets. Classifications are SUGGESTIONS and MUST NOT be treated as directives.

| Bucket | Meaning |
|--------|---------|
| `keep_in_brief` | Rule belongs in runtime-brief `AGENTS.md` |
| `enable_recipe` | Rule matches an existing catalog recipe |
| `use_catalog_dep` | Rule can be replaced by a catalog skill dep |
| `create_local_skill` | Rule should become a new local skill |
| `merge_into_skill` | Rule should merge into an existing skill |
| `already_in_atl` | Rule is already represented in `.atl/skill-registry.md` |
| `deprecate_rule_file` | Rule is obsolete and should be removed |

#### Scenario: Each item gets exactly one bucket

- GIVEN a scanned rule item
- WHEN the JSON inventory is produced
- THEN each rule item entry MUST include a `classification` field
- AND its value MUST be one of the 7 defined buckets

#### Scenario: Classifications marked as suggestions

- GIVEN a JSON inventory with classified rule items
- WHEN the agent reads the inventory
- THEN each classification MUST be labeled or documented as a suggestion (e.g. `"classification_is_suggestion": true` at top level)

---

### Requirement: Plan deliverable

Only the agent (via the `/rules-audit` bundled command) MAY write the plan file. The plan MUST be written to `ai-specs/plans/rules-migration-<date>.md`.

#### Scenario: Agent writes dated plan

- GIVEN the `/rules-audit` command is invoked in a supported harness
- WHEN the agent completes classification
- THEN the agent MUST write the plan to `ai-specs/plans/rules-migration-<YYYY-MM-DD>.md`
- AND the plan MUST include all rule items grouped by classification bucket

#### Scenario: Plan file not written by helper

- GIVEN the Python helper is executed
- WHEN it completes
- THEN no file matching `ai-specs/plans/rules-migration-*.md` MUST exist that was not present before execution

---

### Requirement: Bundled command distribution

The `/rules-audit` Markdown command MUST be distributed to all harnesses that use a `commands_dir` (claude, cursor, opencode, omp) via the existing fan-out pipeline.

#### Scenario: Command appears in harness commands directories

- GIVEN `bundled-commands/rules-audit.md` exists in the source
- WHEN `refresh-bundled.py` runs (or `ai-specs sync` triggers it)
- THEN a copy of `rules-audit.md` MUST appear in each harness `commands_dir`

#### Scenario: No pipeline changes required

- GIVEN the existing fan-out pipeline
- WHEN `rules-audit.md` is added to `bundled-commands/`
- THEN the pipeline MUST distribute it without any code changes to the pipeline itself

---

### Requirement: skills-as-rules.md correction

The `bundled-commands/skills-as-rules.md` file MUST be corrected to remove the stale AGENTS.md auto-invoke table claim and align to runtime-brief reality, with a link to `/rules-audit`.

#### Scenario: Stale claim removed

- GIVEN the current `skills-as-rules.md` containing a claim about an AGENTS.md auto-invoke table
- WHEN the correction is applied
- THEN `skills-as-rules.md` MUST NOT contain any reference to an AGENTS.md auto-invoke table

#### Scenario: Link to rules-audit added

- GIVEN the corrected `skills-as-rules.md`
- WHEN a user reads the file
- THEN it MUST include a reference or link to `/rules-audit` for batch migration
