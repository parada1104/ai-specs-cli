"""Narrowing the global install checkout.

`~/.ai-specs` carries subtrees the CLI never reads at runtime. Narrowing removes
them from the working tree. It is an optimization, never a precondition: any
failure warns and leaves a usable full checkout.

Shallow cloning is deliberately NOT used — `ai-specs upgrade` depends on
`git merge-base --is-ancestor` for its divergence guard, and truncated history
makes that check unreliable.
"""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
NARROW = ROOT / "lib" / "_internal" / "narrow-checkout.sh"

EXCLUDED = ("openspec", "tests", ".github", "tmp")
RUNTIME = ("lib", "bin", "catalog", "bundled-skills", "templates")


def run(args, cwd=None, env=None, check=True):
    result = subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True)
    if check and result.returncode != 0:
        raise subprocess.CalledProcessError(
            result.returncode, args, output=result.stdout, stderr=result.stderr
        )
    return result


class NarrowCheckoutTests(unittest.TestCase):
    def tmpdir(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def make_repo(self) -> tuple[Path, Path]:
        """A repo shaped like the CLI install, with an origin. (repo, bare)."""
        base = self.tmpdir()
        repo = base / "ai-specs"
        repo.mkdir()
        for name in RUNTIME + EXCLUDED:
            directory = repo / name
            directory.mkdir(parents=True)
            (directory / "file.txt").write_text(f"content of {name}\n")
        (repo / "VERSION").write_text("0.22.0\n")
        (repo / "CHANGELOG.md").write_text("# Changelog\n")

        run(["git", "init", "-b", "main"], cwd=repo)
        run(["git", "config", "user.email", "t@t.com"], cwd=repo)
        run(["git", "config", "user.name", "T"], cwd=repo)
        run(["git", "add", "-A"], cwd=repo)
        run(["git", "commit", "-m", "init"], cwd=repo)

        bare = base / "origin.git"
        bare.mkdir()
        run(["git", "init", "--bare"], cwd=bare)
        run(["git", "remote", "add", "origin", str(bare)], cwd=repo)
        run(["git", "push", "-u", "origin", "main"], cwd=repo)
        return repo, bare

    def narrow(self, repo: Path, env=None, check=False):
        return run(["bash", str(NARROW), str(repo)], env=env, check=check)

    # --- happy path ---------------------------------------------------------

    def test_excluded_subtrees_leave_the_working_tree(self):
        repo, _ = self.make_repo()
        result = self.narrow(repo)
        self.assertEqual(result.returncode, 0, msg=result.stdout + result.stderr)
        for name in EXCLUDED:
            self.assertFalse(
                (repo / name).exists(), msg=f"{name}/ should have been excluded"
            )

    def test_runtime_subtrees_survive(self):
        repo, _ = self.make_repo()
        self.narrow(repo)
        for name in RUNTIME:
            self.assertTrue((repo / name / "file.txt").is_file(), msg=f"{name}/ missing")

    def test_root_files_survive(self):
        repo, _ = self.make_repo()
        self.narrow(repo)
        self.assertTrue((repo / "VERSION").is_file())
        self.assertTrue((repo / "CHANGELOG.md").is_file())

    def test_tree_is_not_left_dirty(self):
        """A narrowed checkout must not look dirty to the upgrade guard."""
        repo, _ = self.make_repo()
        self.narrow(repo)
        status = run(["git", "status", "--porcelain"], cwd=repo)
        self.assertEqual(status.stdout.strip(), "")

    def test_history_and_ancestry_are_intact(self):
        """The divergence guard must keep working — this is why not --depth."""
        repo, _ = self.make_repo()
        self.narrow(repo)
        run(["git", "fetch", "origin", "main"], cwd=repo)
        ancestry = run(
            ["git", "merge-base", "--is-ancestor", "HEAD", "origin/main"],
            cwd=repo,
            check=False,
        )
        self.assertEqual(ancestry.returncode, 0)

    # --- idempotence --------------------------------------------------------

    def test_second_run_is_a_noop(self):
        repo, _ = self.make_repo()
        self.narrow(repo)
        second = self.narrow(repo)
        self.assertEqual(second.returncode, 0)
        for name in EXCLUDED:
            self.assertFalse((repo / name).exists())
        for name in RUNTIME:
            self.assertTrue((repo / name / "file.txt").is_file())

    def test_second_run_reports_nothing_to_do(self):
        repo, _ = self.make_repo()
        self.narrow(repo)
        second = self.narrow(repo)
        self.assertIn("already", (second.stdout + second.stderr).lower())

    # --- degradation --------------------------------------------------------

    def _git_without_sparse(self, base: Path) -> dict:
        """PATH shim whose `git` rejects `sparse-checkout` in any position.

        The subcommand is not necessarily $1 — real calls look like
        `git -C <dir> sparse-checkout set ...` — so scan every argument.
        """
        real_git = subprocess.run(
            ["which", "git"], capture_output=True, text=True
        ).stdout.strip()
        self.assertTrue(real_git, "git not found on PATH")

        shim_dir = base / "shim"
        shim_dir.mkdir()
        shim = shim_dir / "git"
        shim.write_text(
            "#!/usr/bin/env bash\n"
            'for arg in "$@"; do\n'
            '  if [[ "$arg" == "sparse-checkout" ]]; then\n'
            "    echo \"git: 'sparse-checkout' is not a git command\" >&2\n"
            "    exit 1\n"
            "  fi\n"
            "done\n"
            f'exec {real_git} "$@"\n'
        )
        shim.chmod(0o755)
        env = os.environ.copy()
        env["PATH"] = str(shim_dir) + ":" + env.get("PATH", "")
        return env

    def test_git_without_sparse_checkout_falls_back(self):
        repo, _ = self.make_repo()
        base = repo.parent
        env = self._git_without_sparse(base)

        result = self.narrow(repo, env=env)
        self.assertEqual(
            result.returncode, 0, msg="narrowing must never fail the caller"
        )
        # Nothing was removed: a full checkout is a valid outcome.
        for name in EXCLUDED:
            self.assertTrue((repo / name).exists())

    def test_fallback_warns_rather_than_failing_silently(self):
        repo, _ = self.make_repo()
        env = self._git_without_sparse(repo.parent)
        result = self.narrow(repo, env=env)
        self.assertTrue(
            (result.stdout + result.stderr).strip(),
            msg="degradation must say something",
        )

    def test_missing_target_is_not_fatal(self):
        result = self.narrow(Path("/nonexistent/ai-specs"))
        self.assertEqual(result.returncode, 0)

    def test_non_git_target_is_not_fatal(self):
        plain = self.tmpdir() / "plain"
        plain.mkdir()
        result = self.narrow(plain)
        self.assertEqual(result.returncode, 0)

    def test_dirty_excluded_path_is_left_alone(self):
        """Never discard uncommitted work, even in an excluded subtree."""
        repo, _ = self.make_repo()
        (repo / "openspec" / "scratch.md").write_text("unsaved work\n")

        result = self.narrow(repo)
        self.assertEqual(result.returncode, 0)
        self.assertTrue((repo / "openspec" / "scratch.md").is_file())


if __name__ == "__main__":
    unittest.main()
