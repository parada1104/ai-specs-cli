# Spec-Driven Development (SDD) in ai-specs

> **Scope:** This document describes the SDD integration shipped with ai-specs v1.
> It is a first-class feature, but **strictly optional** — projects without `[sdd]`
> in their manifest work exactly the same.

## What is SDD?

**Spec-Driven Development** is a workflow where changes are designed, specified,
and verified before (and during) implementation. In ai-specs, SDD is provided by
an external **provider CLI** — today only **OpenSpec** (`@fission-ai/openspec` on npm)
is supported.

When you enable SDD:

1. `ai-specs` declares `[sdd]` in your manifest
2. `openspec/` directory is scaffolded with a `spec-driven` schema configuration
3. Bundled skills and commands for the OpenSpec workflow are copied into your project
4. Your agent gains slash commands (`/opsx-*`) to create, implement, verify, and archive changes

## Enabling SDD

### Prerequisites

- **Node.js ≥ 20.19**
- `openspec` on `PATH` (or use `--install-provider-cli` to auto-install)

### Quick enable

```bash
# Auto-install OpenSpec globally (requires npm)
ai-specs sdd enable --install-provider-cli

# Or if you already have openspec installed
ai-specs sdd enable

# With explicit artifact store
ai-specs sdd enable --artifact-store filesystem
```

This writes to your `ai-specs/ai-specs.toml`:

```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "hybrid"
```

And scaffolds:

```
openspec/
├── config.yaml          ← provider configuration
├── changes/             ← active changes (proposal → design → tasks → apply)
└── specs/               ← canonical specs synced from completed changes
```

### Status check (read-only)

```bash
ai-specs sdd status
```

Shows `[sdd]` manifest block, toolchain presence (`node`, `openspec`), and
`openspec/` directory health.

### Disable

```bash
ai-specs sdd disable
```

Sets `[sdd].enabled = false`. Does **not** delete `openspec/` — your change
history is preserved.

## `[sdd]` manifest fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `enabled` | No | — | Boolean; must be `true` for SDD to be active |
| `provider` | No | `"openspec"` | Provider CLI identifier. **v1 only:** `"openspec"` |
| `artifact_store` | No | `"hybrid"` | Where artifacts live: `filesystem`, `hybrid`, or `memory` |

### `artifact_store` values

| Value | Description | `openspec/` required? |
|-------|-------------|----------------------|
| `filesystem` | All artifacts on disk; most predictable for CI | Yes |
| `hybrid` | Filesystem primary; may layer memory heuristics later | Yes |
| `memory` | Experimental; OpenSpec stays file-first in v1 | No (warns if missing) |

## OpenSpec provider (v1)

### What OpenSpec provides

OpenSpec is the reference implementation of a spec-driven workflow. It manages:

- **Artifact lifecycle:** `proposal` → `exploration` → `specs` → `design` → `tasks` → `apply-progress` → `verify-report` → `archive`
- **Schema enforcement:** Validates that changes follow the declared schema (e.g., `spec-driven`)
- **Skill generation:** Creates agent-compatible skills from change artifacts

### Relationship with ai-specs

```
ai-specs (manifest [sdd])          OpenSpec CLI (external)
        │                                │
        ├─ sdd enable ─────────────────►│ openspec init
        │                                │ openspec update
        ├─ merges config defaults ◄──────┤
        │                                │
        └─ refresh-bundled ─────────────►│ copies skills/commands
        │         --preset openspec      │ to ai-specs/skills/
        │                                │
        ◄──────── opsx-* commands ───────┘ generated slash commands
```

`ai-specs` does **not** reimplement OpenSpec — it:

1. Verifies the toolchain exists
2. Runs `openspec init` / `openspec update` when safe
3. Merges non-destructive config defaults into `openspec/config.yaml`
4. Refreshes bundled OpenSpec skills via `ai-specs refresh-bundled --preset openspec`

### Generated slash commands (opsx-*)

After `ai-specs sdd enable` + `ai-specs sync`, your agent gains:

| Command | Purpose |
|---------|---------|
| `/opsx-onboard` | Guided first-time walkthrough |
| `/opsx-new` | Start a new change, step through artifacts |
| `/opsx-continue` | Continue an existing change |
| `/opsx-propose` | Create a change with all artifacts at once |
| `/opsx-ff` | Fast-forward: skip steps, generate all artifacts |
| `/opsx-explore` | Think through a problem before implementing |
| `/opsx-apply` | Implement tasks from a change |
| `/opsx-verify` | Verify implementation matches change artifacts |
| `/opsx-archive` | Archive a completed change |
| `/opsx-sync` | Sync delta specs to main specs |

These are **generated commands** — they live in `.cursor/commands/`,
`.opencode/commands/`, `.claude/commands/`, etc., and are recreated on every
`ai-specs sync`.

## Provider contract (forward-looking)

Future providers would need to satisfy a minimal toolchain/directory/config/integration
contract modeled on the OpenSpec integration above: an executable on `PATH` with
`init` and `update` commands, an `openspec/`-style workspace directory with
`changes/` and `specs/`, and a config file at `openspec/config.yaml`. The current
v1 only supports `provider = "openspec"`.

For a complete OpenSpec `config.yaml` example see
[`docs/ai/examples/config.yaml`](examples/config.yaml). For common SDD failure
scenarios and fixes see [`docs/ai/troubleshooting.md`](troubleshooting.md).

## Limitations in v1

1. **Single provider:** Only `provider = "openspec"` is supported. Passing any other
   value to `ai-specs sdd enable` is rejected.

2. **File-first:** Even with `artifact_store = "memory"`, OpenSpec itself is
   filesystem-oriented. The memory store is experimental and may WARN during
   `ai-specs doctor`.

3. **No custom schemas:** The schema is always `spec-driven` as determined by
   the OpenSpec CLI profile.

4. **No rollback of openspec init:** `--force` is destructive; there is no
   automatic backup of the previous `openspec/` state.

## See also

- [`README.md`](../../README.md) — Quick start and manifest contract
- [`docs/ai/examples/config.yaml`](examples/config.yaml) — Full OpenSpec config example
- [`docs/ai/troubleshooting.md`](troubleshooting.md) — SDD failure scenarios and fixes
- [`catalog/skills/openspec-sdd-conventions/SKILL.md`](../../catalog/skills/openspec-sdd-conventions/SKILL.md) — Commit conventions during SDD apply
- [`catalog/skills/testing-foundation/SKILL.md`](../../catalog/skills/testing-foundation/SKILL.md) — Default testing commands
- [OpenSpec npm package](https://www.npmjs.com/package/@fission-ai/openspec) — Provider CLI documentation
