"""Tests for recipe_schema.py dataclasses and validation."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeSchemaTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.read_mod = load_module(RECIPE_READ_PATH, "recipe_read_internal")

    def test_config_field_without_validation_parses(self):
        """Config field without 'validation' sub-table parses successfully."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "no-val"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "no-val"\n'
                'name = "No Val"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.my_field]\n'
                'required = true\n'
                'type = "string"\n'
                'default = "hello"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            field = data.config_schema.fields["my_field"]
            self.assertEqual(field.required, True)
            self.assertEqual(field.type, "string")
            self.assertEqual(field.default, "hello")
            self.assertEqual(field.validation, {})

    def test_config_field_with_validation_parses(self):
        """Config field with valid 'validation.regex' parses successfully."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "with-val"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "with-val"\n'
                'name = "With Val"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                '\n'
                '[config.board_id.validation]\n'
                'regex = "^[0-9a-fA-F]{24}$"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            field = data.config_schema.fields["board_id"]
            self.assertEqual(field.required, True)
            self.assertEqual(field.validation, {"regex": "^[0-9a-fA-F]{24}$"})

    def test_config_field_with_unknown_validation_key_fails(self):
        """Config field with unknown key inside 'validation' table raises error."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-val"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bad-val"\n'
                'name = "Bad Val"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                '\n'
                '[config.board_id.validation]\n'
                'regex = "^[0-9a-fA-F]{24}$"\n'
                'min_length = 5\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("unknown key", str(ctx.exception))
            self.assertIn("min_length", str(ctx.exception))

    def test_config_field_with_validation_non_table_rejected(self):
        """Config field with 'validation' set to a non-table value raises error."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-val-type"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bad-val-type"\n'
                'name = "Bad Val Type"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                'validation = "not_a_table"\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("expected table", str(ctx.exception))
            self.assertIn("validation", str(ctx.exception))


    # --- [[provides.hooks]] runtime hooks ------------------------------------

    def _write_recipe(self, tmp: str, name: str, body: str, *, scripts: list[str] | None = None) -> Path:
        recipe_dir = Path(tmp) / name
        recipe_dir.mkdir()
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\n'
            f'id = "{name}"\n'
            f'name = "{name}"\n'
            f'description = "D"\n'
            f'version = "1.0"\n'
            f'\n{body}'
        )
        for rel in (scripts or []):
            sp = recipe_dir / rel
            sp.parent.mkdir(parents=True, exist_ok=True)
            sp.write_text("#!/usr/bin/env bash\nexit 0\n")
        return recipe_dir

    def test_provides_hooks_valid(self):
        """A valid [[provides.hooks]] entry parses and registers the hook."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "hk-valid",
                '[[provides.hooks]]\n'
                'id = "worktree-gate"\n'
                'event = "pre-tool-use"\n'
                'script = "hooks/worktree-gate.sh"\n'
                'matcher = "Edit|Write"\n'
                'blocking = true\n'
                'description = "Block writes"\n',
                scripts=["hooks/worktree-gate.sh"],
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(len(recipe.runtime_hooks), 1)
            hook = recipe.runtime_hooks[0]
            self.assertEqual(hook.id, "worktree-gate")
            self.assertEqual(hook.event, "pre-tool-use")
            self.assertEqual(hook.script, "hooks/worktree-gate.sh")
            self.assertEqual(hook.matcher, "Edit|Write")
            self.assertEqual(hook.blocking, True)
            self.assertEqual(hook.description, "Block writes")

    def test_provides_hooks_absent_ok(self):
        """A recipe with no [[provides.hooks]] parses and registers no runtime hooks."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "hk-none", "")
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(recipe.runtime_hooks, [])

    def test_provides_hooks_missing_field(self):
        """Omitting id/event/script raises an explicit error naming the field."""
        cases = [
            ('event = "pre-tool-use"\nscript = "hooks/x.sh"\n', "id"),
            ('id = "x"\nscript = "hooks/x.sh"\n', "event"),
            ('id = "x"\nevent = "pre-tool-use"\n', "script"),
        ]
        for body_inner, missing in cases:
            with self.subTest(missing=missing):
                with tempfile.TemporaryDirectory() as tmp:
                    recipe_dir = self._write_recipe(
                        tmp, "hk-miss",
                        "[[provides.hooks]]\n" + body_inner,
                        scripts=["hooks/x.sh"],
                    )
                    with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                        self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
                    self.assertIn(missing, str(ctx.exception))

    def test_provides_hooks_unknown_event(self):
        """An unknown event raises an error listing the known events."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "hk-evt",
                '[[provides.hooks]]\n'
                'id = "x"\n'
                'event = "bogus-event"\n'
                'script = "hooks/x.sh"\n',
                scripts=["hooks/x.sh"],
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("bogus-event", msg)
            self.assertIn("pre-tool-use", msg)

    def test_provides_hooks_script_escape(self):
        """A script path that escapes the recipe dir raises a path-escape error."""
        for bad in ("../evil.sh", "/etc/passwd"):
            with self.subTest(bad=bad):
                with tempfile.TemporaryDirectory() as tmp:
                    recipe_dir = self._write_recipe(
                        tmp, "hk-esc",
                        '[[provides.hooks]]\n'
                        'id = "x"\n'
                        'event = "pre-tool-use"\n'
                        f'script = "{bad}"\n',
                    )
                    with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                        self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
                    msg = str(ctx.exception).lower()
                    self.assertTrue(
                        "inside" in msg or "escape" in msg or "absolute" in msg,
                        f"expected a path-escape error, got: {msg}",
                    )

    def test_nonstandard_config_section_parses(self):
        """Non-standard config sections (without 'required' key) parse as extra data."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "ns-cfg"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "ns-cfg"\n'
                'name = "NS Cfg"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                '\n'
                '[config.board_isolation]\n'
                'forbidden_tools = ["trello_get_my_cards", "trello_list_boards"]\n'
                'restricted_tools = ["trello_set_active_board"]\n'
                'card_validation_required = true\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            # Standard field still works
            self.assertIn("board_id", data.config_schema.fields)
            self.assertEqual(data.config_schema.fields["board_id"].required, True)
            # Non-standard section stored in extra
            self.assertIn("board_isolation", data.config_schema.extra)
            isolation = data.config_schema.extra["board_isolation"]
            self.assertEqual(isolation["forbidden_tools"], ["trello_get_my_cards", "trello_list_boards"])
            self.assertEqual(isolation["restricted_tools"], ["trello_set_active_board"])
            self.assertEqual(isolation["card_validation_required"], True)


if __name__ == "__main__":
    unittest.main()
