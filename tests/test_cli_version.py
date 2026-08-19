"""Tests for CLI version policy and semver comparison (black-box).

Every test drives `bin/ai-specs <verb>` as a subprocess via the shared
`invoke` helper. The observable surfaces below are FROZEN against the
untouched Bash/Python implementation and were re-verified live in this
worktree:

- `recipe configure` reads the installed version from $AI_SPECS_HOME/VERSION
  during preflight. An **exact** pin requires equality, so both installed
  "0.12.2"/pin "0.12.3" and installed "0.12.3"/pin "0.12.2" are blocked
  (rc 4); installed == pin proceeds (rc 0). `--ignore-cli-version` bypasses
  a blocked preflight (rc 0).
- `doctor` re-resolves its own install root from $BASH_SOURCE (parity §11), so
  its `cli-version` check always reads the real repo VERSION (0.22.0) and
  ignores a custom VERSION in the isolated home.
- `version` prints $AI_SPECS_HOME/VERSION (rc 0); when it is absent it exits 1
  with empty stdout (defect D32).
- A successful `sync` stamps ai-specs/.ai-specs.lock [meta].cli_version to the
  installed version; `doctor` reports it as "last sync <v>".
"""

import tempfile
import tomllib
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home


class _VersionBlackBox(unittest.TestCase):
    """Shared black-box helpers: one isolated home per scenario + fixtures."""

    def _home(self) -> Path:
        """One shared install+cache root per test (default: real VERSION symlink)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _cli_home(self, version_text: str | None) -> Path:
        """Isolated home whose VERSION is a written file (or absent).

        isolated_home() symlinks the repo's real VERSION (0.22.0) into the
        home; unlink it and write the given content so the CLI-under-test
        observes the custom installed version. version_text None leaves the
        VERSION absent (the D32 missing-file case).
        """
        home = self._home()
        version_path = home / "VERSION"
        if version_path.is_symlink() or version_path.exists():
            version_path.unlink()
        if version_text is not None:
            version_path.write_text(version_text + "\n", encoding="utf-8")
        return home

    def _configure(self, root: Path, extra: tuple[str, ...] = ()):
        """recipe configure preflight for git-pr-flow against the shared home."""
        return invoke(
            root, "recipe", "configure", "git-pr-flow", "--set", "base_branch=main",
            *extra, cli_home=self._home(),
        )

    def _doctor(self, root: Path):
        return invoke(root, "doctor", cli_home=self._home())

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _write_manifest(self, root: Path, tool_section: str | None, pre_sync: bool = False) -> Path:
        """Create an isolated project with the given [tool] block (or none).

        Always creates ai-specs/skills + ai-specs/commands so a pre-sync does
        not fail. pre_sync=True runs `sync` against this test's shared home
        (shared-home trap) so a subsequent `doctor` is clean (rc 0) and the
        lock carries [meta].cli_version.
        """
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = (
            '[project]\nname = "test"\n'
            "[agents]\nenabled = [\"claude\"]\n"
            "[recipes.git-pr-flow]\nenabled = true\n"
        )
        if tool_section:
            manifest += tool_section
        (ai_specs / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        if pre_sync:
            result = invoke(root, "sync", cli_home=self._home())
            self.assertEqual(result.returncode, 0, result.stderr)
        return root


class CliVersionCompareTests(_VersionBlackBox):
    """recipe-configure preflight rc is the observable for compare_versions."""

    def test_patch_ordering(self):
        # compare_versions("0.12.2","0.12.3") == -1: an exact pin NEWER than the
        # installed version is not met -> blocked.
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.12.3"\npolicy = "exact"\n'
        )
        self._cli_home("0.12.2")
        self.assertEqual(self._configure(root).returncode, 4)

        # compare_versions("0.12.3","0.12.2") == 1: an installed version NEWER
        # than the pin passes under a min policy -> proceeds.
        root = self._write_manifest(
            self._make_project(), '[tool]\nmin_version = "0.12.2"\n'
        )
        self._cli_home("0.12.3")
        self.assertEqual(self._configure(root).returncode, 0)
    def test_equal_versions(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.12.2"\npolicy = "exact"\n'
        )
        self._cli_home("0.12.2")
        self.assertEqual(self._configure(root).returncode, 0)

    def test_prerelease_lower_than_release(self):
        # compare_versions("0.12.2-rc1","0.12.2") == -1: a prerelease is lower
        # than its release, so the exact pin 0.12.2 is not met -> blocked.
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.12.2"\npolicy = "exact"\n'
        )
        self._cli_home("0.12.2-rc1")
        self.assertEqual(self._configure(root).returncode, 4)
        # --ignore-cli-version bypasses the blocked preflight (rc 0).
        self.assertEqual(
            self._configure(root, extra=("--ignore-cli-version",)).returncode, 0
        )

    def test_build_metadata_ignored(self):
        # compare_versions("0.12.2+build","0.12.2") == 0: build metadata is
        # ignored, so the exact pin 0.12.2 is satisfied -> proceeds.
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.12.2"\npolicy = "exact"\n'
        )
        self._cli_home("0.12.2+build")
        self.assertEqual(self._configure(root).returncode, 0)


class CliVersionPolicyParseTests(_VersionBlackBox):
    """doctor's cli-version line is the observable for parse_tool_policy.

    doctor always reads the real repo VERSION (0.22.0), so the installed
    version below is 0.22.0 regardless of any custom VERSION in the home.
    """

    def test_exact_pin(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "9.9.9"\npolicy = "exact"\n'
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 1)
        self.assertIn("pinned 9.9.9", result.stdout)

    def test_min_inferred_policy(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nmin_version = "9.9.9"\n'
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 1)
        self.assertIn("minimum 9.9.9", result.stdout)

    def test_conflicting_fields_rejected(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.12.2"\nmin_version = "0.11.0"\n'
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 1)
        self.assertIn("both", result.stdout)

    def test_no_tool_section(self):
        # No [tool] block -> parse_tool_policy returns (None, None): doctor
        # emits the INFO "no [tool] pin, last sync unknown" line. The project
        # must be UNSYNCED for that "last sync unknown" branch to appear (a
        # synced project reports OK with the stamped sync version instead).
        root = self._write_manifest(self._make_project(), None)
        result = self._doctor(root)
        self.assertIn("no [tool] pin", result.stdout)
        self.assertIn("last sync unknown", result.stdout)


class CliVersionCheckPolicyTests(_VersionBlackBox):
    """doctor's cli-version severity is the observable for check_policy."""

    def test_exact_match(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.22.0"\npolicy = "exact"\n',
            pre_sync=True,
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 0)
        self.assertIn("installed 0.22.0, pinned 0.22.0", result.stdout)

    def test_exact_mismatch(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "9.9.9"\npolicy = "exact"\n'
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 1)
        self.assertIn("0.22.0", result.stdout)
        self.assertIn("9.9.9", result.stdout)

    def test_min_satisfied(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nmin_version = "0.21.0"\n', pre_sync=True
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("below minimum", result.stdout)

    def test_min_violation(self):
        root = self._write_manifest(
            self._make_project(), '[tool]\nmin_version = "9.9.9"\n'
        )
        result = self._doctor(root)
        self.assertEqual(result.returncode, 1)
        self.assertIn("below minimum", result.stdout)
        self.assertIn("9.9.9", result.stdout)


class CliVersionInstalledTests(_VersionBlackBox):
    """`version` is the observable for read_installed_version."""

    def test_read_installed_version(self):
        self._cli_home("0.12.2")
        result = invoke(self._make_project(), "version", cli_home=self._home())
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "0.12.2\n")

    def test_missing_version_file(self):
        # Defect D32: a missing VERSION makes `version` exit 1 with empty stdout.
        self._cli_home(None)
        result = invoke(self._make_project(), "version", cli_home=self._home())
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")


class CliVersionLockMetaTests(_VersionBlackBox):
    """doctor's "last sync" line + the lock file are the observable for read_lock_meta."""

    def test_read_lock_meta_absent(self):
        # No lock (unsynced project with a satisfied exact pin): doctor reports
        # "last sync unknown" because the lock meta is empty.
        root = self._write_manifest(
            self._make_project(), '[tool]\nversion = "0.22.0"\npolicy = "exact"\n'
        )
        result = self._doctor(root)
        self.assertIn("last sync unknown", result.stdout)

    def test_read_lock_meta_present(self):
        # Sync stamps [meta] into the lock; doctor reports the stamped version.
        root = self._write_manifest(self._make_project(), None, pre_sync=True)
        result = self._doctor(root)
        self.assertIn("last sync 0.22.0", result.stdout)
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        meta = tomllib.loads(lock_path.read_text(encoding="utf-8"))["meta"]
        self.assertEqual(meta["cli_version"], "0.22.0")


if __name__ == "__main__":
    unittest.main()
