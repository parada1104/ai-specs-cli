# Judgment Day — `baseline-hub-recipe-flow`

## Target identity

- Repository: `ai-specs-cli`
- Worktree: `.worktrees/baseline-hub-recipe-flow`
- Branch: `fix/baseline-hub-recipe-flow`
- HEAD: `da16febf1a4af09aed55947b2ac2f983e91f073e`
- Base: `development` at `cf09e1d4cd1ca5d24cb94cadda089fe809158602`
- Diff range: `git diff development..HEAD`
- `target_identity`: `sha256:5d1e8f9d8b543c3aa8c472fb0ca33ae07945ef1eb38b1485314d0993f882f4d1`

## Method

- Blind dual adversarial review: `jd-judge-a` and `jd-judge-b`, launched with
  identical scope, criteria, and skill paths, without either judge seeing the
  other's findings.
- Judges were read-only and returned one neutral findings result each.
- No refuter was launched; two-judge agreement is the corroboration mechanism.
- Severity scale: `CRITICAL` (severe) / `WARNING` / `SUGGESTION`. Only severe
  findings confirmed by both judges are eligible for a bounded fix round.

## Frozen ledger — round 1

### Confirmed severe (both judges, CRITICAL)

None.

### Corroborated WARNING (both judges, independently)

| ID | Location | Claim | Evidence class | Causality |
|---|---|---|---|---|
| W1 | `lib/_internal/recipe-add.py:247-265` | For recipes declaring both `cli_deps` and config fields, the interactive Add Recipe flow runs the dependency gate twice: once via `_run_cli_dep_gate` on stdout, then again inside `config_wizard.configure_selected_recipes` on stderr. The user sees the dependency panel and offer duplicated on two streams, and declining the first gate is contradicted because the flow continues into the config wizard regardless. | deterministic | introduced |

Judge A reference: `recipe-add.py:255-262`, `config_wizard.py:216`.
Judge B reference: `config_wizard.py:210`, `Console(stderr=True)` at `config_wizard.py:199`.

Confirmed cause: `recipe-add.py` calls `cw.configure_selected_recipes(...)`, which
re-invokes `_dep_gate` internally for the same recipe.

### Suspect (single judge)

None. All remaining single-judge rows are informational by severity.

### Contradictions

None.

### Informational (WARNING/SUGGESTION not eligible for auto-fix)

| ID | Location | Severity | Claim | Judge | Causality |
|---|---|---|---|---|---|
| I1 | `lib/_internal/recipe-add.py:91-104` | SUGGESTION | `_print_cli_dep_guidance` lists every required `cli_deps` binary, including already-satisfied ones, so guidance can overstate what still needs installing. | A | introduced |
| I2 | `lib/_internal/recipe-add.py:162` | SUGGESTION | A recipe declaring only `[[deps.cli]]` (no config fields, no MCP env) never enters the TTY Configure-now block, so the new gate wiring is unreachable for that recipe shape. | B | pre_existing |
| I3 | `lib/_internal/recipe-add.py:82-84` | SUGGESTION | `_run_cli_dep_gate` swallows any exception and returns `True`, silently bypassing the dependency check with only a one-line notice and no install guidance. | B | introduced |
| I4 | `lib/_internal/env_scaffold.py:494,501` | SUGGESTION | `offer_harness_env` collects env vars twice per call (directly and again inside `prompt_env_vars`), re-reading each selected recipe. | B | pre_existing |

## Verified-correct claims (both judges)

- Hub `_MENU` contains exactly 12 entries with `Configure recipes` after
  `Recipes`; the Recipes submenu alias still dispatches
  `Action.CONFIGURE_RECIPES`.
- `collect_env_vars` scoping is additive: `None` preserves aggregate behavior,
  an empty list yields `{}`, a disabled selected recipe contributes nothing, and
  an unknown id does not raise.
- `generate_env_example`, `ensure_root_envrc`, and direnv handling remain
  global; only prompted values are merged into `ai-specs.env`.
- `recipe-add.py` never calls `dep_install.offer_and_install`; install offers
  stay inside `config_wizard._dep_gate`, so no silent install is possible.
- Non-TTY behavior is guidance-only: neither the gate nor the env offer runs.
- Frozen focused suite: `Ran 131 tests ... OK`.

## Round bookkeeping

- Fix rounds used: 0 of 2.
- Scoped re-judgments used: 0 of 2.
- Reason: no `CRITICAL` finding was confirmed by both judges, so no bounded fix
  round was opened. `W1` is corroborated but informational by severity; it needs
  an explicit human decision, not an automatic round.

## Verdict

```yaml
target_identity: sha256:5d1e8f9d8b543c3aa8c472fb0ca33ae07945ef1eb38b1485314d0993f882f4d1
round: 1
confirmed: []
suspect: []
contradictions: []
info: [W1, I1, I2, I3, I4]
fix_work_units: []
scoped_rejudgment: not_run
terminal_state: approved
skill_resolution: paths-injected
```

A judgment issues no receipt and carries no delivery authority. Commit, push, PR,
and release remain separate human decisions under ordinary repository policy.
