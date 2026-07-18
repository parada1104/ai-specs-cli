# Tasks: agent-cli-literacy

Depth: **full**

Source explore: `openspec/changes/agent-cli-literacy/explore.md` (Engram `#1356`)
Source proposal: `openspec/changes/agent-cli-literacy/proposal.md`
Source design: `openspec/changes/agent-cli-literacy/design.md`
Source specs:
- `openspec/changes/agent-cli-literacy/specs/harness-cli-literacy/spec.md`
- `openspec/changes/agent-cli-literacy/specs/runtime-brief-rendering/spec.md`

Execution mode: **strict TDD**. Phase 1 MUST show RED before Phase 2 content/render
changes land.

Legend: `[P]` = can run in parallel with sibling `[P]` tasks in the same phase.
Unmarked tasks are sequential within the phase.

Tracker: Trello https://trello.com/c/FjH6H1Ae (card 43)

Defaults locked at auth: Useful Commands pointer; configure-recipes in lifecycle+recipes;
rules-audit out of primary literacy v1.

---

## Phase 1 — Tests scaffolding (RED)

- [x] **T1.1** — Create `tests/test_harness_cli_literacy.py` scaffold with ROOT,
  paths to `bundled-skills/`, `agents-render` loader helpers, and unittest
  conventions matching nearby bundled-skill tests.
  **Done when:** file collects; no false-green for missing skills.

- [x] **T1.2** — RED: `test_bundled_harness_skills_exist` asserts
  `bundled-skills/harness-{lifecycle,recipes,skills-deps}/SKILL.md` exist.
  **Req:** Always-on bundled literacy skills.

- [x] **T1.3** — RED: `test_harness_skills_frontmatter_valid` loads each SKILL.md
  through `skill_contract` (or project helper) and asserts `scope` includes
  `root` + non-empty `auto_invoke`.
  **Req:** Frontmatter is sync-valid.

- [x] **T1.4** — RED: `test_refresh_bundled_ships_harness_skills` builds a tmp
  project, runs refresh-bundled, asserts the three skills land under
  `ai-specs/skills/`.
  **Req:** Refresh-bundled materializes literacy skills.

- [x] **T1.5** — RED: `test_agents_render_emits_harness_literacy_pointer` renders
  a minimal manifest/resolved-config and asserts the pointer needle
  (`harness-lifecycle`, `harness-recipes`, `harness-skills-deps`) appears even
  with empty useful_commands / no recipes.
  **Req:** Always-on harness literacy pointer bullet.

- [x] **T1.6** — RED: `test_harness_skill_commands_match_cli_help` parses
  `ai-specs <cmd>` tokens from the three SKILL.md files and asserts membership
  in the public command set from `bin/ai-specs` help (allow documented aliases
  like `add-dep`).
  **Req:** Playbook commands stay aligned with the public CLI.

- [x] **T1.7** — Confirm Phase 1 RED evidence (failing for missing skills/pointer,
  not import/syntax errors). Record command + summary in this file or verify notes.

RED evidence: `python3 -m unittest tests.test_harness_cli_literacy -v` → 4 FAIL + 1 ERROR
(missing SKILL.md / missing Useful Commands pointer).

---

## Phase 2 — Literacy skills (GREEN)

- [x] **T2.1** — Author `bundled-skills/harness-lifecycle/SKILL.md` with
  frontmatter + playbook for init/sync/sync-agent/refresh-bundled/doctor/upgrade/hub
  (+ configure-recipes as lifecycle step) and path footnote.
  **Req:** Domain coverage (lifecycle).

- [x] **T2.2** `[P]` — Author `bundled-skills/harness-recipes/SKILL.md` covering
  recipe list/add/init + configure-recipes and add→configure→sync order.
  **Req:** Domain coverage (recipes).

- [x] **T2.3** `[P]` — Author `bundled-skills/harness-skills-deps/SKILL.md`
  covering local skill posture (link skill-creator), skills add/list/remove,
  add-dep, link skill-sync.
  **Req:** Domain coverage (skills/deps).

- [x] **T2.4** — Re-run Phase 1 skill existence/frontmatter/refresh/command-alignment
  tests; leave pointer test RED if still failing.

---

## Phase 3 — Brief pointer (GREEN)

- [x] **T3.1** — Extend `lib/_internal/agents-render.py` to always emit the fixed
  harness literacy pointer bullet (default: `## Useful Commands`), respecting
  `--preserve-if-runtime-brief`.
  **Req:** runtime-brief-rendering delta.

- [x] **T3.2** — GREEN: pointer + full `tests/test_harness_cli_literacy.py` suite
  PASS; fix any agents-render regressions.

---

## Phase 4 — Docs + validation

- [x] **T4.1** `[P]` — Optional light note in `docs/skills-by-agent.md` about
  harness literacy skills + pointer for non-auto-invoke runtimes.

- [x] **T4.2** — Run `skill-sync` / metadata validation over the new skills
  (covered by `test_harness_skills_frontmatter_valid` via `skill_contract`).

- [x] **T4.3** — Run `./tests/validate.sh` (or project full suite) and record
  evidence; leave suite green.

GREEN evidence: `./tests/validate.sh` exit 0 (975+ tests; doctor fixtures updated
to use dynamic `bundled_skill_names()`).

- [x] **T4.4** — Update Trello card 43 with plan-complete / ready-for-build status
  after human authorization (do not implement before auth).

---

## Authorization gate

Authorized by user ("sí vamos con los default"). Implementation complete pending
commit/PR on request.
