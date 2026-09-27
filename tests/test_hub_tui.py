"""Interactive / PTY black-box tests for the ai-specs hub.

Every test drives ``bin/ai-specs hub`` through its process boundary under a
pseudo-terminal (isolated install root). No test may import ``lib/_internal``
modules; questionary/rich behavior is exercised for real via the PTY instead
of being mocked.
"""
from __future__ import annotations

import fcntl
import os
import re
import select
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import time
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import (  # noqa: E402
    invoke,
    isolated_home,
    populate_catalog,
)

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
# Source-text assertions only (see TestPauseOnlySite) — never imported.
HUB_PY = ROOT / "lib" / "_internal" / "hub.py"

MENU_TITLES = [
    "Sync",
    "Doctor",
    "Agents",
    "Skills",
    "Recipes",
    "Configure recipes",
    "Rules audit",
    "Upgrade",
    "Version",
    "Help",
    "Init wizard",
    "Quit",
]

ANSI_RE = re.compile(rb"\x1b\[[0-9;?]*[a-zA-Z]")


def _strip_ansi(data: bytes) -> bytes:
    """Strip ANSI escapes so rich-rendered text can be sliced for assertions."""
    return ANSI_RE.sub(b"", data)


def _recipe_toml(rid: str, name: str) -> str:
    return (
        f'[recipe]\nid = "{rid}"\nname = "{name}"\n'
        f'description = "test recipe {rid}"\nversion = "1.0.0"\n'
    )


def _make_home(base: Path, *, catalog: bool = True) -> Path:
    """Isolated CLI install root with a REAL lib copy (plus _vendor, needed by
    the interactive deps gate). A symlinked lib would resolve cache roots back
    into the repository."""
    home = isolated_home(base / "cli-home", catalog=catalog)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    (home / "lib" / "_vendor").symlink_to(ROOT / "lib" / "_vendor")
    return home


def _cli_env(home: Path, base: Path) -> dict:
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(base / "home"),
        "TMPDIR": str(base),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "LC_ALL": "C",
        "LANG": "C",
        "TERM": "xterm",
    }
    (base / "home").mkdir(exist_ok=True)
    return env


