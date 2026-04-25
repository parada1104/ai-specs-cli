---
title: sdd/definir-contrato-ai-specs-toml/apply-progress
topic_key: sdd/definir-contrato-ai-specs-toml/apply-progress
project: ai-specs-cli
type: architecture
---

# Apply Progress: Definir contrato del ai-specs.toml

## Mode

Strict TDD

## Follow-up Scope

Addressed verify-only gaps without expanding manifest runtime scope:

- added automated behavioral proof for `Campo mínimo por sección`
- added automated behavioral proof for `Campo fuera de alcance`
- added automated behavioral proof for `Plantilla consistente`
- added automated behavioral proof for `README consistente`
- strengthened `Manifest V1 mínimo válido` so documentation classification is asserted automatically
- expanded doc-proof assertions to cover every README V1 field row and every template V1 classification comment individually
- upgraded this artifact with explicit Safety Net and Triangulation evidence fields

## Completed Tasks

- [x] 1.1 Add RED tests in `tests/test_toml_read.py` for missing `[agents]`/`[[deps]]`/`[mcp]` sections resolving to stable defaults.
- [x] 1.2 Extend `tests/test_toml_read.py` with RED coverage for MCP `environment` input normalizing to canonical `env` output and preserving supported passthrough fields.
- [x] 1.3 Refactor `lib/_internal/toml-read.py` to expose explicit normalized readers for `project`, `agents`, `deps`, and `mcp` using the V1 canonical shapes from the design.
- [x] 2.1 Update `lib/_internal/vendor-skills.py` to consume normalized deps and keep validation limited to required `id` and `source` fields.
- [x] 2.2 Update `lib/_internal/mcp-render.py` to consume normalized MCP entries so `env` is canonical while `environment` remains a tolerated input alias.
- [x] 2.3 Add RED coverage in `tests/test_target_resolve.py` for normalized `project.subrepos` handling, keeping current invalid-path errors unchanged.
- [x] 2.4 Update `lib/_internal/target-resolve.py` to read normalized project data without changing existing `subrepos` validation semantics.
- [x] 3.1 Update `templates/ai-specs.toml.tmpl` comments to mark V1 fields as required, optional, defaulted, or tolerated alias without introducing new sections.
- [x] 3.2 Update `README.md` to document the root `ai-specs/ai-specs.toml` as the V1 source of truth, supported sections, conservative compatibility, and explicit out-of-scope items.
- [x] 4.1 Add RED coverage in `tests/test_sync_pipeline.py` for minimal valid manifests and backward-compatible manifests with omitted sections or MCP `environment`.
- [x] 4.2 Implement any sync-path fixes needed so `sync`/`sync-agent` accept the normalized V1 manifest shapes without new precedence or doctor behavior.
- [x] 4.3 Run `./tests/run.sh` and fix failures tied to this change; run `./tests/validate.sh` only as an informational check because known fixture gaps remain out of scope.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `tests/test_toml_read.py` | Unit | Historical baseline recovered from merged progress + verify report (`./tests/run.sh` green before follow-up) | Added missing-section tests before normalized readers existed. | `python3 -m unittest tests.test_toml_read` passed after implementing normalized readers. | Covered omitted `[agents]`, `[[deps]]`, and `[mcp]` defaults as distinct paths. | Centralized stable default normalization helpers in `toml-read.py`. |
| 1.2 | `tests/test_toml_read.py` | Unit | Historical baseline recovered from merged progress + verify report | Added alias test for `environment` → `env` before MCP normalization existed. | Alias test passed after canonical MCP normalization. | Non-empty env map + passthrough `enabled`/`timeout` exercised a second path beyond empty defaults. | Kept passthrough handling conservative. |
| 1.3 | `tests/test_toml_read.py` | Unit | Historical baseline recovered from merged progress + verify report | Same RED cycle as 1.1/1.2 established missing normalized reader API. | `python3 -m unittest tests.test_toml_read` passed with explicit readers. | Reader API had to satisfy both missing-section and alias scenarios. | Simplified shared normalization helpers for strings/maps/subrepos. |
| 2.1 | `tests/test_sync_pipeline.py` | Integration via subprocess unittest | Historical baseline recovered from merged progress + verify report | Sync path would fail against normalized deps until vendoring consumed canonical shape. | Sync tests passed after updating `vendor-skills.py`. | Exercised minimal manifest flow and vendoring-adjacent flow separately. | Validation stayed limited to `id` + `source`. |
| 2.2 | `tests/test_sync_pipeline.py` | Integration via subprocess unittest | Historical baseline recovered from merged progress + verify report | Added sync test for MCP `environment`; renderer still expected legacy shape. | `python3 -m unittest tests.test_sync_pipeline` passed after renderer consumed canonical `env`. | Alias case plus no-MCP case forced non-trivial renderer behavior. | Removed duplicate alias handling from renderer. |
| 2.3 | `tests/test_target_resolve.py` | Unit | Historical baseline recovered from merged progress + verify report | Added normalized `project.subrepos` coverage before runtime alignment. | `python3 -m unittest tests.test_target_resolve` passed after target resolver consumed normalized project data. | Valid target resolution + invalid path cases covered separate branches. | Preserved existing invalid-path errors and dedup semantics. |
| 2.4 | `tests/test_target_resolve.py` | Unit | Historical baseline recovered from merged progress + verify report | Same RED as 2.3. | `target-resolve.py` read canonical `project["subrepos"]` with tests green. | Valid path, escape path, and missing directory scenarios forced current semantics. | No semantic expansion beyond current validation. |
| 3.1 | `tests/test_manifest_contract_docs.py` | Unit | ✅ Historical 9-test safety net before first doc follow-up, plus current `python3 -m unittest tests.test_manifest_contract_docs` → 4 tests passing before strengthening assertions | Added doc-contract tests first; template assertions failed on missing explicit ONLY/deferred language. Current proof-strengthening extended that same test file to assert every template V1 classification comment explicitly. | `python3 -m unittest tests.test_manifest_contract_docs` now passes with 5 tests. | Template surface, deps rows, MCP rows, alias wording, and deferred out-of-scope wording are asserted as distinct paths. | Kept template comments aligned with runtime-only supported behavior. |
| 3.2 | `tests/test_manifest_contract_docs.py` | Unit | ✅ Historical 9-test safety net before first doc follow-up, plus current `python3 -m unittest tests.test_manifest_contract_docs` → 4 tests passing before strengthening assertions | Same RED cycle as 3.1; README lacked explicit no-extra-sections / deferred wording required by the first doc proof. Current proof-strengthening extended coverage to every README V1 field row individually. | `python3 -m unittest tests.test_manifest_contract_docs` now passes with 5 tests. | README source-of-truth, canonical surface, compatibility rules, all field rows, and deferred out-of-scope statements are asserted separately. | Kept README conservative and root-manifest-centric. |
| 4.1 | `tests/test_sync_pipeline.py`, `tests/test_manifest_contract_docs.py` | Integration + Unit | Historical baseline recovered from merged progress; doc follow-up used the 9-test baseline and current doc-test safety net above | Existing runtime RED was already recorded; follow-up docs tests added missing automated proof for manifest classification and current strengthening made that proof exhaustive per field/comment. | Full contract proof green with `python3 -m unittest tests.test_manifest_contract_docs` and `./tests/run.sh`. | Runtime minimal-manifest path plus exhaustive doc classification assertions remove the previous documentation-partial warning. | Reused existing runtime fixtures and added proof-strengthening assertions only. |
| 4.2 | `tests/test_sync_pipeline.py` | Integration via subprocess unittest | Historical baseline recovered from merged progress + verify report | Same RED as 4.1. | `sync-agent.sh` already green; remained green under full suite. | Minimal manifest + MCP alias + subrepo fan-out covered distinct sync paths. | Kept precedence/doctor behavior untouched. |
| 4.3 | `./tests/run.sh` | Suite | ✅ `python3 -m unittest tests.test_toml_read tests.test_sync_pipeline` → 9 tests passing before first follow-up; current doc-test safety net stayed green before the assertion expansion | Full-suite proof expanded first from 14 to 18 tests when doc contract tests were added, then to 19 tests when exhaustive per-field assertions were added. | `./tests/run.sh` remains green after the strengthened proof. | New exhaustive doc tests plus prior runtime tests cover both documentation and execution paths. | No code refactor needed beyond artifact/evidence refresh. |

