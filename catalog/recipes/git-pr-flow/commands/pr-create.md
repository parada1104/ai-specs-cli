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
3. Run **Runtime Preflight: Account Match** (config-gated — skip when `expected_owner` is empty):

   Read from recipe config in `ai-specs.toml`:

   ```toml
   [recipes.git-pr-flow.config]
   expected_owner = ""           # default; set to activate preflight
   auto_switch_account = false     # gh only; requires gh >= 2.50.0
   ```

   ```bash
   # Runtime Preflight: Account Match (GitHub)
   # Config-gated: runs only when expected_owner is a non-empty string.
   EXPECTED_OWNER="{config.expected_owner}"
   AUTO_SWITCH="{config.auto_switch_account}"   # "true" | "false"

   if [ -n "$EXPECTED_OWNER" ]; then
     # 1. Version guard
     GH_VER=$(gh --version | head -1 | awk '{print $3}')
     SWITCH_OK=1
     if ! printf '%s\n%s\n' "2.50.0" "$GH_VER" | sort -V -C; then
       echo "⚠ ai-specs: gh auth switch requires gh >= 2.50.0 (have $GH_VER); auto-switch disabled."
       SWITCH_OK=0
     fi

     # 2. Active account (supports multiple logged-in accounts)
     ACTIVE=$(gh auth status 2>&1 | awk '
       /Logged in to .* account/ {
         if (match($0, /account [^ ]+ \(/))      { a=substr($0, RSTART+8, RLENGTH-2) }
         else if (match($0, /account [^ ]+$/))   { a=substr($0, RSTART+8) }
       }
       /Active account: true/ { print a }
     ' | head -1)

     # 3. Target owner — prefer expected_owner
     TARGET="$EXPECTED_OWNER"

     # 4. Compare & react
     if [ "$ACTIVE" = "$TARGET" ]; then
       : # proceed
     elif [ "$AUTO_SWITCH" = "true" ] && [ "$SWITCH_OK" -eq 1 ]; then
       if ! gh auth switch --user "$TARGET" 2>&1; then
         echo "**Blocker**: gh auth switch failed for '$TARGET'. Aborting before push."
         return 1
       fi
       ACTIVE=$(gh auth status 2>&1 | awk '
         /Logged in to .* account / { if (match($0, /account [^ ]+ \(/)) { a=substr($0, RSTART+8, RLENGTH-2) } else if (match($0, /account [^ ]+$/)) { a=substr($0, RSTART+8) } }
         /Active account: true/ { print a }' | head -1)
       [ "$ACTIVE" = "$TARGET" ] || { echo "**Blocker**: switch did not land. Aborting."; return 1; }
     else
       echo "**Blocker**: active gh account is '$ACTIVE'; expected '$TARGET'."
       echo "Run: gh auth switch --user $TARGET   (or set auto_switch_account = true in ai-specs.toml)"
       return 1
     fi
   fi
   ```

   If the preflight returns a blocker, stop before pushing.

4. Push the feature branch:

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
