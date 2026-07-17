# Tasks: protected-merge-heads

Depth: standard

## Checklist

- [x] Update `git-merge-workflow`: protected vs feature head cleanup + GitHub `delete_branch_on_merge` preflight
- [x] Update `gitlab-merge-workflow` / `bitbucket-merge-workflow`: same protected-head policy (no GitHub API block)
- [x] Dogfood runtime skill via catalog (`.claude/skills` is gitignored symlink to sync cache)
- [x] README + `docs/recipes-catalog.md` long-lived branch notes
- [x] Golden tests for protected-head policy + `delete_branch_on_merge` needles
- [x] Dogfood: `delete_branch_on_merge=false` on `parada1104/ai-specs-cli`
- [x] `./tests/run.sh` and `./tests/validate.sh` green
