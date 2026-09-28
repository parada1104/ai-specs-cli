"""Black-box envrc scaffold tests: CLI-driven wherever a verb exposes the surface.

The ``envrc-scaffold.py`` module is a compatibility shim over ``env_scaffold.py``,
and every file it writes (``ai-specs.env.example``, the root ``.envrc``) is
CLI-observable through ``bin/ai-specs sync`` (the verb that runs the
"harness env (.envrc + ai-specs.env.example)" step) and
``bin/ai-specs recipe configure --inspect --json`` (the verb that exposes
catalog config schemas). No test may import ``lib/_internal`` modules.

The interactive prompt surface has no CLI verb (configure-recipes is
TTY-gated), so that test is kept as a process-boundary driver invocation
against the isolated home's own lib copy, marked with a distinct
``# TRIAGE:`` comment.
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home  # noqa: E402


def _seed_recipe(home: Path, recipe_id: str, toml: str) -> None:
    """Seed a fixture recipe as a REAL directory in the isolated home catalog.

    populate_catalog keeps per-recipe symlinks into the repository catalog for
    existing ids; writing a fixture recipe.toml through one would touch repo
    files. Real dirs keep every write inside the temp home.
    """
    recipes = home / "catalog" / "recipes"
    recipes.mkdir(parents=True, exist_ok=True)
    rdir = recipes / recipe_id
    if rdir.is_symlink():
        rdir.unlink()  # symlink inside the temp home, never repo content
    rdir.mkdir(parents=True, exist_ok=True)
    (rdir / "recipe.toml").write_text(toml, encoding="utf-8")

ROOT = Path(__file__).resolve().parents[1]


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy and a REAL catalog.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository. The catalog is
    NOT symlinked either: recipe seeding must never resolve through a symlink
    into repository catalog files. The vendored tree stays a symlink: child
    drivers only ever read it. (See test_sync_pipeline for the lib-copy
    precedent.)
    """
    home = isolated_home(base, catalog=False)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    (home / "lib" / "_vendor").symlink_to(ROOT / "lib" / "_vendor")
    return home


_HOMES: dict[str, Path] = {}


def _home_for(project: Path) -> Path:
    """One shared isolated install root per command sequence."""
    base = project.parent
    key = str(base)
    if key not in _HOMES:
        _HOMES[key] = _make_home(base)
    return _HOMES[key]


def _run_internal(project: Path, snippet: str, home: Path, tmpdir: Path):
    """Process-boundary driver for scaffold internals with no CLI verb.

    Runs a small driver program against the isolated home's own lib copy with
    AI_SPECS_HOME bound to it (never importing repo lib code into this test
    process). stdin is closed (input="") so no prompt can block.
    """
    driver = (
        "import json, sys\n"
        "from pathlib import Path\n"
        "from unittest.mock import MagicMock, patch\n"
        f"sys.path.insert(0, {str(home / 'lib' / '_internal')!r})\n"
        f"sys.path.insert(0, {str(home / 'lib' / '_vendor')!r})\n"
        f"project = Path({str(project)!r})\n"
        "results = {}\n"
        + snippet
        + "\nprint('RESULT:' + json.dumps(results, default=str))\n"
    )
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(tmpdir / "home"),
        "TMPDIR": str(tmpdir),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "LC_ALL": "C",
        "LANG": "C",
    }
    (tmpdir / "home").mkdir(parents=True, exist_ok=True)
    return subprocess.run(
        [sys.executable, "-c", driver], input="", text=True,
        capture_output=True, cwd=str(tmpdir), env=env, check=False,
    )