## Files Changed

| File | Action | Notes |
|---|---|---|
| `tests/test_toml_read.py` | Created | New normalization coverage for defaults and MCP alias handling. |
| `tests/test_target_resolve.py` | Modified | Added normalized `subrepos` coverage. |
| `tests/test_sync_pipeline.py` | Modified | Added minimal-manifest and MCP alias sync coverage. |
| `tests/test_manifest_contract_docs.py` | Created | Added automated proof for README/template contract classification and deferred out-of-scope items. |
| `tests/fixtures/target-resolve/multi-target/packages/{a,b}/.gitkeep` | Created | Completed target-resolve fixture directories. |
| `tests/fixtures/sync-workspace/root/**/.gitkeep` | Created | Added sync workspace fixture tree required by integration tests. |
| `lib/_internal/toml-read.py` | Modified | Added canonical normalized readers and alias/default handling. |
| `lib/_internal/vendor-skills.py` | Modified | Now consumes normalized deps. |
| `lib/_internal/mcp-render.py` | Modified | Now consumes normalized MCP data. |
| `lib/_internal/target-resolve.py` | Modified | Reads normalized project payload. |
| `lib/sync-agent.sh` | Modified | Reads canonical agents payload shape. |
| `templates/ai-specs.toml.tmpl` | Modified | Added explicit ONLY/deferred wording so template assertions prove canonical surface and out-of-scope limits. |
| `README.md` | Modified | Added explicit no-extra-sections / deferred wording so README assertions prove canonical surface and out-of-scope limits. |
| `openspec/changes/definir-contrato-ai-specs-toml/tasks.md` | Modified | Marked all scoped tasks complete. |

## Deviations from Design

None — implementation matches design.

## Issues Found

- The worktree was missing fixture directories required by existing sync/target tests, so the change added the minimal `.gitkeep` tree to keep the suite reproducible here.

## Verification

- `python3 -m unittest tests.test_toml_read`
- `python3 -m unittest tests.test_target_resolve`
- `python3 -m unittest tests.test_sync_pipeline`
- `python3 -m unittest tests.test_manifest_contract_docs` (RED first: 4 failures, then GREEN)
- `python3 -m unittest tests.test_toml_read tests.test_sync_pipeline` (safety net before doc follow-up: 9 passing)
- `python3 -m unittest tests.test_manifest_contract_docs` (current safety net before exhaustive assertion expansion: 4 passing)
- `python3 -m unittest tests.test_manifest_contract_docs` (after exhaustive assertion expansion: 5 passing)
- `./tests/run.sh`
- `./tests/validate.sh` (informational)

## Test Summary

- **Total tests in `tests/test_manifest_contract_docs.py` now**: 5
- **Total suite test methods now**: 19
- **Layers used in follow-up**: Unit (5)
- **Approval tests**: None — no behavior-preserving refactor of runtime code in this follow-up batch
- **Pure functions created**: 0

## Status

12/12 scoped tasks complete. Verify gaps addressed; ready for re-verify.
