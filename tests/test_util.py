"""Unit tests for lib/_internal/util.py (dep-free helpers).

Converted (black-box): the process-boundary contract (test_recipe_add.py) is
satisfied for the observable helpers by driving `bin/ai-specs <verb>` through
the shared `invoke` helper. The remaining classes exercise in-process-only
branches (sys.path scrubbing, pure path joins consumed by TTY gates, and the
pip-bootstrap path) that have no subprocess analogue and stay coupled with
`# TRIAGE:` comments naming each uncovered assertion.
"""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock

from _blackbox import CLI, invoke, isolated_home

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
    sys.modules["util"] = mod
    spec.loader.exec_module(mod)
    return mod


class TestUtilImportContract(unittest.TestCase):
    # TRIAGE: assertTrue(hasattr(mod, "DEPS_SPEC"/"ai_specs_home"/"ensure_deps"))
    # after scrubbing site-packages AND _vendor from sys.path — the in-process
    # "imports with no third-party deps" contract has no subprocess analogue.
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
    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for sequences)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def test_honors_ai_specs_home_env(self):
        # AI_SPECS_HOME is the install root; the recipe catalog is resolved
        # through it (recipe-add._resolve_catalog_dir). Point the home's catalog
        # at an empty recipes dir so the lookup survives but the recipe is
        # missing, proving the catalog came from AI_SPECS_HOME, not the repo.
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs").mkdir()
            (project / "ai-specs" / "ai-specs.toml").write_text('[project]\nname = "test"\n', encoding="utf-8")
            home = self._cli_home()
            catalog = home / "catalog"
            catalog.unlink()
            empty_catalog = Path(tmp) / "empty-catalog"
            (empty_catalog / "recipes").mkdir(parents=True)
            catalog.symlink_to(empty_catalog)
            result = invoke(project, "recipe", "add", "git-pr-flow", cli_home=home)
            self.assertEqual(result.returncode, 1)
            self.assertIn("no encontrada en catalog/recipes/", result.stderr)

    def test_fallback_to_parents2_when_env_unset(self):
        # invoke() cannot express this scenario: it ALWAYS sets AI_SPECS_HOME.
        # To observe the fallback path (ai_specs_home() -> BASH_SOURCE-derived
        # parent-of-parent = repo root), run bin/ai-specs directly with
        # AI_SPECS_HOME removed from the environment but HOME/TMPDIR hermetic.
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs").mkdir()
            (project / "ai-specs" / "ai-specs.toml").write_text('[project]\nname = "test"\n', encoding="utf-8")
            temp = Path(tmp) / "mytmp"
            temp.mkdir()
            env = {k: v for k, v in os.environ.items() if k != "AI_SPECS_HOME"}
            env.update({
                "PATH": os.environ.get("PATH", ""),
                "HOME": str(temp / "home"),
                "TMPDIR": str(temp),
                "AI_SPECS_NO_NETWORK": "1",
                "LC_ALL": "C",
                "LANG": "C",
            })
            (temp / "home").mkdir(parents=True, exist_ok=True)
            proc = subprocess.run(
                [str(CLI), "recipe", "add", "git-pr-flow", str(project)],
                cwd=ROOT, env=env, text=True, capture_output=True, check=False,
            )
            self.assertEqual(proc.returncode, 0)
            manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
            self.assertIn("[recipes.git-pr-flow]", manifest)


