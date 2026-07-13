"""Unit + shell tests for ai-specs hub (dep-free core)."""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
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

CLI = ROOT / "bin" / "ai-specs"
HUB_PY = ROOT / "lib" / "_internal" / "hub.py"


def _load():
    spec = importlib.util.spec_from_file_location("hub", HUB_PY)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules["hub"] = mod
    spec.loader.exec_module(mod)
    return mod


def _ai_specs_init(path: Path, *, clear_agents: bool = True) -> None:
    subprocess.run([str(CLI), "init", str(path)], check=True, text=True, capture_output=True)
    if clear_agents:
        import re

        toml = path / "ai-specs" / "ai-specs.toml"
        text = toml.read_text()
        text2, n = re.subn(
            r"(?m)^enabled\s*=\s*\[.*?\]\s*$",
            "enabled = []",
            text,
            count=1,
        )
        if n == 1:
            toml.write_text(text2)


class TestHubImportContract(unittest.TestCase):
    def test_imports_without_third_party_deps(self):
        saved = list(sys.path)
        try:
            sys.path[:] = [p for p in sys.path if "_vendor" not in p and "site-packages" not in p]
            _scrub_tui_modules()
            mod = _load()
            self.assertTrue(hasattr(mod, "decide_mode"))
            self.assertTrue(hasattr(mod, "status_summary"))
            self.assertNotIn("rich", sys.modules)
        finally:
            sys.path[:] = saved


class TestGatingDecision(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_four_state_matrix(self):
        Mode = self.mod.Mode
        cases = [
            (True, True, Mode.INTERACTIVE_HUB),
            (True, False, Mode.NONINTERACTIVE_STATUS),
            (False, True, Mode.OFFER_INIT),
            (False, False, Mode.ERROR_UNINITIALIZED),
        ]
        for initialized, tty, expected in cases:
            with self.subTest(initialized=initialized, tty=tty):
                self.assertIs(
                    self.mod.decide_mode(initialized=initialized, tty=tty),
                    expected,
                )


class TestIsInitialized(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_hub_util_is_initialized_same_callable(self):
        self.assertIs(self.mod._util.is_initialized, self.mod._util.is_initialized)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.assertFalse(self.mod._util.is_initialized(root))
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text("x=1\n")
            self.assertTrue(self.mod._util.is_initialized(root))


class TestStatusSummary(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_healthy_init_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            summary = self.mod.status_summary(root)
            self.assertGreaterEqual(summary.ok, 1)
            self.assertEqual(summary.exit_code, 0)
            self.assertTrue(summary.headline)

    def test_warn_when_no_agents(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            # init template may already WARN on empty agents; clear enabled explicitly
            toml = root / "ai-specs" / "ai-specs.toml"
            text = toml.read_text()
            import re

            text2, n = re.subn(
                r"(?m)^enabled\s*=\s*\[.*?\]\s*$",
                "enabled = []",
                text,
                count=1,
            )
            if n == 1:
                toml.write_text(text2)
            summary = self.mod.status_summary(root)
            self.assertGreaterEqual(summary.warn, 1)
            self.assertIn("warning", summary.headline)


class TestDelegateRunner(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_argv_shape_and_returncode(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp)
            cli = Path(tmp) / "fake-cli"
            cli.write_text("#!/bin/sh\nexit 7\n")
            cli.chmod(0o755)
            runner = self.mod.DelegateRunner(cli=cli, target=target)
            with mock.patch.object(self.mod.subprocess, "run") as run_mock:
                run_mock.return_value = mock.Mock(returncode=7)
                rc = runner.run(self.mod.Action.VERSION)
            self.assertEqual(rc, 7)
            argv = run_mock.call_args[0][0]
            self.assertEqual(argv, [str(cli), "version", str(target)])

    def test_extra_args_appended(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp)
            cli = Path(tmp) / "fake-cli"
            runner = self.mod.DelegateRunner(cli=cli, target=target)
            with mock.patch.object(self.mod.subprocess, "run") as run_mock:
                run_mock.return_value = mock.Mock(returncode=0)
                runner.run(self.mod.Action.UPGRADE, extra=["--dry-run"])
            argv = run_mock.call_args[0][0]
            self.assertEqual(argv, [str(cli), "upgrade", "--dry-run", str(target)])


class TestNonInteractiveStatus(unittest.TestCase):
    def test_initialized_piped_prints_status_and_commands(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            result = subprocess.run(
                [str(CLI)],
                cwd=str(root),
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            out = result.stdout
            self.assertIn("Summary", out)
            for name in ("Sync", "Doctor", "Quit", "Version"):
                self.assertIn(name, out)

    def test_uninitialized_piped_exits_2(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = subprocess.run(
                [str(CLI)],
                cwd=tmp,
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("init", result.stderr.lower())

    def test_help_unchanged(self):
        result = subprocess.run(
            [str(CLI), "help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("ai-specs — declarative per-project AI agent config", result.stdout)
        self.assertIn("Usage: ai-specs <command> [args]", result.stdout)
        self.assertIn("doctor", result.stdout)

    def test_unknown_command_still_exits_2(self):
        result = subprocess.run(
            [str(CLI), "definitely-not-a-command"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("unknown command", result.stderr)

    def test_explicit_hub_target(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            result = subprocess.run(
                [str(CLI), "hub", str(root)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("Summary", result.stdout)

    def test_hub_help(self):
        result = subprocess.run(
            [str(CLI), "hub", "--help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("Usage: ai-specs hub", result.stdout)


class TestNoOpenptyInHub(unittest.TestCase):
    def test_hub_py_has_no_openpty_or_termios(self):
        text = HUB_PY.read_text(encoding="utf-8")
        self.assertNotIn("openpty", text)
        self.assertNotIn("termios", text)


if __name__ == "__main__":
    unittest.main()
