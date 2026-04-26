## 1. Test Infrastructure

- [x] 1.1 Add doctor test fixtures/helpers that create isolated temporary ai-specs project trees without touching the real workspace.
- [x] 1.2 Add RED tests for `ai-specs help` listing `doctor` and `ai-specs doctor <path>` selecting the provided target path.
- [x] 1.3 Add RED tests proving doctor is read-only by snapshotting fixture file metadata/content before and after execution.
- [x] 1.4 Add RED tests for manifest and `AGENTS.md` `OK`/`ERROR` diagnostics and exit-code behavior.
- [x] 1.5 Add RED tests for enabled-agent diagnostics, including no enabled agents, unsupported agents, present outputs, and missing outputs.
- [x] 1.6 Add RED tests for bundled skill and bundled command diagnostics.
- [x] 1.7 Add RED tests for generated instruction symlink, skill symlink, and copied OpenCode skill directory diagnostics.
- [x] 1.8 Add RED tests for MCP diagnostics when no servers are declared, config files are present, and config files are missing.
- [x] 1.9 Add RED tests for report severity labels, actionable non-OK guidance, summary counts, and zero/non-zero exit semantics.

## 2. CLI Wiring

- [x] 2.1 Update `bin/ai-specs` subcommand comments, dispatch case, and help text to include `doctor`.
- [x] 2.2 Create `lib/doctor.sh` with existing command style, `--help` handling, target path resolution, and delegation to the Python helper.
- [x] 2.3 Ensure `tests/validate.sh` includes the new shell file through existing `bash -n lib/*.sh` validation.

## 3. Doctor Check Engine

- [x] 3.1 Create `lib/_internal/doctor.py` with a check result model that records severity, check name, message, and optional guidance.
- [x] 3.2 Implement target/project structure checks for `ai-specs/ai-specs.toml`, manifest parseability, `AGENTS.md`, `ai-specs/skills`, and `ai-specs/commands`.
- [x] 3.3 Implement manifest-driven agent checks for supported agent names and expected generated paths.
- [x] 3.4 Implement symlink resolution checks for instruction and skill symlinks that must point at `AGENTS.md` or `ai-specs/skills`.
- [x] 3.5 Implement copied skill directory checks for OpenCode-style project-local skill copies.
- [x] 3.6 Implement bundled asset checks for `skill-creator`, `skill-sync`, and command Markdown assets.
- [x] 3.7 Implement MCP declaration counting and generated MCP config presence checks for enabled agents with MCP support.
- [x] 3.8 Implement line-oriented output with `OK`, `WARN`, and `ERROR` labels plus a summary count.
- [x] 3.9 Implement exit-code aggregation so any `ERROR` exits non-zero and warnings alone exit zero.

## 4. Verification

- [x] 4.1 Run focused doctor tests and record RED/GREEN evidence in `apply-progress.md` during apply.
- [x] 4.2 Run `./tests/run.sh` and confirm the full unittest suite passes.
- [x] 4.3 Run `./tests/validate.sh` and confirm py_compile, shell syntax, and unit tests pass.
- [x] 4.4 Manually inspect a representative healthy local project output to ensure the report is readable and actionable.
- [x] 4.5 Update user-facing docs/README command list if implementation introduces a documented CLI surface beyond help output.
