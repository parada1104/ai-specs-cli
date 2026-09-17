# Delta for vcs-pr-flow

## ADDED Requirements

### Requirement: Bitbucket provider targets the PHP bb-cli contract

The `bitbucket-pr-flow` provider MUST identify the PHP `bb-cli` project as its sole
upstream, using the Homebrew formula `bb-cli` and the project homepage
`https://bb-cli.github.io`. The provider binary MUST remain `bb`, and the renderer
label MUST remain `Bitbucket (`bb` CLI)`. Live recipe surfaces MUST NOT identify
`@pilatos/bitbucket-cli`, paulvanderlei, or the TypeScript CLI as the provider.

Bitbucket authentication guidance MUST inspect credentials with `bb auth show` and
MUST direct remediation through `bb auth save`; it MUST NOT publish `bb auth login`
or `bb auth switch`. Agent-facing auth checks MUST capture `bb auth show` output
and emit only the `Username` line; they MUST reject a missing `Username` line and
MUST reject more than one `Username` line; they MUST NEVER print `AppPassword`.
A positive PHP `bb-cli` identity check MUST run after `command -v bb` and MUST
block a different `bb` binary with Homebrew formula `bb-cli` / https://bb-cli.github.io
guidance. PR guidance MUST use the PHP `create`, `show`, and `merge`
methods and only flags verified from authoritative upstream documentation or the
allowed read-only probes (`bb pr list` and `bb pr show`). The verified PHP global
options are limited to `--project`, `-i`/`--interactive`, `--title`, and
`--description` unless additional options are subsequently verified.

Any invocation detail not verified in the proposal's **Open verification gaps**
table MUST be documented as an open verification gap, never published as executable
guidance, and agents MUST stop with guidance rather than execute it. In particular,
this requirement MUST NOT preserve or infer TypeScript-shaped source, destination,
body, JSON/jq, squash, or source-branch-closure flags.

#### Scenario: Bitbucket identity and renderer label remain stable

- **GIVEN** the `bitbucket-pr-flow` recipe and its rendered capability guidance
- **WHEN** provider identity and renderer metadata are inspected
- **THEN** the provider is identified as PHP `bb-cli` with Homebrew formula `bb-cli`
- **AND** the install or upstream URL is `https://bb-cli.github.io` or an authoritative installation page for that project
- **AND** the executable name is `bb`
- **AND** the renderer label is `Bitbucket (`bb` CLI)`
- **AND** no live recipe surface identifies `@pilatos/bitbucket-cli` or paulvanderlei as the upstream

#### Scenario: PHP authentication verbs are the only executable auth guidance

- **GIVEN** Bitbucket authentication guidance is rendered or reviewed
- **WHEN** the inspection and remediation commands are checked
- **THEN** inspection uses `bb auth show`
- **AND** remediation uses the documented `bb auth save` flow
- **AND** guidance does not publish `bb auth login` or `bb auth switch`

#### Scenario: Verified PHP PR methods are used without inferred flags

- **GIVEN** Bitbucket PR guidance covers creation, inspection, or merging
- **WHEN** its commands are checked against the verified PHP surface
- **THEN** the methods are `bb pr create`, `bb pr show`, and `bb pr merge`
- **AND** executable options are limited to options verified by authoritative upstream documentation or the allowed read-only probes
- **AND** any unverified option or output contract is marked as an open verification gap by reference to the proposal's **Open verification gaps** table
- **AND** agents stop with guidance instead of executing an invocation whose option contract remains unverified

#### Scenario: Unsafe help execution is not used for verification

- **GIVEN** a PHP `bb-cli` PR method has an option or output contract that is not verified
- **WHEN** the recipe contract is validated
- **THEN** verification relies on authoritative upstream documentation or the allowed read-only probes (`bb pr list` and `bb pr show`)
- **AND** the validation does not execute `bb pr create --help` or infer flags from its result

