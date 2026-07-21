## ADDED Requirements

### Requirement: Pre-tool-use artifact gate hook

The `plan-build-flow` recipe SHALL distribute a `pre-tool-use` runtime hook
(`hooks/plan-build-gate.sh`, `matcher = Edit|Write|MultiEdit|NotebookEdit`,
`blocking = true`) that machine-enforces the plan-before-build artifact
precondition. The hook SHALL follow the normalized hook contract: read stdin
JSON `{event, tool_name, tool_input, cwd}`, exit `0` to allow, exit `2` to
block, and fail open (exit `0`) on any parse or lookup error.

The hook SHALL block a matched edit only when BOTH hold: (a) the target path is
under a production directory (default top-level `src`, `lib`, `catalog`,
overridable via `PLAN_BUILD_GATE_PATHS` — scope configuration only), AND (b) no
active change folder exists (no `openspec/changes/*/tasks.md` outside
`archive/`). It SHALL allow edits under `openspec/changes/**`, non-production
paths, and gitignored agent config (`.claude/settings*.json`, `.claude/hooks/*`)
unconditionally. The gate SHALL be non-bypassable: it exposes no on/off/ask
mode, so the only way past it is to write the plan the gate requires.

Because the hook pipeline exposes no pre-file-write event for `cursor`, this
hook enforces on `claude`, `opencode`, `pi`, and `omp` only; `cursor` retains
the advisory skill + workflow-rules layer.

#### Scenario: Production edit blocked without a change folder

- GIVEN no `openspec/changes/*/tasks.md` exists outside `archive/`
- AND a `Write` targets a file under a production directory (e.g. `src/app.py`)
- WHEN the hook receives the normalized event
- THEN it MUST exit 2 and surface a plain-language reason to the agent

#### Scenario: Production edit allowed once a plan exists

- GIVEN `openspec/changes/<slug>/tasks.md` exists outside `archive/`
- AND a `Write` targets a file under a production directory
- WHEN the hook receives the event
- THEN it MUST exit 0

#### Scenario: Writing the plan is never blocked

- GIVEN no change folder exists yet
- AND a `Write` targets `openspec/changes/<slug>/tasks.md`
- WHEN the hook receives the event
- THEN it MUST exit 0

#### Scenario: Fail-open on malformed input

- GIVEN malformed JSON or a missing `file_path` on stdin
- WHEN the hook runs
- THEN it MUST exit 0 (a buggy guard must never wedge all editing)

#### Scenario: No mode bypass

- GIVEN a production `Write` with no active change folder
- AND any `PLAN_BUILD_GATE_MODE` value is set in the environment
- WHEN the hook runs
- THEN it MUST still exit 2 (the mode env has no effect; the gate has no off switch)
