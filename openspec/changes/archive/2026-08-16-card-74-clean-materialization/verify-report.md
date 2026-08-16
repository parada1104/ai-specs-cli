```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:3369bcbd585a0b35f35526800eeb1fa526f1a7dcd4b1771592b8cdc09b7c7256
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 7/7
scenarios: 8/8
test_command: ./tests/validate.sh
test_exit_code: 0
test_output_hash: sha256:fee696d747b7b149a98155456f2f9e0f46e195f0de2e1023b31946460245cd1d
build_command: python3 -m py_compile lib/_internal/*.py tests/*.py && bash -n lib/*.sh bin/ai-specs tests/*.sh && gofmt -l catalog/recipes/worktree-flow/gate
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: card-74-clean-materialization
**Round**: 3 of 3 — maintainer-authorized strict-TDD documentation remediation re-verification; supersedes the round-2 report
**Version**: 0.22.0 (repo `VERSION`)
**Mode**: Strict TDD
**Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card-74-clean-materialization`
**Branch**: `change/card-74-clean-materialization` (HEAD `78a3c30` + uncommitted change files)

**evidence_revision definition**: SHA-256 over the three execution-evidence digests concatenated as lowercase hex with no separators, in order: focused-unittest output digest, then build/syntax output digest, then `validate.sh` output digest.

Round-1 and round-2 `verify-report.md` files are historical inputs only. Every runtime claim below was independently re-executed in this worktree on 2026-08-16. This round resolves the round-2 CRITICAL finding (missing formal `TDD Cycle Evidence` table in `apply-progress.md`): the table now exists and every row was validated against reality. Hybrid store: this report is persisted to OpenSpec and Engram only after native admission.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 25 |
| Tasks complete | 23 |
| Tasks incomplete | 2 (P5 Archive on review branch, P5 PR to development) |

| Phase | Status |
|-------|--------|
| P0 Planning gate | done |
| P1 RED isolated test | done |
| P2 GREEN (test-contract MCP skip) | done |
| P3 Release-flow pointer | done |
| P4 `./tests/validate.sh` | done (re-executed this round) |
| P5 Verify report | this file (round 3) |
| P5 Archive / PR | pending by maintainer-authorized rescope — WARNING, cleanup-class, non-blocking for this run |

The two unchecked tasks are the closure tasks the maintainer authorized leaving pending. They are recorded, not claimed complete, and do not block this authorized verification run.

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Isolated consumer project is the evidence surface | Isolated project is the inspected surface | `tests/test_release_materialization.py > test_isolated_init_sync_doctor_materializes_clean_consumer` | ✅ COMPLIANT |
| Lock version matches the candidate VERSION | Fresh sync stamps the candidate version | same test (`lock["meta"]["cli_version"] == candidate_version()`) | ✅ COMPLIANT |
| Doctor is ERROR-free after isolated sync | Healthy isolated project exits zero | same test (doctor returncode 0, no `^\s*ERROR  ` lines) | ✅ COMPLIANT |
| Generated adapters match enabled agents | Enabled agent outputs are present | same test (`platform_output_relpaths` — 11 concrete paths; `AGENTS.md` exists) | ✅ COMPLIANT |
| Catalog and cache reconcile without in-project leftovers | Bundled skills stay out of the local skill tree | same test (absence asserts for 5 CLI-bundled ids) | ✅ COMPLIANT |
| Gate release evidence matches VERSION and SHA256SUMS | SHA256SUMS tracks the candidate version | `test_sha256sums_declares_candidate_version_and_four_platforms` | ✅ COMPLIANT |
| Gate release evidence matches VERSION and SHA256SUMS | Isolated worktree-flow sync materializes the launcher | integration test (asserts `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` exists; `AI_SPECS_GATE_OFFLINE=1`, `AI_SPECS_GATE_BUILD` popped) | ✅ COMPLIANT |
| Drift is a product defect | Stale dogfood lock does not pass the gate | integration test (dogfood-lock byte snapshot unchanged before/after) | ✅ COMPLIANT |

**Compliance summary**: 7/7 requirements, 8/8 scenarios compliant. Every scenario has a covering test that passed at runtime this round.

