---
name: openspec-sdd-workflow
description: >
  Unified SDD + worktree workflow for the ai-specs-cli repository.
  Covers: worktree creation from development, safety verification, SDD phase
  orchestration (explore → proposal → specs → design → tasks → apply → verify →
  archive), commit conventions, and archival rules. Trigger: when starting any
  OpenSpec change, creating a worktree, or deciding where implementation begins.
license: MIT
metadata:
  author: parada1104
  version: "2.0.0"
  scope: [root]
  auto_invoke:
    - "Starting a new OpenSpec change"
    - "Orchestrating an OpenSpec change phase by phase"
    - "Creating a branch or worktree"
    - "Starting work from development"
    - "Deciding where implementation should begin"
    - "Running a specific SDD phase in isolation"
    - "Editing openspec/config.yaml for spec-driven workflow"
---

# OpenSpec SDD Workflow

## Overview

This skill defines the complete workflow for spec-driven development in the
`ai-specs-cli` repository. It unifies worktree isolation, SDD phase progression,
and commit/archive conventions into a single canonical flow.

**Core principle:** Every implementation starts in an isolated worktree branched
from `development`, progresses through structured SDD artifacts, and closes with
an archived change.

## When to Use

- Starting any new feature, fix, or standardization task
- Creating a worktree or branch
- Orchestrating or continuing an OpenSpec change
- Deciding whether to use `main`, `development`, or a feature branch
- Editing `openspec/config.yaml`

---

## Part 1: Worktree Isolation

### Critical Patterns

- **`main` is protected by convention**: treat it as the stable line. Do not start
day-to-day feature work from `main`.
- **`development` is the integration base**: create it if missing, keep it updated,
and branch new work from there.
- **Every implementation starts in a worktree**: one worktree per feature/fix/experiment.
- **Worktrees live under `.worktrees/` at the repo root** (ignored by git).
- **Worktrees branch from `development`** unless the task is an explicit hotfix
strategy agreed by the maintainer.
- **Do not pile unrelated tasks into one worktree**. One branch, one intent.

### Directory Selection

Follow this priority order:

1. **Check existing directories:**
   ```bash
   ls -d .worktrees 2>/dev/null     # Preferred (hidden)
   ls -d worktrees 2>/dev/null      # Alternative
   ```
   If found, use that directory. If both exist, `.worktrees` wins.

2. **Check `CLAUDE.md` or similar agent config** for a worktree directory preference.

3. **Ask user** only if no directory exists and no preference is specified.

### Safety Verification

**MUST verify directory is ignored before creating worktree:**
```bash
git check-ignore -q .worktrees 2>/dev/null || git check-ignore -q worktrees 2>/dev/null
```

If NOT ignored:
1. Add the directory to `.gitignore`
2. Commit the change
3. Proceed with worktree creation

### Standard Flow

1. Ensure `main` is clean and pushed.
2. Ensure `development` exists and is pushed.
3. Update `development` locally:
   ```bash
   git checkout development
   git pull --ff-only origin development
   ```
4. Create a feature branch and dedicated worktree:
   ```bash
   mkdir -p .worktrees
   git worktree add .worktrees/<branch-name> -b <branch-name> development
   cd .worktrees/<branch-name>
   ```
5. Run project setup (auto-detect from project files):
   ```bash
   if [ -f package.json ]; then npm install; fi
   if [ -f requirements.txt ]; then pip install -r requirements.txt; fi
   if [ -f pyproject.toml ]; then poetry install; fi
   if [ -f Cargo.toml ]; then cargo build; fi
   if [ -f go.mod ]; then go mod download; fi
   ```
6. Verify clean baseline:
   ```bash
   ./tests/run.sh        # or project-appropriate test command
   ```
   If tests fail, report failures and ask whether to proceed.
7. Do all implementation work inside that worktree.

### Anti-patterns

- Putting worktrees outside the repo (e.g. `../worktrees/...`)
- Starting feature work directly on `main`
- Creating a feature branch from stale local state
- Reusing one worktree for multiple unrelated tasks
- Forgetting that the branch base must be `development`
- Skipping `git check-ignore` verification
- Proceeding with failing baseline tests without asking
- Deleting a worktree without verifying whether the branch is still needed

