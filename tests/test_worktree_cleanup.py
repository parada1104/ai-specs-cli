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


if __name__ == "__main__":
    unittest.main()
