"""Tests for CLI version policy and semver comparison."""

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI_VERSION_PATH = ROOT / "lib" / "_internal" / "cli_version.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class CliVersionCompareTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(CLI_VERSION_PATH, "cli_version_test")

    def test_patch_ordering(self):
        self.assertEqual(self.mod.compare_versions("0.12.2", "0.12.3"), -1)
        self.assertEqual(self.mod.compare_versions("0.12.3", "0.12.2"), 1)

    def test_equal_versions(self):
        self.assertEqual(self.mod.compare_versions("0.12.2", "0.12.2"), 0)

    def test_prerelease_lower_than_release(self):
        self.assertEqual(self.mod.compare_versions("0.12.2-rc1", "0.12.2"), -1)

    def test_build_metadata_ignored(self):
        self.assertEqual(self.mod.compare_versions("0.12.2+build", "0.12.2"), 0)


class CliVersionPolicyParseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(CLI_VERSION_PATH, "cli_version_policy_test")

    def test_exact_pin(self):
        policy, err = self.mod.parse_tool_policy(
            {"tool": {"version": "0.12.2", "policy": "exact"}}
        )
        self.assertIsNone(err)
        self.assertEqual(policy.kind, "exact")
        self.assertEqual(policy.version, "0.12.2")

    def test_min_inferred_policy(self):
        policy, err = self.mod.parse_tool_policy({"tool": {"min_version": "0.11.0"}})
        self.assertIsNone(err)
        self.assertEqual(policy.kind, "min")
        self.assertEqual(policy.version, "0.11.0")

    def test_conflicting_fields_rejected(self):
        policy, err = self.mod.parse_tool_policy(
            {"tool": {"version": "0.12.2", "min_version": "0.11.0"}}
        )
        self.assertIsNone(policy)
        self.assertIn("both", err)

    def test_no_tool_section(self):
        policy, err = self.mod.parse_tool_policy({})
        self.assertIsNone(policy)
        self.assertIsNone(err)


class CliVersionCheckPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(CLI_VERSION_PATH, "cli_version_check_test")

    def test_exact_match(self):
        ok, _ = self.mod.check_policy(
            "0.12.2", self.mod.ToolPolicy(kind="exact", version="0.12.2")
        )
        self.assertTrue(ok)

    def test_exact_mismatch(self):
        ok, reason = self.mod.check_policy(
            "0.11.0", self.mod.ToolPolicy(kind="exact", version="0.12.2")
        )
        self.assertFalse(ok)
        self.assertIn("0.11.0", reason)
        self.assertIn("0.12.2", reason)

    def test_min_satisfied(self):
        ok, _ = self.mod.check_policy(
            "0.12.2", self.mod.ToolPolicy(kind="min", version="0.11.0")
        )
        self.assertTrue(ok)

    def test_min_violation(self):
        ok, reason = self.mod.check_policy(
            "0.10.0", self.mod.ToolPolicy(kind="min", version="0.11.0")
        )
        self.assertFalse(ok)
        self.assertIn("below minimum", reason)


class CliVersionInstalledTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(CLI_VERSION_PATH, "cli_version_installed_test")

    def test_read_installed_version(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            (home / "VERSION").write_text("0.12.2\n")
            self.assertEqual(self.mod.read_installed_version(home), "0.12.2")

    def test_missing_version_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertEqual(self.mod.read_installed_version(Path(tmp)), "unknown")


class CliVersionLockMetaTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(CLI_VERSION_PATH, "cli_version_lock_test")

    def test_read_lock_meta_absent(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / ".ai-specs.lock"
            self.assertEqual(self.mod.read_lock_meta(path), {})

    def test_read_lock_meta_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / ".ai-specs.lock"
            path.write_text(
                '[meta]\ncli_version = "0.12.2"\nsynced_at = "2026-06-23T12:00:00Z"\n'
            )
            meta = self.mod.read_lock_meta(path)
            self.assertEqual(meta["cli_version"], "0.12.2")
        self.assertEqual(meta["synced_at"], "2026-06-23T12:00:00Z")

    def test_version_lock_drift_is_reported_without_rewriting_metadata(self):
        manifest = {"tool": {}}
        severity, name, message = self.mod.evaluate_cli_version(
            installed="0.22.0",
            manifest=manifest,
            lock_meta={"cli_version": "0.21.0", "synced_at": "2026-08-01T00:00:00Z"},
        )
        self.assertEqual((severity, name), ("WARN", "cli-version"))
        self.assertIn("last sync 0.21.0", message)
        self.assertIn("installed 0.22.0", message)


if __name__ == "__main__":
    unittest.main()
