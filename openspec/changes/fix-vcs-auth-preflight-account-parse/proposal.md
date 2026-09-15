# Proposal: Fix VCS auth preflight account-name parsing

## Context

The `ai-specs` VCS PR/MR recipes (`git-pr-flow`, `gitlab-mr-flow`)
include a config-gated "account-match preflight" that runs before push/PR/MR creation when
`expected_owner` is set. It executes the provider CLI (`gh auth status`, `glab auth status`)
and compares the detected active account against the expected owner.

The account-name extraction is buggy: it returns the account name padded with trailing
garbage (e.g. `parada1104 (keyrin` instead of `parada1104`) when the CLI reports more than
one logged-in account or includes a trailing credential-source annotation. As a result the
comparison `ACTIVE == EXPECTED_OWNER` never matches and the preflight blocks with a false
"active gh account is 'badname'; expected 'goodname'" error even when the user is correctly
authenticated.

## Problem

- `gh auth status` may list several logged-in accounts and annotate the line with the
  credential source, e.g. `  ✓ Logged in to github.com account parada1104 (keyring)`.
- The current awk `match($0, /account [^ ]+ \(/)` then `substr($0, RSTART+8, RLENGTH-2)`
  captures the account name plus the trailing ` (keyrin` trash.
- The single-account fallback `match($0, /account [^ ]+$/)` captures the whole tail.

## Requirements

- Extract only the provider account-name token: the token immediately following `account`
  for GitHub and the token immediately following `as` for GitLab.
- Treat multiple GitLab login lines as ambiguous because `glab auth status` has no active
  account marker; return no account so the existing mismatch blocker fires rather than guess.
- Keep the preflight behavior identical otherwise: no auth call when `expected_owner` is
  empty; same install/auth checks; same blockers; same `gh auth switch` logic.
- Preserve the exact provider command used (GH uses `gh auth status`, GitLab uses
  `glab auth status`).

## Scope note: Bitbucket

The Bitbucket recipe (`bitbucket-pr-flow`) is out of scope for this change. Current
`development` already contains the upstream Bitbucket auth rewrite, and this branch does not
modify any Bitbucket recipe file. Bitbucket ANSI/color noise behavior around `bb auth show` is
not covered here; if it needs a fix, it will be handled separately against the upstream
Bitbucket implementation.

## Non-goals

- Do not change the config surface (`expected_owner`, `auto_switch_account`).
- Do not alter provider semantics or add new CLI calls.
- Do not rewrite what the providers print; only parse it correctly.
- Do not touch the Bitbucket recipe or its ANSI parsing (see Scope note above).

## Success criteria

1. Running the extraction against a plural-account `gh auth status` output returns the clean
   active account name (no `(keyring`/`(...` suffix).
2. The provider command text (GH/glab) and the auth blocker messaging remain intact so
   existing contract tests still pass.
3. Regression tests cover annotated accounts, active-account selection, and ambiguous GitLab
   multi-account/multi-host output.
4. The full test suite remains green.

## Tracker

- **card_id**: `6a558dd4cefa46b491c394d1`
- **shortLink**: `pMaMPN8r`
- **url**: https://trello.com/c/pMaMPN8r/41-feature-vcs-multi-account-preflight-prevent-wrong-account-forks-in-gh-glab
- **list**: In Progress
