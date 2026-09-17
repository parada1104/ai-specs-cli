# Proposal: local skill `codegraph-worktree`

## Why

CodeGraph indexing is per-workspace: the `.codegraph/` index is machine-local and
git-ignored, so a fresh worktree **never** inherits the index from the main checkout
(`git worktree add` only checks out tracked files). Agents switching into a worktree
keep querying the main repo's index, which misses files created/changed on the branch
and silently returns stale blast-radius data.

Verified in-session (2026-09-02): both active worktrees (`fix-vcs-auth-preflight-account-parse`,
`jinna-mcp-recipe`) had no `.codegraph/` at all, and the Pi-native `codegraph` tool
(provided by gentle-pi's `codegraph-tools.ts` extension) resolves the index from the
current workspace only.

## What

Add one local skill `ai-specs/skills/codegraph-worktree/SKILL.md` that guides agents to:

1. Run `codegraph init` when entering/creating a worktree where code will be edited.
2. Re-index (`codegraph init` again) after large refactors on the branch.
3. Respect freshness rules: the index is a cache; source reads via `explore` are
   live, but symbol/call-path data is only as fresh as the last index run. Critical
   blast-radius claims get corroborated with `grep`.
4. Never commit `.codegraph/` (the tool ships its own `.gitignore` guard).

## Scope

- **In scope**: one SKILL.md under `ai-specs/skills/` (project-local skill), `ai-specs sync`
  to register it, this change folder.
- **Out of scope**: catalog recipe promotion (later, per user decision), any change to
  the CLI product code, gate/recipe changes.

## Non-goals

- No automation hook (no post-`/worktree-new` wiring yet); the skill is advisory for agents.
- No catalog recipe changes.

## Tracker

- card_id: `6a97b783e24cccf672fde946`
- url: https://trello.com/c/GX7ctoBJ