class EnvrcScaffoldTests(unittest.TestCase):
    def _workspace(
        self, recipes: dict[str, tuple[str, bool]] | None = None
    ) -> tuple[Path, Path]:
        """Temp workspace: isolated home (real lib copy) + minimal project.

        Recipe TOMLs are seeded into the home's own catalog via
        populate_catalog (per-recipe symlinks); repo catalog files are never
        touched. Returns (project, home).
        """
        td = tempfile.TemporaryDirectory(prefix="ai-specs-envrc-")
        self.addCleanup(td.cleanup)
        root = Path(td.name)
        project = root / "project"
        home = _home_for(project)
        manifest = '[project]\nname = "p"\n'
        for recipe_id, (toml, enabled) in (recipes or {}).items():
            _seed_recipe(home, recipe_id, toml)
            flag = "true" if enabled else "false"
            manifest += (
                f'\n[recipes.{recipe_id}]\nenabled = {flag}\nversion = "1.0"\n'
            )
        (project / "ai-specs").mkdir(parents=True)
        (project / "ai-specs" / "ai-specs.toml").write_text(
            manifest, encoding="utf-8"
        )
        return project, home

    def _sync(self, project: Path, home: Path):
        result = invoke(project, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def _example(self, project: Path) -> str:
        return (project / "ai-specs.env.example").read_text(encoding="utf-8")

    def test_collect_from_mcp_env(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", "
                    "TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n",
                    True,
                )
            }
        )
        self._sync(project, home)
        text = self._example(project)
        # Collected vars carry the declaring recipe's id in their purpose.
        self.assertIn("TRELLO_API_KEY=", text)
        self.assertIn("TRELLO_TOKEN=", text)
        self.assertIn("required by trello (trello-mcp-workflow)", text)

    def test_non_reference_env_ignored(self):
        project, home = self._workspace(
            {
                "literal-env": (
                    "[recipe]\n"
                    'id = "literal-env"\n'
                    'name = "L"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "svc"\n'
                    'command = "echo"\n'
                    'env = { MODE = "production", TOKEN = "$TOKEN" }\n',
                    True,
                )
            }
        )
        self._sync(project, home)
        text = self._example(project)
        self.assertNotIn("MODE", text)
        self.assertIn("TOKEN", text)

    def test_generate_writes_export_lines(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", "
                    "TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n",
                    True,
                )
            }
        )
        self._sync(project, home)
        path = project / "ai-specs.env.example"
        self.assertTrue(str(path).endswith("ai-specs.env.example"))
        text = path.read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=", text)
        self.assertIn("TRELLO_TOKEN=", text)
        self.assertTrue(text.index("TRELLO_API_KEY") < text.index("TRELLO_TOKEN"))
        self.assertIn("required by trello", text)
        self.assertNotIn("export ", text)

    def test_envrc_never_written(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n",
                    True,
                )
            }
        )
        self._sync(project, home)
        self.assertFalse((project / "ai-specs" / ".envrc").exists())
        self.assertTrue((project / "ai-specs.env.example").is_file())
        self.assertFalse((project / "ai-specs" / ".envrc.example").exists())
        self.assertFalse((project / "ai-specs" / ".env.example").exists())

    def test_existing_example_backed_up(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n",
                    True,
                )
            }
        )
        example = project / "ai-specs.env.example"
        example.write_text("OLD CONTENT\n", encoding="utf-8")
        self._sync(project, home)
        bak = project / "ai-specs.env.example.bak"
        self.assertTrue(bak.is_file())
        self.assertEqual(bak.read_text(encoding="utf-8"), "OLD CONTENT\n")
        self.assertIn("TRELLO_API_KEY", example.read_text(encoding="utf-8"))

    def test_no_enabled_mcp_recipes_writes_empty_template(self):
        project, home = self._workspace(
            {
                "session-context": (
                    "[recipe]\n"
                    'id = "session-context"\n'
                    'name = "Session"\n'
                    'description = "D"\n'
                    'version = "1.0"\n',
                    True,
                )
            }
        )
        self._sync(project, home)
        path = project / "ai-specs.env.example"
        text = path.read_text(encoding="utf-8")
        self.assertIn("no env vars required", text)
        self.assertFalse((project / "ai-specs" / ".envrc").exists())
        self.assertTrue(str(path).endswith("ai-specs.env.example"))

    def test_disabled_recipe_excluded(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n",
                    False,
                )
            }
        )
        self._sync(project, home)
        text = self._example(project)
        self.assertNotIn("TRELLO_API_KEY", text)
        self.assertNotIn("TRELLO_TOKEN", text)
        self.assertIn("no env vars required", text)

    def test_prompt_env_vars_uses_password_api_for_secrets(self):
        """Regression: password= kwarg crashes questionary 2.x; use password()."""
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", "
                    "MODE = \"$MODE\" }\n",
                    True,
                )
            }
        )
        # TRIAGE: ai-specs configure-recipes — interactive prompting via the
        # envrc-scaffold shim runs only inside the TTY-gated offer path (the
        # verb exits 3 otherwise); exercised at the shim's process boundary
        # against the isolated home's own lib copy with a stubbed questionary.
        proc = _run_internal(
            project,
            "import importlib.machinery\n"
            "import importlib.util\n"
            "loader = importlib.machinery.SourceFileLoader(\n"
            f"    'envrc_scaffold', {str(home / 'lib' / '_internal' / 'envrc-scaffold.py')!r}\n"
            ")\n"
            "spec = importlib.util.spec_from_loader(loader.name, loader)\n"
            "m = importlib.util.module_from_spec(spec)\n"
            "sys.modules[spec.name] = m\n"
            "loader.exec_module(m)\n"
            "password = MagicMock()\n"
            "password.return_value.ask.return_value = 'secret-key'\n"
            "text = MagicMock()\n"
            "text.return_value.ask.return_value = 'plain'\n"
            "confirm = MagicMock()\n"
            "confirm.return_value.ask.return_value = True\n"
            "q = MagicMock(password=password, text=text, confirm=confirm)\n"
            "sys.modules['questionary'] = q\n"
            "result = m.prompt_env_vars(project)\n"
            "results['TRELLO_API_KEY'] = result['TRELLO_API_KEY']\n"
            "results['MODE'] = result['MODE']\n"
            "results['password_called'] = bool(password.call_args_list)\n"
            "results['text_password_kwargs'] = [\n"
            "    'password' in c.kwargs for c in text.call_args_list\n"
            "]\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        lines = [l for l in proc.stdout.splitlines() if l.startswith("RESULT:")]
        self.assertTrue(
            lines,
            f"driver produced no RESULT line:\n{proc.stdout}\n{proc.stderr}",
        )
        results = json.loads(lines[-1][len("RESULT:") :])
        self.assertEqual(results["TRELLO_API_KEY"], "secret-key")
        self.assertEqual(results["MODE"], "plain")
        self.assertTrue(results["password_called"])
        # Secrets must not go through text(..., password=...)
        for used in results["text_password_kwargs"]:
            self.assertFalse(used)

    def test_generate_includes_env_var_help_comments(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", "
                    "TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n",
                    True,
                )
            }
        )
        self._sync(project, home)
        path = project / "ai-specs.env.example"
        self.assertTrue(str(path).endswith("ai-specs.env.example"))
        text = path.read_text(encoding="utf-8")
        self.assertIn("trello.com/power-ups/admin", text)
        self.assertIn("TRELLO_API_KEY", text)
        self.assertIn("TRELLO_TOKEN", text)

    def test_env_var_help_map_has_known_vars(self):
        """Help text for known vars renders into the generated example.

        The help map is only CLI-observable through the example comments, so
        this drives sync with the recipes that declare all three known vars
        (trello + vault) and asserts the help needles.
        """
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", "
                    "TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n",
                    True,
                ),
                "vault-canonical-store": (
                    "[recipe]\n"
                    'id = "vault-canonical-store"\n'
                    'name = "Vault"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "vault"\n'
                    'command = "npx"\n'
                    'env = { CANONICAL_VAULT_PATH = "$CANONICAL_VAULT_PATH" }\n',
                    True,
                ),
            }
        )
        self._sync(project, home)
        text = self._example(project)
        self.assertIn("TRELLO_API_KEY=", text)
        self.assertIn("https://trello.com/power-ups/admin", text)
        self.assertIn("TRELLO_TOKEN=", text)
        self.assertIn("Power-Up → API key page → Token", text)
        self.assertIn("CANONICAL_VAULT_PATH=", text)
        self.assertIn("project-scoped vault folder", text)

    def test_catalog_config_fields_have_help_text(self):
        """Key catalog ConfigFields must ship wizard help_text.

        The recipe config schema is CLI-observable through
        ``ai-specs recipe configure <id> --inspect --json`` (schema.fields),
        read from the isolated home's catalog (a read-only symlink to the
        repository catalog).
        """
        required = {
            "trello-mcp-workflow": ["board_id", "default_list", "epic_list"],
            "worktree-flow": ["integration_branch", "worktrees_dir", "gate_mode"],
            "git-pr-flow": ["base_branch", "expected_owner", "auto_switch_account"],
            "gitlab-mr-flow": ["base_branch", "expected_owner", "auto_switch_account"],
            "bitbucket-pr-flow": ["base_branch", "expected_owner", "auto_switch_account"],
            "vault-canonical-store": ["vault_scope", "decisions_folder", "sessions_folder"],
            "tdd-flow": ["test_command"],
        }
        td = tempfile.TemporaryDirectory(prefix="ai-specs-envrc-schema-")
        self.addCleanup(td.cleanup)
        project = Path(td.name) / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "p"\n', encoding="utf-8"
        )
        for recipe_id, keys in required.items():
            result = invoke(
                project, "recipe", "configure", recipe_id, "--inspect", "--json"
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            fields = {
                field["key"]: field
                for field in json.loads(result.stdout)["schema"]["fields"]
            }
            for key in keys:
                self.assertIn(key, fields, f"{recipe_id}.{key} missing from schema")
                self.assertTrue(
                    fields[key]["help_text"] and fields[key]["help_text"].strip(),
                    f"{recipe_id}.{key} missing help_text",
                )


if __name__ == "__main__":
    unittest.main()
