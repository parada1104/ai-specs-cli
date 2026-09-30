# [Go 08] Port doctor + rules-audit

Card: Trello `6a84e7b6a54e995f1c5d353f` — branch `change/go-08-doctor`
(cut from epic tip `bdf0fe87cff810faac068d4e02aad0080e86e68f`). Trello MCP is
off-limits for this session; the card text was supplied inline and is binding.

Status: **implemented, routed and verified** — both commands are native Go
commands, the differential parity harness reports zero deltas on every fixture
(legacy-vs-Go and explicit legacy-vs-legacy), and one filesystem-mutation test
per command proves the port writes nothing.

## Goal

`doctor` and `rules-audit` become native Go commands: identical checks,
severities, exit codes, report formatting and check ordering; strictly
read-only; differential parity against the legacy Python/Bash oracle on
healthy, degraded and broken fixtures.

## Design

Strangler seam, same direction-of-travel as the Python→Go bridges already in
`lib/_internal/` (`GO_LOCK_WRITE_BRIDGE_FALLBACK` etc.): **Go owns the command**
(argument handling, check roster and ordering, every severity/message string,
report formatting, exit codes) and every sub-analysis that can be built on
already-ported Go layers. Exactly **one** dependency stack is still bridged:
the brief-provenance chain, through `internal/doctor/bridge.py` (embedded in the
binary, one `python3` subprocess per run, JSON in/out, per-op try/except so the
Go checks reproduce the Python early-return semantics, `PYTHONDONTWRITEBYTECODE=1`
so the port stays read-only — legacy writes `__pycache__`, defect D26).

| Bridged op | Why not native yet |
|---|---|
| `brief_state` — brief effective ownership state | needs `recipe-materialize.py` (4 265 LOC) + `agents-render.py` (881 LOC), cards 07/09/10 territory |
| `brief_dead_fragments` — `has_dead_recipe_fragments(resolved)` | same chain |
| `toml_error` — the `tomllib` diagnostic rendered as the `manifest` guidance | the text is the interpreter's own; Python versions differ in it (measured), so the port asks the same authority instead of inventing a lookalike |

If the bridge cannot run at all, doctor appends one ERROR `doctor-bridge` check
(fail-closed) instead of silently dropping checks.

Everything else is native Go on top of `internal/{config,toml,lock,schema,target,home,skills}`
and the root `assets` package:

- `cli_version` subset (installed version, `[tool]` policy, `evaluate_cli_version`)
- `brief-render-policy` (`brief_render_enabled`, marker, `ValueError` path)
- `project-cache` layout (cache key/root and the command/bundled/resolved-skills
  roots). The key/root derivation lives once, in `internal/skills`; doctor
  delegates to it.
- `dep_check.py` (full port: PATH probe, `version_check` with `shell=True`/5s
  semantics, version compare, plus the **read-only** provider-install
  resolution `_check_one` reaches for `installer = "github-release"`)
- `gate_binary` doctor subset (`detect_platform`, cache paths, trusted digests,
  digest-mismatch record, `resolve_verified_binary`, `binary_version`,
  `_run_selftest`, `cache_size`) — acquisition and every writing path are
  deliberately not ported
- `ledger_bridge.recipe_id` (witness read + literal fallback)
- `env_scaffold` doctor subset (`collect_env_vars`, `collect_env_allowed`,
  `has_managed_block`, `managed_block_is_current`, `load_harness_env`)
- `util` classify subset (`render_override_bytes`, `classify_managed_override`,
  the resolved projection), mirroring the already-ported gate module
  (`catalog/recipes/worktree-flow/gate/{classify,resolved_config}.go`) that the
  legacy consults when a verified binary exists
- `util.project_repo_topology` → `internal/target` (already ported)
- `rules-inventory.py` → `internal/rulesaudit`, on top of `internal/skills`
  (the native port of `skill_contract` frontmatter parsing +
  `skill-resolution.collect_skills`, which a later card must reuse for `sync`
  rather than re-implement)

## Inventory — `doctor` (lib/_internal/doctor.py)

Report skeleton (FROZEN formatting):

