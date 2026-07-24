import importlib.util
import json
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
RULES_AUDIT_PY = ROOT / "lib" / "_internal" / "rules-inventory.py"
REFRESH_BUNDLED_PY = ROOT / "lib" / "_internal" / "refresh-bundled.py"


def load_rules_inventory():
    spec = importlib.util.spec_from_file_location("rules_inventory", RULES_AUDIT_PY)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def snapshot_fs(root: Path) -> dict[str, tuple[int, int]]:
    state: dict[str, tuple[int, int]] = {}
    for path in root.rglob("*"):
        if path.is_file():
            rel = str(path.relative_to(root))
            stat = path.stat()
            state[rel] = (stat.st_mtime_ns, stat.st_size)
    return state


class RulesAuditTests(unittest.TestCase):
    def setUp(self):
        if not RULES_AUDIT_PY.is_file():
            self.skipTest("rules-inventory.py not implemented yet")
        self.mod = load_rules_inventory()

    def _scan(self, root: Path) -> dict:
        return self.mod.RulesInventory(root).scan()

    def _scan_stdout(self, root: Path) -> dict:
        result = subprocess.run(
            [sys.executable, str(RULES_AUDIT_PY), str(root)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)

    def _write_mode_a_fixture(self, root: Path, *, include_cursorrules: bool = True) -> None:
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "api.mdc").write_text(
            "---\n"
            "description: API conventions\n"
            "globs: [apps/api/**]\n"
            "alwaysApply: false\n"
            "---\n"
            "Use strict typing in handlers.\n"
        )
        if include_cursorrules:
            (root / ".cursorrules").write_text("Legacy monolithic rules.\n")
        (root / "AGENTS.md").write_text(
            "# AGENTS\n\n## Workflow Rules\n\nAlways run tests in a worktree.\n"
        )
        manifest_dir = root / "ai-specs"
        manifest_dir.mkdir()
        (manifest_dir / "ai-specs.toml").write_text(
            '[project]\nname = "fixture"\n\n[agents]\nenabled = ["cursor"]\n'
        )

    def test_placeholder(self):
        self.assertTrue(RULES_AUDIT_PY.is_file())

    def test_read_only_invariant(self):
        root = Path(self._fixture_dir())
        self._write_mode_a_fixture(root)
        before = snapshot_fs(root)
        self._scan(root)
        after = snapshot_fs(root)
        self.assertEqual(before, after)

    def test_json_shape(self):
        root = Path(self._fixture_dir())
        self._write_mode_a_fixture(root)
        data = self._scan_stdout(root)
        for key in (
            "schema_version",
            "mode",
            "target",
            "sources",
            "summary",
            "classification_is_suggestion",
        ):
            self.assertIn(key, data)

    def test_mode_a_detection(self):
        root = Path(self._fixture_dir())
        self._write_mode_a_fixture(root)
        data = self._scan(root)
        self.assertEqual(data["mode"], "A")
        self.assertTrue(data["sources"]["cursor_rules"])

    def test_mode_b_detection(self):
        root = Path(self._fixture_dir())
        (root / "package-lock.json").write_text("{}")
        data = self._scan(root)
        self.assertEqual(data["mode"], "B")
        self.assertIsInstance(data["stack_hints"], list)

    def test_mode_b_with_agents_md_is_mode_a(self):
        root = Path(self._fixture_dir())
        (root / "package-lock.json").write_text("{}")
        (root / "AGENTS.md").write_text("# Runtime brief\n")
        data = self._scan(root)
        self.assertEqual(data["mode"], "A")

    def test_benign_rule_has_no_false_recipe_matches(self):
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "benign.mdc").write_text(
            "---\ndescription: Preferred approach for processing\n---\n"
            "Keep changes minimal.\n"
        )
        data = self._scan(root)
        item = data["sources"]["cursor_rules"][0]
        self.assertEqual(item["candidate_recipes"], [])

    def test_always_apply_false_from_string_frontmatter(self):
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "scoped.mdc").write_text(
            "---\n"
            "description: Scoped rule\n"
            "alwaysApply: false\n"
            "---\n"
            "Body.\n"
        )
        data = self._scan(root)
        item = data["sources"]["cursor_rules"][0]
        self.assertFalse(item["always_apply"])

    def test_tolerant_frontmatter_extracts_despite_bad_line(self):
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "partial.mdc").write_text(
            "---\n"
            "description: API conventions\n"
            "globs: [apps/api/**]\n"
            "  unsupported: nested\n"
            "alwaysApply: false\n"
            "---\n"
            "Body.\n"
        )
        data = self._scan(root)
        item = data["sources"]["cursor_rules"][0]
        self.assertEqual(item["description"], "API conventions")
        self.assertEqual(item["globs"], ["apps/api/**"])
        self.assertFalse(item["always_apply"])

    def test_missing_sources_absent(self):
        root = Path(self._fixture_dir())
        self._write_mode_a_fixture(root, include_cursorrules=False)
        data = self._scan(root)
        cursorrules = data["sources"]["cursorrules"]
        if isinstance(cursorrules, dict):
            self.assertEqual(cursorrules.get("status"), "absent")
        else:
            self.assertTrue(
                any(item.get("status") == "absent" for item in cursorrules)
            )

    def test_manifest_recipes_table_schema_enabled(self):
        root = Path(self._fixture_dir())
        manifest_dir = root / "ai-specs"
        manifest_dir.mkdir()
        (manifest_dir / "ai-specs.toml").write_text(
            '[project]\nname = "fixture"\n\n'
            "[recipes.worktree-flow]\n"
            "enabled = true\n"
        )
        data = self._scan(root)
        recipes = data["sources"]["manifest"]["recipes"]
        self.assertIn("worktree-flow", recipes)

    def test_standalone_keywords_do_not_false_positive(self):
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        for name, body in (
            ("write-test.mdc", "Write a test plan before shipping.\n"),
            ("use-git.mdc", "Use git status to verify the tree.\n"),
        ):
            (rules_dir / name).write_text(f"---\ndescription: Policy\n---\n{body}")
        data = self._scan(root)
        for item in data["sources"]["cursor_rules"]:
            self.assertEqual(item["candidate_recipes"], [], item["path"])

    def test_project_heading_with_vault_is_keep_in_brief(self):
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "# AGENTS\n\n## Project\n\nUse the Vault for canonical notes.\n"
        )
        data = self._scan(root)
        section = next(
            s for s in data["sources"]["agents_md_sections"] if s.get("heading") == "Project"
        )
        self.assertEqual(section["classification"], "keep_in_brief")

    def test_agents_md_h1_h3_and_preamble_sections(self):
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "Intro before headings.\n\n"
            "# Runtime Brief\n\n"
            "Top-level body.\n\n"
            "### Workflow Rules\n\n"
            "Nested workflow text.\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        self.assertGreater(len(sections), 0)
        headings = {s.get("heading") for s in sections}
        self.assertIn("Runtime Brief", headings)
        self.assertIn("Workflow Rules", headings)

    def test_keyword_heuristic(self):
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "testing.mdc").write_text(
            "---\ndescription: TDD policy\n---\n"
            "Always run tdd in a worktree before opening a pull request.\n"
        )
        data = self._scan(root)
        item = data["sources"]["cursor_rules"][0]
        recipes = set(item["candidate_recipes"])
        self.assertIn("tdd-flow", recipes)
        self.assertIn("worktree-flow", recipes)

    def test_cli_help_lists_rules_audit(self):
        result = subprocess.run(
            [str(CLI), "help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertIn("rules-audit", result.stdout)

    def test_bundled_commands_distribution_after_refresh(self):
        import shutil
        import tempfile

        pc_spec = importlib.util.spec_from_file_location(
            "pc_rules_audit_cmd_dist", ROOT / "lib" / "_internal" / "project-cache.py"
        )
        pc = importlib.util.module_from_spec(pc_spec)
        assert pc_spec.loader is not None
        pc_spec.loader.exec_module(pc)

        project = Path(tempfile.mkdtemp())
        try:
            (project / "ai-specs").mkdir()
            subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED_PY), str(project), str(ROOT), "--init"],
                check=True,
                capture_output=True,
                text=True,
            )
            # Bundled commands flatten into the cache — never the project surface.
            bundled_commands_root = pc.bundled_commands_root(project, cli_home=ROOT)
            self.assertTrue((bundled_commands_root / "rules-audit.md").is_file())
            skills_text = (bundled_commands_root / "skills-as-rules.md").read_text(encoding="utf-8")
            self.assertNotIn("auto-invoke table", skills_text.lower())
            local_commands_dir = project / "ai-specs" / "commands"
            local_names = (
                sorted(p.name for p in local_commands_dir.glob("*.md"))
                if local_commands_dir.is_dir() else []
            )
            self.assertEqual(
                local_names, [],
                "bundled commands must not be materialized into ai-specs/commands/",
            )

            subprocess.run([str(CLI), "init", str(project)], check=True, text=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['cursor', 'opencode']\n"
            )
            subprocess.run([str(CLI), "sync", str(project)], check=True, text=True)

            # A project with no local commands still has an empty/absent
            # ai-specs/commands/ after init+sync — bundled commands never land there.
            local_names = (
                sorted(p.name for p in local_commands_dir.glob("*.md"))
                if local_commands_dir.is_dir() else []
            )
            self.assertEqual(local_names, [])
            for rel in (
                ".cursor/commands/rules-audit.md",
                ".cursor/commands/skills-as-rules.md",
                ".opencode/commands/rules-audit.md",
                ".opencode/commands/skills-as-rules.md",
            ):
                path = project / rel
                self.assertTrue(path.is_file(), rel)
                if path.name == "skills-as-rules.md":
                    self.assertNotIn("auto-invoke table", path.read_text(encoding="utf-8").lower())
        finally:
            shutil.rmtree(project)

    def test_cli_missing_path_exits_nonzero(self):
        result = subprocess.run(
            [str(CLI), "rules-audit", "/nonexistent-path-rules-audit"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(result.stderr.strip())

    # ------------------------------------------------------------------ N1
    def test_agents_md_no_headings_still_produces_section(self):
        """N1: monolithic AGENTS.md with no markdown headings must not be dropped."""
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "Use the Vault for canonical notes.\n"
            "Always run tests before opening a PR.\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        self.assertGreaterEqual(len(sections), 1)

    # ------------------------------------------------------------------ N2
    def test_run_tests_alone_does_not_match_tdd_flow(self):
        """N2: generic 'run tests' text must NOT trigger tdd-flow."""
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "ci.mdc").write_text(
            "---\ndescription: CI policy\n---\n"
            "CI pipeline must run tests on every push.\n"
        )
        data = self._scan(root)
        item = data["sources"]["cursor_rules"][0]
        self.assertNotIn("tdd-flow", item["candidate_recipes"])

    def test_tdd_intentional_phrases_still_match_tdd_flow(self):
        """N2: intentional TDD phrases must still match tdd-flow."""
        root = Path(self._fixture_dir())
        rules_dir = root / ".cursor" / "rules"
        rules_dir.mkdir(parents=True)
        (rules_dir / "tdd.mdc").write_text(
            "---\ndescription: TDD policy\n---\n"
            "Follow test-driven development: red-green-refactor cycle.\n"
        )
        data = self._scan(root)
        item = data["sources"]["cursor_rules"][0]
        self.assertIn("tdd-flow", item["candidate_recipes"])

    # ------------------------------------------------------------------ keep_in_brief word-boundary
    def test_reproject_heading_does_not_match_project_token(self):
        """_classify keep_in_brief guard must use word boundary, not substring match."""
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "# AGENTS\n\n## Reproject\n\nSome body text here.\n"
        )
        data = self._scan(root)
        section = next(
            s for s in data["sources"]["agents_md_sections"] if s.get("heading") == "Reproject"
        )
        # "Reproject" must NOT be classified as keep_in_brief via substring "project"
        self.assertNotEqual(section["classification"], "keep_in_brief")

    # ------------------------------------------------------------------ fenced code blocks
    def test_heading_inside_fenced_code_block_not_captured(self):
        """# headings inside fenced code blocks must not produce spurious sections."""
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "# Real Section\n\n"
            "Here is an example:\n\n"
            "```\n"
            "# fake heading inside fence\n"
            "## another fake\n"
            "```\n\n"
            "Trailing text.\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        headings = [s.get("heading") for s in sections]
        self.assertNotIn("fake heading inside fence", headings)
        self.assertNotIn("another fake", headings)
        self.assertIn("Real Section", headings)

    # ------------------------------------------------------------------ array-form manifest enabled filter
    def test_manifest_array_form_respects_enabled_flag(self):
        """_scan_manifest array-form must filter enabled=false entries."""
        root = Path(self._fixture_dir())
        manifest_dir = root / "ai-specs"
        manifest_dir.mkdir()
        (manifest_dir / "ai-specs.toml").write_text(
            "[[recipes]]\n"
            'id = "worktree-flow"\n'
            "enabled = false\n\n"
            "[[recipes]]\n"
            'id = "tdd-flow"\n'
            "enabled = true\n"
        )
        data = self._scan(root)
        recipes = data["sources"]["manifest"]["recipes"]
        self.assertNotIn("worktree-flow", recipes)
        self.assertIn("tdd-flow", recipes)

    # ------------------------------------------------------------------ fence offset regression
    def test_fence_before_heading_body_content_not_corrupted(self):
        """Body content after a fenced block must not be garbled by offset misalignment.

        This is the load-bearing regression test: a fenced block BEFORE a real
        heading shrinks text_no_fences vs text, so heading offsets computed on
        text_no_fences slice the wrong bytes from the original text.
        """
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "```\n"
            "# fake heading inside fence\n"
            "code line two\n"
            "```\n"
            "\n"
            "## Real Heading\n"
            "\n"
            "UNIQUE_BODY_MARKER some real body text here.\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        headings = [s.get("heading") for s in sections]
        self.assertIn("Real Heading", headings)
        real_section = next(s for s in sections if s.get("heading") == "Real Heading")
        # Body must contain the unique marker — if offsets are misaligned it will be absent
        self.assertIn("UNIQUE_BODY_MARKER", real_section["body_excerpt"])
        # Body must NOT contain fence/code-block content
        self.assertNotIn("fake heading inside fence", real_section["body_excerpt"])
        self.assertNotIn("code line two", real_section["body_excerpt"])

    # ------------------------------------------------------------------ tilde fence
    def test_heading_inside_tilde_fenced_block_not_captured(self):
        """# headings inside ~~~ fenced code blocks must not produce spurious sections."""
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "# Real Section\n\n"
            "Example:\n\n"
            "~~~\n"
            "# fake tilde heading\n"
            "## another tilde fake\n"
            "~~~\n\n"
            "Trailing text.\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        headings = [s.get("heading") for s in sections]
        self.assertNotIn("fake tilde heading", headings)
        self.assertNotIn("another tilde fake", headings)
        self.assertIn("Real Section", headings)

    # ------------------------------------------------------------------ unterminated fence
    def test_heading_inside_unterminated_fence_not_captured(self):
        """# headings inside an unterminated ``` block must not produce spurious sections."""
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "# Real Section\n\n"
            "Some body text.\n\n"
            "```\n"
            "# spurious heading inside unterminated fence\n"
            "more code\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        headings = [s.get("heading") for s in sections]
        self.assertNotIn("spurious heading inside unterminated fence", headings)
        self.assertIn("Real Section", headings)

    def test_fence_close_with_trailing_text_does_not_leak_headings(self):
        """A close fence must be whitespace-only; trailing text after the fence
        chars (e.g. ``` # done) must not prematurely close the block and leak
        following # lines as spurious sections."""
        root = Path(self._fixture_dir())
        (root / "AGENTS.md").write_text(
            "# Real Section\n\n"
            "Example:\n\n"
            "```bash\n"
            "ls\n"
            "```  # done\n"
            "# this is a shell comment, not a heading\n"
            "more code\n"
            "```\n\n"
            "Trailing text.\n"
        )
        data = self._scan(root)
        sections = data["sources"]["agents_md_sections"]
        headings = [s.get("heading") for s in sections]
        self.assertNotIn("this is a shell comment, not a heading", headings)
        self.assertIn("Real Section", headings)

    def _fixture_dir(self):
        import tempfile

        return tempfile.mkdtemp()


if __name__ == "__main__":
    unittest.main()
