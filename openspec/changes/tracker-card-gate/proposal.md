# Proposal: tracker-card-gate

## Intent

A full SDD cycle for `compact-sync-output` (PR #166) shipped with **no Trello
card**. That was not a one-off agent lapse: tracker integration is deliberately
soft today.

Evidence from explore (`openspec/changes/tracker-card-gate/explore.md`):

- Recipe skill `auto_invoke: false` — never loads alone.
- Sync hooks `link-trello-card` / `sync-card-state` / `comment-verification` are
  deferred no-ops (`recipe-materialize.py` prints info and continues).
- Skill skip hatch + spec "Skip card creation" allow changes without a card.
- Design Decision #7 / SKILL graceful degradation: Trello failures **never block**.
- `session-bootstrap` consults tracker "**only if needed**".
- Only **2 of 70** archived changes carry a `trello.md` link artifact.
- No `[[provides.hooks]]` hard gate, no doctor check, no schema requires a card.

This change makes **card-per-change a real contract** when
`trello-mcp-workflow` is enabled: recipe hardening + doctor WARN + first-class
hermetic/live evals, plus a **phased hard pre-tool-use gate** (plan-build-gate
pattern) that can escalate from `warn` → `always`. It does **not** intercept
Trello MCP tools (OpenCode #2319 / #5894 make that unreliable).

Tracker: [Trello #56](https://trello.com/c/WHZ3fLzD/56-sdd-gate-de-tracker-garantizar-card-trello-por-cambio-evals)
(`card_id` `6a6ebd5e2cd9a2fcd419e62c`).

Exploration: `openspec/changes/tracker-card-gate/explore.md`.

## Scope

### In scope

1. **Canonical link artifact** — standardize
   `openspec/changes/<slug>/trello.md` as the sole card-link contract for active
   changes (see Locked decisions). Align skill text away from the mythical
   schema field `trello_card_id`.

2. **Recipe hardening (`trello-mcp-workflow`)**
   - Turn on useful `auto_invoke` triggers for new/ambiguous structured changes.
   - Replace the broad skip hatch with a narrow, documented `tracker:none`
     exemption (logged when used).
   - Expand `provides.brief.workflow_rules` for
     link → state-sync → progress-comment per phase.
   - Fix bootstrap path docs: runtime marker lives under
     `cache/projects/<hash>-<name>/.recipe/…`, not a project-local `.recipe/`.
   - Overturn Decision #7 **for missing link artifact** (see Approach); keep
     graceful degrade for MCP/network **availability** failures.

3. **session-bootstrap language** — when a tracker capability is bound, bootstrap
   MUST consult the tracker for new/ambiguous changes (remove "only if needed"
   soft-out for that case). Keep capability-agnostic wording.

4. **Doctor WARN** — when recipe enabled + bootstrap marker present, active
   (non-archive) changes missing a valid `trello.md` → `Severity.WARN`. Optional
   later FAIL via config is out of v1 default.

5. **Phased hard gate** — new `tracker-card-gate.sh` via `[[provides.hooks]]`
   `pre-tool-use`, modeled on `plan-build-gate.sh`:
   - Blocks **production** writes (`lib|catalog|bin|…` — design locks exact set)
     and high-confidence PR/archive shell actions (e.g. `gh pr create`) when an
     **active** change lacks `trello.md`.
   - **Never** blocks writes under `openspec/changes/**` (agents must be able to
     create the link artifact and planning files).
   - Modes: `off` | `warn` | `always` (config + env stamp, dogfood default
     `warn`).
   - Dual-hook distribution if shell PR/archive coverage is included (same
     Cursor lesson as worktree-gate-shell: file-write matcher must not swallow
     shell).

6. **Evals (first-class deliverable)**
   - Hermetic/CI: `tests/test_tracker_card_gate_hook.py`,
     `tests/test_doctor_tracker_card.py`, optional harness-smoke scenario load.
   - Live/golden (mirrors #165): `tests/evals/run-live-trello.sh` →
     `eval_trello_mcp_workflow_live.py` with notes-file scenarios
     (`ac_new_change_writes_trello_md`, `ac_missing_card_gate_no_bash_skip`,
     `ac_phase_transition_state_sync_plan`, `ac_retro_change_without_card_triggers_link`;
     optional expensive `ac_mcp_live_card_link`).

7. **Docs + delta specs** — recipe README, `docs/recipes-catalog.md` /
   runtime-hooks note as needed; delta under this change for
   `trello-card-linking` (and related tracker capabilities) plus doctor/gate
   requirements.

8. **Dogfood config** — this repo keeps gate mode **`warn`** so planning this
   change does not self-deadlock; `always` is opt-in via recipe config.

### Out of scope

- Intercepting Trello MCP tool calls at the harness layer (OpenCode MCP/subagent
  pre-hook gaps; wrong enforcement surface).
- Introducing `.openspec.yaml` / folder-schema `trello_card_id` field.
- Migrating or failing the 68 archives that lack `trello.md` (grandfather).
- Making doctor FAIL-by-default or a project pre-commit hard fail in v1.
- Implementing deferred sync hooks as real sync-time MCP callers (still
  agent-runtime; sync stays agent-less).
- Abstract multi-tracker product (Jira/Linear/GitHub Issues) — design for
  swappable client later; v1 naming stays Trello-specific.
- Closing platform hook gaps (Cursor no pre-file-write; OpenCode subagent/MCP;
  pi/omp child processes) — document + brief anti-bypass only.
- Auto-creating Trello cards from the gate script itself (gate only enforces
  artifact presence; agents/MCP create cards).
- Requiring a card when `trello-mcp-workflow` is disabled or bootstrap marker
  is absent.

## Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| `trello-mcp-workflow` / `trello-card-linking` | **Modified** | Canonical `trello.md`; remove broad skip; narrow `tracker:none`; auto_invoke + brief rules; Decision #7 narrowed |
| `trello-state-sync` / `trello-progress-comment` | **Modified** | Brief/skill phase rules strengthened; still degrade on availability failure |
| `tracker` (abstract capability id already declared) | **Unchanged contract** | Still the capability agents name; enforcement remains Trello-backed in v1 |
| `session-bootstrap` (session-context) | **Modified** | When tracker bound → must consult for new/ambiguous changes |
| `project-doctor` | **Modified** | WARN active changes missing `trello.md` when recipe+marker |
| `runtime-hook-distribution` | **Unchanged (default)** | New `[[provides.hooks]]` entries reuse existing renderers; dual-hook if shell matcher needed |

## Locked decisions

Settled from explore open questions (parent/product lock, 2026-08-02):

| # | Question | Decision |
|---|----------|----------|
| 1 | Activation scope | Gate/doctor/evals active **only** when `trello-mcp-workflow` is enabled **and** bootstrap marker is present. Never hard-require when recipe absent. |
| 2 | Canonical artifact | `openspec/changes/<slug>/trello.md` (frontmatter-free markdown). Required keys: `card_id`, `url`. Optional: `shortLink`, `list`, `pr`. `trello_card_id` in skill text = legacy vocabulary pointing at this file. **No** `.openspec.yaml` schema in this change. |
| 3 | Phase threshold | Soft (doctor WARN + evals) from **proposal** onward. **Hard** gate blocks only at **apply-time production writes** and **PR/archive** actions. `openspec/**` writes never blocked. |
| 4 | Exemption | Narrow `tracker:none` marker (documented; logged when used) replaces broad skip hatch. Skip-hatch skill/spec text removed. |
| 5 | Dogfood default | **`warn`** on this repo (no self-block for this change). `always` opt-in via recipe config. |
| 6 | session-bootstrap | When tracker capability is bound → **MUST** consult tracker for new/ambiguous changes. |
| 7 | Naming | **Trello-specific** for v1 (`trello-mcp-workflow`, `trello.md`). Gate/doctor/artifact shaped so a future abstract tracker can swap the client — note as future, not scope. |
| 8 | Evals | Hermetic CI unit tests (gate + doctor) **and** live golden client (`run-live-trello.sh`, 3–4 scenarios, notes-file assertions; MCP-live optional). |

### Canonical `trello.md` shape (locked)

De-facto samples already in archive
(`2026-07-23-materialization-followup-guidance`,
`2026-07-23-minimal-project-materialization`):

```markdown
# Trello link

- **card_id**: `<24-hex>`
- **shortLink**: `<8-char>`          # optional
- **url**: https://trello.com/c/...
- **list**: <list name>              # optional
- **pr**: https://github.com/...     # optional
```

Validity for gate/doctor: file exists under the active change folder and
contains a non-empty `card_id` (and preferably `url`). Exact parse rules land
in design (tolerant markdown list / bold-key form above).

### Decision #7 overturn (narrow)

| Failure class | Old policy | New policy |
|---------------|------------|------------|
| MCP/network/API **unavailable** | never block | still degrade (warn + continue) |
| Recipe disabled / no bootstrap marker | n/a | gate inactive |
| **Missing link artifact** while recipe+marker and mode=`always` | never block | **block** production / PR-archive actions |
| Missing link + mode=`warn` | n/a | allow + stderr WARN (dogfood default) |
| Explicit `tracker:none` | broad skip | allow with logged exemption |

## Approach

### 1. Artifact-first contract

Agents create or link a card, then write `trello.md` in the change folder.
All enforcement surfaces (skill, doctor, gate, evals) key off that file — not
MCP session state and not a schema field.

### 2. Recipe + brief hardening

In `catalog/recipes/trello-mcp-workflow/`:

- Skill: enable `auto_invoke` for new structured change / missing link;
  delete "Allow the agent to skip card creation"; document `tracker:none`
  exemption only.
- Brief `workflow_rules`: require link artifact before apply; state-sync on
  phase transitions; progress comments on milestones.
- README + SKILL: correct bootstrap marker path to cache layout; document gate
  modes and residual platform gaps.
- Spec delta: remove/replace "Skip card creation SHALL allow…"; add `trello.md`
  SHALL + exemption + availability vs missing-artifact distinction.

### 3. session-bootstrap

Edit `catalog/recipes/session-context/skills/session-bootstrap/SKILL.md`:
when tracker capability is configured/bound, step 2c becomes mandatory for
new or ambiguous changes (not "only if needed"). Memory-first order can remain;
tracker consultation is no longer optional in the ambiguous path.

### 4. Doctor WARN

Extend `lib/_internal/doctor.py` with a check that:

1. Detects recipe enabled + bootstrap-ready marker present.
2. Scans `openspec/changes/*/trello.md` (non-archive).
3. For each active change folder lacking valid `trello.md` (and without
   `tracker:none`), emits `Severity.WARN` naming the slug and remediation
   ("create/link card; write trello.md").

Default WARN only — does not fail doctor exit unless a future config opts in
(not v1).

### 5. Phased `tracker-card-gate.sh`

Portable pre-tool-use script (stdin JSON contract, exit `0` allow / `2` block /
other fail-open), distributed by the trello recipe:

```text
Activation: recipe enabled ∧ bootstrap marker ∧ gate_mode ≠ off
Path mode:  production path edit without valid trello.md on active change
Shell mode: high-confidence gh pr create / archive helpers (design locks list)
Exempt:     openspec/**, tracker:none, mode=off, parse errors (fail-open)
mode=warn:  exit 0 + stderr warning (dogfood)
mode=always: exit 2 + remediation pointing at trello.md / card create
```

Semantic model: **plan-build-gate** (artifact must exist before production work),
not worktree protected-branch logic. Distribution reuses `hooks-render.py`; if
shell PR coverage ships, use a **sibling** shell hook id (Cursor skip rule).

Do **not** call Trello MCP from the gate. Presence of `trello.md` is the proof.

### 6. Evals as TDD backbone

Ship hermetic tests before/with the gate (strict TDD). Live golden scenarios
prove agent-facing behavior with notes-file assertions (same pattern as
`ac_gate_blocked_write_creates_worktree_not_bash_fallback` in #165). MCP-live
board mutation stays optional/expensive with disposable-list hygiene.

### 7. Anti-bypass policy

Brief + skill: if the gate warns/blocks, create/link the card and write
`trello.md` — never bypass via shell writes, skipping sync, or claiming
"Trello unavailable" when the failure is a missing artifact.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | Modified | `gate_mode` config; brief rules; `[[provides.hooks]]`; version bump |
| `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | **Added** | Pre-tool gate script |
| `catalog/recipes/trello-mcp-workflow/skills/…/SKILL.md` | Modified | auto_invoke; artifact contract; remove skip; Decision #7 narrow; path docs |
| `catalog/recipes/trello-mcp-workflow/README.md` | Modified | Contract, modes, gaps, eval how-to |
| `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md` | Modified | Phase map references `trello.md` |
| `catalog/recipes/session-context/skills/session-bootstrap/SKILL.md` | Modified | Tracker consult when bound |
| `lib/_internal/doctor.py` | Modified | Active-change missing-card WARN |
| `docs/runtime-hooks.md` / `docs/recipes-catalog.md` | Modified as needed | Gate pattern + honesty about MCP non-interception |
| `openspec/specs/trello-card-linking/spec.md` (via delta) | Modified | Artifact required; skip → `tracker:none` |
| Related tracker specs (bootstrap/state-sync) | Delta if needed | Availability vs artifact; bootstrap consult |
| `tests/test_tracker_card_gate_hook.py` | **Added** | Hermetic gate cases from explore eval design |
| `tests/test_doctor_tracker_card.py` | **Added** | Doctor WARN cases |
| `tests/evals/eval_trello_mcp_workflow_live.py` | **Added** | Live runner |
| `tests/evals/run-live-trello.sh` | **Added** | Manual/nightly entry |
| `tests/evals/scenarios/trello-mcp-workflow/ac_*` | **Added** | 3–4 golden scenarios (+ optional MCP-live) |
| `ai-specs/ai-specs.toml` (dogfood) | Modified | `gate_mode = "warn"` explicit |
| Generated harness shims | Via `ai-specs sync` only | Never hand-edit |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Dogfood self-blocking this change / retro SDD | High if `always` | Default **`warn`**; never block `openspec/**`; `tracker:none` escape; grandfather archives |
| Agents treat `warn` as ignorable → compact-sync-class gap persists | Medium | Evals + brief anti-bypass; path to `always` after dogfood; doctor visibility |
| Soft→hard culture clash with Decision #7 | Medium | Explicit narrow overturn in proposal/design/skill; availability still degrades |
| Platform hook gaps → false security | High if overclaimed | Gate files/shell not MCP; document OpenCode/Cursor/pi gaps; evals + brief |
| bootstrap-ready path drift breaks activation detection | Medium | Fix docs; doctor/gate resolve cache marker the same way materialize writes it |
| Shell PR heuristic false positives/negatives | Medium | High-precision patterns; fail-open; mode=`warn` first |
| Live MCP evals pollute board | Medium | Notes-only scenarios first; optional MCP-live on disposable list + cleanup |
| Dual vocabulary (`trello_card_id` vs `trello.md`) confuses agents | Low–Med | Skill/spec alignment; one artifact |

## Rollback Plan

1. Revert recipe hook entries, `gate_mode` config, skill/brief/spec deltas, and
   doctor check.
2. Remove gate script + hermetic/live eval files (or leave tests disabled if
   mid-migrate).
3. Run `ai-specs sync` so generated shims drop managed hook ids.
4. No data migration; existing `trello.md` files remain harmless documentation.
5. Partial deploy is safe: `warn`/fail-open never wedges editors; projects that
   never re-sync keep old soft behavior.

## Dependencies

- Existing `trello-mcp-workflow` recipe (≥ v1.2.0) with `board_id` +
  `bootstrap-board` marker contract.
- Runtime hook distribution (`hooks-render.py`, `docs/runtime-hooks.md`) and
  plan-build / worktree gate precedents.
- Doctor `Check` / `Severity` pattern.
- Eval harness conventions + #165 worktree-flow live pattern
  (`eval_worktree_flow_live.py`, `run-live-worktree.sh`).
- Trello MCP available for **optional** MCP-live scenario only — not required
  for hermetic CI or notes-file goldens.

## Success Criteria

- [ ] Canonical artifact documented and referenced consistently as
      `openspec/changes/<slug>/trello.md` (`card_id` + `url` minimum); skill no
      longer treats `trello_card_id` as a real schema field.
- [ ] Broad skip hatch removed; narrow `tracker:none` documented and logged.
- [ ] Decision #7 narrowed in skill/design/spec: availability degrades; missing
      link artifact is enforceable under `always`.
- [ ] session-bootstrap requires tracker consult when tracker is bound for
      new/ambiguous changes.
- [ ] Doctor WARN fires for active changes missing valid `trello.md` when
      recipe+marker present; silent when recipe disabled.
- [ ] Gate inactive when recipe disabled, marker absent, or `gate_mode=off`.
- [ ] Gate never blocks `openspec/changes/**` writes.
- [ ] Gate `warn`: production write without card → exit 0 + stderr warning.
- [ ] Gate `always`: production write / locked PR-archive shell without card →
      exit 2 with remediation naming `trello.md`.
- [ ] Valid `trello.md` (or `tracker:none`) allows production writes.
- [ ] Dogfood `ai-specs.toml` sets `gate_mode = "warn"` explicitly.
- [ ] Hermetic tests cover explore cases: missing blocks (always), with card
      allows, openspec never blocked, mode off allows, shell PR case if shipped,
      doctor WARN.
- [ ] Live golden client + ≥3 notes-file scenarios land; MCP-live optional and
      isolated.
- [ ] Docs state residual gaps (OpenCode MCP/subagent, Cursor file-write, child
      processes) and that MCP interception is explicitly not used.
- [ ] Archives without `trello.md` are not migrated or failed.
- [ ] `./tests/validate.sh` passes.

## Planning depth

**Classification: `domain_change` → full chain** after this proposal:

1. `design.md` — gate algorithm, marker resolution, mode stamping, dual-hook
   matchers, `tracker:none` file shape, doctor scan rules, eval scenario
   contracts.
2. Delta specs under `openspec/changes/tracker-card-gate/specs/`
   (at least `trello-card-linking`; others as touched).
3. `tasks.md` — TDD-ordered phases (hermetic RED→GREEN before live).

Implementation remains `worktree_required: true`. This proposal artifact is
planning-only (no production apply in this step).

## Proposal assumptions (locked for design)

1. Hybrid from explore: recipe harden + doctor WARN + evals + **phased** hard
   gate — **not** MCP interception; **not** soft-only.
2. Semantic precedent is **plan-build-gate** (artifact before production), with
   worktree-gate lessons for dual-hook shell distribution and fail-open.
3. Dogfood ships in **`warn`**; promoting this repo to `always` is a later
   config flip after evals prove the contract.
4. Grandfather archives; enforcement targets **active** change folders only.
5. Generated harness shims are sync-only artifacts.

## Deferred items

| Item | Why deferred |
|------|----------------|
| Abstract tracker package (Jira/Linear/…) | v1 stays Trello-specific; keep swappable seams only |
| `.openspec.yaml` / schema field `trello_card_id` | Canonical file is `trello.md`; avoid dual sources |
| Doctor FAIL / pre-commit hard fail by default | WARN first; opt-in FAIL later if needed |
| Gate mode `always` as dogfood default | Promote after warn dogfood + evals green |
| Implementing deferred sync hooks as real sync MCP | Sync remains agent-less; runtime agents own linking |
| Auto-create card inside the gate script | Agents + MCP create; gate only checks artifact |
| Closing OpenCode/Cursor/pi platform hook gaps | Platform limits; brief + evals mitigate |
| MCP-live eval as required CI | Optional expensive scenario; notes-file goldens first |
| Retro mass backfill of archive `trello.md` | Grandfather; no migration |
| Re-audit trigger automation for rescued changes | Soft/eval coverage via `ac_retro_change_without_card_triggers_link`; no separate product surface in v1 |

## Artifact path

`openspec/changes/tracker-card-gate/proposal.md`
