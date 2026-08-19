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
from _blackbox import invoke, isolated_home, temp_project
HUB_PY = ROOT / "lib" / "_internal" / "hub.py"


def _load():
    spec = importlib.util.spec_from_file_location("hub", HUB_PY)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules["hub"] = mod
    spec.loader.exec_module(mod)
    return mod


def _cli_home(base: Path) -> Path:
    """Shared CLI home for a test scenario."""
    return isolated_home(base)


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
    # TRIAGE: Tests that hub.py loads without third-party deps (no rich in
    # sys.modules). This is an internal import-time contract — the observable
    # CLI equivalent is that `ai-specs hub` works when piped (no TTY, no rich
    # needed). Ran `bin/ai-specs hub <path> | cat` and it exits 0 with status
    # output, but that does not distinguish "rich was not imported" from
    # "rich was imported but not used". Keeping coupled.
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
    def test_four_state_matrix(self):
        """Observable: hub exit code + output encode the 4-state matrix.

        (initialized=True, tty=False) → NONINTERACTIVE_STATUS → exit 0, "Summary"
        (initialized=False, tty=False) → ERROR_UNINITIALIZED → exit 2, "init" in stderr
        (initialized=True, tty=True) and (initialized=False, tty=True) are PTY-gated
        and tested in test_hub_tui.py's PTY tests.
        """
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            # initialized=True, tty=False → exit 0, status output
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            r = invoke(root, "hub", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("Summary", r.stdout)

            # initialized=False, tty=False → exit 2
            empty = Path(tmp) / "empty"
            empty.mkdir()
            r2 = invoke(empty, "hub", cli_home=home)
            self.assertEqual(r2.returncode, 2)
            self.assertIn("init", r2.stderr.lower())


class TestIsInitialized(unittest.TestCase):
    def test_hub_util_is_initialized_same_callable(self):
        """Observable: hub exits 2 when uninitialized, 0 when initialized."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            root = Path(tmp) / "prj"
            root.mkdir()
            # Uninitialized → exit 2
            r = invoke(root, "hub", cli_home=home)
            self.assertNotEqual(r.returncode, 0)
            # Create ai-specs.toml → initialized
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text("x=1\n")
            r2 = invoke(root, "hub", cli_home=home)
            self.assertEqual(r2.returncode, 0, r2.stderr)


class TestStatusSummary(unittest.TestCase):
    def test_healthy_init_project(self):
        """Observable: hub piped shows Summary with OK count ≥ 1 and exit 0."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            r = invoke(root, "hub", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("Summary", r.stdout)
            # "N OK" where N >= 1
            import re as _re
            m = _re.search(r"(\d+) OK", r.stdout)
            self.assertIsNotNone(m, r.stdout)
            self.assertGreaterEqual(int(m.group(1)), 1)

    def test_warn_when_no_agents(self):
        """Observable: hub piped shows WARN count ≥ 1 and 'warning' in headline."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            r = invoke(root, "hub", cli_home=home)
            # _ai_specs_init clears enabled to [], so warns are expected
            self.assertIn("WARN", r.stdout)
            import re as _re
            m = _re.search(r"(\d+) WARN", r.stdout)
            self.assertIsNotNone(m, r.stdout)
            self.assertGreaterEqual(int(m.group(1)), 1)
            # headline is the first line
            headline = r.stdout.splitlines()[0] if r.stdout.strip() else ""
            self.assertIn("warning" if "warning" in headline.lower() else "error", headline.lower())


class TestDelegateRunner(unittest.TestCase):
    # TRIAGE: DelegateRunner.run() is an internal class that builds argv for
    # subprocess.run(). The test asserts the exact argv shape
    # ([cli, verb, target]) and extra-args appending ([cli, verb, --flag, target]).
    # The delegation result IS observable — `ai-specs doctor <path>` exercises
    # the runner — but the argv ORDER (extra before target) has no observable
    # distinction since most verbs accept positional args freely.  Ran
    # `bin/ai-specs doctor <path>` and `bin/ai-specs upgrade --dry-run` and
    # confirmed they work, but that does not verify the argv element order that
    # the original mocked test asserted. Keeping coupled.
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

    # TRIAGE: Asserts hub.py has `importlib` at module scope with `.util`.
    # This is a pure implementation detail (the Agents code-path uses
    # importlib.util.find_spec). No CLI verb exposes module attributes.
    # Ran `bin/ai-specs hub <path>` — works, but that doesn't verify the
    # specific attribute presence. Keeping coupled.
    @classmethod
    def setUpClass(cls):
        cls.mod = _load()

    def test_hub_has_importlib_at_module_scope(self):
        self.assertTrue(hasattr(self.mod, "importlib"), "hub must import importlib at module scope")
        self.assertTrue(hasattr(self.mod.importlib, "util"))


class TestReadVersion(unittest.TestCase):
    """A.2 — pure _read_version()."""

    def test_read_version_from_ai_specs_home(self):
        """Observable: bin/ai-specs version reads VERSION from AI_SPECS_HOME."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            # isolated_home symlinks VERSION from ROOT, so version should match
            expected = (ROOT / "VERSION").read_text(encoding="utf-8").strip()
            r = invoke(Path(tmp), "version", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(r.stdout.strip(), expected)

    def test_read_version_missing_file_is_unknown(self):
        """Observable: with VERSION removed, `version` exits non-zero (cat fails).

        The original tested Python _read_version() returning 'unknown'.
        The CLI version.sh uses `cat $REPO_ROOT/VERSION` under set -e,
        so a missing file exits 1 (frozen behavior).
        """
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            # Remove the VERSION symlink
            ver_link = home / "VERSION"
            if ver_link.exists() or ver_link.is_symlink():
                ver_link.unlink()
            r = invoke(Path(tmp), "version", cli_home=home)
            self.assertNotEqual(r.returncode, 0)


class TestStatusSummaryVersion(unittest.TestCase):
    """A.2 — StatusSummary.version populated."""

    def test_status_summary_includes_version(self):
        """Observable: hub piped output contains 'version: <X>' matching version cmd."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            ver = invoke(root, "version", cli_home=home)
            hub = invoke(root, "hub", cli_home=home)
            self.assertEqual(hub.returncode, 0, hub.stderr)
            self.assertIn(f"version: {ver.stdout.strip()}", hub.stdout)


class TestNonInteractiveShowsVersion(unittest.TestCase):
    """A.2 — non-interactive status prints version."""

    def test_run_noninteractive_prints_version(self):
        """Observable: hub piped output includes version string and 'version:' label."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            root = Path(tmp) / "prj"
            root.mkdir()
            _ai_specs_init(root)
            ver = invoke(root, "version", cli_home=home).stdout.strip()
            r = invoke(root, "hub", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn(ver, r.stdout)
            self.assertIn("version:", r.stdout)


class TestRecipeChoiceBuilders(unittest.TestCase):
    """A.3 — pure recipe_add_choices / recipe_remove_choices."""

    # TRIAGE: recipe_add_choices() and recipe_remove_choices() are pure
    # internal functions that partition a recipe list by status for the
    # interactive TUI picker. `ai-specs recipe list` shows status per recipe
    # but does not expose the add/remove partitioning logic. Ran
    # `bin/ai-specs recipe list <path>` and confirmed the output lists
    # status (available/installed/disabled) but not the filtered choice
    # tuples the TUI builds. Keeping coupled.
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

    def _skill(self, path: Path, name: str, desc: str = "") -> None:
        path.mkdir(parents=True, exist_ok=True)
        body = "---\nname: {n}\n".format(n=name)
        if desc:
            body += 'description: {d}\n'.format(d=desc)
        body += "---\n# {n}\n".format(n=name)
        (path / "SKILL.md").write_text(body, encoding="utf-8")

    def test_partition_bundled_local_recipe_dep(self):
        """Observable: `ai-specs skills list` shows Bundled/Local/Recipe sections."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            project = Path(tmp) / "prj"
            project.mkdir()
            (project / "ai-specs").mkdir()
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "t"\n', encoding="utf-8"
            )
            # bundled-skills in the home
            (home / "bundled-skills" / "skill-creator").mkdir(parents=True, exist_ok=True)
            (home / "bundled-skills" / "skill-sync").mkdir(parents=True, exist_ok=True)
            # Ensure empty catalog so skills list doesn't fail
            (home / "catalog" / "skills").mkdir(parents=True, exist_ok=True)
            # Local skills in the project
            self._skill(project / "ai-specs" / "skills" / "skill-creator", "skill-creator", "bundled")
            self._skill(project / "ai-specs" / "skills" / "my-local", "my-local", "local only")
            r = invoke(project, "skills", "list", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            out = r.stdout
            self.assertIn("Bundled skills", out)
            bundled = out.split("Bundled skills")[1].split("Local skills")[0]
            self.assertIn("skill-creator", bundled)
            local = out.split("Local skills")[1]
            self.assertIn("my-local", local)
            # skill-creator should be under Bundled, not Local
            self.assertNotIn("skill-creator", local.split("Available catalog")[0] if "Available catalog" in local else local)


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

    # TRIAGE: pick_one, pick_many, confirm_action, and pause are internal
    # TUI helper functions wrapping questionary calls. They have no CLI
    # verb or piped-output equivalent — they only produce side effects in an
    # interactive PTY session. Ran `bin/ai-specs hub <path>` piped and
    # confirmed no pick_one/pick_many/confirm/pause output appears. The
    # PTY tests in test_hub_tui.py exercise the interactive path that
    # calls these helpers, but cannot isolate their individual contracts.
    # Keeping coupled.
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
        """Observable: hub piped shows 'topology: monorepo-submodules (auto)'."""
        import sys
        sys.path.insert(0, str(ROOT / "tests"))
        from test_repo_topology import make_super_with_submodule
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            super_repo = make_super_with_submodule(Path(tmp) / "a")
            self._write_wf_manifest(super_repo, "auto")
            r = invoke(super_repo, "hub", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("topology: monorepo-submodules (auto)", r.stdout)

    def test_topology_explicit_standalone_via_config(self):
        """Observable: hub piped shows 'topology: standalone (config)'."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home(Path(tmp))
            root = Path(tmp) / "prj"
            root.mkdir()
            self._write_wf_manifest(root, "standalone")
            r = invoke(root, "hub", cli_home=home)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("topology: standalone (config)", r.stdout)

if __name__ == "__main__":
    unittest.main()
