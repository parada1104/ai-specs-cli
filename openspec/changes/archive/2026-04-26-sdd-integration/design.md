## Context

`ai-specs` is a Bash-first CLI (`bin/ai-specs` → `lib/*.sh`) with Python helpers (`lib/_internal/*.py`) for TOML, templating, vendoring, and agent fan-out. Projects already use `ai-specs init`, `sync`, `refresh-bundled`, and `doctor`. OpenSpec upstream ships as **`@fission-ai/openspec`** on npm (CLI binary `openspec`), requires **Node.js ≥ 20.19.0**, and provides `openspec init`, `openspec update`, `openspec config profile`, and workflow schemas including spec-driven artifacts.

Teams want a **declarative switch** in `ai-specs.toml` plus a **single CLI path** to install/verify the provider toolchain, scaffold or align `openspec/`, and refresh **bundled OpenSpec-oriented skills/commands** from this repository’s catalog—without forking OpenSpec itself.

## Goals / Non-Goals

**Goals:**

- Introduce `ai-specs sdd` (exact flags to be finalized in implementation; proposal used `sdd --enable true --provider openspec`) as the user-facing orchestration entrypoint.
- Persist SDD intent in **`ai-specs/ai-specs.toml`** under an optional **`[sdd]`** table consumed by `sync`, `doctor`, and `sdd`.
- **OpenSpec provider (v1):** verify `openspec` on `PATH`; optionally run **`npm install -g @fission-ai/openspec@latest`** when the user explicitly opts in (see decisions).
- Ensure **`openspec/`** exists and is coherent: prefer delegating layout to **`openspec init`** where possible; merge or post-process only for ai-specs-specific defaults (e.g. `openspec/config.yaml` alignment with `openspec-sdd-conventions` and project testing commands).
- **Reuse** `refresh-bundled` / catalog machinery to materialize OpenSpec skills and slash-commands from the CLI bundle into `ai-specs/skills/` and `ai-specs/commands/`, and optionally add `[[deps]]` rows for policy skills already in `catalog/`.
- **`artifact_store`:** support **`filesystem`** and **`hybrid`** as fully specified behaviors for OpenSpec; **`memory`** as a documented contract with minimal or stub runtime behavior until a concrete agent memory integration exists.
- Extend **`ai-specs doctor`** with SDD-related checks (non-mutating) when `[sdd]` is enabled.

**Non-Goals:**

- Reimplement OpenSpec change lifecycle, validation, or archive semantics inside ai-specs.
- Choose or enforce a single global package manager for all users (npm global is default; document alternatives).
- Persist artifacts only in vendor-specific memory in v1 without an explicit, testable adapter contract.

## Decisions

### D1: Bash wrapper + Python module for TOML merge and validation

**Choice:** `lib/sdd.sh` dispatches subcommands; structured parsing, enum validation, and optional TOML rewrites live in `lib/_internal/sdd.py` (or split helpers).

**Rationale:** Matches `doctor.py`, `target-resolve.py`, keeps bash for argv/`--help` parity with other commands.

**Alternatives:** Pure bash TOML editing (rejected: brittle). Shell-out to `toml-cli` (rejected: new external dep).

### D2: Provider adapter table (internal)

**Choice:** A small Python registry mapping `provider` string → adapter object with methods: `ensure_cli()`, `init_layout(project_root, options)`, `doctor_checks()`, `recommended_catalog_deps()`.

**Rationale:** Keeps `openspec` specifics out of generic `sdd.sh` branches beyond dispatch.

**Alternatives:** One giant shell `case` (works for 1 provider, scales poorly).

### D3: OpenSpec install — verify by default, install only with explicit consent

**Choice:** Default path: if `openspec` missing → **ERROR** with message listing `npm install -g @fission-ai/openspec@latest` (and note Node 20.19+). Separate flag e.g. **`--install-provider-cli`** performs install via `npm` when present.

**Rationale:** Global npm installs are sensitive (permissions, supply chain, CI without node). Users and CI should opt in explicitly.

**Alternatives:** Always auto-install (rejected: surprising in enterprise). Never document install (rejected: poor UX).

### D4: Layout initialization delegates to `openspec init`

**Choice:** When `openspec/` is missing or incomplete, run **`openspec init`** non-interactively with flags derived from `[agents].enabled` in `ai-specs.toml` (e.g. map to `--tools cursor,claude,...`) and **`--profile`** aligned with “extended” workflow (upstream: `openspec config profile` + `openspec update` — design assumes **`custom` or extended profile** equivalent per OpenSpec docs; exact profile name MUST match upstream at implementation time).

**Rationale:** Avoid drift from OpenSpec’s supported tool matrix and update mechanism.

