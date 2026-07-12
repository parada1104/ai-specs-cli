# Apply Progress: CLI version pinning

**Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cli-version-pinning/`  
**Branch**: `feat/cli-version-pinning`  
**Base**: `development`

## Phase 1 — cli_version module (RED → GREEN)

- RED: `tests/test_cli_version.py` — semver, policy parse, check_policy, lock meta read.
- GREEN: `lib/_internal/cli_version.py`.

## Phase 2 — Lock meta (RED → GREEN)

- RED: `tests/test_lock.py::test_meta_section_written_and_ignored_on_skill_load`
- GREEN: `lib/_internal/lock.py` — `[meta]` read/write; `stamp_lock_meta()` with idempotent `synced_at`.

## Phase 3 — Sync gate (RED → GREEN)

- RED: `tests/test_sync_pipeline.py::CliVersionSyncGateTests`
- GREEN: `lib/sync.sh` — pre-flight `check-sync`, `--ignore-cli-version`, post-success `stamp-meta`.
- GREEN: `lib/refresh-bundled.sh` — stamp-meta on standalone refresh.

## Phase 4 — Doctor (RED → GREEN)

- RED: `tests/test_doctor.py::CliVersionDoctorTests`
- GREEN: `lib/_internal/doctor.py::_check_cli_version`

## Phase 5 — Docs

- `CHANGELOG.md` (new)
- `docs/ai-specs-toml.md` — `[tool]` section
- `docs/ai/troubleshooting.md` — CLI version mismatch
- `templates/ai-specs.toml.tmpl` — commented example
- `README.md` — pin + CHANGELOG link
- `tests/test_manifest_contract_docs.py` — `[tool]` contract rows

## Validation

```text
./tests/validate.sh → exit 0 (733 tests)
```

## Not done (optional follow-up)

- Dogfood `[tool].version` pin in `ai-specs/ai-specs.toml` (task 7.1) — deferred until VERSION bump to 0.12.3 at release.
