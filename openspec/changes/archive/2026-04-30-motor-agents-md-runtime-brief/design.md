# Design: `motor-agents-md-runtime-brief`

## Context

`AGENTS.md` currently serves two incompatible roles:

1. **Runtime operational brief** — MCPs, safety rules, context sources, workflow rules, project identity.
2. **Auto-generated skill registry** — Exhaustive skills table, Auto-invoke mappings, and per-scope dispatch tables.

This bloats the context window with duplicated information that agents can already discover from the skills directory. It also blocks recipe `trello-mcp-workflow` (#62) from having a clean runtime brief.

The current `lib/_internal/agents-md-render.py` renders a monolithic document with a skills index, Auto-invoke placeholder, and generic footer. The `skill-sync` script (`ai-specs/skills/skill-sync/assets/sync.sh`) writes Auto-invoke rows into `AGENTS.md` by scope. Both need to change.

**Current transitional state**: The existing `AGENTS.md` in this repo already contains a hand-written runtime brief with the marker `<!-- ai-specs:runtime-brief -->`. `skill-sync` already respects this marker and skips Auto-invoke insertion. However, `agents-md-render.py` still renders the old registry model on `ai-specs sync`, so running `sync` in this repo would overwrite the manual brief. That is why the manual brief says "Do not run `ai-specs sync` until #65 is implemented."

**Stakeholders**: Agent users (who read `AGENTS.md` for runtime context), maintainers (who manage `ai-specs.toml`), and the `ai-specs sync` automation.

## Goals / Non-Goals

**Goals:**

- Split the two concerns: `AGENTS.md` becomes a concise runtime brief; skills remain discoverable through the filesystem and a separate registry artifact.
- Redesign `lib/_internal/agents-md-render.py` to generate a runtime brief from `ai-specs.toml`.
- The runtime brief includes: project identity, enabled runtimes, MCPs (redacted), active recipes/bindings/capabilities, safety rules, context source precedence, and workflow rules.
- Generate a separate, idempotent skill/capability registry artifact containing the canonical skill index and Auto-invoke mappings.
- Ensure idempotency: same input → byte-identical output.
- Support the manual runtime-brief marker so hand-maintained `AGENTS.md` files are preserved.
- Redact MCP secrets in generated output (env references shown, literals hidden).
- Update tests and documentation.

**Non-Goals:**

- Changing the `ai-specs.toml` schema (we read existing fields, not add new ones).
- Changing how skills are discovered on the filesystem (local, recipe, dep resolution stays the same).
- Changing how agent configs (`.cursor/mcp.json`, `opencode.json`, etc.) are generated.
- Adding new linting, type-checking, or coverage tooling.
- Supporting non-markdown registry formats (JSON/YAML) — out of scope for this change.
- Modifying the `skill-contract.py` parsing logic.
- Adding a GUI or web interface.

## Decisions

### 1. Keep `AGENTS.md` as a single file

**Decision**: The runtime brief remains `AGENTS.md` at the repo root (and per-subrepo for monorepos). We do not split it into `AGENTS.md` + `AGENTS-runtime.md`.

**Rationale**:
- Agents already know to read `AGENTS.md` at startup. Changing the filename would break agent conventions and require updating every agent's startup path.
- The file is already the canonical "agent instructions" file in the ecosystem. Keeping the name preserves muscle memory.
- The content shift (from registry to brief) is a semantic change, not a structural one. A single file is sufficient for the concise runtime context.

**Alternatives considered**:
- Split into `AGENTS.md` + `AGENTS-runtime.md`: Rejected — adds indirection and breaks existing agent startup behavior.
- Move brief to `ai-specs/runtime-brief.md`: Rejected — agents would not discover it automatically.

### 2. Generate the registry as Markdown (`.md`)

**Decision**: The skill registry artifact is a Markdown file (conventionally `ai-specs/.skill-registry.md`).

**Rationale**:
- Markdown is human-readable in editors and GitHub, and machine-parseable with simple regex/line-based tools.
- Skills are already documented in Markdown (`SKILL.md`). A Markdown registry is consistent with the ecosystem.
- The Auto-invoke table is naturally a Markdown table — no need to invent a schema.
- Derived artifacts in this project are already Markdown-heavy (`AGENTS.md`, `CLAUDE.md`, commands, docs).
- Avoids adding a JSON/YAML parser dependency to the Bash-based `skill-sync` script.

**Alternatives considered**:
- JSON: Rejected — harder to diff, requires jq/JSON parser in Bash, less readable in PR reviews.
- YAML: Rejected — frontmatter already uses YAML, but a full YAML registry adds complexity for little gain.
- TOML: Rejected — good for config, poor for tabular data like Auto-invoke mappings.

### 3. Runtime-brief marker detection and respect

**Decision**: `agents-md-render.py` checks for `<!-- ai-specs:runtime-brief -->` in the existing `AGENTS.md`. If present, it skips overwriting the file entirely. The `skill-sync` script already does this for Auto-invoke insertion; we extend the same logic to the full render.

**Rationale**:
- The marker is already in use in this repo and has proven its value during the transitional period.
- It is an explicit, unambiguous signal that the file is hand-maintained.
- It is invisible in rendered Markdown (HTML comment), so it does not clutter the reading experience.

**How it works**:
1. Before rendering, `agents-md-render.py` checks if `AGENTS.md` exists and contains the marker.
2. If yes: print a message (`Skipping AGENTS.md — runtime-brief marker detected`) and return success.
3. If no: proceed with auto-generation.
4. The generated output itself does NOT include the marker (unless we decide later to mark auto-generated files differently).
5. `skill-sync` continues to skip Auto-invoke insertion when the marker is present.

**Edge case**: If a project wants to switch from auto-generated to manual, they add the marker and hand-edit. If they want to switch back, they remove the marker and run `sync`. This is simple and reversible.

### 4. MCP secrets redaction

**Decision**: When rendering MCP configuration into the runtime brief, literal secret values are replaced with env variable references or placeholder text.

**Rationale**:
- `AGENTS.md` is checked into git. Exposing secrets would be a security risk.
- The manifest already uses env variable references (e.g., `$TRELLO_API_KEY`). We preserve that intent.
- Agents do not need the actual secret values to understand which MCPs are configured.

**How it works**:
- For `env` tables in `[mcp.*]`: values starting with `$` or `${env:...}` are shown as-is. Literal values are replaced with `***` or shown as `{env:VARNAME}` if the key name suggests a secret.
- For `headers` tables (HTTP MCPs): values are redacted similarly.
- The redaction is conservative: when in doubt, redact. The rule is "show env references, hide literals."
- Example: `env = { API_KEY = "$API_KEY" }` renders as `API_KEY: ${API_KEY}`. `env = { API_KEY = "hardcoded" }` renders as `API_KEY: ***`.

**Implementation note**: The redaction logic lives in `agents-md-render.py`. It does not affect agent config generation (`.cursor/mcp.json`, etc.), which continues to interpolate env variables at runtime.

### 5. Idempotency guarantee

**Decision**: Running `ai-specs sync` twice with the same `ai-specs.toml` and skill tree must produce byte-identical `AGENTS.md` and registry artifact.

**Rationale**:
- Idempotency prevents noisy diffs in git and makes CI verification reliable.
- It is a stated requirement in the specs.

**How it works**:
- **Deterministic ordering**: Skills are sorted by `skill_id` (already done in `skill-resolution.py` via `sorted()`). Recipes and deps are also scanned in sorted order.
- **Stable hashing**: Not needed here because we do not use content hashes in the output. If we did, we would use a deterministic hash algorithm.
- **No timestamps**: The generated output must not include generation timestamps, version strings that change per run, or random identifiers.
- **Consistent line endings**: Use `\n` everywhere (already the convention).
- **No filesystem metadata**: Output must not depend on file mtimes or inode numbers.
- **Test**: A dedicated idempotency test runs `sync` twice and compares bytes.

### 6. Registry artifact path

**Decision**: The conventional path is `ai-specs/.skill-registry.md`. It is not configurable in `ai-specs.toml` for this change.

**Rationale**:
- Placing it under `ai-specs/` keeps derived artifacts together.
- The leading dot makes it clear it is a generated/internal file (similar to `.gitignore`).
- Making it non-configurable reduces complexity. If a use case emerges, configurability can be added later without breaking changes.
- It is already gitignored via the existing `ai-specs/.gitignore` pattern or can be added easily.

### 7. Who generates what

**Decision**: `agents-md-render.py` generates `AGENTS.md`. `skill-sync` (or a new Python helper it calls) generates `ai-specs/.skill-registry.md`.

**Rationale**:
- `agents-md-render.py` already has access to the manifest and multi-source skill resolution. It is the right place to generate the brief.
- `skill-sync` already validates skill metadata and builds Auto-invoke tables. It is the right place to generate the registry artifact.
- However, to keep idempotency simple, we may consolidate registry generation into `agents-md-render.py` and have `skill-sync` call it, or have both be called from the main `ai-specs sync` pipeline in a defined order.
- **Proposed flow**: `ai-specs sync` calls `agents-md-render.py` for `AGENTS.md`, then calls `skill-sync` (or a unified helper) for the registry artifact. Both use the same `skill-resolution.py` data source.

### 8. Content contract for the runtime brief

**Decision**: The brief is structured as follows (generated from `ai-specs.toml`):

```markdown
# {project.name} — Agent Instructions
<!-- ai-specs:generated-runtime-brief -->
> **Auto-generated by `ai-specs sync`.** Source of truth: [`ai-specs/`](ai-specs/).

## Project
- Project: `{name}`
- Manifest: `ai-specs/ai-specs.toml`
- Purpose: {description}
- Enabled runtimes: {agents.enabled}
- Integration branch: {branch}

## Runtime MCPs
- `{mcp_id}`: {description or type} ({redacted env/headers})

## Active Recipes / Bindings / Capabilities
- `{recipe_id}`: enabled

## Safety Rules
- {from manifest or default}

## Context Sources
- {ordered list with precedence}

## Conflict Policy
- {from manifest or default}

## Workflow Rules
- {from manifest or default}

## Useful Commands
- {from manifest or default}
```

**Rationale**:
- This mirrors the manual brief already in this repo, proving the structure works.
- Sections are conditional: if a manifest section is absent, the corresponding brief section is omitted or shows a placeholder.
- The `<!-- ai-specs:generated-runtime-brief -->` marker (distinct from the manual `runtime-brief` marker) signals that this file is auto-generated, not manual.

### 9. Registry artifact content contract

**Decision**: The registry artifact contains:

1. **Skill Index**: A Markdown table with columns `Skill`, `Source`, `Description`, `Path`.
   - Source is one of: `local`, `recipe:{id}`, `dep:{id}`.
2. **Auto-invoke Mappings**: A Markdown table with columns `Action`, `Skill`, `Scope`.
   - One row per trigger per scope per skill.
   - Skills without `auto_invoke` or `scope` appear in the index but not in the Auto-invoke table.
3. **Header**: `> **Auto-generated by `ai-specs sync`. Do not edit by hand.**`

**Rationale**:
- Separates discovery (index) from automation (Auto-invoke).
- Scope column in Auto-invoke makes it clear which context each trigger applies to.
- Source column in the index makes it clear where a skill comes from.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| **Breaking change for projects relying on `AGENTS.md` as skill registry** | Migration plan includes a transition period. The old behavior is removed only after projects have adopted the new flow. The `skill-sync` script's scope-mapping logic for monorepos is preserved but targets are updated. |
| **Manual `AGENTS.md` files without the marker get overwritten** | Document the marker prominently. Add a pre-sync warning if `AGENTS.md` exists and is hand-edited but lacks the marker. (Heuristic: file does not contain `<!-- ai-specs:generated-runtime-brief -->` and is older than the manifest.) |
| **Monorepo scope mapping breaks** | The `skill-sync` script's `get_agents_path()` function and monorepo scope logic are preserved. Auto-invoke tables move to the registry artifact, but scope-to-path mapping remains. |
| **Idempotency violations from unstable sorting** | All iteration over filesystem uses `sorted()`. All dict serialization uses deterministic key order (Python 3.7+ guarantees insertion order; we explicitly sort where needed). |
| **Secret redaction is incomplete** | Redaction is conservative: any `env` or `header` value that is not an obvious env reference is redacted. Review redaction logic in security-focused test cases. |
| **Registry artifact grows large** | The registry artifact is excluded from agent startup reads. It is only consulted when an agent explicitly searches for skills. This keeps the critical path lean. |
| **Skill-sync script still in Bash** | The Bash script is modified to write to the registry artifact instead of `AGENTS.md`. If complexity grows, a future change can port it to Python. For now, minimal changes preserve stability. |
| **Subrepo fan-out behavior changes** | Subrepos currently get an `AGENTS.md` with project metadata from root. After this change, they get a runtime brief (same content contract, smaller size). The registry artifact is not fanned out to subrepos because subrepos can resolve skills locally via symlinks. |

## Migration Plan

**Phase 1: Implement the change (this change)**
1. Rewrite `lib/_internal/agents-md-render.py` to emit the runtime brief format.
2. Modify `skill-sync` to emit the registry artifact at `ai-specs/.skill-registry.md`.
3. Update `skill-sync` to skip `AGENTS.md` entirely (not just Auto-invoke) when the runtime-brief marker is present.
4. Add idempotency tests.
5. Update `test_sync_pipeline.py` assertions that check for skills/Auto-invoke in `AGENTS.md` to check the registry artifact instead.
6. Update `test_sync_pipeline.py` assertions for `AGENTS.md` content to match the new brief format.
7. Update README and docs.

**Phase 2: Dogfooding in this repo**
1. After this change is merged to `development`, run `ai-specs sync` in this repo.
2. The manual `AGENTS.md` has the `<!-- ai-specs:runtime-brief -->` marker, so it will be preserved.
3. The registry artifact `ai-specs/.skill-registry.md` will be generated.
4. Review the generated registry for correctness.
5. Update the manual `AGENTS.md` to reference the new registry artifact if desired.

**Phase 3: Rollout to other projects**
1. Projects without a manual `AGENTS.md` will automatically get the new runtime brief on next `sync`.
2. Projects with a manual `AGENTS.md` should add the marker to preserve their file.
3. Update project templates (e.g., `ai-specs init`) to generate a starter `AGENTS.md` with the runtime-brief marker and minimal content, or to generate no `AGENTS.md` and let `sync` create it.

**Rollback**:
- Revert the commit. `agents-md-render.py` returns to the old registry format. `skill-sync` returns to writing Auto-invoke into `AGENTS.md`.
- If the registry artifact was created, it can be deleted or gitignored.

## Open Questions

1. **Should the generated runtime brief include a `<!-- ai-specs:generated-runtime-brief -->` marker?**
   - Yes, to distinguish auto-generated from manual. This is distinct from `<!-- ai-specs:runtime-brief -->` which signals "do not overwrite."

2. **Should the registry artifact be fanned out to subrepos?**
   - Tentative no: subrepos resolve skills via symlinks to root skills. They can run `skill-sync` locally if needed. But if monorepo subrepos need standalone skill discovery, we may need to fan it out later.

3. **Should `ai-specs init` create a starter `AGENTS.md`?**
   - Tentative yes: create a minimal `AGENTS.md` with the `<!-- ai-specs:runtime-brief -->` marker and placeholder sections. This encourages the manual-brief pattern while still allowing `sync` to overwrite if the marker is removed.

4. **What is the exact redaction heuristic for MCP env values?**
   - Values matching `^\$[A-Z_]+$` or `^\$\{env:[A-Z_]+\}$` or `^\$\{[A-Z_]+\}$` are shown as env references.
   - All other values in `env` or `headers` are replaced with `***`.
   - Keys matching `*key*`, `*token*`, `*secret*`, `*password*`, `*api*`, `*auth*` (case-insensitive) are always redacted regardless of value pattern.

5. **Should we add a `ai-specs.toml` field for `integration_branch`?**
   - Currently the manual brief hard-codes it. We can add an optional `[project]` field `integration_branch` so it is rendered automatically. This is a small schema addition; defer to a follow-up if needed.
