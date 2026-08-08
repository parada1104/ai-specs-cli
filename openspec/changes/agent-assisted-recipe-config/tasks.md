# Tasks: agent-assisted-recipe-config

Depth: full

Branch / worktree: `change/agent-assisted-recipe-config` /
`.worktrees/agent-assisted-recipe-config/`

Plan refs: `explore.md`, `proposal.md`, `design.md`,
`specs/agent-assisted-recipe-config/spec.md`

**Stop for human authorization before production-code apply.** This file is the
implementation plan only — do not write production code or tests while
authoring it.

---

## Tracker

- **card_id**: `6a72b44828a5b2547f679116`
- **shortLink**: `GjfV4sKA`
- **url**: https://trello.com/c/GjfV4sKA

## Locked delivery decisions (human)

| Decision | Value |
|---|---|
| Delivery mode | **Minimum non-interactive helper** (`ai-specs recipe configure`) + skill playbook — not skill-only |
| MVP scope | **Broader**: 3 evidence recipes, 5 eval scenarios |
| Runtime evidence | **New client on the existing, unchanged `tests/evals/` system**; any supported agent CLI runtime, none mandated; no browser, no new platform |
| Orchestration | **Optional, additive** — an Orca/OMP orchestration skill may fan the *existing* runners across runtimes and aggregate; it is not a runtime, not a runner, and never required |
| Interactive wizard | Behavior preserved |
| #63 governance | Out of scope — preserve and report only |

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~900–1400 (helper + write-path fix + unit tests + evals + docs) |
| 400-line budget risk | **High** — broader MVP is a deliberate, authorized expansion |
| Chained PRs recommended | **Yes** |
| Suggested split | PR1 write-path fix + regression · PR2 helper + contract tests + skill/docs · PR3 eval client + scenarios |
| Delivery strategy | Land the shared-writer fix first so PR2 builds on corrected semantics |
| Chain strategy | feature-branch-chain to `development` |

```text
Decision needed before apply: Yes (authorization gate)
Chained PRs recommended: Yes (3)
Chain strategy: feature-branch-chain
400-line budget risk: High
```

### Suggested Work Units

| Unit | Goal | PR | Notes |
|------|------|----|-------|
| 1 | Fix `update_recipe_config`: inline-comment carry + byte-identical no-op | PR1 | Shared with the wizard; smallest reviewable slice |
| 2 | `recipe-configure` helper: inspect / apply / preflight / sync / report | PR2 | Deterministic contract + exit-code table |
| 3 | Skill playbook + literacy cross-links + docs | PR2 | NL entry, approval gate, report fields |
| 4 | Eval **client** on the existing system: additive fixture helper, module, runner, 5 scenarios, README | PR3 | Runtime evidence tier; no change to canonical eval semantics |

---

## Planning depth

- **Classification**: `full` (explore → proposal → design → spec → tasks).
- **Why full**: new agent capability; cross-cutting literacy/config/sync;
  corrects two defects in a shared write path; adds a runtime evidence tier.
- **Delta coverage**: ADDED requirements in
  `specs/agent-assisted-recipe-config/spec.md` — NL entry, deterministic
  helper, grounded recommendation, topology grounding without `init.md`,
  idempotent apply, comment preservation, preserve overrides, CLI-version
  preflight gate, sync + verify with partial semantics, structured report,
  real-runtime evals, optional cross-runtime eval orchestration,
  docs/validation.
- **Authorization**: PENDING until maintainer green-lights apply.

## Non-goals (apply MUST NOT)