class TestVendorDir(unittest.TestCase):
    # TRIAGE: assertEqual(vendor_dir(), <AI_SPECS_HOME>/lib/_vendor) —
    # vendor_dir() is a
    # pure path join consumed only by TTY-gated ensure_deps call sites. Ran
    # `recipe add`, `hub`, and `configure-recipes` non-TTY: none surface the
    # vendor path — recipe-add's ensure_deps sits behind a `tty and` guard,
    # hub's NONINTERACTIVE_STATUS branch returns before it, and configure-recipes
    # returns 3 from its isatty guard without printing the vendor location.
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
    def _cli_home(self) -> Path:
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _hub(self, root: Path):
        return invoke(root, "hub", cli_home=self._cli_home())

    def test_true_when_manifest_file_exists(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text('[project]\nname = "test"\n', encoding="utf-8")
            result = self._hub(root)
            # D5: hub always exits 0 for an initialized project; the status banner confirms it.
            self.assertEqual(result.returncode, 0)
            self.assertIn("ai-specs status", result.stdout)

    def test_false_when_manifest_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = self._hub(Path(tmp))
            self.assertEqual(result.returncode, 2)
            self.assertIn("no ai-specs project", result.stderr)

    def test_false_when_manifest_is_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs" / "ai-specs.toml").mkdir(parents=True)
            result = self._hub(root)
            self.assertEqual(result.returncode, 2)
            self.assertIn("no ai-specs project", result.stderr)


class TestIsInternalTestRecipe(unittest.TestCase):
    def _cli_home(self) -> Path:
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _add(self, root: Path, recipe_id: str):
        return invoke(root, "recipe", "add", recipe_id, cli_home=self._cli_home())

    def _project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "ai-specs").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text('[project]\nname = "test"\n', encoding="utf-8")
        return root

    def test_true_for_test_prefix(self):
        project = self._project()
        result = self._add(project, "test-fixture")
        self.assertEqual(result.returncode, 1)
        self.assertIn("test-fixture", result.stderr)
        self.assertIn("internal test fixture", result.stderr)
        result2 = self._add(project, "test-conflict-a")
        self.assertEqual(result2.returncode, 1)

    def test_false_for_public_recipes(self):
        project = self._project()
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("internal test fixture", result.stderr)
        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.trello-mcp-workflow]", manifest)

    def test_internal_test_recipe_message(self):
        project = self._project()
        result = self._add(project, "test-fixture")
        self.assertEqual(result.returncode, 1)
        self.assertIn("test-fixture", result.stderr)
        self.assertIn("is an internal test fixture and is not part of the public catalog", result.stderr)


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

    # TRIAGE: assertIsNone(rc) — the "rich/questionary importable -> ensure_deps
    # returns None" branch has no observable equivalent. Define $PROJECT = a
    # temp initialized project (ai-specs/ai-specs.toml enabling git-pr-flow).
    # Ran `bin/ai-specs configure-recipes $PROJECT` non-TTY: rc 3, stdout == '',
    # stderr == 'configure-recipes requires an interactive TTY\n', tree diff []
    # created / [] deleted / [] modified — the command runs ensure_deps first
    # (returning None because rich/questionary import through repo lib/_vendor
    # on this machine) then its isatty guard returns 3, so the None return value
    # is masked by the TTY-guard rc-3 and never surfaces in any output or tree.
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

    # TRIAGE: assertEqual(rc, 3) — the mkdir-PermissionError branch of
    # ensure_deps. Define $PROJECT = a temp initialized project
    # (ai-specs/ai-specs.toml). Ran `bin/ai-specs recipe add trello-mcp-workflow
    # $PROJECT`
    # non-TTY: rc 0, stdout starts "Recipe 'trello-mcp-workflow' agregada al
    # manifest.\n", stderr '', tree diff created [] / deleted [] / modified
    # ['ai-specs/ai-specs.toml'] and NO lib/_vendor node appears — the add gate
    # sits behind `tty and`, so ensure_deps never runs and no mkdir fires. Ran
    # `bin/ai-specs configure-recipes $PROJECT` non-TTY: rc 3, stdout '',
    # stderr 'configure-recipes requires an interactive TTY\n', tree unchanged —
    # this rc-3 is the isatty guard, while ensure_deps itself already returned
    # None (deps import) without reaching mkdir. No CLI invocation produces a
    # PermissionError-on-mkdir; only patching builtins.__import__ and the Path
    # object reaches it, so the branch stays coupled.
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

    # TRIAGE: assertEqual(rc,3) + assertFalse(vendor.exists()) — the
    # deps-missing + non-TTY no-prompt branch. Define $PROJECT = a temp
    # initialized project (ai-specs/ai-specs.toml with a git-pr-flow recipe).
    # Ran `bin/ai-specs hub $PROJECT` non-TTY: rc 0, stdout is the status/help
    # banner beginning 'ai-specs status —' then a Summary line and the command
    # menu,
    # stderr = '', tree diff created [] / deleted [] / modified [], and no
    # lib/_vendor node anywhere — hub takes the NONINTERACTIVE_STATUS branch and
    # never calls ensure_deps. Ran `bin/ai-specs recipe add trello-mcp-workflow
    # $PROJECT` non-TTY: rc 0, no vendor dir created (tree modifies only
    # ai-specs/ai-specs.toml) because the add gate is behind `tty and`. The exact
    # deps-MISSING non-TTY path (return 3, no vendor dir) is unreachable since
    # deps import on this machine and both real entry points short-circuit around
    # ensure_deps; the covered intent that a non-TTY add skips the dependency
    # gate is asserted by test_recipe_add::test_non_tty_does_not_call_ensure_deps.
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

    # TRIAGE: assertIsNone(rc) / assertTrue(run_mock.called) /
    # assertIn("--target", argv) / assertIn(str(vendor), argv) — the successful
    # pip `--target <vendor>` install branch. Define $PROJECT = a temp
    # initialized project (ai-specs/ai-specs.toml). Ran `bin/ai-specs recipe
    # add git-pr-flow $PROJECT` non-TTY where git-pr-flow is already enabled:
    # rc 1, stdout == '', stderr == "Recipe 'git-pr-flow' ya está en el
    # manifest. Usa ai-specs sync para materializar.\n", tree created [] /
    # deleted [] / modified [] — the already-present check returns before any
    # dependency gate, so pip never runs. Ran `bin/ai-specs recipe add
    # trello-mcp-workflow $PROJECT` non-TTY (config-bearing recipe): rc 0,
    # stderr == '', stdout starts "Recipe 'trello-mcp-workflow' agregada al
    # manifest.\n" and the project tree gains no lib/_vendor node (modified
    # only ai-specs/ai-specs.toml) — the config gate sits behind `tty and`, so
    # ensure_deps (and pip) never runs and the --target argv is never exposed.
    # The pip branch also needs deps MISSING (they import here) and a TTY
    # yes-prompt; real pip would hit the network (AI_SPECS_NO_NETWORK).
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

    # TRIAGE: assertEqual(rc, 3) — the pip-failure branch (CalledProcessError ->
    # 3). Define $PROJECT = a temp initialized project (ai-specs/ai-specs.toml
    # enabling git-pr-flow). Ran `bin/ai-specs configure-recipes $PROJECT`
    # non-TTY: rc 3, stdout == '', stderr == 'configure-recipes requires an
    # interactive TTY\n', tree created [] / deleted [] / modified [] — this rc-3
    # is the isatty guard that runs AFTER ensure_deps returned None (deps import
    # via repo lib/_vendor), so pip never runs and no pip-failure rc is ever
    # exposed; forcing the failure needs a TTY yes-prompt plus a genuinely
    # failing pip install, both unreachable through a non-TTY subprocess.
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
    # TRIAGE: assertGreaterEqual(major,13)/assertLess(major,15) + the vendored
    # rich/questionary importability — a bootstrap contract. Ran `hub`, `recipe
    # add`, and `configure-recipes` non-TTY: the import itself succeeds through
    # the repo lib/_vendor (so rc 0), but the major-version pin is purely
    # in-process; every rich-consuming CLI path (config_wizard, env_scaffold,
    # hub interactive, init_tui) is TTY-gated, and parity contract §12 classifies
    # lib/_vendor bootstrapping / rich rendering as FREE with no Go analogue.
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