```
(blank)
ai-specs doctor
  target: <resolved root>
(blank)
  <SEVERITY padded to 5><2 spaces><name padded to 15><2 spaces><message>[  (guidance)]
(blank)
Summary: <ok> OK, <info> INFO, <warn> WARN, <err> ERROR
```

Severity enum order: `OK, INFO, WARN, ERROR`. Exit code: `1` iff ≥1 ERROR else
`0`; `2` for unknown flag / extra positional; `1` for a nonexistent path
(`doctor.sh` lacks the `-d` guard, so the Python `not a directory` branch is
dead code — the raw bash `cd:` failure is FROZEN).

Check roster in `Doctor.run()` order (order is FROZEN):

| # | Method | Check name(s) | Severity rule |
|---|---|---|---|
| 1 | `_check_manifest` | `manifest` | OK file found; ERROR unparseable (guidance `ExcType: msg`); ERROR missing (guidance `run ai-specs init`) |
| 2 | `_check_cli_version` | `cli-version` | from `evaluate_cli_version` (OK/WARN/INFO/ERROR); skipped when `lib/_internal/cli_version.py` absent |
| 3 | `_check_legacy_recipe_versions` | `recipe-version` | WARN when any `[recipes.*].version` non-blank; sample 5 + ` (+N more)` |
| 4 | `_check_agents_md` | `agents-md` | OK present; ERROR missing + `brief.render=false`; ERROR missing |
| 5 | `_check_brief_render_policy` | `brief-render`, `brief-render-marker`, `brief-fragments-unused` | ERROR `ValueError` text; INFO disabled; INFO marker present; WARN dead fragments |
| 6 | `_check_brief_provenance` | `brief-provenance` | INFO missing/managed_stale/marker; OK managed_current; WARN user_modified/untracked/undetermined; skipped when manifest absent or render disabled |
| 7 | `_check_bundled_assets` | `bundled-skill` ×N, `bundled-commands` ×N | OK present; ERROR missing (`cache .bundled/...`) |
| 8 | `_check_tracked_bundled_leftovers` | `tracked-bundled-leftover` ×0-2 | WARN per kind when git still tracks removed bundles |
| 9 | `_check_enabled_agents` | `mcp`, `agents`, `agents`+`unsupported agent` | WARN no `[mcp.*]`; WARN none enabled; OK enabled list; ERROR unsupported |
| 10 | `_check_agent_outputs` (per enabled agent) | `CLAUDE.md`/`.omp/AGENTS.md`/`GEMINI.md`/`.github/...`, `.X/skills`, `.X/commands`, `mcp-<agent>` | OK/WARN/ERROR per instruction symlink, skills dir, commands dir, MCP file |
| 11 | `_check_recipe_cli_deps` | `recipe-dep` ×N | OK available; WARN required missing (guidance install_url); INFO optional missing |
| 12 | `_check_tracker_ledger` | `tracker-ledger` ×1-2 | INFO unhosted work-start; ERROR no verified binary; ERROR unreadable verdict; else severity from the Go gate verdict |
| 13 | `_check_harness_env_layout` | `direnv`, `envrc-managed`, `harness-env`, `harness-env-value` | WARN/OK direnv; WARN/OK managed block; WARN/OK missing keys; WARN invalid enum value |
| 14 | `_check_worktree_gate` | `worktree-gate` | ERROR digest mismatch; INFO leftover legacy script; ERROR `gate_impl=bash`; ERROR no usable binary; ERROR selftest failed; WARN version mismatch; OK otherwise |
| 15 | `_check_repo_topology` | `repo-topology`, `repo-topology-deprecated` | INFO resolved; WARN deprecation; silent when source=default and worktree-flow disabled |
| 16 | `_check_stale_template_overrides` | `stale-override` ×N | WARN user_modified / untracked(divergent) / managed_stale(non-auto) |
| 17 | `_check_gate_provenance` | `gate-provenance` ×N | WARN user_modified; WARN untracked |

## Inventory — `rules-audit` (lib/_internal/rules-inventory.py)

