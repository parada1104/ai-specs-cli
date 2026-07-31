# Tasks: worktree-flow-repo-topology

Depth: full

Branch / worktree: `feat/worktree-flow-repo-topology` (or current change branch) under
`.worktrees/worktree-flow-repo-topology/`

Plan refs: `proposal.md`, `design.md`, `specs/worktree-flow/spec.md`

**Stop for human authorization before production-code apply.** This file is the
implementation plan only — do not write production code or tests while authoring
it.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~700–1100 (incl. tests) |
| 400-line budget risk | High (tests dominate; consider chaining) |
| Chained PRs recommended | Yes if impl+tests exceed ~400 reviewable LOC |
| Suggested split | PR1 helper+schema+cleanup+WARN → PR2 surfaces (wizard/hub/brief/doctor) → PR3 docs |
| Delivery strategy | auto-chain if over budget; else single PR |
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
| 1 | Shared helper + recipe schema + cleanup loop + stale WARN | PR 1 | Core behavior; largest test surface |
| 2 | Wizard / hub / brief / doctor surfacing | PR 2 | Thin call sites of the helper |
| 3 | Docs + recipe README + final validate | PR 3 | Docs-only if Unit 1+2 already land |

---

## Planning depth

- **Classification**: `domain_change` (explore → proposal → design → delta spec → tasks).
- **Delta coverage**: 5 ADDED + 2 MODIFIED requirements in
  `specs/worktree-flow/spec.md` — every phase below cites the requirement and
  scenario(s) it closes.
- **Accepted baselines** (from design.md):
  - One Python helper in `lib/_internal/util.py`; bash mirror only inside
    `worktree-cleanup.sh` (`condition = "not_exists"`).
  - `auto` never resolves to `monorepo-apps`.
  - Gate decision logic unchanged (optional message-only hint out of critical
    path).
  - Stale `not_exists` → WARN, never overwrite.
- **Authorization**: PENDING until maintainer green-lights apply.

## Non-goals (apply MUST NOT)

