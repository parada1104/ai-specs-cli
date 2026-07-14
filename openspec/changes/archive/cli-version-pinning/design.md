## Context

Projects pin recipe catalog versions in `ai-specs.toml` but the ai-specs CLI itself
installs globally. Operators cannot tell which CLI version last touched a project
(e.g. venturi_coffee on an unknown old CLI while production uses 0.12.2). This
change adds optional per-project CLI version policy, lock-file provenance, and
doctor/sync integration.

## Goals / Non-Goals

**Goals:**

- Optional `[tool]` manifest section with `exact` and `min` policies.
- Lock `[meta]` records `cli_version` + `synced_at` on every lock write.
- `ai-specs doctor` reports installed / pinned / last-synced CLI version.
- `ai-specs sync` fails fast when policy is violated (before file writes).
- `--ignore-cli-version` escape hatch for emergency sync.
- Root `CHANGELOG.md` as migration foundation.

**Non-Goals:**

- Local per-project CLI checkout management.
- `ai-specs migrate` command.
- Auto-pin on `init` (document manual pin instead).
- Fleet audit across repos.

## Decisions

### Decision 1: `[tool]` table name

Use `[tool]` rather than `[cli]` to leave room for future harness metadata without
overloading `[project]`.

**Alternatives:** `[cli]` (too narrow), `[project].cli_version` (mixes concerns).

### Decision 2: Centralize logic in `cli_version.py`

New module `lib/_internal/cli_version.py`:

- `read_installed_version(home: Path) -> str`
- `parse_tool_policy(manifest: dict) -> ToolPolicy | None`
- `compare_versions(a: str, b: str) -> int` (-1/0/1)
- `check_policy(installed: str, policy: ToolPolicy) -> tuple[bool, str]`
- `read_lock_meta(lock_path: Path) -> dict`

Used by `sync.sh` (via small Python entrypoint), `doctor.py`, and lock writer.

**Rationale:** Semver + manifest parsing in one tested module; bash stays thin.

### Decision 3: Stamp meta in `write_lock`

Extend `lock.py` `write_lock()` to accept optional `meta: dict` and emit `[meta]`
before other sections. Callers pass meta after successful refresh/materialize steps.

Also add `stamp_lock_meta(project_root, cli_home)` helper called from sync pipeline
end (and refresh-bundled when it writes lock) so meta updates even when hashes are
unchanged.

**Rationale:** Single write path; avoids duplicating TOML emission.

### Decision 4: Sync gate before target-resolve writes

Insert gate at start of `sync.sh` after path resolution:

```bash
python3 "$CLI_VERSION_PY" check-sync "$TARGET_PATH" "$AI_SPECS_HOME" ${IGNORE_FLAG}
```

Exit non-zero blocks entire pipeline. Matches recipe version fail-fast pattern.

### Decision 5: Doctor severity matrix

| Condition | Severity |
|-----------|----------|
| Pin satisfied, last-sync matches installed | OK |
| No pin, last-sync matches installed | OK |
| No pin, last-sync differs from installed | WARN |
| No pin, no lock meta | INFO |
| Pin violated | ERROR |
| Conflicting `[tool]` fields | ERROR |

Doctor never writes files.

### Decision 6: Init does not auto-pin

`ai-specs init` leaves `[tool]` absent. Template comments show optional pin example.
Rationale: existing projects and cautious adopters should opt in; dogfood can pin
manually in a follow-up commit.

### Decision 7: CHANGELOG Keep a Changelog format

Start with `[Unreleased]` documenting this feature and `[0.12.2]` as baseline
reference for current production.

## Flow

```text
ai-specs sync [path]
  |
  v
cli_version.py check-sync
  |-- read manifest [tool]
  |-- read AI_SPECS_HOME/VERSION
  |-- evaluate policy → abort if fail (unless --ignore-cli-version)
  v
existing sync pipeline ...
  |
  v
stamp_lock_meta → write_lock [meta]
```

```text
ai-specs doctor [path]
  |
  v
doctor.py _check_cli_version()
  |-- installed from VERSION
  |-- pinned from manifest
  |-- last_synced from lock [meta]
  |-- emit OK/WARN/ERROR per matrix
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Semver edge cases | Reuse tested tuple comparison; align with recipe version tests |
| Lock meta stale if sync fails mid-pipeline | Stamp only after successful sync exit path |
| Users surprised by sync failure after global upgrade | Doctor WARN before pin; CHANGELOG migration notes |
| `--ignore-cli-version` abused | Warning on stderr; document as break-glass only |

## Migration plan

1. Ship feature in 0.12.3 (or next patch).
2. Existing projects: no `[tool]` → behavior unchanged except lock meta after next sync.
3. Production projects: add `[tool].version = "0.12.x"` when ready.
4. Future: `ai-specs migrate` reads CHANGELOG between pinned and installed versions.

## Open questions (deferred)

- Should `ai-specs upgrade` warn when global upgrade would break pinned projects in cwd?
- Should AGENTS.md brief include pinned CLI version for agent visibility?