`python3 rules-inventory.py <root>` → `json.dumps(payload, indent=2)` + newline,
exit 0; exit 2 for usage/non-directory; exit 1 with `ERROR: <exc>` on stderr for
a scan failure. `rules-audit.sh` adds the `-d` guard (exit 2 before Python).

Payload: `schema_version`, `mode` (A/B), `target`, `classification_is_suggestion`,
`sources{cursor_rules, cursorrules, agents_md_sections, agents_md_present,
manifest, resolved_skills, recipe_catalog, atl_registry}`, `summary`, and for
mode B `stack_hints` + `recommendations`. Key ordering of the JSON document is
part of the surface (`sort_keys` is NOT used; dict insertion order is emitted,
so the Go port uses structs/ordered writers and emits empty lists as `[]`, never
`null`). `collect_skills` + frontmatter parsing are **native**
(`internal/skills`), not bridged.

## Tasks

- [x] Recon: read doctor.py, dep_check.py, rules-inventory.py, doctor.sh,
      rules-audit.sh, the parity harness, the parity contract, and the existing
      Go packages; inventory every check/severity/exit code (above).
- [x] **WU1** — `internal/doctor`: severity/check/report/exit framework, the
      Python bridge, checks 1-8. Unrouted; unit + differential tests.
- [x] **WU2** — checks 9-11 (enabled agents, per-agent outputs, recipe deps,
      full native `dep_check.py` port including the read-only provider path).
- [x] **WU3** — checks 12, 13, 15 (tracker ledger, harness env layout, repo
      topology).
- [x] **WU4** — checks 14, 16, 17 (worktree gate, stale overrides, gate
      provenance) + the gate-binary module, and two de-duplications
      (`checks_tracker.go` → shared `gatebinary.go`;
      `checks_topology.go` → exported `target.ProjectRepoTopology`).
- [x] **WU5** — route `doctor` natively; three doctor parity fixtures
      (broken/degraded/healthy) and the justified N7 normalization;
      `tests/test_doctor_readonly.py`.
- [x] **WU6** — native `rules-inventory` (`internal/rulesaudit`) + the shared
      `internal/skills` port; unrouted.
- [x] **WU7** — route `rules-audit` natively; two rules-audit fixtures;
      `tests/test_rules_audit_readonly.py`; project-cache ownership moved to
      `internal/skills`.
- [x] Full `./tests/validate.sh` green (see Evidence).

## Evidence

| Work unit | Commit | Verification |
|---|---|---|
| WU1 | `eb7ce3e` | `go test ./internal/doctor/` — differential `cli_version.evaluate_cli_version` + `project-cache.cache_key` vs the legacy modules |
| WU2 | `3c50884` | `go test ./internal/doctor/` — 22 new tests; three parity-gap fixes (dangling-symlink `resolvePy`, `mdNames` directories, `stdout+stderr` version probe) |
| WU3 | `4b26abe` | `go test ./internal/doctor/` — 28 new tests (env parsing, managed block, tracker early returns, topology messages) |
| WU4 | `4464b38` | `go test ./internal/doctor/ ./internal/target/` — 27 new tests; existing tracker/topology tests unchanged after de-duplication |
| WU5 | `421dd18` | `python3 tests/parity/run.py` **11/11 fixtures, 0 deltas** (legacy-vs-Go) incl. `doctor-broken`, `doctor-degraded`, `doctor-healthy`; `run.py --self-test` 11/11; `tests/test_doctor_readonly.py` OK |
| WU6 | `1712fbd` | `internal/rulesaudit/legacy_differential_test.go` — full stdout/exit byte-identical to `rules-inventory.py` on 8 fabricated trees; `internal/skills` differential vs `skill-resolution.py` |
| WU7 | `e7f70f0` | `python3 tests/parity/run.py` **13/13 fixtures, 0 deltas**; `--self-test` 13/13; both read-only tests OK; `go test ./...` green |

Acceptance highlights (real captured evidence):

- `doctor` healthy project: `Summary: 21 OK, 1 INFO, 3 WARN, 0 ERROR`, exit 0;
  unsynced/broken project: exit 1 — both reproduced line-for-line by the harness,
  which compares stdout, stderr, exit code and the post-step project tree.
