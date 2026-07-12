# Changelog

All notable changes to the ai-specs CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.12.3] — 2026-07-12

### Added

- **Interactive onboarding TUI** for `ai-specs init` (`--tui` / auto on TTY;
  `--no-tui` keeps classic path). Rich-based wizard selects project name,
  agents, and catalog recipes. Soft-fails to classic init when Rich unavailable.
  Cancel (Confirm 'n', Ctrl-C, Ctrl-D/EOF) exits cleanly without writing a manifest.
- **Recipe tags and conflict detection** (`tags` + `conflicts_with` fields in
  `recipe.toml`). `ai-specs sync` surfaces tag conflicts as advisory warnings.
  Catalog recipes tagged by domain.
- **CLI version pinning** via optional `[tool]` section in `ai-specs.toml`
  (`version` + `policy = "exact"`, or `min_version` + `policy = "min"`).
  Lock `[meta]` records `cli_version` and `synced_at` on sync.
  `ai-specs doctor` reports installed, pinned, and last-synced version.
  `--ignore-cli-version` escape hatch for bypassing pins.
- **Worktree-flow gate modes** (`always` / `ask` / `off`) configurable per project.
- **Plan-build-flow recipe** — two-verb (`/plan`, `/build`) catalog recipe over
  the existing multi-phase change ceremony. Ambient skill-only v2 workflow.
- **VCS pre-merge archive rule** — SDD/OpenSpec artifacts MUST be archived
  before merge; mirrored into GitHub, GitLab, and Bitbucket merge-workflow skills.
- **Post-merge branch cleanup** codified in the GitHub merge-workflow skill
  (worktree removal + branch deletion after squash merge).
- **Recipe eval harness** — opt-in behavior eval for recipes (runtime-level
  verification beyond materialization tests).

### Changed

- `plan-build-flow` v2 is **breaking** for existing `/plan-build-flow` users:
  the skill-only ambient workflow replaces the earlier multi-command ceremony.

### Fixed

- Tag dedup hardening: blank tag values rejected; dedup is order-preserving.

### Migration notes

- Existing projects without `[tool]` behave as before; run `ai-specs sync` once
  to populate lock `[meta]`.
- To pin production projects, add:
  ```toml
  [tool]
  version = "0.12.3"
  policy = "exact"
  ```
  after upgrading the global CLI to that version.

## [0.12.2] — 2026-06-23

Baseline reference for projects already on production tooling. Includes recipe
version pinning, upgrade command, doctor diagnostics, and bundled lock tracking.
