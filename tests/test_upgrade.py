import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
BIN_AI_SPECS = ROOT / "bin" / "ai-specs"
LIB_UPGRADE = ROOT / "lib" / "upgrade.sh"


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


def run_ai_specs(args, cwd=None, env=None, check=True):
    """Run bin/ai-specs with the given args in the given env."""
    cmd = ["bash", str(BIN_AI_SPECS)] + args
    return run(cmd, cwd=cwd, env=env, check=check)


def run_upgrade(cwd=None, env=None, check=True):
    """Run lib/upgrade.sh directly with the given env."""
    # Use the copied script in the fake install when AI_SPECS_HOME is set
    if env and env.get("AI_SPECS_HOME"):
        script = Path(env["AI_SPECS_HOME"]) / "lib" / "upgrade.sh"
    else:
        script = LIB_UPGRADE
    cmd = ["bash", str(script)]
    return run(cmd, cwd=cwd, env=env, check=check)


class UpgradeTests(unittest.TestCase):
    def fake_home(self):
        """Create a temp directory to act as HOME, return its Path."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def setup_global_install(self, home: Path, version="1.0.0"):
        """
        Create a fake global install under home/.ai-specs with a git repo.
        Returns (ai_specs_home, bin_dir).
        """
        ai_specs = home / ".ai-specs"
        ai_specs.mkdir()
        bin_dir = ai_specs / "bin"
        bin_dir.mkdir()
        lib_dir = ai_specs / "lib"
        lib_dir.mkdir()
        # Copy the real bin/ai-specs into the fake install
        (bin_dir / "ai-specs").write_text(BIN_AI_SPECS.read_text())
        (bin_dir / "ai-specs").chmod(0o755)
        # Copy lib/upgrade.sh into the fake install so it resolves inside ~/.ai-specs
        (lib_dir / "upgrade.sh").write_text(LIB_UPGRADE.read_text())
        (lib_dir / "upgrade.sh").chmod(0o755)
        # Write VERSION
        (ai_specs / "VERSION").write_text(version + "\n")
        # Symlink from ~/.local/bin/ai-specs -> ~/.ai-specs/bin/ai-specs
        local_bin = home / ".local" / "bin"
        local_bin.mkdir(parents=True)
        local_bin_link = local_bin / "ai-specs"
        local_bin_link.symlink_to(bin_dir / "ai-specs")
        # Init git repo
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

    # --- 1. Help lists upgrade ---
    def test_help_lists_upgrade(self):
        result = run_ai_specs(["help"], check=True)
        self.assertIn("upgrade", result.stdout)
        self.assertIn("update", result.stdout.lower())

    # --- 2. Upgrade accepts dry-run flag ---
    def test_upgrade_help_prints_flags(self):
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        result = run_upgrade(env=env, check=False)
        # Dry-run by default? Actually --help should print dry-run and force.
        # lib/upgrade.sh without args should run normally; let's test --help
        script = Path(env["AI_SPECS_HOME"]) / "lib" / "upgrade.sh"
        result = run(["bash", str(script), "--help"], env=env, check=False)
        self.assertEqual(result.returncode, 0)
        self.assertIn("--dry-run", result.stdout)
        self.assertIn("--force", result.stdout)

    # --- 3. Valid global install detection ---
    def test_valid_global_install_proceeds(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        # Run upgrade --dry-run (safe, read-only)
        script = ai_specs / "lib" / "upgrade.sh"
        result = run(["bash", str(script), "--dry-run"], env=env, check=False)
        # Should succeed because install is valid, even though origin/main has no new commits
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("up to date", result.stdout.lower())

    # --- 4. Missing/broken install detection ---
    def test_missing_ai_specs_home(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        del env["AI_SPECS_HOME"]
        script = ai_specs / "lib" / "upgrade.sh"
        result = run(["bash", str(script)], env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("install.sh", result.stderr)

    def test_missing_git_dir(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        # Remove .git
        import shutil
        shutil.rmtree(ai_specs / ".git")
        result = run_upgrade(env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("install.sh", result.stderr)

    def test_broken_symlink(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        # Break the symlink
        local_bin_link = home / ".local" / "bin" / "ai-specs"
        local_bin_link.unlink()
        local_bin_link.symlink_to("/nonexistent")
        result = run_upgrade(env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("install.sh", result.stderr)

    # --- 5. Dev channel protection ---
    def test_dev_channel_blocked(self):
        home = self.fake_home()
        # Set up a dev checkout outside ~/.ai-specs
        dev = home / "dev" / "ai-specs"
        dev.mkdir(parents=True)
        bin_dir = dev / "bin"
        bin_dir.mkdir()
        lib_dir = dev / "lib"
        lib_dir.mkdir()
        (bin_dir / "ai-specs").write_text(BIN_AI_SPECS.read_text())
        (bin_dir / "ai-specs").chmod(0o755)
        (lib_dir / "upgrade.sh").write_text(LIB_UPGRADE.read_text())
        (lib_dir / "upgrade.sh").chmod(0o755)
        (dev / "VERSION").write_text("dev\n")
        # Symlink from ~/.local/bin/ai-specs -> dev checkout
        local_bin = home / ".local" / "bin"
        local_bin.mkdir(parents=True)
        local_bin_link = local_bin / "ai-specs"
        if local_bin_link.exists() or local_bin_link.is_symlink():
            local_bin_link.unlink()
        local_bin_link.symlink_to(bin_dir / "ai-specs")
        env = os.environ.copy()
        env["HOME"] = str(home)
        env["AI_SPECS_HOME"] = str(dev)
        env["PATH"] = str(local_bin) + ":" + env.get("PATH", "")
        script = dev / "lib" / "upgrade.sh"
        result = run(["bash", str(script)], env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("manual", result.stderr.lower())
        self.assertIn("git pull", result.stderr.lower())

    # --- 6. Dirty working tree blocks upgrade ---
    def test_dirty_working_tree_blocks(self):
        home = self.fake_home()
        self.setup_global_install(home)
        env = self.make_env(home)
        # Make a dirty change
        ai_specs = home / ".ai-specs"
        (ai_specs / "foo.txt").write_text("dirty")
        result = run_upgrade(env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("dirty", result.stderr.lower())
        self.assertIn("--force", result.stderr)

    # --- 7. Dirty working tree with --force ---
    def test_dirty_working_tree_force(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home)
        env = self.make_env(home)
        ai_specs = home / ".ai-specs"
        (ai_specs / "foo.txt").write_text("dirty")
        run(["git", "add", "foo.txt"], cwd=ai_specs)
        script = ai_specs / "lib" / "upgrade.sh"
        result = run(["bash", str(script), "--force"], env=env, check=False)
        # Up-to-date but dirty; with --force should warn and succeed (since already up to date)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("warning", (result.stdout + result.stderr).lower())

    # --- 8. Successful fast-forward upgrade ---
    def test_successful_fast_forward(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        # Create a new commit on the bare remote (simulate upstream update)
        bare = home / "origin.git"
        clone = home / "upstream_clone"
        run(["git", "clone", str(bare), str(clone)])
        (clone / "VERSION").write_text("2.0.0\n")
        run(["git", "add", "VERSION"], cwd=clone)
        run(["git", "commit", "-m", "v2"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)
        # Now the local repo is behind
        result = run_upgrade(env=env, check=False)
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
        result = run_upgrade(env=env, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("diverged", (result.stdout + result.stderr).lower())

    # --- 10. Dry-run previews upgrade ---
    def test_dry_run_previews(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        # Push v2 to remote
        bare = home / "origin.git"
        clone = home / "upstream_clone"
        run(["git", "clone", str(bare), str(clone)])
        (clone / "VERSION").write_text("2.0.0\n")
        run(["git", "add", "VERSION"], cwd=clone)
        run(["git", "commit", "-m", "v2"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)
        # Fetch in the local repo so origin/main is up to date for dry-run preview
        run(["git", "fetch", "origin", "main"], cwd=ai_specs)
        script = ai_specs / "lib" / "upgrade.sh"
        result = run(["bash", str(script), "--dry-run"], env=env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("1.0.0", result.stdout)
        self.assertIn("2.0.0", result.stdout)
        self.assertIn("no changes", result.stdout.lower())
        # Local repo should NOT have changed
        self.assertEqual((ai_specs / "VERSION").read_text().strip(), "1.0.0")

    # --- 11. Version diff printed after upgrade ---
    def test_version_diff_after_upgrade(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        bare = home / "origin.git"
        clone = home / "upstream_clone"
        run(["git", "clone", str(bare), str(clone)])
        (clone / "VERSION").write_text("2.0.0\n")
        run(["git", "add", "VERSION"], cwd=clone)
        run(["git", "commit", "-m", "v2"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)
        result = run_upgrade(env=env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("1.0.0", result.stdout)
        self.assertIn("2.0.0", result.stdout)

    # --- 12. Symlink integrity after upgrade ---
    def test_symlink_integrity_after_upgrade(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        bare = home / "origin.git"
        clone = home / "upstream_clone"
        run(["git", "clone", str(bare), str(clone)])
        (clone / "VERSION").write_text("2.0.0\n")
        run(["git", "add", "VERSION"], cwd=clone)
        run(["git", "commit", "-m", "v2"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)
        result = run_upgrade(env=env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("symlink", result.stdout.lower())

    # --- 13. Already up-to-date installation ---
    def test_already_up_to_date(self):
        home = self.fake_home()
        self.setup_global_install(home, version="1.0.0")
        env = self.make_env(home)
        result = run_upgrade(env=env, check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("up to date", result.stdout.lower())


if __name__ == "__main__":
    unittest.main()