### Build & Tests Execution (fresh evidence, 2026-08-16)

Cwd for every command: worktree root.

**Focused gate test**: ✅ 2 passed / 0 failed / 0 skipped
```text
python3 -m unittest tests.test_release_materialization -v
test_isolated_init_sync_doctor_materializes_clean_consumer (tests.test_release_materialization.ReleaseMaterializationTests.test_isolated_init_sync_doctor_materializes_clean_consumer) ... ok
test_sha256sums_declares_candidate_version_and_four_platforms (tests.test_release_materialization.ReleaseMaterializationTests.test_sha256sums_declares_candidate_version_and_four_platforms) ... ok
Ran 2 tests in 2.327s
OK
exit 0 | output sha256:dfa622a3eb5d62c707f03fe30e9d205c5d793695492263d5eaf550893abd6e9a
```

**Build / syntax / format**: ✅ exit 0, empty output
```text
python3 -m py_compile lib/_internal/*.py tests/*.py && bash -n lib/*.sh bin/ai-specs tests/*.sh && gofmt -l catalog/recipes/worktree-flow/gate
exit 0 | output sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

**Full validation** (configured final command): ✅ 1672 tests / 0 failed / 116 skipped
```text
./tests/validate.sh
validate.sh: go found — checking Go formatting (gofmt -l)
All vault-fs-mcp.sh checks passed.
run.sh: go found — running Go gate tests (go test ./catalog/recipes/worktree-flow/gate/...)
ok  	ai-specs.dev/worktree-gate	(cached)
Ran 1672 tests in 418.271s
OK (skipped=116)
exit 0 | output sha256:fee696d747b7b149a98155456f2f9e0f46e195f0de2e1023b31946460245cd1d
```

The 6 `ERROR` lines inside the full-suite output belong to intentional negative fixtures in other tests (doctor cli-version mismatch 99.99.99, readonly vendor dir, interactive init boom, pip failure path); the suite summary is `OK (skipped=116)`.

**Dogfood-lock isolation evidence** (not release evidence; isolation check only):
```text
before any run:  sha256=333ae645776f911ea693a4e80038de9376023caee2cb14876100a737cfd3daef  mtime=1786850961  cli_version=0.21.0
after focused:   sha256=333ae645776f911ea693a4e80038de9376023caee2cb14876100a737cfd3daef  mtime=1786850961
after build:     sha256=333ae645776f911ea693a4e80038de9376023caee2cb14876100a737cfd3daef  mtime=1786850961
after validate:  sha256=333ae645776f911ea693a4e80038de9376023caee2cb14876100a737cfd3daef  mtime=1786850961
git status after all runs: unchanged (M ai-specs/skills/release-flow/SKILL.md; ?? openspec/…, tests/test_release_materialization.py)
```
The candidate dogfood lock stayed byte-identical and untouched across every run.

**Coverage**: Python coverage tooling not configured (deferred per testing-foundation). Skipped, informational.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| Isolated consumer project is the evidence surface | ✅ Implemented | Test uses `tempfile.TemporaryDirectory`, `AI_SPECS_HOME=<candidate ROOT>`, CLI target is the temp workspace; dogfood lock snapshotted and byte-compared. |
| Lock version matches candidate VERSION | ✅ Implemented | `VERSION` reads `0.22.0`; lock assertion passed at runtime. |
| Doctor ERROR-free | ✅ Implemented | Doctor returncode 0 + no `ERROR  ` lines verified at runtime. |
| Generated adapters match enabled agents | ✅ Implemented | `platform_output_relpaths` yields 11 concrete paths for the 5 enabled agents and skips `mcp_config_path` when no `[mcp.*]` is declared (manifest has no MCP section). |
| No in-project bundled-skill leftovers | ✅ Implemented | Absence asserts for 5 bundled ids passed at runtime. |
| SHA256SUMS tracks candidate version | ✅ Implemented | Independent read: header declares `v0.22.0` + exactly four platform digest lines. |
| Isolated worktree-flow launcher materializes | ✅ Implemented | Hook path assert passed; offline gate env set. |
| Drift is a product defect | ✅ Implemented | No `lib/`, `bin/`, or `catalog/` edits in working tree; dogfood lock unchanged; isolated result is the signal. |

### Coherence (Design)

Skipped/degraded: no design artifact exists for this change (parent confirmed). Design coherence was not evaluated and is not claimed. Recorded, not invented.

### Proposal

Skipped/degraded: no proposal artifact exists for this change (parent confirmed). Proposal conformance was not evaluated and is not claimed.

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | Formal `TDD Cycle Evidence` table (RED/GREEN/TRIANGULATE/SAFETY NET/REFACTOR columns) now present in `apply-progress.md` — round-2 CRITICAL resolved |
| All tasks have tests | ✅ | 1/1 gate test file covers P1–P2 behavior; P3 doc pointer verified statically |
| RED confirmed (tests exist) | ✅ | `tests/test_release_materialization.py` exists and is new (untracked); historical RED (`FAILED (failures=1)`, `missing generated output: .mcp.json`) is consistent with the current test code and manifest (no `[mcp.*]` declared) |
| GREEN confirmed (tests pass) | ✅ | 2/2 pass on fresh execution (exit 0) |
| Triangulation adequate | ⚠️ | 2 test methods cover 8 scenarios — accurately reported as "Limited" in the table; not 8 test cases |
| Safety Net for modified files | ✅ | Test file is new (N/A valid); full 1672-test suite re-run (OK, skipped=116) |

**TDD Compliance**: 5/6 checks pass, 1 WARNING (no CRITICAL remains)

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 0 | 0 | — |
| Integration | 2 | 1 | python3 unittest + subprocess (real CLI) |
| E2E | 0 | 0 | not installed/needed |
| **Total** | **2** | **1** | |

### Changed File Coverage

Coverage analysis skipped — no Python coverage tool configured (informational, not blocking).

### Assertion Quality

✅ All assertions verify real behavior. No tautologies, no orphan empty checks (the empty `error_lines` assertion has companion value assertions in the same test), no type-only assertions, no ghost loops (the `platform_output_relpaths` loop demonstrably executes: it resolves 11 concrete output paths for the 5 enabled agents), no smoke-only tests, no mock/implementation-detail coupling (zero mocks).

### Quality Metrics

**Linter**: ➖ Not available (no Python linter configured)
**Type Checker**: ➖ Not available
**Syntax/format**: ✅ `py_compile` + `bash -n` + `gofmt -l` all exit 0 (see Build evidence)

### Issues Found

**CRITICAL**: None. The round-2 CRITICAL (missing `TDD Cycle Evidence` table) is resolved: the table exists and every row was validated against reality this round.

**WARNING**:
1. Two closure tasks pending by maintainer-authorized rescope: `P5 Archive on the review branch before merge` and `P5 PR to development` (cleanup-class; explicitly non-blocking for this run).
2. Limited triangulation granularity: 8 spec scenarios covered by 2 test methods (accurately self-reported as ⚠️ Limited in the evidence table).
3. Degraded artifact set: no proposal and no design artifact; those dimensions are skipped, not evaluated.

**SUGGESTION**:
1. Configure Python coverage tooling so changed-file coverage can be reported in future verifications.
2. Untracked base-style spec exists at `openspec/specs/release-materialization/spec.md` outside the change folder; the pending archive phase should own/reconcile it (verify its content matches the delta before syncing).

### Verdict

**PASS WITH WARNINGS**

Zero CRITICAL findings. All 7 requirements and 8 scenarios are COMPLIANT with fresh runtime evidence; both declared commands exited 0; the strict-TDD documentation remediation is validated against reality. Warnings are cleanup-class (pending archive/PR), granularity (2 test methods for 8 scenarios), and artifact degradation (no proposal/design). This verifier starts no remediation, review, refuter, correction, or scoped validation.

### Runtime notes

- Native runtime attempt is parent-bound (token `sha256:965341efd7fa70902c9f76bc2e6c8f5acf5b29dcc365522f7653ebf9f75a2cc2`). This verifier did not acquire, reset, settle, or transition any native attempt; the only native command invoked was `gentle-ai sdd-verify-validate` for report admission.
- `gentle-ai sdd-status` was not queried by this verifier; the authoritative status was provided by the parent (23/25, archive and PR pending under maintainer rescope).
