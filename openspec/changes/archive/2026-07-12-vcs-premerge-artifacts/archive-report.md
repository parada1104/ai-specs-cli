# Archive report — vcs-premerge-artifacts

**Archived:** 2026-07-12
**Branch:** feat/vcs-premerge-artifacts
**PR:** #106
**Status:** ready-to-merge

## Outcome

- Pre-merge archive contract promoted to `openspec/specs/vcs-pr-flow/spec.md`.
- Provider merge-workflow skills (git/gitlab/bitbucket) mirror archive-before-merge + `tasks.md` PR gate.
- Worktree-flow SDD artifact row updated; dogfood pins and lock hashes refreshed.

## Verification

- `./tests/run.sh` — 768 OK
- `./tests/validate.sh` — OK

## Specs synced

| Capability | Action |
|------------|--------|
| `vcs-pr-flow` | Updated — pre-merge archive requirement |

## Archive move

- Source: `openspec/changes/vcs-premerge-artifacts/`
- Destination: `openspec/changes/archive/2026-07-12-vcs-premerge-artifacts/`
