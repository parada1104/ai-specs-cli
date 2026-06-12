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

    def test_bash_loop_avoids_sigpipe_false_positive(self):
        """The bash loop (NEW) avoids the SIGPIPE false positive that the
        `printf | grep -q` pipeline (OLD) under `set -o pipefail` can produce
        when the cherry output exceeds the OS pipe buffer.

        Pre-fix behavior: pipefail inverts when SIGPIPE fires on printf, so
        the function returns 0 (merged) even when cherry output contains
        `+` lines. This is a false positive that can lead to data loss.

        Post-fix behavior: bash while-read loop reads line by line, returns
        1 (unmerged) correctly when any `+` line is found.
        """
        import textwrap

        # 10000 `+ <sha>` lines = ~430KB. Far exceeds pipe buffer on macOS (~16KB)
        # and Linux (~64KB), guaranteeing the producer gets blocked on write
        # when the consumer (grep -q) exits on the first match.
        cherry_lines = "\n".join(f"+ {'a' * 40}" for _ in range(10000))
        cherry_escaped = cherry_lines.replace("'", "'\\''")

        # OLD pipeline (pre-fix). Set -o pipefail is the trigger.
        # Logic: if cherry has content AND no '+' lines found, return 0 (merged).
        # With SIGPIPE: grep finds '+' on line 1, exits 0, printf gets SIGPIPE (141),
        # pipefail makes pipeline exit 141, ! 141 = 0, condition true, return 0 (FALSE POSITIVE).
        old_script = textwrap.dedent(f"""
            set -euo pipefail
            cherry='{cherry_escaped}'
            if [[ -n "$cherry" ]] && ! printf '%s\\n' "$cherry" | grep -q '^+'; then
                exit 0
            fi
            exit 1
        """).strip()

        result = subprocess.run(
            ["bash", "-c", old_script],
            capture_output=True,
            text=True,
            timeout=30,
        )
        self.assertEqual(
            result.returncode, 0,
            "OLD pipeline should false-positive (return 0 = merged) on this OS "
            "when cherry output exceeds the pipe buffer. If it returned "
            f"{result.returncode}, the SIGPIPE path didn't fire; increase the "
            f"line count. stderr: {result.stderr}",
        )

        # NEW pipeline (post-fix). Bash while-read loop, no pipe.
        new_script = textwrap.dedent(f"""
            set -euo pipefail
            cherry='{cherry_escaped}'
            if [[ -n "$cherry" ]]; then
                while IFS= read -r line; do
                    [[ "$line" == +* ]] && exit 1
                done <<< "$cherry"
                exit 0
            fi
            exit 1
        """).strip()

        result = subprocess.run(
            ["bash", "-c", new_script],
            capture_output=True,
            text=True,
            timeout=30,
        )
        self.assertEqual(
            result.returncode, 1,
            f"NEW pipeline should correctly return 1 (unmerged) when `+` lines "
            f"are present. Got: {result.returncode}. stderr: {result.stderr}",
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


if __name__ == "__main__":
    unittest.main()
