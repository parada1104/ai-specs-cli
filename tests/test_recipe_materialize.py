import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeMaterializeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")

    def _make_project(self, recipe_section: str) -> Path:
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
            + recipe_section
            + "\n"
        )
        return root

    def test_materializes_bundled_skill(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        skill_dir = root / ".recipe" / "test-fixture" / "skills" / "test-skill"
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_materializes_command(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        cmd = root / "ai-specs" / "commands" / "test-command.md"
        self.assertTrue(cmd.is_file())

    def test_materializes_doc(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        doc = root / "docs" / "test-doc-output.md"
        self.assertTrue(doc.is_file())

    def test_materializes_template_not_exists(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        tpl = root / "docs" / "test-template-output.md"
        self.assertTrue(tpl.is_file())

    def test_skips_template_when_target_exists(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        existing = root / "docs" / "test-template-output.md"
        existing.parent.mkdir(parents=True, exist_ok=True)
        existing.write_text("existing")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertEqual(existing.read_text(), "existing")

    def test_writes_recipe_mcp_json(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        mcp_path = root / "ai-specs" / ".recipe-mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("test-mcp", data)
        self.assertEqual(data["test-mcp"]["command"], "npx")

    def test_disabled_recipe_skips_materialization(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = false\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertFalse((root / ".recipe" / "test-fixture" / "skills" / "test-skill").exists())

    def test_version_mismatch_fails(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "2.0.0"\n'
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.materialize_recipes(root, ROOT)
        self.assertIn("version mismatch", str(ctx.exception))

    def test_unknown_recipe_fails(self):
        root = self._make_project(
            '[recipes.nonexistent]\nenabled = true\nversion = "1.0.0"\n'
        )
        with self.assertRaises(Exception):
            self.mod.materialize_recipes(root, ROOT)

    def test_no_recipes_section_succeeds(self):
        root = self._make_project("")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

    def test_recipe_does_not_overwrite_user_local_skill(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        # Pre-create a user-local skill with the same ID
        user_skill = root / "ai-specs" / "skills" / "test-skill"
        user_skill.mkdir(parents=True)
        (user_skill / "SKILL.md").write_text("user local")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        # Recipe version goes to .recipe/ and local skill is preserved
        self.assertEqual((user_skill / "SKILL.md").read_text(), "user local")
        recipe_skill = root / ".recipe" / "test-fixture" / "skills" / "test-skill"
        self.assertTrue(recipe_skill.is_dir())

    # --- V2 materialize tests -----------------------------------------------

    def _make_v2_recipe(self, tmp: str, rid: str, caps: list[str] = None, hooks: list[dict] = None, config: dict = None, skills: list[str] = None):
        recipe_dir = Path(tmp) / rid
        recipe_dir.mkdir()
        cap_lines = "".join(f'[[capabilities]]\nid = "{c}"\n' for c in (caps or []))
        hook_lines = "".join(f'[[hooks]]\nevent = "{h["event"]}"\naction = "{h["action"]}"\n' for h in (hooks or []))
        config_lines = ""
        for key, field in (config or {}).items():
            config_lines += f"[config.{key}]\n"
            for fk, fv in field.items():
                if isinstance(fv, bool):
                    config_lines += f"{fk} = {str(fv).lower()}\n"
                elif isinstance(fv, str):
                    config_lines += f'{fk} = "{fv}"\n'
                else:
                    config_lines += f"{fk} = {fv}\n"
        skill_lines = ""
        for sid in (skills or []):
            skill_lines += f'[[provides.skills]]\nid = "{sid}"\nsource = "bundled"\n'
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\nid = "{rid}"\nname = "{rid.title()}"\ndescription = "D"\nversion = "1.0"\n'
            + cap_lines + hook_lines + config_lines + skill_lines
        )
        # Create dummy skill dirs for bundled skills
        for sid in (skills or []):
            (recipe_dir / "skills" / sid).mkdir(parents=True)
            (recipe_dir / "skills" / sid / "SKILL.md").write_text("skill")

    def test_resolve_bindings_explicit(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "recipe-a", caps=["tracker"])
            bindings = self.mod.resolve_bindings(catalog, ["recipe-a"], [{"capability": "tracker", "recipe": "recipe-a"}])
            self.assertEqual(bindings, {"tracker": "recipe-a"})

    def test_resolve_bindings_auto_bind_single_provider(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "recipe-a", caps=["tracker"])
            bindings = self.mod.resolve_bindings(catalog, ["recipe-a"], [])
            self.assertEqual(bindings, {"tracker": "recipe-a"})

    def test_resolve_bindings_auto_bind_skips_ambiguity(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "recipe-a", caps=["tracker"])
            self._make_v2_recipe(tmp, "recipe-b", caps=["tracker"])
            bindings = self.mod.resolve_bindings(catalog, ["recipe-a", "recipe-b"], [])
            self.assertNotIn("tracker", bindings)

    def test_resolve_bindings_explicit_disabled_fails(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "recipe-a", caps=["tracker"])
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.resolve_bindings(catalog, ["recipe-a"], [{"capability": "tracker", "recipe": "recipe-b"}])
            self.assertIn("disabled/unknown", str(ctx.exception))

    def test_resolve_bindings_duplicate_explicit_fails(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "recipe-a", caps=["tracker"])
            self._make_v2_recipe(tmp, "recipe-b", caps=["tracker"])
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.resolve_bindings(catalog, ["recipe-a", "recipe-b"], [
                    {"capability": "tracker", "recipe": "recipe-a"},
                    {"capability": "tracker", "recipe": "recipe-b"},
                ])
            self.assertIn("duplicate explicit binding", str(ctx.exception))

    def test_merge_config_defaults_and_override(self):
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "timeout": ConfigField(required=False, type="integer", default=30),
                "board_id": ConfigField(required=True, type="string"),
            })
        )
        cfg = self.mod.merge_config(recipe, {"board_id": "abc"})
        self.assertEqual(cfg["timeout"], 30)
        self.assertEqual(cfg["board_id"], "abc")

    def test_merge_config_missing_required_fails(self):
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True, type="string"),
            })
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.merge_config(recipe, {})
        self.assertIn("missing required config field", str(ctx.exception))

    def test_merge_config_warns_on_unknown_key(self):
        from lib._internal.recipe_schema import Recipe, ConfigSchema
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={})
        )
        # Should warn but not fail
        cfg = self.mod.merge_config(recipe, {"unknown": 1})
        self.assertEqual(cfg, {})

    def test_execute_hooks_validate_config_success(self):
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={"key": ConfigField(required=True)}),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        # Should not raise
        self.mod.execute_hooks(recipe, {"key": "value"}, Path(tempfile.gettempdir()))

    def test_execute_hooks_validate_config_fails(self):
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={"key": ConfigField(required=True)}),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))
        self.assertIn("validate-config", str(ctx.exception))

    def test_execute_hooks_unknown_action_warns(self):
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="unknown")]
        )
        # Should warn but not raise
        self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))

    def test_end_to_end_v2_recipe_with_config_and_hooks(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            ai_specs_home = Path(tmp)
            catalog = ai_specs_home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            self._make_v2_recipe(str(catalog), "v2-recipe", caps=["tracker"], hooks=[{"event": "on-sync", "action": "validate-config"}],
                config={"board_id": {"required": True, "type": "string"}}, skills=["v2-skill"])
            # Also create command
            (catalog / "v2-recipe" / "commands").mkdir()
            (catalog / "v2-recipe" / "commands" / "v2-cmd.md").write_text("cmd")
            (catalog / "v2-recipe" / "templates").mkdir()
            (catalog / "v2-recipe" / "templates" / "tpl.md").write_text("tpl")
            (catalog / "v2-recipe" / "docs").mkdir()
            (catalog / "v2-recipe" / "docs" / "doc.md").write_text("doc")

            root = Path(tmp) / "project"
            ai_specs = root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "skills").mkdir()
            (ai_specs / "commands").mkdir()
            manifest = ai_specs / "ai-specs.toml"
            manifest.write_text(
                "[project]\nname = 'fixture'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                "[recipes.v2-recipe]\nenabled = true\nversion = \"1.0\"\n"
                "[recipes.v2-recipe.config]\nboard_id = 'abc123'\n"
            )
            self.assertEqual(self.mod.materialize_recipes(root, ai_specs_home), 0)
            self.assertTrue((root / ".recipe" / "v2-recipe" / "skills" / "v2-skill").is_dir())

    def test_v1_manifest_without_bindings_or_config_succeeds(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

    # --- MCP preset merge safety tests --------------------------------------

    def _make_project_with_mcp(self, recipe_section: str, mcp_section: str) -> Path:
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
            + mcp_section + "\n"
            + recipe_section
            + "\n"
        )
        return root

    def test_mcp_preset_manifest_precedence_on_conflict(self):
        root = self._make_project_with_mcp(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n',
            '[mcp.test-mcp]\ncommand = "custom-cmd"\nargs = ["--flag"]\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        mcp_path = root / "ai-specs" / ".recipe-mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("test-mcp", data)
        # Manifest value must win
        self.assertEqual(data["test-mcp"]["command"], "custom-cmd")
        self.assertEqual(data["test-mcp"]["args"], ["--flag"])
        # Recipe value for key not in manifest is added
        # (args is present in both, so manifest wins; but the recipe does not add new keys here)

    def test_mcp_preset_recipe_creates_when_not_in_manifest(self):
        root = self._make_project_with_mcp(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n',
            ''  # no mcp section
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        mcp_path = root / "ai-specs" / ".recipe-mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("test-mcp", data)
        self.assertEqual(data["test-mcp"]["command"], "npx")
        self.assertEqual(data["test-mcp"]["args"], ["-y", "@test/mcp-server"])

    def test_mcp_preset_merge_warns_on_conflict(self):
        import io
        root = self._make_project_with_mcp(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n',
            '[mcp.test-mcp]\ncommand = "custom-cmd"\n'
        )
        captured = io.StringIO()
        real_stderr = sys.stderr
        sys.stderr = captured
        try:
            self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        finally:
            sys.stderr = real_stderr
        stderr_output = captured.getvalue()
        self.assertIn("conflicts with project manifest", stderr_output)

    # --- Hook execution: bootstrap-board --------------------------------------

    def test_execute_hooks_bootstrap_board_creates_marker(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="bootstrap-board")]
        )
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            self.mod.execute_hooks(recipe, {"board_id": "test-board-123", "default_list": "In Progress", "epic_list": "Epic"}, project_root)
            marker_dir = project_root / ".recipe" / "r"
            self.assertTrue(marker_dir.is_dir())
            marker_file = marker_dir / "bootstrap-ready"
            self.assertTrue(marker_file.is_file())
            content = marker_file.read_text()
            self.assertIn("board_id=test-board-123", content)
            self.assertIn("default_list=In Progress", content)
            self.assertIn("epic_list=Epic", content)

    def test_execute_hooks_bootstrap_board_marker_content(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="myrecipe", name="MyRecipe", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="bootstrap-board")]
        )
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            self.mod.execute_hooks(recipe, {"board_id": "b1", "default_list": "Todo", "epic_list": "Backlog"}, project_root)
            marker_file = project_root / ".recipe" / "myrecipe" / "bootstrap-ready"
            content = marker_file.read_text()
            self.assertEqual(content, "board_id=b1\ndefault_list=Todo\nepic_list=Backlog\n")

    def test_execute_hooks_bootstrap_board_missing_board_id(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={"board_id": ConfigField(required=True)}),
            hooks=[
                Hook(event="on-sync", action="validate-config"),
                Hook(event="on-sync", action="bootstrap-board"),
            ]
        )
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.execute_hooks(recipe, {}, project_root)
            self.assertIn("validate-config", str(ctx.exception))

    # --- Hook execution: deferred hooks --------------------------------------

    def test_execute_hooks_deferred_link_trello_card(self):
        import io
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="link-trello-card")]
        )
        captured = io.StringIO()
        real_stdout = sys.stdout
        sys.stdout = captured
        try:
            self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))
        finally:
            sys.stdout = real_stdout
        output = captured.getvalue()
        self.assertIn("link-trello-card", output)
        self.assertIn("deferred", output)

    def test_execute_hooks_deferred_sync_card_state(self):
        import io
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="sync-card-state")]
        )
        captured = io.StringIO()
        real_stdout = sys.stdout
        sys.stdout = captured
        try:
            self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))
        finally:
            sys.stdout = real_stdout
        output = captured.getvalue()
        self.assertIn("sync-card-state", output)
        self.assertIn("deferred", output)

    def test_execute_hooks_deferred_comment_verification(self):
        import io
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="comment-verification")]
        )
        captured = io.StringIO()
        real_stdout = sys.stdout
        sys.stdout = captured
        try:
            self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))
        finally:
            sys.stdout = real_stdout
        output = captured.getvalue()
        self.assertIn("comment-verification", output)
        self.assertIn("deferred", output)

    # --- Hook execution: project_root parameter ------------------------------

    def test_execute_hooks_project_root_used_by_bootstrap_board(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="bootstrap-board")]
        )
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            self.mod.execute_hooks(recipe, {"board_id": "b1"}, project_root)
            marker = project_root / ".recipe" / "r" / "bootstrap-ready"
            self.assertTrue(marker.is_file())
            self.assertIn("board_id=b1", marker.read_text())

    def test_execute_hooks_project_root_different_paths(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="bootstrap-board")]
        )
        with tempfile.TemporaryDirectory() as tmp1:
            with tempfile.TemporaryDirectory() as tmp2:
                root1 = Path(tmp1)
                root2 = Path(tmp2)
                cfg = {"board_id": "board-1", "default_list": "List1", "epic_list": "Epic1"}
                self.mod.execute_hooks(recipe, cfg, root1)
                cfg2 = {"board_id": "board-2", "default_list": "List2", "epic_list": "Epic2"}
                self.mod.execute_hooks(recipe, cfg2, root2)
                m1 = root1 / ".recipe" / "r" / "bootstrap-ready"
                m2 = root2 / ".recipe" / "r" / "bootstrap-ready"
                self.assertTrue(m1.is_file())
                self.assertTrue(m2.is_file())
                self.assertIn("board_id=board-1", m1.read_text())
                self.assertIn("board_id=board-2", m2.read_text())

    # --- Config validation: board_id / optional fields -----------------------

    def test_config_validation_board_id_required(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={"board_id": ConfigField(required=True)}),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))
        self.assertIn("validate-config", str(ctx.exception))

    def test_config_validation_default_list_optional(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),
                "default_list": ConfigField(required=False, default="In Progress"),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        self.mod.execute_hooks(recipe, {"board_id": "b1"}, Path(tempfile.gettempdir()))

    def test_config_validation_epic_list_optional(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),
                "epic_list": ConfigField(required=False, default="Epic"),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        self.mod.execute_hooks(recipe, {"board_id": "b1"}, Path(tempfile.gettempdir()))

    # --- Integration: trello-mcp-workflow recipe materialization ------------

    def test_materialize_trello_mcp_workflow_recipe(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            ai_specs_home = Path(tmp)
            catalog = ai_specs_home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            rid = "trello-mcp-workflow"
            recipe_dir = catalog / rid
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                f'id = "{rid}"\n'
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
                '[config.default_list]\nrequired = false\ntype = "string"\ndefault = "In Progress"\n'
                '[config.epic_list]\nrequired = false\ntype = "string"\ndefault = "Epic"\n'
                '[[provides.skills]]\nid = "trello-pm-workflow"\nsource = "bundled"\n'
            )
            skill_dir = recipe_dir / "skills" / "trello-pm-workflow"
            skill_dir.mkdir(parents=True)
            (skill_dir / "SKILL.md").write_text("# Trello PM Workflow\n")
            project_root = Path(tempfile.mkdtemp())
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'test-project'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                f"[recipes.{rid}]\nenabled = true\nversion = '1.0'\n"
                f"[recipes.{rid}.config]\nboard_id = 'abc123'\n"
            )
            result = self.mod.materialize_recipes(project_root, ai_specs_home)
            self.assertEqual(result, 0)
            skill_path = project_root / ".recipe" / rid / "skills" / "trello-pm-workflow"
            self.assertTrue(skill_path.is_dir())
            self.assertTrue((skill_path / "SKILL.md").is_file())
            marker = project_root / ".recipe" / rid / "bootstrap-ready"
            self.assertTrue(marker.is_file())
            marker_content = marker.read_text()
            self.assertIn("board_id=abc123", marker_content)


if __name__ == "__main__":
    unittest.main()
