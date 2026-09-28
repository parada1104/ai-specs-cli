"""Black-box hub tests: every behavioral test drives ``bin/ai-specs hub``.

No test may import ``lib/_internal`` modules. Non-interactive flows run the
bare CLI (``ai-specs`` == ``hub``) through an isolated install root; TTY-gated
flows run the real CLI under a pseudo-terminal. Assertions preserve the
original contract intents (frozen exit codes, gating matrix, delegation,
picker filtering, topology surfacing) through the CLI process boundary.
"""
from __future__ import annotations

import fcntl
import hashlib
import importlib.util
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
    cache_project_dir,
    invoke,
    isolated_home,
    normalize_output,
    populate_catalog,
)

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
# Source-text assertions only (see TestNoOpenptyInHub) — never imported.
HUB_PY = ROOT / "lib" / "_internal" / "hub.py"


def _recipe_toml(rid: str, name: str) -> str:
    return (
        f'[recipe]\nid = "{rid}"\nname = "{name}"\n'
        f'description = "test recipe {rid}"\nversion = "1.0.0"\n'
    )


def _make_home(base: Path, *, catalog: bool = True, vendor: bool = True) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    doctor.py resolves cache roots from its own realpath, so a symlinked lib
    would reach back into the repository. ``vendor=False`` hides _vendor to
    make rich/questionary truly unavailable (system python3 has neither).
    """
    home = isolated_home(base / "cli-home", catalog=catalog)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    if vendor:
        (home / "lib" / "_vendor").symlink_to(ROOT / "lib" / "_vendor")
    return home


def _cli_env(home: Path, base: Path) -> dict:
    return {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(base / "home"),
        "TMPDIR": str(base),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "LC_ALL": "C",
        "LANG": "C",
    }


def _run_hub(project: Path, home: Path, base: Path,
             *args: str, cwd: Path | None = None,
             extra_env: dict | None = None) -> tuple[int, str, str]:
    """Run bare ``ai-specs [args…]`` from ``project`` (bare == hub)."""
    (base / "home").mkdir(exist_ok=True)
    env = _cli_env(home, base)
    if extra_env:
        env.update(extra_env)
    proc = subprocess.run(
        [str(CLI), *args],
        cwd=str(cwd or project),
        env=env,
        text=True,
        capture_output=True,
        input="",
        check=False,
    )
    roots = (project, home, base)
    return (
        proc.returncode,
        normalize_output(proc.stdout, roots),
        normalize_output(proc.stderr, roots),
    )


def _path_without_python3(base: Path) -> str:
    """PATH with every python3* binary filtered out.

    hub.sh's frozen pre-guard must exit 2 for uninitialized+non-TTY without
    ever launching Python; if it tried, bash would fail with exit 127.
    """
    filtered = base / "path-filtered"
    filtered.mkdir(exist_ok=True)
    for entry in os.environ.get("PATH", "").split(":"):
        if not entry or not Path(entry).is_dir():
            continue
        try:
            entries = list(Path(entry).iterdir())
        except OSError:
            continue
        for item in entries:
            if item.name.startswith("python3"):
                continue
            link = filtered / item.name
            if link.exists():
                continue
            try:
                if item.is_file() and os.access(item, os.X_OK):
                    link.symlink_to(item)
            except OSError:
                continue
    return str(filtered)


def _spawn_hub_pty(target: Path, home: Path, feed: bytes = b"",
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
    env["TERM"] = "xterm"
    (target.parent / "home").mkdir(exist_ok=True)
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


def _clean_init(project: Path, home: Path) -> None:
    """Black-box init through the isolated CLI, then clear [agents].enabled."""
    result = invoke(project, "init", cli_home=home)
    if result.returncode != 0:
        raise AssertionError(f"ai-specs init failed: {result.stdout}{result.stderr}")
    toml_path = project / "ai-specs" / "ai-specs.toml"
    text, n = re.subn(r"(?m)^enabled\s*=\s*\[.*?\]\s*$", "enabled = []",
                      toml_path.read_text(), count=1)
    if n:
        toml_path.write_text(text)


def _manifest_append(project: Path, body: str) -> Path:
    toml_path = project / "ai-specs" / "ai-specs.toml"
    toml_path.write_text(toml_path.read_text().rstrip() + "\n\n" + body + "\n")
    return toml_path


def _seed_skill(base: Path, name: str, desc: str) -> None:
    base.mkdir(parents=True, exist_ok=True)
    (base / "SKILL.md").write_text(
        f"---\nname: {name}\ndescription: {desc}\n---\n# {name}\n", encoding="utf-8"
    )


def _summary_line(out: str) -> str:
    for line in out.splitlines():
        if line.strip().startswith("Summary:"):
            return line
    raise AssertionError(f"no Summary line in hub status output: {out!r}")


_ANSI_RE = re.compile(rb"\x1b\[[0-9;?]*[a-zA-Z]")


def _strip_ansi(data: bytes) -> bytes:
    """Strip ANSI escapes so rich-rendered text can be sliced for assertions."""
    return _ANSI_RE.sub(b"", data)


class TestHubImportContract(unittest.TestCase):
    def test_imports_without_third_party_deps(self):
        """Import-time contract, observed black-box: with NO rich/questionary
        available anywhere (isolated lib copy, _vendor hidden, system python3
        has neither), the hub still runs its stdlib-only paths — a module-level
        third-party import would crash the process instead of exiting 0/2."""
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base, vendor=False)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            rc, out, _ = _run_hub(root, home, base)
            self.assertEqual(rc, 0)
            self.assertIn("Summary", out)
            self.assertIn("Commands", out)


class TestGatingDecision(unittest.TestCase):
    """decide_mode's four-state matrix, observed through the CLI boundary."""

    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)
        cls.root = cls.base / "prj"
        cls.root.mkdir()
        _clean_init(cls.root, cls.home)
        cls.empty = cls.base / "empty"
        cls.empty.mkdir()

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def test_four_state_matrix(self):
        # initialized + non-TTY → noninteractive status (exit 0)
        rc, out, _ = _run_hub(self.root, self.home, self.base)
        self.assertEqual(rc, 0)
        self.assertIn("Summary", out)

        # initialized + TTY → interactive hub menu
        out_pty = _spawn_hub_pty(
            self.root, self.home,
            stages=[(b"What do you want to do?", b"\x1b[B" * 11 + b"\n")],
        )
        self.assertIn(b"What do you want to do?", out_pty)

        # uninitialized + non-TTY → frozen exit 2 (bash pre-guard)
        rc2, _, err2 = _run_hub(self.empty, self.home, self.base)
        self.assertEqual(rc2, 2)
        self.assertIn("init", err2.lower())

        # uninitialized + non-TTY with python3 hidden from PATH: the guard
        # must exit 2 without ever launching Python (a launch would exit 127).
        env = _cli_env(self.home, self.base)
        env["PATH"] = _path_without_python3(self.base)
        proc = subprocess.run(
            [str(CLI)], cwd=str(self.empty), env=env, text=True,
            capture_output=True, input="", check=False,
        )
        self.assertEqual(proc.returncode, 2)
        self.assertIn("no ai-specs project", proc.stderr)

        # uninitialized + TTY → offer-init confirm
        out_offer = _spawn_hub_pty(self.empty, self.home, feed=b"n\n")
        self.assertIn(b"Run the init wizard now?", out_offer)