- `rules-audit` on a rich project: byte-identical JSON (indented, insertion
  order preserved, `resolved_skills` sorted by id).

## Decisions

- **N7 normalization** (`tests/parity/parity.py`): the legacy Python modules print
  their degraded-authority notices on stderr when no verified worktree-gate
  binary exists (`[  ! ]GO_*_BRIDGE_FALLBACK: ...`). The Go port performs those
  projections and classifications natively, so it has no temporary Python
  authority to announce and printing that claim would be false. N7 removes only
  those anchored announcement lines, identically on both legs, with the same
  rationale discipline as N1-N6; the roster, severities, messages, exit codes and
  report formatting stay compared verbatim.
- **Nonexistent path (doctor)**: the legacy dies inside bash
  (`lib/doctor.sh: line 46: cd: ...`, exit 1) because `doctor.sh` has no `-d`
  guard, which makes `doctor.py`'s own `is not a directory` guard dead code. The
  port keeps the FROZEN exit code 1 and prints the message that guard intended
  instead of fabricating a bash line number. Pinned by its own test in
  `internal/cli/smoke_test.go` rather than hidden in the differential table.
- **Nonexistent path (rules-audit)**: `rules-audit.sh` *does* have the guard, and
  it fires before any resolution, so the port prints the path exactly as given
  and exits 2 — byte-identical to bash, and the existing differential smoke case
  stays green.
- **`doctor-healthy` excludes `trello-mcp-workflow`**: its required `board_id`
  makes `sync` materialize a path-stamped `tracker-card-gate.sh` whose raw hash
  the lock records, so `.ai-specs.lock` can never agree between two isolated
  scratch roots (reproduced in explicit legacy-vs-legacy mode, i.e. it is an
  environment artifact, not the port). That recipe's dep/env coverage lives in
  `doctor-degraded`, which is zero-delta.
- **One grader per behaviour**: the verified-gate-binary resolver moved to
  `gatebinary.go`; the topology chain is reached through
  `target.ProjectRepoTopology`; the project-cache key/root derivation moved to
  `internal/skills`. Residual: `internal/doctor/projectcache.go` still holds a
  test-only `sanitizeBasename` used by `TestSanitizeBasename` (no production
  path calls it).

## Findings / out of scope

- `doctor.sh` has no `-d` guard, so the Python `ERROR: not a directory` branch
  is dead code and a nonexistent path exits 1 through a raw bash `cd:` error
  (parity contract §2). Reproduced as an exit code, not as bash text.
- Legacy doctor writes `__pycache__` into `$AI_SPECS_HOME/lib/_internal/`
  (defect D26), and the legacy `rules-audit` does the same for
  `rules-inventory.py` and its imports. The Go port sets
  `PYTHONDONTWRITEBYTECODE=1` and imports nothing without it, so both commands
  are strictly read-only — proven by one snapshot test per command.
- Legacy doctor also *executes* `git`, the gate binary `--selftest` and every
  recipe dep's `version_check`; those probes are read-only, and the port keeps
  them (with the same 5s/30s budgets) because the checks' verdicts depend on
  them.
- `_check_cli_version` is skipped by legacy when `lib/_internal/cli_version.py`
  is absent; the port runs it unconditionally (a compiled binary needs no
  Python module). Identical output whenever `lib/` is present (every fixture).
  Same for `_load_env_scaffold`/`_load_gate_binary`/`_load_util` module-load
  guards, which cannot fail for compiled code.
- `read_lock_meta`/`tomllib` failures abort legacy doctor with an uncaught
  traceback (a corrupt `.ai-specs.lock`, a manifest holding a TOML datetime,
  invalid UTF-8 in `AGENTS.md`/.envrc); the port degrades instead of fabricating
  a traceback. Documented per-site in code.
- Provider-side `exec.LookPath`/mode-bit checks approximate `shutil.which` and
  `os.access(X_OK)`; the malformed-manifest paths where Python would raise
  `AttributeError` are documented in code.
- Remaining bridged surface: the brief-provenance chain only. Follow-up: port
  `recipe-materialize`/`agents-render` (cards 07/09/10) and delete
  `internal/doctor/bridge.{go,py}`.