- Implement override lock provenance / force-update (#63).
- Add per-override hashes to `.ai-specs.lock` or per-artifact governance
  categories.
- Change interactive `configure-recipes` behavior.
- Add a JSON mode to `ai-specs doctor`.
- Author `init.md` for recipes that lack one.
- MCP-wrap the CLI.
- Hardcode Melón/Alquimia paths or repo names.
- Weaken read-only `ai-specs recipe init` into a mutating command.
- Invent per-project CLI shims under `ai-specs/bin/`.
- Introduce a new eval framework, runner service, or browser/UI automation.
- Change canonical eval semantics: scenario contract, fixtures, assertions,
  isolation, trial rules, or pass criteria.
- Mandate a specific runtime, or make orchestration a prerequisite for running
  or passing the evals.

---

## Implementation (red-green-refactor) — after authorization

### Phase 0 — Confirm slice and worktree

- [x] 0.1 Confirm the locked decisions above with the authorizer; record in the
      PR body (and Engram if available).
- [x] 0.2 Re-verify worktree: `git rev-parse --show-toplevel`, branch
      `change/agent-assisted-recipe-config`, no writes on protected
      `development`.
- [x] 0.3 Confirm `git status --short` is clean before any live CLI run
      (`dogfood-verification-isolation`).

### Phase 1 — RED: shared write-path defects

**Reqs:** Comment preservation on config write; Idempotent canonical config apply.

- [x] 1.1 RED `tests/test_recipe_config_write.py`: replacing a key preserves its
      **trailing inline comment** (today's `test_replace_existing_key` only
      asserts the own-line comment — extend, do not weaken it).
- [x] 1.2 RED: a `#` inside a basic or literal string value is not treated as a
      comment when a sibling key is replaced.
- [x] 1.3 RED: apply of values already equal to the effective config leaves the
      file **byte-identical**, including differing formatting
      (`k='v'` vs `k = "v"`); assert on bytes, not on parsed TOML.
- [x] 1.4 RED: multi-line existing value ⇒ rejection, no partial rewrite.
- [x] 1.5 RED `tests/test_config_wizard.py`: wizard-driven writes still behave
      as today and now also preserve inline comments.

### Phase 2 — GREEN: `update_recipe_config`

- [x] 2.1 TOML-string-aware value/comment splitter; carry the trailing comment
      verbatim on replacement.
- [x] 2.2 Value-equality skip against parsed current values; untouched keys keep
      original bytes.
- [x] 2.3 Short-circuit `write_text` when assembled text equals the original.
- [x] 2.4 Reject multi-line values with a distinguishable error.
- [x] 2.5 Phase 1 tests green; existing write/wizard tests still green.

### Phase 3 — RED: helper contract

**Reqs:** Deterministic non-interactive configure helper; Preflight gate;
Sync and verify; Structured closing report; Grounded recommendation; Topology
grounding without a recipe init contract.

- [x] 3.1 RED `tests/test_recipe_configure.py`: `--inspect --json` twice ⇒
      byte-identical; no timestamps/absolute paths/PIDs in output.
- [x] 3.2 RED: inspect document carries schema fields (type/enum/default/help),
      current config, and `unknown_keys` without deleting them.
- [x] 3.3 RED: topology grounding for a recipe with **no `init.md`**
      (`worktree-flow`) from schema + `resolve_repo_topology`; submodule fixture
      resolves `monorepo-submodules` with evidence.
- [x] 3.4 RED: no `.gitmodules` ⇒ `monorepo-apps` surfaced as an open question,
      not asserted as `standalone` intent.
- [x] 3.5 RED: exit-code table — 0 ok/no-op, 1 failed/partial, 2 usage,
      3 rejected, 4 blocked; codes 3 and 4 write nothing.
- [x] 3.6 RED: `[tool]` pin violation (and malformed policy) ⇒ exit 4,
      `status: "blocked"`, manifest byte-identical, sync never invoked.
      Assert the **ordering**: gate runs before the write.
- [x] 3.7 RED: `--ignore-cli-version` records the bypass and forwards it.
- [x] 3.8 RED: sync failure after a successful write ⇒ `status: "partial"`,
      exit 1, `failed_step` set, `rolled_back: false`, `lock_stamped: false`.
- [x] 3.9 RED: report schema completeness (`report_version`, status, applied
      changed/unchanged/**preserved**, preflight, sync, verify, assumptions,
      drift, gaps).
- [x] 3.10 RED: unparsable doctor summary ⇒ `verify.parsed: false` with null
      counts; never zeros.
- [x] 3.11 RED: lock `cli_version` staleness ⇒ informational gap, flow proceeds.
- [x] 3.12 RED: secret-shaped literal rejected (exit 3) and never echoed;
      `${env:VAR}` passes through.

### Phase 4 — GREEN: helper + CLI wiring

- [x] 4.1 Implement `lib/_internal/recipe-configure.py` (inspect / apply /
      preflight / optional sync / report) reusing `recipe_schema`,
      `resolve_repo_topology`, `update_recipe_config`, `cli_version`.
- [x] 4.2 Register `configure` in `lib/recipe.sh` dispatch + usage; leave
      `configure-recipes` untouched.
- [x] 4.3 Doctor consumption: run, capture exit, parse the final `Summary:`
      line, degrade to `parsed: false`.
- [x] 4.4 Sync invocation behind `--sync`, forwarding `--ignore-cli-version`;
      map exit and failing step into the report.
- [x] 4.5 Phase 3 tests green.

### Phase 5 — Skill playbook + docs

**Reqs:** Natural-language entry; Documentation and validation coverage.

- [x] 5.1 `bundled-skills/harness-recipes/SKILL.md`: inspect → recommend →
      apply → sync/verify → report, with the approval gate, preserve rules,
      no-secret-literals, and the helper's exit-code branches.
- [x] 5.2 `harness-lifecycle` cross-link: assisted path alongside the
      interactive wizard (wizard not deprecated).
- [x] 5.3 Docs pointer (`docs/recipes-catalog.md` and/or
      `docs/ai/troubleshooting.md`) covering the flow and the two evidence tiers.
- [x] 5.4 Skill-content checks: named commands exist; playbook mentions sync,
      preserve, approval, report fields.

### Phase 6 — Eval client on the existing system

**Reqs:** Real-runtime evaluation of the assisted flow; Optional cross-runtime
eval orchestration.

Adds a client only. Scenario contract, fixture model, assertions, isolation,
and runner shape stay exactly as they are.

- [x] 6.1 `tests/evals/lib/project_fixture.py`: add
      `setup_bundled_skills(root, runtime, names)` copying
      `bundled-skills/<name>/SKILL.md` into `RUNTIME_SKILL_DIRS[runtime]`
      (existing `setup_runtime_skills` resolves catalog recipe skills only).
      Additive — existing callers unchanged.
- [x] 6.2 `tests/evals/eval_assisted_configure_live.py` following
      `eval_worktree_flow_live.py` shape (tempdir fixture, `init_git_repo`,
      baseline commit, `EVALS_LIVE` gate, N-of-M trials).
- [x] 6.3 Scenarios under `tests/evals/scenarios/assisted-configure/`:
      `ac_recommend_stops_before_apply`, `ac_topology_grounded_without_initmd`,
      `ac_apply_sync_verify_report`, `ac_noop_reapply_preserves_bytes`,
      `ac_blocked_cli_version_pin`. Prompts must be natural user sentences
      (`assert_natural_prompt` rejects meta-prompts).
- [x] 6.4 Byte-hash assertion helper for the two "must not touch" scenarios.
- [x] 6.5 `tests/evals/run-live-assisted-configure.sh` mirroring
      `run-live-worktree.sh`, keeping the existing `EVALS_PREFER` ordering and
      `EVALS_*` contract. No runtime is privileged or hardcoded.
- [x] 6.6 `tests/evals/README.md`: new client section, scenario table, and the
      unit-vs-eval distinction.
- [x] 6.7 Confirm `./tests/run.sh` does **not** collect `eval_*.py`.
- [x] 6.8 Confirm no existing scenario, fixture, assertion, or runner changed:
      `git diff` over `tests/evals/` shows only the new client plus the
      additive `setup_bundled_skills`.
- [x] 6.9 Document the optional orchestration usage (fan the existing runner
      across `EVALS_RUNTIMES`, aggregate per runtime) in
      `tests/evals/README.md`, stating plainly that it changes no eval
      semantics and that a plain shell run yields identical verdicts.

### Phase 7 — Validation and evidence

**Reqs:** Documentation and validation coverage; Real-runtime evaluation.

- [x] 7.1 Focused tests green.
- [x] 7.2 `./tests/run.sh` and `./tests/validate.sh` green before commit/PR.
- [ ] 7.3 Live eval run across **at least two runtimes** so a runtime-specific
      approval-gate failure is visible; selection via `EVALS_RUNTIMES`.
      Claude/Opus `ac_apply_sync_verify_report` passed and is recorded in
      `verify-report.md`; no second-runtime PASS is claimed because other
      attempts hung/cancelled. The bounded `EVALS_RUNTIMES=none` run was a
      five-test SKIP, not live evidence.
- [x] 7.4 Post-run isolation check: source-worktree status before/after was
      identical (15 pre-existing modified tracked files and 7 pre-existing
      untracked files); `git diff -- AGENTS.md` was empty and fixture/temp
      state was cleaned.
- [x] 7.5 Transcribe eval evidence into `verify-report.md` in this change
      folder, attributed **per runtime** (including exact
      `helper_report_present: false` and the open second-runtime boundary).
- [x] 7.6 Promote the delta into canonical `openspec/specs/` per project norms.

---

## Acceptance traceability

| Card acceptance | Spec requirement | Tasks |
|---|---|---|
| NL request | Natural-language entry to assisted configure | 5.1, 5.2, 6.3 |
| Grounded recommendation | Grounded recommendation before apply; Topology grounding without a recipe init contract | 3.2–3.4, 4.1, 6.3 |
| Idempotent canonical update | Idempotent canonical config apply; Deterministic helper | 1.3, 2.2, 2.3, 3.1, 3.5 |
| Preserve config/overrides | Comment preservation on config write; Preserve overrides | 1.1, 1.2, 1.4, 1.5, 2.1, 2.4 |
| Run/verify sync | Preflight gate on CLI version policy; Sync and verify after apply | 3.6–3.8, 4.4 |
| Report assumptions/drift/gaps | Structured closing report | 3.9–3.11, 4.3 |
| Documented + validation | Documentation and validation coverage; Real-runtime evaluation; Optional cross-runtime eval orchestration | 5.*, 6.*, 7.* |
