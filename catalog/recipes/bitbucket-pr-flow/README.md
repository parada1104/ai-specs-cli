# Bitbucket PR Flow

**Bitbucket pull request (PR) and merge flow for feature branches** via PHP
[`bb-cli`](https://bb-cli.github.io) (Homebrew formula `bb-cli`, binary `bb`).
The base branch is configurable per project.

This recipe targets the PHP project at https://bb-cli.github.io only. It does
not install or document a TypeScript Bitbucket CLI.

## What it provides

- **Skill** `bitbucket-merge-workflow` (bundled) — push, open PR, approval-gated merge, and
  worktree/branch cleanup for Bitbucket.
- **Command** `/bb-pr-create` — thin agent-facing command; stops after opening the PR.

## Capability

Declares the `vcs-pr-flow` capability. Bind this recipe when the project's host is Bitbucket:

```toml
[[bindings]]
capability = "vcs-pr-flow"
recipe = "bitbucket-pr-flow"
```

Sibling recipes cover GitHub ([`git-pr-flow`](../git-pr-flow/README.md)) and
GitLab ([`gitlab-mr-flow`](../gitlab-mr-flow/README.md)).

## Prerequisites

- **PHP `bb-cli`** on `PATH` as `bb`, authenticated with `bb auth save`.
  Host floor is `1.4.1+` via `bb --version` (`min_version = "1.4.1"`). That is
  the PHP bb-cli version, not recipe `version = "1.3.0"`.
  Inspect the active account with a **redacted** `bb auth show` capture
  (not `bb auth status`): emit only `Username`; never print `AppPassword`.
  Reject a missing, empty, or multiple Username line. Install from
  https://bb-cli.github.io (binary/PHP notes:
  https://bb-cli.github.io/installation/). On macOS (and Linux with Homebrew)
  the TTY configure/init offer is `brew install bb-cli`. Never install Homebrew
  formula or cask `bb` (getbb.app, not Bitbucket).
- Auth docs: https://bb-cli.github.io/authentication

### Binary identity / PATH

`command -v bb` only proves some `bb` is on `PATH`. Positively confirm PHP
`bb-cli` with `bb --version` plus the PHP `bb auth show` shape (`Username:` /
`AppPassword:` — capture the output; emit only `Username`). If `bb --version`
does not identify PHP `bb-cli`, that binary is not PHP `bb-cli`: stop and
install from https://bb-cli.github.io (Homebrew: `brew install bb-cli`; never
Homebrew formula/cask `bb`). Uninstall or move a TypeScript Bitbucket CLI, or
put the PHP `bb` earlier on `PATH`.

## Enable in `ai-specs.toml`

```toml
[recipes.bitbucket-pr-flow]
enabled = true
version = "1.3.0"

[recipes.bitbucket-pr-flow.config]
base_branch = "development"
expected_owner = ""
auto_switch_account = false
```

Run `ai-specs sync` to materialize the bundled skill, `/bb-pr-create`, and this doc.

## Config

| Key                   | Required | Type    | Default        | Description |
| --------------------- | -------- | ------- | -------------- | ----------- |
| `base_branch`         | no       | string  | `development`  | Base branch the PR targets. |
| `expected_owner`      | no       | string  | `""`           | Account username this repo expects; activates auth preflight when set. |
| `auto_switch_account` | no       | boolean | `false`        | Reserved for API parity; PHP `bb-cli` has no auth switch — mismatch blocks with `bb auth save` guidance. |

## PHP `bb` verbs (verified)

Create (positional source then destination; omitted source → current branch):

```bash
bb pr create <branch-name> <base_branch> --title "<title>" --description "<summary>"
```

Inspect PR **comments** (not PR JSON):

```bash
bb pr show <pr-id>
```

Merge (method + id only):

```bash
bb pr merge <pr-id>
```

Do not use `-i` as the agent default (interactive prompts). Do not publish
`--source`, `--destination`, `--body`, `--json`, `--jq`, `--strategy squash`,
or `--close-source-branch` as PHP flags.

**Open verification gap:** `bb pr show` lists comments; it does not provide a
verified source-commit SHA. Agents must **stop** rather than guess JSON/jq or
run `bb pr commits` as if it were an allowed discovery probe. Squash and
source-branch closure are Bitbucket UI / git cleanup policy, not verified `bb`
flags. `bb auth save` prompt text is undocumented — run the command with no
invented flags.

## Long-lived branches

Protected heads (`main`, `master`, `development`, `staging`, plus configured
`base_branch` / `integration_branch`) are **not** deleted after merge — not
locally, not via Bitbucket UI "Close source branch", and not with
`git push $REMOTE --delete`. Feature-head cleanup is git/worktree
(`git worktree remove`, `git push "$REMOTE" --delete <branch-name>`,
`git branch -D`), not an unverified `bb` close-source flag. Prefer
`release/vX.Y.Z` → `main` for releases, not `development` as the PR source.

## Safety note

Never push, create, or merge a PR without explicit user instruction. Feature work goes
through a PR — never a local `git merge`.
