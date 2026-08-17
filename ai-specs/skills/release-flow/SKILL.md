---
name: release-flow
description: >
  Dogfooded release ritual for ai-specs-cli: version bump, promote to main,
  tag, and GitHub release. Trigger: when cutting a release, shipping vX.Y.Z,
  promoting development to main, or tagging a version.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Cutting a release or shipping vX.Y.Z"
    - "Promoting development to main for a release"
    - "Tagging a version and creating a GitHub release"
---

# Release Flow

Project-local playbook for shipping ai-specs-cli. This is **not** part of the
`vcs-pr-flow` capability — use the bound VCS skill for PR/merge primitives;
this skill owns product release policy (VERSION, CHANGELOG, promote, tag).

## When to use

- User asks to release, cut a version, ship `vX.Y.Z`, or promote to `main`
- After features land on `development` and the tree is ready to publish
- When recreating the release steps that agents otherwise rediscover from memory

## Critical rules

1. **Never** open the promote PR with `development` (or `main`/`staging`) as
   the head. Use a disposable `release/vX.Y.Z` head into `main`.
2. **Never** push directly to `main` or `development` — PR + explicit approval.
3. Bump only `VERSION` and `CHANGELOG.md` on the bump branch (no feature work).
4. Hard stop for explicit user approval before: promote merge, tag push, and
   publishing release notes. The tag push is the irreversible step — it is what
   builds and attaches the gate binaries.
5. Before promote, run the VCS `delete_branch_on_merge` preflight (GitHub). If
   it is `true`, warn — long-lived heads can be wiped when used as a PR head.
6. After merge, delete only **feature** heads (`chore/release-*`, `release/v*`).
   Never delete protected heads.

Before the version bump (step 1 of the ritual), the isolated
clean-materialization gate must pass. Evidence is
`tests/test_release_materialization.py` (isolated temp consumer project
+ in-tree CLI), not this repo's dogfood `ai-specs/.ai-specs.lock`. Do
**not** treat a stale dogfood lock as release evidence.

## Ritual

```text
decide bump → chore/release-vX.Y.Z (VERSION+CHANGELOG)
  → PR → development (squash OK)
  → reset release/vX.Y.Z from origin/development
  → PR → main (merge commit)
  → annotated tag + gh release
  → cleanup feature heads
```

### 1. Decide the bump

From repo root (or release worktree):

```bash
cat VERSION
git describe --tags --abbrev=0
git log "$(git describe --tags --abbrev=0)..origin/development" --oneline
```

Choose major / minor / patch from `[Unreleased]` in `CHANGELOG.md` and commits
since the last tag. Confirm with the user if unclear.

### 2. Bump branch

Worktree off `development` (do not edit protected branches in the main tree):

```bash
git fetch origin development
git worktree add .worktrees/release-vX.Y.Z -b chore/release-vX.Y.Z origin/development
```

In that worktree:

1. Set `VERSION` to `X.Y.Z` (no `v` prefix).
2. Move `CHANGELOG.md` `[Unreleased]` notes into `## [X.Y.Z] — YYYY-MM-DD`.
3. Leave a fresh empty `## [Unreleased]` section at the top.
4. **Decide whether this release needs an upgrade notice** (see below).
5. Commit: `chore(release): bump to X.Y.Z`.

#### Upgrade notes (post-upgrade actions)

If a user must *do* something after upgrading, say so in an `### Upgrade notes`
subsection under the new version heading. `ai-specs upgrade` replays it for
everyone who crosses this version, oldest release first.

```markdown
## [X.Y.Z] — YYYY-MM-DD

### Upgrade notes
Run `ai-specs sync` in each project to acquire the verified Go worktree-gate
binary. Until you do, the gate keeps falling back to the Bash implementation.
```

Rules:

- **Prose only.** `upgrade` runs against `~/.ai-specs` and has no consumer
  project in scope, so it cannot evaluate project-dependent conditions. Name
  the command; let `ai-specs doctor` handle anything conditional.
- **Nothing is executed.** A notice is displayed verbatim.
- Write one only when an action is genuinely required. A notice on every
  release trains users to skip them.

Ask: *would a user who upgrades and does nothing else be silently worse off?*
If yes, write the notice.

### 3. PR into development

