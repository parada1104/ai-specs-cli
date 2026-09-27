"""Black-box recipe materialization tests: every test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (exit codes, stdout/stderr needles, materialized artifacts
under the per-project cache layout, agent configs, recipe hook markers,
stamped overrides) through the CLI process boundary. Internal surfaces with no
CLI-observable equivalent are kept as process-boundary invocations of the
exact command line sync.sh/sync-agent.sh run against the isolated home's own
lib copy, each marked with a distinct ``# TRIAGE:`` comment.

Staging pattern (proven in test_sync_pipeline.py / test_agents_render_brief_fragments.py):
  - one isolated install root per test with a REAL lib copy (symlinked lib
    would resolve back into the repository and let the CLI touch repo state),
  - catalog materialized into a real dir of per-recipe symlinks, plus the
    internal ``test-*`` fixture recipes symlinked in for recipe tests,
  - per-test temp project; cache observed via ``cache_project_dir`` layout,
  - sync stderr carries materialize warnings (FROZEN compact filter drops
    only blank lines and lines starting with ✓ · ⇢ ▸ from stdout).
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import (  # noqa: E402
    CLIResult,
    cache_project_dir,
    invoke,
    isolated_home,
    normalize_output,
)
from _fixture_catalog import FIXTURE_RECIPES  # noqa: E402

KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"


# ---------------------------------------------------------------------------
# Staging helpers
# ---------------------------------------------------------------------------

def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def _materialize_catalog(home: Path) -> Path:
    """Replace the home's catalog symlink with a real dir of per-recipe links.

    Returns the recipes dir. Test recipes are then seeded as REAL dirs, so no
    test ever writes through a symlink into the repository's own catalog.
    """
    catalog = home / "catalog"
    recipes = catalog / "recipes"
    if not catalog.is_symlink():
        recipes.mkdir(parents=True, exist_ok=True)
        return recipes
    target = catalog.resolve()
    catalog.unlink()
    catalog.mkdir()
    recipes.mkdir()
    for entry in sorted(target.iterdir()):
        if entry.name == "recipes":
            for recipe in sorted(entry.iterdir()):
                (recipes / recipe.name).symlink_to(recipe)
        else:
            (catalog / entry.name).symlink_to(entry)
    return recipes


def _fixture_home(base: Path) -> Path:
    """Isolated home whose catalog also exposes the internal test-* fixtures."""
    home = _make_home(base)
    recipes = _materialize_catalog(home)
    for child in sorted(FIXTURE_RECIPES.iterdir()):
        if child.is_dir() and not child.name.startswith("."):
            (recipes / child.name).symlink_to(child)
    return home


def _seed_recipe(home: Path, rid: str, toml: str,
                 files: dict[str, str] | None = None) -> Path:
    """Seed (or override) one catalog recipe as a REAL dir in the temp home."""
    recipes = _materialize_catalog(home)
    rdir = recipes / rid
    if rdir.is_symlink():
        rdir.unlink()
    rdir.mkdir(parents=True, exist_ok=True)
    (rdir / "recipe.toml").write_text(toml)
    for rel, content in (files or {}).items():
        path = rdir / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
    return rdir


def _run(project_root: Path, *args: str, home: Path, allow_internal: bool = True,
         append_root: bool = True) -> CLIResult:
    """Run bin/ai-specs hermetically (stdin closed so the CLI can never block).

    Mirrors invoke()'s environment plus AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES
    (opt-out via allow_internal=False) and the offline vendor fixture root.
    """
    tmpdir = project_root.parent
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(tmpdir / "home"),
        "TMPDIR": str(tmpdir),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        "LC_ALL": "C",
        "LANG": "C",
    }
    if allow_internal:
        env["AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES"] = "1"
    (tmpdir / "home").mkdir(parents=True, exist_ok=True)
    argv = [str(ROOT / "bin" / "ai-specs"), *args]
    if append_root:
        argv.append(str(project_root))
    proc = subprocess.run(argv, cwd=ROOT, env=env, text=True,
                          capture_output=True, check=False, input="")
    return CLIResult(
        proc.returncode,
        normalize_output(proc.stdout, (project_root, home, tmpdir)),
        normalize_output(proc.stderr, (project_root, home, tmpdir)),
    )


def _materialize_process(project_root: Path, home: Path, *extra: str,
                         allow_internal: bool = True) -> CLIResult:
    """Run the EXACT recipe-materialize command line sync.sh runs.

    Used only by ``# TRIAGE:`` tests whose contract is observable solely at
    the materialize process boundary (resolved-config/hooks JSON files that
    no verb exposes at a predictable path). Runs the isolated home's own lib
    copy — never a repository import.
    """
    tmpdir = project_root.parent
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(tmpdir / "home"),
        "TMPDIR": str(tmpdir),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        "LC_ALL": "C",
        "LANG": "C",
    }
    if allow_internal:
        env["AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES"] = "1"
    (tmpdir / "home").mkdir(parents=True, exist_ok=True)
    argv = [
        sys.executable, str(home / "lib" / "_internal" / "recipe-materialize.py"),
        str(project_root), str(home), *extra,
    ]
    proc = subprocess.run(argv, cwd=ROOT, env=env, text=True,
                          capture_output=True, check=False, input="")
    return CLIResult(
        proc.returncode,
        normalize_output(proc.stdout, (project_root, home, tmpdir)),
        normalize_output(proc.stderr, (project_root, home, tmpdir)),
    )


# Cache layout accessors (frozen key: sha256(realpath)[:12]-basename).
def _recipe_skill(project_root: Path, home: Path, rid: str, sid: str) -> Path:
    return cache_project_dir(project_root, home) / ".recipe" / rid / "skills" / sid


def _recipe_marker(project_root: Path, home: Path, rid: str) -> Path:
    return cache_project_dir(project_root, home) / ".recipe" / rid / "bootstrap-ready"


def _cache_command(project_root: Path, home: Path, cmd_id: str) -> Path:
    return cache_project_dir(project_root, home) / "commands" / f"{cmd_id}.md"


class RecipeMaterializeTests(unittest.TestCase):
    def setUp(self):
        self._tmpdirs: list[tempfile.TemporaryDirectory] = []

    def _new_base(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _project(self, recipe_section: str = "", mcp_section: str = "",
                 home: Path | None = None) -> tuple[Path, Path]:
        """Temp project (manifest + ai-specs dirs) with an isolated home."""
        base = self._new_base()
        root = base / "project"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + mcp_section
            + recipe_section
            + "\n"
        )
        return root, home or _fixture_home(base)

    def _sync(self, root: Path, home: Path, allow_internal: bool = True) -> CLIResult:
        return _run(root, "sync", home=home, allow_internal=allow_internal)

    # --- materialized artifacts through `ai-specs sync` ----------------------

    def test_materializes_bundled_skill(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        skill_dir = _recipe_skill(root, home, "test-fixture", "test-skill")
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_tag_conflicts_stay_advisory_and_keep_todays_text(self):
        # The bridge only changes who grades tag conflicts; the call site keeps
        # its warning text and never lets an advisory conflict change the exit
        # code. Enable a warning-grade overlap pair AND a fatal-grade
        # conflicts_with pair and pin both messages through `sync`.
        root, home = self._project(
            "[recipes.bb-tag-warn-a]\nenabled = true\n"
            "[recipes.bb-tag-warn-b]\nenabled = true\n"
            "[recipes.bb-tag-flow-a]\nenabled = true\n"
            "[recipes.bb-tag-flow-b]\nenabled = true\n"
        )
        _seed_recipe(home, "bb-tag-warn-a",
                     '[recipe]\nid = "bb-tag-warn-a"\nname = "A"\n'
                     'description = "D"\nversion = "1.0"\ntags = ["vcs"]\n')
        _seed_recipe(home, "bb-tag-warn-b",
                     '[recipe]\nid = "bb-tag-warn-b"\nname = "B"\n'
                     'description = "D"\nversion = "1.0"\ntags = ["vcs"]\n')
        _seed_recipe(home, "bb-tag-flow-a",
                     '[recipe]\nid = "bb-tag-flow-a"\nname = "FA"\n'
                     'description = "D"\nversion = "1.0"\ntags = ["flow"]\n'
                     'conflicts_with = ["bb-tag-flow-b"]\n')
        _seed_recipe(home, "bb-tag-flow-b",
                     '[recipe]\nid = "bb-tag-flow-b"\nname = "FB"\n'
                     'description = "D"\nversion = "1.0"\ntags = ["flow"]\n'
                     'conflicts_with = ["bb-tag-flow-a"]\n')
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        stderr = result.stderr
        self.assertIn(
            "tag overlap: recipes bb-tag-warn-a, bb-tag-warn-b share tag 'vcs' "
            "(same capability category).",
            stderr,
        )
        self.assertIn(
            "tag conflict: recipes bb-tag-flow-a, bb-tag-flow-b share tag 'flow' "
            "and declare an explicit conflicts_with. Review whether both should "
            "be enabled.",
            stderr,
        )

    def test_materializes_command(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        cmd = _cache_command(root, home, "test-command")
        self.assertTrue(cmd.is_file())

    def test_materializes_doc(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        doc = root / "docs" / "test-doc-output.md"
        self.assertTrue(doc.is_file())

    def test_materializes_template_not_exists(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        tpl = root / "docs" / "test-template-output.md"
        self.assertTrue(tpl.is_file())

    def test_skips_template_when_target_exists(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        existing = root / "docs" / "test-template-output.md"
        existing.parent.mkdir(parents=True, exist_ok=True)
        existing.write_text("existing")
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(existing.read_text(), "existing")

    def test_writes_recipe_mcp_json(self):
        # The recipe MCP preset flows through sync into the rendered agent
        # config (.mcp.json for claude): id, command, and args preserved.
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        mcp_path = root / ".mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("test-mcp", data["mcpServers"])
        self.assertEqual(data["mcpServers"]["test-mcp"]["command"], "npx")

    def test_disabled_recipe_skips_materialization(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = false\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(
            (_recipe_skill(root, home, "test-fixture", "test-skill")).exists()
        )

    def test_sync_without_version_succeeds(self):
        root, home = self._project(
            "[recipes.test-fixture]\nenabled = true\n"
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        skill_dir = _recipe_skill(root, home, "test-fixture", "test-skill")
        self.assertTrue(skill_dir.is_dir())

    def test_legacy_version_warns_and_succeeds(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "2.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        stderr_output = result.stderr
        self.assertIn("legacy", stderr_output.lower())
        self.assertIn("version", stderr_output.lower())
        skill_dir = _recipe_skill(root, home, "test-fixture", "test-skill")
        self.assertTrue(skill_dir.is_dir())

    def test_unknown_recipe_fails(self):
        root, home = self._project(
            '[recipes.nonexistent]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("nonexistent", result.stderr)

    def test_no_recipes_section_succeeds(self):
        root, home = self._project("")
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_recipe_does_not_overwrite_user_local_skill(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        # Pre-create a user-local skill with the same ID
        user_skill = root / "ai-specs" / "skills" / "test-skill"
        user_skill.mkdir(parents=True)
        (user_skill / "SKILL.md").write_text("user local")
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        # Local skill is preserved; recipe version goes to the cache
        self.assertEqual((user_skill / "SKILL.md").read_text(), "user local")
        recipe_skill = _recipe_skill(root, home, "test-fixture", "test-skill")
        self.assertTrue(recipe_skill.is_dir())

    def test_v1_manifest_without_bindings_or_config_succeeds(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_materialize_refuses_internal_test_recipe_without_allow_env(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n',
            home=_make_home(self._new_base()),
        )
        result = self._sync(root, home, allow_internal=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("internal test fixture", result.stderr)
        skill = _recipe_skill(root, home, "test-fixture", "test-skill")
        self.assertFalse(skill.exists())

    # --- ai_specs_home forwarding regression --------------------------------

    def _resolved_context(self, root: Path, home: Path) -> dict:
        # TRIAGE: ai-specs sync — no verb exposes the resolved-config JSON at a
        # predictable path, so the home-forwarding contract (the explicit
        # $AI_SPECS_HOME reaches build_resolved_config and is never replaced by
        # a module-default fallback) is pinned at the materialize process
        # boundary using the exact sync.sh command line against the isolated
        # home's own lib copy.
        out = root.parent / "resolved-config.json"
        result = _materialize_process(root, home, "--resolved-config-out", str(out))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(out.is_file())
        return json.loads(out.read_text())

    def test_materialize_forwards_ai_specs_home_when_no_recipes_enabled(self):
        root, home = self._project("")
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = self._resolved_context(root, home)
        self.assertEqual(data["project_root"], str(root.resolve()))
        self.assertEqual(data["enabled"], [])

    def test_materialize_forwards_ai_specs_home_when_recipes_enabled(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = self._resolved_context(root, home)
        self.assertEqual(data["project_root"], str(root.resolve()))
        self.assertIn("test-fixture", data["enabled"])

    # --- V2 binding resolution -----------------------------------------------

    def _cap_recipe(self, home: Path, rid: str, caps: tuple[str, ...] = ()) -> None:
        cap_lines = "".join(f'\n[[capabilities]]\nid = "{c}"\n' for c in caps)
        _seed_recipe(home, rid,
                     f'[recipe]\nid = "{rid}"\nname = "{rid.title()}"\n'
                     f'description = "D"\nversion = "1.0"\n{cap_lines}')

    def _resolved_bindings(self, root: Path, home: Path) -> dict:
        # TRIAGE: ai-specs sync — the resolved capability→recipe binding map is
        # only observable in the resolved-config JSON (no verb prints it); the
        # binding contract is pinned at the materialize process boundary using
        # the exact sync.sh command line against the isolated home's own lib
        # copy.
        out = root.parent / "resolved-config.json"
        result = _materialize_process(root, home, "--resolved-config-out", str(out))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return json.loads(out.read_text())["bindings"]

    def test_resolve_bindings_explicit(self):
        root, home = self._project(
            "[recipes.recipe-a]\nenabled = true\n\n"
            "[[bindings]]\ncapability = 'tracker'\nrecipe = 'recipe-a'\n"
        )
        self._cap_recipe(home, "recipe-a", caps=["tracker"])
        self.assertEqual(self._resolved_bindings(root, home), {"tracker": "recipe-a"})

    def test_resolve_bindings_auto_bind_single_provider(self):
        root, home = self._project("[recipes.recipe-a]\nenabled = true\n")
        self._cap_recipe(home, "recipe-a", caps=["tracker"])
        self.assertEqual(self._resolved_bindings(root, home), {"tracker": "recipe-a"})

    def test_resolve_bindings_auto_bind_skips_ambiguity(self):
        root, home = self._project(
            "[recipes.recipe-a]\nenabled = true\n[recipes.recipe-b]\nenabled = true\n"
        )
        self._cap_recipe(home, "recipe-a", caps=["tracker"])
        self._cap_recipe(home, "recipe-b", caps=["tracker"])
        bindings = self._resolved_bindings(root, home)
        self.assertNotIn("tracker", bindings)

    def test_resolve_bindings_explicit_disabled_fails(self):
        root, home = self._project(
            "[recipes.recipe-a]\nenabled = true\n\n"
            "[[bindings]]\ncapability = 'tracker'\nrecipe = 'recipe-b'\n"
        )
        self._cap_recipe(home, "recipe-a", caps=["tracker"])
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("disabled/unknown", result.stderr)

    def test_resolve_bindings_duplicate_explicit_fails(self):
        root, home = self._project(
            "[recipes.recipe-a]\nenabled = true\n[recipes.recipe-b]\nenabled = true\n\n"
            "[[bindings]]\ncapability = 'tracker'\nrecipe = 'recipe-a'\n\n"
            "[[bindings]]\ncapability = 'tracker'\nrecipe = 'recipe-b'\n"
        )
        self._cap_recipe(home, "recipe-a", caps=["tracker"])
        self._cap_recipe(home, "recipe-b", caps=["tracker"])
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("duplicate explicit binding", result.stderr)

    # --- V2 config merge ------------------------------------------------------

    def _config_recipe(self, home: Path, rid: str = "cfg-recipe", *,
                       hooks: str = "") -> None:
        """Seed a recipe with a board_id/default_list/epic_list config schema."""
        _seed_recipe(home, rid,
                     f'[recipe]\nid = "{rid}"\nname = "CR"\n'
                     f'description = "D"\nversion = "1.0"\n\n'
                     '[config.board_id]\nrequired = true\ntype = "string"\n\n'
                     '[config.default_list]\nrequired = false\ntype = "string"\n'
                     'default = "In Progress"\n\n'
                     '[config.epic_list]\nrequired = false\ntype = "string"\n'
                     'default = "Epic"\n\n' + hooks)

    def test_merge_config_defaults_and_override(self):
        # Schema defaults fill absent keys; manifest values win. The merged
        # config is observable through the bootstrap-board marker content.
        root, home = self._project(
            "[recipes.cfg-recipe]\nenabled = true\n"
            "[recipes.cfg-recipe.config]\nboard_id = 'abc123'\n"
        )
        self._config_recipe(
            home, hooks='[[hooks]]\nevent = "on-sync"\naction = "bootstrap-board"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        marker = _recipe_marker(root, home, "cfg-recipe")
        self.assertTrue(marker.is_file())
        # board_id overridden from the manifest; default_list/epic_list come
        # from the catalog recipe's own schema defaults.
        self.assertEqual(
            marker.read_text(),
            "board_id=abc123\ndefault_list=In Progress\nepic_list=Epic\n",
        )

    def test_merge_config_missing_required_fails(self):
        root, home = self._project("[recipes.cfg-recipe]\nenabled = true\n")
        self._config_recipe(home)
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    def test_merge_config_warns_on_unknown_key(self):
        root, home = self._project(
            "[recipes.cfg-recipe]\nenabled = true\n"
            "[recipes.cfg-recipe.config]\n"
            "board_id = 'abc123'\nunknown = 1\n"
        )
        self._config_recipe(home)
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("unknown config key 'unknown'", result.stderr)

    def test_merge_config_carries_declared_reconcile_table(self):
        # A declared [config.reconcile] structured table passes through the
        # merge unflagged: sync succeeds and no unknown-key warning appears.
        root, home = self._project(
            "[recipes.reconcile-recipe]\nenabled = true\n"
            "[recipes.reconcile-recipe.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
            "[recipes.reconcile-recipe.config.reconcile]\n"
            "scope_field = 'board_id'\n"
            "max_age_seconds = 900\n"
            "[[recipes.reconcile-recipe.config.reconcile.expectations]]\n"
            "event = 'delivery'\nproperty = 'list'\nconfig_field = 'default_list'\n"
        )
        _seed_recipe(
            home, "reconcile-recipe",
            '[recipe]\nid = "reconcile-recipe"\nname = "Reconcile"\n'
            'description = "D"\nversion = "1.0"\n\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n\n'
            '[config.reconcile]\n'
            'scope_field = "board_id"\n'
            'max_age_seconds = 900\n\n'
            '[[config.reconcile.expectations]]\n'
            'event = "delivery"\nproperty = "list"\nconfig_field = "default_list"\n',
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("unknown config key", result.stderr)

    def test_merge_config_rejects_malformed_reconcile_table(self):
        root, home = self._project(
            "[recipes.reconcile-recipe]\nenabled = true\n"
            "[recipes.reconcile-recipe.config]\n"
            "board_id = 'abc123'\n"
            "[recipes.reconcile-recipe.config.reconcile]\n"
            "max_age_seconds = 'soon'\n"
        )
        _seed_recipe(
            home, "reconcile-recipe",
            '[recipe]\nid = "reconcile-recipe"\nname = "Reconcile"\n'
            'description = "D"\nversion = "1.0"\n\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n\n'
            '[config.reconcile]\n'
            'scope_field = "board_id"\n'
            'max_age_seconds = 900\n',
        )
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("reconcile", result.stderr)
        self.assertIn("integer", result.stderr)

    # --- Hook execution -------------------------------------------------------

    def _seed_hook_recipe(self, home: Path, rid: str, fields, hooks) -> None:
        """Seed a recipe with a config schema and on-sync hooks.

        fields: (name, required, default, regex) tuples; hooks: action names.
        """
        toml = (f'[recipe]\nid = "{rid}"\nname = "R"\ndescription = "D"\n'
                f'version = "1.0"\n\n')
        for name, required, default, regex in fields:
            toml += (f'[config.{name}]\n'
                     f'required = {"true" if required else "false"}\n'
                     f'type = "string"\n')
            if default is not None:
                toml += f'default = "{default}"\n'
            if regex:
                toml += f'[config.{name}.validation]\nregex = "{regex}"\n'
        for action in hooks:
            toml += f'\n[[hooks]]\nevent = "on-sync"\naction = "{action}"\n'
        _seed_recipe(home, rid, toml)

    def test_execute_hooks_validate_config_success(self):
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nkey = 'value'\n"
        )
        self._seed_hook_recipe(home, "hk",
                               [("key", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_execute_hooks_validate_config_fails(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk",
                               [("key", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        # The required-field contract fails the sync; the message names the
        # hook layer's contract (missing required config field 'key').
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("key", result.stderr)

    def test_execute_hooks_unknown_action_warns(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk", [], ["unknown"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("unknown hook action 'unknown'", result.stderr)

    def test_end_to_end_v2_recipe_with_config_and_hooks(self):
        root, home = self._project(
            "[recipes.v2-recipe]\nenabled = true\n"
            "[recipes.v2-recipe.config]\nboard_id = 'abc123'\n"
        )
        _seed_recipe(
            home, "v2-recipe",
            '[recipe]\nid = "v2-recipe"\nname = "V2 Recipe"\n'
            'description = "D"\nversion = "1.0"\n\n'
            '[[capabilities]]\nid = "tracker"\n\n'
            '[[hooks]]\nevent = "on-sync"\naction = "validate-config"\n\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n\n'
            '[[provides.skills]]\nid = "v2-skill"\nsource = "bundled"\n\n'
            '[[provides.commands]]\nid = "v2-cmd"\npath = "commands/v2-cmd.md"\n\n'
            '[[provides.docs]]\nsource = "docs/doc.md"\ntarget = "docs/v2-doc-output.md"\n',
            files={
                "skills/v2-skill/SKILL.md": "skill",
                "commands/v2-cmd.md": "cmd",
                "docs/doc.md": "doc",
            },
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        skill = _recipe_skill(root, home, "v2-recipe", "v2-skill")
        self.assertTrue(skill.is_dir())
        self.assertTrue((skill / "SKILL.md").is_file())
        self.assertTrue(_cache_command(root, home, "v2-cmd").is_file())
        self.assertTrue((root / "docs" / "v2-doc-output.md").is_file())

    # --- MCP preset merge safety ---------------------------------------------

    def test_mcp_preset_manifest_precedence_on_conflict(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n',
            mcp_section='[mcp.test-mcp]\ncommand = "custom-cmd"\nargs = ["--flag"]\n',
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = json.loads((root / ".mcp.json").read_text())
        self.assertIn("test-mcp", data["mcpServers"])
        # Manifest value must win
        self.assertEqual(data["mcpServers"]["test-mcp"]["command"], "custom-cmd")
        self.assertEqual(data["mcpServers"]["test-mcp"]["args"], ["--flag"])

    def test_mcp_preset_recipe_creates_when_not_in_manifest(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = json.loads((root / ".mcp.json").read_text())
        self.assertIn("test-mcp", data["mcpServers"])
        self.assertEqual(data["mcpServers"]["test-mcp"]["command"], "npx")
        self.assertEqual(data["mcpServers"]["test-mcp"]["args"],
                         ["-y", "@test/mcp-server"])

    def test_mcp_preset_merge_warns_on_conflict(self):
        root, home = self._project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n',
            mcp_section='[mcp.test-mcp]\ncommand = "custom-cmd"\n',
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("conflicts with project manifest", result.stderr)

    # --- Hook execution: bootstrap-board --------------------------------------

    # Config fields must be declared in the recipe schema for manifest values
    # to survive the merge (undeclared keys are dropped with a warning).
    _BOARD_FIELDS = [
        ("board_id", False, None, ""),
        ("default_list", False, None, ""),
        ("epic_list", False, None, ""),
    ]

    def test_execute_hooks_bootstrap_board_creates_marker(self):
        root, home = self._project(
            "[recipes.bb]\nenabled = true\n"
            "[recipes.bb.config]\n"
            "board_id = 'test-board-123'\n"
            "default_list = 'In Progress'\n"
            "epic_list = 'Epic'\n"
        )
        self._seed_hook_recipe(home, "bb", self._BOARD_FIELDS, ["bootstrap-board"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        marker = _recipe_marker(root, home, "bb")
        self.assertTrue(marker.is_file())
        content = marker.read_text()
        self.assertIn("board_id=test-board-123", content)
        self.assertIn("default_list=In Progress", content)
        self.assertIn("epic_list=Epic", content)

    def test_execute_hooks_bootstrap_board_marker_content(self):
        root, home = self._project(
            "[recipes.myrecipe]\nenabled = true\n"
            "[recipes.myrecipe.config]\n"
            "board_id = 'b1'\ndefault_list = 'Todo'\nepic_list = 'Backlog'\n"
        )
        self._seed_hook_recipe(home, "myrecipe", self._BOARD_FIELDS, ["bootstrap-board"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        marker = _recipe_marker(root, home, "myrecipe")
        self.assertEqual(marker.read_text(),
                         "board_id=b1\ndefault_list=Todo\nepic_list=Backlog\n")

    def test_execute_hooks_bootstrap_board_missing_board_id(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk",
                               [("board_id", True, None, "")],
                               ["validate-config", "bootstrap-board"])
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    # --- Hook execution: deferred hooks ---------------------------------------

    def test_execute_hooks_deferred_link_trello_card(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk", [], ["link-trello-card"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("link-trello-card", result.stdout)
        self.assertIn("deferred", result.stdout)

    def test_execute_hooks_deferred_sync_card_state(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk", [], ["sync-card-state"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("sync-card-state", result.stdout)
        self.assertIn("deferred", result.stdout)

    def test_execute_hooks_deferred_comment_verification(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk", [], ["comment-verification"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("comment-verification", result.stdout)
        self.assertIn("deferred", result.stdout)

    # --- Hook execution: project_root parameter -------------------------------

    def test_execute_hooks_project_root_used_by_bootstrap_board(self):
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'b1'\n"
        )
        self._seed_hook_recipe(home, "hk", self._BOARD_FIELDS[:1], ["bootstrap-board"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        marker = _recipe_marker(root, home, "hk")
        self.assertTrue(marker.is_file())
        self.assertIn("board_id=b1", marker.read_text())

    def test_execute_hooks_project_root_different_paths(self):
        root1, home1 = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\n"
            "board_id = 'board-1'\ndefault_list = 'List1'\nepic_list = 'Epic1'\n"
        )
        root2, home2 = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\n"
            "board_id = 'board-2'\ndefault_list = 'List2'\nepic_list = 'Epic2'\n"
        )
        self._seed_hook_recipe(home1, "hk", self._BOARD_FIELDS, ["bootstrap-board"])
        self._seed_hook_recipe(home2, "hk", self._BOARD_FIELDS, ["bootstrap-board"])
        result1 = self._sync(root1, home1)
        result2 = self._sync(root2, home2)
        self.assertEqual(result1.returncode, 0, result1.stdout + result1.stderr)
        self.assertEqual(result2.returncode, 0, result2.stdout + result2.stderr)
        m1 = _recipe_marker(root1, home1, "hk")
        m2 = _recipe_marker(root2, home2, "hk")
        self.assertTrue(m1.is_file())
        self.assertTrue(m2.is_file())
        self.assertIn("board_id=board-1", m1.read_text())
        self.assertIn("board_id=board-2", m2.read_text())

    # --- Config validation: board_id / optional fields -----------------------

    def test_config_validation_board_id_required(self):
        root, home = self._project("[recipes.hk]\nenabled = true\n")
        self._seed_hook_recipe(home, "hk",
                               [("board_id", True, None, "")],
                               ["validate-config"])
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    def test_config_validation_default_list_optional(self):
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'b1'\n"
        )
        self._seed_hook_recipe(
            home, "hk",
            [("board_id", True, None, ""),
             ("default_list", False, "In Progress", "")],
            ["validate-config"],
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_config_validation_epic_list_optional(self):
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'b1'\n"
        )
        self._seed_hook_recipe(
            home, "hk",
            [("board_id", True, None, ""),
             ("epic_list", False, "Epic", "")],
            ["validate-config"],
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    # --- Regex validation in validate-config hook -----------------------------

    def test_regex_validation_pass(self):
        # Valid 24-char hex string
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = '69ec0a2099ea20956e371d62'\n"
        )
        self._seed_hook_recipe(
            home, "hk",
            [("board_id", True, None, "^[a-f0-9]{24}$")],
            ["validate-config"],
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_regex_validation_fail(self):
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'not-a-valid-board-id'\n"
        )
        self._seed_hook_recipe(
            home, "hk",
            [("board_id", True, None, "^[a-f0-9]{24}$")],
            ["validate-config"],
        )
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not match required pattern", result.stderr)
        self.assertIn("board_id", result.stderr)
        self.assertIn("not-a-valid-board-id", result.stderr)

    def test_regex_validation_missing_validation_dict(self):
        # Should not raise even though board_id has no regex
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'valid-board-id-123'\n"
        )
        self._seed_hook_recipe(
            home, "hk", [("board_id", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_regex_validation_empty_pattern(self):
        # Pattern is empty string — regex should be skipped
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'valid-board-id-123'\n"
        )
        self._seed_hook_recipe(
            home, "hk", [("board_id", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_shortlink_detection_on_board_id(self):
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = 'AbCd1234'\n"
        )
        self._seed_hook_recipe(
            home, "hk", [("board_id", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        # 8 alphanumeric chars looks like a Trello shortLink
        self.assertIn("shortLink", result.stderr)
        self.assertIn("24 hex characters", result.stderr)

    def test_shortlink_allows_24_hex_board_id(self):
        # Full 24 hex char board ID should pass shortLink detection
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nboard_id = '69ec0a2099ea20956e371d62'\n"
        )
        self._seed_hook_recipe(
            home, "hk", [("board_id", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_shortlink_non_board_id_field_ignored(self):
        # 8 chars on a non-board_id field should not trigger shortLink error
        root, home = self._project(
            "[recipes.hk]\nenabled = true\n"
            "[recipes.hk.config]\nother_field = 'AbCd1234'\n"
        )
        self._seed_hook_recipe(
            home, "hk", [("other_field", True, None, "")], ["validate-config"])
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    # --- Integration: trello-mcp-workflow recipe materialization --------------

    def test_materialize_trello_mcp_workflow_recipe(self):
        root, home = self._project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\nboard_id = 'abc123'\n"
        )
        # Seed the full recipe into the isolated home's catalog (real dir).
        _seed_recipe(
            home, "trello-mcp-workflow",
            '[recipe]\n'
            'id = "trello-mcp-workflow"\n'
            'name = "Trello MCP Workflow"\n'
            'description = "Trello-based project tracking"\n'
            'version = "1.0"\n'
            '[[capabilities]]\nid = "tracker"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "validate-config"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "bootstrap-board"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "link-trello-card"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "sync-card-state"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "comment-verification"\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n'
            '[config.default_list]\nrequired = false\ntype = "string"\n'
            'default = "In Progress"\n'
            '[config.epic_list]\nrequired = false\ntype = "string"\n'
            'default = "Epic"\n'
            '[[provides.skills]]\nid = "trello-pm-workflow"\nsource = "bundled"\n',
            files={"skills/trello-pm-workflow/SKILL.md": "# Trello PM Workflow\n"},
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        skill = _recipe_skill(root, home, "trello-mcp-workflow", "trello-pm-workflow")
        self.assertTrue(skill.is_dir())
        self.assertTrue((skill / "SKILL.md").is_file())
        marker = _recipe_marker(root, home, "trello-mcp-workflow")
        self.assertTrue(marker.is_file())
        marker_content = marker.read_text()
        self.assertIn("board_id=abc123", marker_content)


class ResolvedConfigContextTests(unittest.TestCase):
    """2.3 — resolved-config carries project_root and topology context."""

    def _project(self, *, topology: str | None = None,
                 project_topology: str | None = None) -> tuple[Path, Path]:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        root = base / "project"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        text = "[project]\nname = 'ctx'\n"
        if project_topology is not None:
            text += f"repo_topology = '{project_topology}'\n"
        text += "\n[agents]\nenabled = ['claude']\n"
        if topology is not None:
            text += (
                "[recipes.worktree-flow]\nenabled = true\n"
                "[recipes.worktree-flow.config]\n"
                f"repo_topology = '{topology}'\n"
            )
        (root / "ai-specs" / "ai-specs.toml").write_text(text)
        home = _make_home(base)
        return root, home

    def _resolved(self, root: Path, home: Path, out_name: str) -> dict:
        # TRIAGE: ai-specs sync — no verb exposes the resolved-config JSON at a
        # predictable path; the context-carried-by-resolved-config contract is
        # pinned at the materialize process boundary using the exact sync.sh
        # command line against the isolated home's own lib copy. The CLI sync
        # run itself is asserted separately where the original asserted rc 0.
        out = root.parent / out_name
        result = _materialize_process(root, home, "--resolved-config-out", str(out))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(out.is_file())
        return json.loads(out.read_text())

    def test_resolved_config_carries_project_root(self):
        root, home = self._project()
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = self._resolved(root, home, "resolved.json")
        self.assertEqual(data["project_root"], str(root.resolve()))

    def test_resolved_config_carries_stable_monorepo_apps_topology(self):
        root, home = self._project(topology="monorepo-apps")
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = self._resolved(root, home, "resolved.json")
        self.assertEqual(data["topology"]["resolved"], "monorepo-apps")
        self.assertEqual(data["topology"]["via"], "config")

    def test_resolved_config_only_also_carries_context(self):
        root, home = self._project()
        # The --resolved-config-only surface is what sync-agent.sh runs for
        # standalone targets; pin the same context keys through it.
        out = root.parent / "resolved-only.json"
        result = _materialize_process(
            root, home, "--resolved-config-out", str(out), "--resolved-config-only",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(out.is_file())
        data = json.loads(out.read_text())
        self.assertEqual(data["project_root"], str(root.resolve()))
        self.assertIn("topology", data)

    def test_project_field_wins_in_resolved_config(self):
        root, home = self._project(
            topology="standalone", project_topology="monorepo-apps"
        )
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = self._resolved(root, home, "resolved.json")
        self.assertEqual(data["topology"]["resolved"], "monorepo-apps")
        self.assertEqual(data["topology"]["configured"], "monorepo-apps")
        self.assertEqual(data["topology"]["source"], "project")

    def test_stamps_project_topology_into_gate_and_cleanup(self):
        root, home = self._project(
            topology="standalone", project_topology="monorepo-apps"
        )
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        hook = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "hooks"
            / "worktree-gate.sh"
        )
        self.assertIn('stamped_repo_topology="monorepo-apps"', hook.read_text())
        cleanup = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )
        self.assertIn('stamped_repo_topology="monorepo-apps"', cleanup.read_text())


class RuntimeHookMaterializeTests(unittest.TestCase):
    def _build(self, *, config_section: str = "", config_override: str = ""):
        """Build a temp home (catalog) + project enabling a runtime-hook recipe.

        Returns (project_root, home, recipe_id).
        """
        base = self._new_base()
        home = _make_home(base)
        rid = "wt-hook"
        _seed_recipe(
            home, rid,
            '[recipe]\n'
            f'id = "{rid}"\n'
            'name = "WT Hook"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            f'{config_section}'
            '[[provides.hooks]]\n'
            'id = "gate"\n'
            'event = "pre-tool-use"\n'
            'script = "hooks/gate.sh"\n'
            'matcher = "Edit|Write"\n'
            'blocking = true\n',
            files={"hooks/gate.sh": "#!/usr/bin/env bash\nexit 0\n"},
        )
        root = base / "project"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{rid}]\nenabled = true\nversion = '1.0'\n"
            f"{config_override}"
        )
        return root, home, rid

    def _new_base(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def test_script_materialized_executable_at_neutral_path(self):
        project_root, home, rid = self._build()
        result = _run(project_root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        script = project_root / "ai-specs" / "recipes" / rid / "hooks" / "gate.sh"
        self.assertTrue(script.is_file(), "hook script should materialize at neutral path")
        self.assertTrue(os.access(script, os.X_OK), "hook script should be executable")

    def test_resolved_hooks_out_shape(self):
        project_root, home, rid = self._build()
        # The resolved-hooks JSON shape is what sync.sh hands to hooks-render;
        # pin it at the materialize process boundary.
        # TRIAGE: ai-specs sync — no verb exposes the resolved-hooks JSON at a
        # predictable path; pinned with the exact sync.sh materialize command
        # line against the isolated home's own lib copy.
        out = project_root.parent / "hooks.json"
        result = _materialize_process(project_root, home, "--resolved-hooks-out", str(out))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = json.loads(out.read_text())
        self.assertIn("claude", data["enabled_agents"])
        self.assertEqual(len(data["hooks"]), 1)
        h = data["hooks"][0]
        self.assertEqual(h["recipe"], rid)
        self.assertEqual(h["id"], "gate")
        self.assertEqual(h["event"], "pre-tool-use")
        self.assertEqual(h["matcher"], "Edit|Write")
        self.assertEqual(h["blocking"], True)
        self.assertEqual(
            h["script_path"], f"ai-specs/recipes/{rid}/hooks/gate.sh"
        )
        # End-to-end through the CLI: the hook is wired into the claude agent
        # config with the neutral script path and its managed identity.
        sync = _run(project_root, "sync", home=home)
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        settings = json.loads((project_root / ".claude" / "settings.json").read_text())
        entries = settings["hooks"]["PreToolUse"]
        managed = [e for e in entries
                   if e.get("_ai_specs_managed") == f"ai-specs:hooks:{rid}:gate"]
        self.assertEqual(len(managed), 1)
        self.assertEqual(managed[0]["matcher"], "Edit|Write")
        self.assertEqual(
            managed[0]["hooks"][0]["command"],
            f"$CLAUDE_PROJECT_DIR/ai-specs/recipes/{rid}/hooks/gate.sh",
        )

    def test_resolved_hooks_env_carries_config(self):
        project_root, home, rid = self._build(
            config_section=(
                '[config.WORKTREE_GATE_PROTECTED]\n'
                'required = false\n'
                'type = "string"\n'
                'default = "main"\n'
                '[config.worktrees_dir]\n'
                'required = false\n'
                'type = "string"\n'
                'default = ".worktrees"\n'
            ),
            config_override=(
                "[recipes.wt-hook.config]\nWORKTREE_GATE_PROTECTED = 'main development'\n"
            ),
        )
        # TRIAGE: ai-specs sync — the ENV-shaped config export contract is only
        # observable in the resolved-hooks JSON; pinned with the exact sync.sh
        # materialize command line against the isolated home's own lib copy.
        out = project_root.parent / "hooks.json"
        result = _materialize_process(project_root, home, "--resolved-hooks-out", str(out))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = json.loads(out.read_text())
        env = data["hooks"][0]["env"]
        # ENV-shaped config keys are exported...
        self.assertEqual(env.get("WORKTREE_GATE_PROTECTED"), "main development")
        # ...recipe-internal lowercase keys are not.
        self.assertNotIn("worktrees_dir", env)
        # End-to-end through the CLI: the exported env reaches the claude wiring.
        sync = _run(project_root, "sync", home=home)
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        settings = json.loads((project_root / ".claude" / "settings.json").read_text())
        entries = settings["hooks"]["PreToolUse"]
        managed = [e for e in entries
                   if e.get("_ai_specs_managed") == f"ai-specs:hooks:{rid}:gate"]
        self.assertEqual(
            managed[0]["hooks"][0]["env"].get("WORKTREE_GATE_PROTECTED"),
            "main development",
        )
        self.assertNotIn("worktrees_dir", managed[0]["hooks"][0]["env"])


class FragmentsToJsonTests(unittest.TestCase):
    """2.1 — recipe brief fragments render section-by-section through sync.

    The original suite pinned the internal _fragments_to_json JSON shape;
    the CLI-observable equivalent is the rendered AGENTS.md: populated
    sections appear with their texts (and keys for mcp_descriptions), while
    absent (None) sections render nothing at all.
    """

    def _new_base(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _seed_and_sync(self, brief_toml: str, *, rid: str = "frag-recipe",
                       mcp_section: str = "") -> tuple[Path, str]:
        base = self._new_base()
        home = _make_home(base)
        _seed_recipe(
            home, rid,
            f'[recipe]\nid = "{rid}"\nname = "Frag"\ndescription = "D"\n'
            f'version = "1.0"\n{brief_toml}',
        )
        root = base / "project"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'frag'\n\n[agents]\nenabled = ['claude']\n\n"
            f"{mcp_section}[recipes.{rid}]\nenabled = true\n"
        )
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = (root / "AGENTS.md")
        self.assertTrue(agents.is_file())
        return root, agents.read_text()

    def test_none_input_returns_empty_dict(self):
        # No [provides.brief] → no fragments → no section rendered at all.
        _, agents = self._seed_and_sync("")
        self.assertNotIn("## Workflow Rules", agents)
        self.assertNotIn("## Runtime Flow", agents)

    def test_only_workflow_rules_populated(self):
        _, agents = self._seed_and_sync(
            '[provides.brief]\nworkflow_rules = ["Do X."]\n'
        )
        self.assertIn("## Workflow Rules", agents)
        self.assertIn("Do X.", agents)

    def test_key_set_in_output(self):
        # A keyed mcp_descriptions fragment renders as the description for its
        # MCP id — the key survives into the rendered output.
        _, agents = self._seed_and_sync(
            '[provides.brief]\n'
            'mcp_descriptions = [{ key = "foo", text = "Context here." }]\n'
        )
        self.assertIn("## Runtime MCPs", agents)
        self.assertIn("foo", agents)
        self.assertIn("Context here.", agents)

    def test_none_sections_omitted(self):
        # Only the populated section renders; the None sections are omitted.
        _, agents = self._seed_and_sync(
            '[provides.brief]\nworkflow_rules = ["Rule."]\n'
        )
        self.assertIn("## Workflow Rules", agents)
        self.assertIn("Rule.", agents)
        # Only fragment-driven sections render; CLI-default sections (Useful
        # Commands) always render and are not fragment evidence.
        for section in ("## Runtime Flow", "## Context Sources",
                        "## Conflict Policy", "## Runtime MCPs"):
            self.assertNotIn(section, agents, f"Expected {section} to be omitted")

    def test_all_sections_populated(self):
        _, agents = self._seed_and_sync(
            '[provides.brief]\n'
            'runtime_flow = ["Flow."]\n'
            'context_sources = [{ key = "k1", text = "Ctx." }]\n'
            'conflict_policy = ["Policy."]\n'
            'workflow_rules = ["Rule."]\n'
            'useful_commands = ["Cmd."]\n'
            'mcp_descriptions = [{ key = "srv", text = "Desc." }]\n'
        )
        self.assertIn("Flow.", agents)
        self.assertIn("Ctx.", agents)
        self.assertIn("Policy.", agents)
        self.assertIn("Rule.", agents)
        self.assertIn("Cmd.", agents)
        self.assertIn("Desc.", agents)
        for section in ("## Runtime Flow", "## Context Sources",
                        "## Conflict Policy", "## Workflow Rules",
                        "## Useful Commands", "## Runtime MCPs"):
            self.assertIn(section, agents)


class BriefFragmentsMaterializeIntegrationTests(unittest.TestCase):
    """2.3 — brief_fragments attached in both materialize paths."""

    def _new_base(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _stage(self, base: Path, rid: str, brief_toml: str) -> tuple[Path, Path]:
        home = _make_home(base)
        _seed_recipe(
            home, rid,
            f'[recipe]\nid = "{rid}"\nname = "{rid.title()}"\n'
            f'description = "D"\nversion = "1.0"\n{brief_toml}',
        )
        root = base / "project"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{rid}]\nenabled = true\nversion = \"1.0\"\n"
        )
        return root, home

    def test_materialize_attaches_brief_fragments_for_recipe_with_brief(self):
        # sync path: recipe with [provides.brief] → fragments render into AGENTS.md.
        base = self._new_base()
        root, home = self._stage(
            base, "my-recipe",
            '[provides.brief]\nworkflow_rules = ["Do not push to main directly."]\n',
        )
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("## Workflow Rules", agents)
        self.assertIn("Do not push to main directly.", agents)

    def test_materialize_no_brief_fragments_key_absent_for_recipe_without_brief(self):
        # sync path: recipe without [provides.brief] → no fragment rendering.
        base = self._new_base()
        root, home = self._stage(base, "no-brief-recipe", "")
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertNotIn("## Workflow Rules", agents)

    def test_build_resolved_config_only_attaches_brief_fragments(self):
        # TRIAGE: ai-specs sync-agent — the --resolved-config-only surface runs
        # inside sync-agent on a mktemp path no verb exposes; the
        # brief_fragments-in-resolved-config contract is pinned with the exact
        # sync-agent.sh materialize command line against the isolated home's
        # own lib copy.
        base = self._new_base()
        root, home = self._stage(
            base, "brief-recipe",
            '[provides.brief]\nworkflow_rules = ["A rule from brief-recipe."]\n',
        )
        out = root.parent / "resolved-config.json"
        result = _materialize_process(
            root, home, "--resolved-config-out", str(out), "--resolved-config-only",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(out.is_file())
        data = json.loads(out.read_text())
        recipe_entry = data["recipes"].get("brief-recipe", {})
        self.assertIn("brief_fragments", recipe_entry)
        bf = recipe_entry["brief_fragments"]
        self.assertIn("workflow_rules", bf)
        self.assertEqual(bf["workflow_rules"][0]["text"], "A rule from brief-recipe.")


class StaleCleanupOverrideTests(unittest.TestCase):
    """Stale cleanup-override detection — sync WARN path (real catalog recipe)."""

    @classmethod
    def setUpClass(cls):
        with open(ROOT / "catalog" / "recipes" / "worktree-flow" / "recipe.toml", "rb") as fh:
            cls.version = tomllib.load(fh)["recipe"]["version"]

    def _new_base(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _make_wf_project(self) -> tuple[Path, Path]:
        base = self._new_base()
        root = base / "project"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.worktree-flow]\nenabled = true\nversion = "{self.version}"\n'
        )
        return root, _make_home(base)

    def _cleanup_target(self, root: Path) -> Path:
        return (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )

    def _catalog_src(self) -> Path:
        return (
            ROOT / "catalog" / "recipes" / "worktree-flow" / "templates"
            / "worktree-cleanup.sh"
        )

    def _sync_wf(self, root: Path, home: Path) -> CLIResult:
        result = _run(root, "sync", home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def test_identical_override_no_stale_warn(self):
        root, home = self._make_wf_project()
        dest = self._cleanup_target(root)
        dest.parent.mkdir(parents=True, exist_ok=True)
        payload = self._catalog_src().read_bytes()
        dest.write_bytes(payload)
        result = self._sync_wf(root, home)
        self.assertEqual(dest.read_bytes(), payload)
        self.assertNotIn("not refreshed", result.stderr)
        self.assertNotIn("condition=not_exists", result.stderr)

    def test_divergent_override_warns_and_sync_succeeds(self):
        root, home = self._make_wf_project()
        dest = self._cleanup_target(root)
        dest.parent.mkdir(parents=True, exist_ok=True)
        custom = b"# customized override\n"
        dest.write_bytes(custom)
        result = self._sync_wf(root, home)
        self.assertEqual(dest.read_bytes(), custom)
        err = result.stderr
        self.assertIn("preserving existing file", err)
        self.assertIn("leave it unchanged", err)
        self.assertIn("remove it and run sync again", err)
        self.assertIn("worktree-cleanup.sh", err)
        self.assertIn("rm ", err)
        self.assertIn("ai-specs sync", err)
        self.assertNotIn("user-managed", err.lower())
        self.assertNotIn("customized", err.lower())

    def test_missing_override_gets_fresh_copy(self):
        root, home = self._make_wf_project()
        dest = self._cleanup_target(root)
        self.assertFalse(dest.exists())
        result = self._sync_wf(root, home)
        self.assertTrue(dest.is_file())
        # Sync stamps __WORKTREE_REPO_TOPOLOGY__ (default auto), mirroring gate_mode.
        expected = (
            self._catalog_src()
            .read_text()
            .replace("__WORKTREE_REPO_TOPOLOGY__", "auto")
        )
        self.assertEqual(dest.read_text(), expected)
        self.assertNotIn("__WORKTREE_REPO_TOPOLOGY__", dest.read_text())
        self.assertNotIn("not refreshed", result.stderr)
