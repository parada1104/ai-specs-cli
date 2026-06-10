"""Functional tests for mode-only dirty-tree remediation in upgrade.sh and install.sh.

A previous installer ran chmod +x on lib/*.sh and lib/_internal/*.py, which
are tracked as mode 100644 in git.  On systems with core.fileMode=true this
leaves the working tree appearing dirty (mode-bits only, no content changes).
Both upgrade.sh and install.sh must auto-remediate this class of dirt so that
users are not stuck in an update loop.
"""
import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
INSTALL_SH = ROOT / "install.sh"
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


def _make_fake_install(home: Path, version: str = "1.0.0"):
    """
    Set up a minimal fake global install at home/.ai-specs (git repo).
    Returns (ai_specs_path, bare_remote_path).
    """
    ai_specs = home / ".ai-specs"
    ai_specs.mkdir()
    bin_dir = ai_specs / "bin"
    bin_dir.mkdir()
    lib_dir = ai_specs / "lib"
    lib_dir.mkdir()

    # Minimal bin/ai-specs (executable)
    bin_script = bin_dir / "ai-specs"
    bin_script.write_text("#!/usr/bin/env bash\necho ai-specs\n")
    bin_script.chmod(0o755)

    # Copy the real install.sh and upgrade.sh so changes are tested
    (ai_specs / "install.sh").write_text(INSTALL_SH.read_text())
    (lib_dir / "upgrade.sh").write_text(LIB_UPGRADE.read_text())
    (lib_dir / "upgrade.sh").chmod(0o755)

    # A tracked 100644 file that the old installer would chmod
    tracked_file = lib_dir / "dummy.sh"
    tracked_file.write_text("#!/usr/bin/env bash\necho dummy\n")
    # Leave it 100644 (no chmod here — mirrors git storage)

    # VERSION
    (ai_specs / "VERSION").write_text(version + "\n")

    # Symlink
    local_bin = home / ".local" / "bin"
    local_bin.mkdir(parents=True)
    local_bin_link = local_bin / "ai-specs"
    local_bin_link.symlink_to(bin_dir / "ai-specs")

    # Init git repo with a deterministic branch name (main) regardless of the
    # system's init.defaultBranch setting.  Try --initial-branch first (git
    # 2.28+); fall back to renaming the default branch for older git versions.
    try:
        run(["git", "init", "--initial-branch=main"], cwd=ai_specs)
    except subprocess.CalledProcessError:
        run(["git", "init"], cwd=ai_specs)
        # Rename whatever default branch was created to "main"
        try:
            run(["git", "symbolic-ref", "HEAD", "refs/heads/main"], cwd=ai_specs)
        except subprocess.CalledProcessError:
            run(["git", "branch", "-m", "main"], cwd=ai_specs)

    run(["git", "config", "user.email", "test@test.com"], cwd=ai_specs)
    run(["git", "config", "user.name", "Test"], cwd=ai_specs)
    run(["git", "config", "core.fileMode", "true"], cwd=ai_specs)
    run(["git", "add", "."], cwd=ai_specs)
    run(["git", "commit", "-m", "init"], cwd=ai_specs)

    # Bare remote (also pin to main for the same reason)
    bare = home / "origin.git"
    bare.mkdir()
    try:
        run(["git", "init", "--bare", "--initial-branch=main"], cwd=bare)
    except subprocess.CalledProcessError:
        run(["git", "init", "--bare"], cwd=bare)
    run(["git", "remote", "add", "origin", str(bare)], cwd=ai_specs)
    run(["git", "push", "-u", "origin", "main"], cwd=ai_specs)

    return ai_specs, bare


def _make_env(home: Path, extra=None):
    env = os.environ.copy()
    env["HOME"] = str(home)
    env["AI_SPECS_HOME"] = str(home / ".ai-specs")
    env["PATH"] = str(home / ".local" / "bin") + ":" + env.get("PATH", "")
    if extra:
        env.update(extra)
    return env


def _apply_chmod_to_tracked_file(ai_specs: Path, rel_path: str):
    """chmod +x a tracked 100644 file to simulate what the old installer did."""
    target = ai_specs / rel_path
    current = target.stat().st_mode
    target.chmod(current | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)


