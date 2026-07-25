"""Unit tests for lib/_internal/util.py (dep-free helpers)."""

from __future__ import annotations

import importlib.util
import os
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

def _scrub_tui_modules():
    for name in list(sys.modules):
        if (
            name == "questionary"
            or name == "rich"
            or name.startswith("questionary.")
            or name.startswith("rich.")
            or name.startswith("prompt_toolkit")
            or name.startswith("wcwidth")
            or name.startswith("markdown_it")
            or name.startswith("mdurl")
            or name.startswith("pygments")
        ):
            sys.modules.pop(name, None)

UTIL_PATH = ROOT / "lib" / "_internal" / "util.py"


def _load():
    spec = importlib.util.spec_from_file_location("util", UTIL_PATH)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules["util"] = mod
    spec.loader.exec_module(mod)
    return mod


class TestUtilImportContract(unittest.TestCase):
    def test_imports_without_third_party_deps(self):
        saved = list(sys.path)
        try:
            sys.path[:] = [p for p in sys.path if "_vendor" not in p and "site-packages" not in p]
            _scrub_tui_modules()
            mod = _load()
            self.assertTrue(hasattr(mod, "DEPS_SPEC"))
            self.assertTrue(hasattr(mod, "ai_specs_home"))
            self.assertTrue(hasattr(mod, "ensure_deps"))
        finally:
            sys.path[:] = saved


