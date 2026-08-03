# Tasks: plan-build-delivery-contracts

Depth: **standard** (inherent) — full planning chain executed at explicit user request

Branch / worktree: `feat/plan-build-delivery-contracts` —
`/Users/robert/proyectos/nnodes/ai-specs-cli-plan-build-delivery-contracts`

Plan refs: `proposal.md`, `specs/plan-build-flow/spec.md`, `design.md`

Planning classification: **standard** by change signals (bounded, known-area multi-file change:
recipe config, brief rules, docs, focused tests; intent clear, files identifiable). The full
chain was executed because the user explicitly requested the complete SDD flow.

Implementation boundary: change only the recipe source, bundled skill/docs, and focused tests.
Do not modify external runtimes, add a recipe-specific renderer, hand-edit generated outputs, or
touch `plan-build-gate.sh`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~120–180 |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | not applicable |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: not applicable

The implementation is intentionally one reviewable delivery: recipe/config, skill/docs, and
focused regression coverage are tightly coupled. Rollback is a single revert of the recipe/docs/test
changes; no migration is required.

---

## P0 — Contract baseline and test setup

- [x] **P0.1 Confirm the locked surface before editing.** Re-read `design.md` and
      `specs/plan-build-flow/spec.md`; map each task below to the config, workflow-rule,
      documentation, and scope-exclusion requirements. Preserve the existing five workflow
      rules, hook pair, and artifact-presence gate.
      **Acceptance:** The implementation checklist names only `artifact_store_default`; it
      explicitly excludes `chained_pr_default`, execution-mode configuration, preflight code,
      and `gentle-ai`/`gentle-pi` changes.

- [x] **P0.2 Establish focused test commands.** Use existing unittest conventions for
      `test_plan_build_flow_recipe.py`, `test_agents_render_brief_fragments.py`,
      `test_recipes_catalog.py`, `test_runtime_brief_baseline.py`,
      `test_brief_render_policy.py`, and `test_plan_build_gate_hook.py` through `./tests/run.sh`.
      **Acceptance:** Iterative feedback uses this focused set, not a project-wide suite.

## P1 — RED: regression tests for the new contract

- [x] **P1.1 Replace the no-schema assertion with a complete recipe-surface test.** In
      `tests/test_plan_build_flow_recipe.py`, replace `test_recipe_adds_no_schema_surface` with
      a test asserting exactly one field, `artifact_store_default`, `required is False`,
      `type == "string"`, default `openspec`, exact enum `['openspec', 'engram', 'both']`,
      and the unchanged hook pair `[('on-sync', 'validate-config')]`.
      **Acceptance:** The test fails against the current config-less recipe and if any second
      field, required field, default, or enum member appears.

- [x] **P1.2 Narrow the vocabulary guard without deleting it.** Add the exact
      `[config.artifact_store_default]` TOML-table excision helper to the raw recipe scan and
      remove exactly one `## Delivery contracts` README section through the next `## ` heading
      or EOF. Keep `FORBIDDEN_TERMS` active elsewhere; positively pin the exact store enum.
      **Acceptance:** Only the locked store table/README section is exempt; forbidden terms
      elsewhere still fail.

- [x] **P1.3 Add recipe rule and config-path regression cases.** In
      `tests/test_plan_build_flow_recipe.py`, add tests for the string-form (`key is None`)
      store workflow rule and exact `{config.artifact_store_default}` placeholder, preserving
      the original five rules first; cover defaults, manifest overrides through `merge_config`,
      and enum rejection.
      **Acceptance:** RED cases cover default resolution to `openspec`, override resolution to
      values such as `both`, invalid enum rejection, unchanged hook behavior, and ordering.

- [x] **P1.4 Add renderer interpolation tests.** In
      `tests/test_agents_render_brief_fragments.py`, add one string-enum substitution case;
      leave existing missing-key, bare-key, escape, and manifest-prose tests intact.
      **Acceptance:** The test distinguishes the resolved store value from an absent placeholder
      and protects the generic renderer path rather than a recipe-specific branch.

- [x] **P1.5 Add negative-surface and skill-contract tests.** In focused recipe/gate tests, add
      assertions that the recipe schema and raw source contain no `chained_pr_default` or
      execution-mode key, and that the recipe surface contains no `review_budget` or
      `review budget` token. Keep the existing artifact-presence blocking tests unchanged.
      **Acceptance:** Tests fail if session controls or review-budget tokens enter the recipe
      surface, while the artifact-presence gate remains blocking.

- [x] **P1.6 Add materialization and catalog regression cases.** Extend
      `tests/test_plan_build_flow_recipe.py` with an end-to-end fixture that overrides
      `[recipes.plan-build-flow.config]`, materializes the recipe, renders the brief, and checks
      the resolved store value (not a placeholder) in `AGENTS.md`. Set
      `CONFIG_KEYS_IN_CATALOG` to `plan-build-flow: ['artifact_store_default']` only.
      **Acceptance:** Defaults and project overrides are observable in generated brief output,
      and catalog drift expects exactly one key.

- [x] **P1.7 Check exact-list baseline assumptions before changing them.** Inspect
      `tests/test_brief_render_policy.py` and `tests/test_runtime_brief_baseline.py` for exact
      plan-build rule/list assertions. Update only assertions that genuinely fail because of
      the one intentional additive rule.
      **Acceptance:** No baseline is weakened or rewritten speculatively.