**Post-init:** Optionally overlay or merge **`openspec/config.yaml`** snippets from ai-specs templates so `testing.command` and schema `spec-driven` match this repo’s conventions.

### D5: Bundled skills/commands — same pipeline as today

**Choice:** Reuse **`refresh-bundled`** / lockfile semantics for OpenSpec skills in `catalog/skills/openspec-*` and commands; `sdd --enable` may invoke the same Python entrypoint with a curated file list or “preset name” `openspec`.

**Rationale:** One codepath for upgrades and user customization (`.new` sidecars).

### D6: `artifact_store` behavior matrix (OpenSpec provider)

| Value        | `openspec/` on disk | Session / operative memory |
|-------------|---------------------|-----------------------------|
| `filesystem`| Required canonical | Not used by ai-specs |
| `hybrid`    | Required canonical | MAY emit hints for agents (e.g. AGENTS.md snippet or `doctor` WARN) when memory API unavailable |
| `memory`    | MUST NOT be required for CLI operation in v1; if `openspec/` absent, `doctor` MAY WARN; full memory persistence **deferred** to adapter | Contract only until adapter ships |

**Rationale:** OpenSpec is inherently file-based; “memory-only” without files conflicts with `openspec validate`. v1 defines honest semantics.

### D7: Disable / rollback

**Choice:** `sdd --enable false` removes or sets `enabled = false` in `[sdd]` without deleting `openspec/` or skills; optional **`--purge`** (future task) guarded and documented.

## Risks / Trade-offs

- **[Risk]** Node version &lt; 20.19 → OpenSpec fails after install → **Mitigation:** `doctor` and `sdd` preflight `node -v` when install or execute path requested.
- **[Risk]** `openspec init` overwrites user customizations → **Mitigation:** refuse init when `openspec/` already exists unless `--force` (explicit); document diff strategy.
- **[Risk]** Profile names drift upstream → **Mitigation:** pin documented profile string in tests; single helper constant sourced from template.
- **[Risk]** `openspec/config.yaml` rules reference `apply`/`verify` while schema is `spec-driven` (already warned by OpenSpec CLI) → **Mitigation:** align repo root `openspec/config.yaml` in a separate small change or same apply phase; not blocking SDD CLI design.
- **[Trade-off]** npm global vs npx → global matches user story; document npx/bun as unsupported-but-possible for power users.

## Flows

### Sequence: first-time enable (happy path)

```text
User          bin/ai-specs    lib/sdd.sh      sdd.py       openspec (npm)     refresh-bundled
  |                |              |              |                |                  |
  | sdd --enable   |              |              |                |                  |
  |--------------->|              |              |                |                  |
  |                | load TOML    |              |                |                  |
  |                |------------->| parse [sdd]  |                |                  |
  |                |              |------------->|                |                  |
  |                |              | verify PATH  |                |                  |
  |                |              |------------------------------>| version / help   |
  |                |              |              | init if needed |                  |
  |                |              |------------------------------>| openspec init    |
  |                |              | merge config.yaml template    |                  |
  |                |              |--------------|                |                  |
  |                | refresh-bundled preset openspec              |                  |
  |                |------------------------------------------------|----------------->|
  |                | write [sdd] enabled=true                         |                  |
  |<---------------| OK summary                                     |                  |
```

### Sequence: doctor with SDD enabled

```text
User       doctor.sh     doctor.py        filesystem
 |             |             |                |
 | doctor      |             |                |
 |------------>|             |                |
 |             | read [sdd]  |                |
 |             |------------>|                |
 |             |             | stat openspec/ |
 |             |             |-------------->|
 |             |             | run openspec validate (optional WARN)
 |             |<------------|                |
 |<------------| OK/WARN/ERROR lines          |
```

## Migration Plan

1. Ship `sdd` behind documented experimental note in README if needed (optional).
2. Existing projects: no `[sdd]` → no behavior change.
3. Enabling SDD: user runs `ai-specs sdd ...`; commit `ai-specs.toml`, lockfile changes, new `openspec/`, skills — same PR hygiene as `init`/`sync`.
4. Rollback: `enabled = false` or remove `[sdd]`; keep `openspec/` in repo for history unless team deletes manually.

## Open Questions

1. **Exact CLI surface:** subcommand `sdd` only vs `sdd enable` / `sdd doctor` — needs product consistency with `doctor`.
2. **Profile string:** confirm at implementation time the exact `openspec config profile` value for “extended” (upstream naming).
3. **`memory` mode MVP:** only `doctor` metadata + documentation, or also emit a machine-readable sidecar for external memory tools?
4. **Interaction with `ai-specs sync`:** should `sync` auto-emit WARN when `[sdd].enabled` and `openspec/` missing, or keep all checks in `doctor` only?
5. **Version pinning:** always `@latest` vs optional `openspec_version` key in TOML?
