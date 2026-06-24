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


class RecipeTagsConflictsTests(unittest.TestCase):
    """Card #27 — RED: Recipe.tags and Recipe.conflicts_with parsing/validation."""

    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_tags")

    def _write_recipe(self, tmp: str, name: str, extra: str = "") -> Path:
        recipe_dir = Path(tmp) / name
        recipe_dir.mkdir()
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\n'
            f'id = "{name}"\n'
            f'name = "{name}"\n'
            f'description = "D"\n'
            f'version = "1.0"\n'
            f'{extra}'
        )
        return recipe_dir

    def test_tags_and_conflicts_default_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "no-tags")
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(recipe.tags, [])
            self.assertEqual(recipe.conflicts_with, [])

    def test_tags_parsed(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "tagged", 'tags = ["vcs", "github"]\n')
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(recipe.tags, ["vcs", "github"])

    def test_conflicts_with_parsed(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "confl", 'conflicts_with = ["git-pr-flow", "gitlab-mr-flow"]\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(recipe.conflicts_with, ["git-pr-flow", "gitlab-mr-flow"])

    def test_tags_non_list_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "bad-tags", 'tags = "vcs"\n')
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("tags", str(ctx.exception))

    def test_tags_non_string_element_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "bad-tag-el", 'tags = ["vcs", 3]\n')
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("tags", str(ctx.exception))

    def test_conflicts_with_non_list_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "bad-confl", 'conflicts_with = "git-pr-flow"\n')
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("conflicts_with", str(ctx.exception))

    def test_conflicts_with_self_reference_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "selfref", 'conflicts_with = ["selfref"]\n')
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("conflicts_with", msg)
            self.assertIn("selfref", msg)

    def test_blank_tag_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "blank-tag", 'tags = ["vcs", ""]\n')
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("tags", str(ctx.exception))

    def test_blank_conflicts_with_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "blank-confl", 'conflicts_with = ["git-pr-flow", "  "]\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("conflicts_with", str(ctx.exception))


