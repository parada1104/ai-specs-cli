# Proposal: Deps opt-in install + envrc spoon-feed

## Why (motivation)

Recipe CLI prerequisites (`gh`, `glab`, `jq`, `npx`, `git`, `bb`) are declared in
`[[deps.cli]]` and checked by `dep_check.py` / doctor, but the tool never offers to
install them — users get a WARN and an `install_url`. Separately, MCP env setup for
agents is broken in practice:

1. Interactive flows write secrets to `ai-specs/.envrc`.
2. `direnv allow` is invoked on the **project root**.
3. direnv only loads a `.envrc` in the current directory tree — so from the repo root
   the harness file is invisible.
4. Docs still claim the tool never writes `.envrc` (drift from CHANGELOG 0.12.4+).
5. App `.env` and harness secrets get mixed when users improvise a single file.
6. `direnv` itself is not a first-class dependency.

Agents (Cursor/Claude/IDE) resolve MCP `$VAR` from the process environment. A correct
direnv layout is the supported shell path; IDEs without a direnv hook remain a
documented gap (no secrets in MCP JSON).

## Intent

1. **Spoon-feed env layout** so the user only supplies values and runs (or accepts)
   `direnv allow`.
2. **Opt-in install** of missing system CLIs on TTY via brew/apt — never silent
   auto-install; non-TTY stays check-only.

## Scope (in)

1. **Harness env files**
   - Write `ai-specs/.env` (gitignored) with MCP var values from the wizard.
   - Generate committed `ai-specs/.env.example` (names + help; deprecate
     `ai-specs/.envrc.example` as the primary template).
   - Ensure project-root `.envrc` contains a merge-safe managed block:

     ```bash
     # managed-by: ai-specs (do not remove block)
     dotenv_if_exists .env
     dotenv_if_exists ai-specs/.env
     # end managed-by: ai-specs
     ```

   - Never modify the application's root `.env`.
   - Migrate existing `ai-specs/.envrc` → `ai-specs/.env` when present.
   - `direnv allow` on project root after scaffold.
   - Treat `direnv` as a checkable CLI dep with opt-in install offer.

2. **Opt-in CLI install**
   - New `dep_install.py`: resolve brew/apt command from a binary→package map;
     prompt per missing required binary on TTY; run only after explicit yes.
   - Wire into configure-recipes / init / recipe-add dep gates.
   - Non-TTY / CI: no prompt, no install (doctor WARN only).
   - `npx` / Node: guidance + strong confirmation, no blind `brew install node`.

3. **Doctor**
   - WARN: missing direnv; missing managed root `.envrc` block when MCP env vars
     required; missing/empty required keys in `ai-specs/.env`.
   - Keep existing recipe-dep WARN rows; guidance may mention opt-in install path.

4. **Docs + CHANGELOG** — sync `docs/ai-specs-toml.md`, recipe-schema notes, vault
   README pointers; record behavior change.

5. **Tests** — strict TDD via `./tests/run.sh` / `./tests/validate.sh`.

## Scope (out)

- Silent auto-install of any system binary.
- Writing secrets into `.cursor/mcp.json`, `.mcp.json`, or other agent MCP configs.
- Managing contents of the application's own `.env`.
- Replacing direnv with dotenvx / 1Password / other substrates.
- New `ai-specs env print|check` CLI (deferred; may appear as follow-up).
- Changing TUI Python vendor policy (`rich` / `questionary` / `ensure_deps`).

## Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| `harness-env-scaffold` | **New** | Root `.envrc` managed block, `ai-specs/.env` + `.env.example`, migration, direnv allow |
| `recipe-cli-deps` | **New** (formalize) | Check + TTY opt-in install for `[[deps.cli]]` (and direnv) |
| `project-doctor` | **Modified** | Env layout + direnv + missing harness env diagnostics |

## Impact (modules)

| Area | Change |
|------|--------|
| `lib/_internal/envrc-scaffold.py` | Reshape → env scaffold (`write_env`, `ensure_root_envrc`, migrate) |
| `lib/_internal/dep_install.py` | **New** — offer/run install |
| `lib/_internal/dep_check.py` | Keep checks; may expose helpers for re-check after install |
| `lib/_internal/config_wizard.py` | Dep gate offers install; `_offer_envrc` → new layout |
| `lib/_internal/init_tui.py`, `recipe-add.py` | Same env + install wiring |
| `lib/_internal/doctor.py` | New WARN checks |
| Catalog / doctor global | `direnv` as declared or global dep |
| Docs, CHANGELOG, `.gitignore` if needed for `ai-specs/.env` | Sync |

## Decisions already locked

- Deliverable tier: **Full** planning chain; implement only after human authorization.
- System CLIs: **TTY opt-in** brew/apt (never forced).
- Substrate: **direnv** retained.

## Rollback

- Revert the change branch / PR.
- Users with new layout keep working (root `.envrc` + `ai-specs/.env` are additive).
- Legacy `ai-specs/.envrc` can remain unread after migration; restore from git history
  if a project never migrated.

## Success criteria

1. From repo root with direnv hooked, `cd` into project loads harness + app env without
   hand-written exports of MCP secrets in root `.envrc`.
2. Wizard prompts for values once; writes `ai-specs/.env`; ensures managed root block;
   attempts `direnv allow`.
3. Missing `gh` (etc.) on TTY offers `brew install …` / apt command; declining leaves
   check-only behavior.
4. Non-TTY doctor never installs anything.
5. Docs match code (no “never writes `.envrc`” lie).
