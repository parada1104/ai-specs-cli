# /pr-create

Open a pull request for the current feature branch using the GitHub PR flow.
This command operationalizes the bundled `git-merge-workflow` skill — read
that skill first for the full guardrails before acting.

## Configuration

Read the base branch from the recipe config in `ai-specs.toml`:

```toml
[recipes.git-pr-flow.config]
base_branch = "main"  # default
```

If unset, fall back to the recipe default (`main`) and to the runtime brief
(`AGENTS.md`) for branch context.

## Preconditions

- The user explicitly asked to create a PR (this command does not run unprompted).
- The working branch belongs to one focused change.
- The worktree has no unrelated uncommitted changes.
- Required verification evidence is complete, or the user accepts the gap.
- `gh` is installed and authenticated.

## Steps

1. Inspect the current branch, worktree path, and `git status`.
2. Confirm or run the verification required by the runtime brief / change.
3. Push the feature branch:

   ```bash
   git push -u origin <branch-name>
   ```

4. Create the PR against the configured base branch:

   ```bash
   gh pr create --base <base_branch> --title "<title>" --body "<summary and verification>"
   ```

5. STOP. Do not merge. Report the PR URL and wait for explicit user approval.

For the full merge workflow (approval → merge → cleanup), see the `git-merge-workflow` skill.

## Guardrails

- Never push, create, or merge a PR without explicit user instruction.
- Never merge locally with `git merge` for feature work that should go through a PR.
- Preserve unrelated changes; stop and ask if any step would touch them.
- If `gh` is unavailable or unauthenticated, stop with the exact blocker.

See the bundled `git-merge-workflow` skill for the complete workflow and cleanup steps.
