# Verification Report — rules-migration-audit

**Change**: rules-migration-audit (Trello #14, `/rules-audit`)
**Version**: spec v1 (#734)
**Mode**: Strict TDD
**Verdict**: **PASS WITH WARNINGS**

---

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 26 |
| Tasks complete | 26 (all implemented in tree) |
| Tasks incomplete | 0 |

All 5 phases delivered. tasks.md checkboxes are left unchecked `[ ]` in the file (cosmetic — apply agent did not tick them), but every task is realized in the working tree and proven by tests.

---

## Build & Tests Execution (real)

**`./tests/run.sh`**: ✅ PASS — `Ran 414 tests in 79.343s` / `OK` / exit 0
**`./tests/validate.sh`**: ✅ PASS — py_compile + bash -n stages clean, `Ran 414 tests` / `OK` / exit 0

Targeted module `python3 -m unittest tests.test_rules_audit -v`: **9/9 OK** (0.139s):
- test_placeholder, test_read_only_invariant, test_json_shape, test_mode_a_detection,
  test_mode_b_detection, test_missing_sources_absent, test_keyword_heuristic,
  test_cli_help_lists_rules_audit, test_cli_missing_path_exits_nonzero

**Coverage**: ➖ Not available (project has no coverage tool configured).

---

## TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ⚠️ | No apply-progress artifact in engram; verified directly against tree |
| All tasks have tests | ✅ | Core behaviors all covered by test_rules_audit.py |
| RED confirmed (tests exist) | ✅ | 9 tests present and discoverable |
| GREEN confirmed (tests pass) | ✅ | 9/9 pass on execution |
| Triangulation adequate | ✅ | Mode A, Mode B, absent-source, keyword heuristic all distinct cases |
| Safety Net for modified files | ✅ | Full 414-test suite passes (no regression in modified bin/ai-specs etc.) |

## Assertion Quality (Step 5f)
✅ All assertions verify real behavior. `test_read_only_invariant` snapshots `(st_mtime_ns, st_size)` for every file via `rglob` before and after `scan()` and asserts dict equality — this catches creation, modification, AND deletion. No tautologies, no orphan-empty checks, no smoke-only tests, no ghost loops. The CLI tests assert exit codes and stderr/stdout content (real behavior).

## Test Layer Distribution
| Layer | Tests | Files |
|-------|-------|-------|
| Unit (in-process scan) | 6 | test_rules_audit.py |
| Integration (subprocess CLI) | 3 | test_rules_audit.py |
| **Total** | **9** | **1** |

---

## Read-Only Invariant (load-bearing) — VERIFIED

- **Test**: `test_read_only_invariant` genuinely asserts zero filesystem mutation (rglob + mtime/size snapshot diff). ✅
- **Static scan of `rules-inventory.py`**: only one `open(...)` call — `toml_path.open("rb")` (read binary, line 352). No `write`, `write_text`, `write_bytes`, `mkdir`, `makedirs`, `unlink`, `remove`, `rmtree`, `shutil`, `touch`, or `json.dump(file)`. `json.dumps()` → stdout only. ✅
- **Live run**: `ai-specs rules-audit .` against the repo left `git status` unchanged (no new/modified files); no `ai-specs/plans/` created. ✅

---

## Spec Compliance Matrix

| Requirement | Scenario | Evidence | Result |
|-------------|----------|----------|--------|
| CLI command availability | Help lists rules-audit | `test_cli_help_lists_rules_audit` PASS; live `ai-specs help` shows line | ✅ COMPLIANT |
| CLI command availability | Accepts optional path arg | `rules-audit.sh` L29-47 parses path; live Mode A run on tmp dir | ✅ COMPLIANT |
| CLI command availability | Exits non-zero on missing path | `test_cli_missing_path_exits_nonzero` PASS; live exit=2 + stderr | ✅ COMPLIANT |
| Inventory scope | Full-scope scan emits JSON | `test_json_shape` PASS; live JSON has all source keys | ✅ COMPLIANT |
| Inventory scope | Missing sources represented | `test_missing_sources_absent` PASS (`status: absent`) | ✅ COMPLIANT |
| Read-only invariant | No file written after scan | `test_read_only_invariant` PASS + static scan + live git-clean | ✅ COMPLIANT |
| Read-only invariant | Failure exits non-destructively | `main()` try/except returns 1, prints stderr only (L404-411); no test exercises the exception path | ⚠️ PARTIAL |
| Mode detection | Mode A — legacy detected | `test_mode_a_detection` PASS; live tmp fixture mode=A | ✅ COMPLIANT |
| Mode detection | Mode B — greenfield | `test_mode_b_detection` PASS (recommendations + stack_hints) | ⚠️ PARTIAL (see W1) |
| Classification taxonomy | Each item exactly one bucket | live Mode A: `enable_recipe` / `create_local_skill`, both in 7-set | ✅ COMPLIANT |
| Classification taxonomy | Marked as suggestions | `classification_is_suggestion: true` top-level (L118); `test_json_shape` asserts key | ✅ COMPLIANT |
| Plan deliverable | Agent writes dated plan | `bundled-commands/rules-audit.md` Step 3 writes `ai-specs/plans/rules-migration-<YYYY-MM-DD>.md` | ✅ COMPLIANT (doc) |
| Plan deliverable | Plan not written by helper | read-only invariant (above) + live no-plan-created | ✅ COMPLIANT |
| Bundled command distribution | Appears in harness commands_dir | run.sh evidence: `✓ copy commands/rules-audit.md` + `= commands/rules-audit.md tracked` on init/refresh-bundled test workspaces | ✅ COMPLIANT |
| Bundled command distribution | No pipeline changes required | No edits to refresh-bundled.py/sync-agent.sh in diff; distribution worked | ✅ COMPLIANT |
| skills-as-rules.md correction | Stale claim removed | `grep "auto-invoke table"` → NONE; diff removed all 4 references | ✅ COMPLIANT |
| skills-as-rules.md correction | Link to rules-audit added | L14-15 "use `/rules-audit` first to produce an advisory plan" | ✅ COMPLIANT |

**Compliance summary**: 15/17 COMPLIANT, 2/17 PARTIAL, 0 FAILING, 0 UNTESTED.

---

## Decisions D1–D5

| Decision | Followed? | Notes |
|----------|-----------|-------|
| D1 Mode A + lightweight Mode B | ✅ | Mode A inventory; Mode B emits recommendations{init, default_recipes, brief_hint} + stack_hints |
| D2 7-bucket taxonomy, suggestions | ✅ | CLASSIFICATION_BUCKETS exactly 7; `classification_is_suggestion: true` |
| D3 .cursorrules in scope | ✅ | `_scan_cursorrules` included; absent→`{status: absent}` |
| D4 skills-as-rules.md fix + link | ✅ | Stale claims gone, /rules-audit linked |
| D5 Read-only guarantee | ✅ | Verified 3 ways (test, static, live) |

---

## Issues Found

**CRITICAL** (must fix before archive): None.

**WARNING** (should fix):
- **W1 — Mode B definition deviates from spec.** Spec ("Mode detection" requirement) defines Mode B as "Neither [.mdc nor .cursorrules] present AND AGENTS.md **absent** (greenfield)." `_detect_mode` (rules-inventory.py L169-176) ignores AGENTS.md entirely: a project with AGENTS.md present but no cursor rules is classified Mode B. Confirmed live: running against this repo (which HAS AGENTS.md, 8 sections) returns `"mode": "B"` with greenfield recommendations ("ai-specs init") — misleading for an already-initialized project. Spec scenario "Mode B — greenfield" requires no AGENTS.md. Either tighten the detector (B only when AGENTS.md absent) or relax the spec wording. The unit test `test_mode_b_detection` uses a fixture with no AGENTS.md, so it doesn't catch the gap.
- **W2 — "Failure exits non-destructively" scenario has no test.** The error path (L404-411) is correct by inspection (exception → stderr + return 1, no file write), but no test forces an exception during scan to prove the THEN. Marked PARTIAL.
- **W3 — tasks.md checkboxes not ticked.** All 26 tasks are done in the tree but remain `[ ]` in tasks.md. Cosmetic; will mislead an archive reader. Recommend ticking before archive.

**SUGGESTION** (nice to have):
- S1 — `agents_md_present: True` is exposed in `sources` but does not influence mode; surfacing an "already-initialized" hint in Mode B output (or a distinct mode) would make the greenfield recommendation less confusing for projects like this repo.
- S2 — No test asserts the bundled fan-out distribution directly for `rules-audit.md` (relied on incidental init/refresh-bundled test-workspace output). A dedicated assertion would harden spec scenario "Command appears in harness commands directories."
- S3 — `bundled-commands/rules-audit.md` Step 2 says "Python may emit a `classification` field" — the helper always emits it (default `create_local_skill`). Minor doc/impl wording mismatch.

---

## Verdict: PASS WITH WARNINGS

Implementation is complete and behaviorally sound: 414/414 tests pass, validate.sh green, the load-bearing read-only invariant is verified three independent ways, CLI is wired parallel to doctor and emits valid schema-conformant JSON, all stale skills-as-rules.md claims are removed, README updated, and D1–D5 honored. The one substantive issue (W1) is a Mode B detection semantics deviation from the spec — not a crash or data-loss bug, but it produces a misleading "greenfield" result for already-initialized projects. W2/W3 are test-coverage and bookkeeping gaps. None are blockers; recommend addressing W1 + W3 before archive.