#### Scenario: Agent-facing auth checks emit only Username

- **GIVEN** the Bitbucket skill or create command runs an authentication check
- **WHEN** `bb auth show` is captured
- **THEN** the agent-facing output contains the `Username` line
- **AND** `AppPassword` is not printed
- **AND** a missing `Username` line is rejected
- **AND** more than one `Username` line is rejected

#### Scenario: Foreign bb binary is blocked with PHP bb-cli guidance

- **GIVEN** some `bb` is on `PATH` but it is not PHP `bb-cli`
- **WHEN** the Bitbucket identity guard runs
- **THEN** the workflow is blocked
- **AND** the guidance names Homebrew formula `bb-cli` and https://bb-cli.github.io
- **AND** the guidance does not propose `brew install bb`

### Requirement: Bitbucket feature remote-branch deletion after merge

After a merged feature-head pull request, the `bitbucket-pr-flow` merge skill
MUST explicitly delete the feature remote branch (`git push $REMOTE --delete`
or the equivalent worktree-cleanup remote-deletion step) while the head is a
feature branch. Protected heads (`main`, `master`, `development`, `staging`,
the configured base branch, and the configured integration branch when set)
MUST NOT be deleted via Bitbucket close-source UI, worktree cleanup, local
branch deletion, or remote-branch deletion.

#### Scenario: Feature remote branch is deleted after merge

- **GIVEN** a merged Bitbucket PR whose source is a feature head
- **WHEN** post-merge cleanup runs
- **THEN** the skill deletes the feature remote branch
- **AND** protected-head exclusions remain in force

#### Scenario: Protected heads are never remotely deleted

- **GIVEN** the merged PR source is a protected head
- **WHEN** post-merge cleanup is considered
- **THEN** the workflow does not delete that head locally or on the remote

## MODIFIED Requirements

### Requirement: Bitbucket (bb) Auth Preflight

When the bound provider is `bitbucket-pr-flow` and `expected_owner` is set, the preflight MUST
detect the active account via a redacted `bb auth show` capture (NOT `bb auth status`),
compare the single `Username` value to `expected_owner`, and block with actionable
`bb auth save` guidance when it does not match. The capture MUST emit only `Username`,
MUST reject a missing or multiple `Username` line, and MUST NEVER print `AppPassword`.
The preflight MUST NOT attempt `bb auth switch`, because the PHP `bb-cli`
contract has no such method. When `expected_owner` is unset or empty, the general
`Auth Preflight Gating` requirement applies and the Bitbucket auth command MUST NOT
run.

#### Scenario: bb auth show replaces bb auth status

- **GIVEN** the `bitbucket-pr-flow` recipe's bb-pr-create.md command
- **WHEN** the authentication verification step is reviewed
- **THEN** the command used is `bb auth show` (not `bb auth status`)

#### Scenario: bb account matches — proceed

- **GIVEN** `expected_owner = "myworkspace"` and `bb auth show` reports `myworkspace`
- **WHEN** the Bitbucket auth preflight runs
- **THEN** no account switch or save operation is attempted
- **AND** the workflow proceeds to the next preflight or PR step

#### Scenario: bb account mismatch — block with PHP remediation

- **GIVEN** `expected_owner = "myworkspace"` and `bb auth show` reports `other`
- **WHEN** the Bitbucket auth preflight runs
- **THEN** the workflow is blocked
- **AND** the blocker identifies the current and expected accounts
- **AND** the guidance directs the user to the documented `bb auth save` flow
- **AND** the workflow does not attempt `bb auth login` or `bb auth switch`

#### Scenario: Empty expected_owner skips Bitbucket auth inspection

- **GIVEN** `expected_owner = ""` or the setting is absent
- **WHEN** the Bitbucket preflight step is reached
- **THEN** `bb auth show` is not run
- **AND** no account comparison or remediation is attempted
- **AND** the workflow proceeds without the additional account-match gate
