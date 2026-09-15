# Tasks: Fix VCS auth preflight account-name parsing

Depth: light

Explore: skipped — concrete files and expected behavior are known; one obvious approach
exists in a known area; no conflict, uncertainty, or retry signal applies.

## Success Criteria

1. Running the extraction against a plural-account `gh auth status` output returns the clean
   active account name (no `(keyring`/`(...` suffix).
2. The provider command text (GH/glab) and the auth blocker messaging remain intact so
   existing contract tests still pass.
3. The full test suite remains green.

## Tasks

- [x] 1. Replace the buggy awk account-name extraction in `git-pr-flow` with a field-based scan
      that captures only the token following `account`, without literal substring offsets.
      - Files: `catalog/recipes/git-pr-flow/commands/pr-create.md`,
        `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md`
      - Both the initial active-account read and the post-`gh auth switch` re-read.
- [x] 2. Apply provider-specific account extraction to the GitLab recipe: capture the token
      immediately following `as` because `glab auth status` does not emit an `account` token;
      reject multiple login lines as ambiguous instead of selecting the first with `head -1`.
      - Files: `catalog/recipes/gitlab-mr-flow/commands/mr-create.md`,
        `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md`
- [x] 3. Reconcile scope: keep this change to GitHub and GitLab only. No Bitbucket edits
      were made — current `development` already carries the upstream Bitbucket auth rewrite,
      and the Bitbucket ANSI/color edge case is explicitly out of scope here.
      - Files: none (scope decision only; `catalog/recipes/bitbucket-pr-flow/**` untouched)
- [x] 4. Add regression tests that execute the extracted awk snippets for annotated accounts,
      active-account selection, and ambiguous GitLab multi-account/multi-host output.
      - Files: `tests/test_git_pr_flow_recipe.py`, `tests/test_gitlab_mr_flow_recipe.py`
- [x] 5. Verify the fix by simulating the GitHub and GitLab provider outputs and running the
      auth-relevant and full test suites.