- Stamp `repo_topology` into `worktree-gate.sh` or the cleanup template.
- Change gate allow/deny decisions.
- Nested/`--recursive` submodules; per-submodule `integration_branch`.
- Reuse `project.subrepos` as submodule source of truth.
- Support per-submodule `.worktrees/` layouts.
- Silently overwrite consumer `not_exists` cleanup overrides.
- Rewrite merge helpers (`is_merged`, `resolve_base_candidates`, etc.) to plumb
  `git -C` — wrap in `_cleanup_one` + outer loop only (design §3 / #7).
- Edit `proposal.md` / `design.md` / `specs/` during apply unless a blocking
  contradiction is found (then stop and ask).

---

## Implementation (red-green-refactor)

Phases follow design.md **File Changes** dependency order and close the delta’s
7 requirements. Each task is one focused TDD cycle unless marked docs-only or
prose-verification.

### Phase 1 — Shared fixture + `util.py` helpers (foundation)

**Files:** `lib/_internal/util.py`; new or extended tests (prefer
`tests/test_repo_topology.py`; may extend `tests/test_util.py` only if keeping
stdlib helpers co-located — topology suite is large enough for its own file).
**Fixture:** one reusable submodule temp-repo builder mirroring
`tests/test_worktree_cleanup.py` (`TemporaryDirectory` + `git init` + second
local repo via `git submodule add`); must support initialized checkouts and an
uninitialized `-` status entry.
**Reqs:** ADDED *Auto Topology Detection*; ADDED *Repo Topology Configuration*
(explicit-bypass scenarios); ADDED *Stale Cleanup Override Detection* (comparator
unit only).

- [x] 1.1 RED: add shared submodule fixture builder used by helper + cleanup
      tests (e.g. `_make_super_with_submodule(tmp, *, initialized=True,
      path="apps/api", name=...)` plus optional second path / uninitialized
      entry). Assert fixture produces `.gitmodules` and the expected
      `git submodule status` prefix (`' '` vs `'-'`). No production code yet.
- [x] 1.2 RED: `detect_submodules` unit tests — (a) prefixes `' '`, `'+'`, `'U'`
      count as initialized; (b) `'-'` skipped; (c) no `.gitmodules` →
      `(False, ())`; (d) path registered in `.gitmodules` but missing from
      status ignored; (e) name≠path still returns the **path**. Satisfies Auto
      Topology Detection scenarios *Initialized submodules…*, *Only
      uninitialized…*, *No gitmodules…*.
- [x] 1.3 RED: `resolve_repo_topology` unit tests — `auto`+initialized →
      `monorepo-submodules` / `via="auto"`; `auto`+only `-` or no gitmodules →
      `standalone`; `auto` never returns `monorepo-apps`; explicit
      `standalone` / `monorepo-apps` / `monorepo-submodules` bypass detection
      (`via="config"`, empty subs for standalone/apps); absent/empty
      `config_value` treated as `auto`; git-missing / not-a-repo degrades to
      `standalone` without raising. Satisfies Auto Topology Detection (*all
      four scenarios*) + Repo Topology Configuration (*Explicit standalone /
      monorepo-apps / monorepo-submodules bypasses detection*).
- [x] 1.4 RED: `override_is_stale` unit tests — missing dest → `False`; missing
      catalog src → `False`; identical bytes → `False`; divergent bytes →
      `True` (content/sha256, not mtime). Satisfies Stale Cleanup Override
      Detection comparator contract (design §6).
- [x] 1.5 GREEN: implement in `lib/_internal/util.py` (stdlib + `subprocess`
      only; no `toml-read`/rich): frozen `@dataclass TopologyResolution`;
      `detect_submodules`; `resolve_repo_topology`; `override_is_stale` — match
      design §1 / §6 signatures exactly. Make 1.2–1.4 pass via `./tests/run.sh`
      on the new file.
- [x] 1.6 REFACTOR: keep git config path parsing (`git config -f .gitmodules
      --get-regexp`) and status parsing in small private helpers; confirm
      util.py still imports without third-party deps (existing
      `tests/test_util.py` import guard remains green).

### Phase 2 — Recipe schema `[config.repo_topology]`

**Files:** `catalog/recipes/worktree-flow/recipe.toml` (add enum + version bump);
`tests/test_worktree_flow_recipe.py` (sibling of `test_sync_rejects_invalid_gate_mode`).
**Reqs:** ADDED *Repo Topology Configuration* (Default when unset / Invalid enum
rejected / materialize still succeeds with default).

- [x] 2.1 RED: extend `tests/test_worktree_flow_recipe.py` with
      `test_sync_defaults_repo_topology_to_auto` (manifest omits key → resolved
      config is `auto`, sync succeeds) and
      `test_sync_rejects_invalid_repo_topology` (`repo_topology = "nested"` →
      non-zero; stderr names `nested` and lists
      `auto | standalone | monorepo-apps | monorepo-submodules`) — mirror
      `test_sync_rejects_invalid_gate_mode`.
- [x] 2.2 RED: `test_sync_materializes_with_repo_topology_default` (or extend
      existing materialize assertion) — with no override, sync still
      materializes skill/commands/cleanup template; config block may omit the
      key (default applied by `merge_config`) or write `auto` if the test
      asserts stamped manifest output. Covers Default + materialize path.
- [x] 2.3 GREEN: add to `catalog/recipes/worktree-flow/recipe.toml`:
      ```toml
      [config.repo_topology]
      type = "string"
      required = false
      default = "auto"
      enum = ["auto", "standalone", "monorepo-apps", "monorepo-submodules"]
      help_text = "…"  # auto-detect vs explicit; monorepo-apps naming-only
      ```
      Bump recipe `version`. Rely on existing `merge_config()` enum reject — no
      new stamp plumbing for this key. Pass 2.1–2.2.

### Phase 3 — Prose contracts: `/worktree-new`, `/worktree-clean`, SKILL, which-repo brief

**Files:** `catalog/recipes/worktree-flow/commands/worktree-new.md`,
`commands/worktree-clean.md`, `skills/worktree-flow/SKILL.md`,
`recipe.toml` `[provides.brief].workflow_rules`.
**Reqs:** ADDED *Submodule Worktree Creation Contract* (all scenarios);
MODIFIED *Pre-delegation worktree/branch check…* (*Brief rule present* +
*Brief requires which-repo check…*); cleanup command docs for MODIFIED
*worktree-cleanup.sh submodule enumeration*.

**Verification honesty (commands / skills):** recipe commands and skills are
**byte-copied** by materialize; nothing in the test suite executes the `.md` as
code. Live `git worktree add` under a real submodule monorepo is
**agent-behavior / manual smoke only** (do not invent a fake runner). What *is*
automatable: (1) doc-content assertion tests that the catalog prose contains the
locked contract phrases; (2) recipe.toml `workflow_rules` string assertions
(already pattern-matched elsewhere for brief fragments).

- [x] 3.1 PROSE: update `commands/worktree-new.md` per design §2–3 — `<subrepo>`
      require/infer only under resolved `monorepo-submodules`; cwd inference via
      `rev-parse --show-toplevel` (primary path + longest `<path>-` prefix under
      `worktrees_dir`); explicit vs inferred mismatch hard-error; path-then-unique-name
      validation; reject uninitialized/unknown/ambiguous; locked create:
      `git -C <subrepo_path> worktree add <absolute-super>/<worktrees_dir>/<subrepo>-<slug>
      -b <branch> <integration_branch>`; standalone/apps keep today’s single-repo
      command. Closes Creation Contract scenarios (inference, longest-prefix,
      path/name validation, mismatch, uninitialized, unknown, ambiguous).
- [x] 3.2 RED (doc-content only): extend `tests/test_worktree_flow_recipe.py`
      (or small dedicated test) asserting materialized/catalog `worktree-new.md`
      contains: `git -C`, absolute destination under `worktrees_dir`,
      `<subrepo>-<slug>`, longest-prefix / `show-toplevel` inference notes, and
      rejection guidance for uninitialized (`submodule update --init`). **Not** a
      runtime create test — document that end-to-end create remains manual/agent
      verification. GREEN by landing 3.1 content.
- [x] 3.3 PROSE: update `commands/worktree-clean.md` — optional `--submodule` /
      `--subrepo` scope (default = all initialized); under submodules enumerate
      via per-module `git -C` / `submodule foreach`, never superproject
      `worktree list` alone; standalone/apps unchanged. Closes MODIFIED cleanup
      requirement (docs side).
- [x] 3.4 PROSE: update `skills/worktree-flow/SKILL.md` — per-topology
      create/clean matrix; `git -C` absolute-destination contract; strengthen
      pre-delegation to verify *which git repository* (`rev-parse --show-toplevel`)
      under `monorepo-submodules`, not only branch + worktree list.
- [x] 3.5 RED→GREEN: extend `workflow_rules` in `recipe.toml` so under
      monorepo-submodules the always-on rule requires which-repo verification via
      `rev-parse --show-toplevel` (keep existing “hooks are not the sole guard”
      language). Add/extend a recipe test asserting the rule text mentions
      which-repo / `show-toplevel` (MODIFIED *Brief rule present in recipe
      declaration* + *Brief requires which-repo check under monorepo-submodules*).
      Content assertion on catalog strings — not agent runtime.

### Phase 4 — `worktree-cleanup.sh` submodule loop + standalone regression

**Files:** `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`;
`tests/test_worktree_cleanup.py` (reuse Phase 1 fixture).
**Reqs:** MODIFIED *worktree-cleanup.sh submodule enumeration* (all five
scenarios); preserve main-spec Positive Base Candidate Resolution behavior on
the standalone path.

- [x] 4.1 RED: using the shared submodule fixture, add cleanup tests:
      (a) merged feature wt at `<super>/<worktrees_dir>/<module>-feat-done`
      reported `would remove` when scanned from super root (*Merged feature
      worktree under one submodule…*);
      (b) two initialized modules each owning a linked wt → both scanned with
      default scope (*Worktrees under multiple submodules…*);
      (c) `--submodule <path>` / `--subrepo` limits to one module (*Scoped
      --submodule flag…*);
      (d) uninitialized `-` module is not `git -C`’d (*Uninitialized submodules
      are skipped*). Assert shared `WT_PREFIX` is super `worktrees_dir`.
- [x] 4.2 GREEN: restructure template per design #7 — add `--submodule` /
      `--subrepo` (repeatable) to flag parser; `enumerate_modules`,
      `_cleanup_one` (wrap today’s scan→`flush` block unchanged), `_in_scope`;
      outer loop `cd`’s each module with shared `WT_PREFIX=$SUPER_ROOT/$WORKTREES_DIR/`;
      bash topology self-detect mirrors Python (`gitmodules` + non-`-` status).
      Leave `flush` / `is_merged` / `resolve_base_candidates` /
      `candidate_has_*` / `WORKTREE_CLEANUP_SOURCE_ONLY` byte-identical.
      Pass 4.1.
- [x] 4.3 REGRESSION GUARD (must stay green after 4.2): re-run the existing
      standalone merge-detection suite in `tests/test_worktree_cleanup.py` and
      assert the **7 Positive Base Candidate Resolution scenarios** from
      `openspec/specs/worktree-flow/spec.md` still pass with unchanged output
      lines (`would remove` / `skipped … (unmerged|dirty)`):
      1. Regular merge on origin/base with stale local base
      2. Squash merge still resolves by patch-id
      3. Rebase merge still resolves by patch-id
      4. Fast-forward merge remains merged
      5. Local-only branch with no match stays unmerged
      6. Branch ahead of base stays unmerged
      7. Remote-deleted branch still merges from local base
      Also keep dirty-skip + bounded no-fetch + dual-remote safety tests green.
      Closes *Standalone repo cleanup unchanged* (byte-for-byte single-pass when
      no `.gitmodules`).
- [x] 4.4 REFACTOR / sanity: `--submodule` on a standalone repo is inert (single
      sentinel pass, not an error); confirm `bash -n` on the template; no
      accidental edits to merge-helper function bodies (diff review).

### Phase 5 — Sync stale `not_exists` override WARN

**Files:** `lib/_internal/recipe-materialize.py` (`materialize_template`
not_exists branch); `tests/test_recipe_materialize.py` (hand-edited override).
**Reqs:** ADDED *Stale Cleanup Override Detection* (all three scenarios).
Depends on 1.4/1.5 (`override_is_stale`).

- [x] 5.1 RED: tests for worktree-flow cleanup template target
      `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`:
      (a) identical override → sync succeeds, **no** stale WARN, file untouched
      (*Unmodified override produces no warning*);
      (b) divergent override → non-blocking WARN naming path + refresh
      (`rm <target> && ai-specs sync`), exit 0, bytes unchanged (*Diverged
      override warns and sync succeeds*);
      (c) missing target → normal fresh copy, no stale WARN (*Missing override
      gets a fresh copy*).
- [x] 5.2 GREEN: in `materialize_template`, inside existing
      `condition == "not_exists"` + `dest.exists()` branch, call
      `util.override_is_stale(src, dest)` and `warn(...)` with the design §6
      message; **never** overwrite. Pass 5.1.

### Phase 6 — Doctor mirror (in scope as optional surface; include)

Design marks doctor optional; proposal keeps sync WARN primary and doctor as
mirror — **include** both checks so surfaces stay consistent.

**Files:** `lib/_internal/doctor.py`; `tests/test_doctor.py`.
**Reqs:** ADDED *Stale Cleanup Override Detection* (doctor mirror); ADDED
*Topology Surfacing* (informational echo only — not a blocking scenario).

- [x] 6.1 RED: `_check_repo_topology` — INFO (or OK) line echoing resolved
      topology + initialized submodule count via `resolve_repo_topology`;
      non-blocking. `_check_stale_template_overrides` — WARN when enabled
      recipe `not_exists` template dest is stale via shared
      `override_is_stale`; no WARN when identical/missing. Doctor never
      overwrites files (`test_doctor_is_read_only` remains true).
- [x] 6.2 GREEN: implement both checks; wire into doctor’s check list via
      existing sibling-load of `util`. Pass 6.1.

### Phase 7 — Wizard integration

**Files:** `lib/_internal/init_tui.py`; `tests/test_init_tui.py`.
**Reqs:** ADDED *Topology Surfacing* (*Wizard proposes auto-detected default
and accepts override*). `config_wizard` needs no special-case (schema enum
select already works once Phase 2 lands) — smoke optional only.

- [x] 7.1 RED: extend `tests/test_init_tui.py` — after project-name prompt,
      topology select default is `auto` with detected label (e.g. fixture repo
      resolving to `monorepo-submodules`); user override to `standalone` writes
      `recipes.worktree-flow.config.repo_topology = "standalone"` into staged
      manifest via `_render_manifest` / wizard `configured` map when
      worktree-flow is enabled. Mock `questionary` like existing wizard tests.
- [x] 7.2 GREEN: in `run_wizard`, immediately after “Project name:” (~L241),
      call `_util.resolve_repo_topology(target, "auto")`, present hub-style
      select over `auto|standalone|monorepo-apps|monorepo-submodules` with
      default `auto` (label shows detected), thread into `configured` so
      `_render_manifest` emits `[recipes.worktree-flow.config] repo_topology = …`.
      Pass 7.1. Drop answer when worktree-flow is not among selected recipes
      (design §4).

### Phase 8 — Hub / status surfacing

**Files:** `lib/_internal/hub.py`; `tests/test_hub.py`.
**Reqs:** ADDED *Topology Surfacing* (*Hub panel…* + *Noninteractive status…*).

- [x] 8.1 RED: `StatusSummary` gains `topology` + `topology_via`;
      `status_summary()` reads manifest `repo_topology` and calls
      `resolve_repo_topology`; `StatusPanel.render` adds one grid row
      (`topology  monorepo-submodules (auto→…)` / equivalent);
      `_run_noninteractive` prints `topology: {resolved} ({via})`. Tests:
      auto→monorepo-submodules shows resolved + via auto; explicit standalone
      shows via config.
- [x] 8.2 GREEN: implement hub wiring (`_util` already loaded). Pass 8.1.

### Phase 9 — Agent brief `_section_project` surfacing

**Files:** `lib/_internal/agents-render.py`; prefer
`tests/test_agents_render_brief_fragments.py` and/or the sync pipeline assertion
pattern that already checks `- **Integration branch**:`.
**Reqs:** ADDED *Topology Surfacing* (*Brief Project section includes resolved
topology*).

- [x] 9.1 RED: assert Project section includes
      `- **Repo topology**: \`<resolved>\` (via <config|auto>)` (or equivalent
      provenance) when worktree-flow is enabled and topology resolves via auto
      to `monorepo-submodules`.
- [x] 9.2 GREEN: in `_section_project`, after the integration_branch block
      (~L211–212), read `recipes.worktree-flow.config.repo_topology` from
      `resolved`, call shared helper, append the Repo topology line. Load
      `util` via existing sibling-load stanza. Pass 9.1.

### Phase 10 — Docs (lower priority; near end)

**Files:** `catalog/recipes/worktree-flow/README.md`, `docs/recipes-catalog.md`,
`docs/ai-specs-toml.md`. Optional: commented example in
`templates/ai-specs.toml.tmpl`; optional message-only hint in
`hooks/worktree-gate.sh` (skip unless cheap — no decision change).

- [x] 10.1 Update recipe `README.md` — topology table; `repo_topology` config
      row; shared `<worktrees_dir>/<subrepo>-<slug>` layout; stale-override
      refresh (`rm … && ai-specs sync`); `monorepo-apps` naming-only note.
- [x] 10.2 Update `docs/recipes-catalog.md` — worktree-flow topology /
      layout / cleanup enumeration notes.
- [x] 10.3 Update `docs/ai-specs-toml.md` — document
      `recipes.worktree-flow.config.repo_topology` enum + default `auto`.
- [x] 10.4 OPTIONAL: commented `repo_topology` example in
      `templates/ai-specs.toml.tmpl`; OPTIONAL gate stderr hint naming
      `/worktree-new <subrepo>` only if it stays message-only (no test change
      required unless `test_worktree_gate_hook.py` asserts exact stderr).

### Phase 11 — Final validation gate

- [x] 11.1 Run focused suites green:
      `tests/test_repo_topology.py` (or chosen helper file),
      `tests/test_worktree_flow_recipe.py`,
      `tests/test_worktree_cleanup.py`,
      `tests/test_recipe_materialize.py`,
      `tests/test_doctor.py`,
      `tests/test_init_tui.py`,
      `tests/test_hub.py`,
      agents-render brief tests — via `./tests/run.sh` on those paths.
- [x] 11.2 Cross-check every delta scenario (5 ADDED + 2 MODIFIED) has a
      RED→GREEN task or an explicit prose/manual verification note (Creation
      Contract live create = manual/agent only; doc-content tests cover the
      catalog contract).
- [x] 11.3 FINAL GATE: from the change worktree root run `./tests/validate.sh`
      (py_compile + `bash -n` + full tests per Useful Commands). Fix drift
      until exit 0. Change is ready for apply/verify only after this passes.

---

## File touch checklist (implement phase)

| File | Action |
|------|--------|
| `lib/_internal/util.py` | Modify — `TopologyResolution`, `detect_submodules`, `resolve_repo_topology`, `override_is_stale` |
| `catalog/recipes/worktree-flow/recipe.toml` | Modify — `[config.repo_topology]`; which-repo `workflow_rules`; version bump |
| `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` | Modify — submodule loop + flags |
| `catalog/recipes/worktree-flow/commands/worktree-new.md` | Modify — `<subrepo>` + `git -C` contract |
| `catalog/recipes/worktree-flow/commands/worktree-clean.md` | Modify — scope + enumeration docs |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Modify — topology matrix + which-repo |
| `lib/_internal/recipe-materialize.py` | Modify — stale-override WARN plug point |
| `lib/_internal/doctor.py` | Modify — `_check_repo_topology`, `_check_stale_template_overrides` |
| `lib/_internal/init_tui.py` | Modify — topology question after project name |
| `lib/_internal/hub.py` | Modify — `StatusSummary` + panel + noninteractive |
| `lib/_internal/agents-render.py` | Modify — `_section_project` Repo topology line |
| `catalog/recipes/worktree-flow/README.md` | Modify — docs |
| `docs/recipes-catalog.md` | Modify — docs |
| `docs/ai-specs-toml.md` | Modify — docs |
| `tests/test_repo_topology.py` (new) or `tests/test_util.py` | Add helper unit tests + submodule fixture builder |
| `tests/test_worktree_flow_recipe.py` | Extend — enum + doc-content + workflow_rules |
| `tests/test_worktree_cleanup.py` | Extend — submodule loop + standalone regression |
| `tests/test_recipe_materialize.py` | Extend — stale WARN paths |
| `tests/test_doctor.py` | Extend — topology INFO + stale WARN |
| `tests/test_init_tui.py` | Extend — wizard topology node |
| `tests/test_hub.py` | Extend — status topology row/line |
| `tests/test_agents_render_brief_fragments.py` (and/or sync pipeline) | Extend — brief Repo topology line |
| `templates/ai-specs.toml.tmpl` | Optional |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Optional message-only |

---

## Requirement → phase map

| Delta requirement | Kind | Phases |
|-------------------|------|--------|
| Repo Topology Configuration | ADDED | 1 (explicit bypass), 2 |
| Auto Topology Detection | ADDED | 1 |
| Submodule Worktree Creation Contract | ADDED | 3 (prose + doc-content; live create = manual) |
| Stale Cleanup Override Detection | ADDED | 1 (comparator), 5 (sync), 6 (doctor) |
| Topology Surfacing | ADDED | 6 (info), 7, 8, 9 |
| worktree-cleanup.sh submodule enumeration | MODIFIED | 3 (docs), 4 |
| Pre-delegation which-repo brief check | MODIFIED | 3.4–3.5 |

---

## Authorization checkpoint

**Status: PLANNING COMPLETE — apply NOT authorized by this artifact alone.**
Await maintainer go-ahead before RED/GREEN implementation.