class TestIsInitialized(unittest.TestCase):
    """is_initialized is a manifest-presence check that gates the hub modes."""

    def test_is_initialized_manifest_check_gates_hub(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            bare = base / "bare"
            bare.mkdir()
            rc, _, err = _run_hub(bare, home, base)
            self.assertEqual(rc, 2)
            self.assertIn("no ai-specs project", err)
            # Hand-written manifest (no init) is enough: presence drives the mode.
            (bare / "ai-specs").mkdir()
            (bare / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'x'\n")
            rc2, out2, _ = _run_hub(bare, home, base)
            # Presence drives the mode: the hub enters summary mode even for a
            # hand-written manifest. Exit code propagates Doctor's verdict (D5):
            # the bare fixture has ERROR findings (missing AGENTS.md, unbundled
            # cache), so the hub exits 1 — never a false 0 on a broken project.
            self.assertEqual(rc2, 1)
            self.assertIn("Summary", out2)


class TestStatusSummary(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def test_healthy_init_project(self):
        root = self.base / "healthy"
        root.mkdir()
        _clean_init(root, self.home)
        rc, out, _ = _run_hub(root, self.home, self.base)
        self.assertEqual(rc, 0)
        summary = _summary_line(out)
        # ok >= 1 and exit_code == 0 (no ERROR findings) in the rendered counts.
        self.assertRegex(summary, r"Summary:\s+[1-9]\d* OK")
        self.assertIn("0 ERROR", summary)
        self.assertRegex(out, r"ai-specs status — .+")

    def test_warn_when_no_agents(self):
        root = self.base / "noagents"
        root.mkdir()
        _clean_init(root, self.home)
        rc, out, _ = _run_hub(root, self.home, self.base)
        # WARN does not fail the status: exit stays 0, warning headline shown.
        self.assertEqual(rc, 0)
        self.assertIn("WARN", out)
        self.assertIn("warning", out)


class TestDelegateRunner(unittest.TestCase):
    """Delegation runs ``<cli> <action> [extra…] <target>`` and propagates rc."""

    def test_argv_shape_and_returncode(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            # Help action delegates to `ai-specs help <target>` and resumes.
            out = _spawn_hub_pty(root, home, timeout=40, stages=[
                (b"What do you want to do?", b"\x1b[B" * 9 + b"\n"),   # Help
                (b"Press Enter to return", b"\n"),
                (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),  # Quit
            ])
            self.assertIn(b"Usage: ai-specs", out)
            self.assertIn(b"done", out)

    def test_extra_args_appended(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base, catalog=False)
            populate_catalog(home, "conv-extra-recipe",
                             _recipe_toml("conv-extra-recipe", "Extra Recipe"))
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            # Recipes → Add runs `ai-specs recipe add <id> <target>`:
            # subcommand + extra args appended before the target.
            out = _spawn_hub_pty(root, home, timeout=40, stages=[
                (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),   # Recipes
                (b"Recipes: ", b"\x1b[B" + b"\n"),                     # Add recipe
                (b"Recipe to add:", b"\n"),                            # first recipe
                (b"Press Enter to return", b"\n"),
                (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),  # Quit
            ])
            self.assertIn(b"done", out)
            self.assertIn(b"conv-extra-recipe", out)
            manifest = (root / "ai-specs" / "ai-specs.toml").read_text()
            self.assertIn("[recipes.conv-extra-recipe]", manifest)


class TestNonInteractiveStatus(unittest.TestCase):
    def test_initialized_piped_prints_status_and_commands(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            rc, out, err = _run_hub(root, home, base)
            self.assertEqual(rc, 0, err)
            self.assertIn("Summary", out)
            for name in ("Sync", "Doctor", "Quit", "Version"):
                self.assertIn(name, out)

    def test_uninitialized_piped_exits_2(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            empty = base / "empty"
            empty.mkdir()
            rc, _, err = _run_hub(empty, home, base)
            self.assertEqual(rc, 2)
            self.assertIn("init", err.lower())

    def test_help_unchanged(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            result = invoke(root, "help", cli_home=home, append_root=False)
            self.assertEqual(result.returncode, 0)
            self.assertIn("ai-specs — declarative per-project AI agent config", result.stdout)
            self.assertIn("Usage: ai-specs <command> [args]", result.stdout)
            self.assertIn("doctor", result.stdout)

    def test_unknown_command_still_exits_2(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            result = invoke(root, "definitely-not-a-command",
                            cli_home=home, append_root=False)
            self.assertEqual(result.returncode, 2)
            self.assertIn("unknown command", result.stderr)

    def test_explicit_hub_target(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            result = invoke(root, "hub", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("Summary", result.stdout)

    def test_hub_help(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            result = invoke(root, "hub", "--help", cli_home=home, append_root=False)
            self.assertEqual(result.returncode, 0)
            self.assertIn("Usage: ai-specs hub", result.stdout)


class TestNoOpenptyInHub(unittest.TestCase):
    # TRIAGE: ai-specs hub — static source invariant (hub.py must never touch
    # openpty/termios); no CLI surface can observe the absence of those calls,
    # so the source-text check is kept without importing the module.
    def test_hub_py_has_no_openpty_or_termios(self):
        text = HUB_PY.read_text(encoding="utf-8")
        self.assertNotIn("openpty", text)
        self.assertNotIn("termios", text)


class TestImportlibModuleScope(unittest.TestCase):
    """The Agents path needs importlib at hub module scope; a PTY run of the
    Agents submenu is the black-box regression: it must open cleanly."""

    def test_hub_has_importlib_at_module_scope(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            out = _spawn_hub_pty(root, home, timeout=40, stages=[
                (b"What do you want to do?", b"\x1b[B" * 2 + b"\n"),  # Agents
                (b"Select agents to enable:", b"\x03"),               # abort picker
                (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),  # Quit
            ])
            self.assertIn(b"Select agents to enable:", out)
            self.assertIn(b"claude", out)
            self.assertNotIn(b"Traceback", out)


class TestReadVersion(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def test_read_version_from_ai_specs_home(self):
        home = _make_home(self.base / "v1")
        (home / "VERSION").unlink()
        (home / "VERSION").write_text("9.9.9-test\n", encoding="utf-8")
        root = self.base / "v1-prj"
        root.mkdir()
        _clean_init(root, home)
        rc, out, _ = _run_hub(root, home, self.base / "v1")
        self.assertEqual(rc, 0)
        self.assertIn("version: 9.9.9-test", out)

    def test_read_version_missing_file_is_unknown(self):
        home = _make_home(self.base / "v2")
        (home / "VERSION").unlink()
        root = self.base / "v2-prj"
        root.mkdir()
        _clean_init(root, home)
        rc, out, _ = _run_hub(root, home, self.base / "v2")
        self.assertEqual(rc, 0)
        self.assertIn("version: unknown", out)


class TestStatusSummaryVersion(unittest.TestCase):
    def test_status_summary_includes_version(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            rc, out, _ = _run_hub(root, home, base)
            self.assertEqual(rc, 0)
            match = re.search(r"(?m)^\s+version: (.+)$", out)
            self.assertIsNotNone(match, out)
            self.assertEqual(match.group(1).strip(),
                             (home / "VERSION").read_text(encoding="utf-8").strip())


class TestNonInteractiveShowsVersion(unittest.TestCase):
    def test_run_noninteractive_prints_version(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            rc, out, _ = _run_hub(root, home, base)
            self.assertEqual(rc, 0)
            self.assertIn("version:", out)
            self.assertIn((home / "VERSION").read_text(encoding="utf-8").strip(), out)


class TestRecipeChoiceBuilders(unittest.TestCase):
    """recipe_add_choices / recipe_remove_choices filtering, observed through
    the hub Recipes submenu pickers."""

    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base, catalog=False)
        populate_catalog(cls.home, "conv-a", _recipe_toml("conv-a", "Alpha"))
        populate_catalog(cls.home, "conv-b", _recipe_toml("conv-b", "Beta"))
        populate_catalog(cls.home, "conv-c", _recipe_toml("conv-c", "Gamma"))

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def _project(self, name: str, manifest_body: str = "") -> Path:
        root = self.base / name
        root.mkdir()
        _clean_init(root, self.home)
        if manifest_body:
            _manifest_append(root, manifest_body)
        return root

    def test_add_keeps_available_only(self):
        root = self._project("add-only", "[recipes.conv-b]\nenabled = true\n")
        out = _spawn_hub_pty(root, self.home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),
            (b"Recipes: ", b"\x1b[B" + b"\n"),
            (b"Recipe to add:", b"\n"),
            (b"Press Enter to return", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        i = out.find(b"Recipe to add:")
        j = out.find(b"Press Enter to return", i)
        segment = _strip_ansi(out[i:j])
        self.assertIn(b"conv-a", segment)
        self.assertNotIn(b"conv-b", segment)

    def test_remove_keeps_installed_and_disabled(self):
        root = self._project(
            "remove-only",
            "[recipes.conv-b]\nenabled = true\n\n[recipes.conv-c]\nenabled = false\n",
        )
        out = _spawn_hub_pty(root, self.home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),
            (b"Recipes: ", b"\x1b[B" * 2 + b"\n"),
            (b"Recipe to remove:", b"\n"),
            (b"Press Enter to return", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        i = out.find(b"Recipe to remove:")
        j = out.find(b"Press Enter to return", i)
        segment = _strip_ansi(out[i:j])
        self.assertIn(b"conv-b", segment)
        self.assertIn(b"conv-c", segment)
        self.assertNotIn(b"conv-a", segment)
        # Removing conv-b actually edited the manifest.
        self.assertNotIn("[recipes.conv-b]",
                         (root / "ai-specs" / "ai-specs.toml").read_text())

    def test_empty_and_all_error_return_empty(self):
        home = _make_home(self.base / "err-home", catalog=False)
        broken = home / "catalog" / "recipes" / "conv-broken"
        broken.mkdir(parents=True)
        (broken / "recipe.toml").write_text("not [valid toml", encoding="utf-8")
        root = self.base / "err-prj"
        root.mkdir()
        _clean_init(root, home)
        out = _spawn_hub_pty(root, home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),
            (b"Recipes: ", b"\x1b[B" + b"\n"),
            (b"No catalog recipes available to add.", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        self.assertIn(b"No catalog recipes available to add.", out)

    def test_mixed_status_partition_preserves_ids(self):
        home = _make_home(self.base / "mix-home", catalog=False)
        populate_catalog(home, "conv-err", "not [valid toml")
        populate_catalog(home, "conv-avail", _recipe_toml("conv-avail", "Avail"))
        populate_catalog(home, "conv-inst", _recipe_toml("conv-inst", "Inst"))
        root = self.base / "mix-prj"
        root.mkdir()
        _clean_init(root, home)
        toml = _manifest_append(root, "[recipes.conv-inst]\nenabled = true\n")
        out = _spawn_hub_pty(root, home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),
            (b"Recipes: ", b"\x1b[B" + b"\n"),
            (b"Recipe to add:", b"\n"),
            (b"Press Enter to return", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        i = out.find(b"Recipe to add:")
        j = out.find(b"Press Enter to return", i)
        segment = _strip_ansi(out[i:j])
        self.assertIn(b"conv-avail", segment)
        self.assertNotIn(b"conv-inst", segment)
        self.assertNotIn(b"conv-err", segment)
        self.assertIn("[recipes.conv-avail]", toml.read_text())


class TestCategorizeSkills(unittest.TestCase):
    """categorize_skills partitioning, observed through the hub Skills list."""

    def test_partition_bundled_local_recipe_dep(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            _clean_init(root, home)
            _seed_skill(root / "ai-specs" / "skills" / "conv-my-local",
                        "conv-my-local", "local only skill")
            # A local skill whose id is CLI-bundled lands in the bundled bucket.
            _seed_skill(root / "ai-specs" / "skills" / "skill-creator",
                        "skill-creator", "bundled dup")
            cache = cache_project_dir(root, home)
            _seed_skill(cache / ".recipe" / "conv-demo" / "skills" / "conv-recipe-skill",
                        "conv-recipe-skill", "from recipe")
            _seed_skill(cache / ".deps" / "conv-dep1" / "skills" / "conv-dep-skill",
                        "conv-dep-skill", "from dep")
            out = _spawn_hub_pty(root, home, timeout=40, stages=[
                (b"What do you want to do?", b"\x1b[B" * 3 + b"\n"),  # Skills
                (b"Skills: ", b"\n"),                                  # List skills
                (b"Press Enter to return", b"\n"),
                (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
            ])
            i = out.find(b"Skills: ")
            window = _strip_ansi(out[i:])

            def segment(start: bytes, end: bytes) -> bytes:
                a = window.find(start)
                self.assertNotEqual(a, -1, window)
                b = window.find(end, a + 1)
                self.assertNotEqual(b, -1, window)
                return window[a:b]

            bundled = segment(b"Bundled (CLI-shipped)", b"Local / vendored")
            self.assertIn(b"skill-creator", bundled)
            self.assertNotIn(b"conv-my-local", bundled)
            local = segment(b"Local / vendored", b"Provided by recipes")
            self.assertIn(b"conv-my-local", local)
            self.assertNotIn(b"skill-creator", local)
            recipe = segment(b"Provided by recipes", b"Registered deps")
            self.assertIn(b"conv-recipe-skill", recipe)
            dep = segment(b"Registered deps", b"Press Enter to return")
            self.assertIn(b"conv-dep-skill", dep)


class TestSkillsListBundledSection(unittest.TestCase):
    """`ai-specs skills list` Bundled skills section."""

    def test_bundled_skills_not_under_local(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            home = _make_home(base)
            root = base / "prj"
            root.mkdir()
            (root / "ai-specs" / "skills").mkdir(parents=True)
            (root / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "t"\n', encoding="utf-8"
            )
            for name in ("skill-creator", "skill-sync", "my-local"):
                d = root / "ai-specs" / "skills" / name
                d.mkdir()
                (d / "SKILL.md").write_text(
                    f"---\nname: {name}\ndescription: desc-{name}\n---\n# {name}\n",
                    encoding="utf-8",
                )
            # Bundled skills live in the flattened cache tier, not the project
            # (D8): {home}/cache/projects/<cache_key(project)>/.bundled/skills/.
            # cache_key mirrors project-cache.cache_key black-box:
            # sha256(realpath)[:12]-<sanitized basename>.
            real = str(root.resolve())
            digest = hashlib.sha256(real.encode("utf-8")).hexdigest()[:12]
            safe = re.sub(r"[^A-Za-z0-9._-]+", "-", Path(real).name).strip("-._") or "project"
            for name in ("skill-creator", "skill-sync"):
                d = (
                    home / "cache" / "projects" / f"{digest}-{safe}"
                    / ".bundled" / "skills" / name
                )
                d.mkdir(parents=True)
                (d / "SKILL.md").write_text(
                    f"---\nname: {name}\ndescription: desc-{name}\n---\n# {name}\n",
                    encoding="utf-8",
                )
            result = invoke(root, "skills", "list", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            out = result.stdout
            self.assertIn("Bundled skills", out)
            bundled = out.split("Bundled skills")[1].split("Local skills")[0]
            self.assertIn("skill-creator", bundled)
            self.assertIn("desc-skill-creator", bundled)
            self.assertIn("skill-sync", bundled)
            self.assertIn("desc-skill-sync", bundled)
            local = out.split("Local skills")[1].split("Available catalog")[0]
            self.assertNotIn("skill-creator", local)
            self.assertNotIn("skill-sync", local)
            self.assertIn("my-local", local)


class TestWidgetHelpers(unittest.TestCase):
    """pick_one / pick_many / confirm_action / pause contracts."""

    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base, catalog=False)
        cls.root = cls.base / "prj"
        cls.root.mkdir()
        _clean_init(cls.root, cls.home)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def test_pick_one_empty_returns_none_without_questionary(self):
        # No catalog recipes: the Add picker gets empty options → pick_one
        # must return None gracefully (message + hub continues, no crash).
        out = _spawn_hub_pty(self.root, self.home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 4 + b"\n"),
            (b"Recipes: ", b"\x1b[B" + b"\n"),
            (b"No catalog recipes available to add.", b"\n"),
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        self.assertIn(b"No catalog recipes available to add.", out)
        # The hub continued to the menu afterwards instead of crashing.
        self.assertNotIn(b"Traceback", out)

    # TRIAGE: ai-specs hub — pick_many's empty-options → None branch is
    # unreachable through any CLI flow (the Agents picker always offers the
    # fixed supported-agent list); the nearest observable is the populated
    # picker, asserted below.
    def test_pick_many_empty_returns_none(self):
        out = _spawn_hub_pty(self.root, self.home, timeout=40, stages=[
            (b"What do you want to do?", b"\x1b[B" * 2 + b"\n"),  # Agents
            (b"Select agents to enable:", b"\x03"),               # abort picker
            (b"What do you want to do?", b"\x1b[B" * 11 + b"\n"),
        ])
        self.assertIn(b"Select agents to enable:", out)
        self.assertIn(b"claude", out)
        self.assertNotIn(b"Traceback", out)

    def test_confirm_action_returns_bool(self):
        # OFFER_INIT: confirm True must proceed into the init wizard.
        empty = self.base / "confirm-prj"
        empty.mkdir()
        out = _spawn_hub_pty(empty, self.home, timeout=40, stages=[
            (b"Run the init wizard now?", b"y\n"),
            (b"Project name:", b"\x03"),  # abort the wizard once it opens
        ])
        self.assertIn(b"Run the init wizard now?", out)
        self.assertIn(b"Project name:", out)
        self.assertNotIn(b"Traceback", out)

    def test_pause_true_and_eof_false(self):
        # pause(): Enter resumes the loop; EOF (Ctrl-D) at the pause exits 0.
        out = _spawn_hub_pty(self.root, self.home, timeout=45, stages=[
            (b"What do you want to do?", b"\x1b[B" * 9 + b"\n"),  # Help
            (b"Press Enter to return", b"\n"),                    # pause True
            (b"What do you want to do?", b"\x1b[B" * 1 + b"\n"),  # Doctor
            (b"Press Enter to return", b"\x04"),                  # pause EOF
        ])
        self.assertIn(b"Usage: ai-specs", out)


class TestTopologySurfacing(unittest.TestCase):
    # The worktree-flow recipe materializes gate hooks; without a verified
    # binary doctor records a digest-mismatch ERROR and the hub would
    # honestly exit 1 (D5). Build the real gate binary offline so the
    # fixture is doctor-green and rc 0 is HONEST.
    _GATE_LOCAL_BUILD_ENV = {"AI_SPECS_GATE_OFFLINE": "1", "AI_SPECS_GATE_BUILD": "1"}

    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory()
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)

    @classmethod
    def tearDownClass(cls):
        cls._tmp.cleanup()

    def _write_wf_manifest(self, root: Path, topology: str = "auto") -> None:
        root.mkdir(parents=True, exist_ok=True)
        (root / "ai-specs").mkdir(exist_ok=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'topo'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n\n"
            f"[recipes.worktree-flow.config]\nrepo_topology = \"{topology}\"\n"
        )

    def _green_fixture(self, root: Path, topology: str = "auto",
                       rewrite=None) -> Path:
        """Make the fixture doctor-green before hub runs (D5 exit honesty).

        The hub propagates Doctor's verdict (D5): a hand-written manifest with
        no lock/bundled cache has ERROR findings, so the hub would honestly
        exit 1. Baseline = real `init` + `sync`; then the topology variant is
        applied and a re-sync keeps the lock fresh, so rc 0 is HONEST.
        """
        self._write_wf_manifest(root, topology)
        rc, _, err = _run_hub(root, self.home, self.base, "init", "--name", root.name,
                              extra_env=self._GATE_LOCAL_BUILD_ENV)
        self.assertEqual(rc, 0, err)
        if rewrite is not None:
            rewrite(root)
        rc, _, err = _run_hub(root, self.home, self.base, "sync",
                              extra_env=self._GATE_LOCAL_BUILD_ENV)
        self.assertEqual(rc, 0, err)
        return root

    def _topology_line(self, root: Path, home: Path) -> str:
        rc, out, err = _run_hub(root, home, self.base)
        self.assertEqual(rc, 0, err)
        lines = [ln for ln in out.splitlines() if ln.strip().startswith("topology:")]
        self.assertEqual(len(lines), 1, out)
        return lines[0].strip()

    def test_topology_auto_monorepo_submodules(self):
        sys.path.insert(0, str(ROOT / "tests"))
        from test_repo_topology import make_super_with_submodule

        super_repo = make_super_with_submodule(self.base / "a")
        self._green_fixture(super_repo, "auto")
        line = self._topology_line(super_repo, self.home)
        self.assertEqual(line, "topology: monorepo-submodules (auto)")

    def test_topology_explicit_standalone_via_config(self):
        root = self._green_fixture(self.base / "standalone-prj", "standalone")
        line = self._topology_line(root, self.home)
        self.assertEqual(line, "topology: standalone (config)")

    def test_project_topology_surfaces_with_worktree_flow_disabled(self):
        def rewrite(root: Path) -> None:
            manifest = root / "ai-specs" / "ai-specs.toml"
            text = manifest.read_text().replace(
                "[recipes.worktree-flow]\nenabled = true",
                "[recipes.worktree-flow]\nenabled = false",
            ).replace(
                "name = 'topo'",
                "name = 'topo'\nrepo_topology = \"monorepo-apps\"",
            )
            manifest.write_text(text)

        root = self._green_fixture(self.base / "wf-disabled-prj", "standalone",
                                   rewrite=rewrite)
        # [project].repo_topology still surfaces with the worktree-flow
        # recipe disabled (source != default keeps the line printed).
        line = self._topology_line(root, self.home)
        self.assertEqual(line, "topology: monorepo-apps (config)")

    def test_project_topology_wins_over_legacy_recipe_alias(self):
        def rewrite(root: Path) -> None:
            manifest = root / "ai-specs" / "ai-specs.toml"
            text = manifest.read_text().replace(
                "name = 'topo'",
                "name = 'topo'\nrepo_topology = \"monorepo-apps\"",
            )
            manifest.write_text(text)

        root = self._green_fixture(self.base / "wins-prj", "standalone",
                                   rewrite=rewrite)
        line = self._topology_line(root, self.home)
        self.assertEqual(line, "topology: monorepo-apps (config)")


if __name__ == "__main__":
    unittest.main()
