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
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import recipe_skill_dir, recipe_root, cache_command, resolved_skills_dir
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



class TestImportlibModuleScope(unittest.TestCase):
    """A.1 — importlib must be available at hub module scope (Agents path)."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_hub_has_importlib_at_module_scope(self):
        self.assertTrue(hasattr(self.mod, "importlib"), "hub must import importlib at module scope")
        self.assertTrue(hasattr(self.mod.importlib, "util"))


class TestReadVersion(unittest.TestCase):
    """A.2 — pure _read_version()."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_read_version_from_ai_specs_home(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            (home / "VERSION").write_text("9.9.9-test\n", encoding="utf-8")
            with mock.patch.object(self.mod._util, "ai_specs_home", return_value=home):
                self.assertEqual(self.mod._read_version(), "9.9.9-test")

    def test_read_version_missing_file_is_unknown(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            with mock.patch.object(self.mod._util, "ai_specs_home", return_value=home):
                self.assertEqual(self.mod._read_version(), "unknown")


class TestStatusSummaryVersion(unittest.TestCase):
    """A.2 — StatusSummary.version populated."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_status_summary_includes_version(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            summary = self.mod.status_summary(root)
            self.assertEqual(summary.version, self.mod._read_version())


class TestNonInteractiveShowsVersion(unittest.TestCase):
    """A.2 — non-interactive status prints version."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_run_noninteractive_prints_version(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            version = self.mod._read_version()
            with mock.patch("sys.stdout", new_callable=lambda: __import__("io").StringIO()) as buf:
                rc = self.mod._run_noninteractive(root)
            self.assertEqual(rc, 0)
            out = buf.getvalue()
            self.assertIn(version, out)
            self.assertIn("version:", out)


class TestRecipeChoiceBuilders(unittest.TestCase):
    """A.3 — pure recipe_add_choices / recipe_remove_choices."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_add_keeps_available_only(self):
        recipes = [
            {"id": "a", "name": "Alpha", "version": "1.0.0", "status": "available"},
            {"id": "b", "name": "Beta", "version": "2.0.0", "status": "installed"},
            {"id": "c", "name": "", "version": "", "status": "error (boom)"},
        ]
        choices = self.mod.recipe_add_choices(recipes)
        self.assertEqual(len(choices), 1)
        label, value = choices[0]
        self.assertEqual(value, "a")
        self.assertIn("Alpha", label)
        self.assertIn("(a)", label)
        self.assertIn("v1.0.0", label)

    def test_remove_keeps_installed_and_disabled(self):
        recipes = [
            {"id": "a", "name": "Alpha", "version": "1.0.0", "status": "available"},
            {"id": "b", "name": "Beta", "version": "2.0.0", "status": "installed"},
            {"id": "c", "name": "Gamma", "version": "3.0.0", "status": "disabled"},
        ]
        choices = self.mod.recipe_remove_choices(recipes)
        self.assertEqual([v for _, v in choices], ["b", "c"])
        self.assertIn("[installed]", choices[0][0])
        self.assertIn("[disabled]", choices[1][0])

    def test_empty_and_all_error_return_empty(self):
        self.assertEqual(self.mod.recipe_add_choices([]), [])
        self.assertEqual(self.mod.recipe_remove_choices([]), [])
        self.assertEqual(
            self.mod.recipe_add_choices([{"id": "x", "name": "", "version": "", "status": "error (x)"}]),
            [],
        )

    def test_mixed_status_partition_preserves_ids(self):
        recipes = [
            {"id": "err-one", "name": "", "version": "", "status": "error (bad toml)"},
            {"id": "avail", "name": "Avail", "version": "0.1.0", "status": "available"},
            {"id": "inst", "name": "Inst", "version": "0.2.0", "status": "installed"},
        ]
        add = self.mod.recipe_add_choices(recipes)
        rem = self.mod.recipe_remove_choices(recipes)
        self.assertEqual([v for _, v in add], ["avail"])
        self.assertEqual([v for _, v in rem], ["inst"])


class TestCategorizeSkills(unittest.TestCase):
    """A.4 — categorize_skills partitioning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def _skill(self, path: Path, name: str, desc: str = "") -> None:
        path.mkdir(parents=True, exist_ok=True)
        body = "---\nname: {n}\n".format(n=name)
        if desc:
            body += 'description: {d}\n'.format(d=desc)
        body += "---\n# {n}\n".format(n=name)
        (path / "SKILL.md").write_text(body, encoding="utf-8")

    def test_partition_bundled_local_recipe_dep(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp) / "home"
            project = Path(tmp) / "prj"
            (home / "bundled-skills" / "skill-creator").mkdir(parents=True)
            (home / "bundled-skills" / "skill-sync").mkdir(parents=True)
            self._skill(project / "ai-specs" / "skills" / "skill-creator", "skill-creator", "bundled")
            self._skill(project / "ai-specs" / "skills" / "my-local", "my-local", "local only")
            self._skill(
                recipe_root(project, "demo", cli_home=home) / "skills" / "recipe-skill",
                "recipe-skill",
                "from recipe",
            )
            from _cache_paths import deps_skill_dir

            self._skill(
                deps_skill_dir(project, "dep1", "dep-skill", cli_home=home),
                "dep-skill",
                "from dep",
            )
            buckets = self.mod.categorize_skills(project, home)
            self.assertEqual([e["id"] for e in buckets["bundled"]], ["skill-creator"])
            self.assertEqual([e["id"] for e in buckets["local"]], ["my-local"])
            self.assertEqual([e["id"] for e in buckets["recipe"]], ["recipe-skill"])
            self.assertEqual([e["id"] for e in buckets["dep"]], ["dep-skill"])
            self.assertNotIn("skill-creator", [e["id"] for e in buckets["local"]])


class TestSkillsListBundledSection(unittest.TestCase):
    """A.5 — skills-list.sh Bundled skills section."""

    def test_bundled_skills_not_under_local(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp) / "home"
            project = Path(tmp) / "prj"
            # Minimal AI_SPECS_HOME with bundled-skills + empty catalog
            for name in ("skill-creator", "skill-sync"):
                (home / "bundled-skills" / name).mkdir(parents=True)
            (home / "catalog" / "skills").mkdir(parents=True)
            # Copy skills-list.sh dependencies: script uses AI_SPECS_HOME
            ai = project / "ai-specs"
            (ai / "skills").mkdir(parents=True)
            (ai / "ai-specs.toml").write_text('[project]\nname = "t"\n', encoding="utf-8")
            for name in ("skill-creator", "skill-sync", "my-local"):
                d = ai / "skills" / name
                d.mkdir()
                (d / "SKILL.md").write_text(
                    f"---\nname: {name}\ndescription: desc-{name}\n---\n# {name}\n",
                    encoding="utf-8",
                )
            env = {**os.environ, "AI_SPECS_HOME": str(home)}
            # Point script at real repo script but override home
            script = ROOT / "lib" / "skills-list.sh"
            proc = subprocess.run(
                ["bash", str(script), str(project)],
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            out = proc.stdout
            self.assertIn("Bundled skills", out)
            bundled = out.split("Bundled skills")[1].split("Local skills")[0]
            self.assertIn("skill-creator", bundled)
            self.assertIn("skill-sync", bundled)
            local = out.split("Local skills")[1].split("Available catalog")[0]
            self.assertNotIn("skill-creator", local)
            self.assertNotIn("skill-sync", local)
            self.assertIn("my-local", local)


class TestWidgetHelpers(unittest.TestCase):
    """B.1 — pick_one / pick_many / confirm_action / pause contracts."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_pick_one_empty_returns_none_without_questionary(self):
        with mock.patch.dict(sys.modules, {"questionary": mock.Mock()}):
            q = sys.modules["questionary"]
            self.assertIsNone(self.mod.pick_one("m", []))
            q.select.assert_not_called()

    def test_pick_many_empty_returns_none(self):
        self.assertIsNone(self.mod.pick_many("m", []))

    def test_confirm_action_returns_bool(self):
        fake = mock.Mock()
        fake.ask.return_value = True
        with mock.patch.dict("sys.modules", {"questionary": mock.Mock(confirm=mock.Mock(return_value=fake))}):
            # Reload path: patch questionary where imported inside function
            import types
            qmod = types.ModuleType("questionary")
            qmod.confirm = mock.Mock(return_value=fake)
            with mock.patch.dict(sys.modules, {"questionary": qmod}):
                got = self.mod.confirm_action("ok?")
            self.assertIs(got, True)

    def test_pause_true_and_eof_false(self):
        with mock.patch("builtins.input", return_value=""):
            self.assertTrue(self.mod.pause())
        with mock.patch("builtins.input", side_effect=EOFError):
            self.assertFalse(self.mod.pause())




class TestTopologySurfacing(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def _write_wf_manifest(self, root: Path, topology: str = "auto") -> None:
        ai = root / "ai-specs"
        ai.mkdir(parents=True, exist_ok=True)
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'topo'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n\n"
            f"[recipes.worktree-flow.config]\nrepo_topology = \"{topology}\"\n"
        )
        (root / "AGENTS.md").write_text("# brief\n")

    def test_topology_auto_monorepo_submodules(self):
        import sys
        sys.path.insert(0, str(ROOT / "tests"))
        from test_repo_topology import make_super_with_submodule
        with tempfile.TemporaryDirectory() as tmp:
            super_repo = make_super_with_submodule(Path(tmp) / "a")
            self._write_wf_manifest(super_repo, "auto")
            summary = self.mod.status_summary(super_repo)
            self.assertEqual(summary.topology, "monorepo-submodules")
            self.assertEqual(summary.topology_via, "auto")
            import io
            from unittest import mock
            buf = io.StringIO()
            with mock.patch("sys.stdout", buf):
                self.mod._run_noninteractive(super_repo)
            self.assertIn("topology: monorepo-submodules (auto)", buf.getvalue())

    def test_topology_explicit_standalone_via_config(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            self._write_wf_manifest(root, "standalone")
            summary = self.mod.status_summary(root)
            self.assertEqual(summary.topology, "standalone")
            self.assertEqual(summary.topology_via, "config")

if __name__ == "__main__":
    unittest.main()
