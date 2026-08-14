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


    def test_worktree_new_documents_submodule_create_contract(self):
        """Doc-content only — live git worktree add under submodules is manual/agent."""
        text = (RECIPE_DIR / "commands" / "worktree-new.md").read_text()
        self.assertIn("git -C", text)
        self.assertIn("worktrees_dir", text)
        self.assertIn("<subrepo>-<slug>", text)
        self.assertIn("show-toplevel", text)
        self.assertIn("longest", text.lower())
        self.assertIn("submodule update --init", text)

    def test_worktree_new_documents_superrepo_context_requires_explicit_subrepo(self):
        """1.2 — RED: a superrepo-context request must not infer a subrepo."""
        text = (RECIPE_DIR / "commands" / "worktree-new.md").read_text()
        self.assertIn("explicit", text)
        self.assertIn("hard-error", text.lower().replace("hard error", "hard-error"))
        self.assertIn("not infer", text.lower())
        self.assertIn("git worktree add", text)

    def test_worktree_new_documents_owner_vs_planning_root(self):
        """1.2 — RED: owner root and planning root are distinct request facts."""
        text = (RECIPE_DIR / "commands" / "worktree-new.md").read_text()
        self.assertIn("planning root", text.lower())
        self.assertIn("owner", text.lower())
        self.assertIn("superproject", text)
        self.assertIn("planning", text.lower())

    def test_worktree_new_is_generated_markdown_not_executable_helper(self):
        """1.2 — RED: /worktree-new is generated Markdown; no executable helper."""
        cmd = RECIPE_DIR / "commands" / "worktree-new.md"
        self.assertTrue(cmd.is_file())
        self.assertNotIn("#!/", cmd.read_text(), "command must stay Markdown, not a script")
        self.assertFalse(
            (RECIPE_DIR / "bin" / "worktree-new").exists(),
            "no executable /worktree-new helper may be added",
        )

    def test_skill_md_documents_request_context_and_no_helper(self):
        """1.2 — RED: SKILL.md create block pins request-context + no helper."""
        skill = (RECIPE_DIR / "skills" / "worktree-flow" / "SKILL.md").read_text()
        self.assertIn("resolve_request_context", skill)
        self.assertIn("planning_root", skill)
        self.assertIn("explicit", skill)
        self.assertNotIn("bin/worktree-new", skill)

    def test_brief_workflow_rules_require_which_repo_check(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        frags = recipe.brief_fragments
        self.assertIsNotNone(frags)
        rules = " ".join(
            f.text if hasattr(f, "text") else str(f)
            for f in (frags.workflow_rules or [])
        )
        if not rules:
            # fragments may be plain strings depending on schema version
            raw = (RECIPE_DIR / "recipe.toml").read_text()
            start = raw.index("workflow_rules")
            rules = raw[start:start + 800]
        self.assertIn("which", rules.lower())
        self.assertIn("show-toplevel", rules)
        self.assertIn("monorepo-submodules", rules)



    def test_skill_md_create_block_matches_worktree_new_contract(self):
        """SKILL.md create block must stay byte-consistent with worktree-new.md."""
        skill = (RECIPE_DIR / "skills" / "worktree-flow" / "SKILL.md").read_text()
        # Must use super_root-scoped rev-parse (not bare show-toplevel).
        self.assertIn('git -C "$super_root" rev-parse --show-toplevel', skill)
        # Must use configurable worktrees_dir placeholder, not hardcoded .worktrees
        # as the create destination (default may still be mentioned as prose).
        self.assertIn("<worktrees_dir>", skill)
        # Hardcoded ".worktrees/<subrepo>" create path recreates the original bug.
        self.assertNotIn("$super_abs/.worktrees/", skill)
        self.assertNotIn("git worktree add .worktrees/", skill)

    def test_gate_scope_defaults_to_auto_and_is_independent(self):
        root = self._make_project_with_config(
            '[recipes.worktree-flow.config]\ngate_mode = "always"\nrepo_topology = "monorepo-submodules"'
        )
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        import tomllib
        with open(root / "ai-specs" / "ai-specs.toml", "rb") as fh:
            manifest = tomllib.load(fh)
        merged = self.materialize.merge_config(recipe, manifest["recipes"]["worktree-flow"]["config"])
        self.assertEqual(merged.get("gate_scope"), "auto")
        self.assertEqual(merged.get("gate_mode"), "always")
        self.assertEqual(merged.get("repo_topology"), "monorepo-submodules")

    def test_gate_scope_materializes_stamp(self):
        root = self._make_project_with_config(
            '[recipes.worktree-flow.config]\ngate_scope = "superrepo"'
        )
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        hook = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        content = hook.read_text()
        self.assertIn('stamped_gate_scope="superrepo"', content)
        self.assertIn('stamped_repo_topology="auto"', content)
        self.assertNotIn("__WORKTREE_REPO_TOPOLOGY__", content)
        self.assertNotIn("__WORKTREE_GATE_SCOPE__", content)

    def test_gate_scope_rejects_invalid_value(self):
        root = self._make_project_with_config(
            '[recipes.worktree-flow.config]\ngate_scope = "super-repo"'
        )
        import subprocess
        proc = subprocess.run(["python3", str(RECIPE_MATERIALIZE_PATH), str(root), str(ROOT)], capture_output=True, text=True)
        self.assertEqual(proc.returncode, 1)
        combined = proc.stderr + proc.stdout
        self.assertIn("super-repo", combined)
        self.assertIn("auto | superrepo | subrepo", combined)
    def test_stale_gate_hook_is_preserved_with_refresh_guidance(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        hook = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        hook.write_text("custom legacy hook\n")
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        self.assertEqual(hook.read_text(), "custom legacy hook\n")

if __name__ == "__main__":
    unittest.main()
