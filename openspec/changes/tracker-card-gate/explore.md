# Explore: tracker-card-gate

Phase: **explore** (sdd-explore subagent, read-only, 2026-08-02)
Branch: `feat/tracker-card-gate` — Worktree: `/Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate`
Base: `development@2e8e952`
Tracker: Trello #56 (`WHZ3fLzD`)
Engram: topic `sdd/tracker-card-gate/explore` (obs #1572)

## Problem framing

A complete SDD cycle for compact-sync-output (PR #166) finished with no Trello card.
This is not a one-off agent lapse: tracker integration is deliberately soft. Sync hooks
for link/sync/comment are deferred no-ops (recipe-materialize.py:570-574), the skill has
`auto_invoke:false` and an explicit skip hatch (SKILL.md:12,128), design Decision #7 says
Trello failures never block (design.md:64-67), and only 2 of 70 archived changes carry a
`trello.md` link artifact. No hard gate, doctor check, or schema requires a card.

## Findings by target

### A. Enforcement surface inventory

Classification: 1 ENFORCED / 8 ADVISORY / 3 ABSENT.

- **ENFORCED**: recipe enabled + `board_id` required at sync (`ai-specs.toml:64-67`;
  `recipe.toml` config.board_id; validate-config/bootstrap-board on-sync).
- **ADVISORY**:
  - Deferred sync hooks `link-trello-card` / `sync-card-state` / `comment-verification`
    (`recipe-materialize.py:570-574`).
  - Skill `auto_invoke:false` (`SKILL.md:12`).
  - Skip hatch `SKILL.md:128` + spec "Skip card creation" SHALL allow change without card
    (`openspec/.../specs/trello-card-linking/spec.md:34-36`).
  - Never-block policy `SKILL.md:202` + design Decision #7 (`design.md:64-67`).
  - Brief `workflow_rules` "inspect the active card" (`recipe.toml` provides.brief →
    `AGENTS.md:75`).
  - "Trello is source of truth" brief (`AGENTS.md:42`).
  - Command `trello-workflow.md` phase map.
  - `session-bootstrap` consults tracker "only if needed" (`session-bootstrap/SKILL.md:6,41`).
- **ABSENT**:
  - `[[provides.hooks]]` pre-tool-use hard gate for card presence (trello recipe has only
    on-sync `[[hooks]]`).
  - Schema/doctor/pre-commit requiring `trello.md` or `trello_card_id`.
  - Re-audit trigger for retro-rescued changes.

Key evidence: `SKILL.md:110` (trigger "New structured change"), `SKILL.md:116`
(`trello_card_id` field mentioned but not a real schema), `recipe-materialize.py:564`
(bootstrap-ready write), archive `2025-04-30-trello-mcp-workflow-recipe/design.md:40`
(Decision #2: deferred runtime), `session-bootstrap/SKILL.md:41`.

### B. Hard-gate precedent (worktree / plan-build)

- **worktree-flow**: `catalog/recipes/worktree-flow/recipe.toml:87-100`
  `[[provides.hooks]]` worktree-gate + worktree-gate-shell, event=pre-tool-use,
  blocking=true; `hooks/worktree-gate.sh` exit 0/2/fail-open.
- **plan-build-flow**: `catalog/recipes/plan-build-flow/recipe.toml:29-34`
  `[[provides.hooks]]`; `hooks/plan-build-gate.sh` **blocks production edits**
  (src|lib|catalog) until `openspec/changes/<slug>/tasks.md` exists; intentionally
  non-bypassable. This is the closest semantic model for a tracker gate (artifact must
  exist before production work).
- **Distribution**: `docs/runtime-hooks.md` + `lib/_internal/hooks-render.py` materialize
  adapters for claude/cursor/opencode/pi/omp; exit 2 = block.
- **Platform limits** (cannot reliably intercept Trello MCP tools):
  - OpenCode `tool.execute.before` does not fire for MCP (#2319) or subagents (#5894) —
    `docs/runtime-hooks.md:67-68`.
  - Cursor has no pre-file-write hook — `docs/runtime-hooks.md:38,65,118`.
  - pi/omp this-process only; delegated subagents may miss hooks —
    `docs/runtime-hooks.md:70,115`.
  - Implication: gate file/shell actions, or use doctor/schema/evals — not MCP
    interception.

### C. Change-folder contract

- `trello_card_id` field: mentioned only in `SKILL.md:116` — not a real folder schema.
- De-facto artifact: `openspec/changes/<slug>/trello.md` with card_id/shortLink/url/list
  — present in only 2 of 70 archives (`2026-07-23-materialization-followup-guidance`,
  `2026-07-23-minimal-project-materialization`).
- Doctor (`doctor.py:140-152`) checks manifest/cli/agents/bundled/deps/env/topology/
  templates — no openspec/trello card check.
- No bin/lib/scripts validator requires card metadata; no project pre-commit gate.
- Candidate hook points: doctor WARN/FAIL · sync/archive helper · plan-build-style
  pre-tool-use when active change lacks trello.md · eval contract · future
  `.openspec.yaml` field.

### D. Eval framework

Conventions (`tests/evals/`):
- `eval_*.py` naming so test discovery skips them; `scenarios/<recipe>/<ac_*>/`
  {scenario.toml, prompt.txt}; natural-language prompts; `assert_natural_prompt` rejects
  meta prompts; assertions via required/absent/forbidden path globs,
  `required_content.contains_any`, `forbidden_phrases`; `wire_runtime_hooks` for
  `[[provides.hooks]]`; `EVALS_LIVE=1` for live; N-of-M via `EVALS_TRIALS`; fixtures =
  temp project + materialize + seed + git.
- Hermetic/CI: `eval_harness_smoke.py` + `tests/evals/run.sh`; unit gate tests
  (`test_*_gate_hook.py`) ride validate.sh/run.sh.
- Live/manual: `run-live.sh`, `run-live-vcs.sh`, `run-live-vault.sh`,
  `run-live-worktree.sh` — billed, flaky, not per-PR.
- Precedent #165 (commit `2e8e952`, merged): "tests: golden eval scenarios for
  worktree-flow (topology + gate)" — +723 lines: `eval_worktree_flow_live.py`,
  `run-live-worktree.sh`, harness/project_fixture extensions, 4 scenarios incl.
  `ac_gate_blocked_write_creates_worktree_not_bash_fallback` (notes-file plan
  assertions). This is the template for `run-live-trello.sh`.
- No Trello eval client exists today. No CI workflow wires live evals; hermetic/unit ride
  validate.sh, live = manual/nightly.

### E. Option shapes (effort / risk)

1. **Pure process** — low effort / high miss rate. Already mostly present;
   session-bootstrap deprioritizes tracker.
2. **Recipe hardening** — low-med effort / medium risk. auto_invoke, remove skip hatch,
   per-phase workflow_rules; agents can still ignore.
3. **Hard pre-tool-use gate** — high effort / high platform+dogfood risk. Strong on
   Claude; weak OpenCode MCP/subagents; Cursor file-write gap.
4. **Change-folder schema** — med effort / medium migration. Require trello.md;
   grandfather 68 archives.
5. **Doctor check** — low-med effort / low risk. WARN first; FAIL opt-in.
6. **Eval-only contract** — med effort / medium risk. Proves intent; does not stop
   production agents alone.

### F. Dogfood state

- Recipe enabled, `board_id: 69ec097f13e2d38ecd89a557` (`ai-specs/ai-specs.toml:64-67`).
- Project-local `.recipe/.../bootstrap-ready`: absent in this worktree; present in main
  project cache (`ai-specs-cli/cache/projects/df4360950abd-ai-specs-cli/.recipe/
  trello-mcp-workflow/bootstrap-ready`).
- Path drift: SKILL.md still documents `.recipe/...` project-local; runtime uses
  `cache/projects/<hash>-<name>/.recipe/`.
- Hard gate today: NONE — would not currently self-block.
- Self-block risk if hard gate ships: HIGH for retro SDD / archive without card / this
  explore, unless exempts + mode exist.

## Reuse opportunities

- `plan-build-gate.sh` — closest semantic model (artifact must exist before production
  work).
- worktree-gate dual matchers — path + shell bypass coverage.
- `hooks-render.py` + `docs/runtime-hooks.md` — distribution + known gaps.
- #165 eval pattern — golden notes-file scenarios + `run-live-<client>.sh`.
- Existing `trello.md` samples as de-facto card link format.
- Doctor Check/Severity pattern (`doctor.py`) for cheap WARN.
- Deferred-hook tests in `test_recipe_materialize.py` if sync behavior changes.

## Approach options

| Name | Components | Pros | Cons |
|---|---|---|---|
| Soft-only hardening | 2 | cheap | won't stop compact-sync-class gaps |
| Soft + doctor + evals | 2,5,6 | visibility + TDD contract without deadlock | no hard block |
| **Soft + phased hard gate + evals (recommended)** | 2,5,6,3 | dogfood-safe warn→always; reuses plan-build pattern | platform gaps remain for MCP/subagents |
| Full hard gate + schema | 2,3,4,5,6 | maximum enforcement | highest migration/dogfood friction |

## Eval design (first-class deliverable)

### Hermetic / CI (unit + dry)

- `test_tracker_card_gate_hook.py::missing_card_blocks_prod_write` — active change
  without trello.md; Edit lib/foo.py → exit 2, stderr mentions tracker/card.
- `test_tracker_card_gate_hook.py::with_card_allows_prod_write` — trello.md has card_id
  → exit 0.
- `test_tracker_card_gate_hook.py::openspec_paths_never_blocked` — write
  `openspec/changes/x/proposal.md` without card → exit 0.
- `test_tracker_card_gate_hook.py::recipe_disabled_or_mode_off_allows` — gate mode off /
  recipe unset → exit 0.
- `test_tracker_card_gate_hook.py::shell_gh_pr_create_blocked_without_card` — Bash
  `gh pr create` without card → exit 2 if shell matcher adopted.
- `test_doctor_tracker_card.py` — active change missing trello.md with recipe enabled →
  WARN (or FAIL if configured).
- `eval_harness_smoke` extension — scenario.toml for trello client loads.

### Live / manual (golden, mirrors #165)

Runner: `tests/evals/run-live-trello.sh` → `eval_trello_mcp_workflow_live.py`.
Scenarios (notes-file assertions, MCP not required except the last):
- `ac_new_change_writes_trello_md` — openspec/changes/*/trello.md with card_id; notes
  mention create/link.
- `ac_missing_card_gate_no_bash_skip` — notes say create/link first; forbidden bash
  bypass phrases; needs wire_runtime_hooks.
- `ac_phase_transition_state_sync_plan` — notes include move/list/label/comment from
  phase map.
- `ac_retro_change_without_card_triggers_link` — seeded change without trello.md; agent
  links before claiming done.
- `ac_mcp_live_card_link` (optional expensive) — tool evidence
  trello_add_card_to_list / trello_add_comment; board isolation; MCP required.

CI-runnable: hermetic unit + doctor + dry smoke. Manual/nightly: live golden + optional
MCP-live.

## Open questions (proposal must settle)

1. Required always, or only when trello-mcp-workflow enabled + bootstrap marker present?
   (recommend latter)
2. Canonical artifact: `trello.md` vs `trello_card_id` in `.openspec.yaml` vs both?
3. Which phases hard-require a card: proposal / tasks / apply / PR / archive?
4. Keep narrow `tracker:none` exemption or delete skip hatch entirely?
5. Retro changes: auto-create at first gated action, or block until link?
6. Default dogfood mode: warn vs always on this board?
7. Should session-bootstrap stop saying "tracker only if needed" when tracker is bound?
8. Gate on abstract tracker capability (future Jira) or Trello-specific?

## Risks & constraints

- **Dogfood self-blocking** (this change / retro SDD / archive without card) — high;
  mitigate with phased mode, exempt openspec writes, require marker, grandfather archives.
- **Platform hook gaps** (OpenCode MCP #2319, subagents #5894, Cursor no pre-file-write)
  — high for hard MCP gate; mitigate: gate files/shell not MCP; brief anti-bypass; evals.
- **Soft→hard culture clash** with Decision #7 "never block" — medium; explicit overturn
  in proposal/design; distinguish availability failure vs missing link artifact.
- **False security from eval-only** — medium; pair evals with doctor and/or gate.
- **bootstrap-ready path drift** (docs vs cache layout) — medium; fix docs in change.
- **Disposable-board pollution** from MCP-live evals — medium; dedicated list + cleanup
  or notes-only first.
- **Metadata dual vocabulary** (`trello_card_id` vs `trello.md`) — low-med; standardize
  in proposal.

## Recommended approach

Hybrid **recipe hardening + doctor WARN + first-class hermetic/live evals**, with an
**optional phased hard gate (warn→always)** modeled on plan-build-gate — not MCP
interception.

Steps:
1. Standardize `openspec/changes/<slug>/trello.md` (card_id, url, optional
   shortLink/list/pr); align skill away from vague `trello_card_id`.
2. Recipe harden: auto_invoke triggers; replace broad skip hatch with rare
   `tracker:none`; per-phase workflow_rules for link→state-sync→progress-comment; fix
   bootstrap path docs to cache `.recipe`.
3. session-context: when tracker bound, bootstrap must consult tracker for
   new/ambiguous changes.
4. Doctor WARN for active changes missing trello.md when recipe enabled + marker present.
5. Evals first-class (TDD): hermetic gate/doctor tests in CI; live golden client
   mirroring #165 (`run-live-trello.sh` + 3–4 scenarios).
6. Phased hard gate after evals green: `tracker-card-gate.sh` blocks production writes /
   `gh pr create` when active change lacks trello.md; default warn or config `always`
   for dogfood; never block openspec artifact writes.
7. Do not rely on intercepting Trello MCP tools (OpenCode #2319).
8. Overturn design Decision #7 for card-link presence (availability failures may still
   degrade gracefully).

## Ready for proposal

**Yes.** The proposal must decide: activation scope (recipe-enabled+marker only vs
always), canonical link artifact shape, phase threshold for hard require, exemption
shape (`tracker:none` vs none), default gate mode for this dogfood repo, whether
session-bootstrap language changes when tracker is bound, and abstract-tracker vs
Trello-specific naming.
