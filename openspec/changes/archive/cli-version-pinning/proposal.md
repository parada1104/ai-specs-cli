# Proposal: Per-project CLI version pinning

## Why (motivation)

`ai-specs` installs globally (`~/.ai-specs`) while each project owns a versioned
manifest for recipes, deps, and bundled assets. There is no durable record of
**which CLI version** last synced a project, and no way to **pin** the CLI version
a project expects.

This creates operational blind spots:

- Projects like **venturi_coffee** can run on an unknown old CLI while production
  tooling is already at **0.12.2**.
- Upgrading the global CLI can silently change sync behavior across all projects
  without any per-repo signal.
- Migration planning lacks a baseline: recipe pins say *what catalog version* a
  project uses, not *what CLI semantics* produced its derived artifacts.

Recipe version pins already solve the catalog half of this problem. This change
adds the missing **tool version** half: visibility, optional enforcement, and a
foundation for migration guides.

## What changes (scope)

### In scope

1. **`[tool]` manifest section** — optional per-project CLI version policy:
   - `version` — exact pin (semver)
   - `min_version` — minimum acceptable CLI (semver)
   - `policy` — `exact` | `min` (default inferred from which field is set)

2. **Lock file metadata** — on every successful sync/refresh that writes the lock,
   record:
   - `meta.cli_version` — CLI version that performed the write
   - `meta.synced_at` — ISO-8601 UTC timestamp

3. **`ai-specs doctor` diagnostics** — report installed vs pinned vs last-synced
   CLI version with OK/WARN/ERROR severity.

4. **`ai-specs sync` enforcement** — when `[tool]` policy is set, fail before writes
   if the running CLI does not satisfy the policy (with actionable guidance).

5. **`CHANGELOG.md`** — start a user-facing release/migration log at repo root.

6. **Documentation** — update `docs/ai-specs-toml.md`, README CLI table, and
   troubleshooting for version mismatch.

7. **Tests** — unit tests for semver comparison, lock meta, doctor, and sync
   gate (TDD per `openspec/config.yaml`).

### Out of scope

- Per-project local CLI installs (`AI_SPECS_HOME` checkout management).
- `ai-specs migrate` command (future; depends on CHANGELOG maturity).
- Fleet-wide audit across multiple repos.
- Pinning to git SHAs or pre-release tags beyond existing semver tolerance.
- npm/npx distribution channel.

## Capabilities (new / modified)

| Capability | Type | Description |
|------------|------|-------------|
| `cli-version-contract` | **New** | Manifest `[tool]` schema, lock `[meta]`, semver policy, sync gate |
| `project-doctor` | **Modified** | CLI version visibility and mismatch diagnostics |

## Impact (affected modules)

| File / area | Change |
|-------------|--------|
| `lib/_internal/cli_version.py` | **New** — read installed version, parse policy, compare semver |
| `lib/_internal/lock.py` | Write/read `[meta]` table |
| `lib/_internal/refresh-bundled.py` | Stamp lock meta after write |
| `lib/sync.sh` | Pre-flight CLI policy check |
| `lib/_internal/doctor.py` | CLI version checks |
| `lib/init.sh` | Optionally seed `[tool].version` on first init |
| `templates/ai-specs.toml.tmpl` | Document `[tool]` section |
| `docs/ai-specs-toml.md` | Canonical reference for `[tool]` |
| `docs/ai/troubleshooting.md` | Version mismatch fixes |
| `CHANGELOG.md` | **New** — release notes starting 0.12.x |
| `tests/test_cli_version.py` | **New** |
| `tests/test_doctor.py` | Extend for CLI version scenarios |
| `tests/test_sync_pipeline.py` | Extend for sync gate |

## Rollback plan

1. Remove `[tool]` parsing and sync gate — manifests without `[tool]` behave as today.
2. Lock `[meta]` is additive; older CLIs ignore unknown tables; new CLI stops writing
   meta if rolled back.
3. Revert doctor checks — no functional impact on sync when gate is removed.
4. Delete `CHANGELOG.md` if undesired (docs-only).

## Success criteria

- [ ] After sync, `ai-specs/.ai-specs.lock` contains `meta.cli_version` matching
  the running CLI.
- [ ] `ai-specs doctor` reports installed, pinned (if any), and last-synced CLI
  version.
- [ ] Sync fails with clear guidance when `[tool].policy = "exact"` and installed
  CLI ≠ pin.
- [ ] Projects without `[tool]` remain backward compatible (WARN only when
  installed ≠ last-synced).
- [ ] `./tests/validate.sh` passes.