# ---------------------------------------------------------------------------
# Mode-only dirt is auto-remediated by upgrade.sh
# ---------------------------------------------------------------------------
class UpgradeModeOnlyDirtTests(unittest.TestCase):
    def fake_home(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _run_upgrade(self, env, args=None):
        script = Path(env["AI_SPECS_HOME"]) / "lib" / "upgrade.sh"
        cmd = ["bash", str(script)] + (args or [])
        return run(cmd, env=env, check=False)

    def test_mode_only_dirt_is_remediated_not_aborted(self):
        """
        upgrade.sh must auto-restore mode-only dirt (chmod on tracked 100644
        files, no content changes) instead of aborting with exit 3.
        """
        home = self.fake_home()
        ai_specs, _ = _make_fake_install(home)
        env = _make_env(home)

        # Simulate what the old installer did: chmod +x a tracked 100644 file
        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        # Verify this really is dirty when fileMode=true
        status_before = run(
            ["git", "status", "--porcelain"], cwd=ai_specs, check=True
        )
        self.assertNotEqual(
            status_before.stdout.strip(),
            "",
            "Pre-condition: tree must be dirty after chmod (fileMode=true)",
        )

        result = self._run_upgrade(env)
        self.assertEqual(
            result.returncode,
            0,
            msg=f"upgrade.sh should succeed on mode-only dirt, got:\n"
            f"stdout: {result.stdout}\nstderr: {result.stderr}",
        )

    def test_mode_only_dirt_tree_is_clean_after_remediation(self):
        """
        After upgrade.sh runs on mode-only dirt, the working tree must be clean.
        """
        home = self.fake_home()
        ai_specs, _ = _make_fake_install(home)
        env = _make_env(home)

        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        self._run_upgrade(env)

        status_after = run(
            ["git", "status", "--porcelain"], cwd=ai_specs, check=True
        )
        self.assertEqual(
            status_after.stdout.strip(),
            "",
            "Working tree must be clean after mode-only remediation.",
        )

    def test_mode_only_dirt_prints_informational_message(self):
        """
        upgrade.sh must print an informational message when it remediates
        mode-only dirt, so users understand what happened.
        """
        home = self.fake_home()
        ai_specs, _ = _make_fake_install(home)
        env = _make_env(home)

        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        result = self._run_upgrade(env)
        combined = (result.stdout + result.stderr).lower()
        self.assertTrue(
            "mode" in combined or "restoring" in combined or "file mode" in combined,
            msg=f"Expected an informational message about mode restoration, got:\n"
            f"stdout: {result.stdout}\nstderr: {result.stderr}",
        )


# ---------------------------------------------------------------------------
# Content changes still block the upgrade
# ---------------------------------------------------------------------------
class UpgradeContentDirtBlocksTests(unittest.TestCase):
    def fake_home(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _run_upgrade(self, env, args=None):
        script = Path(env["AI_SPECS_HOME"]) / "lib" / "upgrade.sh"
        cmd = ["bash", str(script)] + (args or [])
        return run(cmd, env=env, check=False)

    def test_content_change_still_blocks_upgrade(self):
        """
        A real content edit must still cause upgrade.sh to abort (exit != 0)
        with a dirty-tree message.
        """
        home = self.fake_home()
        ai_specs, _ = _make_fake_install(home)
        env = _make_env(home)

        # Real content change (not just mode bits)
        (ai_specs / "lib" / "dummy.sh").write_text("# modified content\n")

        result = self._run_upgrade(env)
        self.assertNotEqual(
            result.returncode,
            0,
            msg="upgrade.sh must abort when there is a real content change.",
        )
        combined = (result.stdout + result.stderr).lower()
        self.assertIn(
            "dirty",
            combined,
            msg="Error output must mention 'dirty' when content changes are present.",
        )

    def test_mode_plus_content_change_still_blocks(self):
        """
        If a file has BOTH a mode change and a content change, it is not
        mode-only dirt — upgrade.sh must still abort.
        """
        home = self.fake_home()
        ai_specs, _ = _make_fake_install(home)
        env = _make_env(home)

        target = ai_specs / "lib" / "dummy.sh"
        # Content change
        target.write_text("# different content\n")
        # Mode change on top
        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        result = self._run_upgrade(env)
        self.assertNotEqual(
            result.returncode,
            0,
            msg="upgrade.sh must abort when content is also changed.",
        )


# ---------------------------------------------------------------------------
# install.sh dirty-tree gate also remediates mode-only dirt
# ---------------------------------------------------------------------------
class InstallModeOnlyDirtTests(unittest.TestCase):
    """
    install.sh also has a dirty-tree gate before pull.  It must apply the same
    mode-only remediation logic as upgrade.sh.
    """

    def fake_home(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _make_install_env(self, home: Path, ai_specs: Path, bare: Path):
        env = os.environ.copy()
        env["HOME"] = str(home)
        env["AI_SPECS_HOME"] = str(ai_specs)
        # Point installer at the local bare repo so it doesn't hit the network
        env["AI_SPECS_REPO"] = str(bare)
        env["AI_SPECS_REF"] = "main"
        env["INSTALL_BIN"] = str(home / ".local" / "bin")
        env["PATH"] = str(home / ".local" / "bin") + ":" + env.get("PATH", "")
        return env

    def test_install_sh_remediates_mode_only_dirt(self):
        """
        install.sh must not exit 1 when the only dirt is mode-only changes
        on tracked 100644 files — it must restore and continue.
        """
        home = self.fake_home()
        ai_specs, bare = _make_fake_install(home)
        env = self._make_install_env(home, ai_specs, bare)

        # Simulate the old installer having dirtied the tree with chmod
        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        # Pre-condition: tree is dirty
        status_before = run(
            ["git", "status", "--porcelain"], cwd=ai_specs, check=True
        )
        self.assertNotEqual(status_before.stdout.strip(), "")

        result = run(
            ["bash", str(ai_specs / "install.sh")],
            env=env,
            check=False,
        )
        self.assertEqual(
            result.returncode,
            0,
            msg=f"install.sh should succeed on mode-only dirt.\n"
            f"stdout: {result.stdout}\nstderr: {result.stderr}",
        )

    def test_install_sh_tree_is_clean_after_mode_only_remediation(self):
        """
        After install.sh remediates mode-only dirt, the working tree must be
        clean (mirrors the upgrade.sh equivalent assertion).
        """
        home = self.fake_home()
        ai_specs, bare = _make_fake_install(home)
        env = self._make_install_env(home, ai_specs, bare)

        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        result = run(
            ["bash", str(ai_specs / "install.sh")],
            env=env,
            check=False,
        )
        self.assertEqual(
            result.returncode,
            0,
            msg=f"install.sh should succeed on mode-only dirt.\n"
            f"stdout: {result.stdout}\nstderr: {result.stderr}",
        )

        status_after = run(
            ["git", "status", "--porcelain"], cwd=ai_specs, check=True
        )
        self.assertEqual(
            status_after.stdout.strip(),
            "",
            "Working tree must be clean after install.sh mode-only remediation.",
        )

    def test_install_sh_content_dirt_still_blocks(self):
        """
        install.sh must still exit 1 when there is a real content change.
        """
        home = self.fake_home()
        ai_specs, bare = _make_fake_install(home)
        env = self._make_install_env(home, ai_specs, bare)

        # Real content change
        (ai_specs / "lib" / "dummy.sh").write_text("# content changed\n")

        result = run(
            ["bash", str(ai_specs / "install.sh")],
            env=env,
            check=False,
        )
        self.assertNotEqual(
            result.returncode,
            0,
            msg="install.sh must abort on real content changes.",
        )


# ---------------------------------------------------------------------------
# Mode-only dirt + untracked file → upgrade ABORTS (restore not triggered)
# ---------------------------------------------------------------------------
class UpgradeModeOnlyDirtPlusUntrackedTests(unittest.TestCase):
    """
    When there is mode-only dirt on tracked files AND an untracked file,
    the upgrade must abort (exit != 0).  The restore-and-continue path
    must NOT be taken, and the untracked file must remain untouched.

    This pins the restore condition so it cannot be accidentally widened to
    cover untracked files.
    """

    def fake_home(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _run_upgrade(self, env, args=None):
        script = Path(env["AI_SPECS_HOME"]) / "lib" / "upgrade.sh"
        cmd = ["bash", str(script)] + (args or [])
        return run(cmd, env=env, check=False)

    def test_mode_dirt_plus_untracked_file_aborts_upgrade(self):
        """
        upgrade.sh must abort when there is mode-only dirt AND an untracked
        file.  The untracked file must still be present after the abort.
        """
        home = self.fake_home()
        ai_specs, _ = _make_fake_install(home)
        env = _make_env(home)

        # Mode-only dirt (tracked 100644 file chmod'd)
        _apply_chmod_to_tracked_file(ai_specs, "lib/dummy.sh")

        # Add an untracked file on top of the mode-only dirt
        untracked = ai_specs / "lib" / "untracked_file.sh"
        untracked.write_text("# untracked\n")

        result = self._run_upgrade(env)

        self.assertNotEqual(
            result.returncode,
            0,
            msg="upgrade.sh must abort when mode-only dirt is combined with an "
            f"untracked file.\nstdout: {result.stdout}\nstderr: {result.stderr}",
        )
        self.assertTrue(
            untracked.exists(),
            "Untracked file must remain untouched when upgrade aborts.",
        )


if __name__ == "__main__":
    unittest.main()
