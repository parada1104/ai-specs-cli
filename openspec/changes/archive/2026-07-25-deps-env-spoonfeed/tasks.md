# Tasks: deps-env-spoonfeed

Depth: full

Branch: `feat/deps-env-spoonfeed`  
Worktree: `.worktrees/deps-env-spoonfeed`  
Plan refs: `proposal.md`, `design.md`, specs under `specs/`

**Stop for human authorization before any production code implementation.**

---

## P0 — Planning gate (this PR slice / session)

- [x] `proposal.md`
- [x] `design.md` (env layout + opt-in install)
- [x] Spec deltas: `harness-env-scaffold`, `recipe-cli-deps`, `project-doctor`
- [x] `tasks.md` (this file)
- [x] Human authorization to implement

---

## P1 — Env scaffold core (TDD)

**Goal:** new layout APIs without wiring all call sites yet.

### P1.1 — `write_env` + `generate_env_example` (RED → GREEN)

- Create/reshape module (`env_scaffold.py` preferred; thin compat in
  `envrc-scaffold.py` if needed).
- `write_env(project_root, values)` → `ai-specs/.env` (dotenv, merge preserve extras).
- `generate_env_example(project_root)` → `ai-specs/.env.example` + `.bak` backup;
  deprecate primary `.envrc.example` (stub or stop writing).
- Tests in `tests/test_env_scaffold.py` (extend/rename from `test_envrc_scaffold.py`):
  - writes KEY=value not export
  - never writes project-root `.env`
  - never puts secrets in root `.envrc`
  - example includes ENV_VAR_HELP
  - merge preserves unrelated keys

### P1.2 — `ensure_root_envrc` merge-safe (RED → GREEN)

- Markers per design Decision A2.
- Tests: create missing; append without markers; replace marked region; idempotent;
  preserve custom lines.

### P1.3 — `migrate_legacy_envrc` (RED → GREEN)

- Parse export lines; merge; backup legacy; ensure root block.
- Tests: migrate; do not overwrite non-empty `.env` keys; no-op when absent.

### P1.4 — `offer_harness_env` orchestration unit (RED → GREEN)

- Soft-fail wrapper; calls migrate → prompt (mocked) → write → example → root →
  direnv_allow(root).
- Test soft-fail on prompt error (parity with hub-wizard-help).

**Verify:** `./tests/run.sh` green for new suite.

---

## P2 — Opt-in install (TDD)

**Goal:** `dep_install.py` + gate integration helpers.

### P2.1 — `resolve_install_plan` map (RED → GREEN)

- Map: gh, glab, jq, direnv, git; guidance-only npx, bb.
- brew when brew present; apt when apt-get present; else guidance.
- Tests: mock `which` / platform; assert argv; npx never brew node.

### P2.2 — `offer_and_install` (RED → GREEN)

- Non-TTY → no-op.
- TTY confirm default False; on Yes run subprocess; re-check.
- Tests: decline; accept (mocked run); guidance-only no run.

### P2.3 — Wire `_dep_gate` in `config_wizard.py` (RED → GREEN)

- Offer install then re-check then configure-anyway.
- Update `tests/test_config_wizard.py`.

**Verify:** `./tests/run.sh` green.

---

## P3 — Doctor diagnostics (TDD)

### P3.1 — direnv / managed envrc / harness-env checks

- Implement in `doctor.py` per `specs/project-doctor/spec.md`.
- Tests in `tests/test_doctor.py`:
  - WARN direnv when MCP env required
  - skip direnv WARN when no MCP env
  - WARN missing managed markers
  - WARN empty harness key without leaking secrets
  - doctor never calls install

**Verify:** `./tests/run.sh` green.

---

## P4 — Call-site wiring

### P4.1 — Replace `_offer_envrc` / init / recipe-add paths

- `config_wizard.py`, `init_tui.py`, `recipe-add.py` → `offer_harness_env`.
- direnv missing → offer install then allow.
- Update affected tests (`test_config_wizard`, `test_init_tui`, recipe-add if any).

### P4.2 — Gitignore / template

- Ensure `ai-specs/.env` ignored in project templates / docs.
- Dual-check root `.gitignore` patterns.

**Verify:** `./tests/run.sh` green.

---

## P5 — Docs + CHANGELOG

- Update `docs/ai-specs-toml.md` (remove “never writes `.envrc`”; document layout).
- Update `docs/recipe-schema.md` note: checks + TTY opt-in install (still no silent install).
- Vault / catalog READMEs pointing at `.envrc` exports → point to `ai-specs/.env` + root managed block.
- `CHANGELOG.md` under Unreleased / next version: env layout fix + opt-in install.

**Verify:** `./tests/validate.sh` green.

---

## P6 — Close-out (post-implement)

- [x] Full `./tests/validate.sh` (re-verify 2026-07-25: 1086/1086 OK — see verify-report.md)
- [ ] Commit planning + implementation on `feat/deps-env-spoonfeed` (parent owns)
- [ ] PR to `development` (only after planning files committed + impl verified; parent owns)
- [x] Archive change folder on review branch before merge

---

## File touch checklist (implement phase)

| File | Action |
|------|--------|
| `lib/_internal/env_scaffold.py` | New (or reshape) |
| `lib/_internal/envrc-scaffold.py` | Compat shim or delete after migrate imports |
| `lib/_internal/dep_install.py` | New |
| `lib/_internal/dep_check.py` | Minor helpers if needed |
| `lib/_internal/config_wizard.py` | Gate + env offer |
| `lib/_internal/init_tui.py` | Env offer |
| `lib/_internal/recipe-add.py` | Env offer |
| `lib/_internal/doctor.py` | New checks |
| `tests/test_env_scaffold.py` | New/rename |
| `tests/test_dep_install.py` | New |
| `tests/test_doctor.py` | Extend |
| `tests/test_config_wizard.py` | Extend |
| `docs/ai-specs-toml.md`, `docs/recipe-schema.md`, catalog READMEs | Docs |
| `CHANGELOG.md` | Notes |
| `.gitignore` / templates | `ai-specs/.env` |

---

## Authorization checkpoint

**Status: AUTHORIZED + IMPLEMENTED** — P1–P5 landed on `feat/deps-env-spoonfeed`.
Await commit / PR when requested.
