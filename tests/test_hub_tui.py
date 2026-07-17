"""Interactive / PTY tests for ai-specs hub (deps-gated)."""

from __future__ import annotations

import importlib.util
import io
import os
import select
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
HUB_PY = ROOT / "lib" / "_internal" / "hub.py"
VENDOR = ROOT / "lib" / "_vendor"


def _has_deps() -> bool:
    vendor = VENDOR
    saved = list(sys.path)
    if vendor.is_dir():
        sys.path.insert(0, str(vendor))
    try:
        importlib.import_module("rich")
        importlib.import_module("questionary")
        return True
    except ImportError:
        return False
    finally:
        sys.path[:] = saved


def _load_hub():
    if VENDOR.is_dir() and str(VENDOR) not in sys.path:
        sys.path.insert(0, str(VENDOR))
    spec = importlib.util.spec_from_file_location("hub_tui", HUB_PY)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules["hub_tui"] = mod
    spec.loader.exec_module(mod)
    return mod


def _ai_specs_init(path: Path) -> None:
    subprocess.run([str(CLI), "init", str(path)], check=True, text=True, capture_output=True)
    import re

    toml = path / "ai-specs" / "ai-specs.toml"
    text = toml.read_text()
    text2, n = re.subn(r"(?m)^enabled\s*=\s*\[.*?\]\s*$", "enabled = []", text, count=1)
    if n == 1:
        toml.write_text(text2)


