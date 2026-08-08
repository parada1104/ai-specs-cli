# Spec: agent-assisted recipe configuration

## Purpose

The harness provides a deterministic, non-interactive recipe configuration helper
and an agent-facing playbook. The flow inspects repository state, presents a
recommendation, applies approved schema-valid values surgically, optionally
syncs and verifies, and emits a versioned report.

## Requirements

### Deterministic helper

`ai-specs recipe configure <id> [path]` SHALL support `--inspect --json`,
repeatable `--set KEY=VALUE`, `--dry-run`, `--sync`, and
`--ignore-cli-version`. JSON output SHALL be deterministic, versioned, free of
absolute paths/timestamps/PIDs/hostnames, and use project-relative paths. Exit
codes SHALL be 0 success/no-op, 1 apply/sync failure or partial, 2 usage, 3
rejected request, and 4 preflight block. Codes 3 and 4 SHALL perform no write.
The existing interactive `configure-recipes` wizard SHALL remain available and
unchanged except for shared writer preservation behavior.

### Grounded recommendation

Inspection SHALL include recipe schema fields (type, required, enum, default,
and help text), current config and unknown keys, dependency/MCP signals, and
repository topology when `repo_topology` is declared. Topology detection SHALL
work without `init.md`; no submodule signal SHALL ask about `monorepo-apps`
rather than asserting standalone. The agent playbook SHALL stop for explicit
approval before apply.

### Canonical apply

Approved values SHALL update only `[recipes.<id>.config]`, preserve unmentioned
keys and comments, and be idempotent. Equivalent parsed values SHALL leave the
manifest byte-identical and report `status: no-op` with no changed keys. The
shared writer SHALL preserve trailing inline comments, distinguish `#` inside
strings from comments, and reject multiline replacements without partial write.

### Preserve overrides and secrets

Apply and sync SHALL NOT overwrite or delete files under project recipe
`overrides/` trees. Drift MAY be reported. Secret-shaped literal values SHALL
be rejected; `${env:VAR}` references MAY pass through.

### Preflight, sync, and verify

The `[tool]` CLI version policy, including malformed policies, SHALL be checked
before apply. A violation SHALL report `blocked`, exit 4, leave files unchanged,
and not invoke sync. `--ignore-cli-version` SHALL record and forward the bypass.
Lock `meta.cli_version` staleness SHALL be informational. With `--sync`, a
successful write SHALL invoke sync and doctor; sync failure after a write SHALL
report `partial`, exit 1, failed step, no rollback, and no lock stamp.

### Structured report

Apply reports SHALL include `report_version`, status, recipe, changed/unchanged/
preserved keys, preflight, sync, doctor verify counts (null and `parsed: false`
when the summary cannot be parsed), assumptions, drift, and gaps. A partial
report SHALL never claim the project is fully configured.

### Runtime evidence

A new additive client under `tests/evals/eval_assisted_configure_live.py` SHALL
exercise five natural-language scenarios for worktree-flow,
trello-mcp-workflow evidence, and plan-build-flow plain config. It SHALL reuse
the existing scenario, fixture, assertion, isolation, trial, and runner
semantics; `./tests/run.sh` SHALL not collect it. An optional Orca/OMP layer MAY
invoke existing runners across runtimes and aggregate per-runtime provenance,
but SHALL NOT alter eval semantics or be required.

### Documentation

The harness-recipes and harness-lifecycle bundled skills SHALL document
inspect → recommend → approval → apply → sync/verify → report, preservation,
and no-secret-literal rules. Project and eval documentation SHALL describe the
helper, evidence tiers, and optional orchestration.
