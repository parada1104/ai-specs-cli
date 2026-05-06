## Context

`ai-specs` is installed globally via `install.sh`, which clones the repository to `~/.ai-specs` and symlinks `bin/ai-specs` into `~/.local/bin`. There is no built-in mechanism for users to update this global checkout safely. Today, users must manually `cd ~/.ai-specs && git pull`, which risks dirty-tree conflicts, divergence from `origin/main`, and accidental mutation of a dev checkout. The Trello card requires a first-class `ai-specs upgrade` command that detects the installation channel, validates pre-flight conditions, performs a safe fast-forward, and verifies the result.

The existing command architecture is: `bin/ai-specs` dispatches to `lib/<command>.sh`. Commands like `install.sh` are pure Bash orchestrators around git and filesystem operations. `lib/version.sh` reads the `VERSION` file. Tests exercise Bash scripts via Python `subprocess` calls. This design must fit into that pattern.

## Goals / Non-Goals

**Goals:**
- Add `ai-specs upgrade [--dry-run] [--force]` as a top-level CLI command.
- Detect whether the running binary belongs to a valid global install (`~/.ai-specs`) or a dev/local checkout.
- Refuse to mutate non-standard / dev checkouts.
- Verify pre-flight conditions: on a fast-forwardable branch, `origin/main` reachable, working tree clean (unless `--force`).
- Perform the upgrade with `git fetch origin main && git merge --ff-only origin/main`.
- After a successful pull, verify the version changed and print an old-to-new diff.
- Verify symlink integrity at `~/.local/bin/ai-specs` post-upgrade.
- Support `--dry-run` to preview changes without mutating the repository.
- Cover behavior with tests before implementation.

**Non-Goals:**
- Do not implement auto-repair for broken installations; only recommend re-running `install.sh`.
- Do not support upgrading to arbitrary refs or tags in MVP; target is always `origin/main`.
- Do not add a new manifest schema or Python parser for this command.
- Do not mutate dev checkouts even with `--force`; dev checkouts are blocked unconditionally.
- Do not require network access for the base detection logic; only fetch/ls-remote need network.

## Decisions

### Decision 1: Implement `upgrade.sh` as pure Bash

`lib/upgrade.sh` should be a pure Bash script, following the pattern of `install.sh` and `version.sh`. It will orchestrate git commands, resolve symlinks, read the `VERSION` file, and print structured output directly.

**Rationale:** The upgrade flow is a linear git workflow with filesystem checks, not a matrix of structured validations. Bash is sufficient for symlink resolution (`readlink` / BASH_SOURCE loop), git porcelain checks, and the `VERSION` read. Keeping it in one file matches `install.sh` and avoids indirection.

**Alternatives considered:**
- **Bash wrapper + Python helper:** Would improve cross-platform path handling and make unit testing individual functions easier. Rejected because the logic is straightforward subprocess orchestration; Python would add a file and indirection without reducing complexity. The existing test suite already exercises Bash scripts via `subprocess`.
- **Pure Python entrypoint:** Would diverge from the existing `lib/*.sh` command convention and require changing `bin/ai-specs` dispatch logic.

### Decision 2: Detect global install by resolving the script's real path

`upgrade.sh` will resolve its own real filesystem path by walking symlinks (replicating the loop already present in `bin/ai-specs`). It will then verify, in order:
1. The resolved path lies inside `$HOME/.ai-specs`.
2. `AI_SPECS_HOME` is set and points to the same directory.
3. `$HOME/.ai-specs/.git` exists.
4. `~/.local/bin/ai-specs` is a symlink that resolves into `$HOME/.ai-specs`.

If the resolved binary lives outside `$HOME/.ai-specs`, the command aborts with a dev-channel message. If any of the four checks fail, it aborts with a broken-install message recommending `install.sh`.

**Rationale:** This is deterministic and uses the same symlink-walking logic already shipped in `bin/ai-specs`. It protects both against accidental dev-checkout mutation and against partially broken installs.

**Alternatives considered:**
- **Pass detection metadata from `bin/ai-specs` via environment variable:** Rejected because `lib/upgrade.sh` should be self-contained and testable in isolation.
- **Rely only on `AI_SPECS_HOME`:** Rejected because the env var could be stale or point to a different directory than the running binary.

### Decision 3: Dirty working tree blocks by default; `--force` overrides

If `git status --porcelain` reports any changes, the command aborts with an explicit error listing the dirty state and suggesting `--force` or `git stash`. When `--force` is passed, the command prints a warning about the dirty tree and proceeds with the fast-forward pull, but only after all other pre-flight checks pass.

**Rationale:** Safe-by-default prevents users from losing uncommitted work or encountering merge surprises. `--force` is an explicit opt-in. Auto-stash was rejected because it mutates state implicitly and could leave the user confused about where their changes went.