class TestAiSpecsHome(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_honors_ai_specs_home_env(self):
        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.dict(os.environ, {"AI_SPECS_HOME": tmp}):
                self.assertEqual(self.mod.ai_specs_home(), Path(tmp).resolve())

    def test_fallback_to_parents2_when_env_unset(self):
        env = {k: v for k, v in os.environ.items() if k != "AI_SPECS_HOME"}
        with mock.patch.dict(os.environ, env, clear=True):
            expected = UTIL_PATH.resolve().parents[2]
            self.assertEqual(self.mod.ai_specs_home(), expected)


class TestVendorDir(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_vendor_dir_under_ai_specs_home(self):
        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.dict(os.environ, {"AI_SPECS_HOME": tmp}):
                self.assertEqual(
                    self.mod.vendor_dir(),
                    Path(tmp).resolve() / "lib" / "_vendor",
                )


class TestIsInitialized(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_true_when_manifest_file_exists(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text("x = 1\n")
            self.assertTrue(self.mod.is_initialized(root))

    def test_false_when_manifest_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertFalse(self.mod.is_initialized(Path(tmp)))

    def test_false_when_manifest_is_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs" / "ai-specs.toml").mkdir(parents=True)
            self.assertFalse(self.mod.is_initialized(root))


class TestIsInternalTestRecipe(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_true_for_test_prefix(self):
        self.assertTrue(self.mod.is_internal_test_recipe("test-fixture"))
        self.assertTrue(self.mod.is_internal_test_recipe("test-conflict-a"))

    def test_false_for_public_recipes(self):
        self.assertFalse(self.mod.is_internal_test_recipe("trello-mcp-workflow"))
        self.assertFalse(self.mod.is_internal_test_recipe("tdd-flow"))
        self.assertFalse(self.mod.is_internal_test_recipe("testing-helpers"))

    def test_internal_test_recipe_message(self):
        msg = self.mod.internal_test_recipe_message("test-fixture")
        self.assertIn("test-fixture", msg)
        self.assertIn("internal test fixture", msg)


def _force_missing_deps():
    """Remove rich/questionary from importability."""
    _scrub_tui_modules()


class TestEnsureDeps(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def setUp(self):
        self._saved_path = list(sys.path)
        self._saved_modules = {
            name: sys.modules.get(name)
            for name in list(sys.modules)
            if name == "questionary"
            or name == "rich"
            or name.startswith("rich.")
            or name.startswith("questionary.")
        }
        for name in list(self._saved_modules):
            sys.modules.pop(name, None)

    def tearDown(self):
        sys.path[:] = self._saved_path
        for name in list(sys.modules):
            if (
                name == "questionary"
                or name == "rich"
                or name.startswith("rich.")
                or name.startswith("questionary.")
            ):
                sys.modules.pop(name, None)
        for name, mod in self._saved_modules.items():
            if mod is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = mod

    def test_returns_none_when_packages_importable(self):
        vendor = ROOT / "lib" / "_vendor"
        if not vendor.is_dir():
            try:
                import questionary  # noqa: F401
                from rich.console import Console  # noqa: F401
            except ImportError:
                self.skipTest("rich/questionary not available yet (P1.2 vendors them)")
        rc = self.mod.ensure_deps(vendor if vendor.is_dir() else Path(tempfile.mkdtemp()))
        self.assertIsNone(rc)

    def test_mkdir_failure_returns_3(self):
        class BoomPath(type(Path())):
            def is_dir(self):
                return False

            def mkdir(self, *args, **kwargs):
                raise PermissionError("boom")

        boom = BoomPath("/tmp/ai-specs-util-boom-vendor")
        _force_missing_deps()
        with mock.patch.object(self.mod.sys.stdin, "isatty", return_value=True), mock.patch.object(
            self.mod.sys.stdout, "isatty", return_value=True
        ), mock.patch.object(self.mod.sys.stdin, "readline", return_value="y\n"):
            real_import = __import__

            def fake_import(name, globals=None, locals=None, fromlist=(), level=0):
                if name in ("questionary", "rich") or name.startswith("rich."):
                    raise ImportError(name)
                return real_import(name, globals, locals, fromlist, level)

            with mock.patch("builtins.__import__", side_effect=fake_import):
                rc = self.mod.ensure_deps(boom)
        self.assertEqual(rc, 3)

    def test_non_tty_returns_3_without_prompt(self):
        with tempfile.TemporaryDirectory() as tmp:
            vendor = Path(tmp) / "vendor"
            _force_missing_deps()
            with mock.patch.object(self.mod.sys.stdin, "isatty", return_value=False), mock.patch.object(
                self.mod.sys.stdout, "isatty", return_value=False
            ):
                real_import = __import__

                def fake_import(name, globals=None, locals=None, fromlist=(), level=0):
                    if name in ("questionary", "rich") or name.startswith("rich."):
                        raise ImportError(name)
                    return real_import(name, globals, locals, fromlist, level)

                with mock.patch("builtins.__import__", side_effect=fake_import):
                    rc = self.mod.ensure_deps(vendor)
            self.assertEqual(rc, 3)
            self.assertFalse(vendor.exists())

    def test_pip_install_success_returns_none(self):
        with tempfile.TemporaryDirectory() as tmp:
            vendor = Path(tmp) / "vendor"
            _force_missing_deps()

            with mock.patch.object(self.mod.sys.stdin, "isatty", return_value=True), mock.patch.object(
                self.mod.sys.stdout, "isatty", return_value=True
            ), mock.patch.object(self.mod.sys.stdin, "readline", return_value="y\n"), mock.patch.object(
                self.mod.subprocess, "run"
            ) as run_mock:
                def fake_run(cmd, check=True):
                    # Seed fake packages so the post-install import check passes.
                    q = types.ModuleType("questionary")
                    rich = types.ModuleType("rich")
                    console = types.ModuleType("rich.console")
                    panel = types.ModuleType("rich.panel")
                    console.Console = object
                    panel.Panel = object
                    sys.modules["questionary"] = q
                    sys.modules["rich"] = rich
                    sys.modules["rich.console"] = console
                    sys.modules["rich.panel"] = panel
                    return mock.Mock(returncode=0)

                run_mock.side_effect = fake_run
                real_import = __import__

                def fake_import(name, globals=None, locals=None, fromlist=(), level=0):
                    # Before pip: always miss. After pip: real_import finds seeded modules.
                    if name in ("questionary", "rich") or name.startswith("rich."):
                        if name not in sys.modules:
                            raise ImportError(name)
                    return real_import(name, globals, locals, fromlist, level)

                with mock.patch("builtins.__import__", side_effect=fake_import):
                    rc = self.mod.ensure_deps(vendor)
            self.assertIsNone(rc)
            self.assertTrue(run_mock.called)
            argv = run_mock.call_args[0][0]
            self.assertIn("--target", argv)
            self.assertIn(str(vendor), argv)

    def test_pip_install_failure_returns_3(self):
        with tempfile.TemporaryDirectory() as tmp:
            vendor = Path(tmp) / "vendor"
            _force_missing_deps()
            with mock.patch.object(self.mod.sys.stdin, "isatty", return_value=True), mock.patch.object(
                self.mod.sys.stdout, "isatty", return_value=True
            ), mock.patch.object(self.mod.sys.stdin, "readline", return_value="y\n"), mock.patch.object(
                self.mod.subprocess,
                "run",
                side_effect=self.mod.subprocess.CalledProcessError(1, "pip"),
            ):
                real_import = __import__

                def fake_import(name, globals=None, locals=None, fromlist=(), level=0):
                    if name in ("questionary", "rich") or name.startswith("rich."):
                        raise ImportError(name)
                    return real_import(name, globals, locals, fromlist, level)

                with mock.patch("builtins.__import__", side_effect=fake_import):
                    rc = self.mod.ensure_deps(vendor)
            self.assertEqual(rc, 3)


class TestVendorTree(unittest.TestCase):
    @unittest.skipUnless(
        (ROOT / "lib" / "_vendor" / "rich").is_dir(),
        "vendor not present",
    )
    def test_vendor_packages_importable_with_pin(self):
        vendor = ROOT / "lib" / "_vendor"
        saved = list(sys.path)
        saved_mods = {
            name: sys.modules.get(name)
            for name in list(sys.modules)
            if name in ("questionary", "rich", "prompt_toolkit", "wcwidth")
            or name.startswith("rich.")
            or name.startswith("questionary.")
            or name.startswith("prompt_toolkit.")
            or name.startswith("wcwidth.")
        }
        for name in list(saved_mods):
            sys.modules.pop(name, None)
        try:
            sys.path.insert(0, str(vendor))
            import rich  # noqa: F401
            import questionary  # noqa: F401
            from importlib.metadata import version

            ver = version("rich")
            major = int(str(ver).split(".")[0])
            self.assertGreaterEqual(major, 13)
            self.assertLess(major, 15)
        finally:
            sys.path[:] = saved
            for name in list(sys.modules):
                if (
                    name in ("questionary", "rich", "prompt_toolkit", "wcwidth")
                    or name.startswith("rich.")
                    or name.startswith("questionary.")
                    or name.startswith("prompt_toolkit.")
                    or name.startswith("wcwidth.")
                ):
                    sys.modules.pop(name, None)
            for name, mod in saved_mods.items():
                if mod is None:
                    sys.modules.pop(name, None)
                else:
                    sys.modules[name] = mod


if __name__ == "__main__":
    unittest.main()
