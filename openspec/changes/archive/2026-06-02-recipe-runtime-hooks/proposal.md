# Proposal: Recipe-declared runtime hooks, distributed to every harness

## Intent

Recipes can ship `skills`, `commands`, `mcp`, `templates`, and `docs` — but **not
runtime lifecycle hooks** (e.g. a `PreToolUse` guard that blocks a write). Today
the only way to add one is a hand-wired, Claude-only script in
`.claude/settings.json`. That is wrong on three counts:

- **Not a product primitive**: hooks live outside the recipe contract, so they
  are not declared, versioned, validated, or materialized like every other
  primitive. They "can't be defined just any way" — they must belong to a recipe.
- **Single-harness**: a `.claude/settings.json` script only works for Claude
  Code. `ai-specs`' whole premise is one manifest fanning out to every enabled
  harness (claude, cursor, opencode, pi).
- **Not reproducible**: `.claude/settings.json` is gitignored and machine-local,
  so the enforcement does not travel with the repo.

This change makes runtime hooks a first-class `[provides.hooks]` recipe primitive
that `ai-specs sync` **distributes to every enabled harness in its native
format**, from a single script the recipe author writes once.

> Distinct from `sync-hooks`. The existing `[[hooks]]` (`event = "on-sync"`,
> `action = "validate-config"`, …) run **during `ai-specs sync`** materialization.
> This proposal is about **agent-runtime** hooks that fire while the coding agent
> runs (per tool call, session, stop). Different lifecycle, different mechanism.

## Scope

### In Scope
- **New `[provides.hooks]` primitive** in `recipe.toml`: array of tables with
  `id`, `event` (abstract), `script` (path inside the recipe), optional
  `matcher`, `blocking` (bool), `description`.
  - **Placement:** hooks are declared **only** in a recipe's `recipe.toml`,
    exactly like every other `[provides.*]`. They are **never** declared in the
    project manifest (`ai-specs/ai-specs.toml`); that file only *enables* the
    recipe. Tunable hook values (e.g. protected branches for `worktree-gate`)
    ride the existing `[config.*]` → `[recipes.<id>.config]` override path — the
    declaration stays in the recipe, only its values can be overridden.
- **Abstract event vocabulary** owned by the product, mapped to each harness's
  native event name. v1 set: `pre-tool-use`, `post-tool-use`, `session-start`,
  `stop`.
- **Normalized script contract**: the hook script reads a normalized JSON event
  on stdin; `exit 0` = allow, `exit 2` = block (stderr surfaced to the agent).
  One contract for all harnesses.
- **Per-harness rendering by `ai-specs sync`**:
  - Claude → merge into `.claude/settings.json` `hooks` (command → the script).
  - Cursor → write `.cursor/hooks.json` (command → the script).
  - OpenCode → generate `.opencode/plugin/<recipe>-<hook>.ts` shim that spawns
    the script, feeds it the normalized event, maps exit/stdout to block/allow.
  - Pi → generate `.pi/extensions/<recipe>-<hook>.ts` shim (`pi.on(...)`) that
    does the same.
- **Idempotent, managed materialization**: generated wiring lives in a managed
  block / generated file so re-sync never clobbers user-authored hooks.
- **Unsupported (event, harness) pairs**: `sync` warns and skips — never emits a
  broken hook. No silent drops.
- **First consumer**: `worktree-flow` declares its existing `worktree-gate.sh`
  via `[provides.hooks]` (replacing the prototype template + manual wiring).
- **Tests + docs**: schema parse, per-harness render goldens, unsupported-pair
  warning, idempotency; update `docs/recipe-schema.md`, `docs/runtime-hooks.md`,
  README compatibility table.

### Out of Scope
- **Per-harness hand-authored hook code** in a recipe (writing separate
  Claude/Cursor/OpenCode/Pi implementations). v1 is "write one script, product
  adapts." Native code hooks can be a later escape hatch.
- **Events beyond the v1 set**. More abstract events added later, behind the same
  mapping table.
- **Harnesses ai-specs does not already target.**
- **Rich conflict UX** when a user has hand-edited a harness's hook config beyond
  the managed block (we define the managed-block boundary; full merge/3-way is
  out).
- **Removing the gitignore on `.claude/settings.json`** — hooks are rendered into
  it via the managed block; the file stays local, the *source* (recipe) travels.

## Capabilities

### New Capabilities
- `runtime-hook-distribution`: how `ai-specs sync` renders recipe-declared
  `[provides.hooks]` into each enabled harness's native runtime-hook format,
  including the abstract→native event map, the normalized script stdin/exit
  contract, the generated shims for code-based harnesses (OpenCode, Pi), the
  managed-block idempotency rule, and the warn-and-skip rule for unsupported
  pairs.

### Modified Capabilities
- `recipe-schema`: extend the `[provides]` primitive list to include `hooks`
  (array of tables with `id`, `event`, `script`, optional `matcher`, `blocking`,
  `description`) and its validation rules (known event, script path resolves
  inside the recipe dir, etc.).

## Approach

A recipe author writes **one** language-agnostic script honoring the normalized
stdin-JSON + exit-code contract. `ai-specs sync` owns the four adapters:

- **Claude + Cursor** share a command-runs-a-script-with-stdin-JSON-and-exit-2
  model, so sync wires the materialized script directly into each one's JSON
  hook config under a managed block.
- **OpenCode + Pi** require JS/TS, so sync **generates** a thin shim that spawns
  the script, translates the harness's native event into the normalized JSON,
  and translates the script's exit code back into the harness's block/allow API.

Renderers receive resolved data via a `--resolved-*` JSON arg (mirroring the
`runtime-brief-rendering` / `--resolved-config` pattern) so catalog resolution
stays out of the renderers. Materialization is idempotent via a managed block
keyed by recipe+hook id.

### Alternatives considered & rejected
- **Per-harness implementations authored in the recipe** — defeats "write once",
  high authoring burden, drifts across four files. Rejected for v1.
- **Claude-only wired script** (the current prototype) — not the product goal;
  single-harness; non-reproducible. Rejected.
- **Overloading `[[hooks]]` (sync-hooks)** — those are sync-time actions on a
  different lifecycle; conflating them would muddy both contracts. Rejected.
- **A new top-level `[hooks.runtime]` manifest section instead of a recipe
  primitive** — contradicts the directive that hooks belong to recipes, not the
  bare manifest. Rejected.
