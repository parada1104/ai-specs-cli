"""Black-box env scaffold tests: CLI-driven wherever a verb exposes the surface.

CLI-observable tests drive ``bin/ai-specs sync`` (the verb that runs the
"harness env (.envrc + ai-specs.env.example)" step) through its process
boundary and assert on ai-specs.env.example / .envrc bytes, stderr warning
needles, and exit codes. No test may import ``lib/_internal`` modules.

Internal scaffold / dep-install surfaces with no CLI verb (write_env, legacy
migration, the TTY-gated configure-recipes offer/prompt path, install plans)
are kept as process-boundary driver invocations against the isolated home's
own lib copy, each marked with a distinct ``# TRIAGE:`` comment.
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
from types import SimpleNamespace

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

# Frozen .envrc managed-block contract (also asserted through the CLI below).
MANAGED_START = "# managed-by: ai-specs (do not remove block)"
MANAGED_END = "# end managed-by: ai-specs"
MANAGED_BODY = "dotenv_if_exists .env\ndotenv_if_exists ai-specs.env"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy and a REAL catalog.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository. The catalog is
    NOT symlinked either: recipe seeding must never resolve through a symlink
    into repository catalog files. The vendored tree stays a symlink: child
    drivers only ever read it.
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


def _trello_toml() -> str:
    return (
        "[recipe]\n"
        'id = "trello-mcp-workflow"\n'
        'name = "Trello"\n'
        'description = "D"\n'
        'version = "1.0"\n\n'
        "[[provides.mcp]]\n"
        'id = "trello"\n'
        'command = "npx"\n'
        "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
    )


def _vault_toml() -> str:
    return (
        "[recipe]\n"
        'id = "vault-canonical-store"\n'
        'name = "Vault"\n'
        'description = "D"\n'
        'version = "1.0"\n\n'
        "[[provides.mcp]]\n"
        'id = "vault"\n'
        'command = "npx"\n'
        'env = { CANONICAL_VAULT_PATH = "$CANONICAL_VAULT_PATH" }\n'
    )


def _jinna_toml() -> str:
    """Minimal jinna recipe whose MCP preset references the OpenProject vars."""
    return (
        "[recipe]\n"
        'id = "jinna-mcp-recipe"\n'
        'name = "OpenProject Provider MCP"\n'
        'description = "D"\n'
        'version = "1.0.0"\n\n'
        "[[provides.mcp]]\n"
        'id = "jinna"\n'
        'command = "{dep:jinna}"\n'
        'args = ["mcp"]\n'
        'env = { OPENPROJECT_BASE_URL = "$OPENPROJECT_BASE_URL", '
        'OPENPROJECT_API_TOKEN = "$OPENPROJECT_API_TOKEN", '
        'OPENPROJECT_AUTH = "$OPENPROJECT_AUTH" }\n'
        'env_allowed = { OPENPROJECT_AUTH = ["basic", "bearer"] }\n'
    )


class EnvScaffoldTests(unittest.TestCase):
    def _workspace(
        self, recipes: dict[str, tuple[str, bool]] | None = None
    ) -> tuple[Path, Path]:
        """Temp workspace: isolated home (real lib copy) + minimal project.

        Recipe TOMLs are seeded into the home's own catalog via
        populate_catalog (per-recipe symlinks); repo catalog files are never
        touched. Returns (project, home).
        """
        td = tempfile.TemporaryDirectory(prefix="ai-specs-env-")
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

    def _result(self, proc: subprocess.CompletedProcess) -> dict:
        """Parse the driver's RESULT JSON payload (last one wins)."""
        lines = [l for l in proc.stdout.splitlines() if l.startswith("RESULT:")]
        if not lines:
            self.fail(
                "internal driver produced no RESULT line\n"
                f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
            )
        return json.loads(lines[-1][len("RESULT:") :])

    def _sync(self, project: Path, home: Path):
        return invoke(project, "sync", cli_home=home)

    # ------------------------------------------------------------------
    # write_env — TRIAGE: ai-specs sync never writes ai-specs.env (only its
    # .example); ai-specs.env is written only by the TTY-gated
    # configure-recipes offer path, so there is no non-interactive CLI
    # surface. Exercised at the module's process boundary against the
    # isolated home's own lib copy.
    # ------------------------------------------------------------------

    def test_write_env_uses_dotenv_not_export(self):
        project, home = self._workspace()
        app_env = project / ".env"
        app_env.write_text("APP=keep\n", encoding="utf-8")
        # TRIAGE: ai-specs sync — write_env (ai-specs.env) has no non-interactive
        # CLI surface (configure-recipes offer path is TTY-gated).
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "m.write_env(project, {'TRELLO_API_KEY': 'secret'})\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=secret", text)
        self.assertNotIn("export ", text)
        self.assertEqual(app_env.read_text(encoding="utf-8"), "APP=keep\n")

    def test_write_env_merges_preserves_extras(self):
        project, home = self._workspace()
        env = project / "ai-specs.env"
        env.write_text("CUSTOM=1\nTRELLO_API_KEY=old\n", encoding="utf-8")
        # TRIAGE: ai-specs sync — write_env (ai-specs.env) has no non-interactive
        # CLI surface (configure-recipes offer path is TTY-gated).
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "m.write_env(project, {'TRELLO_API_KEY': 'new'})\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        text = env.read_text(encoding="utf-8")
        self.assertIn("CUSTOM=1", text)
        self.assertIn("TRELLO_API_KEY=new", text)

    def test_write_env_quotes_spaces(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs sync — write_env (ai-specs.env) has no non-interactive
        # CLI surface (configure-recipes offer path is TTY-gated).
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "m.write_env(project, {'CANONICAL_VAULT_PATH': '/path with spaces/x'})\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn('CANONICAL_VAULT_PATH="/path with spaces/x"', text)

    def test_write_env_blank_preserves_existing_secret(self):
        """JD-1: blank/whitespace updates must not wipe non-empty harness secrets."""
        project, home = self._workspace()
        # TRIAGE: ai-specs sync — write_env (ai-specs.env) has no non-interactive
        # CLI surface (configure-recipes offer path is TTY-gated).
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "m.write_env(project, {'TRELLO_API_KEY': 'secret', 'TRELLO_TOKEN': 'tok'})\n"
            "m.write_env(\n"
            "    project,\n"
            "    {'TRELLO_API_KEY': '', 'TRELLO_TOKEN': '   ', 'CUSTOM': 'keep-me'},\n"
            ")\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=secret", text)
        self.assertIn("TRELLO_TOKEN=tok", text)
        self.assertIn("CUSTOM=keep-me", text)

    def test_offer_harness_env_blank_prompt_preserves_existing(self):
        """JD-1: offer path with blank prompt values preserves prior ai-specs.env."""
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_API_KEY=keep-key\nTRELLO_TOKEN=keep-tok\n",
            encoding="utf-8",
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env runs only on an
        # interactive TTY (exit 3 otherwise); exercised at its process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "blanks = {'TRELLO_API_KEY': '', 'TRELLO_TOKEN': ''}\n"
            "with patch.object(m, 'prompt_env_vars', return_value=dict(blanks)), "
            "patch.object(m, 'direnv_allow', return_value=True):\n"
            "    m.offer_harness_env(project, offer_direnv_install=False)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=keep-key", text)
        self.assertIn("TRELLO_TOKEN=keep-tok", text)

    # ------------------------------------------------------------------
    # generate_env_example / ensure_root_envrc — CLI-observable through
    # `ai-specs sync` (the harness-env step).
    # ------------------------------------------------------------------

    def test_generate_env_example(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        path = project / "ai-specs.env.example"
        text = path.read_text(encoding="utf-8")
        self.assertTrue(str(path).endswith("ai-specs.env.example"))
        self.assertIn("TRELLO_API_KEY=", text)
        self.assertIn("trello.com/power-ups/admin", text)
        self.assertNotIn("export ", text)
        self.assertFalse((project / "ai-specs" / ".envrc.example").exists())
        self.assertFalse((project / "ai-specs" / ".env.example").exists())

    def test_generate_env_example_backup(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        example = project / "ai-specs.env.example"
        example.write_text("OLD\n", encoding="utf-8")
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            (project / "ai-specs.env.example.bak").read_text(encoding="utf-8"),
            "OLD\n",
        )

    def test_generate_env_example_skips_identical_rewrite(self):
        """Idempotent sync must not create .bak when example content is unchanged."""
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        first = self._sync(project, home)
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        second = self._sync(project, home)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertFalse((project / "ai-specs.env.example.bak").exists())
        self.assertTrue((project / "ai-specs.env.example").is_file())

    def test_generate_env_example_renders_openproject_auth_provider_default(self):
        """The example renders the provider's effective default `basic`; others stay blank."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), True)})
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = (project / "ai-specs.env.example").read_text(encoding="utf-8")
        self.assertRegex(text, r"(?m)^OPENPROJECT_AUTH=basic\s+#")
        self.assertRegex(text, r"(?m)^OPENPROJECT_BASE_URL=\s+#")
        self.assertRegex(text, r"(?m)^OPENPROJECT_API_TOKEN=\s+#")
        # Example rendering only: it must never create the runtime env file.
        self.assertFalse((project / "ai-specs.env").exists())

    def test_generate_env_example_default_is_example_only_and_idempotent(self):
        """A prefilled default stays out of runtime env and keeps .bak idempotence."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), True)})
        first = self._sync(project, home)
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        second = self._sync(project, home)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        combined = second.stdout + second.stderr
        self.assertIn("! OPENPROJECT_AUTH has no value in ai-specs.env", combined)
        self.assertFalse((project / "ai-specs.env.example.bak").exists())
        self.assertFalse((project / "ai-specs.env").exists())

    def test_generate_env_example_matches_real_catalog_jinna_recipe(self):
        """Triangulation: the real catalog declaration yields the same default line."""
        td = tempfile.TemporaryDirectory(prefix="ai-specs-env-real-")
        self.addCleanup(td.cleanup)
        project = Path(td.name) / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "p"\n\n'
            '[recipes.jinna-mcp-recipe]\nenabled = true\nversion = "1.0.0"\n',
            encoding="utf-8",
        )
        # Default isolated home: catalog symlinks the repo catalog (read-only).
        result = invoke(project, "sync")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = (project / "ai-specs.env.example").read_text(encoding="utf-8")
        self.assertRegex(text, r"(?m)^OPENPROJECT_AUTH=basic\s+#")
        self.assertIn("provider's effective default", text)
        self.assertRegex(text, r"(?m)^OPENPROJECT_BASE_URL=\s+#")

    def test_ensure_root_envrc_creates(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = (project / ".envrc").read_text(encoding="utf-8")
        self.assertIn(MANAGED_START, text)
        self.assertIn("dotenv_if_exists .env", text)
        self.assertIn("dotenv_if_exists ai-specs.env", text)

    def test_ensure_root_envrc_appends_preserving_custom(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        envrc = project / ".envrc"
        envrc.write_text("use nix\n", encoding="utf-8")
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = envrc.read_text(encoding="utf-8")
        self.assertTrue(text.startswith("use nix\n"))
        self.assertIn(MANAGED_START, text)

    def test_ensure_root_envrc_idempotent(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        first = self._sync(project, home)
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        second = self._sync(project, home)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        text = (project / ".envrc").read_text(encoding="utf-8")
        self.assertEqual(text.count(MANAGED_START), 1)

    # ------------------------------------------------------------------
    # Legacy migration — TRIAGE: ai-specs sync — legacy harness-env
    # migration (migrate_legacy_envrc / migrate_nested_harness_env /
    # migrate_legacy_harness_env) is not reachable from any CLI verb
    # (env_scaffold's sync step only generates the example and the root
    # .envrc). Exercised at the module's process boundary.
    # ------------------------------------------------------------------

    def test_migrate_legacy_envrc(self):
        project, home = self._workspace()
        legacy = project / "ai-specs" / ".envrc"
        legacy.write_text(
            'export TRELLO_TOKEN="abc"\nexport EMPTY=""\n',
            encoding="utf-8",
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_API_KEY=keep\nTRELLO_TOKEN=existing\n",
            encoding="utf-8",
        )
        # TRIAGE: ai-specs sync — legacy .envrc migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_legacy_envrc(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["ran"])
        env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=keep", env_text)
        self.assertIn("TRELLO_TOKEN=existing", env_text)
        self.assertFalse(legacy.exists())
        self.assertTrue((project / "ai-specs" / ".envrc.bak").is_file())
        self.assertTrue((project / ".envrc").is_file())

    def test_migrate_legacy_envrc_fills_absent_key(self):
        """Absent harness key receives migrated export value from legacy .envrc."""
        project, home = self._workspace()
        legacy = project / "ai-specs" / ".envrc"
        legacy.write_text(
            'export TRELLO_TOKEN="abc"\n',
            encoding="utf-8",
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_API_KEY=keep\n",
            encoding="utf-8",
        )
        # TRIAGE: ai-specs sync — legacy .envrc migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_legacy_envrc(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["ran"])
        env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=keep", env_text)
        self.assertIn("TRELLO_TOKEN=abc", env_text)
        self.assertFalse(legacy.exists())
        self.assertTrue((project / "ai-specs" / ".envrc.bak").is_file())

    def test_migrate_nested_harness_env(self):
        project, home = self._workspace()
        nested = project / "ai-specs" / ".env"
        nested.write_text("TRELLO_API_KEY=legacy\n", encoding="utf-8")
        # TRIAGE: ai-specs sync — nested ai-specs/.env migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_nested_harness_env(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["ran"])
        env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=legacy", env_text)
        self.assertFalse(nested.exists())
        self.assertTrue((project / "ai-specs" / ".env.bak").is_file())
        self.assertIn(
            "dotenv_if_exists ai-specs.env",
            (project / ".envrc").read_text(encoding="utf-8"),
        )

    def test_migrate_nested_empty_does_not_rename(self):
        """JD-6: comment-only nested .env must not be renamed to .env.bak."""
        project, home = self._workspace()
        nested = project / "ai-specs" / ".env"
        nested.write_text("# only comments\n\n", encoding="utf-8")
        # TRIAGE: ai-specs sync — nested ai-specs/.env migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_nested_harness_env(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertFalse(results["ran"])
        self.assertTrue(nested.is_file())
        self.assertFalse((project / "ai-specs" / ".env.bak").exists())

    def test_migrate_nested_export_lines_merged(self):
        """JD-6: export-prefixed nested .env keys must parse, merge, then rename."""
        project, home = self._workspace()
        nested = project / "ai-specs" / ".env"
        nested.write_text('export TRELLO_TOKEN="from-export"\n', encoding="utf-8")
        # TRIAGE: ai-specs sync — nested ai-specs/.env migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_nested_harness_env(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["ran"])
        env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_TOKEN=from-export", env_text)
        self.assertFalse(nested.exists())
        self.assertTrue((project / "ai-specs" / ".env.bak").is_file())

    def test_migrate_legacy_envrc_empty_does_not_rename(self):
        """JD-6 parity: non-export legacy .envrc is left in place (no silent bak)."""
        project, home = self._workspace()
        legacy = project / "ai-specs" / ".envrc"
        legacy.write_text("# no exports\n", encoding="utf-8")
        # TRIAGE: ai-specs sync — legacy .envrc migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_legacy_envrc(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertFalse(results["ran"])
        self.assertTrue(legacy.is_file())
        self.assertFalse((project / "ai-specs" / ".envrc.bak").exists())

    def test_managed_block_is_current_rejects_stale_body(self):
        """JD-8 helper: markers with old dotenv path are not current."""
        project, home = self._workspace()
        # TRIAGE: ai-specs sync — has_managed_block/managed_block_is_current are
        # internal predicates (the .envrc managed block itself is CLI-asserted
        # by test_ensure_root_envrc_*); exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "stale = m.MANAGED_START + '\\ndotenv_if_exists .env\\n"
            "dotenv_if_exists ai-specs/.env\\n' + m.MANAGED_END + '\\n'\n"
            "results['stale_has_block'] = m.has_managed_block(stale)\n"
            "results['stale_is_current'] = m.managed_block_is_current(stale)\n"
            "results['current_is_current'] = m.managed_block_is_current("
            "m.managed_block_text() + '\\n')\n"
            "results['block_text'] = m.managed_block_text()\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["stale_has_block"])
        self.assertFalse(results["stale_is_current"])
        self.assertTrue(results["current_is_current"])
        # Frozen managed-block contract, checked against the module's own text.
        self.assertEqual(
            results["block_text"],
            f"{MANAGED_START}\n{MANAGED_BODY}\n{MANAGED_END}",
        )

    def test_migrate_noop_when_absent(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs sync — legacy harness-env migration has no CLI verb surface.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "results['ran'] = m.migrate_legacy_harness_env(project)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertFalse(results["ran"])

    # ------------------------------------------------------------------
    # offer_harness_env paths — TRIAGE: ai-specs configure-recipes — the
    # offer path runs only on an interactive TTY (the verb exits 3
    # otherwise), so there is no non-interactive CLI surface. Exercised at
    # the module's process boundary against the isolated home's own lib copy.
    # ------------------------------------------------------------------

    def test_offer_harness_env_soft_fails_on_prompt_error(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env is TTY-gated;
        # exercised at its process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "with patch.object(m, 'prompt_env_vars', "
            "side_effect=TypeError('password=')):\n"
            "    m.offer_harness_env(project, offer_direnv_install=False)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertFalse((project / "ai-specs.env").exists())

    def test_offer_harness_env_invokes_direnv_allow(self):
        """When direnv is present, offer path runs `direnv allow <project_root>`."""
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env is TTY-gated;
        # exercised at its process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "values = {'TRELLO_API_KEY': 'k', 'TRELLO_TOKEN': 't'}\n"
            "proc = MagicMock(returncode=0)\n"
            "allow_calls = []\n"
            "def run_recorder(*args, **kwargs):\n"
            "    if args and isinstance(args[0], list):\n"
            "        allow_calls.append(list(args[0]))\n"
            "    return proc\n"
            "with patch.object(m, 'prompt_env_vars', return_value=dict(values)), "
            "patch('shutil.which', return_value='/usr/bin/direnv'), "
            "patch('subprocess.run', side_effect=run_recorder):\n"
            "    m.offer_harness_env(project, offer_direnv_install=False)\n"
            "results['allow_calls'] = allow_calls\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        allow_calls = [
            c for c in results["allow_calls"] if c[:2] == ["direnv", "allow"]
        ]
        self.assertEqual(len(allow_calls), 1)
        self.assertEqual(allow_calls[0], ["direnv", "allow", str(project)])
        self.assertTrue((project / "ai-specs.env").is_file())

    def test_offer_harness_env_soft_fails_without_direnv(self):
        """Missing direnv does not abort; non-fatal install guidance is printed."""
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env is TTY-gated;
        # exercised at its process boundary.
        proc = _run_internal(
            project,
            "import builtins\n"
            "import env_scaffold as m\n"
            "values = {'TRELLO_API_KEY': 'k', 'TRELLO_TOKEN': 't'}\n"
            "prints = []\n"
            "def print_recorder(*args, **kwargs):\n"
            "    if args:\n"
            "        prints.append(' '.join(str(a) for a in args))\n"
            "with patch.object(m, 'prompt_env_vars', return_value=dict(values)), "
            "patch('shutil.which', return_value=None), "
            "patch('subprocess.run', side_effect=FileNotFoundError('direnv')), "
            "patch.object(builtins, 'print', side_effect=print_recorder):\n"
            "    # Soft-fail path only — isolate from direnv install offer.\n"
            "    m.offer_harness_env(project, offer_direnv_install=False)\n"
            "results['guidance'] = ' '.join(prints)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue((project / "ai-specs.env").is_file())
        self.assertTrue((project / ".envrc").is_file())
        guidance = results["guidance"]
        self.assertIn("direnv", guidance.lower())
        self.assertIn("brew install direnv", guidance)

    def test_offer_harness_env_offers_direnv_install_when_missing_tty(self):
        """When direnv is missing on a TTY, offer path calls dep_install.offer_and_install."""
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env is TTY-gated;
        # exercised at its process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "values = {'TRELLO_API_KEY': 'k', 'TRELLO_TOKEN': 't'}\n"
            "dep_install = MagicMock()\n"
            "plan = MagicMock(\n"
            "    binary='direnv',\n"
            "    command=['brew', 'install', 'direnv'],\n"
            "    kind='brew',\n"
            ")\n"
            "dep_install.resolve_install_plan.return_value = plan\n"
            "dep_install.offer_and_install.return_value = []\n"
            "with patch.object(m, 'prompt_env_vars', return_value=dict(values)), "
            "patch.object(m, '_load_sibling', return_value=dep_install), "
            "patch.object(m, 'direnv_allow', return_value=False), "
            "patch('shutil.which', return_value=None), "
            "patch.object(sys.stdin, 'isatty', return_value=True), "
            "patch.object(sys.stdout, 'isatty', return_value=True):\n"
            "    m.offer_harness_env(project, offer_direnv_install=True)\n"
            "results['resolve_called'] = dep_install.resolve_install_plan.called\n"
            "results['resolve_binary'] = "
            "dep_install.resolve_install_plan.call_args.args[0]\n"
            "results['offer_count'] = dep_install.offer_and_install.call_count\n"
            "results['offer_plan_binaries'] = [\n"
            "    p.binary for p in dep_install.offer_and_install.call_args.args[0]\n"
            "]\n"
            "results['offer_tty'] = "
            "dep_install.offer_and_install.call_args.kwargs.get('tty')\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["resolve_called"])
        self.assertEqual(results["resolve_binary"], "direnv")
        self.assertEqual(results["offer_count"], 1)
        self.assertEqual(results["offer_plan_binaries"], ["direnv"])
        self.assertTrue(results["offer_tty"])

    # ------------------------------------------------------------------
    # prompt_env_vars / collect_* — TRIAGE: ai-specs configure-recipes —
    # interactive prompting and recipe_ids-scoped collection run only inside
    # the TTY-gated offer path (the aggregate map is CLI-asserted via the
    # generated example in the sync tests above). Exercised at the module's
    # process boundary with a stubbed questionary.
    # ------------------------------------------------------------------

    def test_prompt_env_vars_uses_password_api_for_secrets(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        # TRIAGE: ai-specs configure-recipes — interactive prompting is TTY-gated;
        # exercised at its process boundary with a stubbed questionary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "password = MagicMock()\n"
            "password.return_value.ask.return_value = 'secret-key'\n"
            "text = MagicMock()\n"
            "text.return_value.ask.return_value = 'plain'\n"
            "confirm = MagicMock()\n"
            "confirm.return_value.ask.return_value = True\n"
            "q = MagicMock(password=password, text=text, confirm=confirm)\n"
            "sys.modules['questionary'] = q\n"
            "# MODE not in trello toml — only secrets\n"
            "result = m.prompt_env_vars(project)\n"
            "results['TRELLO_API_KEY'] = result['TRELLO_API_KEY']\n"
            "results['password_called'] = bool(password.call_args_list)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["TRELLO_API_KEY"], "secret-key")
        self.assertTrue(results["password_called"])

    def test_collect_env_vars_selected_recipe_only(self):
        """recipe_ids narrows collection to the recipe being configured."""
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (_trello_toml(), True),
                "vault-canonical-store": (_vault_toml(), True),
            }
        )
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "selected = m.collect_env_vars(project, recipe_ids=['trello-mcp-workflow'])\n"
            "results['selected'] = sorted(selected)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["selected"], ["TRELLO_API_KEY", "TRELLO_TOKEN"])

    def test_collect_env_vars_aggregate_when_omitted(self):
        """Backward compatibility: omitting recipe_ids keeps aggregate behavior."""
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (_trello_toml(), True),
                "vault-canonical-store": (_vault_toml(), True),
            }
        )
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "aggregate = m.collect_env_vars(project)\n"
            "explicit_none = m.collect_env_vars(project, recipe_ids=None)\n"
            "results['aggregate'] = sorted(aggregate)\n"
            "results['explicit_none'] = sorted(explicit_none)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(
            results["aggregate"],
            ["CANONICAL_VAULT_PATH", "TRELLO_API_KEY", "TRELLO_TOKEN"],
        )
        self.assertEqual(results["explicit_none"], results["aggregate"])

    def test_collect_env_vars_empty_selection_collects_nothing(self):
        """An empty selection is a valid scope: no recipe contributes vars."""
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (_trello_toml(), True),
                "vault-canonical-store": (_vault_toml(), True),
            }
        )
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "selected = m.collect_env_vars(project, recipe_ids=[])\n"
            "results['selected'] = selected\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["selected"], {})

    def test_collect_env_vars_selected_skips_disabled_recipe(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), False)}
        )
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "selected = m.collect_env_vars(project, recipe_ids=['trello-mcp-workflow'])\n"
            "results['selected'] = selected\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["selected"], {})

    def test_collect_env_allowed_reads_recipe_declaration(self):
        """A recipe may constrain an MCP env reference to declared allowed values."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), True)})
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "allowed = m.collect_env_allowed(project, recipe_ids=['jinna-mcp-recipe'])\n"
            "results['allowed'] = allowed\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["allowed"], {"OPENPROJECT_AUTH": ["basic", "bearer"]})

    def test_collect_env_allowed_skips_disabled_recipe(self):
        """A disabled recipe contributes no constraints."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), False)})
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "allowed = m.collect_env_allowed(project, recipe_ids=['jinna-mcp-recipe'])\n"
            "results['allowed'] = allowed\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["allowed"], {})

    def test_collect_env_allowed_empty_for_unconstrained_recipe(self):
        """A recipe declaring env references but no allowed values constrains nothing."""
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped collection runs
        # only inside the TTY-gated offer path; exercised at the module's
        # process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "allowed = m.collect_env_allowed(project)\n"
            "results['allowed'] = allowed\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["allowed"], {})

    def test_collect_env_allowed_ignores_malformed_declaration(self):
        """A malformed env_allowed shape is ignored, never a bogus or crashing constraint."""
        for malformed in ('"basic"', "7"):
            with self.subTest(malformed=malformed):
                toml = _jinna_toml().replace('["basic", "bearer"]', malformed)
                project, home = self._workspace(
                    {"jinna-mcp-recipe": (toml, True)}
                )
                # TRIAGE: ai-specs configure-recipes — recipe_ids-scoped
                # collection runs only inside the TTY-gated offer path;
                # exercised at the module's process boundary.
                proc = _run_internal(
                    project,
                    "import env_scaffold as m\n"
                    "allowed = m.collect_env_allowed(\n"
                    "    project, recipe_ids=['jinna-mcp-recipe']\n"
                    ")\n"
                    "results['allowed'] = allowed\n",
                    home,
                    project.parent,
                )
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                results = self._result(proc)
                self.assertEqual(results["allowed"], {})

    def test_prompt_env_vars_uses_select_for_declared_allowed_values(self):
        """A constrained var is a closed choice, so a typo like 'basicc' is unrepresentable."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), True)})
        # TRIAGE: ai-specs configure-recipes — interactive prompting is TTY-gated;
        # exercised at its process boundary with a stubbed questionary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "password = MagicMock()\n"
            "password.return_value.ask.return_value = 'token'\n"
            "text = MagicMock()\n"
            "text.return_value.ask.return_value = 'https://op.example'\n"
            "select = MagicMock()\n"
            "select.return_value.ask.return_value = 'bearer'\n"
            "confirm = MagicMock()\n"
            "confirm.return_value.ask.return_value = True\n"
            "q = MagicMock(password=password, text=text, select=select, confirm=confirm)\n"
            "sys.modules['questionary'] = q\n"
            "result = m.prompt_env_vars(project, recipe_ids=['jinna-mcp-recipe'])\n"
            "results['select_called_once'] = select.call_count == 1\n"
            "results['choices'] = select.call_args.kwargs.get('choices')\n"
            "results['value'] = result['OPENPROJECT_AUTH']\n"
            "results['text_vars'] = [c.args[0] for c in text.call_args_list]\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertTrue(results["select_called_once"])
        self.assertEqual(results["choices"], ["basic", "bearer"])
        self.assertEqual(results["value"], "bearer")
        self.assertNotIn("OPENPROJECT_AUTH", results["text_vars"])

    def test_select_default_is_always_one_of_the_choices(self):
        """A stale example default must not leak a value outside the declared set."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), True)})
        (project / "ai-specs.env").write_text(
            "OPENPROJECT_AUTH=legacy\n", encoding="utf-8"
        )
        # TRIAGE: ai-specs configure-recipes — interactive prompting is TTY-gated;
        # exercised at its process boundary with a stubbed questionary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "password = MagicMock()\n"
            "password.return_value.ask.return_value = 'token'\n"
            "text = MagicMock()\n"
            "text.return_value.ask.return_value = 'https://op.example'\n"
            "select = MagicMock()\n"
            "select.return_value.ask.return_value = 'basic'\n"
            "q = MagicMock(\n"
            "    password=password,\n"
            "    text=text,\n"
            "    select=select,\n"
            "    confirm=MagicMock(\n"
            "        return_value=MagicMock(ask=MagicMock(return_value=True))\n"
            "    ),\n"
            ")\n"
            "sys.modules['questionary'] = q\n"
            "with patch.object(\n"
            "    m, 'ENV_EXAMPLE_DEFAULTS', {'OPENPROJECT_AUTH': 'obsolete'}\n"
            "):\n"
            "    m.prompt_env_vars(project, recipe_ids=['jinna-mcp-recipe'])\n"
            "results['default'] = select.call_args.kwargs.get('default')\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertIn(results["default"], ["basic", "bearer"])

    def test_prompt_env_vars_select_default_prefers_existing_value(self):
        """Re-prompting keeps a valid configured value instead of resetting the example default."""
        project, home = self._workspace({"jinna-mcp-recipe": (_jinna_toml(), True)})
        (project / "ai-specs.env").write_text(
            "OPENPROJECT_AUTH=Bearer\n", encoding="utf-8"
        )
        # TRIAGE: ai-specs configure-recipes — interactive prompting is TTY-gated;
        # exercised at its process boundary with a stubbed questionary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "password = MagicMock()\n"
            "password.return_value.ask.return_value = 'token'\n"
            "text = MagicMock()\n"
            "text.return_value.ask.return_value = 'https://op.example'\n"
            "select = MagicMock()\n"
            "select.return_value.ask.return_value = 'bearer'\n"
            "q = MagicMock(\n"
            "    password=password,\n"
            "    text=text,\n"
            "    select=select,\n"
            "    confirm=MagicMock(\n"
            "        return_value=MagicMock(ask=MagicMock(return_value=True))\n"
            "    ),\n"
            ")\n"
            "sys.modules['questionary'] = q\n"
            "m.prompt_env_vars(project, recipe_ids=['jinna-mcp-recipe'])\n"
            "results['default'] = select.call_args.kwargs.get('default')\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["default"], "bearer")

    def test_prompt_env_vars_selected_recipe_only(self):
        """Selected-only prompting keeps secret/non-secret behavior per var."""
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (_trello_toml(), True),
                "vault-canonical-store": (_vault_toml(), True),
            }
        )
        # TRIAGE: ai-specs configure-recipes — interactive prompting is TTY-gated;
        # exercised at its process boundary with a stubbed questionary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "password = MagicMock()\n"
            "password.return_value.ask.return_value = 'secret-key'\n"
            "text = MagicMock()\n"
            "text.return_value.ask.return_value = 'plain'\n"
            "confirm = MagicMock()\n"
            "confirm.return_value.ask.return_value = True\n"
            "q = MagicMock(password=password, text=text, confirm=confirm)\n"
            "sys.modules['questionary'] = q\n"
            "result = m.prompt_env_vars(project, recipe_ids=['trello-mcp-workflow'])\n"
            "results['sorted_keys'] = sorted(result)\n"
            "results['password_called'] = bool(password.call_args_list)\n"
            "results['text_called'] = bool(text.call_args_list)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["sorted_keys"], ["TRELLO_API_KEY", "TRELLO_TOKEN"])
        self.assertTrue(results["password_called"])
        self.assertFalse(results["text_called"])

    def test_offer_harness_env_selected_recipe_only(self):
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (_trello_toml(), True),
                "vault-canonical-store": (_vault_toml(), True),
            }
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env is TTY-gated;
        # exercised at its process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "values = {'TRELLO_API_KEY': 'k', 'TRELLO_TOKEN': 't'}\n"
            "captured = {}\n"
            "def fake_prompt(project_root, recipe_ids=None):\n"
            "    captured['recipe_ids'] = recipe_ids\n"
            "    return dict(values)\n"
            "with patch.object(m, 'prompt_env_vars', side_effect=fake_prompt), "
            "patch.object(m, 'direnv_allow', return_value=True):\n"
            "    m.offer_harness_env(\n"
            "        project,\n"
            "        offer_direnv_install=False,\n"
            "        recipe_ids=['trello-mcp-workflow'],\n"
            "    )\n"
            "results['recipe_ids'] = captured.get('recipe_ids')\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["recipe_ids"], ["trello-mcp-workflow"])
        text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=k", text)
        self.assertIn("TRELLO_TOKEN=t", text)
        self.assertNotIn("CANONICAL_VAULT_PATH", text)

    def test_offer_harness_env_aggregate_when_omitted(self):
        """Backward compatibility: omitted recipe_ids prompts the aggregate map."""
        project, home = self._workspace(
            {
                "trello-mcp-workflow": (_trello_toml(), True),
                "vault-canonical-store": (_vault_toml(), True),
            }
        )
        # TRIAGE: ai-specs configure-recipes — offer_harness_env is TTY-gated;
        # exercised at its process boundary.
        proc = _run_internal(
            project,
            "import env_scaffold as m\n"
            "values = {'TRELLO_API_KEY': 'k', 'TRELLO_TOKEN': 't'}\n"
            "captured = {}\n"
            "def fake_prompt(project_root, recipe_ids=None):\n"
            "    captured['recipe_ids'] = recipe_ids\n"
            "    return dict(values)\n"
            "with patch.object(m, 'prompt_env_vars', side_effect=fake_prompt), "
            "patch.object(m, 'direnv_allow', return_value=True):\n"
            "    m.offer_harness_env(project, offer_direnv_install=False)\n"
            "results['recipe_ids'] = captured.get('recipe_ids')\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertIsNone(results["recipe_ids"])
        text = (project / "ai-specs.env").read_text(encoding="utf-8")
        self.assertIn("TRELLO_API_KEY=k", text)

    # ------------------------------------------------------------------
    # missing_required_values / env_scaffold main — CLI-observable through
    # `ai-specs sync` stderr warnings and exit code.
    # ------------------------------------------------------------------

    def test_missing_required_values_reports_absent_and_blank(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_API_KEY=present\nTRELLO_TOKEN=\n",
            encoding="utf-8",
        )
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        err_text = result.stderr
        self.assertIn(
            "! TRELLO_TOKEN has no value in ai-specs.env — run ai-specs configure-recipes",
            err_text,
        )
        self.assertNotIn("TRELLO_API_KEY", err_text)

    def test_main_warns_missing_values_nonfatal(self):
        project, home = self._workspace(
            {"trello-mcp-workflow": (_trello_toml(), True)}
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_API_KEY=k\n",
            encoding="utf-8",
        )
        result = self._sync(project, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        err_text = result.stderr
        self.assertIn(
            "! TRELLO_TOKEN has no value in ai-specs.env — run ai-specs configure-recipes",
            err_text,
        )
        self.assertNotIn("TRELLO_API_KEY", err_text)
        self.assertFalse((project / "ai-specs" / ".env.example").exists())
        self.assertTrue((project / "ai-specs.env.example").is_file())
        self.assertTrue((project / ".envrc").is_file())
        self.assertEqual(
            (project / "ai-specs.env").read_text(encoding="utf-8"),
            "TRELLO_API_KEY=k\n",
        )


class DepInstallTests(unittest.TestCase):
    def _workspace(self) -> tuple[Path, Path]:
        td = tempfile.TemporaryDirectory(prefix="ai-specs-dep-")
        self.addCleanup(td.cleanup)
        root = Path(td.name)
        project = root / "project"
        project.mkdir()
        return project, _home_for(project)

    def _result(self, proc: subprocess.CompletedProcess) -> dict:
        lines = [l for l in proc.stdout.splitlines() if l.startswith("RESULT:")]
        if not lines:
            self.fail(
                "internal driver produced no RESULT line\n"
                f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
            )
        return json.loads(lines[-1][len("RESULT:") :])

    def _plan(self, results: dict) -> SimpleNamespace:
        return SimpleNamespace(
            binary=results.get("binary", ""),
            command=results["command"],
            display=results.get("display", ""),
        )

    def _assert_never_brew_install_bb(self, plan):
        """Token-aware: never argv `brew install bb`. Do not use substring 'brew install bb'."""
        self.assertNotEqual(plan.command, ["brew", "install", "bb"])
        if len(plan.command) >= 3 and plan.command[:2] == ["brew", "install"]:
            self.assertNotEqual(plan.command[2], "bb")
        tokens = plan.display.split()
        if len(tokens) >= 3 and tokens[:2] == ["brew", "install"]:
            self.assertNotEqual(tokens[2], "bb")

    # ------------------------------------------------------------------
    # dep_install — TRIAGE: ai-specs configure-recipes — install plans are
    # TTY-gated consent offers (resolve_install_plan / offer_and_install
    # have no non-interactive CLI surface). Exercised at the module's
    # process boundary against the isolated home's own lib copy.
    # ------------------------------------------------------------------

    def test_npx_guidance_only(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "plan = m.resolve_install_plan(\n"
            "    'npx', install_url='https://nodejs.org/en/download'\n"
            ")\n"
            "results['kind'] = plan.kind\n"
            "results['command'] = list(plan.command)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["kind"], "guidance")
        self.assertEqual(results["command"], [])

    def test_bb_brew_plan_on_darwin(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "with patch.object(\n"
            "    m.shutil, 'which',\n"
            "    side_effect=lambda b: '/opt/brew' if b == 'brew' else None\n"
            "), patch.object(m.platform, 'system', return_value='Darwin'):\n"
            "    plan = m.resolve_install_plan(\n"
            "        'bb', install_url='https://bb-cli.github.io'\n"
            "    )\n"
            "results['kind'] = plan.kind\n"
            "results['command'] = list(plan.command)\n"
            "results['display'] = plan.display\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["kind"], "brew")
        self.assertEqual(results["command"], ["brew", "install", "bb-cli"])
        self.assertEqual(results["display"], "brew install bb-cli")
        self._assert_never_brew_install_bb(self._plan(results))

    def test_bb_brew_plan_on_linux(self):
        """Linux + Homebrew still offers formula bb-cli (brew wins over apt)."""
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "def which(b):\n"
            "    if b == 'brew':\n"
            "        return '/home/linuxbrew/.linuxbrew/bin/brew'\n"
            "    if b == 'apt-get':\n"
            "        return '/usr/bin/apt-get'\n"
            "    return None\n"
            "with patch.object(m.shutil, 'which', side_effect=which), "
            "patch.object(m.platform, 'system', return_value='Linux'):\n"
            "    plan = m.resolve_install_plan(\n"
            "        'bb', install_url='https://bb-cli.github.io'\n"
            "    )\n"
            "results['kind'] = plan.kind\n"
            "results['command'] = list(plan.command)\n"
            "results['display'] = plan.display\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["kind"], "brew")
        self.assertEqual(results["command"], ["brew", "install", "bb-cli"])
        self.assertEqual(results["display"], "brew install bb-cli")
        self._assert_never_brew_install_bb(self._plan(results))

    def test_bb_apt_only_is_guidance(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "def which(b):\n"
            "    if b == 'apt-get':\n"
            "        return '/usr/bin/apt-get'\n"
            "    return None\n"
            "with patch.object(m.shutil, 'which', side_effect=which), "
            "patch.object(m.platform, 'system', return_value='Linux'):\n"
            "    plan = m.resolve_install_plan(\n"
            "        'bb', install_url='https://bb-cli.github.io'\n"
            "    )\n"
            "results['kind'] = plan.kind\n"
            "results['command'] = list(plan.command)\n"
            "results['display'] = plan.display\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["kind"], "guidance")
        self.assertEqual(results["command"], [])
        self.assertEqual(results["display"], "https://bb-cli.github.io")
        self._assert_never_brew_install_bb(self._plan(results))

    def test_unknown_binary_guidance_only(self):
        """Binary outside _PACKAGE_MAP / _GUIDANCE_ONLY stays guidance with empty command."""
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "plan = m.resolve_install_plan('totally-unknown-bin')\n"
            "results['kind'] = plan.kind\n"
            "results['command'] = list(plan.command)\n"
            "results['binary'] = plan.binary\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["kind"], "guidance")
        self.assertEqual(results["command"], [])
        self.assertEqual(results["binary"], "totally-unknown-bin")

    def test_brew_plan_when_brew_present(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "with patch.object(\n"
            "    m.shutil, 'which',\n"
            "    side_effect=lambda b: '/opt/brew' if b == 'brew' else None\n"
            "), patch.object(m.platform, 'system', return_value='Darwin'):\n"
            "    plan = m.resolve_install_plan('gh')\n"
            "results['kind'] = plan.kind\n"
            "results['command'] = list(plan.command)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["kind"], "brew")
        self.assertEqual(results["command"], ["brew", "install", "gh"])

    def test_apt_plan_on_linux(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install plans are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "def which(b):\n"
            "    if b == 'apt-get':\n"
            "        return '/usr/bin/apt-get'\n"
            "    return None\n"
            "with patch.object(m.shutil, 'which', side_effect=which), "
            "patch.object(m.platform, 'system', return_value='Linux'):\n"
            "    plan = m.resolve_install_plan('jq')\n"
            "results['command'] = list(plan.command)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["command"][:3], ["sudo", "apt-get", "install"])

    def test_offer_non_tty_noop(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install offers are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "plan = m.InstallPlan(\n"
            "    binary='jq',\n"
            "    command=['brew', 'install', 'jq'],\n"
            "    display='brew install jq',\n"
            "    guidance_url='',\n"
            "    kind='brew',\n"
            ")\n"
            "with patch.object(m.subprocess, 'run') as run:\n"
            "    out = m.offer_and_install([plan], tty=False)\n"
            "results['out'] = list(out)\n"
            "results['ran'] = bool(run.call_args_list)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["out"], [])
        self.assertFalse(results["ran"])

    def test_offer_decline_no_run(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install offers are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "plan = m.InstallPlan(\n"
            "    binary='jq',\n"
            "    command=['brew', 'install', 'jq'],\n"
            "    display='brew install jq',\n"
            "    guidance_url='',\n"
            "    kind='brew',\n"
            ")\n"
            "confirm = MagicMock()\n"
            "confirm.return_value.ask.return_value = False\n"
            "q = MagicMock(confirm=confirm)\n"
            "sys.modules['questionary'] = q\n"
            "with patch.object(m.subprocess, 'run') as run:\n"
            "    out = m.offer_and_install([plan], tty=True)\n"
            "results['out'] = list(out)\n"
            "results['ran'] = bool(run.call_args_list)\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["out"], [])
        self.assertFalse(results["ran"])

    def test_offer_accept_runs_and_rechecks(self):
        project, home = self._workspace()
        # TRIAGE: ai-specs configure-recipes — install offers are TTY-gated
        # consent offers; exercised at the module's process boundary.
        proc = _run_internal(
            project,
            "import dep_install as m\n"
            "plan = m.InstallPlan(\n"
            "    binary='jq',\n"
            "    command=['brew', 'install', 'jq'],\n"
            "    display='brew install jq',\n"
            "    guidance_url='',\n"
            "    kind='brew',\n"
            ")\n"
            "confirm = MagicMock()\n"
            "confirm.return_value.ask.return_value = True\n"
            "q = MagicMock(confirm=confirm)\n"
            "sys.modules['questionary'] = q\n"
            "proc = MagicMock(returncode=0)\n"
            "with patch.object(m.subprocess, 'run', return_value=proc) as run, "
            "patch.object(m.shutil, 'which', return_value='/usr/bin/jq'):\n"
            "    out = m.offer_and_install([plan], tty=True)\n"
            "results['out'] = list(out)\n"
            "results['run_count'] = run.call_count\n",
            home,
            project.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        results = self._result(proc)
        self.assertEqual(results["out"], ["jq"])
        self.assertEqual(results["run_count"], 1)


if __name__ == "__main__":
    unittest.main()
