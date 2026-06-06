# Delta for runtime-brief-rendering

## ADDED Requirements

### Requirement: Default template pre-enables session-context recipe

The `ai-specs.toml` template (`templates/ai-specs.toml.tmpl`) MUST ship with
`[recipes.session-context]` enabled by default (`enabled = true`). This ensures
`recipe-materialize.py` produces a non-empty `enabled` list on a fresh project
without any user edits.

#### Scenario: Fresh template parse yields session-context enabled

- GIVEN a freshly written `ai-specs.toml` produced from the default template
- WHEN `recipe-materialize.py` builds `resolved-config.json`
- THEN the `enabled` list in the resolved config MUST contain `"session-context"`
- AND `session-context.brief_fragments` MUST be present in the resolved output

#### Scenario: Template default does not include project-specific values

- GIVEN the default template is applied with only a placeholder `PROJECT_NAME`
- WHEN `recipe-materialize.py` resolves the config
- THEN the resolved output MUST NOT contain board IDs, vault scopes, tracker URLs,
  or any other project-specific token beyond `PROJECT_NAME`

---

### Requirement: init renders a non-empty AGENTS.md immediately

After writing `ai-specs.toml`, `ai-specs init` MUST run `recipe-materialize.py`
followed by `agents-render.py --preserve-if-runtime-brief` to produce a
semantically complete `AGENTS.md`. The bare one-line placeholder MUST NOT be
the final init output when the render succeeds.

The rendered brief MUST include at least the baseline behavioral sections
contributed by `session-context`: one `## Workflow Rules` bullet and two
`## Conflict Policy` bullets.

#### Scenario: Fresh init produces non-empty behavioral brief

- GIVEN a new project directory with no existing `ai-specs.toml` or `AGENTS.md`
- WHEN `ai-specs init` completes successfully
- THEN `AGENTS.md` MUST contain a `## Workflow Rules` section with at least one bullet
- AND MUST contain a `## Conflict Policy` section with at least two bullets
- AND those bullets MUST match the fragments declared in
  `catalog/recipes/session-context/recipe.toml [provides.brief]`

#### Scenario: Init render failure falls back to placeholder

- GIVEN a new project directory
- AND `agents-render.py` or `recipe-materialize.py` exits non-zero (e.g. Python
  not found, offline dependency)
- WHEN `ai-specs init` runs
- THEN `AGENTS.md` MUST still be created (with at minimum a one-line placeholder)
- AND `ai-specs init` MUST exit with code 0 (render failure is non-fatal)
- AND an error message MUST be printed to stderr indicating the render was skipped

#### Scenario: Baseline brief contains no project-specific tokens

- GIVEN a freshly initialized project with only the default template
- WHEN `ai-specs init` completes and `AGENTS.md` is inspected
- THEN `AGENTS.md` MUST NOT contain board IDs, vault paths, tracker identifiers,
  or any value that requires a project-specific binding to resolve
- AND all `{config.KEY}` placeholders for unbound keys MUST be absent or verbatim

---

### Requirement: init→sync idempotency

Running `ai-specs sync` after `ai-specs init` on an unmodified manifest MUST
produce a byte-identical `AGENTS.md`. The `--preserve-if-runtime-brief` marker
contract MUST be honored at both init-time and sync-time.

#### Scenario: Second render after init is byte-stable

- GIVEN `ai-specs init` has completed and `AGENTS.md` exists
- AND the manifest has not been modified
- WHEN `ai-specs sync` is run
- THEN the resulting `AGENTS.md` MUST be byte-identical to the file written by init

#### Scenario: User-authored marker prevents re-render

- GIVEN `AGENTS.md` contains the line `<!-- ai-specs:runtime-brief -->`
  (user has opted out of managed rendering)
- WHEN `ai-specs init` is run again or `ai-specs sync` runs
- THEN `AGENTS.md` MUST NOT be modified
- AND both commands MUST exit with code 0

---

### Requirement: Fragment deduplication on additional recipe enable

When a user later enables additional recipes that contribute fragments to the
same sections as `session-context`, the renderer MUST NOT produce duplicate
bullets. Key-based and exact-string deduplication (from the existing
`Fragment deduplication` requirement) applies across `session-context` and
all additionally enabled recipes.

#### Scenario: No duplication when second recipe provides same key

- GIVEN `session-context` is enabled (contributing `conflict-policy-source-authority`)
- AND the user enables a second recipe contributing a fragment with the same
  `key = "conflict-policy-source-authority"`
- WHEN `agents-render.py` renders the manifest
- THEN the `## Conflict Policy` section MUST contain that bullet exactly ONCE
- AND the second recipe's version MUST be silently discarded (first-wins)
