# Proposal: readable upgrades with version-keyed notices and a narrowed install

## Tracker

- card_id: `#80`
- url: https://trello.com/c/x7Pj31vF/80-upgrade-ux-compact-output-changelog-surfacing-version-keyed-upgrade-notices

## Depth

**Full.** Introduces a new capability (upgrade notices contract) and changes the
distribution channel (checkout shape). Neither is a bounded edit in a known
area.

## Why

`ai-specs upgrade` currently forwards raw git output to the terminal. The
`0.21.0 -> 0.22.0` upgrade printed 243 file names and a transfer log, and told
the user nothing about what changed or what to do next.

Two distinct problems hide behind that noise:

1. **The signal is absent, not just buried.** Even a perfectly quiet upgrade
   would not tell a user that `0.22.0` requires `ai-specs sync` to acquire the
   verified Go gate binary. A user who upgrades and never syncs stays on the
   Bash fallback silently; `doctor` classifies that exact state as ERROR. A
   fast-forward diffstat structurally cannot carry that message.

2. **The install carries what it never runs.** `openspec/` and `tests/` are 47%
   of tracked files and 35% of tracked bytes in `~/.ai-specs`, and nothing in
   `lib/` or `bin/` reads either at runtime.

The upgrade *logic* is not the problem and is not being changed. Its
install-channel detection, divergence guard, dirty-tree handling and symlink
verification stay exactly as they are.

## What changes

### 1. Compact output (apply an existing in-house convention)

Adopt the `run_step` pattern already proven in `lib/sync.sh:119` and
`lib/sync-agent.sh:235`: one labelled line per step, captured output, full dump
on failure, `-v/--verbose` for detail. This is deliberately not a new design.

### 2. Version crossing summary

`upgrade.sh` already holds `CURRENT_VERSION` (185) and `NEW_VERSION` (246).
Print the `CHANGELOG.md` sections crossed by the upgrade instead of a file
list.

### 3. Upgrade notices (new capability)

A release may declare an action the user must take after upgrading. Notices are
authored in `CHANGELOG.md` under the version they belong to, replayed for every
version in the crossed range, and are **unconditional prose**.

The prose constraint is forced by the runtime, not chosen for simplicity:
`upgrade` operates on `~/.ai-specs` and has no consumer project in scope, so it
cannot evaluate project-dependent conditions. Conditional guidance stays in
`doctor`, which has that state. A notice points at the command; it never tries
to be the command.

### 4. Narrowed checkout

Install with `--filter=blob:none` plus a cone-mode sparse checkout that excludes
`openspec/`, `tests/`, `.github/` and `tmp/`. Partial clone rather than shallow,
because `--depth` breaks the `git merge-base --is-ancestor` divergence guard at
`upgrade.sh:149` and `:211`.

Existing full installs narrow idempotently on upgrade. Narrowing is best-effort:
on any unsupported Git, failure, or ambiguity it falls back to today's full
checkout and the upgrade still succeeds.

## Success criteria

1. A successful upgrade prints no raw git output by default; `-v`/`--verbose`
   restores it, and a failing step prints everything it produced.
2. Every existing upgrade safety check, abort condition, and exit code (1–5)
   behaves exactly as before.
3. A successful upgrade summarizes the CHANGELOG versions crossed, and an
   unreadable or malformed CHANGELOG degrades to the plain version line without
   failing the upgrade.
4. A release can declare an `### Upgrade notes` action that is replayed for
   every crossed version, oldest first, displayed and never executed.
5. The global install excludes `openspec/`, `tests/`, `.github/` and `tmp/`
   while retaining every runtime path and the full commit history.
6. Narrowing is best effort: an unsupported git, a dirty tree, or any failure
   leaves a usable checkout and never blocks install or upgrade.

## Non-goals

- Changing any upgrade safety check, exit code, or abort condition.
- Replacing the git-checkout distribution channel with packaged artifacts.
- Runtime-brief / `AGENTS.md` ownership (separate change, card #81).
- Evaluating consumer-project state from `upgrade`.

## Capabilities

### New

- `upgrade-experience` — output contract, version crossing summary, notices
  contract, and install footprint policy.

### Modified

None. No existing capability changes behavior.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `lib/upgrade.sh` | Modified | Step output, changelog summary, notice replay, narrowing |
| `install.sh` | Modified | Partial clone + sparse checkout with fallback |
| `lib/_internal/` | New | Changelog/notice parser shared by upgrade |
| `CHANGELOG.md` | Convention | `### Upgrade notes` subsection per version |
| `ai-specs/skills/release-flow/SKILL.md` | Modified | Authoring notices becomes part of the ritual |
| `tests/` | New | Parser, replay range, fallback, and narrowing coverage |

## Risks

| Risk | Mitigation |
|---|---|
| Sparse checkout hides a path something reads at runtime | Enumerate reads before excluding; keep exclusion list minimal and reversible; doctor check |
| Old Git lacks `--filter` / cone mode | Detect and fall back to full checkout; never block the upgrade |
| Notices drift from releases | Author inside `CHANGELOG.md`, which the release ritual already updates |
| Changelog parsing breaks on format drift | Parse defensively; a parse failure degrades to the plain version line, never aborts |
| Narrowing an existing install surprises a contributor | Dev checkouts are already rejected by the channel guard at `upgrade.sh:88` |
