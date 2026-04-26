## Context

`ai-specs` already has an initialization and synchronization pipeline, but validation is distributed across command failures and generated files. `bin/ai-specs` dispatches Bash subcommands in `lib/*.sh`; structured TOML reads, target resolution, MCP rendering, gitignore rendering, and AGENTS rendering live in `lib/_internal/*.py`. The project now has a documented manifest contract, context precedence policy, and testing foundation, so the next operational gap is a read-only diagnostic command that can tell a human or agent whether the local project is in a trustworthy state.

The Trello card requires checks for the manifest, `AGENTS.md`, enabled agents, bundled skills, bundled commands, generated symlinks, MCP configs, and clear `OK`/`WARN`/`ERROR` output. Because `doctor` is a diagnostic command, it must avoid the mutation semantics of `init`, `sync`, and `sync-agent`.

## Goals / Non-Goals

**Goals:**
- Add `ai-specs doctor [path]` as a read-only CLI command.
- Validate the minimum project structure needed by generated agent tooling.
- Report actionable checks with stable severities: `OK`, `WARN`, and `ERROR`.
- Exit `0` when no blocking errors are found and non-zero when one or more `ERROR` checks are found.
- Reuse the existing manifest readers and platform path definitions where practical.
- Cover behavior with strict TDD tests before implementation.

**Non-Goals:**
- Do not repair state automatically.
- Do not run `init`, `sync`, `sync-agent`, `refresh-bundled`, or vendoring as part of doctor.
- Do not introduce a new manifest schema engine.
- Do not validate semantic freshness by diffing every generated file byte-for-byte unless a focused check is necessary for a listed requirement.
- Do not require network access or inspect remote Git state.

## Decisions

### Decision 1: Implement doctor as a Bash wrapper plus Python helper

`lib/doctor.sh` should follow existing CLI command patterns: resolve the target path, expose `--help`, and delegate structured checks to `lib/_internal/doctor.py`. The Python helper should handle TOML parsing, JSON/TOML config inspection, path checks, symlink checks, and exit-code aggregation.

Rationale: Bash is consistent for command wiring, but the validation matrix is easier to test and maintain in Python using existing `tomllib`, `json`, and `pathlib` support. A pure Bash doctor would duplicate parsing logic and make unit tests brittle. A Python-only entrypoint from `bin/ai-specs` would diverge from existing command layout.

Alternatives considered:
- Pure Bash: simpler file count, but poor for structured MCP and manifest checks.
- Add checks to `sync`: rejected because doctor must be read-only and callable without mutating generated artifacts.

### Decision 2: Severity model is deterministic and minimal

Checks should return one of three severities:
- `OK`: expected state is present.
- `WARN`: state is valid but potentially incomplete or advisory, such as no enabled agents or no MCP servers declared.
- `ERROR`: required state is missing, invalid, or inconsistent enough that generated tooling cannot be trusted.

Rationale: This maps directly to the card and keeps the command useful for both humans and automation. The non-zero exit condition should depend only on `ERROR`, not `WARN`.

Alternatives considered:
- More severities such as `INFO` or `SKIP`: rejected for MVP because they complicate both parsing and acceptance criteria without adding necessary behavior.

### Decision 3: Check generated paths from the manifest and platform table

For each enabled agent, doctor should validate the generated outputs that `sync-agent` is expected to create from `platform.sh`:
- instructions symlink when `instructions_path` is defined (`CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`).
- skills symlink or copied skill directory when `skills_dir` is defined.
- commands directory when `commands_dir` is defined and bundled/local command sources exist.
- MCP config file when both an agent MCP path and one or more `[mcp.*]` entries exist.

Rationale: `platform.sh` is the existing source of path truth. Doctor should not invent a second matrix. If direct Python reuse of `platform.sh` is awkward, implementation may encode the same small static table in `doctor.py`, but tests must lock it to expected generated paths.

Alternatives considered:
- Validate every possible agent output regardless of `[agents].enabled`: rejected because disabled agents should not be treated as missing generated state.
- Only validate root-level files: rejected because it misses the card's symlink and MCP config requirements.

### Decision 4: Doctor is structural, not a hidden sync diff engine

Doctor should validate that critical files and directories exist, symlinks point to expected generated targets, and configs are parseable where practical. It should not perform full render-and-diff freshness checks in MVP.

Rationale: Full render diffing risks duplicating `sync-agent`, creating false positives from merge-safe MCP preservation, and making doctor slower and more invasive. Structural checks are enough to catch missing initialization, missing sync outputs, disabled agents, and absent MCP configs.

Alternatives considered:
- Render expected outputs to temporary files and diff: deferred because it increases implementation complexity and could be introduced later as `--strict`.

### Decision 5: Output stays human-readable, stable enough for tests

The MVP output should be line-oriented and readable, for example:

```text
ai-specs doctor
  target: /path/to/project

  OK    manifest       ai-specs/ai-specs.toml found
  OK    agents         enabled: claude, opencode
  WARN  mcp            no [mcp.*] entries declared
  ERROR agents-md      AGENTS.md missing; run ai-specs sync

Summary: 2 OK, 1 WARN, 1 ERROR
```

Rationale: The card asks for clear checks, not machine-readable output. A stable line format makes shell and unittest assertions straightforward without adding a JSON mode to the MVP.

Alternatives considered:
- JSON output first: useful later, but not required by current card and increases CLI surface.

## Flow

```text
User/agent
  |
  | ai-specs doctor [path]
  v
bin/ai-specs
  |
  | dispatch doctor
  v
lib/doctor.sh
  |
  | resolve target, handle --help
  v
lib/_internal/doctor.py
  |
  | parse ai-specs/ai-specs.toml when present
  | inspect root files and ai-specs assets
  | inspect enabled-agent generated paths
  | inspect MCP config presence when declared
  v
line-oriented OK/WARN/ERROR report + summary + exit code
```

## Risks / Trade-offs

- [Risk] Duplicating the platform path table in Python could drift from `platform.sh` → Mitigation: keep the table minimal, test all supported agents, and prefer invoking or sharing platform metadata if implementation can do so cleanly.
- [Risk] Users may expect doctor to fix issues → Mitigation: output must include actionable guidance such as `run ai-specs init` or `run ai-specs sync`, while staying read-only.
- [Risk] Structural checks may miss stale generated content → Mitigation: explicitly document MVP scope and reserve strict render-diff validation for a later change if needed.
- [Risk] Missing manifest makes downstream checks noisy → Mitigation: emit a manifest `ERROR`, skip manifest-dependent checks, and still check root artifacts that can be evaluated independently.
- [Risk] OpenCode copies skills while Claude/Gemini symlink them → Mitigation: model checks per agent path semantics instead of assuming all skills paths are symlinks.

## Migration Plan

1. Add tests that define doctor behavior for healthy, warning, and error fixtures.
2. Wire `doctor` into `bin/ai-specs` and help output.
3. Add `lib/doctor.sh` as the CLI wrapper.
4. Add `lib/_internal/doctor.py` with read-only checks.
5. Run focused tests with `./tests/run.sh`, then full validation with `./tests/validate.sh`.

Rollback is file-level and safe because doctor is additive: remove the CLI dispatch/help entry, delete the new doctor files, and delete the tests.

## Open Questions

- Should a future `ai-specs doctor --strict` render expected outputs into a temp directory and compare generated artifacts byte-for-byte?
- Should a future `--json` mode be added for CI integrations after the human-readable MVP is stable?
