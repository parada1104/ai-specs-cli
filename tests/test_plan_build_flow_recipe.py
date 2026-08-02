"""Validation + materialization tests for the plan-build-flow catalog recipe."""

import importlib.util
import re
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "plan-build-flow"
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import recipe_skill_dir, recipe_root, cache_command, resolved_skills_dir

FORBIDDEN_TERMS = ("sdd", "openspec", "spec-driven")
FORBIDDEN_SLASH = ("/plan", "/build", "/archive")
STORE_ENUM = ["openspec", "engram", "both"]


def _without_store_config_table(raw: str) -> str:
    """Remove exactly the store table, leaving all other recipe prose guarded."""
    pattern = r"(?ms)^\[config\.artifact_store_default\]\n.*?(?=^\[|\Z)"
    stripped, count = re.subn(pattern, "", raw, count=1)
    assert count == 1, "store config table must be present exactly once"
    return stripped


def _without_delivery_contracts_section(raw: str) -> str:
    """Remove exactly one README section through the next same-level heading."""
    pattern = r"(?ms)^## Delivery contracts\n.*?(?=^## |\Z)"
    stripped, count = re.subn(pattern, "", raw, count=1)
    assert count == 1, "README delivery contracts section must be present exactly once"
    return stripped


def _recipe_surface_text(recipe_dir: Path) -> str:
    recipe = _without_store_config_table((recipe_dir / "recipe.toml").read_text())
    readme = _without_delivery_contracts_section((recipe_dir / "README.md").read_text())
    skill = (recipe_dir / "skills" / RECIPE_ID / "SKILL.md").read_text()
    return "\n".join((recipe, readme, skill)).lower()

