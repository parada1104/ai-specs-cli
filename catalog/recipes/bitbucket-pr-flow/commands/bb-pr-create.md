# /bb-pr-create

Open a pull request for the current feature branch using the Bitbucket PR flow.
This command operationalizes the bundled `bitbucket-merge-workflow` skill — read
that skill first for the full guardrails before acting.

## Configuration

Read the base branch from the recipe config in `ai-specs.toml`:

```toml
[recipes.bitbucket-pr-flow.config]
base_branch = "development"  # default
```

If unset, fall back to the recipe default (`development`) and to the runtime brief
(`AGENTS.md`) for branch context.

## Preconditions

- The user explicitly asked to create a PR (this command does not run unprompted).
- The working branch belongs to one focused change.
- The worktree has no unrelated uncommitted changes.
- Required verification evidence is complete, or the user accepts the gap.
- `bb` is installed and authenticated.

## Steps

1. Inspect the current branch, worktree path, and `git status`.

2. Run the runtime preflight:

   ```bash
   command -v bb
   ```

   If `bb` is not found, stop and report:

   > **Blocker**: `bb` is not installed. Install it from https://bitbucket-cli.paulvanderlei.com/getting-started/installation/
   > and retry.

3. Verify authentication:

   ```bash
   bb auth status
   ```

   If authentication fails (output includes "Not logged in"), stop and report:

   > **Blocker**: `bb` is not authenticated. Run `bb auth login` and retry.

4. Run **Runtime Preflight: Account Match** (config-gated — skip when `expected_owner` is empty):

   Read from recipe config in `ai-specs.toml`:

   ```toml
   [recipes.bitbucket-pr-flow.config]
   expected_owner = ""           # default; set to activate preflight
   ```

   ```bash
   # Runtime Preflight: Account Match (Bitbucket)
   # Note: the bb CLI has no `bb auth show` (verified on bb 1.23.2, which answers
   # `unknown command 'show'`). The subcommand is `bb auth status`, like gh and glab.
   EXPECTED_OWNER="{config.expected_owner}"
   if [ -n "$EXPECTED_OWNER" ]; then
     ACTIVE=$(bb auth status 2>&1 | awk '/Username|username/ {print $2}' | head -1)
     if [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
       echo "**Blocker**: active bb account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
       echo "bb has no 'auth switch'. Run: bb auth login"
       return 1
     fi
   fi
   ```

   If the preflight returns a blocker, stop before pushing.

5. Confirm or run the verification required by the runtime brief / change.

6. Resolve the Bitbucket remote and push the feature branch explicitly:

   ```bash
   REMOTE=$(git remote | grep -E '^(origin|bitbucket|upstream)$' | head -1 || echo "origin")
   git push -u $REMOTE <branch-name>
   ```

   > **Note**: The remote is resolved dynamically to support repos where the Bitbucket remote is named `bitbucket` or `upstream` instead of `origin`. Falls back to `origin` if no known name matches.

7. Create the PR against the configured base branch:

   ```bash
   bb pr create --source <branch-name> --destination <base_branch> --title "<title>" --body "<summary and verification>"
   ```

8. STOP. Do not merge. Report the PR URL and wait for explicit user approval.

For the full merge workflow (approval → merge with source-commit check → cleanup), see the `bitbucket-merge-workflow` skill.

## Guardrails

- Never push or create a PR without explicit user instruction.
- Preserve unrelated changes; stop and ask if any step would touch them.
- Never rely on implicit push behavior from the Bitbucket CLI — always push explicitly before creating the PR.
- If `bb` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating a PR.

See the bundled `bitbucket-merge-workflow` skill for the complete workflow and cleanup steps.