@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestCommandMenu(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load_hub()

    def test_prompt_returns_each_action(self):
        import questionary

        menu = self.mod.CommandMenu()
        for action in self.mod.Action:
            with self.subTest(action=action):
                fake_select = mock.Mock()
                fake_select.ask.return_value = action
                with mock.patch.object(questionary, "select", return_value=fake_select) as sel:
                    got = menu.prompt()
                self.assertIs(got, action)
                self.assertTrue(sel.called)

    def test_none_maps_to_quit(self):
        import questionary

        menu = self.mod.CommandMenu()
        fake_select = mock.Mock()
        fake_select.ask.return_value = None
        with mock.patch.object(questionary, "select", return_value=fake_select):
            self.assertIs(menu.prompt(), self.mod.Action.QUIT)

    def test_menu_has_exact_eleven_entries(self):
        self.assertEqual(len(self.mod._MENU), 11)
        titles = [t for _, t, _ in self.mod._MENU]
        self.assertEqual(
            titles,
            [
                "Sync",
                "Doctor",
                "Agents",
                "Skills",
                "Recipes",
                "Rules audit",
                "Upgrade",
                "Version",
                "Help",
                "Init wizard",
                "Quit",
            ],
        )

    def test_agents_in_menu(self):
        entry = self.mod._MENU[2]
        self.assertIs(entry[0], self.mod.Action.AGENTS)
        self.assertEqual(entry[1], "Agents")

    def test_configure_recipes_nested_under_recipes_action(self):
        self.assertTrue(hasattr(self.mod.Action, "CONFIGURE_RECIPES"))
        self.assertEqual(self.mod.Action.CONFIGURE_RECIPES.value, "configure-recipes")


@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestStatusPanelRender(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load_hub()

    def test_render_contains_summary_and_title(self):
        from rich.console import Console

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            summary = self.mod.status_summary(root)
            panel = self.mod.StatusPanel(summary).render()
            buf = io.StringIO()
            Console(file=buf, width=80, force_terminal=True).print(panel)
            text = buf.getvalue()
            self.assertIn("ai-specs", text)
            self.assertIn(str(root), text)
            self.assertIn("Summary", text)
            self.assertIn(summary.version, text)
            self.assertIn("version", text)


@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestDelegateRunnerResume(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load_hub()

    def test_loop_runs_then_input_then_quit(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            actions = iter([self.mod.Action.DOCTOR, self.mod.Action.QUIT])

            with mock.patch.object(self.mod.CommandMenu, "prompt", side_effect=lambda self=None: next(actions)), mock.patch.object(
                self.mod.DelegateRunner, "run", return_value=0
            ) as run_mock, mock.patch("builtins.input", return_value=""), mock.patch.object(
                self.mod.StatusPanel, "render", return_value="panel"
            ), mock.patch("rich.console.Console") as cons:
                cons.return_value.print = mock.Mock()
                rc = self.mod._run_interactive_hub(root)
            self.assertEqual(rc, 0)
            self.assertEqual(run_mock.call_count, 1)


@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestHubPTYE2E(unittest.TestCase):
    """PTY end-to-end: real questionary under a pseudo-terminal."""

    def _workspace(self) -> Path:
        tmp = tempfile.mkdtemp(prefix="ai-specs-hub-pty-")
        self.addCleanup(lambda: __import__("shutil").rmtree(tmp, ignore_errors=True))
        return Path(tmp)

    def _spawn_pty(self, target: Path, feed: bytes, timeout: float = 12, stages=None):
        """Spawn hub under a PTY.

        ``feed`` is written once after a short delay (simple flows).
        ``stages`` is an optional list of (needle: bytes|None, payload: bytes)
        written when ``needle`` appears in the accumulated output (None = immediate).
        """
        master, slave = os.openpty()
        env = os.environ.copy()
        env["AI_SPECS_HOME"] = str(ROOT)
        env["TERM"] = "xterm"
        proc = subprocess.Popen(
            [sys.executable, str(HUB_PY), str(target)],
            stdin=slave,
            stdout=slave,
            stderr=slave,
            close_fds=True,
            env=env,
            cwd=str(target),
        )
        os.close(slave)

        output = b""
        deadline = time.monotonic() + timeout
        stage_i = 0
        pending = list(stages) if stages is not None else [(None, feed)]
        sent_initial = False

        try:
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    proc.kill()
                    proc.wait()
                    self.fail(f"hub timed out after {timeout}s; output: {output!r}")

                # Send staged input when needles match.
                while stage_i < len(pending):
                    needle, payload = pending[stage_i]
                    if needle is None or needle in output:
                        time.sleep(0.15)
                        os.write(master, payload)
                        stage_i += 1
                        sent_initial = True
                    else:
                        break

                rlist, _, _ = select.select([master], [], [], 0.5)
                if rlist:
                    try:
                        chunk = os.read(master, 4096)
                        if not chunk:
                            break
                        output += chunk
                    except OSError:
                        break

                if proc.poll() is not None:
                    try:
                        while True:
                            r, _, _ = select.select([master], [], [], 0.1)
                            if not r:
                                break
                            c = os.read(master, 4096)
                            if not c:
                                break
                            output += c
                    except OSError:
                        pass
                    break
        finally:
            try:
                os.close(master)
            except OSError:
                pass
            proc.wait(timeout=5)

        return proc.returncode, output

    def test_quit_immediately(self):
        target = self._workspace()
        _ai_specs_init(target)
        # Menu default is Sync (index 0). Arrow down 9 times to Quit, Enter.
        feed = b"\x1b[B" * 10 + b"\n"
        rc, output = self._spawn_pty(target, feed)
        self.assertEqual(rc, 0, f"output: {output!r}")
        self.assertNotIn(b"Traceback", output)

    def test_version_inline_then_quit(self):
        target = self._workspace()
        _ai_specs_init(target)
        version = (ROOT / "VERSION").read_text(encoding="utf-8").strip().encode()
        # Arrow down 7 → Version, Enter; then 10 → Quit, Enter.
        # After Version the menu reappears at Sync again.
        feed = b"\x1b[B" * 7 + b"\n" + b"\x1b[B" * 10 + b"\n"
        rc, output = self._spawn_pty(target, feed, timeout=15)
        self.assertEqual(rc, 0, f"output: {output!r}")
        self.assertIn(version, output)

    def test_doctor_delegates_and_resumes(self):
        target = self._workspace()
        _ai_specs_init(target)
        stages = [
            (b"What do you want to do?", b"\x1b[B\n"),  # Doctor
            (b"Press Enter to return", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 10 + b"\n"),  # Quit
        ]
        rc, output = self._spawn_pty(target, b"", timeout=25, stages=stages)
        self.assertEqual(rc, 0, f"output: {output!r}")
        self.assertTrue(
            b"Summary:" in output or b"ai-specs doctor" in output or b"doctor" in output.lower(),
            f"doctor output missing: {output!r}",
        )
        self.assertIn(b"Press Enter to return", output)

    def test_offer_init_decline(self):
        target = self._workspace()
        # Uninitialized: confirm prompt — answer n.
        feed = b"n\n"
        rc, output = self._spawn_pty(target, feed, timeout=12)
        self.assertEqual(rc, 0, f"output: {output!r}")
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists())



@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestPauseOnlySite(unittest.TestCase):
    """B.2 — every Press Enter pause goes through pause()."""

    def test_no_bare_press_enter_input_outside_pause(self):
        text = HUB_PY.read_text(encoding="utf-8")
        # Strip the pause() helper body so we only catch call sites.
        import re
        stripped = re.sub(
            r"def pause\(.*?(?=\n(?:def |class |\Z))",
            "",
            text,
            count=1,
            flags=re.DOTALL,
        )
        self.assertNotIn('input("Press Enter', stripped)
        self.assertIn("def pause(", text)

    def test_aborted_pause_returns_zero(self):
        mod = _load_hub()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            actions = iter([mod.Action.DOCTOR, mod.Action.QUIT])
            with mock.patch.object(mod.CommandMenu, "prompt", side_effect=lambda self=None: next(actions)), mock.patch.object(
                mod.DelegateRunner, "run", return_value=0
            ), mock.patch.object(mod, "pause", return_value=False), mock.patch.object(
                mod.StatusPanel, "render", return_value="panel"
            ), mock.patch("rich.console.Console") as cons:
                cons.return_value.print = mock.Mock()
                rc = mod._run_interactive_hub(root)
            self.assertEqual(rc, 0)


@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestRecipeAddPicker(unittest.TestCase):
    """A.3 — recipe Add uses pick_one over list_recipes, not questionary.text."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load_hub()

    def test_add_uses_pick_one_not_text(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            captured = {}

            def fake_pick_one(message, options, **kwargs):
                captured["message"] = message
                captured["options"] = options
                return options[0][1] if options else None

            actions = iter([self.mod.Action.RECIPES, self.mod.Action.QUIT])
            with mock.patch.object(self.mod.CommandMenu, "prompt", side_effect=lambda self=None: next(actions)), mock.patch.object(
                self.mod, "pick_one", side_effect=fake_pick_one
            ) as pick_mock, mock.patch.object(
                self.mod, "pause", return_value=True
            ), mock.patch.object(
                self.mod.DelegateRunner, "run", return_value=0
            ) as run_mock, mock.patch.object(
                self.mod.StatusPanel, "render", return_value="panel"
            ), mock.patch("rich.console.Console") as cons, mock.patch(
                "questionary.text"
            ) as text_mock, mock.patch.object(
                self.mod._recipes, "list_recipes", return_value=[
                    {"id": "git-pr-flow", "name": "Git PR Flow", "version": "1.0.0", "status": "available"},
                ]
            ):
                cons.return_value.print = mock.Mock()
                # First pick_one is submenu; return "add". Second is recipe id.
                picks = iter(["add", "git-pr-flow"])

                def pick_side(message, options, **kwargs):
                    captured.setdefault("calls", []).append((message, list(options)))
                    return next(picks)

                pick_mock.side_effect = pick_side
                rc = self.mod._run_interactive_hub(root)
            self.assertEqual(rc, 0)
            text_mock.assert_not_called()
            # At least one pick_one call carried real catalog ids as values
            value_lists = [[v for _, v in opts] for _, opts in captured["calls"]]
            self.assertTrue(any("git-pr-flow" in vs for vs in value_lists), captured)
            run_mock.assert_any_call(self.mod.Action.RECIPES, extra=["add", "git-pr-flow"])


@unittest.skipUnless(_has_deps(), "rich/questionary not importable")
class TestSkillsSubmenuPTY(unittest.TestCase):
    """B.3 — Skills submenu shows categorized headers."""

    def _workspace(self) -> Path:
        tmp = tempfile.mkdtemp(prefix="ai-specs-hub-skills-")
        self.addCleanup(lambda: __import__("shutil").rmtree(tmp, ignore_errors=True))
        return Path(tmp)

    def test_skills_shows_categorized_headers(self):
        # Reuse PTY harness from TestHubPTYE2E
        harness = TestHubPTYE2E()
        harness.addCleanup = self.addCleanup
        target = self._workspace()
        _ai_specs_init(target)
        # Arrow to Skills (index 3), Enter; List skills (index 0), Enter;
        # Press Enter to return; Quit (index 10).
        stages = [
            (b"What do you want to do?", b"\x1b[B" * 3 + b"\n"),  # Skills
            (b"Skills:", b"\n"),  # List skills (default)
            (b"Press Enter to return", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 10 + b"\n"),  # Quit
        ]
        rc, output = harness._spawn_pty(target, b"", timeout=25, stages=stages)
        self.assertEqual(rc, 0, f"output: {output!r}")
        self.assertNotIn(b"Traceback", output)
        lower = output.lower()
        self.assertTrue(b"bundled" in lower, output)
        self.assertTrue(b"local" in lower and b"vendored" in lower, output)
        self.assertTrue(b"recipe" in lower or b"catalog" in lower, output)



if __name__ == "__main__":
    unittest.main()