class BriefFragmentDataclassTests(unittest.TestCase):
    """Task 1.1 — RED: BriefFragment and BriefFragments dataclass structure."""

    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_bf_dc")

    def test_brief_fragment_default_key_is_none(self):
        """BriefFragment(text='Do X.') has key=None."""
        frag = self.schema.BriefFragment(text="Do X.")
        self.assertIsNone(frag.key)
        self.assertEqual(frag.text, "Do X.")

    def test_brief_fragment_explicit_key(self):
        """BriefFragment(text='Do Y.', key='foo') preserves key."""
        frag = self.schema.BriefFragment(text="Do Y.", key="foo")
        self.assertEqual(frag.key, "foo")
        self.assertEqual(frag.text, "Do Y.")

    def test_brief_fragments_defaults_all_none(self):
        """BriefFragments() has None for all six section fields."""
        bf = self.schema.BriefFragments()
        for section in ("runtime_flow", "context_sources", "conflict_policy",
                        "workflow_rules", "useful_commands", "mcp_descriptions"):
            self.assertIsNone(getattr(bf, section), f"Expected {section} to be None")

    def test_recipe_has_brief_fragments_field(self):
        """Recipe.brief_fragments field exists and defaults to None."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bf-test"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bf-test"\n'
                'name = "BF Test"\n'
                'description = "D"\n'
                'version = "1.0"\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertTrue(hasattr(recipe, "brief_fragments"))
            self.assertIsNone(recipe.brief_fragments)

    def test_recipe_brief_fragments_accepts_brief_fragments_object(self):
        """Recipe.brief_fragments field accepts a BriefFragments object."""
        bf = self.schema.BriefFragments(
            workflow_rules=[self.schema.BriefFragment(text="A rule.")]
        )
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bf-val-test"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bf-val-test"\n'
                'name = "BF Val Test"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[provides.brief]\n'
                'workflow_rules = ["A rule."]\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIsNotNone(recipe.brief_fragments)


class ParseBriefFragmentsHappyPathTests(unittest.TestCase):
    """Task 1.3 — RED: _parse_brief_fragments happy path cases."""

    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_bf_hp")

    def _write_recipe(self, tmp: str, name: str, body: str) -> Path:
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
        return recipe_dir

    def test_absent_provides_brief_returns_none(self):
        """Absent [provides.brief] -> brief_fragments is None."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "no-brief", "")
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIsNone(recipe.brief_fragments)

    def test_simple_array_normalizes_key_none(self):
        """Simple array form -> each string normalized to BriefFragment(key=None, text=s)."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "simple-arr",
                '[provides.brief]\n'
                'workflow_rules = ["Step one.", "Step two."]\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            bf = recipe.brief_fragments
            self.assertIsNotNone(bf)
            self.assertIsNotNone(bf.workflow_rules)
            self.assertEqual(len(bf.workflow_rules), 2)
            self.assertIsNone(bf.workflow_rules[0].key)
            self.assertEqual(bf.workflow_rules[0].text, "Step one.")
            self.assertIsNone(bf.workflow_rules[1].key)
            self.assertEqual(bf.workflow_rules[1].text, "Step two.")

    def test_simple_array_preserves_order(self):
        """Simple array form preserves declaration order."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "order-test",
                '[provides.brief]\n'
                'workflow_rules = ["First.", "Second.", "Third."]\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            texts = [f.text for f in recipe.brief_fragments.workflow_rules]
            self.assertEqual(texts, ["First.", "Second.", "Third."])

    def test_inline_table_form_normalizes_with_key(self):
        """Inline-table form -> BriefFragment(key=k, text=t), key preserved."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "inline-tbl",
                '[[provides.brief.context_sources]]\n'
                'key = "trello-source"\n'
                'text = "Trello is the source of truth."\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            bf = recipe.brief_fragments
            self.assertIsNotNone(bf)
            self.assertIsNotNone(bf.context_sources)
            self.assertEqual(len(bf.context_sources), 1)
            self.assertEqual(bf.context_sources[0].key, "trello-source")
            self.assertEqual(bf.context_sources[0].text, "Trello is the source of truth.")

    def test_both_sections_populated(self):
        """Both workflow_rules (array) and context_sources (inline-table) can coexist."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "both-secs",
                '[provides.brief]\n'
                'workflow_rules = ["Rule A."]\n'
                '[[provides.brief.context_sources]]\n'
                'key = "src-key"\n'
                'text = "Context here."\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            bf = recipe.brief_fragments
            self.assertIsNotNone(bf)
            self.assertEqual(len(bf.workflow_rules), 1)
            self.assertEqual(bf.workflow_rules[0].text, "Rule A.")
            self.assertEqual(len(bf.context_sources), 1)
            self.assertEqual(bf.context_sources[0].key, "src-key")

    def test_empty_array_is_valid(self):
        """Empty array [] is valid and produces zero fragments for that section."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "empty-arr",
                '[provides.brief]\n'
                'workflow_rules = []\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            bf = recipe.brief_fragments
            self.assertIsNotNone(bf)
            self.assertEqual(bf.workflow_rules, [])


class ParseBriefFragmentsValidationTests(unittest.TestCase):
    """Task 1.5 — RED: _parse_brief_fragments validation error cases."""

    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_bf_val")

    def _write_recipe(self, tmp: str, name: str, body: str) -> Path:
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
        return recipe_dir

    def test_intro_in_brief_raises_project_only_error(self):
        """`intro` in [provides.brief] -> validation error naming 'project-only section'."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "intro-err",
                '[provides.brief]\n'
                'intro = ["My intro."]\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("intro", msg)
            self.assertIn("project-only", msg)

    def test_purpose_in_brief_raises_project_only_error(self):
        """`purpose` in [provides.brief] -> validation error naming 'project-only section'."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "purpose-err",
                '[provides.brief]\n'
                'purpose = ["My purpose."]\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("purpose", msg)
            self.assertIn("project-only", msg)

    def test_unknown_section_raises_error_with_valid_list(self):
        """Unknown section name -> error naming the key and listing valid sections."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "unknown-sec",
                '[provides.brief]\n'
                'custom_section = ["A rule."]\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("custom_section", msg)
            # Should mention at least one valid section
            self.assertTrue(
                any(s in msg for s in ("workflow_rules", "runtime_flow", "context_sources")),
                f"Expected valid section names in error message: {msg}"
            )

    def test_inline_table_missing_text_raises_error(self):
        """Inline-table entry missing text -> error naming missing field 'text'."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "missing-text",
                '[[provides.brief.workflow_rules]]\n'
                'key = "foo"\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("text", msg)

    def test_inline_table_missing_key_raises_error(self):
        """Inline-table entry missing key -> error naming missing field 'key'."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "missing-key",
                '[[provides.brief.workflow_rules]]\n'
                'text = "A rule."\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            msg = str(ctx.exception)
            self.assertIn("key", msg)

    def test_mixed_forms_raises_error(self):
        """Mixed string-array and inline-table in same section -> error."""
        # TOML doesn't allow this directly in a single table, so we test the
        # validation logic by passing raw data programmatically.
        raw = {
            "workflow_rules": [
                "A string item.",  # string
                {"key": "foo", "text": "A table item."},  # dict
            ]
        }
        with self.assertRaises(self.schema.RecipeValidationError) as ctx:
            self.schema._parse_brief_fragments(raw, "[provides.brief]")
        msg = str(ctx.exception)
        self.assertIn("workflow_rules", msg)
        # "mixes" or "mixed" — implementation says "mixes string-array and inline-table forms"
        self.assertTrue("mix" in msg.lower(), f"Expected 'mix*' in error message: {msg}")


if __name__ == "__main__":
    unittest.main()
