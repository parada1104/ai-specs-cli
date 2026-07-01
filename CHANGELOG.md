# Changelog

All notable changes to the ai-specs CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Optional `[tool]` section in `ai-specs/ai-specs.toml` to pin the CLI version
  per project (`version` + `policy = "exact"`, or `min_version` + `policy = "min"`).
- Lock file metadata (`[meta].cli_version`, `[meta].synced_at`) written on sync
  and `refresh-bundled` to record which CLI version last touched the project.
- `ai-specs doctor` reports installed, pinned, and last-synced CLI version.
- `ai-specs sync --ignore-cli-version` escape hatch when a pin must be bypassed.

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