```bash
git push -u origin chore/release-vX.Y.Z
gh pr create --base development --head chore/release-vX.Y.Z \
  --title "chore(release): bump to X.Y.Z" \
  --body "$(cat <<'EOF'
## Summary
- Bump VERSION to X.Y.Z
- Document changes in CHANGELOG.md

## Test plan
- [ ] VERSION reads X.Y.Z
- [ ] After merge: promote via release/vX.Y.Z → main, tag, gh release

EOF
)"
```

Squash-merge is fine for the bump PR (after user approval). Pull
`origin/development` after it lands.

### 4. Promote head (disposable)

```bash
git fetch origin development
git branch -f release/vX.Y.Z origin/development
git push -u origin release/vX.Y.Z
```

Run delete_branch_on_merge preflight via the bound VCS merge skill, then:

```bash
gh pr create --base main --head release/vX.Y.Z \
  --title "chore(release): promote vX.Y.Z to main" \
  --body "$(cat <<'EOF'
## Summary
Promote development tip (includes VERSION X.Y.Z) to main for release.

## Test plan
- [ ] Merge with merge commit (not squash)
- [ ] Tag vX.Y.Z on main
- [ ] Release workflow green and 4 gate assets attached
- [ ] gh release edit — title and notes

EOF
)"
```

Merge **only** after explicit user approval, using a **merge commit**
(not squash):

```bash
gh pr merge <n> --merge --delete-branch
```

`--delete-branch` is safe here: `release/vX.Y.Z` is a feature head.

### 5. Tag and GitHub release

On `main` after the promote lands:

```bash
git fetch origin main
git checkout -B main origin/main
git tag -a "vX.Y.Z" -m "vX.Y.Z"
git push origin "vX.Y.Z"
```

**The tag push creates the release — you do not.** Pushing `v*` triggers
`.github/workflows/release-worktree-gate.yml`, which builds the four gate
binaries, verifies them against the committed
`catalog/recipes/worktree-flow/bin/SHA256SUMS`, and attaches them via
`softprops/action-gh-release`. That action creates the GitHub Release for the
tag, with an empty body and the bare tag as its title.

So `gh release create` **fails with "already exists"**. Wait for the workflow,
then fill in the release:

```bash
gh run watch <run-id> --exit-status          # 6 jobs, all must pass
gh release view vX.Y.Z --json assets          # expect 4 worktree-gate-* assets

gh release edit "vX.Y.Z" --title "vX.Y.Z — <short summary>" \
  --notes-file - <<'EOF'
## Highlights
- <from CHANGELOG>

## Install
curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
EOF
```

Verify the published assets match the trust root without downloading them —
useful when a sandbox blocks the asset CDN:

```bash
gh api repos/parada1104/ai-specs-cli/releases/tags/vX.Y.Z \
  --jq '.assets[] | "\(.digest)  \(.name)"'
```

Before tagging, reproduce the digests locally with the CANONICAL toolchain
(`go1.24.13`): run `scripts/build-gate.sh`, then compare `shasum -a 256` of the
four `dist/worktree-gate-<os>-<arch>` files against the committed `SHA256SUMS`.
Exclude `dist/worktree-gate-current` — CI does not build it. A mismatch means
the release workflow will fail and publish **no** assets, leaving every user on
the Bash fallback.

Installer default ref is `main` (`AI_SPECS_REF=main`). Point release notes at
repo-root `install.sh`, not a nested path.

### 6. Cleanup

```bash
git worktree remove .worktrees/release-vX.Y.Z 2>/dev/null || true
git branch -D chore/release-vX.Y.Z release/vX.Y.Z 2>/dev/null || true
git push origin --delete chore/release-vX.Y.Z 2>/dev/null || true
# release/v* may already be deleted by --delete-branch on the promote PR
```

Do **not** delete `development` or `main`.

## Post-release checklist

- [ ] `VERSION` on `main` matches the tag
- [ ] GitHub Release exists and notes look right
- [ ] `development` still exists on origin
- [ ] Feature release branches/worktrees removed
- [ ] Optional: sync any post-tag fixes back to `development` via PR if needed

## Relation to other skills

| Skill / capability | Role |
|---|---|
| `vcs-pr-flow` (`git-merge-workflow`) | PR create/merge, protected vs feature heads, account + delete_branch_on_merge preflight |
| `worktree-flow` | Isolated worktrees for bump and cleanup |
| `release-flow` (this skill) | Semver bump, promote shape, tag, GitHub release notes |
