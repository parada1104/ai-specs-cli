"""Black-box tests for CLI version policy and semver comparison.

Every test drives ``bin/ai-specs``; no test may import ``lib/_internal``
modules. The two CLI surfaces that consume the version policy are:

- ``recipe configure <id> --inspect --json`` — the read-only preflight report
  exposes the parsed ``[tool]`` policy (``pin``/``pin_kind``), the installed
  CLI version (from the install root's ``VERSION`` file), lock meta state, and
  the policy verdict (``policy_ok`` + ``blocked_reason``).
- ``recipe configure <id> --set ...`` — the frozen exit-4 contract: an apply
  blocked by the ``[tool]`` CLI version policy exits 4 with status "blocked".

The installed version is controlled hermetically by writing a real ``VERSION``
file into the isolated install root (never the repository's).
"""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home  # noqa: E402

BASE_MANIFEST = (
    "[project]\n"
    "name = 'fixture'\n\n"
    "[recipes.worktree-flow]\n"
    "enabled = true\n"
    "version = '1.4.0'\n"
)


class CliVersionBlackBoxTests(unittest.TestCase):
    def _home(self, base: Path, installed: str = "0.12.2") -> Path:
        """Isolated install root with a hermetic VERSION file."""
        home = isolated_home(base)
        version_path = home / "VERSION"
        if version_path.exists() or version_path.is_symlink():
            version_path.unlink()
        version_path.write_text(f"{installed}\n", encoding="utf-8")
        return home

    def _project(self, base: Path, name: str, tool_toml: str = "") -> Path:
        root = base / name
        (root / "ai-specs").mkdir(parents=True)
        manifest = root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(BASE_MANIFEST + tool_toml, encoding="utf-8")
        return root

    def _run(self, installed: str = "0.12.2", tool_toml: str = "",
             lock_toml: str | None = None):
        """One temp workspace with a hermetic install root + project.

        Returns (preflight cli_version dict, project root, home, CLIResult of
        the inspect run).
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = self._home(base, installed)
        root = self._project(base, "prj", tool_toml)
        if lock_toml is not None:
            (root / "ai-specs" / ".ai-specs.lock").write_text(lock_toml, encoding="utf-8")
        result = invoke(
            root, "recipe", "configure", "worktree-flow", "--inspect", "--json",
            cli_home=home, tmpdir=base,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        doc = json.loads(result.stdout)
        # Inspect exposes the parsed policy state; the blocked_reason string is
        # only surfaced by a blocked --set apply (see _apply).
        return doc["preflight"]["cli_version"], root, home, result

    def _apply(self, root: Path, home: Path, *args: str):
        """Blocked-apply surface: the exit-4 report carries blocked_reason."""
        return invoke(
            root, "recipe", "configure", "worktree-flow", "--set", *args,
            "--json", cli_home=home, tmpdir=root.parent,
        )

    def _blocked_reason(self, root: Path, home: Path, *args: str) -> tuple[int, str]:
        result = self._apply(root, home, *args)
        report = json.loads(result.stdout)
        return result.returncode, report["preflight"]["blocked_reason"]


class CliVersionCompareTests(CliVersionBlackBoxTests):
    def test_patch_ordering(self):
        # min policy: installed 0.12.3 satisfies min 0.12.2, but not the reverse.
        state, root, home, _ = self._run(
            installed="0.12.3", tool_toml="\n[tool]\nmin_version = '0.12.2'\n",
        )
        self.assertTrue(state["policy_ok"])
        state, root, home, _ = self._run(
            installed="0.12.2", tool_toml="\n[tool]\nmin_version = '0.12.3'\n",
        )
        self.assertFalse(state["policy_ok"])
        rc, reason = self._blocked_reason(root, home, "integration_branch=dev")
        self.assertEqual(rc, 4)
        self.assertIn("below minimum", reason)

    def test_equal_versions(self):
        state, _root, _home, _ = self._run(
            installed="0.12.2", tool_toml="\n[tool]\nversion = '0.12.2'\n",
        )
        self.assertTrue(state["policy_ok"])
        self.assertEqual(state["pin"], "0.12.2")
        self.assertEqual(state["pin_kind"], "exact")

    def test_prerelease_lower_than_release(self):
        # A release is not equal to its prerelease: exact pin must reject it.
        state, root, home, _ = self._run(
            installed="0.12.2", tool_toml="\n[tool]\nversion = '0.12.2-rc1'\n",
        )
        self.assertFalse(state["policy_ok"])
        rc, reason = self._blocked_reason(root, home, "integration_branch=dev")
        self.assertEqual(rc, 4)
        self.assertIn("does not match pinned", reason)

    def test_build_metadata_ignored(self):
        state, _root, _home, _ = self._run(
            installed="0.12.2", tool_toml="\n[tool]\nversion = '0.12.2+build'\n",
        )
        self.assertTrue(state["policy_ok"])


class CliVersionPolicyParseTests(CliVersionBlackBoxTests):
    def test_exact_pin(self):
        # [tool].version without an explicit policy parses as an exact pin.
        state, _root, _home, _ = self._run(
            tool_toml="\n[tool]\nversion = '0.12.2'\n",
        )
        self.assertEqual(state["pin_kind"], "exact")
        self.assertEqual(state["pin"], "0.12.2")

    def test_min_inferred_policy(self):
        # [tool].min_version without an explicit policy parses as a min pin.
        state, _root, _home, _ = self._run(
            tool_toml="\n[tool]\nmin_version = '0.11.0'\n",
        )
        self.assertEqual(state["pin_kind"], "min")
        self.assertEqual(state["pin"], "0.11.0")

    def test_conflicting_fields_rejected(self):
        state, root, home, _ = self._run(
            tool_toml="\n[tool]\nversion = '0.12.2'\nmin_version = '0.11.0'\n",
        )
        self.assertFalse(state["policy_ok"])
        rc, reason = self._blocked_reason(root, home, "integration_branch=dev")
        self.assertEqual(rc, 4)
        self.assertIn("both", reason)

    def test_no_tool_section(self):
        state, _root, _home, _ = self._run()
        self.assertIsNone(state["pin"])
        self.assertIsNone(state["pin_kind"])
        self.assertTrue(state["policy_ok"])


class CliVersionCheckPolicyTests(CliVersionBlackBoxTests):
    def test_exact_match(self):
        state, _root, _home, _ = self._run(
            installed="0.12.2", tool_toml="\n[tool]\nversion = '0.12.2'\npolicy = 'exact'\n",
        )
        self.assertTrue(state["policy_ok"])
        self.assertEqual(state["pin"], "0.12.2")

    def test_exact_mismatch(self):
        state, root, home, _ = self._run(
            installed="0.11.0", tool_toml="\n[tool]\nversion = '0.12.2'\npolicy = 'exact'\n",
        )
        self.assertEqual(state["installed"], "0.11.0")
        self.assertFalse(state["policy_ok"])
        rc, reason = self._blocked_reason(root, home, "integration_branch=dev")
        self.assertEqual(rc, 4)
        self.assertIn("0.11.0", reason)
        self.assertIn("0.12.2", reason)
        self.assertIn("does not match pinned", reason)
        self.assertEqual(json.loads(self._apply(root, home, "integration_branch=dev").stdout)["status"], "blocked")

    def test_min_satisfied(self):
        state, _root, _home, _ = self._run(
            installed="0.12.2", tool_toml="\n[tool]\nmin_version = '0.11.0'\n",
        )
        self.assertTrue(state["policy_ok"])

    def test_min_violation(self):
        state, root, home, _ = self._run(
            installed="0.10.0", tool_toml="\n[tool]\nmin_version = '0.11.0'\n",
        )
        self.assertFalse(state["policy_ok"])
        rc, reason = self._blocked_reason(root, home, "integration_branch=dev")
        self.assertEqual(rc, 4)
        self.assertIn("below minimum", reason)
        # Frozen exit-4 contract for a blocked min-policy apply.
        result = self._apply(root, home, "integration_branch=dev")
        self.assertEqual(result.returncode, 4)
        self.assertEqual(json.loads(result.stdout)["status"], "blocked")


class CliVersionInstalledTests(CliVersionBlackBoxTests):
    def test_read_installed_version(self):
        state, _root, _home, _ = self._run(installed="0.12.2")
        self.assertEqual(state["installed"], "0.12.2")

    def test_missing_version_file(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = isolated_home(base)
        (home / "VERSION").unlink()
        root = self._project(base, "prj")
        result = invoke(
            root, "recipe", "configure", "worktree-flow", "--inspect", "--json",
            cli_home=home, tmpdir=base,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        state = json.loads(result.stdout)["preflight"]["cli_version"]
        self.assertEqual(state["installed"], "unknown")


class CliVersionLockMetaTests(CliVersionBlackBoxTests):
    def test_read_lock_meta_absent(self):
        state, _root, _home, _ = self._run(lock_toml=None)
        self.assertIsNone(state["lock_cli_version"])
        self.assertEqual(state["lock_state"], "unknown")

    # TRIAGE: recipe configure — inspect exposes the lock's cli_version and a
    # current/stale state but not the synced_at timestamp itself; the original
    # asserted both lock keys from read_lock_meta directly.
    def test_read_lock_meta_present(self):
        state, _root, _home, _ = self._run(
            installed="0.12.2",
            lock_toml='[meta]\ncli_version = "0.12.2"\nsynced_at = "2026-06-23T12:00:00Z"\n',
        )
        self.assertEqual(state["lock_cli_version"], "0.12.2")
        self.assertEqual(state["lock_state"], "current")


if __name__ == "__main__":
    unittest.main()
