"""Tests for recipe_schema.py dataclasses and validation."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import invoke, isolated_home  # noqa: E402

RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"


def _cli_home_with_recipes(register, recipe_tomls: dict[str, str]) -> Path:
    """isolated_home whose catalog/recipes holds exactly the given custom recipes."""
    tmp = tempfile.TemporaryDirectory()
    register(tmp.cleanup)
    home = isolated_home(Path(tmp.name))
    catalog = home / "catalog"
    catalog.unlink()
    recipes_dir = catalog / "recipes"
    recipes_dir.mkdir(parents=True)
    for rid, toml in recipe_tomls.items():
        (recipes_dir / rid).mkdir()
        (recipes_dir / rid / "recipe.toml").write_text(toml)
    return home


def _project_root(register, recipes_section: str = "") -> Path:
    """Minimal initialized project used by the read-only recipe verbs."""
    tmp = tempfile.TemporaryDirectory()
    register(tmp.cleanup)
    root = Path(tmp.name)
    ai_specs = root / "ai-specs"
    ai_specs.mkdir()
    (ai_specs / "skills").mkdir()
    (ai_specs / "commands").mkdir()
    (ai_specs / "ai-specs.toml").write_text(
        "[project]\nname = 'fixture'\n\n"
        "[agents]\nenabled = ['claude']\n\n"
        + recipes_section
    )
    return root


def _recipe_header(rid: str, name: str, body: str = "") -> str:
    """Recipe [recipe] front-matter plus an optional extra TOML body."""
    return (
        "[recipe]\n"
        f'id = "{rid}"\n'
        f'name = "{name}"\n'
        'description = "D"\n'
        'version = "1.0"\n'
        f"\n{body}"
    )


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

    def _list_recipe(self, recipes: dict[str, str]):
        """Shared wrapper: `ai-specs recipe list <project>` over a catalog holding `recipes`."""
        return invoke(
            _project_root(self._cleanup),
            "recipe", "list",
            cli_home=_cli_home_with_recipes(self._cleanup, recipes),
        )

    def _init_recipe(self, rid: str, toml: str):
        """Shared wrapper: `ai-specs recipe init <rid>` against a home holding one custom recipe."""
        return invoke(
            _project_root(self._cleanup),
            "recipe", "init", rid,
            cli_home=_cli_home_with_recipes(self._cleanup, {rid: toml}),
        )

    def _cleanup(self, handle):
        self.addCleanup(handle)

    def test_config_field_without_validation_parses(self):
        """Config field without 'validation' sub-table parses successfully."""
        # TRIAGE: required/type/default values and the validation dict defaulting
        # to {} are dataclass normalization; `ai-specs recipe list <project>`
        # surfaces only recipe id/name/version/status and no config-field detail.
        res = self._list_recipe({
            "no-val": _recipe_header("no-val", "No Val",
                '[config.my_field]\nrequired = true\ntype = "string"\ndefault = "hello"\n'),
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  no-val", res.stdout)
        self.assertNotIn("[error", res.stdout)
        self.assertIn("1.0", res.stdout)

    def test_config_field_with_validation_parses(self):
        """Config field with a valid 'validation.regex' parses correctly."""
        # TRIAGE: the parsed field's `validation == {"regex": ...}` representation
        # is internal; `ai-specs recipe list <project>` prints recipe availability
        # only, so parse success is the observable contract.
        res = self._list_recipe({
            "with-val": _recipe_header("with-val", "With Val",
                '[config.board_id]\nrequired = true\ntype = "string"\n\n'
                '[config.board_id.validation]\nregex = "^[0-9a-fA-F]{24}$"\n'),
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  with-val", res.stdout)

    def test_config_field_with_unknown_validation_key_fails(self):
        """Unknown key inside a 'validation' table raises an error naming it."""
        res = self._init_recipe("bad-val", _recipe_header(
            "bad-val", "Bad Val",
            '[config.board_id]\nrequired = true\ntype = "string"\n\n'
            '[config.board_id.validation]\nregex = "^[0-9a-fA-F]{24}$"\nmin_length = 5\n',
        ))
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("unknown key", res.stderr)
        self.assertIn("min_length", res.stderr)

    def test_config_field_with_validation_non_table_rejected(self):
        """Config field with 'validation' set to a non-table value is rejected."""
        res = self._init_recipe("bad-val-type", _recipe_header(
            "bad-val-type", "Bad Val Type",
            '[config.board_id]\nrequired = true\ntype = "string"\nvalidation = "not_a_table"\n',
        ))
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("expected table", res.stderr)
        self.assertIn("validation", res.stderr)

    def test_provides_hooks_valid(self):
        """A valid [[provides.hooks]] entry parses and the recipe stays available."""
        # TRIAGE: hook field values (id/event/script/matcher/blocking/description)
        # are not printed by `ai-specs recipe list <project>`; it reports hook
        # recipes only as available, so parse success is the observed contract.
        res = self._list_recipe({
            "hk-valid": _recipe_header("hk-valid", "Hk Valid",
                '[[provides.hooks]]\nid = "worktree-gate"\nevent = "pre-tool-use"\n'
                'script = "hooks/worktree-gate.sh"\nmatcher = "Edit|Write"\n'
                'blocking = true\ndescription = "Block writes"\n'),
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  hk-valid", res.stdout)
        self.assertNotIn("[error", res.stdout)
        self.assertIn("1.0", res.stdout)

    def test_provides_hooks_absent_ok(self):
        """A recipe with no [[provides.hooks]] parses and stays available."""
        res = self._list_recipe({"hk-none": _recipe_header("hk-none", "Hk None")})
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  hk-none", res.stdout)
        self.assertNotIn("[error", res.stdout)

    def test_provides_hooks_missing_field(self):
        """Omitting id/event/script raises an error naming the missing field."""
        cases = [
            ('event = "pre-tool-use"\nscript = "hooks/x.sh"\n', "id"),
            ('id = "x"\nscript = "hooks/x.sh"\n', "event"),
            ('id = "x"\nevent = "pre-tool-use"\n', "script"),
        ]
        for body_inner, missing in cases:
            with self.subTest(missing=missing):
                res = self._init_recipe(
                    "hk-miss",
                    _recipe_header("hk-miss", "Hk Miss", "[[provides.hooks]]\n" + body_inner),
                )
                self.assertNotEqual(res.returncode, 0)
                self.assertIn(f"required field '{missing}'", res.stderr)

    def test_provides_hooks_unknown_event(self):
        """An unknown event raises an error listing the known events."""
        res = self._init_recipe("hk-evt", _recipe_header(
            "hk-evt", "Hk Evt",
            '[[provides.hooks]]\nid = "x"\nevent = "bogus-event"\nscript = "hooks/x.sh"\n',
        ))
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("bogus-event", res.stderr)
        self.assertIn("pre-tool-use", res.stderr)

    def test_provides_hooks_script_escape(self):
        """A script path escaping the recipe dir raises a path-escape error."""
        for bad in ("../evil.sh", "/etc/passwd"):
            with self.subTest(bad=bad):
                res = self._init_recipe("hk-esc", _recipe_header(
                    "hk-esc", "Hk Esc",
                    '[[provides.hooks]]\nid = "x"\nevent = "pre-tool-use"\n'
                    f'script = "{bad}"\n',
                ))
                self.assertNotEqual(res.returncode, 0)
                msg = res.stderr.lower()
                self.assertTrue(
                    "inside" in msg or "escape" in msg or "absolute" in msg,
                    f"expected a path-escape error, got: {msg}",
                )

    def test_nonstandard_config_section_parses(self):
        """Non-standard config sections parse as extra data and stay available."""
        # TRIAGE: the parsed extra-section structure (board_isolation keys) lives
        # only in the dataclass; `ai-specs recipe list <project>` prints the
        # recipe as available and nothing about config.extra contents.
        res = self._list_recipe({
            "ns-cfg": _recipe_header("ns-cfg", "NS Cfg",
                '[config.board_id]\nrequired = true\ntype = "string"\n\n'
                '[config.board_isolation]\n'
                'forbidden_tools = ["trello_get_my_cards", "trello_list_boards"]\n'
                'restricted_tools = ["trello_set_active_board"]\n'
                'card_validation_required = true\n'),
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  ns-cfg", res.stdout)
        self.assertNotIn("[error", res.stdout)
        self.assertIn("1.0", res.stdout)

    def test_boolean_type_normalizes_to_bool(self):
        """`type = "boolean"` normalizes to "bool" in the parsed dataclass."""
        # TRIAGE: no CLI verb prints the normalized config-field type; ran
        # `ai-specs recipe list <project>` and `ai-specs recipe init <id>` — the
        # "boolean" -> "bool" alias lives only in the recipe_schema dataclass.
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bool-alias"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bool-alias"\n'
                'name = "Bool Alias"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.auto_switch_account]\n'
                'required = false\n'
                'type = "boolean"\n'
                'default = false\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            field = data.config_schema.fields["auto_switch_account"]
            self.assertEqual(field.type, "bool")


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

    def _init_recipe(self, rid: str, body: str = ""):
        """Shared wrapper: `ai-specs recipe init <rid>` over a home holding one custom recipe."""
        return invoke(
            _project_root(self.addCleanup),
            "recipe", "init", rid,
            cli_home=_cli_home_with_recipes(self.addCleanup, {rid: _recipe_header(rid, rid, body)}),
        )

    def _sync_recipes(self, bodies: dict[str, str]):
        """Shared wrapper: `ai-specs sync <project>` enabling every custom recipe."""
        home = _cli_home_with_recipes(self.addCleanup, {
            rid: _recipe_header(rid, rid, body) for rid, body in bodies.items()
        })
        enabled = "".join(f"[recipes.{rid}]\nenabled = true\n" for rid in bodies)
        root = _project_root(self.addCleanup, enabled)
        return invoke(root, "sync", cli_home=home)

    def test_tags_and_conflicts_default_empty(self):
        """A recipe without tags/conflicts_with parses to empty lists."""
        # TRIAGE: the `tags == []` / `conflicts_with == []` defaults are dataclass
        # normalization; `ai-specs recipe list`, `recipe init <id>`, and `sync`
        # were all run and none prints tag/conflict metadata for a tag-less recipe.
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(tmp, "no-tags")
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(recipe.tags, [])
            self.assertEqual(recipe.conflicts_with, [])

    def test_tags_parsed(self):
        """Declared tags parse; sync's tag-overlap scan then names each tag value."""
        res = self._sync_recipes({
            "tag-a": 'tags = ["vcs", "github"]\n',
            "tag-b": 'tags = ["vcs", "github"]\n',
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("share tag 'vcs'", res.stderr)
        self.assertIn("share tag 'github'", res.stderr)

    def test_conflicts_with_parsed(self):
        """Declared conflicts_with parses; no CLI verb prints the parsed list."""
        # TRIAGE: the parsed `conflicts_with` list is not surfaced by any verb;
        # `ai-specs sync` enabling a recipe whose conflicts_with names another
        # enabled recipe exits 0 with no conflict output, so the parsed values
        # live only in the recipe_schema dataclass.
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = self._write_recipe(
                tmp, "confl", 'conflicts_with = ["git-pr-flow", "gitlab-mr-flow"]\n'
            )
            recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(recipe.conflicts_with, ["git-pr-flow", "gitlab-mr-flow"])

    def test_tags_non_list_rejected(self):
        """A scalar `tags` value is rejected with an array-typed error."""
        res = self._init_recipe("bad-tags", 'tags = "vcs"\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("'tags' must be an array of strings", res.stderr)

    def test_tags_non_string_element_rejected(self):
        """A non-string element inside `tags` is rejected naming the index."""
        res = self._init_recipe("bad-tag-el", 'tags = ["vcs", 3]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("'tags'[1] must be a string", res.stderr)

    def test_conflicts_with_non_list_rejected(self):
        """A scalar `conflicts_with` value is rejected as a non-array."""
        res = self._init_recipe("bad-confl", 'conflicts_with = "git-pr-flow"\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("'conflicts_with' must be an array of strings", res.stderr)

    def test_conflicts_with_self_reference_rejected(self):
        """A recipe cannot list itself in conflicts_with."""
        res = self._init_recipe("selfref", 'conflicts_with = ["selfref"]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("conflicts_with", res.stderr)
        self.assertIn("selfref", res.stderr)

    def test_blank_tag_rejected(self):
        """A blank string inside `tags` is rejected."""
        res = self._init_recipe("blank-tag", 'tags = ["vcs", ""]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("'tags'[1] must be a non-empty string", res.stderr)

    def test_blank_conflicts_with_rejected(self):
        """A blank string inside `conflicts_with` is rejected."""
        res = self._init_recipe("blank-confl", 'conflicts_with = ["git-pr-flow", "  "]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("'conflicts_with'[1] must be a non-empty string", res.stderr)


class BriefFragmentDataclassTests(unittest.TestCase):
    """Task 1.1 — RED: BriefFragment and BriefFragments dataclass structure."""

    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_bf_dc")

    def test_brief_fragment_default_key_is_none(self):
        """BriefFragment(text='Do X.') has key=None."""
        # TRIAGE: BriefFragment is constructed programmatically, so no CLI surface
        # can exercise it; `ai-specs recipe list`/`recipe init`/`sync` render brief
        # fragments but never expose per-fragment key defaults.
        frag = self.schema.BriefFragment(text="Do X.")
        self.assertIsNone(frag.key)
        self.assertEqual(frag.text, "Do X.")

    def test_brief_fragment_explicit_key(self):
        """BriefFragment(text='Do Y.', key='foo') preserves key."""
        # TRIAGE: the constructed key is dataclass state, not CLI output; the same
        # three `ai-specs` verbs used above do not print fragment keys.
        frag = self.schema.BriefFragment(text="Do Y.", key="foo")
        self.assertEqual(frag.key, "foo")
        self.assertEqual(frag.text, "Do Y.")

    def test_brief_fragments_defaults_all_none(self):
        """BriefFragments() has None for all six section fields."""
        # TRIAGE: the six-section None defaults are dataclass structure; rendered
        # AGENTS.md sections (from `ai-specs sync <project>`) omit rather than
        # spell out absent sections, so these assertions must stay coupled.
        bf = self.schema.BriefFragments()
        for section in ("runtime_flow", "context_sources", "conflict_policy",
                        "workflow_rules", "useful_commands", "mcp_descriptions"):
            self.assertIsNone(getattr(bf, section), f"Expected {section} to be None")

    def test_recipe_has_brief_fragments_field(self):
        """Recipe.brief_fragments field exists and defaults to None."""
        # TRIAGE: `brief_fragments is None` for a recipe without [provides.brief]
        # is internal state; `ai-specs recipe list <project>` just prints the
        # recipe as available and `ai-specs sync` renders no brief sections.
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
        # TRIAGE: the accepted BriefFragments object is constructed in-process;
        # no CLI verb can pass one, so the field-type acceptance assertion is
        # not reachable through `ai-specs recipe list`/`init`/`sync`.
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
    """Task 1.3 — RED: _parse_brief_fragments happy path cases, driven via sync."""

    def _sync_brief(self, rid: str, body: str):
        """Shared wrapper: `ai-specs sync <project>` enabling one custom brief recipe."""
        home = _cli_home_with_recipes(self.addCleanup, {
            rid: _recipe_header(rid, rid, body),
        })
        root = _project_root(self.addCleanup, f"[recipes.{rid}]\nenabled = true\n")
        result = invoke(root, "sync", cli_home=home)
        return result, root

    def test_absent_provides_brief_returns_none(self):
        """Absent [provides.brief] renders no brief sections at all."""
        # TRIAGE: the `brief_fragments is None` datum lives in the dataclass;
        # `ai-specs sync <project>` renders the absence as omitted AGENTS.md
        # sections, which is the observable contract asserted below.
        res, root = self._sync_brief("no-brief", "")
        agents = (root / "AGENTS.md").read_text()
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertNotIn("## Runtime Flow", agents)
        self.assertNotIn("## Workflow Rules", agents)
        self.assertNotIn("## Context Sources", agents)

    def test_simple_array_normalizes_key_none(self):
        """Simple array form renders every fragment, in order, as a bullet."""
        # TRIAGE: per-fragment key normalization (key=None) is not rendered;
        # `ai-specs sync <project>` surfaces only the fragment texts as bullets,
        # which is what is asserted here.
        res, root = self._sync_brief(
            "simple-arr",
            '[provides.brief]\nworkflow_rules = ["Step one.", "Step two."]\n',
        )
        agents = (root / "AGENTS.md").read_text()
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("## Workflow Rules", agents)
        self.assertIn("- Step one.", agents)
        self.assertIn("- Step two.", agents)
        self.assertNotIn("- Step three.", agents)

    def test_simple_array_preserves_order(self):
        """Simple array form preserves declaration order in the rendered brief."""
        res, root = self._sync_brief(
            "order-test",
            '[provides.brief]\nworkflow_rules = ["First.", "Second.", "Third."]\n',
        )
        agents = (root / "AGENTS.md").read_text()
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("- First.", agents)
        self.assertLess(agents.index("- First."), agents.index("- Second."))
        self.assertLess(agents.index("- Second."), agents.index("- Third."))

    def test_inline_table_form_normalizes_with_key(self):
        """Inline-table form renders its text under the context_sources section."""
        # TRIAGE: preservation of the declared key is dataclass-internal; the
        # frozen context_sources renderer drops fragment keys (`ai-specs sync
        # <project>` renders only the text bullet), so the key stays coupled.
        res, root = self._sync_brief(
            "inline-tbl",
            '[[provides.brief.context_sources]]\n'
            'key = "trello-source"\n'
            'text = "Trello is the source of truth."\n',
        )
        agents = (root / "AGENTS.md").read_text()
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("## Context Sources", agents)
        self.assertIn("- Trello is the source of truth.", agents)

    def test_both_sections_populated(self):
        """workflow_rules and context_sources can coexist and both render."""
        res, root = self._sync_brief(
            "both-secs",
            '[provides.brief]\nworkflow_rules = ["Rule A."]\n'
            '[[provides.brief.context_sources]]\n'
            'key = "src-key"\n'
            'text = "Context here."\n',
        )
        agents = (root / "AGENTS.md").read_text()
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("## Workflow Rules", agents)
        self.assertIn("- Rule A.", agents)
        self.assertIn("## Context Sources", agents)
        self.assertIn("- Context here.", agents)
        self.assertNotIn("## Runtime Flow", agents)

    def test_empty_array_is_valid(self):
        """Empty array is valid and renders no section for that key."""
        # TRIAGE: the zero-length fragment list is internal; `ai-specs sync
        # <project>` renders nothing for an empty brief section, which is the
        # observable equivalent asserted below.
        res, root = self._sync_brief(
            "empty-arr",
            '[provides.brief]\nworkflow_rules = []\n',
        )
        agents = (root / "AGENTS.md").read_text()
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertNotIn("## Workflow Rules", agents)


class ParseBriefFragmentsValidationTests(unittest.TestCase):
    """Task 1.5 — RED: _parse_brief_fragments validation error cases."""

    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_bf_val")

    def _init_recipe(self, rid: str, body: str):
        """Shared wrapper: `ai-specs recipe init <rid>` over a home holding one custom recipe."""
        return invoke(
            _project_root(self.addCleanup),
            "recipe", "init", rid,
            cli_home=_cli_home_with_recipes(self.addCleanup, {rid: _recipe_header(rid, rid, body)}),
        )

    def test_intro_in_brief_raises_project_only_error(self):
        """`intro` in [provides.brief] is rejected as a project-only section."""
        res = self._init_recipe("intro-err", '[provides.brief]\nintro = ["My intro."]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("intro", res.stderr)
        self.assertIn("project-only", res.stderr)

    def test_purpose_in_brief_raises_project_only_error(self):
        """`purpose` in [provides.brief] is rejected as a project-only section."""
        res = self._init_recipe("purpose-err", '[provides.brief]\npurpose = ["My purpose."]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("purpose", res.stderr)
        self.assertIn("project-only", res.stderr)

    def test_unknown_section_raises_error_with_valid_list(self):
        """Unknown section name -> error naming the key and listing valid sections."""
        res = self._init_recipe("unknown-sec", '[provides.brief]\ncustom_section = ["A rule."]\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("custom_section", res.stderr)
        self.assertTrue(
            any(s in res.stderr for s in ("workflow_rules", "runtime_flow", "context_sources")),
            f"Expected valid section names in error message: {res.stderr}",
        )

    def test_inline_table_missing_text_raises_error(self):
        """Inline-table entry missing text -> error naming missing field 'text'."""
        res = self._init_recipe(
            "missing-text",
            '[[provides.brief.workflow_rules]]\nkey = "foo"\n',
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("required field 'text'", res.stderr)

    def test_inline_table_missing_key_raises_error(self):
        """Inline-table entry missing key -> error naming missing field 'key'."""
        res = self._init_recipe(
            "missing-key",
            '[[provides.brief.workflow_rules]]\ntext = "A rule."\n',
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("required field 'key'", res.stderr)

    def test_mixed_forms_raises_error(self):
        """Mixed string-array and inline-table in the same section -> error."""
        # TOML cannot express a mixed string-array + inline-table under one
        # brief key, so the original passed a raw dict to the internal parser;
        # `ai-specs recipe list`/`recipe init <id>` read only parsed TOML and
        # cannot feed this case, which stays coupled.
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
        self.assertTrue("mix" in msg.lower(), f"Expected 'mix*' in error message: {msg}")



class CliDepParsingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_clidep")
        cls.read_mod = load_module(RECIPE_READ_PATH, "recipe_read_clidep")

    def _list_recipe(self, recipes: dict[str, str]):
        """Shared wrapper: `ai-specs recipe list <project>` over a catalog holding `recipes`."""
        return invoke(
            _project_root(self.addCleanup),
            "recipe", "list",
            cli_home=_cli_home_with_recipes(self.addCleanup, recipes),
        )

    def _init_recipe(self, rid: str, body: str):
        """Shared wrapper: `ai-specs recipe init <rid>` over a home holding one custom recipe."""
        return invoke(
            _project_root(self.addCleanup),
            "recipe", "init", rid,
            cli_home=_cli_home_with_recipes(self.addCleanup, {rid: _recipe_header(rid, rid, body)}),
        )

    def test_valid_cli_dep_parses(self):
        """A fully-specified [[deps.cli]] entry parses; the recipe stays available."""
        # TRIAGE: the per-field values (binary/purpose/required/install_url/
        # version_check/min_version) are not printed by `ai-specs recipe list
        # <project>`; it exposes only id/name/version/status for a valid recipe.
        res = self._list_recipe({
            "full-dep": _recipe_header("full-dep", "Full Dep",
                '[[deps.cli]]\nbinary = "gh"\npurpose = "Create PRs"\nrequired = true\n'
                'install_url = "https://cli.github.com/"\nversion_check = "gh --version"\n'
                'min_version = "2.0.0"\n'),
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  full-dep", res.stdout)
        self.assertNotIn("[error", res.stdout)
        self.assertIn("1.0", res.stdout)

    def test_optional_defaults(self):
        """A minimal [[deps.cli]] entry parses with only binary and purpose."""
        # TRIAGE: the derived defaults (required=True, empty url/check/version)
        # are dataclass normalization; the CLI asserts parse success only.
        res = self._list_recipe({
            "defaults-dep": _recipe_header("defaults-dep", "Defaults Dep",
                '[[deps.cli]]\nbinary = "jq"\npurpose = "Parse JSON"\n'),
        })
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  defaults-dep", res.stdout)
        self.assertNotIn("[error", res.stdout)

    def test_missing_binary_raises(self):
        """A [[deps.cli]] entry without binary is rejected naming the field."""
        res = self._init_recipe("missing-binary", '[[deps.cli]]\npurpose = "Create PRs"\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("missing or invalid required field 'binary'", res.stderr)

    def test_missing_purpose_raises(self):
        """A [[deps.cli]] entry without purpose is rejected naming the field."""
        res = self._init_recipe("missing-purpose", '[[deps.cli]]\nbinary = "gh"\n')
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("missing or invalid required field 'purpose'", res.stderr)

    def test_unknown_key_raises(self):
        """An unknown key inside [[deps.cli]] is rejected naming the key."""
        res = self._init_recipe(
            "unknown-key",
            '[[deps.cli]]\nbinary = "gh"\npurpose = "Create PRs"\nfoo = "x"\n',
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("unknown key", res.stderr)
        self.assertIn("foo", res.stderr)

    def test_required_non_bool_raises(self):
        """A non-boolean `required` value inside [[deps.cli]] is rejected."""
        res = self._init_recipe(
            "bad-required",
            '[[deps.cli]]\nbinary = "gh"\npurpose = "Create PRs"\nrequired = "yes"\n',
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("expected boolean", res.stderr)
        self.assertIn("required", res.stderr)

    def test_absent_deps_yields_empty_list(self):
        """A recipe without [[deps.cli]] parses; deps are then empty internally."""
        # TRIAGE: `cli_deps == []` is dataclass state; `ai-specs recipe list
        # <project>` prints the deps-less recipe as available, the observable
        # contract asserted here.
        res = self._list_recipe({"no-deps": _recipe_header("no-deps", "No Deps")})
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("[available   ]  no-deps", res.stdout)
        self.assertNotIn("[error", res.stdout)

    def test_recipe_to_dict_serializes_cli_deps(self):
        """recipe_to_dict serialization shape of cli_deps stays coupled."""
        # TRIAGE: recipe_to_dict is an internal serializer; no CLI verb emits its
        # full dict — `ai-specs recipe list` derives only id/name/version from it
        # and `recipe init <id>` prints the init brief, not this structure.
        recipe = self.schema.Recipe(
            id="x",
            name="X",
            description="D",
            version="1.0",
            cli_deps=[
                self.schema.CliDep(
                    binary="gh",
                    purpose="PRs",
                    required=True,
                    install_url="https://cli.github.com/",
                    version_check="gh --version",
                    min_version="2.0.0",
                )
            ],
        )
        data = self.read_mod.recipe_to_dict(recipe)
        self.assertIn("cli_deps", data)
        self.assertEqual(
            data["cli_deps"],
            [
                {
                    "binary": "gh",
                    "purpose": "PRs",
                    "required": True,
                    "install_url": "https://cli.github.com/",
                    "version_check": "gh --version",
                    "min_version": "2.0.0",
                }
            ],
        )

    def test_catalog_git_pr_flow_has_cli_deps(self):
        """The shipped git-pr-flow recipe keeps its gh dep (coupled)."""
        # TRIAGE: the cli_deps entries of shipped catalog recipes are not printed
        # by any verb; `ai-specs recipe list <project>` shows them only as
        # available, so the binary/required assertions must stay coupled.
        catalog = ROOT / "catalog" / "recipes"
        recipe = self.schema.load_recipe_toml(catalog / "git-pr-flow" / "recipe.toml")
        self.assertGreaterEqual(len(recipe.cli_deps), 1)
        self.assertEqual(recipe.cli_deps[0].binary, "gh")
        self.assertIs(recipe.cli_deps[0].required, True)

    def test_catalog_gitlab_mr_flow_has_two_deps(self):
        """The shipped gitlab-mr-flow recipe declares glab and jq (coupled)."""
        # TRIAGE: multi-dep content is dataclass state; `ai-specs recipe list
        # <project>` exposes only availability for catalog recipes.
        catalog = ROOT / "catalog" / "recipes"
        recipe = self.schema.load_recipe_toml(catalog / "gitlab-mr-flow" / "recipe.toml")
        self.assertEqual(len(recipe.cli_deps), 2)
        self.assertEqual([d.binary for d in recipe.cli_deps], ["glab", "jq"])

    def test_catalog_worktree_flow_has_git_dep(self):
        """The shipped worktree-flow recipe declares a git dep (coupled)."""
        # TRIAGE: `cli_deps[0].binary == "git"` is not surfaced; only the parse
        # status of the catalog recipe is, via `ai-specs recipe list <project>`.
        catalog = ROOT / "catalog" / "recipes"
        recipe = self.schema.load_recipe_toml(catalog / "worktree-flow" / "recipe.toml")
        self.assertEqual(recipe.cli_deps[0].binary, "git")

    def test_recipe_conflicts_tolerates_deps_block(self):
        """Conflict checking tolerates recipes declaring [[deps.cli]] (coupled)."""
        # TRIAGE: check_recipe_conflicts has no CLI caller; `ai-specs sync`
        # runs the tag-conflict scan, not this registry check, so the assertion
        # that conflict detection tolerates deps blocks stays coupled.
        conflicts_mod = load_module(
            ROOT / "lib" / "_internal" / "recipe-conflicts.py",
            "recipe_conflicts_clidep",
        )
        catalog = ROOT / "catalog" / "recipes"
        # Must not crash when recipes declare [[deps.cli]].
        conflicts = conflicts_mod.check_recipe_conflicts(
            catalog, ["git-pr-flow", "worktree-flow"]
        )
        self.assertIsInstance(conflicts, list)



if __name__ == "__main__":
    unittest.main()
