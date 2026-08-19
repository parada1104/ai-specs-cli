import importlib.util
import tempfile
import unittest
from pathlib import Path
from _blackbox import cache_project_dir, cache_project_key, invoke, isolated_home, snapshot, temp_project, tree_diff

class BlackBoxHelperTests(unittest.TestCase):
    def test_invoke_uses_real_install_and_isolated_cache(self):
        td, root = temp_project()
        self.addCleanup(td.cleanup)
        result = invoke(root, "doctor")
        self.assertIn("ERROR", result.stdout)
        self.assertNotEqual(result.returncode, 127)
        # Explicit home keeps the cache observable for this assertion.
        with tempfile.TemporaryDirectory() as home:
            home_path = isolated_home(Path(home))
            result = invoke(root, "refresh-bundled", cli_home=home_path)
            self.assertEqual(result.returncode, 0)
            self.assertTrue(cache_project_dir(root, home_path).is_dir())
            self.assertFalse(cache_project_dir(root, Path(__file__).resolve().parents[1]).exists())

    def test_doctor_warn_only_is_zero_and_errors_nonzero(self):
        td, root = temp_project()
        self.addCleanup(td.cleanup)
        with tempfile.TemporaryDirectory() as home:
            home_path = isolated_home(Path(home))
            invoke(root, "sync", cli_home=home_path)
            result = invoke(root, "doctor", cli_home=home_path)
            self.assertEqual(result.returncode, 0)
            self.assertRegex(result.stdout, r"[0-9]+ OK, [0-9]+ INFO, [1-9][0-9]* WARN, 0 ERROR")
            (root / "ai-specs" / "ai-specs.toml").unlink()
            result = invoke(root, "doctor", cli_home=home_path)
            self.assertEqual(result.returncode, 1)
            self.assertRegex(result.stdout, r"[0-9]+ ERROR")

    def test_frozen_cache_key_matches_implementation(self):
        # TRIAGE: the parity test intentionally couples to the frozen implementation rule.
        impl_path = Path(__file__).resolve().parents[1] / "lib" / "_internal" / "project-cache.py"
        spec = importlib.util.spec_from_file_location("project_cache_parity", impl_path)
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        for name in ("_leading", "trailing_", ".dotfile", "a.b.", "__x__", "normal"):
            with tempfile.TemporaryDirectory(prefix="cache-parity-") as td:
                root = Path(td) / name
                root.mkdir()
                self.assertEqual(cache_project_key(root), module.cache_key(root))

    def test_snapshot_and_tree_diff_capture_kinds_and_changes(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td); before = snapshot(root)
            (root / "created").write_text("a")
            (root / "target").write_text("x")
            (root / "relative").symlink_to("target")
            after = snapshot(root)
            self.assertEqual(after["relative"], ("symlink", "target"))
            (root / "target").write_text("y")
            (root / "created").unlink()
            (root / "modified").write_text("z")
            diff = tree_diff(after, snapshot(root))
            self.assertEqual(diff["created"], ["modified"])
            self.assertEqual(diff["deleted"], ["created"])
            self.assertIn("target", diff["modified"])

if __name__ == "__main__":
    unittest.main()