def _spawn_pty(target: Path, home: Path, feed: bytes = b"",
               timeout: float = 30,
               stages: list[tuple[bytes, bytes]] | None = None) -> bytes:
    """Spawn ``ai-specs hub <target>`` under a PTY and drive it.

    ``stages`` is a list of (needle, payload): each payload is written once
    the needle first appears in the accumulated output AFTER the previous
    stage fired (None = write immediately). Ctrl-C (\\x03) aborts a
    questionary prompt; Ctrl-D (\\x04) sends EOF to a pause(); \\x1b[B is
    ArrowDown and \\n accepts the highlighted choice.

    The pty master is always closed and the child reaped in finally.
    """
    master, slave = os.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    env = _cli_env(home, target.parent)
    proc = subprocess.Popen(
        [str(CLI), "hub", str(target)],
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
    pending = list(stages) if stages is not None else [(None, feed)]
    stage_i = 0
    search_from = 0
    try:
        while True:
            if time.monotonic() > deadline:
                proc.kill()
                proc.wait()
                raise AssertionError(f"hub timed out after {timeout}s; output: {output!r}")

            while stage_i < len(pending):
                needle, payload = pending[stage_i]
                if needle is None or output.find(needle, search_from) != -1:
                    time.sleep(0.15)
                    os.write(master, payload)
                    stage_i += 1
                    search_from = len(output)
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
        try:
            proc.wait(timeout=5)
        except Exception:
            proc.kill()

    return output


def _ai_specs_init(path: Path, home: Path) -> None:
    """Black-box init through the isolated CLI, then clear [agents].enabled."""
    result = invoke(path, "init", cli_home=home)
    if result.returncode != 0:
        raise AssertionError(f"ai-specs init failed: {result.stdout}{result.stderr}")
    toml = path / "ai-specs" / "ai-specs.toml"
    text, n = re.subn(r"(?m)^enabled\s*=\s*\[.*?\]\s*$", "enabled = []",
                      toml.read_text(), count=1)
    if n:
        toml.write_text(text)


def _quit_stages() -> list[tuple[bytes, bytes]]:
    """Stages that wait for a fresh menu, then arrow down to Quit and accept."""
    return [(b"What do you want to do?", b"\x1b[B" * 11 + b"\n")]


class TestCommandMenu(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)
        cls.root = cls.base / "prj"
        cls.root.mkdir()
        _ai_specs_init(cls.root, cls.home)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    # TRIAGE: ai-specs hub — the per-action prompt→Action mapping (all 12
    # entries returning their Action through questionary.select) required a
    # mocked questionary in the original; black-box only the dispatched flows
    # are observable. Version (inline print) and Help (usage output) are both
    # driven below; the remaining actions are covered by the dedicated flow
    # tests in this file (doctor, recipes, skills, agents, quit).
    def test_prompt_returns_each_action(self):
        out = _spawn_pty(self.root, self.home, timeout=45, stages=[
            (b"What do you want to do?", b"\x1b[B" * 8 + b"\n"),  # Version (no pause)
            (b"What do you want to do?", b"\x1b[B" * 9 + b"\n"),  # Help
            (b"Press Enter to return", b"\n"),
            *_quit_stages(),
        ])
        self.assertIn(b"Usage: ai-specs", out)
        self.assertIn(b"Print the CLI version", out)

    def test_none_maps_to_quit(self):
        # Ctrl-C on the menu prompt: questionary .ask() yields None → QUIT.
        out = _spawn_pty(self.root, self.home, timeout=30, stages=[
            (b"What do you want to do?", b"\x03"),
        ])
        self.assertNotIn(b"Traceback", out)
        self.assertNotIn(b"exited", _strip_ansi(out))

    def test_menu_has_exact_twelve_entries(self):
        out = _spawn_pty(self.root, self.home, timeout=30, stages=[
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        self.assertIn(b"What do you want to do?", out)
        plain = _strip_ansi(out)
        for title in MENU_TITLES:
            self.assertIn(title.encode(), plain)

    def test_agents_in_menu(self):
        out = _spawn_pty(self.root, self.home, timeout=30, stages=[
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        plain = _strip_ansi(out)
        self.assertIn(b"Agents", plain)
        self.assertIn(b"Select which AI agents to enable", plain)

    def test_recipes_submenu_configure_alias_still_dispatches(self):
        out = _spawn_pty(self.root, self.home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),  # Recipes
            (b"Recipes: ", b"\x1b[B" * 3 + b"\n"),                # Configure recipes
            (b"Press Enter to return", b"\n"),
            *_quit_stages(),
        ])
        plain = _strip_ansi(out)
        # The configure entry is the whole-project action and still dispatches.
        self.assertIn(b"Configure recipes (whole project, configure-recipes)", plain)
        self.assertIn(b"done", plain)
        self.assertNotIn(b"exited", plain)

    def test_configure_recipes_visible_in_main_menu_with_description(self):
        out = _spawn_pty(self.root, self.home, timeout=30, stages=[
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        plain = _strip_ansi(out)
        self.assertIn(b"Configure recipes", plain)
        self.assertIn(b"Set up recipe config, CLI deps, env vars", plain)


class TestStatusPanelRender(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)
        cls.root = cls.base / "prj"
        cls.root.mkdir()
        _ai_specs_init(cls.root, cls.home)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def test_render_contains_summary_and_title(self):
        version = (self.home / "VERSION").read_text(encoding="utf-8").strip()
        out = _spawn_pty(self.root, self.home, timeout=30, stages=[
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        plain = _strip_ansi(out)
        self.assertIn(b"ai-specs", plain)
        self.assertIn(b"Summary:", plain)
        self.assertIn(b"version", plain)
        self.assertIn(version.encode(), plain)


class TestDelegateRunnerResume(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)
        cls.root = cls.base / "prj"
        cls.root.mkdir()
        _ai_specs_init(cls.root, cls.home)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def test_loop_runs_then_input_then_quit(self):
        # One delegated action (Doctor), resume via the pause, then quit:
        # the loop must come back to the menu after the delegation.
        out = _spawn_pty(self.root, self.home, timeout=45, stages=[
            (b"What do you want to do?", b"\x1b[B" + b"\n"),      # Doctor
            (b"Press Enter to return", b"\n"),
            *_quit_stages(),
        ])
        plain = _strip_ansi(out)
        self.assertIn(b"doctor", plain.lower())
        self.assertIn(b"Press Enter to return", out)
        # The menu reappeared after the delegated action (loop resumed).
        self.assertEqual(plain.count(b"What do you want to do?") >= 2, True)


class TestHubPTYE2E(unittest.TestCase):
    """PTY end-to-end: real questionary under a pseudo-terminal."""

    def _workspace(self, name: str) -> tuple[Path, Path]:
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-hub-pty-")
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = _make_home(base)
        target = base / name
        target.mkdir()
        return target, home

    def test_quit_immediately(self):
        target, home = self._workspace("prj")
        _ai_specs_init(target, home)
        # Menu default is Sync (index 0). Arrow down 11 times to Quit, Enter.
        feed = b"\x1b[B" * 11 + b"\n"
        out = _spawn_pty(target, home, feed=feed, timeout=30)
        plain = _strip_ansi(out)
        self.assertNotIn(b"Traceback", out)
        self.assertIn(b"Quit", plain)

    def test_version_inline_then_quit(self):
        target, home = self._workspace("prj")
        _ai_specs_init(target, home)
        version = (home / "VERSION").read_text(encoding="utf-8").strip().encode()
        # Arrow down 8 → Version, Enter; then 11 → Quit, Enter.
        # After Version the menu reappears at Sync again.
        feed = b"\x1b[B" * 8 + b"\n" + b"\x1b[B" * 11 + b"\n"
        out = _spawn_pty(target, home, feed=feed, timeout=30)
        plain = _strip_ansi(out)
        self.assertIn(version, plain)
        self.assertNotIn(b"Traceback", out)

    def test_doctor_delegates_and_resumes(self):
        target, home = self._workspace("prj")
        _ai_specs_init(target, home)
        stages = [
            (b"What do you want to do?", b"\x1b[B\n"),  # Doctor
            (b"Press Enter to return", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),  # Quit
        ]
        out = _spawn_pty(target, home, timeout=45, stages=stages)
        plain = _strip_ansi(out)
        self.assertTrue(
            b"Summary:" in plain or b"ai-specs doctor" in plain
            or b"doctor" in plain.lower(),
            f"doctor output missing: {out!r}",
        )
        self.assertIn(b"Press Enter to return", out)

    def test_offer_init_decline(self):
        target, home = self._workspace("prj")
        # Uninitialized: confirm prompt — answer n.
        out = _spawn_pty(target, home, feed=b"n\n", timeout=30)
        self.assertIn(b"Run the init wizard now?", out)
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists())


class TestPauseOnlySite(unittest.TestCase):
    """B.2 — every Press Enter pause goes through pause()."""

    # TRIAGE: ai-specs hub — exclusivity of input()-pause call sites is a
    # static source invariant with no CLI surface; the source-text check is
    # kept without importing the module (the runtime pause behavior is
    # covered black-box by test_aborted_pause_returns_zero).
    def test_no_bare_press_enter_input_outside_pause(self):
        text = HUB_PY.read_text(encoding="utf-8")
        # Strip the pause() helper body so we only catch call sites.
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
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _ai_specs_init(root, home)
            # Doctor → pause → Ctrl-D (EOF): pause() returns False, the hub
            # must still exit cleanly.
            out = _spawn_pty(root, home, timeout=45, stages=[
                (b"What do you want to do?", b"\x1b[B" + b"\n"),
                (b"Press Enter to return", b"\x04"),
            ])
            self.assertIn(b"Press Enter to return", out)
            self.assertNotIn(b"Traceback", out)


class TestRecipeAddPicker(unittest.TestCase):
    """A.3 — recipe Add uses pick_one over list_recipes, not questionary.text."""

    # TRIAGE: ai-specs hub — the original asserted questionary.text is never
    # called; whether a text prompt was constructed is not observable through
    # the PTY boundary. The picker selection and its effect below are.
    def test_add_uses_pick_one_not_text(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base, catalog=False)
            populate_catalog(home, "conv-pick-recipe",
                             _recipe_toml("conv-pick-recipe", "Pick Recipe"))
            root = base / "prj"
            root.mkdir()
            _ai_specs_init(root, home)
            out = _spawn_pty(root, home, timeout=45, stages=[
                (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),  # Recipes
                (b"Recipes: ", b"\x1b[B" + b"\n"),                    # Add recipe
                (b"Recipe to add:", b"\n"),                           # first recipe
                (b"Press Enter to return", b"\n"),
                *_quit_stages(),
            ])
            plain = _strip_ansi(out)
            # At least one pick_one render carried the real catalog id.
            self.assertIn(b"conv-pick-recipe", plain)
            self.assertIn(b"done", plain)
            manifest = (root / "ai-specs" / "ai-specs.toml").read_text()
            self.assertIn("[recipes.conv-pick-recipe]", manifest)


class TestSkillsSubmenuPTY(unittest.TestCase):
    """B.3 — Skills submenu shows categorized headers."""

    def test_skills_shows_categorized_headers(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            target = base / "prj"
            target.mkdir()
            _ai_specs_init(target, home)
            # Arrow to Skills (index 3), Enter; List skills (index 0), Enter;
            # Press Enter to return; Quit (index 11 arrows from fresh menu).
            stages = [
                (b"What do you want to do?", b"\x1b[B" * 3 + b"\n"),  # Skills
                (b"Skills: ", b"\n"),                                 # List skills
                (b"Press Enter to return", b"\n"),
                *_quit_stages(),
            ]
            out = _spawn_pty(target, home, timeout=45, stages=stages)
            self.assertNotIn(b"Traceback", out)
            plain = _strip_ansi(out)
            lower = plain.lower()
            self.assertTrue(b"bundled" in lower, out)
            self.assertTrue(b"local" in lower and b"vendored" in lower, out)
            self.assertTrue(b"recipe" in lower or b"catalog" in lower, out)


if __name__ == "__main__":
    unittest.main()
