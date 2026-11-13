"""Black-box tests for `ai-specs upgrade` (native Go engine, GO-20 U2d).

The engine resolves its install from the EXECUTABLE path (it compares the
running binary against $HOME/.ai-specs and ~/.local/bin/ai-specs), so every
fixture copies the native binary into a scratch install under a fake HOME and
runs that copy. The operator's real installation is never touched: HOME,
AI_SPECS_HOME and PATH are always redirected into the fixture.
"""

import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cli_path  # noqa: E402


ROOT = Path(__file__).resolve().parents[1]
LEGACY_LAUNCHER = ROOT / "bin" / "ai-specs"


def run(args, cwd=None, env=None, check=True, capture_output=True):
    """Run a command and return CompletedProcess."""
    result = subprocess.run(
        args, cwd=cwd, env=env, capture_output=capture_output, text=True
    )
    if check and result.returncode != 0:
        raise subprocess.CalledProcessError(
            result.returncode, args, output=result.stdout, stderr=result.stderr
        )
    return result


class UpgradeTests(unittest.TestCase):
    def fake_home(self):
        """Create a temp directory to act as HOME, return its resolved Path.

        Resolved so the engine's physical-path comparisons match the paths it
        receives (macOS /var -> /private/var).
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(os.path.realpath(tmp.name))

    def native_binary(self):
        binary = cli_path()
        if binary == LEGACY_LAUNCHER:
            self.fail("native Go binary unavailable: the upgrade route cannot be exercised")
        return binary

    def setup_global_install(self, home: Path, version="1.0.0"):
        """
        Create a fake global install under home/.ai-specs with a git repo.
        The native binary is the installed entry point, reached through the
        ~/.local/bin/ai-specs symlink as on a real machine.
        Returns (ai_specs_home, bin_dir).
        """
        ai_specs = home / ".ai-specs"
        ai_specs.mkdir()
        bin_dir = ai_specs / "bin"
        bin_dir.mkdir()
        shutil.copy2(self.native_binary(), bin_dir / "ai-specs")
        (bin_dir / "ai-specs").chmod(0o755)
        # Write VERSION
        (ai_specs / "VERSION").write_text(version + "\n")
        # Symlink from ~/.local/bin/ai-specs -> ~/.ai-specs/bin/ai-specs
        local_bin = home / ".local" / "bin"
        local_bin.mkdir(parents=True)
        local_bin_link = local_bin / "ai-specs"
        local_bin_link.symlink_to(bin_dir / "ai-specs")
        # Init git repo; the binary stays out of version control, as the
        # installed entry is not a tracked source file of the fixture.
        (ai_specs / ".gitignore").write_text("bin/\n")
        run(["git", "init"], cwd=ai_specs)
        run(["git", "config", "user.email", "test@test.com"], cwd=ai_specs)
        run(["git", "config", "user.name", "Test"], cwd=ai_specs)
        run(["git", "add", "."], cwd=ai_specs)
        run(["git", "commit", "-m", "init"], cwd=ai_specs)
        # Set up a bare remote and push to it
        bare = home / "origin.git"
        bare.mkdir()
        run(["git", "init", "--bare"], cwd=bare)
        run(["git", "remote", "add", "origin", str(bare)], cwd=ai_specs)
        run(["git", "push", "-u", "origin", "main"], cwd=ai_specs)
        return ai_specs, bin_dir

    def make_env(self, home: Path, extra=None):
        """Build an env dict with HOME, AI_SPECS_HOME, PATH set."""
        env = os.environ.copy()
        env["HOME"] = str(home)
        env["AI_SPECS_HOME"] = str(home / ".ai-specs")
        # Ensure our fake ~/.local/bin/ai-specs is in PATH ahead of the real one
        env["PATH"] = str(home / ".local" / "bin") + ":" + env.get("PATH", "")
        if extra:
            env.update(extra)
        return env

    def run_upgrade(self, home: Path, env, args=(), check=True):
        """Run the installed native binary: `<install>/bin/ai-specs upgrade ...`."""
        binary = home / ".ai-specs" / "bin" / "ai-specs"
        return run([str(binary), "upgrade", *args], env=env, check=check)

    def publish_upstream_version(self, home: Path, version: str):
        """Push a new VERSION commit to the bare origin (simulated upstream)."""
        bare = home / "origin.git"
        clone = home / "upstream_clone"
        run(["git", "clone", str(bare), str(clone)])
        (clone / "VERSION").write_text(version + "\n")
        run(["git", "add", "VERSION"], cwd=clone)
        run(["git", "commit", "-m", "v" + version], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)

    # --- 1. Help lists upgrade ---
    def test_help_lists_upgrade(self):
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        result = run([str(self.native_binary()), "help"], env=env, check=True)
        self.assertIn("upgrade", result.stdout)
        self.assertIn("installation", result.stdout.lower())

    # --- 2. Upgrade accepts dry-run flag ---
    def test_upgrade_help_prints_flags(self):
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        # A plain upgrade on a fresh install is a no-op against the local origin.
        self.run_upgrade(home, env, check=False)
        result = self.run_upgrade(home, env, args=["--help"], check=False)
        self.assertEqual(result.returncode, 0)
        self.assertIn("--dry-run", result.stdout)
        self.assertIn("--force", result.stdout)

    # --- 3. Valid global install detection ---
    def test_valid_global_install_proceeds(self):
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        # Run upgrade --dry-run (safe, read-only)
        result = self.run_upgrade(home, env, args=["--dry-run"], check=False)
        # Should succeed because install is valid, even though origin/main has no new commits
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("up to date", result.stdout.lower())

    # --- 4. AI_SPECS_HOME is derived from the executable, as the launcher did ---
    def test_missing_ai_specs_home_is_derived_from_executable(self):
        # The legacy `lib/upgrade.sh` refused when AI_SPECS_HOME was unset, but
        # only when run directly: the public `ai-specs upgrade` launcher always
        # exported it, deriving it from the executable location. The native
        # binary does the same derivation, so the unset and set environments
        # must behave identically. The refusal branch itself is pinned by the
        # Go unit test TestDetectMissingHome (internal/upgrade/upgrade_test.go).
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        with_home = self.run_upgrade(home, env, args=["--dry-run"], check=False)
        unset_env = dict(env)
        del unset_env["AI_SPECS_HOME"]
        without_home = self.run_upgrade(home, unset_env, args=["--dry-run"], check=False)
        self.assertEqual(with_home.returncode, 0, msg=with_home.stderr)
        self.assertEqual(without_home.returncode, with_home.returncode)
        self.assertEqual(without_home.stdout, with_home.stdout)
        self.assertEqual(without_home.stderr, with_home.stderr)

    def test_missing_git_dir(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        # Remove .git
        shutil.rmtree(ai_specs / ".git")
        result = self.run_upgrade(home, env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("install.sh", result.stderr)

    def test_broken_symlink(self):
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        # Break the symlink
        local_bin_link = home / ".local" / "bin" / "ai-specs"
        local_bin_link.unlink()
        local_bin_link.symlink_to("/nonexistent")
        result = self.run_upgrade(home, env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("install.sh", result.stderr)

    # --- 5. Dev channel protection ---
    def test_dev_channel_blocked(self):
        home = self.fake_home()
        # Set up a dev checkout outside ~/.ai-specs
        dev = home / "dev" / "ai-specs"
        bin_dir = dev / "bin"
        bin_dir.mkdir(parents=True)
        shutil.copy2(self.native_binary(), bin_dir / "ai-specs")
        (bin_dir / "ai-specs").chmod(0o755)
        (dev / "VERSION").write_text("dev\n")
        # Symlink from ~/.local/bin/ai-specs -> dev checkout
        local_bin = home / ".local" / "bin"
        local_bin.mkdir(parents=True)
        local_bin_link = local_bin / "ai-specs"
        local_bin_link.symlink_to(bin_dir / "ai-specs")
        env = os.environ.copy()
        env["HOME"] = str(home)
        env["AI_SPECS_HOME"] = str(dev)
        env["PATH"] = str(local_bin) + ":" + env.get("PATH", "")
        result = run([str(bin_dir / "ai-specs"), "upgrade"], env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("manual", result.stderr.lower())
        self.assertIn("git pull", result.stderr.lower())

    # --- 6. Dirty working tree blocks upgrade ---
    def test_dirty_working_tree_blocks(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        (ai_specs / "foo.txt").write_text("dirty")
        result = self.run_upgrade(home, env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("dirty", result.stderr.lower())
        self.assertIn("--force", result.stderr)

    # --- 7. Dirty working tree with --force ---
    def test_dirty_working_tree_force(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        (ai_specs / "foo.txt").write_text("dirty")
        run(["git", "add", "foo.txt"], cwd=ai_specs)
        # Up-to-date but dirty; with --force should warn and succeed (since already up to date)
        result = self.run_upgrade(home, env, args=["--force"], check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("warning", (result.stdout + result.stderr).lower())

    # --- 8. Successful fast-forward upgrade ---
    def test_successful_fast_forward(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        self.publish_upstream_version(home, "2.0.0")
        # Now the local repo is behind
        result = self.run_upgrade(home, env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("1.0.0", result.stdout)
        self.assertIn("2.0.0", result.stdout)
        # Verify the local repo was updated
        self.assertEqual((ai_specs / "VERSION").read_text().strip(), "2.0.0")

    # --- 9. Non-fast-forward blocked ---
    def test_non_fast_forward_blocked(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        # Diverge locally
        (ai_specs / "local.txt").write_text("local change")
        run(["git", "add", "local.txt"], cwd=ai_specs)
        run(["git", "commit", "-m", "local"], cwd=ai_specs)
        result = self.run_upgrade(home, env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("diverged", (result.stdout + result.stderr).lower())

    # --- 10. Dry-run previews upgrade ---
    def test_dry_run_previews(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        self.publish_upstream_version(home, "2.0.0")
        # Fetch in the local repo so origin/main is up to date for dry-run preview
        run(["git", "fetch", "origin", "main"], cwd=ai_specs)
        result = self.run_upgrade(home, env, args=["--dry-run"], check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("1.0.0", result.stdout)
        self.assertIn("2.0.0", result.stdout)
        self.assertIn("no changes", result.stdout.lower())
        # Local repo should NOT have changed
        self.assertEqual((ai_specs / "VERSION").read_text().strip(), "1.0.0")

    # --- 11. Version diff printed after upgrade ---
    def test_version_diff_after_upgrade(self):
        home = self.fake_home()
        self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        self.publish_upstream_version(home, "2.0.0")
        result = self.run_upgrade(home, env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("1.0.0", result.stdout)
        self.assertIn("2.0.0", result.stdout)

    # --- 12. Symlink integrity after upgrade ---
    def test_symlink_integrity_after_upgrade(self):
        home = self.fake_home()
        self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        self.publish_upstream_version(home, "2.0.0")
        result = self.run_upgrade(home, env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("symlink", result.stdout.lower())

    # --- 13. Already up-to-date installation ---
    def test_already_up_to_date(self):
        home = self.fake_home()
        self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        result = self.run_upgrade(home, env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("up to date", result.stdout.lower())


if __name__ == "__main__":
    unittest.main()