**Alternatives considered:**
- **Auto-stash → pull → stash pop:** Rejected because it is mutating and surprising; a failed pop would leave the repo in an awkward state.
- **Warn but continue:** Rejected because a dirty tree makes `--ff-only` pulls unreliable and could produce conflicts that break a non-technical user's global CLI.

### Decision 4: Dry-run skips fetch and merge; targets cached `origin/main` for version preview

In `--dry-run` mode, the command performs all read-only detection and pre-flight checks, reads the current `VERSION`, and attempts to read the target version from the locally cached `origin/main` ref (`git show origin/main:VERSION`). If `origin/main` has never been fetched, the target version is reported as `unknown` and the user is advised that a real run will fetch it. The command explicitly states that no changes were made and exits 0.

**Rationale:** The spec prohibits fetch, merge, or any write in dry-run. Using the cached remote ref gives a best-effort version preview without violating that constraint. `git ls-remote` could theoretically query the remote read-only, but it introduces a network dependency and still does not give us the `VERSION` file content without fetching the object. Keeping dry-run fully offline-after-detection keeps it predictable and fast.

**Alternatives considered:**
- **Perform `git fetch` anyway because it does not touch the working tree:** Rejected because the spec explicitly says "MUST NOT fetch".
- **Use `git ls-remote origin main` to show the target SHA:** Rejected for MVP because the SHA is less user-friendly than the version string, and the command still cannot read `VERSION` from that SHA without fetching.

## Flow

```text
User
  |
  | ai-specs upgrade [--dry-run] [--force]
  v
bin/ai-specs
  |
  | dispatch upgrade
  v
lib/upgrade.sh
  |
  | resolve real path of this script (symlink walk)
  | verify path is inside ~/.ai-specs
  | verify AI_SPECS_HOME matches
  | verify ~/.ai-specs/.git exists
  | verify ~/.local/bin/ai-specs symlink points into ~/.ai-specs
  | abort if dev or broken install
  v
pre-flight checks
  |
  | git rev-parse --abbrev-ref HEAD
  | git merge-base --is-ancestor HEAD origin/main  (or check branch tracking)
  | git status --porcelain  (blocks unless --force)
  | read current VERSION
  v
[dry-run?]
  |--yes--> attempt git show origin/main:VERSION
  |         print preview (current -> target)
  |         exit 0
  |--no---> git fetch origin main
            git merge --ff-only origin/main
            read new VERSION
            compare old vs new
            verify ~/.local/bin/ai-specs symlink still valid
            print upgrade summary
            exit 0
```

## Risks / Trade-offs

- **[Risk]** Symlink resolution differs between GNU and BSD `readlink` → **Mitigation:** Use the same pure-Bash symlink-walking loop already present in `bin/ai-specs` instead of relying on `readlink -f`.
- **[Risk]** A user with a dirty working tree and `--force` could lose work if the fast-forward touches tracked files → **Mitigation:** The warning message explicitly says the tree is dirty; `--force` is the user's explicit choice.
- **[Risk]** Network failure during `git fetch` produces a confusing error → **Mitigation:** Capture and surface git stderr; suggest checking network or running `install.sh` if the remote is unreachable.
- **[Risk]** The local branch has diverged from `origin/main` → **Mitigation:** `--ff-only` guarantees we never create a merge commit; the error message recommends manual resolution or a fresh `install.sh`.
- **[Risk]** Post-upgrade symlink breakage if `bin/ai-specs` is renamed in a future release → **Mitigation:** Post-upgrade check warns and recommends `install.sh`; this also serves as an early signal for release process mistakes.
- **[Risk]** Dry-run cannot show the target version on first install because `origin/main` has not been fetched → **Mitigation:** Documented limitation; dry-run is most useful after the first fetch, which is acceptable for MVP.

## Migration Plan

1. Add tests that define upgrade behavior for:
   - valid global install (clean and dirty, with and without `--force`)
   - dev checkout rejection
   - broken install rejection
   - dry-run preview
   - non-fast-forward blockage
   - post-upgrade version diff and symlink check
2. Wire `upgrade` into `bin/ai-specs` dispatch and help output.
3. Add `lib/upgrade.sh` with detection, pre-flight, pull, and verification logic.
4. Run focused tests with `./tests/run.sh`, then full validation with `./tests/validate.sh`.

Rollback is file-level and safe: remove the CLI dispatch/help entry and delete `lib/upgrade.sh` and the tests.

## Open Questions

- Should a future `--check` flag report whether an upgrade is available without performing it, perhaps by fetching and comparing versions?
- Should dry-run use `git ls-remote origin main` as a read-only network fallback when `origin/main` is not cached locally, even though it cannot read `VERSION` from the remote commit?
- Should the command support upgrading to a specific tag or release branch (e.g., `ai-specs upgrade --to v2.0.0`) in a future iteration?
