"""Behavior tests for the worktree-flow cleanup script.

The script lives in the catalog recipe and is materialized into consumer
projects. It removes git worktrees under a configured directory whose branch
is fully merged into the integration branch, while preserving worktrees that
have uncommitted changes or unmerged branches.
"""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLEANUP_SCRIPT = (
    ROOT / "catalog" / "recipes" / "worktree-flow" / "templates" / "worktree-cleanup.sh"
)

import sys as _sys
from pathlib import Path as _Path
_sys.path.insert(0, str(_Path(__file__).resolve().parent))
from test_repo_topology import make_super_with_submodule


def git(repo: Path, *args: str) -> str:
    env = dict(os.environ)
    env.update(
        {
            "GIT_AUTHOR_NAME": "t",
            "GIT_AUTHOR_EMAIL": "t@t",
            "GIT_COMMITTER_NAME": "t",
            "GIT_COMMITTER_EMAIL": "t@t",
        }
    )
    return subprocess.run(
        ["git", *args],
        cwd=repo,
        env=env,
        check=True,
        capture_output=True,
        text=True,
    ).stdout


class WorktreeCleanupTests(unittest.TestCase):
    def _make_repo(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        repo = Path(tmp.name) / "repo"
        repo.mkdir()
        git(repo, "init", "-q", "-b", "main")
        (repo / "README.md").write_text("base\n")
        (repo / ".gitignore").write_text(".worktrees/\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "init")
        return repo

    def _add_worktree(self, repo: Path, branch: str) -> Path:
        wt = repo / ".worktrees" / branch
        git(repo, "worktree", "add", "-q", "-b", branch, str(wt), "main")
        return wt

    def _run_cleanup(self, repo: Path, *extra: str):
        return subprocess.run(
            ["bash", str(CLEANUP_SCRIPT), "--base", "main", *extra],
            cwd=repo,
            check=True,
            capture_output=True,
            text=True,
        )

    def test_removes_merged_worktree(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-merged")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(repo, "merge", "-q", "--no-ff", "-m", "merge", "feat-merged")

        out = self._run_cleanup(repo)

        self.assertFalse(wt.exists(), "merged worktree directory should be removed")
        branches = git(repo, "branch", "--format=%(refname:short)")
        self.assertNotIn("feat-merged", branches.split())
        self.assertIn("removed feat-merged", out.stdout)

    def test_removes_squash_merged_worktree(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-squash")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        # squash merge: base gets a NEW commit with the same diff, so the
        # branch tip is NOT an ancestor of main (the squash-merge blind spot).
        git(repo, "merge", "-q", "--squash", "feat-squash")
        git(repo, "commit", "-qm", "squash: feat-squash")

        # Sanity: ancestry alone would not detect this as merged.
        self.assertNotEqual(
            subprocess.run(
                ["git", "merge-base", "--is-ancestor", "feat-squash", "main"],
                cwd=repo,
            ).returncode,
            0,
        )

        out = self._run_cleanup(repo)

        self.assertFalse(wt.exists(), "squash-merged worktree should be removed")
        self.assertIn("removed feat-squash", out.stdout)

    def test_removes_multi_commit_regular_merge(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-multi-regular")
        (wt / "one.txt").write_text("one\n")
        git(wt, "add", "one.txt")
        git(wt, "commit", "-qm", "feat: first change")
        (wt / "two.txt").write_text("two\n")
        git(wt, "add", "two.txt")
        git(wt, "commit", "-qm", "feat: second change")
        git(repo, "merge", "-q", "--no-ff", "-m", "merge multi-commit feature", "feat-multi-regular")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(wt.exists(), "dry-run must preserve the worktree")
        self.assertIn("would remove feat-multi-regular", out.stdout)

    def test_removes_multi_commit_squash_merge(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-multi-squash")
        (wt / "one.txt").write_text("one\n")
        git(wt, "add", "one.txt")
        git(wt, "commit", "-qm", "feat: first change")
        (wt / "two.txt").write_text("two\n")
        git(wt, "add", "two.txt")
        git(wt, "commit", "-qm", "feat: second change")
        git(repo, "merge", "-q", "--squash", "feat-multi-squash")
        git(repo, "commit", "-qm", "squash: multi-commit feature")
        (repo / "base-extra.txt").write_text("unrelated base change\n")
        git(repo, "add", "base-extra.txt")
        git(repo, "commit", "-qm", "base: unrelated change")

        self.assertNotEqual(
            subprocess.run(
                ["git", "merge-base", "--is-ancestor", "feat-multi-squash", "main"],
                cwd=repo,
            ).returncode,
            0,
        )
        out = self._run_cleanup(repo)

        self.assertFalse(wt.exists(), "a complete multi-commit squash is removable")
        self.assertIn("removed feat-multi-squash", out.stdout)
        self.assertNotIn("feat-multi-squash", git(repo, "branch", "--format=%(refname:short)"))

    def test_preserves_partial_multi_commit_squash(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-partial-squash")
        (wt / "one.txt").write_text("one\n")
        git(wt, "add", "one.txt")
        git(wt, "commit", "-qm", "feat: first change")
        (wt / "two.txt").write_text("two\n")
        git(wt, "add", "two.txt")
        git(wt, "commit", "-qm", "feat: second change")
        git(repo, "merge", "-q", "--squash", "feat-partial-squash~1")
        git(repo, "commit", "-qm", "squash: partial feature")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(wt.exists(), "partial squash must preserve the worktree")
        self.assertIn("skipped feat-partial-squash (unmerged)", out.stdout)
        self.assertIn("feat-partial-squash", git(repo, "branch", "--format=%(refname:short)"))

    def test_preserves_reverted_multi_commit_squash(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-reverted-squash")
        (wt / "one.txt").write_text("one\n")
        git(wt, "add", "one.txt")
        git(wt, "commit", "-qm", "feat: first change")
        (wt / "two.txt").write_text("two\n")
        git(wt, "add", "two.txt")
        git(wt, "commit", "-qm", "feat: second change")
        git(repo, "merge", "-q", "--squash", "feat-reverted-squash")
        git(repo, "commit", "-qm", "squash: reverted feature")
        git(repo, "revert", "--no-edit", "HEAD")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(wt.exists(), "reverted squash must preserve the worktree")
        self.assertIn("skipped feat-reverted-squash (unmerged)", out.stdout)

    def test_preserves_newline_pathname_worktree(self):
        # JD-B-001: a branch whose only changed path contains a newline must
        # never be classified as merged when the base does not contain that
        # path. git's non-NUL `--name-only` output C-quotes such a pathname;
        # feeding the quoted literal to `git ls-tree` yields an empty entry on
        # BOTH sides, so a naive byte compare would falsely report equality and
        # the genuinely unmerged worktree would be removed.
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-newline")
        name = "bug\nreport.txt"
        (wt / name).parent.mkdir(parents=True, exist_ok=True)
        (wt / name).write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: newline pathname")
        # An unrelated main commit guarantees a larger candidate combined
        # patch, forcing the combined-tree comparison onto the changed paths.
        (repo / "base-extra.txt").write_text("unrelated\n")
        git(repo, "add", "base-extra.txt")
        git(repo, "commit", "-qm", "base: unrelated change")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(wt.exists(), "unmerged branch must be preserved")
        self.assertIn("skipped feat-newline (unmerged)", out.stdout)
        self.assertIn("feat-newline", git(repo, "branch", "--format=%(refname:short)"))

    def test_preserves_unmerged_worktree(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-unmerged")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")  # never merged into main

        out = self._run_cleanup(repo)

        self.assertTrue(wt.exists(), "unmerged worktree must be preserved")
        self.assertIn("skipped feat-unmerged (unmerged)", out.stdout)

    def test_preserves_dirty_worktree(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-dirty")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(repo, "merge", "-q", "--no-ff", "-m", "merge", "feat-dirty")
        # uncommitted change makes it dirty even though the branch is merged
        (wt / "dirty.txt").write_text("uncommitted\n")

        out = self._run_cleanup(repo)

        self.assertTrue(wt.exists(), "dirty worktree must be preserved")
        self.assertIn("skipped feat-dirty (dirty)", out.stdout)

    def test_preserves_detached_worktree_under_configured_directory(self):
        repo = self._make_repo()
        wt = repo / ".worktrees" / "detached"
        git(repo, "worktree", "add", "-q", "--detach", str(wt), "main")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(wt.exists())
        self.assertIn("skipped detached (detached)", out.stdout)

    def test_preserves_candidate_during_incomplete_main_merge(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-active-merge")
        (wt / "README.md").write_text("feature side\n")
        git(wt, "add", "README.md")
        git(wt, "commit", "-qm", "feature conflicting change")
        (repo / "README.md").write_text("main side\n")
        git(repo, "add", "README.md")
        git(repo, "commit", "-qm", "main conflicting change")
        merge = subprocess.run(
            ["git", "merge", "--no-ff", "-m", "merge feature", "feat-active-merge"],
            cwd=repo,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(merge.returncode, 0)

        try:
            out = self._run_cleanup(repo, "--dry-run")
        finally:
            git(repo, "merge", "--abort")

        self.assertTrue(wt.exists())
        self.assertIn("skipped feat-active-merge (unmerged)", out.stdout)

    def test_never_reports_main_worktree_as_removable(self):
        repo = self._make_repo()

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(repo.exists())
        self.assertNotIn("would remove", out.stdout)
        self.assertNotIn("removed", out.stdout)

    def test_dry_run_removes_nothing(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-merged")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(repo, "merge", "-q", "--no-ff", "-m", "merge", "feat-merged")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertTrue(wt.exists(), "--dry-run must not remove anything")
        self.assertIn("would remove feat-merged", out.stdout)

    # ── T2: regular merge on remote base, stale local base (PR #93 repro) ──

    def test_detects_regular_merge_on_remote_base_with_stale_local_base(self):
        repo = self._make_repo()
        main_sha_before = git(repo, "rev-parse", "main").strip()

        wt = self._add_worktree(repo, "feat-regular")
        (wt / "feature.txt").write_text("regular work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: regular work")

        # Merge into local main (creates merge commit)
        git(repo, "merge", "-q", "--no-ff", "-m", "Merge feat-regular", "feat-regular")
        merge_sha = git(repo, "rev-parse", "HEAD").strip()

        # Simulate: origin/main has the merge, local main is stale
        git(repo, "update-ref", "refs/remotes/origin/main", merge_sha)
        git(repo, "config", "remote.origin.url", "https://example.com/repo.git")
        git(repo, "update-ref", "refs/heads/main", main_sha_before)

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("would remove", out.stdout)
        self.assertNotIn("unmerged", out.stdout)

    # ── T4: rebase merge detected by patch-id ──

    def test_removes_rebase_merged_worktree_by_patch_id(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-rebase")
        (wt / "rebase.txt").write_text("rebase work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: rebase work")

        # Simulate rebase merge: apply the same diff on main with a different message.
        # This creates a commit with the same patch-id but a different SHA.
        (repo / "rebase.txt").write_text("rebase work\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "main: rebased feat-rebase work")

        # Sanity: feat-rebase tip is NOT an ancestor of main (different SHA)
        rc = subprocess.run(
            ["git", "merge-base", "--is-ancestor", "feat-rebase", "main"],
            cwd=repo,
        ).returncode
        self.assertNotEqual(rc, 0, "rebase tip should not be ancestor of main")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("would remove", out.stdout)

    # ── T7: branch ahead of base with remote candidate present ──

    def test_preserves_branch_ahead_of_base(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-ahead")
        (wt / "ahead.txt").write_text("unlanded work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: unlanded work")

        # origin/main exists but does NOT contain the branch's commits
        main_sha = git(repo, "rev-parse", "main").strip()
        git(repo, "update-ref", "refs/remotes/origin/main", main_sha)
        git(repo, "config", "remote.origin.url", "https://example.com/repo.git")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("skipped feat-ahead (unmerged)", out.stdout)

    # ── T9: remote-deleted branch, local base still proves merge ──

    def test_removes_remote_deleted_branch_when_local_base_contains_tip(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-gone")
        (wt / "gone.txt").write_text("gone work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: gone work")

        # Merge into local main (local main proves the merge)
        git(repo, "merge", "-q", "--no-ff", "-m", "merge feat-gone", "feat-gone")

        # No origin/main exists — remote branch was deleted
        # (we simply don't create refs/remotes/origin/main)

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("would remove", out.stdout)

    # ── T10: conflict-resolution merge commit on remote base ──

    def test_detects_conflict_resolution_merge_on_remote_base(self):
        repo = self._make_repo()
        main_sha_before = git(repo, "rev-parse", "main").strip()

        wt = self._add_worktree(repo, "feat-conflict")

        # Create conflicting changes on the same file
        (repo / "README.md").write_text("main version\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "main: update README")

        (wt / "README.md").write_text("feature version\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: update README")

        # Attempt merge — will conflict
        env = dict(os.environ)
        env.update({
            "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
            "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
        })
        subprocess.run(
            ["git", "merge", "--no-ff", "-m", "Merge feat-conflict", "feat-conflict"],
            cwd=repo,
            env=env,
        )
        # Resolve conflict and complete merge
        (repo / "README.md").write_text("resolved version\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "Merge feat-conflict (resolved)")

        merge_sha = git(repo, "rev-parse", "HEAD").strip()

        # Simulate: origin/main has the conflict-resolution merge, local main is stale
        git(repo, "update-ref", "refs/remotes/origin/main", merge_sha)
        git(repo, "config", "remote.origin.url", "https://example.com/repo.git")
        git(repo, "update-ref", "refs/heads/main", main_sha_before)

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("would remove", out.stdout)
        self.assertNotIn("unmerged", out.stdout)

    # ── Fast-forward merge: branch tip is a direct ancestor of local main ──

    def test_removes_fast_forward_merged_worktree(self):
        """Fast-forward merge locks down the exact-base candidate path.

        When the branch tip is a direct ancestor of the local base (no merge
        commit, no remote involvement), the script must detect the merge via
        the exact base ref alone. This is the simplest positive case for the
        first candidate in `resolve_base_candidates`.
        """
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-ff")
        (wt / "ff.txt").write_text("ff work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: ff work")

        # Fast-forward merge: branch tip becomes the new main tip with no
        # merge commit. Use --ff-only to assert the operation is reachable
        # by fast-forward; this fails if the branches have actually diverged.
        git(repo, "merge", "-q", "--ff-only", "feat-ff")

        # Sanity: the branch tip is now equal to main, so the exact-base
        # candidate MUST detect the merge.
        self.assertEqual(
            git(repo, "rev-parse", "feat-ff").strip(),
            git(repo, "rev-parse", "main").strip(),
        )

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("would remove feat-ff", out.stdout)
        self.assertNotIn("unmerged", out.stdout)

    # ── Bounded candidate resolution: script must NOT call `git fetch` ──

    def test_does_not_invoke_git_fetch_when_remote_missing(self):
        """Bounded candidate resolution: no `git fetch`, no network.

        Even when no `origin/<base>` ref exists, the script must fall back to
        the local `git cherry` / `git rev-list` behavior. It must never
        attempt to fetch over the network. This locks down the third
        "Bounded Candidate Resolution" requirement from the spec.
        """
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-nonet")
        (wt / "nonet.txt").write_text("no-network work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: no-network work")

        # Capture every git invocation via GIT_TRACE. The script may call
        # `git rev-parse`, `git merge-base`, `git cherry`, `git rev-list`,
        # etc. — none of those are `git fetch`. Any `git fetch` invocation
        # would violate the bounded-candidate requirement.
        env = dict(os.environ)
        env["GIT_TRACE"] = "1"

        result = subprocess.run(
            ["bash", str(CLEANUP_SCRIPT), "--base", "main", "--dry-run"],
            cwd=repo,
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )

        # Sanity: the script reached a stable verdict for the unmerged branch.
        self.assertIn("skipped feat-nonet (unmerged)", result.stdout)

        # Look for any `git fetch` invocation in the trace. GIT_TRACE writes
        # lines like `trace: built-in: git 'fetch' ...` to stderr; we treat
        # any such line as a violation of the bounded-candidate requirement.
        fetch_traces = [
            line
            for line in result.stderr.splitlines()
            if "trace:" in line and "fetch" in line.lower()
        ]
        self.assertEqual(
            fetch_traces,
            [],
            f"script must not invoke `git fetch`; found: {fetch_traces}",
        )


    # ── A1: SIGPIPE false positive in patch-id check ──

    def _fast_import_linear_commits(
        self, repo: Path, branch: str, base_sha: str, count: int
    ) -> str:
        """Create `count` empty commits on `branch`, built on `base_sha`.

        Uses a single `git fast-import` call instead of `count` individual
        `git commit` invocations, which would be far too slow for the commit
        count needed to exceed the OS pipe buffer.
        """
        parts = []
        for i in range(1, count + 1):
            msg = f"c{i}\n"
            parts.append(f"commit refs/heads/{branch}\n")
            parts.append(f"mark :{i}\n")
            parts.append("committer t <t@t> 0 +0000\n")
            parts.append(f"data {len(msg)}\n{msg}")
            parts.append(f"from {base_sha}\n" if i == 1 else f"from :{i - 1}\n")
        subprocess.run(
            ["git", "fast-import", "--quiet"],
            cwd=repo,
            input="".join(parts),
            text=True,
            check=True,
        )
        return git(repo, "rev-parse", branch).strip()

    def test_bash_loop_avoids_sigpipe_false_positive(self):
        """`candidate_has_patch_equivalence` (the real, shipped function)
        must correctly report "not patch-equivalent" via its bash while-read
        loop, even when `git cherry` output exceeds the OS pipe buffer
        (~16KB macOS, ~64KB Linux).

        This locks down against a regression to a `printf | grep -q`
        pipeline: under `set -o pipefail`, `grep -q` exits on the first
        match, the producer (`printf`) can be killed by SIGPIPE once its
        write blocks on a full pipe, and pipefail then reports the whole
        pipeline as failed — `! <pipeline>` becomes true, and the function
        returns 0 (merged) even though `+` lines are present. That is a
        false positive that can lead to a merged/unmerged worktree and its
        branch being deleted while still unmerged.

        Sources the actual script (guarded by WORKTREE_CLEANUP_SOURCE_ONLY
        so only function definitions load, not the worktree-scanning main
        loop) and calls the real `candidate_has_patch_equivalence` function
        against a repo with 2000 commits unique to the feature branch —
        enough for `git cherry` to emit ~85KB of `+` lines, comfortably past
        both platforms' pipe buffers.
        """
        repo = self._make_repo()
        base_sha = git(repo, "rev-parse", "main").strip()
        feature_sha = self._fast_import_linear_commits(
            repo, "feat-huge", base_sha, 2000
        )

        result = subprocess.run(
            [
                "bash",
                "-c",
                f'source "{CLEANUP_SCRIPT}"; '
                f'candidate_has_patch_equivalence "{feature_sha}" "{base_sha}"',
            ],
            cwd=repo,
            env={**os.environ, "WORKTREE_CLEANUP_SOURCE_ONLY": "1"},
            capture_output=True,
            text=True,
            timeout=30,
        )

        self.assertEqual(
            result.returncode,
            1,
            "candidate_has_patch_equivalence must return 1 (not equivalent) "
            "when cherry output is all '+' lines, even past the pipe buffer. "
            f"Got: {result.returncode}. stderr: {result.stderr}",
        )

    # ── B2: origin/<base> fallback when configured remote is stale ──

    def test_origin_base_fallback_when_configured_remote_missing(self):
        """origin/<base> must prove merge when branch.<base>.remote is stale.

        When branch.main.remote points to a non-existent remote (fake-remote)
        and refs/remotes/fake-remote/main does NOT exist, but
        refs/remotes/origin/main DOES contain the merge commit, the script
        must fall back to origin/main. Pre-fix, the third candidate fails
        rev-parse and there is no fourth candidate — regression to unmerged.
        Post-fix, the fourth candidate finds origin/main and detects merged.
        """
        repo = self._make_repo()
        main_sha_before = git(repo, "rev-parse", "main").strip()

        wt = self._add_worktree(repo, "feat-fallback")
        (wt / "fallback.txt").write_text("fallback work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: fallback work")

        # Merge into local main
        git(repo, "merge", "-q", "--no-ff", "-m", "merge feat-fallback", "feat-fallback")
        merge_sha = git(repo, "rev-parse", "HEAD").strip()

        # Set origin/main to the merge commit
        git(repo, "update-ref", "refs/remotes/origin/main", merge_sha)
        git(repo, "config", "remote.origin.url", "https://example.com/repo.git")

        # Set branch.main.remote to a non-existent remote
        git(repo, "config", "branch.main.remote", "fake-remote")
        # Do NOT set remote.fake-remote.url — the ref won't resolve

        # Reset local main to before the merge (stale local)
        git(repo, "update-ref", "refs/heads/main", main_sha_before)

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("would remove feat-fallback", out.stdout)
        self.assertNotIn("unmerged", out.stdout)

    # ── B3: dual-remote safety — configured remote resolving blocks the
    #        unconditional origin fallback ──

    def test_dual_remote_configured_remote_resolves_skips_unrelated_origin(self):
        """A resolvable configured remote must block the origin fallback.

        When `branch.main.remote` points to a different, valid remote
        (`upstream`) whose ref `refs/remotes/upstream/main` exists locally
        but does NOT contain the feature branch's commit, and the feature
        is merged only into `refs/remotes/origin/main` (e.g. a contributor's
        personal fork used for review), the script must NOT treat
        `origin/main` as proof of merge.

        Pre-fix, the last-resort candidate unconditionally adds
        `origin/<base>` regardless of whether the configured remote-tracking
        ref already resolved, so `is_merged` reports merged even though the
        branch's actual configured upstream (`upstream/main`) does not
        contain it. That false positive causes cleanup to delete an
        unmerged worktree and branch.
        """
        repo = self._make_repo()
        main_sha_before = git(repo, "rev-parse", "main").strip()

        wt = self._add_worktree(repo, "feat-dual-remote")
        (wt / "feature.txt").write_text("dual remote work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: dual remote work")
        feature_sha = git(wt, "rev-parse", "feat-dual-remote").strip()

        # upstream/main is a VALID, resolvable candidate but does NOT
        # contain the feature branch's commit.
        git(repo, "update-ref", "refs/remotes/upstream/main", main_sha_before)
        git(repo, "config", "branch.main.remote", "upstream")

        # origin/main is a DIFFERENT remote (e.g. a personal fork) that DOES
        # contain the feature tip — but branch.main.remote does not point at
        # it, so it must not be consulted while upstream/main resolves.
        git(repo, "update-ref", "refs/remotes/origin/main", feature_sha)
        git(repo, "config", "remote.origin.url", "https://example.com/fork.git")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("skipped feat-dual-remote (unmerged)", out.stdout)
        self.assertNotIn("would remove feat-dual-remote", out.stdout)

    def test_dual_remote_with_full_upstream_tracking_skips_unrelated_origin(self):
        """Dual-remote safety must hold with full upstream tracking config.

        Same dual-remote scenario as above, but with the standard
        `git branch --set-upstream-to=upstream/main` shape: BOTH
        `branch.main.remote=upstream` AND `branch.main.merge=refs/heads/main`
        are set, so `main@{u}` resolves. The upstream candidate then emits
        `refs/remotes/upstream/main` first, and the configured-remote
        candidate resolves to that exact same ref and is deduplicated.

        This pins down that "the configured remote ref resolves" must be
        decided by ref resolution alone, NOT by whether the ref was newly
        emitted: if dedup (the ref was already emitted as the upstream
        candidate) is mistaken for "did not resolve", the origin fallback
        fires and the unrelated fork's `origin/main` falsely proves the
        merge — deleting an unmerged worktree and branch.
        """
        repo = self._make_repo()
        main_sha_before = git(repo, "rev-parse", "main").strip()

        wt = self._add_worktree(repo, "feat-dual-upstream")
        (wt / "feature.txt").write_text("dual remote upstream work\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "feat: dual remote upstream work")
        feature_sha = git(wt, "rev-parse", "feat-dual-upstream").strip()

        # upstream/main is a VALID, resolvable candidate that does NOT
        # contain the feature branch's commit.
        git(repo, "update-ref", "refs/remotes/upstream/main", main_sha_before)

        # Full upstream tracking shape (`git remote add upstream` +
        # `git branch --set-upstream-to=upstream/main`): the remote's fetch
        # refspec is required for main@{u} to resolve — without it, git
        # cannot map refs/heads/main to refs/remotes/upstream/main.
        git(repo, "config", "remote.upstream.url", "https://example.com/upstream.git")
        git(
            repo,
            "config",
            "remote.upstream.fetch",
            "+refs/heads/*:refs/remotes/upstream/*",
        )
        git(repo, "config", "branch.main.remote", "upstream")
        git(repo, "config", "branch.main.merge", "refs/heads/main")

        # Sanity: main@{u} must resolve, so the upstream candidate emits
        # refs/remotes/upstream/main BEFORE the configured-remote candidate
        # considers the very same ref (the dedup-collision path under test).
        self.assertEqual(
            git(
                repo, "rev-parse", "--symbolic-full-name", "main@{u}"
            ).strip(),
            "refs/remotes/upstream/main",
        )

        # origin/main belongs to a different remote (personal fork) and DOES
        # contain the feature tip — it must not be consulted.
        git(repo, "update-ref", "refs/remotes/origin/main", feature_sha)
        git(repo, "config", "remote.origin.url", "https://example.com/fork.git")

        out = self._run_cleanup(repo, "--dry-run")

        self.assertIn("skipped feat-dual-upstream (unmerged)", out.stdout)
        self.assertNotIn("would remove feat-dual-upstream", out.stdout)


    # ── Submodule topology enumeration (worktree-flow-repo-topology) ──

    def _add_submodule_worktree(self, super_repo: Path, module: str, branch: str) -> Path:
        """Create a linked worktree owned by submodule at shared .worktrees/<module>-<branch>."""
        wt = super_repo / ".worktrees" / f"{module}-{branch}"
        wt.parent.mkdir(parents=True, exist_ok=True)
        git(
            super_repo / module,
            "worktree",
            "add",
            "-q",
            "-b",
            branch,
            str(wt.resolve()),
            "main",
        )
        return wt

    def test_submodule_merged_worktree_scanned_from_super(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api")
        wt = self._add_submodule_worktree(super_repo, "apps/api", "feat-done")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(super_repo / "apps/api", "merge", "-q", "--no-ff", "-m", "merge", "feat-done")

        out = self._run_cleanup(super_repo, "--dry-run")
        self.assertIn("would remove apps/api-feat-done", out.stdout)
        # Shared WT_PREFIX is super worktrees_dir
        self.assertTrue(str(wt).startswith(str(super_repo / ".worktrees")))

    def test_multiple_submodules_both_scanned(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="apps/api",
            second={"path": "apps/web", "name": "web"},
        )
        wt_api = self._add_submodule_worktree(super_repo, "apps/api", "feat-api")
        wt_web = self._add_submodule_worktree(super_repo, "apps/web", "feat-web")
        for wt, mod, br in (
            (wt_api, "apps/api", "feat-api"),
            (wt_web, "apps/web", "feat-web"),
        ):
            (wt / "f.txt").write_text("x\n")
            git(wt, "add", "-A")
            git(wt, "commit", "-qm", "work")
            git(super_repo / mod, "merge", "-q", "--no-ff", "-m", "merge", br)

        out = self._run_cleanup(super_repo, "--dry-run")
        self.assertIn("would remove apps/api-feat-api", out.stdout)
        self.assertIn("would remove apps/web-feat-web", out.stdout)

    def test_submodule_scope_flag_limits_to_one_module(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="apps/api",
            second={"path": "apps/web", "name": "web"},
        )
        wt_api = self._add_submodule_worktree(super_repo, "apps/api", "feat-api")
        wt_web = self._add_submodule_worktree(super_repo, "apps/web", "feat-web")
        for wt, mod, br in (
            (wt_api, "apps/api", "feat-api"),
            (wt_web, "apps/web", "feat-web"),
        ):
            (wt / "f.txt").write_text("x\n")
            git(wt, "add", "-A")
            git(wt, "commit", "-qm", "work")
            git(super_repo / mod, "merge", "-q", "--no-ff", "-m", "merge", br)

        out = self._run_cleanup(
            super_repo, "--dry-run", "--submodule", "apps/api"
        )
        self.assertIn("would remove apps/api-feat-api", out.stdout)
        self.assertNotIn("would remove apps/web-feat-web", out.stdout)

        out2 = self._run_cleanup(
            super_repo, "--dry-run", "--subrepo", "apps/web"
        )
        self.assertIn("would remove apps/web-feat-web", out2.stdout)
        self.assertNotIn("would remove apps/api-feat-api", out2.stdout)

    def test_uninitialized_submodule_skipped(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="apps/api",
            second={"path": "apps/web", "name": "web", "initialized": False},
        )
        wt = self._add_submodule_worktree(super_repo, "apps/api", "feat-done")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(super_repo / "apps/api", "merge", "-q", "--no-ff", "-m", "merge", "feat-done")

        # Must not fail trying to git -C the uninitialized module
        out = self._run_cleanup(super_repo, "--dry-run")
        self.assertEqual(out.returncode, 0)
        self.assertIn("would remove apps/api-feat-done", out.stdout)

    def test_submodule_flag_inert_on_standalone(self):
        repo = self._make_repo()
        wt = self._add_worktree(repo, "feat-merged")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(repo, "merge", "-q", "--no-ff", "-m", "merge", "feat-merged")
        out = self._run_cleanup(repo, "--dry-run", "--submodule", "apps/api")
        self.assertIn("would remove feat-merged", out.stdout)



    def test_explicit_topology_standalone_skips_submodule_enumeration(self):
        """Configured standalone must not scan submodules even if .gitmodules exists."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api")
        wt = self._add_submodule_worktree(super_repo, "apps/api", "feat-done")
        (wt / "f.txt").write_text("x\n")
        git(wt, "add", "-A")
        git(wt, "commit", "-qm", "work")
        git(super_repo / "apps/api", "merge", "-q", "--no-ff", "-m", "merge", "feat-done")

        # Auto/default would find the submodule-owned worktree.
        auto_out = self._run_cleanup(super_repo, "--dry-run")
        self.assertIn("would remove apps/api-feat-done", auto_out.stdout)

        # Explicit standalone: only SUPER_ROOT is scanned; submodule worktrees
        # never appear on the superproject worktree list.
        out = self._run_cleanup(super_repo, "--dry-run", "--topology", "standalone")
        self.assertNotIn("would remove apps/api-feat-done", out.stdout)
        self.assertNotIn("apps/api-feat-done", out.stdout)


if __name__ == "__main__":
    unittest.main()
