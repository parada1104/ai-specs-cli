"""Unit + shell-gating tests for interactive `ai-specs init` TUI."""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
INIT_TUI = ROOT / "lib" / "_internal" / "init_tui.py"


def _load():
    spec = importlib.util.spec_from_file_location("init_tui", INIT_TUI)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


class TestParseSelection(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_blank_uses_defaults(self):
        self.assertEqual(self.mod._parse_selection("", 5, [0, 2]), [0, 2])

    def test_all_and_none(self):
        self.assertEqual(self.mod._parse_selection("all", 3, []), [0, 1, 2])
        self.assertEqual(self.mod._parse_selection("none", 3, [1]), [])

    def test_csv_and_dedupe(self):
        self.assertEqual(self.mod._parse_selection("1,3,3,2", 4, []), [0, 2, 1])

    def test_invalid(self):
        self.assertIsNone(self.mod._parse_selection("0", 3, []))
        self.assertIsNone(self.mod._parse_selection("9", 3, []))
        self.assertIsNone(self.mod._parse_selection("a,1", 3, []))


class TestRenderManifest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()
        cls.tw = cls.mod._load_toml_write()

    def test_roundtrip_toml(self):
        text = self.mod._render_manifest(
            self.tw,
            "demo",
            ["claude", "pi"],
            [{"id": "session-context", "version": "2.0.0"}, {"id": "tdd-flow", "version": "1.0.0"}],
        )
        data = tomllib.loads(text)
        self.assertEqual(data["project"]["name"], "demo")
        self.assertEqual(data["agents"]["enabled"], ["claude", "pi"])
        self.assertTrue(data["recipes"]["session-context"]["enabled"])
        self.assertEqual(data["recipes"]["tdd-flow"]["version"], "1.0.0")

    def test_dotted_recipe_id_is_quoted_literal_key(self):
        text = self.mod._render_manifest(
            self.tw,
            "demo",
            ["claude"],
            [{"id": "foo.bar", "version": "1.2.3"}],
        )
        self.assertIn('[recipes."foo.bar"]', text)
        data = tomllib.loads(text)
        self.assertIn("foo.bar", data["recipes"])
        self.assertEqual(data["recipes"]["foo.bar"]["version"], "1.2.3")


class TestCatalogRecipes(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_skips_broken_recipe_toml(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            catalog = home / "catalog" / "recipes"
            good = catalog / "good-one"
            bad = catalog / "bad-one"
            good.mkdir(parents=True)
            bad.mkdir(parents=True)
            (good / "recipe.toml").write_text(
                '[recipe]\nid = "good-one"\nname = "Good"\nversion = "1.0.0"\n'
                'description = "ok"\n',
                encoding="utf-8",
            )
            (bad / "recipe.toml").write_text("this is not toml {{{", encoding="utf-8")
            with mock.patch.dict(os.environ, {"AI_SPECS_HOME": str(home)}):
                recipes = self.mod._catalog_recipes()
            ids = [r["id"] for r in recipes]
            self.assertEqual(ids, ["good-one"])


class TestEnsureRichAndMain(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_run_wizard_returns_3_on_non_tty(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp)
            out = target / "out.toml"
            with mock.patch.object(self.mod.sys.stdin, "isatty", return_value=False), mock.patch.object(
                self.mod.sys.stdout, "isatty", return_value=True
            ), mock.patch.object(self.mod, "_ensure_rich", return_value=None):
                rc = self.mod.run_wizard(target=target, name_prefill="x", out_path=out)
            self.assertEqual(rc, 3)
            self.assertFalse(out.exists())

    def test_main_unexpected_exception_returns_3_not_1(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp)
            out = str(Path(tmp) / "out.toml")
            with mock.patch.object(sys, "argv", ["init_tui.py", "--target", str(target), "--out", out]), mock.patch.object(
                self.mod, "run_wizard", side_effect=RuntimeError("boom")
            ):
                rc = self.mod.main()
            self.assertEqual(rc, 3)

    def test_ensure_rich_mkdir_failure_returns_3(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            vendor = home / "lib" / "_vendor"

            class BoomPath(type(vendor)):
                def mkdir(self, *args, **kwargs):  # noqa: ANN002, ANN003
                    raise PermissionError("readonly")

            with mock.patch.object(self.mod, "_ai_specs_home", return_value=home), mock.patch.object(
                self.mod, "_vendor_dir", return_value=BoomPath(vendor)
            ), mock.patch.dict(sys.modules, {"rich": None}):
                # Force ImportError path by making import rich fail, then TTY yes,
                # then answer y, then mkdir boom.
                real_import = __import__

                def fake_import(name, *args, **kwargs):  # noqa: ANN002, ANN003
                    if name == "rich" or name.startswith("rich."):
                        raise ImportError("no rich")
                    return real_import(name, *args, **kwargs)

                with mock.patch("builtins.__import__", side_effect=fake_import), mock.patch.object(
                    self.mod.sys.stdin, "isatty", return_value=True
                ), mock.patch.object(self.mod.sys.stdout, "isatty", return_value=True), mock.patch.object(
                    self.mod.sys.stdin, "readline", return_value="y\n"
                ):
                    rc = self.mod._ensure_rich()
            self.assertEqual(rc, 3)


class TestInitShellGating(unittest.TestCase):
    """Shell integration: auto-TUI stays off without a TTY; stubs cover rc paths."""

    def _workspace(self) -> Path:
        tmp = tempfile.mkdtemp(prefix="ai-specs-init-tui-")
        self.addCleanup(lambda: __import__("shutil").rmtree(tmp, ignore_errors=True))
        return Path(tmp)

    def _run_init(self, target: Path, *args: str, env: dict | None = None):
        full_env = os.environ.copy()
        if env:
            full_env.update(env)
        return subprocess.run(
            [str(CLI), "init", str(target), *args],
            text=True,
            capture_output=True,
            env=full_env,
        )

    def _write_stub(self, target: Path, rc: int, write_out: bool = False) -> Path:
        stub = target / f"stub-tui-{rc}.py"
        body = f"""#!/usr/bin/env python3
import argparse, sys
from pathlib import Path
p = argparse.ArgumentParser()
p.add_argument("--target", required=True)
p.add_argument("--name", default="")
p.add_argument("--out", required=True)
args = p.parse_args()
if {write_out!r}:
    Path(args.out).write_text('[project]\\nname = "stub"\\n\\n[agents]\\nenabled = ["claude"]\\n')
    Path(args.out).with_suffix('.json').write_text('{{"name":"stub","agents":["claude"],"recipes":[]}}\\n')
sys.exit({rc})
"""
        stub.write_text(body, encoding="utf-8")
        stub.chmod(0o755)
        return stub

    def test_non_tty_auto_is_classic(self):
        target = self._workspace()
        result = self._run_init(target)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("tui:     no", result.stdout)
        self.assertTrue((target / "ai-specs" / "ai-specs.toml").is_file())

    def test_no_tui_flag_is_classic(self):
        target = self._workspace()
        result = self._run_init(target, "--no-tui")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("tui:     no", result.stdout)

    def test_name_suppresses_auto_tui(self):
        target = self._workspace()
        result = self._run_init(target, "--name", "named-app")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("tui:     no", result.stdout)
        self.assertIn('name = "named-app"', (target / "ai-specs" / "ai-specs.toml").read_text())

    def test_force_suppresses_auto_tui(self):
        target = self._workspace()
        first = self._run_init(target, "--no-tui")
        self.assertEqual(first.returncode, 0, first.stderr)
        result = self._run_init(target, "--force")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("tui:     no", result.stdout)

    def test_tui_cancel_leaves_no_toml(self):
        target = self._workspace()
        stub = self._write_stub(target, rc=1)
        result = self._run_init(target, "--tui", env={"AI_SPECS_INIT_TUI_PY": str(stub)})
        self.assertEqual(result.returncode, 1)
        self.assertIn("cancelled", result.stderr.lower())
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists())

    def test_tui_unavailable_falls_back_to_classic(self):
        target = self._workspace()
        stub = self._write_stub(target, rc=3)
        result = self._run_init(target, "--tui", env={"AI_SPECS_INIT_TUI_PY": str(stub)})
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertIn("falling back to classic init", result.stderr)
        self.assertIn("tui:     no", result.stdout)
        self.assertTrue((target / "ai-specs" / "ai-specs.toml").is_file())

    def test_tui_success_writes_stub_manifest(self):
        target = self._workspace()
        stub = self._write_stub(target, rc=0, write_out=True)
        result = self._run_init(target, "--tui", env={"AI_SPECS_INIT_TUI_PY": str(stub)})
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertIn("tui:     yes", result.stdout)
        self.assertIn("(from TUI)", result.stdout)
        text = (target / "ai-specs" / "ai-specs.toml").read_text()
        self.assertIn('name = "stub"', text)


if __name__ == "__main__":
    unittest.main()
