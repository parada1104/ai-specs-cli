# Tasks: hub-wizard-help

Depth: standard

Trello: https://trello.com/c/4lCqI2Da (card #42)

## Checklist

- [x] Fix `prompt_env_vars` to use `questionary.password` / `is_password` (not `password=`)
- [x] Harden `_offer_envrc` with try/except soft-fail
- [x] Add `ENV_VAR_HELP` map; show in prompts and `.envrc.example` comments
- [x] Normalize config `type = "boolean"` → `"bool"` in recipe schema parse
- [x] Add `help_text` to all catalog ConfigFields (trello, worktree, vcs×3, vault, tdd)
- [x] Unit tests for crash regression, soft-fail, help comments, boolean normalize
- [x] `./tests/run.sh` green; `./tests/validate.sh` before commit
