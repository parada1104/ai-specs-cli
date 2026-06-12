# Design: trello-card-24 Candidate Base Resolution

## Technical Approach

Extend the canonical cleanup script only at `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`. Insert small helpers immediately above `is_merged` and replace the body around lines 99-110 with candidate loops; do not refactor worktree parsing/removal.

Function shape:

```bash
debug_log() { ... }                                  # stderr, env-gated
resolve_base_candidates() { local base="$1"; ... } # prints valid refs
candidate_has_merged_tip() { local sha="$1" candidate="$2"; ... }
candidate_has_patch_equivalence() { local sha="$1" candidate="$2"; ... }
is_merged() { local sha="$1" base="$2" candidate; ... }
```

Minimal diff shape:

```diff
+ resolve_base_candidates() { ... }
+ candidate_has_merged_tip() { git merge-base --is-ancestor "$sha" "$candidate" 2>/dev/null; }
+ candidate_has_patch_equivalence() { # move existing lines 107-110 here }
  is_merged() {
      local sha="$1" base="$2"
-     if git merge-base --is-ancestor "$sha" "$base" 2>/dev/null; then return 0; fi
-     if [[ -n "$(git rev-list "$base..$sha" 2>/dev/null)" ]]; then ... fi
+     while read -r candidate; do candidate_has_merged_tip "$sha" "$candidate" && debug_log "merged by ancestry: $candidate" && return 0; done < <(resolve_base_candidates "$base")
+     while read -r candidate; do candidate_has_patch_equivalence "$sha" "$candidate" && debug_log "merged by patch-id: $candidate" && return 0; done < <(resolve_base_candidates "$base")
      return 1
  }
```

Candidate order is exact `--base`, configured upstream of that base, then configured remote-tracking `<remote>/<base>`. Patch-id remains the final fallback; with no upstream or remote candidate, local-only branches run the same `git cherry "$base" "$sha"` behavior as today.

## Bash Correctness Details

- Upstream detection: `upstream=$(git rev-parse --verify --quiet --abbrev-ref "${base}@{u}" 2>/dev/null)`; validate with `git rev-parse --verify --quiet "$upstream" >/dev/null` before printing.
- Remote detection: resolve `remote` from `git config --get "branch.${base}.remote"`; if absent, use `origin` only when `git config --get remote.origin.url` proves that remote exists. Validate `refs/remotes/${remote}/${base}` with `git rev-parse --verify --quiet "refs/remotes/${remote}/${base}" 2>/dev/null`.
- Every lookup is best-effort: missing refs return non-zero silently and only skip that candidate.
- De-duplicate candidates in Bash before printing to avoid repeated checks.
- `debug_log` is disabled by default, e.g. `WORKTREE_CLEANUP_DEBUG=1`, so stable stdout remains greppable while operators can see which candidate justified cleanup.
- No `git fetch`, network access, or inferred remote freshness.

## Test Strategy

Use the existing Python `unittest` file `tests/test_worktree_cleanup.py`; `tests/run.sh` already runs `python3 -m unittest discover -s tests -p 'test_*.py'`, and current cleanup tests use `tempfile` plus `subprocess` against the canonical template.

Add hermetic helpers for synthetic refs: `git init --bare`, local temp repos, `git commit-tree`, and `git update-ref refs/remotes/<remote>/main <sha>` to simulate remote-only evidence without network.

Required scenario mapping:

| Test function | Scenario |
|---|---|
| `test_detects_regular_merge_on_remote_base_with_stale_local_base` | remote merge commit proves ancestry |
| `test_removes_squash_merged_worktree` | existing patch-id squash path |
| `test_removes_rebase_merged_worktree_by_patch_id` | rebase patch-id path |
| `test_removes_fast_forward_merged_worktree` | fast-forward ancestry |
| `test_preserves_local_only_unmerged_branch_without_remote_candidate` | local-only fallback stays conservative |
| `test_preserves_branch_ahead_of_base` | unlanded commits remain unmerged |
| `test_removes_remote_deleted_branch_when_local_base_contains_tip` | remote deletion irrelevant when local base proves merge |
| `test_preserves_dirty_worktree` | dirty skip precedes merge detection |
| `test_missing_remote_candidate_does_not_fetch` | bounded local refs only |

Keep existing dry-run/removal smoke coverage and include conflict-resolution merge coverage either inside the remote regular merge helper or as a tenth focused test if the fixture stays small.

## Materialized Script Handling

`ai-specs/ai-specs.toml` enables `worktree-flow` version `1.2.0`; `recipe.toml` materializes the script to `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` with `condition = "not_exists"`. This worktree lacks that materialized file, so the implementation should not hand-create it or require `ai-specs sync`. Commit the canonical template and tests; if a local sync later creates a tracked/drifted materialized copy, add the optional chore commit to refresh it.

## Commit Split

1. `test(recipe): add hermetic tests for worktree-cleanup candidate base resolution` — RED, captures the false negative and conservative edge cases first.
2. `fix(recipe): resolve upstream and remote base candidates in worktree-cleanup heuristic` — GREEN, minimal helper/body change.
3. Optional `chore(sync): refresh materialized worktree-cleanup.sh if drift detected` — only if sync creates or updates a tracked copy.

## Risks and Mitigations

- Drift: canonical template is tested; materialized copy is refreshed only when present/tracked.
- False positive cleanup: require positive ancestry or patch-id proof; never infer from branch names or remotes.
- Missing refs: all candidate resolution is quiet and falls through to the existing patch-id behavior.
- Stale remote-tracking refs: accepted local evidence only; no fetch side effects.
