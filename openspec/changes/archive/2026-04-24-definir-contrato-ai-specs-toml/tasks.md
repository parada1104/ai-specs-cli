# Tasks: Definir contrato del ai-specs.toml

## Phase 1: Contract normalization foundation

- [x] 1.1 Add RED tests in `tests/test_toml_read.py` for missing `[agents]`/`[[deps]]`/`[mcp]` sections resolving to stable defaults.
- [x] 1.2 Extend `tests/test_toml_read.py` with RED coverage for MCP `environment` input normalizing to canonical `env` output and preserving supported passthrough fields.
- [x] 1.3 Refactor `lib/_internal/toml-read.py` to expose explicit normalized readers for `project`, `agents`, `deps`, and `mcp` using the V1 canonical shapes from the design.

## Phase 2: Runtime consumers and validation

- [x] 2.1 Update `lib/_internal/vendor-skills.py` to consume normalized deps and keep validation limited to required `id` and `source` fields.
- [x] 2.2 Update `lib/_internal/mcp-render.py` to consume normalized MCP entries so `env` is canonical while `environment` remains a tolerated input alias.
- [x] 2.3 Add RED coverage in `tests/test_target_resolve.py` for normalized `project.subrepos` handling, keeping current invalid-path errors unchanged.
- [x] 2.4 Update `lib/_internal/target-resolve.py` to read normalized project data without changing existing `subrepos` validation semantics.

## Phase 3: Contract docs and template alignment

- [x] 3.1 Update `templates/ai-specs.toml.tmpl` comments to mark V1 fields as required, optional, defaulted, or tolerated alias without introducing new sections.
- [x] 3.2 Update `README.md` to document the root `ai-specs/ai-specs.toml` as the V1 source of truth, supported sections, conservative compatibility, and explicit out-of-scope items.

## Phase 4: Integration checks and verification

- [x] 4.1 Add RED coverage in `tests/test_sync_pipeline.py` for minimal valid manifests and backward-compatible manifests with omitted sections or MCP `environment`.
- [x] 4.2 Implement any sync-path fixes needed so `sync`/`sync-agent` accept the normalized V1 manifest shapes without new precedence or doctor behavior.
- [x] 4.3 Run `./tests/run.sh` and fix failures tied to this change; run `./tests/validate.sh` only as an informational check because known fixture gaps remain out of scope.
