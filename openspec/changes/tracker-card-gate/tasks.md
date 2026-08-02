# Tasks: tracker-card-gate

Depth: **full**

Branch / worktree: `feat/tracker-card-gate` —
`/Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate`

Plan refs: `explore.md`, `proposal.md`, `design.md`,
`specs/{trello-card-linking,trello-state-sync,trello-progress-comment,session-bootstrap,project-doctor,tracker-card-gate}/spec.md`

**Gate script location (canonical):**
`catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`
(dual hook ids `tracker-card-gate` + `tracker-card-gate-shell` share that one
script; materialize stamps `__TRACKER_CARD_GATE_MODE__` + `__TRACKER_CLI_HOME__`
into the gitignored project copy under `ai-specs/recipes/…`).

**Stop for human authorization before any further production code implementation.**

This file is the implementation plan only — do not write production code, tests,
or eval runners while authoring/editing it. Await maintainer go-ahead before
RED/GREEN apply.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~900–1400 (incl. hermetic + live eval scaffolding) |
| 400-line budget risk | High (tests + evals dominate) |
| Chained PRs recommended | Yes if impl+tests exceed ~400 reviewable LOC |
| Suggested split | PR1 parser+gate+materialize+doctor → PR2 recipe/skill/docs/dogfood → PR3 live evals |
| Delivery strategy | ask-on-risk / auto-chain if over budget |
| Chain strategy | feature-branch-chain (tracker → `development`) |

```text
Decision needed before apply: Yes (authorization gate)
Chained PRs recommended: Conditional (size)
Chain strategy: feature-branch-chain when over budget
400-line budget risk: High
```

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | `trello_link.py` + `tracker-card-gate.sh` + materialize stamps + dual hooks + doctor WARN + hermetic tests | PR 1 | Core contract; largest test surface |
| 2 | Skill/brief/command/session-bootstrap/docs + dogfood `gate_mode=warn` + CHANGELOG | PR 2 | Prose + config; depends on Unit 1 ids/paths |
| 3 | Live golden client (`run-live-trello.sh` + scenarios) + final validate/verify/archive | PR 3 | Manual/nightly; not CI-gating |

---

## Planning depth

- **Classification**: `domain_change` → full chain (explore → proposal → design →
  delta specs → tasks). Matches proposal §Planning depth.
- **Delta coverage**: 6 change-scoped specs — every phase below cites the
  requirement / scenario(s) it closes (see Requirement → phase map).
- **Accepted baselines** (from design.md Decisions 1–10):
  - Canonical link contract = `## Tracker` section in the change's
    `proposal.md` (fallback `tasks.md`); validity = non-empty `card_id`
    (shared predicate). Contract declared in `openspec/config.yaml`
    `tracking:` (see Phase 8).
  - On-disk exemption marker = `openspec/changes/<slug>/tracker.none`
    (conceptual name `tracker:none` — see Flagged issues #1).
  - Gate semantic model = **plan-build-gate** (artifact before production), with
    worktree-gate dual-hook + fail-open lessons.
  - Activation = recipe enabled ∧ bootstrap marker present ∧ `gate_mode ≠ off`.
  - Modes = `off` \| `warn` \| `always`; dogfood default **`warn`**.
  - Default production dirs = `lib catalog bin src` (`ai-specs/` excluded).
  - Shell coverage = high-confidence `gh pr create` + change-archive only.
  - Doctor = `Severity.WARN` only (no FAIL in v1).
  - No MCP interception; no auto-create from the gate script.
- **Authorization**: PENDING until maintainer green-lights apply.

## Non-goals (apply MUST NOT)

- Intercept Trello MCP tools at the harness layer.
- Introduce `.openspec.yaml` / folder-schema `trello_card_id`.
- Migrate or fail the 68 archives lacking a card link (grandfather).
- Make doctor FAIL-by-default or add a project pre-commit hard-fail in v1.
- Implement deferred sync hooks (`link-trello-card` / `sync-card-state` /
  `comment-verification`) as real sync-time MCP callers.
- Build an abstract multi-tracker product (Jira/Linear/…) — keep swappable seams
  only.
- Auto-create cards inside `tracker-card-gate.sh`.
- Close platform hook gaps (Cursor no pre-file-write; OpenCode subagent/MCP;
  pi/omp child processes) — document + brief + evals only.
- Promote this repo's dogfood `gate_mode` to `always` in this change.
- Hand-edit generated harness shims under `ai-specs/recipes/**` (sync-only).
- Edit `proposal.md` / `design.md` / `specs/` during apply unless a blocking
  contradiction is found (then stop and ask) — except the on-disk exemption
  filename reconciliation in Flagged #1, which apply MUST align without
  expanding scope.

---

## Apply conventions (locked for implementers)

1. **On-disk exemption filename = `tracker.none`.** Conceptual name remains
   `tracker:none` in prose/brief. Align design Decision 4f/6 path literals to
   `tracker.none` when touching those call sites (specs already lock this).
2. **Strict TDD** for gate, doctor, materialize, and parser. Docs/skill prose
   may be GREEN-only with doc-content assertions where cheap.
3. **Hermetic tests before live evals.** Live goldens are first-class but
   manual/nightly — `./tests/validate.sh` must pass without `EVALS_LIVE=1`.
4. **Fail-open absolute** for the gate: parse/lookup/`git`/`python3`/ambiguous
   shell → exit 0.
5. **Dogfood isolation**: keep `ai-specs/recipes/**` and regenerated
   `AGENTS.md` out of commits unless intentionally documenting a catalog
   change (see `dogfood-verification-isolation`).
6. **This change already has Trello #56** (`card_id`
   `6a6ebd5e2cd9a2fcd419e62c`, shortLink `WHZ3fLzD`). Write the `## Tracker`
   section in this change's `proposal.md` in the close phase (Phase 12)
   before archive / PR; do not invent a second card.

