# /bb-pr-create

Open a pull request for the current feature branch using the Bitbucket PR flow.
This command operationalizes the bundled `bitbucket-merge-workflow` skill — read
that skill first for the full guardrails before acting.

This recipe uses PHP [`bb-cli`](https://bb-cli.github.io) (formula `bb-cli`,
binary `bb`). `command -v bb` cannot distinguish a leftover TypeScript
Bitbucket CLI. Positively confirm PHP `bb-cli` with `bb --version` plus the
PHP `bb auth show` shape; if the binary is not PHP `bb-cli`, stop and install
from https://bb-cli.github.io (`brew install bb-cli`) rather than guessing flags.

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
   BB_VERSION=$(bb --version 2>&1)
   printf '%s\n' "$BB_VERSION"
   if ! printf '%s\n' "$BB_VERSION" | grep -q 'Version:'; then
     echo "**Blocker**: \`bb\` on PATH is not PHP bb-cli. Install PHP bb-cli from https://bb-cli.github.io (Homebrew: \`brew install bb-cli\`; never Homebrew formula/cask \`bb\`) and retry."
     return 1
   fi
   ```

   If `bb` is not found, stop and report:

   > **Blocker**: `bb` is not installed. Install it from https://bb-cli.github.io
   > (Homebrew: `brew install bb-cli`; never Homebrew formula/cask `bb`) and retry.

   If `bb --version` does not identify PHP `bb-cli`, stop — that binary is
   not PHP `bb-cli`.

3. Verify authentication with a redacted capture (emit only `Username`;
   never print `AppPassword`). Parse conservatively and block on a missing,
   empty, or multiple Username line:

   ```bash
   AUTH_CAPTURE=$(bb auth show 2>&1)
   USERNAME_LINES=$(printf '%s\n' "$AUTH_CAPTURE" | awk -F': ' '$1 == "Username" { print }')
   USERNAME_COUNT=$(printf '%s\n' "$USERNAME_LINES" | awk 'NF { n++ } END { print n+0 }')
   if [ "$USERNAME_COUNT" -ne 1 ]; then
     echo "**Blocker**: missing, empty, or multiple Username lines. \`bb\` is not authenticated. Run \`bb auth save\` and retry."
     return 1
   fi
   USERNAME=$(printf '%s\n' "$USERNAME_LINES" | awk -F': ' '{ gsub(/^ +| +$/, "", $2); print $2 }')
   if [ -z "$USERNAME" ]; then
     echo "**Blocker**: missing or empty Username. \`bb\` is not authenticated. Run \`bb auth save\` and retry."
     return 1
   fi
   printf '%s\n' "$USERNAME_LINES"
   ```

   Observed PHP output shape (do not print `AppPassword`): lines
   `Username: <value>` and `AppPassword: <secret>`. A missing or empty
   `Username`, or multiple Username lines, means credentials are absent or
   ambiguous. If unauthenticated, stop and report:

   > **Blocker**: `bb` is not authenticated. Run `bb auth save` and retry.

   **Open verification gap:** exact `bb auth save` prompts are not encoded here.
   Run `bb auth save` with no invented flags. See
   https://bb-cli.github.io/authentication

4. Run **Runtime Preflight: Account Match** (config-gated — skip when `expected_owner` is empty):

   Read from recipe config in `ai-specs.toml`:

   ```toml
   [recipes.bitbucket-pr-flow.config]
   expected_owner = ""           # default; set to activate preflight
   ```

   ```bash
   # Runtime Preflight: Account Match (Bitbucket)
   # PHP bb-cli: capture bb auth show (not bb auth status). Username parse is conservative.
   EXPECTED_OWNER="{config.expected_owner}"
   if [ -n "$EXPECTED_OWNER" ]; then
     AUTH_CAPTURE=$(bb auth show 2>&1)
     USERNAME_LINES=$(printf '%s\n' "$AUTH_CAPTURE" | awk -F': ' '$1 == "Username" { print }')
     USERNAME_COUNT=$(printf '%s\n' "$USERNAME_LINES" | awk 'NF { n++ } END { print n+0 }')
     if [ "$USERNAME_COUNT" -ne 1 ]; then
       echo "**Blocker**: missing, empty, or multiple Username lines. Stop rather than guessing."
       echo "bb has no 'auth switch'. Run: bb auth save"
       return 1
     fi
     ACTIVE=$(printf '%s\n' "$USERNAME_LINES" | awk -F': ' '{ gsub(/^ +| +$/, "", $2); print $2 }')
     if [ -z "$ACTIVE" ] || [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
       echo "**Blocker**: active bb account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
       echo "bb has no 'auth switch'. Run: bb auth save"
       return 1
     fi
   fi
   ```

   **Open verification gap:** do not treat this awk as a stable public API.
   Never log `AppPassword`. If Username is missing, empty, or there are
   multiple Username lines, stop rather than guessing. There is no
   `bb auth switch`.

   If the preflight returns a blocker, stop before pushing.

5. Confirm or run the verification required by the runtime brief / change.

6. Resolve the Bitbucket remote and push the feature branch explicitly:

   ```bash
   REMOTE=$(git remote | grep -E '^(origin|bitbucket|upstream)$' | head -1 || echo "origin")
   git push -u $REMOTE <branch-name>
   ```

   > **Note**: The remote is resolved dynamically to support repos where the Bitbucket remote is named `bitbucket` or `upstream` instead of `origin`. Falls back to `origin` if no known name matches.

7. Create the PR against the configured base branch (PHP positional source then
   destination; `--title` / `--description`; never `-i` as the agent default):

   ```bash
   bb pr create <branch-name> <base_branch> --title "<title>" --description "<summary>"
   ```

   Do not invent `--source`, `--destination`, or `--body`. Do not run
   `bb pr create --help` (it performs a real operation). If destination
   semantics are unclear for this repo, **stop** before create.

8. STOP. Do not merge. Report the PR URL and wait for explicit user approval.

   Inspection of an existing PR's **comments** uses `bb pr show <pr-id>` (not JSON).
   Merge stays in the bundled skill.

For the full merge workflow (approval → merge with source-commit check → cleanup), see the `bitbucket-merge-workflow` skill.

## Guardrails

- Never push or create a PR without explicit user instruction.
- Preserve unrelated changes; stop and ask if any step would touch them.
- Never rely on implicit push behavior from the Bitbucket CLI — always push explicitly before creating the PR.
- If `bb` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating a PR.

See the bundled `bitbucket-merge-workflow` skill for the complete workflow and cleanup steps.