- [x] **P1.8 Add behavioral evals for the store contract (hermetic + live, runtime-agnostic).
      Following the existing plan-build-flow eval pattern, add a hermetic scenario where a
      project with `artifact_store_default = "both"` syncs and `AGENTS.md` contains the resolved
      value. Add an opt-in live scenario asking what artifact store the project uses with the
      brief injected; assert the answer is `both`.
      **Acceptance:** Hermetic eval runs normally; live eval is opt-in via `EVALS_LIVE=1` and
      requires no external preflight package.

## P2 — GREEN: recipe, skill, and documentation implementation

- [x] **P2.1 Add the one optional config table and version bump.** Update
      `catalog/recipes/plan-build-flow/recipe.toml`: bump version `1.2.0` to `1.3.0`, add only
      `[config.artifact_store_default]` with `required = false`, `type = 'string'`, default
      `openspec`, exact enum `['openspec', 'engram', 'both']`, and non-empty help text. Keep the
      existing on-sync validation hook unchanged.
      **Acceptance:** Existing schema/merge/hook paths resolve defaults, valid overrides, enum
      rejection, and hook behavior without new materializer code.

- [x] **P2.2 Append the delivery workflow rule using the existing format.** In `recipe.toml`
      append exactly one string entry to `provides.brief.workflow_rules` (not an inline table,
      so `key = None`) interpolating `{config.artifact_store_default}`. State that the value is
      the project delivery default to provide when a session asks where artifacts live; do not
      add session control flow or external runtime dependencies.
      **Acceptance:** The original five rules and order are unchanged and the new entry is last.

- [x] **P2.3 Document the user-facing contract.** Update the recipe README with a `## Delivery
      contracts` section documenting `artifact_store_default` only. Update
      `docs/recipes-catalog.md` so the plan-build row lists only that key, the entry replaces
      `Config: none`, and version examples use `1.3.0`.
      **Acceptance:** README and catalog describe the same locked store contract and keep the
      vocabulary exemption limited to the intended README section.

## P3 — TRIANGULATE: integration and scope regression coverage

- [x] **P3.1 Exercise generic interpolation end to end.** Run the materialization fixture with
      defaults and manifest overrides through the existing materialize/brief-render path. Assert
      generated `AGENTS.md` contains `openspec` or the override, has no unresolved store
      placeholder, and keeps unrelated workflow rules in order.
      **Acceptance:** The generic config namespace handles the string value; no recipe-specific
      renderer or sync branch is required.

- [x] **P3.2 Triangulate validation and failure behavior.** Run enum invalid-value cases through
      existing merge and `validate-config` paths. Confirm absent config remains backwards
      compatible and invalid values do not materialize into the brief.
      **Acceptance:** Invalid enum values fail with the field identified; accepted enum values
      pass.

- [x] **P3.3 Triangulate gate preservation and excluded session controls.** Run the focused gate
      and negative-surface tests together; verify the existing missing-active-tasks artifact
      condition still blocks production edits and that no `chained_pr_default`, execution-mode,
      preflight, or `gentle-ai` reference enters the changed recipe/docs/tests.
      **Acceptance:** Only the repository store contract is declared and the gate behavior is
      unchanged.
- [x] **P3.4 Triangulate removed contract tokens.** Search the changed recipe, skill, README,
      catalog entry, and focused tests for `review_budget_lines`, `review budget`, the removed
      budget regex, `400 changed lines`, and the removed warning-section marker. Any explanation
      of the removal must remain framed as the decision to leave review-budget handling to the
      external session preflight, not as a recipe feature.
      **Acceptance:** No removed contract token remains in this change's feature surface.

## P4 — REFACTOR: consistency and review cleanup

- [x] **P4.1 Refactor only after green coverage.** Reuse existing helpers where appropriate, keep
      table/section excisions narrow, and preserve exact enum, hook, rule-order, and gate-token
      assertions. **Acceptance:** Tests remain deterministic and behavior-focused; no shim,
      alias, broad allowlist, TODO, or generated-file workaround remains.

- [x] **P4.2 Reconcile version/docs surfaces.** Search recipe README, catalog, focused tests, and
      source for stale `1.2.0`, `Config: None`, or `Config: none`; leave generated outputs to
      sync. **Acceptance:** Source/docs consistently advertise `1.3.0` and the store field.

## P5 — Verification and delivery boundary

- [x] **P5.1 Run focused unit verification first.** Execute the focused recipe, renderer, catalog,
      baseline/policy, and gate tests listed in P0.2; capture RED/GREEN/TRIANGULATE evidence.
      **Acceptance:** All focused tests pass before full validation is attempted.

- [x] **P5.2 Run repository validation second.** Execute `./tests/validate.sh` after focused tests
      pass; this is the required final validation for Python compilation, shell syntax, and the
      configured unittest layer. **Acceptance:** Validation passes, or environmental failure is
      recorded precisely.

- [x] **P5.3 Final review and PR boundary.** Check worktree status, verify no commit, and review
      every acceptance item. **Acceptance:** Deliver the implementation as one PR after
      apply/verify, without archive/publish.

---

## Follow-up (out of scope, card #59 LOb6pZLj)

Adversarial depth classifier: detect conflict between an explicit user depth request and the
classifier signal; ask the user to decide instead of silently picking one; and record the
disagreement. Tracker: https://trello.com/c/LOb6pZLj

## Follow-up (out of scope, card #60 lxv2WQ5g)

Adjust minimum artifacts per depth tier and add a staged verify gate. Tracker:
https://trello.com/c/lxv2WQ5g
