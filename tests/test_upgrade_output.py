"""Output contract for `ai-specs upgrade`.

`tests/test_upgrade.py` locks the safety behavior (channel detection, dirty
tree, divergence, symlink integrity). This file locks what the command
*prints*: compact by default, full detail under --verbose, and everything
dumped when a step fails.

The safety assertions here are deliberately narrow — they exist to prove the
output rework did not change exit codes, not to re-test the guards.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
BIN_AI_SPECS = ROOT / "bin" / "ai-specs"
LIB_UPGRADE = ROOT / "lib" / "upgrade.sh"
CHANGELOG_PY = ROOT / "lib" / "_internal" / "changelog.py"

# Markers that only appear when raw git output reaches the terminal.
GIT_NOISE = (
    "remote: Enumerating objects",
    "remote: Counting objects",
    "Receiving objects:",
    "Resolving deltas:",
    "Fast-forward",
    "files changed,",
    "create mode ",
)


def run(args, cwd=None, env=None, check=True):
    result = subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True)
    if check and result.returncode != 0:
        raise subprocess.CalledProcessError(
            result.returncode, args, output=result.stdout, stderr=result.stderr
        )
    return result


class UpgradeOutputTests(unittest.TestCase):
    # --- harness ------------------------------------------------------------

    def fake_home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def setup_install(self, home: Path, version="0.21.0") -> tuple[Path, Path]:
        """Fake global install plus a bare origin. Returns (ai_specs, bare)."""
        ai_specs = home / ".ai-specs"
        (ai_specs / "bin").mkdir(parents=True)
        (ai_specs / "lib" / "_internal").mkdir(parents=True)

        (ai_specs / "bin" / "ai-specs").write_text(BIN_AI_SPECS.read_text())
        (ai_specs / "bin" / "ai-specs").chmod(0o755)
        (ai_specs / "lib" / "upgrade.sh").write_text(LIB_UPGRADE.read_text())
        (ai_specs / "lib" / "upgrade.sh").chmod(0o755)
        shutil.copy(CHANGELOG_PY, ai_specs / "lib" / "_internal" / "changelog.py")
        (ai_specs / "VERSION").write_text(version + "\n")
        (ai_specs / "CHANGELOG.md").write_text(self.changelog(version))

        local_bin = home / ".local" / "bin"
        local_bin.mkdir(parents=True)
        (local_bin / "ai-specs").symlink_to(ai_specs / "bin" / "ai-specs")

        run(["git", "init", "-b", "main"], cwd=ai_specs)
        run(["git", "config", "user.email", "t@t.com"], cwd=ai_specs)
        run(["git", "config", "user.name", "T"], cwd=ai_specs)
        run(["git", "add", "."], cwd=ai_specs)
        run(["git", "commit", "-m", "init"], cwd=ai_specs)

        bare = home / "origin.git"
        bare.mkdir()
        run(["git", "init", "--bare"], cwd=bare)
        run(["git", "remote", "add", "origin", str(bare)], cwd=ai_specs)
        run(["git", "push", "-u", "origin", "main"], cwd=ai_specs)
        return ai_specs, bare

    def changelog(self, *versions: str) -> str:
        head = "# Changelog\n\n## [Unreleased]\n\n"
        body = ""
        for version in versions:
            body += f"## [{version}] — 2026-08-17\n\n### Added\n- thing for {version}.\n\n"
        return head + body

    def publish_new_version(self, ai_specs: Path, bare: Path, version: str, files=30):
        """Push a new version to origin so the install has something to pull.

        `files` is large enough that a raw diffstat would be unmistakable.
        """
        clone = ai_specs.parent / "publisher"
        if clone.exists():
            shutil.rmtree(clone)
        run(["git", "clone", str(bare), str(clone)])
        run(["git", "config", "user.email", "t@t.com"], cwd=clone)
        run(["git", "config", "user.name", "T"], cwd=clone)
        (clone / "VERSION").write_text(version + "\n")
        (clone / "CHANGELOG.md").write_text(self.changelog(version, "0.21.0"))
        payload = clone / "payload"
        payload.mkdir(exist_ok=True)
        for index in range(files):
            (payload / f"file{index:03d}.txt").write_text(f"content {index}\n")
        run(["git", "add", "-A"], cwd=clone)
        run(["git", "commit", "-m", f"release {version}"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)

    def make_env(self, home: Path) -> dict:
        env = os.environ.copy()
        env["HOME"] = str(home)
        env["AI_SPECS_HOME"] = str(home / ".ai-specs")
        env["PATH"] = str(home / ".local" / "bin") + ":" + env.get("PATH", "")
        return env

    def upgrade(self, home: Path, *args: str, check=False):
        script = home / ".ai-specs" / "lib" / "upgrade.sh"
        return run(
            ["bash", str(script), *args], env=self.make_env(home), check=check
        )

    # --- compact by default -------------------------------------------------

    def test_successful_upgrade_hides_git_output(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        self.publish_new_version(ai_specs, bare, "0.22.0")

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stderr)

        combined = result.stdout + result.stderr
        for marker in GIT_NOISE:
            self.assertNotIn(marker, combined, f"raw git output leaked: {marker!r}")
        self.assertNotIn("payload/file000.txt", combined)

    def test_successful_upgrade_still_reports_the_version_change(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        self.publish_new_version(ai_specs, bare, "0.22.0")

        result = self.upgrade(home)
        self.assertIn("0.21.0", result.stdout)
        self.assertIn("0.22.0", result.stdout)

    def test_compact_mode_prints_one_line_per_step(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        self.publish_new_version(ai_specs, bare, "0.22.0")

        result = self.upgrade(home)
        self.assertIn("fetching", result.stdout.lower())

    # --- verbose ------------------------------------------------------------

    def test_verbose_restores_git_output(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        self.publish_new_version(ai_specs, bare, "0.22.0")

        result = self.upgrade(home, "--verbose")
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        combined = result.stdout + result.stderr
        self.assertTrue(
            any(marker in combined for marker in GIT_NOISE),
            msg=f"expected raw git detail under --verbose, got:\n{combined}",
        )

    def test_short_verbose_flag_is_accepted(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        self.publish_new_version(ai_specs, bare, "0.22.0")

        result = self.upgrade(home, "-v")
        self.assertEqual(result.returncode, 0, msg=result.stderr)

    def test_unknown_flag_is_still_rejected(self):
        home = self.fake_home()
        self.setup_install(home)
        result = self.upgrade(home, "--nonsense")
        self.assertNotEqual(result.returncode, 0)

    # --- failure surfaces everything ---------------------------------------

    def test_fetch_failure_dumps_output_and_keeps_exit_code(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        shutil.rmtree(bare)  # origin disappears after preflight

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 4, msg=result.stdout + result.stderr)
        combined = result.stdout + result.stderr
        self.assertIn("Failed to fetch", combined)
        # The underlying git error must not be swallowed by compact mode.
        self.assertTrue(
            "not a git repository" in combined.lower()
            or "does not appear to be a git repository" in combined.lower()
            or "could not read from remote" in combined.lower(),
            msg=f"git's own error was swallowed:\n{combined}",
        )

    # --- safety behavior unchanged -----------------------------------------

    def test_dirty_tree_still_blocks_with_exit_3(self):
        home = self.fake_home()
        ai_specs, _ = self.setup_install(home)
        (ai_specs / "VERSION").write_text("9.9.9\n")

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("dirty", (result.stdout + result.stderr).lower())

    def test_already_up_to_date_is_quiet(self):
        home = self.fake_home()
        self.setup_install(home)

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        combined = result.stdout + result.stderr
        self.assertIn("up to date", combined.lower())
        for marker in GIT_NOISE:
            self.assertNotIn(marker, combined)

    def test_dry_run_output_is_unchanged(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home)
        self.publish_new_version(ai_specs, bare, "0.22.0")
        run(["git", "fetch", "origin", "main"], cwd=ai_specs)

        result = self.upgrade(home, "--dry-run")
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("Dry-run: no changes will be made.", result.stdout)
        self.assertIn("Current version: 0.21.0", result.stdout)
        self.assertIn("Target version:  0.22.0", result.stdout)

    def test_help_documents_verbose(self):
        home = self.fake_home()
        self.setup_install(home)
        result = self.upgrade(home, "--help")
        self.assertEqual(result.returncode, 0)
        self.assertIn("--verbose", result.stdout)


if __name__ == "__main__":
    unittest.main()