### Cleanup

```bash
# remove a finished worktree
git worktree remove .worktrees/<branch-name>
git branch -d <branch-name>
```

---

## Part 2: SDD Phase Orchestration

### Phase Definitions (spec-driven schema)

| Phase | Artifact(s) Produced | Specialist Skill |
|---|---|---|
| `explore` | Thinking captured in conversation or proposal | `openspec-explore` |
| `proposal` | `openspec/changes/<name>/proposal.md` | `openspec-new-change` |
| `specs` | `openspec/changes/<name>/specs/<capability>/spec.md` | `openspec-continue-change` |
| `design` | `openspec/changes/<name>/design.md` | `openspec-continue-change` |
| `tasks` | `openspec/changes/<name>/tasks.md` | `openspec-continue-change` |
| `apply` | Code changes + updated `tasks.md` checkboxes | `openspec-apply-change` |
| `verify` | `openspec/changes/<name>/verify-report.md` | `openspec-verify-change` |
| `archive` | Archived change in `openspec/changes/archive/` | `openspec-archive-change` |

### Phase Detection

```bash
openspec status --change "<name>" --json
```

Map artifact statuses to current phase:
- If user requested a specific phase → use that phase
- Else if `isComplete: true` → suggest `verify` or `archive`
- Else if no `proposal.md` → `explore` or `proposal`
- Else if proposal exists but specs incomplete → `specs`
- Else if specs done but design missing → `design`
- Else if design done but tasks missing → `tasks`
- Else if tasks exist with pending checkboxes → `apply`
- Else → `verify`

If the target phase is `blocked` (missing dependencies), show which artifacts
are blocking it and ask whether to jump to the missing dependency phase first.

---

## Part 3: Commit & Archive Conventions

### OpenSpec config shape

`openspec/config.yaml` should be valid for the installed `spec-driven` schema.
The `rules` map should only contain schema artifacts as lists of strings:

- `rules.proposal`
- `rules.specs`
- `rules.design`
- `rules.tasks`
- `rules.apply`
- `rules.verify`

Do not put nested objects under `rules.apply` or `rules.verify`.
Do not add `rules.archive` while `archive` is not an artifact in the installed schema.

Project-specific metadata that does not fit the `rules` shape lives under
`ai_specs_guidance` (for humans and local tooling, not part of the artifact
contract).

### Commits during apply

1. **Prefer one commit per top-level block** in `openspec/changes/<change>/tasks.md`
   (sections `## 1`, `## 2`, …) when that block is a coherent unit of work.

2. **Within a long phase**, split by sub-phase if the diff would otherwise be
   hard to review, but keep each commit **green** (`./tests/run.sh` or the
   narrowest test slice you are using for TDD).

3. **Update `tasks.md` checkboxes** as each task completes; commits and checkboxes
   advance together so progress stays auditable.

4. **End of apply** (before PR): run `./tests/validate.sh`, refresh
   `apply-progress.md` / `verify-report.md` as required by the change, then push.

### Archive after merge

When the implementation is merged into **`development`** (or the integration
branch your team uses), close the change with **`openspec-archive-change`** so
delta specs and the change folder follow the project archive rules. Verify first
with **`openspec-verify-change`** / `verify-report.md` if the change required it.

---

## Part 4: Complete Workflow Example

```
[Worktree setup]
  git checkout development
  git pull --ff-only origin development
  git worktree add .worktrees/my-feature -b my-feature development
  cd .worktrees/my-feature
  ./tests/run.sh            # verify clean baseline

[SDD cycle inside worktree]
  openspec new change "my-feature"
  openspec continue         # proposal → specs → design → tasks
  openspec apply            # implement tasks
  openspec verify           # validate against specs
  openspec sync-specs       # delta → main specs
  openspec archive-change   # move to archive/

[Cleanup]
  cd ../..
  git worktree remove .worktrees/my-feature
  git branch -d my-feature
```

---

## Related Skills

- `openspec-explore` — exploration stance
- `openspec-new-change` — change creation
- `openspec-continue-change` — artifact creation
- `openspec-apply-change` — implementation
- `openspec-verify-change` — verification
- `openspec-archive-change` — archival
- `testing-foundation` — default test commands and evidence
