# /mr-create

Open a merge request for the current feature branch using the GitLab MR flow.
This command operationalizes the bundled `gitlab-merge-workflow` skill — read
that skill first for the full guardrails before acting.

## Configuration

Read the provider and base branch from the recipe config in `ai-specs.toml`:

```toml
[recipes.gitlab-mr-flow.config]
provider = "gitlab"       # default
base_branch = "development"  # default
```

If a value is unset, fall back to the defaults (`gitlab`, `development`) and to
the runtime brief (`AGENTS.md`) for any provider/branch context it declares.

## Preconditions

- The user explicitly asked to create an MR (this command does not run unprompted).
- The working branch belongs to one focused change.
- The worktree has no unrelated uncommitted changes.
- Required verification evidence is complete, or the user accepts the gap.
- `glab` is installed and authenticated.

## Steps

1. Inspect the current branch, worktree path, and `git status`.

2. Run the runtime preflight:

   ```bash
   command -v glab
   ```

   If `glab` is not found, stop and report:

   > **Blocker**: `glab` is not installed. Install it from https://gitlab.com/gitlab-org/cli
   > and retry.

3. Verify authentication:

   ```bash
   glab auth status
   ```

   If authentication fails, stop and report:

   > **Blocker**: `glab` is not authenticated. Run `glab auth login` and retry.

4. Confirm or run the verification required by the runtime brief / change.

5. Push the feature branch explicitly:

   ```bash
   git push -u origin <branch-name>
   ```

6. Create the MR against the configured base branch:

   ```bash
   glab mr create --source-branch <branch-name> --target-branch <base_branch> --title "<title>" --description "<summary and verification>" --yes
   ```

7. STOP. Do not merge. Report the MR URL and wait for explicit user approval.

8. Merge ONLY after the user explicitly approves and required checks/review pass:

   ```bash
   glab mr merge <mr-number> --squash
   ```

## Guardrails

- Never push, create, or merge an MR without explicit user instruction.
- Never merge locally with `git merge` for feature work that should go through an MR.
- Preserve unrelated changes; stop and ask if any step would touch them.
- Never use implicit push options on `glab mr create` — always push explicitly before creating the MR.
- Never use options that merge without explicit user approval.
- If `glab` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating an MR.

See the bundled `gitlab-merge-workflow` skill for the complete workflow and cleanup steps.
