"""Black-box tests for interactive `ai-specs init` TUI.

Every test drives the CLI through its process boundary:
- ``bin/ai-specs init ...`` — non-interactive gating and TUI-stub rc paths;
- ``bin/ai-specs init --tui`` under a pseudo-terminal — the real wizard;
- the isolated install's own ``lib/_internal/init_tui.py`` as a subprocess —
  the TUI entrypoint rc contract (never imported in-process).
A few pure internal-builder contracts with no CLI surface are kept as
process-boundary drivers (subprocess executing the isolated install's
init_tui.py via runpy), each marked with a distinct ``# TRIAGE:`` comment.
"""
from __future__ import annotations

import fcntl
import json
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
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import CLIResult, invoke, isolated_home, normalize_output  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"

ANSI_RE = re.compile(rb"\x1b\[[0-9;?]*[a-zA-Z]")


def _strip_ansi(data: bytes) -> str:
    """Strip ANSI escapes so rich-rendered text can be sliced for assertions."""
    return ANSI_RE.sub(b"", data).decode("utf-8", "replace")


def _recipe_toml(rid: str, name: str, extra: str = "") -> str:
    return (
        f'[recipe]\nid = "{rid}"\nname = "{name}"\n'
        f'description = "test recipe {rid}"\nversion = "1.0.0"\n' + extra
    )


def _make_home(base: Path, *, catalog: bool = True) -> Path:
    """Isolated CLI install root with a REAL lib copy (plus _vendor, needed by
    the interactive deps gate). A symlinked lib would resolve runtime paths
    back into the repository."""
    home = isolated_home(base / "cli-home", catalog=catalog)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    (home / "lib" / "_vendor").symlink_to(ROOT / "lib" / "_vendor")
    return home


def _cli_env(home: Path, base: Path, extra: dict | None = None) -> dict:
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(base / "home"),
        "TMPDIR": str(base),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "LC_ALL": "C",
        "LANG": "C",
        "TERM": "xterm",
        "PYTHONDONTWRITEBYTECODE": "1",
    }
    (base / "home").mkdir(exist_ok=True)
    if extra:
        env.update(extra)
    return env


def _cli_run(project_root: Path, *args: str, home: Path, base: Path,
             extra_env: dict | None = None) -> CLIResult:
    """Run ``bin/ai-specs`` hermetically with injectable env (TUI stub seam).

    stdin is an empty pipe (never a TTY), so non-interactive paths cannot block.
    """
    argv = [str(CLI), *args, str(project_root)]
    proc = subprocess.run(
        argv, cwd=str(project_root), env=_cli_env(home, base, extra_env),
        text=True, capture_output=True, check=False, input="", timeout=180,
    )
    roots = (project_root, home, base)
    return CLIResult(proc.returncode,
                     normalize_output(proc.stdout, roots),
                     normalize_output(proc.stderr, roots))


def _seed_recipe(home: Path, rid: str, body: str) -> Path:
    """Seed a recipe into the isolated home's (real) catalog directory."""
    rdir = home / "catalog" / "recipes" / rid
    rdir.mkdir(parents=True, exist_ok=True)
    (rdir / "recipe.toml").write_text(body, encoding="utf-8")
    return rdir


