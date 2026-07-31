"""Validation + materialization tests for the worktree-flow catalog recipe."""

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import recipe_skill_dir, recipe_root, cache_command, resolved_skills_dir
RECIPE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class WorktreeFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.materialize = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal_wtf"
        )

    def test_recipe_validates(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(recipe.id, "worktree-flow")
        cap_ids = {c.id for c in recipe.capabilities}
        self.assertIn("worktree-isolation", cap_ids)

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        import tomllib

        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            version = tomllib.load(fh)["recipe"]["version"]
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.worktree-flow]\nenabled = true\nversion = "{version}"\n'
        )
        return root

    def _make_project_with_config(self, config_block: str = "") -> Path:
        root = self._make_project()
        manifest = root / "ai-specs" / "ai-specs.toml"
        text = manifest.read_text()
        if config_block:
            text = text.rstrip() + "\n" + config_block + "\n"
        manifest.write_text(text)
        return root

    def test_sync_defaults_to_always(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        hook = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "hooks"
            / "worktree-gate.sh"
        )
        self.assertTrue(hook.is_file())
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="always"', content)

    def test_sync_materializes_gate_mode_into_hook(self):
        root = self._make_project_with_config(
            '[recipes.worktree-flow.config]\ngate_mode = "ask"'
        )
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        hook = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "hooks"
            / "worktree-gate.sh"
        )
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="ask"', content)
        self.assertNotIn("__WORKTREE_GATE_MODE__", content)

    def test_sync_rejects_invalid_gate_mode(self):
        root = self._make_project_with_config(
            '[recipes.worktree-flow.config]\ngate_mode = "bogus"'
        )
        with self.assertRaises(SystemExit) as ctx:
            self.materialize.materialize_recipes(root, ROOT)
        self.assertEqual(ctx.exception.code, 1)
        combined = ""
        # materialize_recipes calls fail() which prints to stderr then exits
        # Re-run via subprocess to capture diagnostic text.
        import subprocess
        proc = subprocess.run(
            [
                "python3",
                str(RECIPE_MATERIALIZE_PATH),
                str(root),
                str(ROOT),
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(proc.returncode, 1)
        combined = proc.stderr + proc.stdout
        self.assertIn("bogus", combined)
        self.assertRegex(combined, r"always.*ask.*off|always \| ask \| off")

    def test_materializes_skill_commands_and_script(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)

        skill = (
            recipe_root(root, "worktree-flow") / "skills"
            / "worktree-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), "bundled skill should materialize")

        for cmd in ("worktree-new", "worktree-clean"):
            path = cache_command(root, cmd)
            self.assertTrue(path.is_file(), f"command {cmd} should materialize")

        script = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )
        self.assertTrue(script.is_file(), "cleanup script should materialize")


    def test_skill_mentions_sdd_artifact_phases(self):
        skill = RECIPE_DIR / "skills" / "worktree-flow" / "SKILL.md"
        text = skill.read_text()
        self.assertIn("SDD artifact phases", text)


    def test_sync_defaults_repo_topology_to_auto(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        import tomllib
        with open(root / "ai-specs" / "ai-specs.toml", "rb") as fh:
            manifest = tomllib.load(fh)
        user_cfg = (manifest.get("recipes") or {}).get("worktree-flow", {}).get("config") or {}
        merged = self.materialize.merge_config(recipe, user_cfg)
        self.assertEqual(merged.get("repo_topology"), "auto")

    def test_sync_rejects_invalid_repo_topology(self):
        root = self._make_project_with_config(
            '[recipes.worktree-flow.config]\nrepo_topology = "nested"'
        )
        import subprocess
        proc = subprocess.run(
            [
                "python3",
                str(RECIPE_MATERIALIZE_PATH),
                str(root),
                str(ROOT),
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(proc.returncode, 1)
        combined = proc.stderr + proc.stdout
        self.assertIn("nested", combined)
        self.assertRegex(
            combined,
            r"auto.*standalone.*monorepo-apps.*monorepo-submodules"
            r"|auto \| standalone \| monorepo-apps \| monorepo-submodules",
        )

    def test_sync_materializes_with_repo_topology_default(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        skill = (
            recipe_skill_dir(root, "worktree-flow", "worktree-flow") / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), "skill should materialize with default topology")
        for cmd in ("worktree-new", "worktree-clean"):
            path = cache_command(root, cmd)
            self.assertTrue(path.is_file(), f"command {cmd} should materialize")
        script = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )
        self.assertTrue(script.is_file(), "cleanup script should materialize")


if __name__ == "__main__":
    unittest.main()
