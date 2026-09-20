# go-resolved-config

Strangler slice: move the `build_resolved_config` manifest projection into the Go gate binary.

## Tracker

- card_id: 6aaff7239bb3dbc948bbbd60
- url: https://trello.com/c/ek58XxrE/143-migrate-buildresolvedconfig-projection-into-go-strangler

## Objective

Make Go the authority for the resolved-config projection consumed by sync renderers and doctor: a pure function of `ai-specs/ai-specs.toml` + project root. Python keeps a fail-open compatibility bridge plus a parity suite that forbids Python authority.

## Contract (from T1 map)

`build_resolved_config(project_root)` returns:

- `bindings`: `{capability: recipe}` from explicit `[[bindings]]` (both present, else skipped)
- `recipes`: `{id: config}` — config merges `[recipes.<id>.config]` table with flat keys; drops `enabled`/`version`; non-table values skipped
- `enabled`: ids with `enabled = true`, manifest order
- `project_root`: `str(Path(project_root).resolve())`
- `topology`: `topology_config` ([project].repo_topology wins; `recipes.worktree-flow.config.repo_topology` legacy alias with deprecation; else auto) + `resolve_repo_topology` (standalone/monorepo-apps passthrough; monorepo-submodules/auto → `detect_submodules`; git failure degrades to standalone)

Downstream (stays Python this slice): bindings override with auto-bind map, catalog-default merge, brief fragments attach.

Consumers: `materialize_recipes` (no-enabled path + resolved-config write), `build_resolved_config_only`, `doctor.py` (x2).

## Shape

- Go: new `--plan-resolved-config --project <root>` query in `worktree-gate` (JSON envelope on stdout, exit 0; malformed manifest → exit 2), reusing the embedded tomllib seam (`recipe_toml.go` pattern) and `parseManifestBindings`.
- Python: `build_resolved_config` tries Go first; fail-open to the existing Python body when Go is unavailable (env switch `GO_RESOLVED_CONFIG_BRIDGE_FALLBACK`), following `GO_ORPHANS_BRIDGE_FALLBACK`.
- Parity suite: fresh-project fixture comparing Go envelope vs Python projection; Python authority forbidden in tests.

## Tasks

- [x] T1 — Read-only map: contract, consumers, existing Go seams (`recipe_toml.go`, `bindings.go`), missing pieces (recipes projection, topology resolution). Done: contract above.
- [ ] T2 — RED tests: Go query contract (JSON envelope, exit codes, topology cases) in `gate/resolved_config_test.go`.
- [ ] T3 — GREEN: implement `--plan-resolved-config` in Go (`resolved_config.go` + `topology_resolve.go`).
- [ ] T4 — Python fail-open bridge in `build_resolved_config` + parity suite in `tests/test_resolved_config_bridge.py`.
- [ ] T5 — SHA256SUMS regen (canonical go1.24.13) + docs touchpoints.
- [ ] T6 — Full validation `./tests/validate.sh` + smoke fresh project.
- [ ] T7 — Delivery: PR to `development` via gh; user decides merge.

## Constraints

- Go owns the projection decision; Python is a thin acquisition/bridge.
- No new Go TOML dependency — reuse the embedded tomllib seam.
- Recipes stay declarative; no unrelated rewrites.
- TDD runner: focused Go/pytest suites first, then `./tests/validate.sh`.

## Progress

- Worktree: `.worktrees/go-resolved-config` (branch `feat/go-resolved-config`)
- Base: development @ 5c03bdd
- T1 evidence: map recorded in this document (2026-09-20)
