# Tasks: `ai-specs skills` subcommand group

## Phase 1 — Infrastructure

- [x] 1.1 Create `lib/skills.sh` — sub-dispatcher with `add`, `list`, `remove` cases
- [x] 1.2 Update `bin/ai-specs` — add `skills` case, redirect `add-dep` to `skills-add.sh`
- [x] 1.3 Create `lib/skills-add.sh` — register vendored dep (migrate from `add-dep.sh`)
- [x] 1.4 Create `lib/skills-list.sh` — list deps, local skills, catalog skills
- [x] 1.5 Create `lib/skills-remove.sh` — remove `[[deps]]` by id

## Phase 2 — Testing

- [x] 2.1 Create `tests/test_skills_add.py` — tests for skills-add (migrated from add-dep tests)
- [x] 2.2 Create `tests/test_skills_remove.py` — tests for skills-remove

## Phase 3 — Documentation

- [x] 3.1 Update README.md — document `skills add | list | remove`, note `add-dep` alias
