# Route omp runtime brief through its native `.omp/AGENTS.md` slot

## Problem

`omp` (Oh My Pi, `can1357/oh-my-pi`) is registered in `lib/_internal/platform.sh`
with an empty `instructions_path` and `native=true`, so sync renders only the
root `AGENTS.md` and creates no instruction symlink for omp. The design assumed
omp reads the root `AGENTS.md` as its native project context.

Per oh-my-pi's own `docs/context-files.md`, that assumption is fragile. omp
resolves project instructions through prioritized providers:

| Provider   | Path                              | Priority |
|------------|-----------------------------------|----------|
| native     | `<ancestor>/.omp/AGENTS.md`       | 100      |
| claude     | `.claude/CLAUDE.md`               | 80       |
| gemini     | `.gemini/GEMINI.md`               | 60       |
| github     | `.github/copilot-instructions.md` | 30       |
| agents-md  | standalone root `AGENTS.md`       | 10       |

Rules that matter here:
- At the same ancestor depth, the highest-priority provider **shadows** the rest.
- The `agents-md` provider **ignores** any `AGENTS.md` whose parent directory
  name starts with a dot (so a file inside `.omp/` is only picked up by the
  native provider, never by `agents-md`).

Consequence: today omp loads the root `AGENTS.md` only through the lowest-priority
`agents-md` provider (priority 10). It works only because no `.claude/CLAUDE.md`,
`.gemini/GEMINI.md`, or `.github/copilot-instructions.md` exists at the root. The
moment any of those appears, it shadows the runtime brief and omp silently stops
reading it. The native, highest-priority slot `.omp/AGENTS.md` is left empty.

## Solution

Populate omp's native slot instead of relying on the fallback provider. Set
omp's `instructions_path` to `.omp/AGENTS.md`. The existing sync symlink pass
(`lib/sync-agent.sh`, driven purely by a non-empty `instructions_path`) then
creates `.omp/AGENTS.md -> ../AGENTS.md` on every sync — the same declarative
mechanism that already gives `claude` its `CLAUDE.md` and `gemini` its
`GEMINI.md`. `copilot` already combines `native=true` with a non-empty
`instructions_path`, so the model is unchanged.

With the symlink in place, the native provider (priority 100) loads the brief and
shadows the standalone root `AGENTS.md` (priority 10) at the same depth, so the
content loads exactly once — no duplication — and survives the introduction of any
other provider directory.

## Affected modules

- `lib/_internal/platform.sh` — omp `instructions_path` and its comment.
- `lib/_internal/doctor.py` — omp `PLATFORM` entry `instructions_path`.
- `tests/test_sync_pipeline.py` — sync coverage for the omp native symlink.
- `tests/test_doctor.py` — assert omp `instructions_path`.
- `openspec/specs/omp-agent-target/spec.md` — contract update (delta).

## Out of scope

- Global `~/.omp/config.yml` provider toggles (outside this repo).
- Any change to `pi`, which keeps reading the root `AGENTS.md` natively.
