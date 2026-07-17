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
   `gh release create`.
5. Before promote, run the VCS `delete_branch_on_merge` preflight (GitHub). If
   it is `true`, warn — long-lived heads can be wiped when used as a PR head.
6. After merge, delete only **feature** heads (`chore/release-*`, `release/v*`).
   Never delete protected heads.

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
4. Commit: `chore(release): bump to X.Y.Z`.

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
- [ ] gh release create

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
gh release create "vX.Y.Z" --title "vX.Y.Z — <short summary>" \
  --notes-file - <<'EOF'
## Highlights
- <from CHANGELOG>

## Install
curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
EOF
```

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