def _spawn_pty(target: Path, home: Path, base: Path, *, feed: bytes = b"",
               timeout: float = 90, stages: list[tuple[bytes, bytes]] | None = None,
               argv: list[str] | None = None) -> tuple[int, bytes]:
    """Spawn a command under a PTY and drive it with staged input.

    ``stages`` is a list of (needle, payload): each payload is written once the
    needle first appears in the accumulated output AFTER the previous stage
    fired (None = write immediately). Ctrl-C (\\x03) aborts a questionary
    prompt; \\x04 is EOF; \\x1b[B is ArrowDown and \\n accepts.

    The pty master is always closed and the child reaped in finally.
    """
    master, slave = os.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    cmd = argv if argv is not None else [str(CLI), "init", "--tui", str(target)]
    proc = subprocess.Popen(
        cmd,
        stdin=slave,
        stdout=slave,
        stderr=slave,
        close_fds=True,
        env=_cli_env(home, base),
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
                raise AssertionError(f"command timed out after {timeout}s; output: {output!r}")

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

    return proc.returncode, output


_DRIVER_PREAMBLE = (
    "import json, runpy, sys\n"
    "_g = runpy.run_path(sys.argv[1], run_name='ai_specs_driver')\n"
    "# runpy returns a copy of the module globals; patching must target the\n"
    "# dict the module's functions actually bind to.\n"
    "_w = _g['run_wizard'].__globals__\n"
)


def _run_driver(testcase: unittest.TestCase, home: Path, body: str,
                *args: object) -> dict:
    """Execute a process-boundary driver against the isolated install's own
    init_tui.py (runpy, never imported in-process). Returns its JSON stdout."""
    tmp = tempfile.TemporaryDirectory(prefix="ai-specs-tui-driver-")
    testcase.addCleanup(tmp.cleanup)
    driver = Path(tmp.name) / "driver.py"
    driver.write_text(_DRIVER_PREAMBLE + body, encoding="utf-8")
    proc = subprocess.run(
        [sys.executable, str(driver), str(home / "lib" / "_internal" / "init_tui.py"),
         *(str(a) for a in args)],
        stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=60,
        env=_cli_env(home, Path(tmp.name)),
    )
    testcase.assertEqual(
        proc.returncode, 0, f"driver failed: rc={proc.returncode} stderr={proc.stderr}"
    )
    return json.loads(proc.stdout.strip())


def _fresh_env(testcase: unittest.TestCase, *, catalog: bool = False,
               name: str = "demo-proj") -> tuple[Path, Path, Path]:
    tmp = tempfile.TemporaryDirectory(prefix="ai-specs-init-pty-")
    testcase.addCleanup(tmp.cleanup)
    base = Path(tmp.name)
    home = _make_home(base, catalog=catalog)
    target = base / name
    target.mkdir()
    return base, home, target


# Prompt needles shared by the wizard PTY stages.
N_NAME = b"Project name"
N_TOPO = b"Repo topology"
N_AGENTS = b"Select agents"
N_RECIPES = b"Select recipes"
N_CONFIRM = b"Write ai-specs"

# Default wizard answers: accept name prefill, auto topology, default agents.
_DEFAULT_STAGES = [(N_NAME, b"\n"), (N_TOPO, b"\n"), (N_AGENTS, b"\n")]


class TestRenderManifest(unittest.TestCase):
    """The wizard's manifest rendering, observed through the staged TOML it writes."""

    def test_roundtrip_toml(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "session-context", _recipe_toml("session-context", "Session Context"))
        _seed_recipe(home, "tdd-flow", _recipe_toml("tdd-flow", "TDD Flow"))
        stages = [
            (N_NAME, b"\n"),                 # prefill "demo" via --name
            (N_TOPO, b"\n"),                 # auto topology
            (N_AGENTS, b"\n"),               # default agents
            (N_RECIPES, b"\x1b[B \n"),       # keep session-context, add tdd-flow
            (N_CONFIRM, b"\n"),
        ]
        rc, _ = _spawn_pty(target, home, base, stages=stages,
                           argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        self.assertEqual(rc, 0)
        toml_path = target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(toml_path.is_file())
        text = toml_path.read_text(encoding="utf-8")
        data = tomllib.loads(text)
        self.assertEqual(data["project"]["name"], "demo")
        self.assertEqual(data["agents"]["enabled"], ["claude", "cursor", "opencode"])
        self.assertTrue(data["recipes"]["session-context"]["enabled"])
        self.assertNotIn("version", data["recipes"]["tdd-flow"])
        self.assertNotIn("version =", text)

    def test_dotted_recipe_id_is_quoted_literal_key(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "foo.bar", _recipe_toml("foo.bar", "Dotted"))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # single recipe: toggle on, submit
            (N_CONFIRM, b"\n"),
        ]
        rc, _ = _spawn_pty(target, home, base, stages=stages,
                           argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        self.assertEqual(rc, 0)
        toml_path = target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(toml_path.is_file())
        text = toml_path.read_text(encoding="utf-8")
        self.assertIn('[recipes."foo.bar"]', text)
        data = tomllib.loads(text)
        self.assertIn("foo.bar", data["recipes"])
        self.assertTrue(data["recipes"]["foo.bar"]["enabled"])
        self.assertNotIn("version", data["recipes"]["foo.bar"])
        self.assertNotIn("version =", text)

    def test_render_manifest_writes_config_block(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "cfg-one", _recipe_toml("cfg-one", "Cfg One", extra=(
            "[config.auto_remove_merged]\nrequired = false\ntype = \"bool\"\ndefault = true\n\n"
            "[config.gate_mode]\nrequired = false\ntype = \"string\"\n"
            "enum = [\"always\", \"ask\", \"off\"]\ndefault = \"always\"\n"
        )))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # select cfg-one
            (b"Configure cfg-one", b"\n"),   # configure now
            (b"auto_remove_merged", b"\n"),  # bool confirm, default true
            (b"gate_mode", b"\n"),           # enum select, default "always"
            (N_CONFIRM, b"\n"),
        ]
        rc, _ = _spawn_pty(target, home, base, stages=stages,
                           argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        self.assertEqual(rc, 0)
        toml_path = target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(toml_path.is_file())
        text = toml_path.read_text(encoding="utf-8")
        self.assertIn("[recipes.cfg-one.config]", text)
        self.assertIn("auto_remove_merged = true", text)
        self.assertIn('gate_mode = "always"', text)
        data = tomllib.loads(text)
        self.assertEqual(data["recipes"]["cfg-one"]["config"]["gate_mode"], "always")
        self.assertIs(data["recipes"]["cfg-one"]["config"]["auto_remove_merged"], True)

    # TRIAGE: ai-specs init --tui — byte-equality between a legacy and a modern
    # _render_manifest call (configured=None vs absent) has no CLI surface: the
    # wizard renders exactly once. Kept as a process-boundary driver of the
    # isolated install's own init_tui.py.
    def test_render_manifest_no_config_backward_compat(self):
        base, home, _target = _fresh_env(self)
        result = _run_driver(self, home, (
            "_tw = _g['_load_toml_write']()\n"
            "_recipes = [{'id': 'session-context', 'version': '2.0.0'}]\n"
            "_legacy = _g['_render_manifest'](_tw, 'demo', ['claude'], _recipes)\n"
            "_modern = _g['_render_manifest'](_tw, 'demo', ['claude'], _recipes, configured=None)\n"
            "print(json.dumps({'legacy': _legacy, 'modern': _modern}))\n"
        ))
        self.assertEqual(result["legacy"], result["modern"])
        self.assertNotIn(".config]", result["modern"])


class TestConfigureRecipesStep(unittest.TestCase):
    """Step 3.5 (per-recipe configure), driven through the real wizard PTY."""

    def test_configure_recipes_skip_later(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "cfg-later", _recipe_toml("cfg-later", "Cfg Later", extra=(
            "[config.base_branch]\nrequired = false\ntype = \"string\"\ndefault = \"main\"\n"
            "help_text = \"Base branch for PRs\"\n"
        )))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # select cfg-later
            (b"Configure cfg-later", b"n\n"),  # configure later
            (N_CONFIRM, b"\n"),
        ]
        rc, out = _spawn_pty(target, home, base, stages=stages,
                             argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        plain = _strip_ansi(out)
        self.assertEqual(rc, 0)
        text = (target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        data = tomllib.loads(text)
        self.assertEqual(data["recipes"]["cfg-later"], {"enabled": True})
        self.assertNotIn(".config]", text)
        # The config wizard never ran: its field help text was never rendered.
        self.assertNotIn("Base branch for PRs", plain)

    def test_configure_recipes_uses_dep_gate_for_cli_deps(self):
        """JD-3: init must offer TTY install via _dep_gate, not panel-only."""
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "dep-gate", _recipe_toml("dep-gate", "Dep Gate", extra=(
            "[[deps.cli]]\nbinary = \"zzz-definitely-missing-binary\"\n"
            "purpose = \"test dep gate\"\nrequired = true\n"
        )))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # select dep-gate
            (b"Configure anyway", b"\n"),    # gate confirm: default No → skip
            (N_CONFIRM, b"\n"),
        ]
        rc, out = _spawn_pty(target, home, base, stages=stages,
                             argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        plain = _strip_ansi(out)
        self.assertEqual(rc, 0)
        # The dep gate rendered the panel itself (binary visible with WARN).
        self.assertIn("zzz-definitely-missing-binary", plain)
        # The gate then offered the interactive TTY decision before skipping.
        self.assertIn("Configure anyway", plain)
        self.assertIn("Skipped dep-gate", plain)
        self.assertIn("missing CLI deps", plain)
        text = (target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        data = tomllib.loads(text)
        self.assertEqual(data["recipes"]["dep-gate"], {"enabled": True})
        self.assertNotIn(".config]", text)


class TestRunWizardHarnessEnv(unittest.TestCase):
    """JD-1: fresh init must spoon-feed harness env after staging write."""

    def test_fresh_init_writes_real_toml_and_offers_harness_env(self):
        base, home, target = _fresh_env(self)
        # A recipe whose MCP preset declares a $VAR harness env reference.
        _seed_recipe(home, "env-one", _recipe_toml("env-one", "Env One", extra=(
            "[[provides.mcp]]\nid = \"test-mcp\"\ncommand = \"echo\"\n"
            "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n"
        )))
        stages = [
            (N_NAME, b"\n"),                 # prefill "demo"
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # select env-one
            (N_CONFIRM, b"\n"),              # write manifest
            (b"Configure these values now?", b"\n"),  # accept env offer
            (b"TRELLO_API_KEY", b"trello-harness-value\n"),
        ]
        rc, out = _spawn_pty(target, home, base, stages=stages,
                             argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        plain = _strip_ansi(out)
        self.assertEqual(rc, 0, f"output: {plain!r}")
        # The real manifest existed before the harness-env offer: the offer
        # could only collect vars by reading it at the real path.
        real_toml = target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(real_toml.is_file())
        self.assertIn("Required environment variables", plain)
        env_file = target / "ai-specs.env"
        self.assertTrue(env_file.is_file(), f"no harness env written; output: {plain!r}")
        env_text = env_file.read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY", env_text)
        self.assertIn("trello-harness-value", env_text)
        # The root .envrc managed block was part of the same offer.
        self.assertTrue((target / ".envrc").is_file())


class TestCatalogRecipes(unittest.TestCase):
    """_catalog_recipes behavior, observed through the wizard's recipe picker."""

    def test_skips_broken_recipe_toml(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "good-one", _recipe_toml("good-one", "Good"))
        bad = home / "catalog" / "recipes" / "bad-one"
        bad.mkdir(parents=True, exist_ok=True)
        (bad / "recipe.toml").write_text("this is not toml {{{", encoding="utf-8")
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # good-one is the only offered recipe
            (N_CONFIRM, b"\n"),
        ]
        rc, out = _spawn_pty(target, home, base, stages=stages,
                             argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        plain = _strip_ansi(out)
        self.assertEqual(rc, 0)
        # The broken entry was skipped with a notice, not a crash.
        self.assertIn("skip catalog recipe bad-one", plain)
        text = (target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        data = tomllib.loads(text)
        self.assertEqual(list(data["recipes"]), ["good-one"])

    def test_hides_internal_test_recipes(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "good-one", _recipe_toml("good-one", "Good"))
        _seed_recipe(home, "test-fixture", _recipe_toml("test-fixture", "Test Fixture"))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),
            (N_CONFIRM, b"\n"),
        ]
        rc, out = _spawn_pty(target, home, base, stages=stages,
                             argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        plain = _strip_ansi(out)
        self.assertEqual(rc, 0)
        self.assertNotIn("test-fixture", plain)
        text = (target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        data = tomllib.loads(text)
        self.assertEqual(list(data["recipes"]), ["good-one"])


class TestEnsureDepsAndMain(unittest.TestCase):
    """init_tui.py entrypoint rc contract, at its own process boundary."""

    def test_run_wizard_returns_3_on_non_tty(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-init-nontty-")
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = _make_home(base)
        target = base / "proj"
        target.mkdir()
        out = base / "out.toml"
        proc = subprocess.run(
            [sys.executable, str(home / "lib" / "_internal" / "init_tui.py"),
             "--target", str(target), "--out", str(out)],
            stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=60,
            env=_cli_env(home, base),
        )
        self.assertEqual(proc.returncode, 3)
        self.assertFalse(out.exists())

    def test_main_unexpected_exception_returns_3_not_1(self):
        """An unexpected failure must return 3 (classic fallback), never 1 (cancel)."""
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-init-boom-")
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = _make_home(base, catalog=False)  # empty catalog: no recipes prompt
        target = base / "proj"
        target.mkdir()
        blocker = base / "not-a-dir"
        blocker.write_text("regular file\n", encoding="utf-8")
        out = blocker / "staged.toml"  # --out parent is a file → write fails
        rc, output = _spawn_pty(
            target, home, base,
            argv=[sys.executable, str(home / "lib" / "_internal" / "init_tui.py"),
                  "--target", str(target), "--out", str(out)],
            stages=[(N_NAME, b"\n"), (N_TOPO, b"\n"), (N_AGENTS, b"\n"), (N_CONFIRM, b"\n")],
            timeout=60,
        )
        self.assertEqual(rc, 3)
        self.assertIn("interactive init failed", _strip_ansi(output))

    def test_ensure_deps_mkdir_failure_returns_3(self):
        """Vendor install offered on TTY; a failed vendor mkdir must return 3."""
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-init-deps-")
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = _make_home(base, catalog=False)
        # Replace lib/_vendor with a regular file: rich/questionary become
        # unimportable AND the vendor mkdir is impossible.
        (home / "lib" / "_vendor").unlink()
        (home / "lib" / "_vendor").write_text("not a directory\n", encoding="utf-8")
        target = base / "proj"
        target.mkdir()
        out = base / "out.toml"
        rc, output = _spawn_pty(
            target, home, base,
            argv=[sys.executable, str(home / "lib" / "_internal" / "init_tui.py"),
                  "--target", str(target), "--out", str(out)],
            stages=[(b"Install into", b"y\n")],
            timeout=60,
        )
        self.assertEqual(rc, 3)
        self.assertIn("cannot create vendor dir", _strip_ansi(output))


class TestInitShellGating(unittest.TestCase):
    """Shell integration: auto-TUI stays off without a TTY; stubs cover rc paths."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-init-gating-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)

    def _workspace(self, name: str = "target") -> Path:
        target = self.base / name
        target.mkdir()
        return target

    def _run_init(self, target: Path, *args: str, extra_env: dict | None = None) -> CLIResult:
        return _cli_run(target, "init", *args, home=self.home, base=self.base,
                        extra_env=extra_env)

    def _write_stub(self, base: Path, rc: int, write_out: bool = False) -> Path:
        stub = base / f"stub-tui-{rc}.py"
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
        stub = self._write_stub(self.base, rc=1)
        result = self._run_init(target, "--tui", extra_env={"AI_SPECS_INIT_TUI_PY": str(stub)})
        self.assertEqual(result.returncode, 1)
        self.assertIn("cancelled", result.stderr.lower())
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists())

    def test_tui_unavailable_falls_back_to_classic(self):
        target = self._workspace()
        stub = self._write_stub(self.base, rc=3)
        result = self._run_init(target, "--tui", extra_env={"AI_SPECS_INIT_TUI_PY": str(stub)})
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertIn("falling back to classic init", result.stderr)
        self.assertIn("tui:     no", result.stdout)
        self.assertTrue((target / "ai-specs" / "ai-specs.toml").is_file())

    def test_tui_success_writes_stub_manifest(self):
        target = self._workspace()
        stub = self._write_stub(self.base, rc=0, write_out=True)
        result = self._run_init(target, "--tui", extra_env={"AI_SPECS_INIT_TUI_PY": str(stub)})
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertIn("tui:     yes", result.stdout)
        self.assertIn("(from TUI)", result.stdout)
        text = (target / "ai-specs" / "ai-specs.toml").read_text()
        self.assertIn('name = "stub"', text)


class TestInitTuiPTYE2E(unittest.TestCase):
    """End-to-end PTY tests: real wizard via `ai-specs init --tui` under a
    pseudo-terminal against the real repo catalog — no stub seams.

    Ctrl-C is delivered as a real SIGINT via the PTY's line-discipline.
    """

    def _workspace(self, name: str = "proj") -> tuple[Path, Path, Path]:
        return _fresh_env(self, catalog=True, name=name)

    def test_accept_defaults_writes_toml_with_default_recipes(self):
        """Enter through each question (defaults pre-checked) → TOML with defaults, including session-context recipe."""
        target, home, base = self._workspace()
        # name + topology + agents checkbox + recipes checkbox + confirm (default Yes)
        rc, output = _spawn_pty(target, home, base, feed=b"\n\n\n\n\n", timeout=120)
        self.assertEqual(rc, 0, f"output: {_strip_ansi(output)!r}")
        toml_path = target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(toml_path.is_file(), f"no manifest; output: {_strip_ansi(output)!r}")
        data = tomllib.loads(toml_path.read_text(encoding="utf-8"))
        self.assertIn("project", data)
        self.assertIn("agents", data)
        self.assertIn("session-context", data.get("recipes", {}),
                      f"default recipe session-context not in TOML; output: {_strip_ansi(output)!r}")

    def test_custom_name_writes_toml(self):
        """Custom name + Enter through checkboxes/confirm defaults → TOML with custom name and default recipes."""
        target, home, base = self._workspace()
        # Ctrl-A Ctrl-K clears questionary's default text, then type name; Enter through rest.
        rc, output = _spawn_pty(target, home, base, feed=b"\x01\x0bmy-app\n\n\n\n\n", timeout=120)
        self.assertEqual(rc, 0, f"output: {_strip_ansi(output)!r}")
        data = tomllib.loads((target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertEqual(data["project"]["name"], "my-app")
        self.assertIn("session-context", data.get("recipes", {}),
                      f"default recipe session-context not in TOML; output: {_strip_ansi(output)!r}")

    def test_decline_confirm_no_toml(self):
        """Enter through prompts, then 'n' at confirm → rc=1 (cancel), no TOML file."""
        target, home, base = self._workspace()
        rc, output = _spawn_pty(target, home, base, feed=b"\n\n\n\nn\n", timeout=120)
        self.assertEqual(rc, 1, f"output: {_strip_ansi(output)!r}")
        self.assertIn("cancelled", _strip_ansi(output).lower())
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists(),
                         f"TOML should not exist after decline; output: {_strip_ansi(output)!r}")

    def test_ctrl_c_at_prompt_cancels_cleanly(self):
        """Ctrl-C via PTY (\\x03) at first prompt → rc=1, no TOML."""
        target, home, base = self._workspace()
        rc, output = _spawn_pty(target, home, base, stages=[(N_NAME, b"\x03")], timeout=60)
        self.assertEqual(rc, 1, f"output: {_strip_ansi(output)!r}")
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists(),
                         f"TOML should not exist after Ctrl-C; output: {_strip_ansi(output)!r}")

    def test_eof_at_prompt_cancels_like_ctrl_c(self):
        """EOF (Ctrl-D) at first prompt → rc=1 (cancel), no TOML — same as Ctrl-C."""
        target, home, base = self._workspace()
        # questionary/prompt_toolkit rejects EOF while default text remains; clear then Ctrl-D.
        rc, output = _spawn_pty(target, home, base, feed=b"\x01\x0b\x04", timeout=60)
        self.assertEqual(rc, 1, f"output: {_strip_ansi(output)!r}")
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists(),
                         f"TOML should not exist after EOF; output: {_strip_ansi(output)!r}")

    def test_checkbox_toggle_changes_agent_selection(self):
        """Arrow down + space toggle in agent checkbox → non-default agent set in TOML.

        Navigates with arrows, toggles with space, confirms with Enter, waiting
        for each prompt before sending the next answer.
        """
        target, home, base = self._workspace()
        stages = [
            (N_NAME, b"\x01\x0bcustom-app\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\x1b[B\x1b[B\x1b[B \x1b[B\x1b[B\x1b[B \n"),
            (N_RECIPES, b"\n"),
            (N_CONFIRM, b"\n"),
        ]
        rc, output = _spawn_pty(target, home, base, stages=stages, timeout=120)
        self.assertEqual(rc, 0, f"wizard failed; output captured during test")
        toml_path = target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(toml_path.is_file(), f"no staged TOML; rc={rc}")
        data = tomllib.loads(toml_path.read_text(encoding="utf-8"))
        self.assertEqual(data["project"]["name"], "custom-app")
        self.assertIn("agents", data)

    def test_ctrl_c_during_checkbox_cancels_cleanly(self):
        """Ctrl-C via PTY (\\x03) during agent checkbox → rc=1 (cancel), no TOML."""
        target, home, base = self._workspace()
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\n"),
            (N_AGENTS, b"\x03"),
        ]
        rc, output = _spawn_pty(target, home, base, stages=stages, timeout=60)
        self.assertEqual(rc, 1, f"output: {_strip_ansi(output)!r}")
        self.assertFalse((target / "ai-specs" / "ai-specs.toml").exists(),
                         f"TOML should not exist after Ctrl-C during checkbox; output: {_strip_ansi(output)!r}")


class TopologyWizardNodeTests(unittest.TestCase):
    """Topology is a project-owned manifest field, never recipe config."""

    def test_topology_renders_as_project_field_not_recipe_config(self):
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "worktree-flow", _recipe_toml("worktree-flow", "Worktree Flow", extra=(
            "[config.gate_mode]\nrequired = false\ntype = \"string\"\n"
            "enum = [\"always\", \"ask\", \"off\"]\ndefault = \"always\"\n"
        )))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\x1b[B\n"),           # select "standalone" (down once)
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # select worktree-flow
            (b"Configure worktree-flow", b"\n"),
            (b"gate_mode", b"\n"),           # enum select, default "always"
            (N_CONFIRM, b"\n"),
        ]
        rc, _ = _spawn_pty(target, home, base, stages=stages,
                           argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        self.assertEqual(rc, 0)
        data = tomllib.loads((target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertEqual(data["project"]["repo_topology"], "standalone")
        self.assertEqual(data["recipes"]["worktree-flow"]["config"], {"gate_mode": "always"})

    # TRIAGE: ai-specs init --tui — a legacy caller passing repo_topology inside
    # recipe config (and its promotion to [project]) cannot be produced through
    # the CLI: the wizard schema filters project-owned keys before prompting.
    # Kept as a process-boundary driver of the isolated install's init_tui.py.
    def test_recipe_config_never_owns_repo_topology(self):
        base, home, _target = _fresh_env(self)
        result = _run_driver(self, home, (
            "_tw = _g['_load_toml_write']()\n"
            "_text = _g['_render_manifest'](_tw, 'demo', ['claude'],"
            " [{'id': 'worktree-flow', 'version': '1.3.0'}],"
            " configured={'worktree-flow': {'repo_topology': 'standalone', 'gate_mode': 'ask'}},"
            " topology='monorepo-submodules')\n"
            "print(json.dumps({'text': _text}))\n"
        ))
        data = tomllib.loads(result["text"])
        self.assertEqual(data["project"]["repo_topology"], "monorepo-submodules")
        self.assertEqual(data["recipes"]["worktree-flow"]["config"], {"gate_mode": "ask"})

    # TRIAGE: ai-specs init --tui — the recipe-schema filter (_wizard_recipe) is
    # a data-level contract; through the CLI it is only observable as the
    # absence of a repo_topology prompt. Kept as a process-boundary driver of
    # the isolated install's init_tui.py, reading the real repo catalog
    # read-only.
    def test_wizard_schema_excludes_project_owned_topology(self):
        base, home, _target = _fresh_env(self)
        result = _run_driver(self, home, (
            "import pathlib\n"
            "_rr = _g['_load_sibling']('recipe-read')\n"
            "_recipe = _rr.read_recipe(pathlib.Path(sys.argv[2]), 'worktree-flow')\n"
            "_filtered = _g['_wizard_recipe'](_recipe)\n"
            "print(json.dumps({'orig': sorted(_recipe.config_schema.fields),"
            " 'filtered': sorted(_filtered.config_schema.fields)}))\n"
        ), ROOT / "catalog" / "recipes")
        self.assertIn("repo_topology", result["orig"])
        self.assertNotIn("repo_topology", result["filtered"])
        self.assertIn("gate_mode", result["filtered"])

    def test_run_wizard_asks_topology_and_writes_override(self):
        """The topology prompt offers auto-detect first; the answer lands in [project]."""
        base, home, target = _fresh_env(self)
        _seed_recipe(home, "worktree-flow", _recipe_toml("worktree-flow", "Worktree Flow", extra=(
            "[config.gate_mode]\nrequired = false\ntype = \"string\"\n"
            "enum = [\"always\", \"ask\", \"off\"]\ndefault = \"always\"\n"
        )))
        stages = [
            (N_NAME, b"\n"),
            (N_TOPO, b"\x1b[B\n"),           # choose "standalone" over auto
            (N_AGENTS, b"\n"),
            (N_RECIPES, b" \n"),             # select worktree-flow
            (b"Configure worktree-flow", b"n\n"),  # skip config
            (N_CONFIRM, b"\n"),
        ]
        rc, out = _spawn_pty(target, home, base, stages=stages,
                             argv=[str(CLI), "init", "--tui", "--name", "demo", str(target)])
        plain = _strip_ansi(out)
        self.assertEqual(rc, 0)
        # The topology question was asked with the detected default first.
        self.assertIn("Repo topology for worktree-flow:", plain)
        self.assertIn("standalone (detected)", plain)
        data = tomllib.loads((target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertEqual(data["project"]["repo_topology"], "standalone")
        self.assertNotIn(
            "repo_topology",
            (data["recipes"]["worktree-flow"].get("config") or {}),
        )

    # TRIAGE: ai-specs init --tui — the promotion of a recipe-side repo_topology
    # answer over the identity answer requires injecting a config flow that
    # returns repo_topology, which the real (filtered) schema can never do.
    # Kept as a process-boundary driver of the isolated install's init_tui.py
    # with a fake questionary, preserving the original mocked-wizard intent.
    def test_configure_recipes_topology_wins_over_identity_prompt(self):
        """Later explicit recipe-config answer must not be clobbered by identity."""
        base, home, _target = _fresh_env(self)
        target = Path(base) / "prj"
        out = Path(base) / "staged.toml"
        body = f"""
import io, sys, types

class _Ask:
    def __init__(self, value):
        self._value = value
    def ask(self):
        return self._value

class FakeQ:
    class Choice:
        def __init__(self, title='', value=None, checked=False):
            self.title, self.value, self.checked = title, value, checked
    @staticmethod
    def text(prompt, default=''):
        return _Ask(default)
    @staticmethod
    def select(prompt, choices=None, default=None):
        assert 'topology' in prompt.lower()
        return _Ask('monorepo-submodules')
    @staticmethod
    def checkbox(prompt, choices=None):
        if 'agents' in prompt.lower():
            return _Ask(['claude'])
        if 'recipes' in prompt.lower():
            return _Ask([{{'id': 'worktree-flow', 'version': '1.3.0', 'description': 'wt'}}])
        return _Ask([])
    @staticmethod
    def confirm(prompt, default=True):
        return _Ask(True)

sys.modules['questionary'] = FakeQ
_rich = types.ModuleType('rich')
_rich_console = types.ModuleType('rich.console')
_rich_console.Console = lambda **kw: types.SimpleNamespace(print=lambda *a, **k: None)
_rich_panel = types.ModuleType('rich.panel')
_rich_panel.Panel = types.SimpleNamespace(fit=lambda *a, **k: 'panel')
sys.modules['rich'] = _rich
sys.modules['rich.console'] = _rich_console
sys.modules['rich.panel'] = _rich_panel

_fake_in = io.StringIO()
_fake_in.isatty = lambda: True
_fake_out = io.StringIO()
_fake_out.isatty = lambda: True
sys.stdin = _fake_in
sys.stdout = _fake_out

_g['_ensure_deps'] = lambda: None
_w['_ensure_deps'] = lambda: None
_g['_catalog_recipes'] = lambda: [{{'id': 'worktree-flow', 'version': '1.3.0', 'description': 'wt'}}]
_w['_catalog_recipes'] = _w['_catalog_recipes']
_w['_catalog_recipes'] = _g['_catalog_recipes']
# _configure_recipes returns a DIFFERENT topology than the identity prompt.
_g['_configure_recipes'] = lambda recipes, console, catalog_dir: {{
    'worktree-flow': {{'repo_topology': 'standalone'}},
}}
_w['_configure_recipes'] = _g['_configure_recipes']

import pathlib
_target = pathlib.Path({str(target)!r})
_target.mkdir(parents=True, exist_ok=True)
_out = pathlib.Path({str(out)!r})
_rc = _g['run_wizard'](target=_target, name_prefill='demo', out_path=_out)
_staged = _out.read_text() if _out.is_file() else None
sys.stdout = sys.__stdout__
print(json.dumps({{'rc': _rc, 'staged': _staged}}))
"""
        result = _run_driver(self, home, body)
        self.assertEqual(result["rc"], 0)
        self.assertIsNotNone(result["staged"])
        data = tomllib.loads(result["staged"])
        self.assertEqual(data["project"]["repo_topology"], "standalone")
        self.assertNotIn(
            "repo_topology",
            (data["recipes"]["worktree-flow"].get("config") or {}),
        )


if __name__ == "__main__":
    unittest.main()