def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _version_of(recipe_id: str) -> str:
    text = (CATALOG / recipe_id / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, f"could not find version in {recipe_id}/recipe.toml"
    return match.group(1)


def _recipe_version() -> str:
    return _version_of(RECIPE_ID)


class PlanBuildFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_pbf")
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_pbf")

    def _make_project(self, extra_recipes: str = "") -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{_recipe_version()}"\n'
            + extra_recipes
        )
        return root
    def _render_agents(self, root: Path) -> str:
        resolved = root / "resolved-config.json"
        self.assertEqual(
            self.mod.materialize_recipes(root, ROOT, resolved_config_out=resolved), 0
        )
        renderer = load_module(
            ROOT / "lib" / "_internal" / "agents-render.py", "agents_render_pbf_e2e"
        )
        agents = root / "AGENTS.md"
        renderer.render(
            root / "ai-specs" / "ai-specs.toml",
            agents,
            preserve_if_marker=False,
            resolved_config_path=resolved,
        )
        return agents.read_text()

    def test_recipe_materializes_skill_only(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(recipe.id, RECIPE_ID)
        self.assertEqual(len(recipe.commands), 0)
        skill_ids = [(s.id, s.source) for s in recipe.skills]
        self.assertIn(("plan-build-flow", "bundled"), skill_ids)

        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        skill = (
            recipe_root(root, RECIPE_ID)
            / "skills" / "plan-build-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file())
        for forbidden in ("plan.md", "build.md", "archive.md"):
            self.assertFalse(
                (root / "ai-specs" / "commands" / forbidden).exists(),
                f"unexpected command {forbidden}",
            )

    def test_recipe_declares_exact_store_schema_and_hook_pair(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(list(recipe.config_schema.fields), ["artifact_store_default"])
        field = recipe.config_schema.fields["artifact_store_default"]
        self.assertFalse(field.required)
        self.assertEqual(field.type, "string")
        self.assertEqual(field.default, "openspec")
        self.assertEqual(field.enum, STORE_ENUM)
        self.assertTrue(field.help_text.strip())
        hook_pairs = [(h.event, h.action) for h in recipe.hooks]
        self.assertEqual(hook_pairs, [("on-sync", "validate-config")])

        raw = (recipe_dir / "recipe.toml").read_text()
        self.assertEqual(raw.count("[config.artifact_store_default]"), 1)
        self.assertIn('enum = ["openspec", "engram", "both"]', raw)

    def test_recipe_brief_rule_is_last_string_fragment_with_store_placeholder(self):
        recipe = self.schema.load_recipe_toml(CATALOG / RECIPE_ID / "recipe.toml")
        rules = recipe.brief_fragments.workflow_rules
        self.assertEqual(len(rules), 6)
        self.assertEqual([fragment.key for fragment in rules], [None] * 6)
        self.assertEqual(
            [fragment.text for fragment in rules[:5]],
            [
                "Classify each substantial change (full planning chain, spec+tasks, or tasks-only) before writing production code; record depth in tasks.md and stop for authorization.",
                "Direct implementation requests without a change folder still require planning at the classified depth; approval verbs do not skip the plan step.",
                "Do not open a PR until the change folder on the branch contains the tier minimum planning files, committed.",
                "After authorization, implement and validate in the change worktree when isolated worktrees are enabled.",
                "Archive the change folder on the review branch before merge; never defer archive until after merge.",
            ],
        )
        self.assertIn("{config.artifact_store_default}", rules[-1].text)
        self.assertEqual(rules[-1].text.count("{config.artifact_store_default}"), 1)

    def test_skill_has_ambient_auto_invoke(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        text = skill.read_text()
        self.assertIn("auto_invoke:", text)
        self.assertIn("substantial", text.lower())
        self.assertNotIn("/plan", text.split("auto_invoke")[0])  # frontmatter ok

    def test_brief_and_readme_vocabulary_clean(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        brief = recipe.brief_fragments
        self.assertIsNotNone(brief)
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]
        fragments = "\n".join(rules).lower()
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, fragments)
        for slash in FORBIDDEN_SLASH:
            self.assertNotIn(slash, fragments)

        root = self._make_project()
        self.mod.materialize_recipes(root, ROOT)
        readme = (root / "ai-specs" / "recipes" / RECIPE_ID / "README.md").read_text()
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, _without_delivery_contracts_section(readme).lower())

    def test_store_defaults_override_and_enum_rejection(self):
        recipe = self.schema.load_recipe_toml(CATALOG / RECIPE_ID / "recipe.toml")
        self.assertEqual(self.mod.merge_config(recipe, {}), {"artifact_store_default": "openspec"})
        self.assertEqual(
            self.mod.merge_config(recipe, {"artifact_store_default": "both"}),
            {"artifact_store_default": "both"},
        )
        with self.assertRaisesRegex(RuntimeError, "artifact_store_default"):
            self.mod.merge_config(recipe, {"artifact_store_default": "vault"})

    def test_recipe_surface_excludes_session_controls_and_removed_contract(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        schema_keys = set(recipe.config_schema.fields)
        self.assertNotIn("chained_" + "pr_default", schema_keys)
        self.assertFalse(any("mode" in key.lower() for key in schema_keys))

        surface = _recipe_surface_text(recipe_dir)
        removed_root = "bud" + "get"
        removed_key = "review_" + removed_root
        removed_phrase = "review " + removed_root
        self.assertNotIn(removed_key, surface)
        skill = (recipe_dir / "skills" / RECIPE_ID / "SKILL.md").read_text()
        self.assertNotRegex(skill, r"(?im)^#{1,6}\s*7\.5\b")
        self.assertNotRegex(skill, r"(?im)^#{1,6}\s*Review workload budget\b")
        self.assertNotRegex(skill, r"(?im)^\s*WARN:\s*review budget\b")
        gate = (recipe_dir / "hooks" / "plan-build-gate.sh").read_text().lower()
        self.assertNotIn(removed_root, gate)
        self.assertNotIn("forecast", gate)
        external_terms = ("gentle-" + "ai", "gentle-" + "pi")
        catalog_section = (ROOT / "docs" / "recipes-catalog.md").read_text().lower()
        for term in external_terms:
            self.assertNotIn(term, surface)
            self.assertNotIn(term, catalog_section)

    def test_materialization_renders_manifest_store_override_into_agents(self):
        root = self._make_project(
            "\n[recipes.plan-build-flow.config]\nartifact_store_default = 'both'\n"
        )
        content = self._render_agents(root)
        self.assertIn("Default artifact store", content)
        self.assertIn("`both`", content)
        self.assertNotIn("{config.artifact_store_default}", content)
        self.assertLess(content.index("Classify each substantial change"), content.index("Default artifact store"))

    def test_materialization_renders_default_store_into_agents(self):
        content = self._render_agents(self._make_project())
        self.assertIn("`openspec`", content)
        self.assertNotIn("{config.artifact_store_default}", content)

    def test_validate_config_hook_accepts_each_store_enum(self):
        recipe = self.schema.load_recipe_toml(CATALOG / RECIPE_ID / "recipe.toml")
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        for value in STORE_ENUM:
            with self.subTest(value=value):
                self.mod.execute_hooks(recipe, {"artifact_store_default": value}, Path(tmp.name))

    def test_version_and_catalog_documentation_use_current_contract(self):
        self.assertEqual(_recipe_version(), "1.3.0")
        readme = (CATALOG / RECIPE_ID / "README.md").read_text()
        catalog = (ROOT / "docs" / "recipes-catalog.md").read_text()
        for text in (readme, catalog):
            self.assertIn("artifact_store_default", text)
            self.assertIn("openspec", text)
            self.assertIn("engram", text)
            self.assertIn("both", text)
            self.assertIn("1.3.0", text)

    def test_implementation_brief_references_worktree_flow(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        brief = recipe.brief_fragments
        self.assertIsNotNone(brief)
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]
        combined = "\n".join(rules).lower()
        self.assertIn("worktree", combined)
        self.assertNotIn("/build", combined)
        self.assertNotIn("worktree-flow", recipe.conflicts_with)

    def test_classic_sdd_commands_unchanged(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        commands = ai_specs / "commands"
        commands.mkdir()
        legacy = commands / "legacy-sdd-cmd.md"
        legacy.write_text("# Legacy\n")
        (ai_specs / "skills" / "legacy-sdd-skill").mkdir()
        (ai_specs / "skills" / "legacy-sdd-skill" / "SKILL.md").write_text("---\nname: legacy\n---\n")

        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{_recipe_version()}"\n'
        )
        before = legacy.read_text()
        self.mod.materialize_recipes(root, ROOT)
        self.assertEqual(legacy.read_text(), before)

    def test_skill_has_change_depth_classifier(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        text = skill.read_text().lower()
        self.assertIn("change depth classifier", text)
        for tier in ("full", "standard", "light"):
            self.assertIn(tier, text)

    def test_skill_has_pr_and_archive_gates(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        raw = skill.read_text()
        text = raw.lower()
        self.assertIn("pr creation gate", text)
        self.assertIn("pre-merge archive gate", text)
        self.assertIn("pre-merge merge guardian", text)
        self.assertIn("premerge_guardian", text)
        self.assertIn("AI_SPECS_HOME", raw)
        self.assertIn("lib/_internal/premerge_guardian.py", text)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", text)
        self.assertIn("gh pr create", text)
        self.assertIn("before merge", text)

    def test_recipe_does_not_stage_premerge_guardian_into_project(self):
        recipe = self.schema.load_recipe_toml(CATALOG / RECIPE_ID / "recipe.toml")
        targets = [t.target for t in recipe.templates]
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", targets)
        self.assertTrue(
            (ROOT / "lib" / "_internal" / "premerge_guardian.py").is_file()
        )

    def test_brief_mentions_depth_and_pr_gate(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        brief = recipe.brief_fragments
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]
        combined = "\n".join(rules).lower()
        self.assertIn("classify", combined)
        self.assertIn("tasks-only", combined)
        self.assertIn("do not open a pr", combined)
        self.assertIn("before merge", combined)


if __name__ == "__main__":
    unittest.main()