---

## P0 — Planning gate (this session)

- [x] P0.1 `explore.md`
- [x] P0.2 `proposal.md` (Locked decisions 1–8 + Decision #7 overturn)
- [x] P0.3 `design.md` (Decisions 1–10 + apply order)
- [x] P0.4 Delta specs under `specs/` (6 capabilities)
- [x] P0.5 `tasks.md` (this file)
- [ ] P0.6 **Human authorization to continue implementation**

---

## Implementation (red-green-refactor)

Phases follow design.md **Migration / Rollback** apply order and close every
delta requirement. Each task is one focused TDD cycle unless marked docs-only,
config-only, or verification.

### Phase 1 — Canonical parser `lib/_internal/trello_link.py` (Decision 1)

**Files:** `lib/_internal/trello_link.py` (new);
`tests/test_trello_link.py` (new; may later merge assertions into
`test_doctor_tracker_card.py` / `test_tracker_card_gate_hook.py` via
`parser_parity`, but a focused module test is preferred).
**Reqs:** `trello-card-linking` — *Canonical `## Tracker` link section*;
shared validity predicate used by doctor + gate.

- [ ] 1.1 RED: `parse_trello_md` / `is_valid_link` / `card_id_looks_canonical`
      matrix for fixtures:
      - bold-key list form (`- **card_id**: \`…\``) → keys lowercased
      - plain `key: value`
      - backticked values + trailing ` #comment` stripped
      - duplicate keys → first wins
      - headings / unknown keys / blank lines ignored
      - missing file / unreadable → `{}` / invalid
      - empty `card_id` → invalid; non-empty → valid even if `url` absent
      - `card_id_looks_canonical` true only for `^[0-9a-fA-F]{24}$`
      No production module yet.
- [ ] 1.2 GREEN: implement `lib/_internal/trello_link.py` exactly per design
      Decision 1 regex + cleaning rules. Pass 1.1 via
      `./tests/run.sh tests/test_trello_link.py`.

### Phase 2 — Gate hermetic harness + path-mode RED (Decisions 3, 4 path, 9a)

**Files:** `tests/test_tracker_card_gate_hook.py` (new). Mirror
`tests/test_plan_build_gate_hook.py` + `tests/test_worktree_gate_hook.py`:
temp git repo in `setUp`, `_stamped_gate(mode)` replacing
`__TRACKER_CARD_GATE_MODE__` + `__TRACKER_CLI_HOME__`, `_run(event, env)`,
`_event` / `_shell_event` / `_cursor_shell_event` builders. Bootstrap seam =
project-local fallback
`repo/.recipe/trello-mcp-workflow/bootstrap-ready` (design Decision 3 §5).
**Reqs:** `tracker-card-gate` — activation, production-path by mode, openspec
never blocked, fail-open, dogfood-relevant warn semantics.

- [ ] 2.1 RED scaffolding: helpers + failing placeholders for the explore
      matrix (script may be absent/stub — tests must fail for the right reason):
      - `missing_card_blocks_prod_write` (mode=`always`, marker, active change
        w/o `## Tracker` section, `Edit lib/foo.py` → exit 2; stderr names
        tracker/card + slug + link section)
      - `with_card_allows_prod_write` (valid `## Tracker` section in
        proposal.md → exit 0)
      - `openspec_paths_never_blocked` (`Write openspec/changes/x/proposal.md`
        → exit 0)
      - `recipe_disabled_or_mode_off_allows` / `marker_absent_fail_open` /
        `mode_off_allows` (exit 0)
      - `warn_mode_allows_with_stderr` (mode=`warn` → exit 0 + non-empty stderr)
      - `tracker_none_allows_prod_write` (file `tracker.none` present → exit 0
        under `always`)
      - `no_active_change_allows`, `non_production_path_allows`
        (`Edit tests/x.py`), `malformed_stdin_fail_open`,
        `missing_file_path_fail_open`
- [ ] 2.2 RED: active-change enumeration edge cases —
      archive-only tree ignored; stray dir without proposal/tasks/spec/design
      ignored; at least one deficient active change among several → block under
      `always`.
- [ ] 2.3 RED: `TRACKER_CARD_GATE_PATHS` override honored (e.g. include
      `ai-specs` → block; default excludes it → allow `Edit ai-specs/…`).

### Phase 3 — `tracker-card-gate.sh` path-mode GREEN (Decision 4 path/activation)

**Files:** `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`
(new; create `hooks/` directory). Portable `#!/usr/bin/env bash` + `python3`
heredoc (argv[1] JSON, not single-quoted stdin — worktree-gate lesson).
**Reqs:** `tracker-card-gate` — Pre-tool-use hook; activation; production-path;
openspec never blocked; fail-open.

- [ ] 3.1 GREEN: implement path-mode algorithm (design Decision 4 steps 1–4, 4a,
      4b, 4d, 4e, 4f):
      - tokens `__TRACKER_CARD_GATE_MODE__` (default stamp `warn`) and
        `__TRACKER_CLI_HOME__`
      - env override `TRACKER_CARD_GATE_MODE` beats stamp; invalid → warn +
        fall back stamp → `warn`
      - marker resolution: cache key recompute + project-local fallback
      - production dirs default `lib catalog bin src`;
        `TRACKER_CARD_GATE_PATHS` override
      - never block `openspec/changes/**` or gitignored agent config
      - deficiency = no valid `## Tracker` section in proposal.md/tasks.md AND
        no `tracker.none`
      - `warn` → stderr + exit 0; `always` → remediation stderr + exit 2
      - **no Trello MCP calls**
- [ ] 3.2 GREEN: pass all Phase 2 path-mode tests via
      `./tests/run.sh tests/test_tracker_card_gate_hook.py`.
- [ ] 3.3 TRIANGULATE: embed tolerant parser twin in the heredoc; add
      `parser_parity` asserting gate validity equals
      `trello_link.is_valid_link` on the Phase 1 fixture matrix.

### Phase 4 — Shell-mode PR / archive coverage (Decision 4c, dual-hook ready)

**Files:** same gate script; extend `tests/test_tracker_card_gate_hook.py`.
**Reqs:** `tracker-card-gate` — *Optional shell-mode PR and archive coverage*
(shipping — design locks both shapes).

- [ ] 4.1 RED:
      - `shell_gh_pr_create_blocked_without_card` (`always` → exit 2)
      - `shell_gh_pr_create_warn_allows` (`warn` → exit 0 + stderr)
      - `shell_gh_pr_create_allowed_when_carded` (valid card → exit 0)
      - `archive_command_blocked_for_deficient_slug` (`openspec archive` /
        `ai-specs … archive` / `mv|git mv` into `openspec/changes/archive/`)
      - `ambiguous_shell_command_fail_open` (e.g. `gh pr view`, `git status`)
      - Cursor native top-level `{command,cwd}` PR-create block case
- [ ] 4.2 GREEN: implement shell extraction (design 4c) — `shlex` segments;
      wrappers/`VAR=val` strip; triggers only:
      1. `gh` + non-flag args begin `pr` `create`
      2. archive helpers / `mv|git mv` → `openspec/changes/archive/`
      Precision over recall; everything else fail-open. Pass 4.1; keep Phase 2–3
      green.

### Phase 5 — Materialize stamps + `recipe.toml` dual hooks (Decisions 2, 5)

**Files:** `lib/_internal/recipe-materialize.py`;
`catalog/recipes/trello-mcp-workflow/recipe.toml`;
`tests/test_recipe_materialize.py` and/or `tests/test_hooks_render.py` /
recipe tests as needed.
**Reqs:** `tracker-card-gate` — hook declaration + dual-hook registration;
`trello-card-linking` / state-sync / progress-comment brief rules.

- [ ] 5.1 RED: materialize tests —
      - content with `__TRACKER_CARD_GATE_MODE__` stamped from
        `merged_cfg["gate_mode"]` (default `warn`)
      - `__WORKTREE_GATE_MODE__` still stamped (regression; map not a
        single-constant regression)
      - `__TRACKER_CLI_HOME__` replaced with resolved `cli_home` when present;
        empty string when `cli_home is None`
      - worktree-gate script without `__TRACKER_CLI_HOME__` unaffected
- [ ] 5.2 GREEN: replace `GATE_MODE_PLACEHOLDER` with
      `GATE_MODE_PLACEHOLDERS` map; add `cli_home: Path | None = None` to
      `materialize_hook_script`; pass `cli_home=cli_home` from
      `materialize_recipes` call site (~line 887). Pass 5.1.
- [ ] 5.3 RED→GREEN `recipe.toml`:
      - `[config.gate_mode]` enum `off|warn|always`, default `warn`,
        `required=false` (design 2a)
      - two `[[provides.hooks]]`:
        - `id=tracker-card-gate`, matcher `Edit|Write|MultiEdit|NotebookEdit`
        - `id=tracker-card-gate-shell`, matcher `Bash|Shell|Execute|Terminal`
        both `event=pre-tool-use`, `script=hooks/tracker-card-gate.sh`,
        `blocking=true`
      - `version = "1.3.0"` (from `1.2.0`)
      - append design 2d `workflow_rules` (link before apply; phase
        state-sync; progress comments; anti-bypass; `tracker.none` exemption)
- [ ] 5.4 RED→GREEN render/recipe assertions (mirror worktree dual-hook tests):
      - Claude: two PreToolUse managed entries, same script path
      - Cursor: file-write matcher skipped; shell id registers
        `beforeShellExecution` without Edit/Write tokens
      - omp/pi: both matchers present, case-insensitive
      Pass via `./tests/run.sh` on the touched render/recipe suites.

### Phase 6 — Doctor WARN (Decision 6, 9b)

**Files:** `lib/_internal/doctor.py`; `tests/test_doctor_tracker_card.py` (new)
(and/or extend `tests/test_doctor.py` if project convention prefers — dedicated
file matches proposal Affected Areas).
**Reqs:** `project-doctor` — *Active-change missing Tracker link section WARN*
(all scenarios).

- [ ] 6.1 RED: Doctor on temp project —
      - recipe enabled + marker + missing `## Tracker` section → `tracker-card`
        WARN; doctor exit still 0
      - valid `## Tracker` section → OK (no missing-card WARN)
      - `tracker.none` → no missing-card WARN
      - recipe disabled → silent (no `tracker-card` check)
      - marker absent → silent
      - archive-only missing card → ignored (grandfather)
      - invalid/empty `card_id` in present file → WARN (deficient)
      - optional INFO nudge for non-24-hex `card_id` / missing `url` (if
        implemented; do not fail validity)
- [ ] 6.2 GREEN: add `_check_tracker_card_link` after
      `_check_recipe_cli_deps`, before `_check_harness_env_layout`; sibling-load
      `trello_link.py`; resolve marker via `recipe_skills_root` + local
      fallback; same active-change enumeration as the gate. Pass 6.1.
      `test_doctor_is_read_only` remains true.

### Phase 7 — Recipe skill / command / session-bootstrap (Decisions 7, 8a, 8c)

**Files:**
`catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md`;
`catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md`;
`catalog/recipes/session-context/skills/session-bootstrap/SKILL.md`;
doc-content assertions in recipe tests where cheap.
**Reqs:** `trello-card-linking` (canonical artifact, narrow exemption,
availability vs missing, brief before apply, MODIFIED create-from-template,
REMOVED skip free-pass); `trello-state-sync` / `trello-progress-comment`
per-phase brief rules + degrade-on-availability; `session-bootstrap`
mandatory consult; `tracker-card-gate` anti-bypass guidance.

- [ ] 7.1 GREEN (skill):
      - `auto_invoke` triggers per design 8a (new structured change / missing
        card / stale or unknown card)
      - new **"Card link section (`## Tracker`)"** section (format + validity)
      - replace mythical `trello_card_id` schema language with the
        `## Tracker` section
      - remove skip hatch; document `tracker.none` (prose: `tracker:none`) +
        logged rare exemption
      - narrow Decision #7: availability degrades; missing artifact is not
        "Trello unavailable"
      - fix bootstrap marker path docs to
        `<AI_SPECS_HOME>/cache/projects/<hash>-<name>/.recipe/trello-mcp-workflow/bootstrap-ready`
        (+ legacy project-local fallback note)
- [ ] 7.2 GREEN (command): `trello-workflow.md` phase map references the
      `## Tracker` link section.
- [ ] 7.3 GREEN (session-bootstrap): rewrite step 2c to mandatory tracker
      consult when a tracker capability is bound (new/ambiguous changes);
      keep memory-first order; unavailable tracker degrades without blocking.
- [ ] 7.4 RED→GREEN doc-content tests (prefer recipe test module): brief
      `workflow_rules` contain link-before-apply + anti-bypass + phase
      state-sync/progress-comment language; SKILL contains no "Allow the agent
      to skip card creation"; SKILL/brief forbid unavailable-excuse for missing
      artifact.

### Phase 8 — Docs honesty (Decisions 8b, 8d)

**Files:** `catalog/recipes/trello-mcp-workflow/README.md`;
`docs/runtime-hooks.md`; `docs/recipes-catalog.md` (+
`tests/test_recipes_catalog.py` only if assertions break).
**Reqs:** residual platform gaps documentation; dual-hook honesty; version
blurb.

- [ ] 8.1 README: **"Card-per-change contract"**, **"Gate modes"**
      (`off|warn|always`, `TRACKER_CARD_GATE_MODE`,
      `TRACKER_CARD_GATE_PATHS`, never blocks `openspec/**`, fail-open),
      **"Residual platform gaps"** (Cursor / OpenCode / pi-omp; MCP not
      intercepted).
- [ ] 8.2 `docs/runtime-hooks.md`: add `tracker-card-gate` /
      `tracker-card-gate-shell` to the dual-hook table; note mode stamp + env
      override; shell matcher = PR/archive (not file writes). MUST NOT claim
      MCP interception or uniform full prevention.
- [ ] 8.3 `docs/recipes-catalog.md`: bump trello-mcp-workflow version /
      description for card-per-change gate + `gate_mode`. Touch catalog tests
      only if needed.
- [ ] 8.4 `openspec/config.yaml`: add `tracking:` section declaring the global
      contract — `tracker: trello`, `board_id` (this repo's dogfood board),
      `artifact_section: "## Tracker"`, `required_fields: [card_id, url]`,
      `gate_mode: warn`. IMPORTANT — consumers of this file: the `openspec`
      CLI and the SDD phase agents read it by convention; NO ai-specs code
      (bin/lib) reads it, and its presence is NOT guaranteed in projects
      without SDD enabled. Therefore it is a canonical DECLARATIVE contract
      for agents/humans only: doctor/gate MUST keep reading the operational
      `gate_mode`/`board_id` from the recipe config in `ai-specs.toml` (never
      depend on config.yaml for enforcement). Do not duplicate other top-level
      keys. Add a doc-content test only if the repo asserts config.yaml shape;
      otherwise document the convention in the recipe README/skill.

### Phase 9 — Dogfood config (Decision 10)

**Files:** `ai-specs/ai-specs.toml`.
**Reqs:** `tracker-card-gate` — *Dogfood default warn*.

- [ ] 9.1 Set under `[recipes.trello-mcp-workflow.config]`:
      `gate_mode = "warn"` (keep existing `board_id`).
- [ ] 9.2 Run `ai-specs sync` in this worktree; confirm materialized
      `ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` stamps
      mode `warn` + CLI home; both hook ids wired for enabled harnesses.
      Revert/leave gitignored generated surface out of the commit per
      dogfood-isolation (commit only the intentional `ai-specs.toml` config
      line + catalog/docs/tests).

### Phase 10 — Hermetic eval smoke + focused suite green (Decision 9c)

**Files:** `tests/evals/eval_harness_smoke.py` and/or
`tests/evals/scenarios/trello-mcp-workflow/` load fixture; hermetic suites.
**Reqs:** eval harness recognizes trello client; CI path stays hermetic.

- [ ] 10.1 Optional but in-scope: harness-smoke / scenario.toml load assertion
      so `eval_harness_smoke` recognizes `trello-mcp-workflow` (cheap, CI-safe).
- [ ] 10.2 Run focused hermetic suites green:
      `./tests/run.sh tests/test_trello_link.py tests/test_tracker_card_gate_hook.py tests/test_doctor_tracker_card.py`
      (+ materialize / hooks_render / recipe / doctor suites touched above).

### Phase 11 — Live golden evals (Decision 9d) — first-class, not CI-gating

**Files:**
`tests/evals/run-live-trello.sh` (new; model on `run-live-worktree.sh`);
`tests/evals/eval_trello_mcp_workflow_live.py` (new; model on
`eval_worktree_flow_live.py`);
`tests/evals/scenarios/trello-mcp-workflow/ac_*/{scenario.toml,prompt.txt}`.
**Reqs:** proposal success criteria — ≥3 notes-file scenarios; MCP-live
optional. Mirrors #165 / explore eval design.

- [ ] 11.1 Add `run-live-trello.sh` with `client=trello-mcp-workflow`,
      `python3 -m unittest tests.evals.eval_trello_mcp_workflow_live -v`, and
      the same env surface as worktree live
      (`EVALS_LIVE`, `EVALS_PREFER`, `EVALS_TRIALS`, `EVALS_TIMEOUT_SEC`,
      `EVALS_MAX_TURNS`, `EVALS_RUNTIMES`, `EVALS_SCENARIOS`).
- [ ] 11.2 Add `eval_trello_mcp_workflow_live.py`: `RECIPE_ID =
      "trello-mcp-workflow"`; skipUnless live+runtimes; materialize with
      `board_id` + `gate_mode="always"`; `wire_runtime_hooks` when scenario
      meta `wire_hooks = true`; assertion battery identical to worktree
      (`required_path_globs`, `required_content.contains_any`,
      `forbidden_path_globs`, `forbidden_phrases`); N-of-M via `EVALS_TRIALS`.
- [ ] 11.3 Scenario `ac_new_change_writes_tracker_section` — required: change
      proposal.md contains a `## Tracker` section with `card_id`; notes mention
      create/link.
- [ ] 11.4 Scenario `ac_missing_card_gate_no_bash_skip` — `wire_hooks=true`;
      seeds card-less active change; notes say create/link first;
      `forbidden_phrases` reject `python3 -c`, `cat >`, `tee `, `sed -i`,
      `heredoc` (reuse worktree gate-plan phrase set).
- [ ] 11.5 Scenario `ac_phase_transition_state_sync_plan` — notes include
      move/list/label/comment from the phase map.
- [ ] 11.6 Scenario `ac_retro_change_without_card_triggers_link` — seeded
      active change without a `## Tracker` section; agent writes the section
      before claiming done.
- [ ] 11.7 Optional expensive `ac_mcp_live_card_link` — tool evidence
      `trello_add_card_to_list` / `trello_add_comment`; disposable list +
      cleanup; board isolation. Not required for CI or for marking Phase 11
      done; document how to run it.
- [ ] 11.8 Manual/nightly smoke note: document
      `EVALS_LIVE=1 ./tests/evals/run-live-trello.sh` in recipe README (or
      evals README pointer). Do **not** wire into `validate.sh`.

### Phase 12 — CHANGELOG + verification / close

**Files:** `CHANGELOG.md`; this change folder; Trello #56.
**Reqs:** proposal success criteria checklist; archive hygiene.

- [ ] 12.1 `CHANGELOG.md` under `## [Unreleased]`:
      - **Added**: tracker-card gate (`warn|always|off`), doctor missing-card
        WARN, hermetic gate/doctor tests, live trello eval client + scenarios
      - **Changed**: trello-mcp-workflow `1.2.0` → `1.3.0`; skip hatch →
        `tracker.none`; Decision #7 narrowed; session-bootstrap consult when
        tracker bound
- [ ] 12.2 Write the `## Tracker` section in this change's `proposal.md` for
      existing card #56 (`card_id` `6a6ebd5e2cd9a2fcd419e62c`, `url`
      `https://trello.com/c/WHZ3fLzD/56-…`, optional shortLink/list/pr).
- [ ] 12.3 Trello card sync (agent/MCP, not the gate): move/label for
      tasks→apply→verify phases; progress comment at milestones; keep card
      state honest through archive.
- [ ] 12.4 Cross-check every delta scenario has a RED→GREEN task or an
      explicit prose/manual verification note (live agent behaviors = Phase 11
      / manual).
- [ ] 12.5 Dogfood smoke (isolated): with mode `warn`, confirm a production
      edit without card warns on stderr and does not block; with
      `TRACKER_CARD_GATE_MODE=always` one-shot, confirm block + remediation;
      writing under `openspec/changes/**` never blocks. Revert any leaked
      generated files.
- [ ] 12.6 FINAL GATE: `./tests/validate.sh` green from the change worktree
      root (py_compile + `bash -n` + full tests). Fix drift only.
- [ ] 12.7 Independent verify against proposal/design/specs/tasks; write
      verify report if the verify skill requires it.
- [ ] 12.8 Archive the change folder on the review branch after verify PASS
      (and after Judgment Day / adversarial review if that gate is invoked);
      open/attach PR with Trello card URL.

---

## File touch checklist (implement phase)

| File | Action |
|------|--------|
| `lib/_internal/trello_link.py` | **Add** — parse + validity predicate |
| `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | **Add** — path + shell gate |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | Modify — `gate_mode`, dual hooks, brief rules, version `1.3.0` |
| `catalog/recipes/trello-mcp-workflow/skills/…/SKILL.md` | Modify — auto_invoke, artifact, exemption, Decision #7, marker path |
| `catalog/recipes/trello-mcp-workflow/README.md` | Modify — contract, modes, gaps, live how-to |
| `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md` | Modify — phase map → `## Tracker` section |
| `catalog/recipes/session-context/skills/session-bootstrap/SKILL.md` | Modify — step 2c mandatory when bound |
| `lib/_internal/recipe-materialize.py` | Modify — `GATE_MODE_PLACEHOLDERS` + `cli_home` stamp |
| `lib/_internal/doctor.py` | Modify — `_check_tracker_card_link` |
| `docs/runtime-hooks.md` | Modify — dual-hook + mode stamp honesty |
| `docs/recipes-catalog.md` | Modify — version/blurb |
| `ai-specs/ai-specs.toml` | Modify — `gate_mode = "warn"` |
| `openspec/config.yaml` | **Add** — `tracking:` section (tracker, board_id, section name, required fields, gate mode) |
| `CHANGELOG.md` | Modify — Unreleased entry |
| `tests/test_trello_link.py` | **Add** |
| `tests/test_tracker_card_gate_hook.py` | **Add** |
| `tests/test_doctor_tracker_card.py` | **Add** |
| `tests/test_recipe_materialize.py` / `tests/test_hooks_render.py` / recipe tests | Extend as needed |
| `tests/evals/run-live-trello.sh` | **Add** |
| `tests/evals/eval_trello_mcp_workflow_live.py` | **Add** |
| `tests/evals/scenarios/trello-mcp-workflow/ac_*` | **Add** (3–4 + optional MCP-live) |
| `## Tracker` section in `openspec/changes/tracker-card-gate/proposal.md` | **Add** in close phase (card #56) |
| Generated `ai-specs/recipes/**` shims | Via `ai-specs sync` only — never hand-edit / prefer not commit |

---

## Requirement → phase map

| Delta requirement | Spec | Kind | Phases |
|-------------------|------|------|--------|
| Canonical `## Tracker` link section | trello-card-linking | ADDED | 1, 7, 8, 12.2 |
| Narrow tracker:none exemption (`tracker.none` on disk) | trello-card-linking | ADDED | 2–4, 6, 7 |
| Availability failure vs missing link artifact | trello-card-linking | ADDED | 7, 8 |
| Brief rules require link artifact before apply | trello-card-linking | ADDED | 5.3, 7.4 |
| Create card from template when absent | trello-card-linking | MODIFIED | 7.1 |
| Skip card creation free pass | trello-card-linking | REMOVED | 7.1, 7.4 |
| Per-phase brief rules for state sync | trello-state-sync | ADDED | 5.3, 7 |
| Degrade on availability failure only (state-sync) | trello-state-sync | ADDED | 7, 8 |
| Per-phase brief rules for progress comments | trello-progress-comment | ADDED | 5.3, 7 |
| Degrade on availability failure only (progress) | trello-progress-comment | ADDED | 7, 8 |
| Mandatory tracker consult when tracker is bound | session-bootstrap | ADDED | 7.3 |
| Active-change missing Tracker link section WARN | project-doctor | ADDED | 6 |
| Pre-tool-use tracker card gate hook | tracker-card-gate | ADDED | 3, 5 |
| Gate activation predicate | tracker-card-gate | ADDED | 2, 3, 5 |
| Production-path enforcement by mode | tracker-card-gate | ADDED | 2, 3 |
| openspec paths never blocked | tracker-card-gate | ADDED | 2, 3 |
| Fail-open on parse and lookup errors | tracker-card-gate | ADDED | 2, 3 |
| Optional shell-mode PR and archive coverage | tracker-card-gate | ADDED | 4, 5.3–5.4 |
| Dogfood default warn | tracker-card-gate | ADDED | 9 |
| Anti-bypass brief and skill guidance | tracker-card-gate | ADDED | 5.3, 7, 8 |

---

## Notes

- **Exemption on-disk name (apply alignment):** conceptual exemption is
  `tracker:none`; delta specs lock the on-disk filename as **`tracker.none`**.
  Design Decision 4f/6 still writes the colon form — apply does a **one-line
  rename** of those path literals to `tracker.none` (do not create a
  colon-named file). Tasks and tests already use `tracker.none`.
- Dogfood default remains `gate_mode = "warn"`; promoting this repo to `always`
  is a later config flip after evals prove the contract.
- Live MCP-board scenario (`ac_mcp_live_card_link`) is optional/expensive and
  not CI-gating; notes-file goldens land first.
- Generated harness shims are sync-only artifacts — never hand-edit.

---

## Flagged issues (non-blocking for planning; resolve at apply)

1. **On-disk exemption filename contradiction (design vs specs).**
   See Notes: design uses `tracker:none` path literals; specs/tasks lock
   **`tracker.none`**. Apply aligns with a one-line rename; do not create a
   colon-named file.
2. **`ai-specs/` production-dir default (design §Proposal notes).** Excluded
   from default `TRACKER_CARD_GATE_PATHS` (`lib catalog bin src`). Product may
   flip later; Phase 2.3 covers opt-in. Not blocking.
3. **Shell-gate scope locked narrow.** Only `gh pr create` + archive helpers
   (design 4c). Broader shell write heuristics are intentionally out
   (fail-open, precision-first).
4. **`tracker:none` logging is agent/brief responsibility**, not a gate
   side-effect (design §Proposal notes). Gate honors `tracker.none` silently;
   skill/brief own the log.
5. **This change's proposal.md currently lacks a `## Tracker` section.** Card
   #56 already exists. Phase 12.2 writes the section before archive/PR;
   dogfood stays on `warn` so planning/apply does not self-deadlock.
6. **Platform residual gaps** (Cursor no pre-file-write; OpenCode
   MCP/subagent; pi/omp children) remain — documented in Phase 8; mitigated by
   dual-hook + brief anti-bypass + live evals. Do not overclaim in docs.

---

## Authorization checkpoint

**Status: PLANNING COMPLETE — apply NOT authorized by this artifact alone.**

Await maintainer go-ahead (check P0.6) before any RED/GREEN implementation.
After authorization, follow Phases 1→12 in order; hermetic suites must stay
green before live eval work is treated as merge-blocking evidence.
