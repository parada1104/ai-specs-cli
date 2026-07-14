# /mr-create

Open a merge request for the current feature branch using the GitLab MR flow.
This command operationalizes the bundled `gitlab-merge-workflow` skill — read
that skill first for the full guardrails before acting.

## Configuration

Read the base branch from the recipe config in `ai-specs.toml`:

```toml
[recipes.gitlab-mr-flow.config]
base_branch = "development"  # default
```

If unset, fall back to the recipe default (`development`) and to the runtime brief
(`AGENTS.md`) for branch context.

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

4. Run **Runtime Preflight: Account Match** (config-gated — skip when `expected_owner` is empty):

   Read from recipe config in `ai-specs.toml`:

   ```toml
   [recipes.gitlab-mr-flow.config]
   expected_owner = ""           # default; set to activate preflight
   ```

   ```bash
   # Runtime Preflight: Account Match (GitLab)
   EXPECTED_OWNER="{config.expected_owner}"
   if [ -n "$EXPECTED_OWNER" ]; then
     ACTIVE=$(glab auth status 2>&1 | awk '
       /Logged in to gitlab\.com account/ {
         if (match($0, /account [^ ]+ \(/))      { a=substr($0, RSTART+8, RLENGTH-2) }
         else if (match($0, /account [^ ]+$/))   { a=substr($0, RSTART+8) }
       }
       /Active account: true/ { print a }' | head -1)
     if [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
       echo "**Blocker**: active glab account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
       echo "glab has no 'auth switch'. Run: glab auth login   (or export GLAB_TOKEN=<token>)."
       return 1
     fi
   fi
   ```

   If the preflight returns a blocker, stop before pushing.

5. Verify `jq` is available (required for SHA pinning during merge):

   ```bash
   command -v jq
   ```

   If `jq` is not found, stop and report:

   > **Blocker**: `jq` is not installed. Install it from https://jqlang.github.io/jq/download/ and retry.

5. Confirm or run the verification required by the runtime brief / change.

6. Resolve the GitLab remote and push the feature branch explicitly:

   ```bash
   REMOTE=$(git remote | grep -E '^(origin|gitlab|upstream)$' | head -1 || echo "origin")
   git push -u $REMOTE <branch-name>
   ```

   > **Note**: The remote is resolved dynamically to support repos where the GitLab remote is named `gitlab` or `upstream` instead of `origin`. Falls back to `origin` if no known name matches.

7. Create the MR against the configured base branch:

   ```bash
   glab mr create --source-branch <branch-name> --target-branch <base_branch> --title "<title>" --description "<summary and verification>" --yes
   ```

8. STOP. Do not merge. Report the MR URL and wait for explicit user approval.

For the full merge workflow (approval → merge with SHA pinning → cleanup), see the `gitlab-merge-workflow` skill.

## Guardrails

- Never push or create an MR without explicit user instruction.
- Preserve unrelated changes; stop and ask if any step would touch them.
- Never use implicit push options on `glab mr create` — always push explicitly before creating the MR.
- If `glab` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating an MR.

See the bundled `gitlab-merge-workflow` skill for the complete workflow and cleanup steps.
