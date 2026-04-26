## Why

Teams already standardize on **ai-specs** (`ai-specs.toml`, sync, bundled skills) but adopting **spec-driven development (SDD)** with **OpenSpec** today means manual steps: install the OpenSpec CLI, scaffold `openspec/`, copy or vendor OpenSpec-oriented skills/commands, and align `openspec/config.yaml` with project testing conventions. We want a **single, declarative onboarding path** so `ai-specs` becomes the front door for SDD, starting with OpenSpec as the first provider while leaving room for other SDD backends.

## What Changes

- **New subcommand (shape TBD)**: e.g. `ai-specs sdd --enable true --provider openspec` that:
  - Ensures the selected SDD provider is available (for OpenSpec: install or verify `openspec` — e.g. `npm install -g @fission-ai/openspec` or documented package name, with clear errors if Node/npm missing).
  - **Materializes** OpenSpec layout under the project (`openspec/config.yaml`, `openspec/changes/`, etc.) when missing, using templates aligned with existing `openspec-sdd-conventions` / `openspec/config.yaml` patterns in this repo.
  - **Extends `ai-specs.toml`**: new `[sdd]` (or `[sdd.openspec]`) section declaring provider, options, and artifact-store mode (see below) so `sync` / `doctor` can reason about SDD state.
  - **Refreshes bundled OpenSpec skills and slash-commands** from the CLI catalog into the target project (same pipeline as `refresh-bundled` / init: copy + lock), optionally adding catalog `[[deps]]` entries for policy skills (`openspec-sdd-conventions`, `testing-foundation`, etc.) instead of duplicating logic ad hoc.
- **Provider abstraction**: configuration names a `provider` (`openspec` first); implementation uses a small internal interface (shell + Python helper) so future providers (e.g. other spec toolchains) can plug in without rewriting the CLI surface.
- **`artifact_store` setting** (declarative, per project):
  - `memory` — prefer **operative session memory** only when the runtime exposes a supported API (e.g. agent memory / MCP); artifacts are prompts and checklists, not persisted under `openspec/` (or optional ephemeral cache).
  - `hybrid` — session memory when available **plus** canonical files under `openspec/` (default for teams that want git-auditable history).
  - `filesystem` — **only** `openspec/` on disk; no dependence on memory APIs.
  - CLI and docs MUST state which modes are implemented in v1 vs. planned (memory modes may be no-ops until a concrete integration exists).
- **Doctor / status**: extend `ai-specs doctor` (or `sdd status`) to report provider binary presence, `openspec/` health, and manifest alignment.
- **Rollback plan**: `ai-specs sdd --enable false` (or remove `[sdd]` block) stops advertising SDD hooks; does not delete user-authored `openspec/` content without explicit `--purge` (if we add it).

## Capabilities

### New Capabilities

- `sdd-cli-integration`: User-facing CLI and manifest fields to enable SDD by provider, install/verify toolchain, and sync bundled skills/commands for that provider.
- `sdd-artifact-store`: Requirements for where change artifacts live (memory / hybrid / filesystem) and how they interact with `openspec` when provider is OpenSpec.

### Modified Capabilities

- `manifest-contract`: New optional `[sdd]` (or nested) tables and validation rules in `ai-specs.toml` (documented in contract spec when we add delta).

## Impact

- **Affected**: `bin/ai-specs`, new `lib/sdd*.sh` (or similar), Python helpers under `lib/_internal/` for TOML merge / validation, `ai-specs/ai-specs.toml` template and contract docs, `tests/` for new flows, `README.md` / user-facing help strings.
- **Dependencies**: Node/npm for global OpenSpec install path; optional future MCP or agent-specific APIs for `memory` mode.
- **Non-goals in this proposal**: reimplementing OpenSpec inside ai-specs; changing OpenSpec upstream behavior.
