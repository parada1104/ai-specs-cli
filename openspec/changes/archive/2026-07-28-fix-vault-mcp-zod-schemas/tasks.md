# Tasks: fix-vault-mcp-zod-schemas

Depth: **tasks-only**

Rationale for depth: this is a defect fix that **upholds** every specified behavior
rather than superseding one. The pin requirement from `vault-canonical-reinforce`
("**AND** the package pin is `@modelcontextprotocol/server-filesystem@2025.7.1`",
archived `specs/vault-canonical-store/spec.md:66`) is preserved, and no live spec under
`openspec/specs/` asserts anything this change alters. Scope is one template line plus
its test and doc assertions.

Precedent: template fixes in this repo do not bump the recipe version (see #151,
`f4c5ecf`, which changed `recipe.toml` targets without touching `version`), but do add
a `CHANGELOG.md` entry.

Execution mode: **strict TDD** (`openspec/config.yaml: strict_tdd: true`).
Phase 1 MUST show RED before the Phase 2 template change lands.

Tracker: not linked — no card exists for this defect yet.

---

## Problem

`vault-fs-mcp.sh` runs `npx -y @modelcontextprotocol/server-filesystem@2025.7.1`. That
version does not declare `zod` directly; it inherits it from
`@modelcontextprotocol/sdk`, which `npx` now resolves to **zod 4.x**. Its pinned
`zod-to-json-schema@^3.23.5` only understands zod 3 internals (`_def.typeName`), so
with zod 4 schemas it silently emits `{"$schema": "..."}` — no `type`, no
`properties`. Hosts that validate tool schemas reject the entire list:

```
tools[0].inputSchema.type: Invalid input: expected "object"
```

Measured over stdio: 11 of 12 tools emit an empty schema. Only
`list_allowed_directories` survives, because its schema is a hand-written object
literal rather than a zod conversion.

Nothing in this repository changed. `npx -y` re-resolves transitive dependencies on
every launch, so zod 4's release broke a working pin retroactively.

## Why the pin stays

Bumping the package was considered and rejected. `2025.7.29+` replaces argv
directories with MCP client roots whenever the client advertises the capability, with
no opt-out (`dist/index.js:602`; package README: "Server replaces ALL allowed
directories with client's roots"). Measured consequences:

- With only the workspace as a root, the vault is denied outright:
  `Access denied - path outside allowed directories: <vault> not in <workspace>`.
- With workspace **and** vault as roots, allowed directories become **both**, and
  `list_directory` on the consumer repo's `evidence/` succeeds. A host always
  advertises its cwd as a root, so a vault-only scope is unreachable under roots.

That second point is the deciding one: the recipe's narrow scope — a store reachable
only inside `CANONICAL_VAULT_PATH` — cannot be expressed under roots at all. Keeping
the pin keeps `CANONICAL_VAULT_PATH` authoritative and the MCP surface narrow.

## Fix

Force `zod@3` to hoist above the SDK's zod 4 by naming it as a second `npx -p`
package, so `server-filesystem` resolves the zod its `zod-to-json-schema` expects:

```bash
exec npx -y \
  -p "@modelcontextprotocol/server-filesystem@2025.7.1" \
  -p "zod@3" \
  mcp-server-filesystem "$ROOT"
```

Measured against a real vault path: 12 tools with valid schemas, vault listing works,
and a path outside the scope is still denied. As a side benefit this makes the launch
more reproducible — the previous form let a transitive dependency float on every run,
which is what broke it.

---

## Phase 1 — Tests (RED)

- [x] **T1.1** — `tests/test_vault_fs_mcp.sh`: assert the argv log contains the `zod@3`
  pin and the `mcp-server-filesystem` binary name, alongside the existing
  `server-filesystem@2025.7.1` assertion.
  **Done when:** the new assertions fail against the current template.

- [x] **T1.2** — `tests/test_vault_canonical_store_recipe.py`: assert the wrapper text
  pins `zod@3` and still pins `server-filesystem@2025.7.1`.
  **Done when:** the new assertion fails against the current template.

## Phase 2 — Template (GREEN)

- [x] **T2.1** — Update the `exec` line in
  `catalog/recipes/vault-canonical-store/templates/vault-fs-mcp.sh`, with a comment
  recording why zod is pinned separately from the package.
  **Done when:** Phase 1 tests pass and the existing argv/scope assertions stay green.

## Phase 3 — Docs

- [x] **T3.1** — `catalog/recipes/vault-canonical-store/README.md`: document the zod
  pin next to the package pin; correct the stale "Claude Code 2.1.x fails tools-fetch
  on the recipe pin `2025.7.1`" host note, which this change fixes.

- [x] **T3.2** — `tests/evals/README.md`: same correction to the host notes; the
  `2025.11.25` + `--add-dir` live-registration workaround exists only because the
  recipe pin could not fetch tools.

- [x] **T3.3** — `CHANGELOG.md`: entry under `## [Unreleased]` → `### Fixed`.

## Phase 4 — Validation

- [x] **T4.1** — `./tests/validate.sh` green.
- [x] **T4.2** — `python3 tests/smoke_vault_mcp_fs.py` green.
