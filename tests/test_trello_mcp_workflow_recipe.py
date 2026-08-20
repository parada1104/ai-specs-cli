"""Validation + materialization tests for trello-mcp-workflow tracker-card-gate."""
from __future__ import annotations

import importlib.util
import json
import re
import tomllib
import tempfile
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _cache_paths import recipe_root  # noqa: E402

RECIPE_DIR = ROOT / "catalog" / "recipes" / "trello-mcp-workflow"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
HOOKS_RENDER_PATH = ROOT / "lib" / "_internal" / "hooks-render.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class TrelloMcpWorkflowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_trello_gate")
        cls.materialize = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_trello_gate"
        )
        cls.hooks_render = load_module(HOOKS_RENDER_PATH, "hooks_render_trello_gate")

    def test_recipe_validates_with_dual_hooks_and_gate_mode(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(recipe.id, "trello-mcp-workflow")
        self.assertEqual(recipe.version, "1.4.0")
        fields = recipe.config_schema.fields
        self.assertIn("gate_mode", fields)
        self.assertEqual(fields["gate_mode"].default, "off")
        self.assertEqual(set(fields["gate_mode"].enum or []), {"off", "warn", "always"})
        ids = {h.id for h in recipe.runtime_hooks}
        self.assertEqual(ids, {"tracker-card-gate", "tracker-card-gate-shell"})
        by_id = {h.id: h for h in recipe.runtime_hooks}
        self.assertEqual(by_id["tracker-card-gate"].matcher, "Edit|Write|MultiEdit|NotebookEdit")
        self.assertEqual(
            by_id["tracker-card-gate-shell"].matcher, "Bash|Shell|Execute|Terminal"
        )
        self.assertEqual(by_id["tracker-card-gate"].script, "hooks/tracker-card-gate.sh")
        self.assertEqual(by_id["tracker-card-gate-shell"].script, "hooks/tracker-card-gate.sh")
        self.assertTrue(by_id["tracker-card-gate"].blocking)
        frags = (recipe.brief_fragments.workflow_rules or []) if recipe.brief_fragments else []
        rules = " ".join(f.text for f in frags)
        self.assertIn("## Tracker", rules)
        self.assertIn("tracker.none", rules)
        self.assertIn("never bypass", rules.lower())
        self.assertIn("phase", rules.lower())
        # The tracker surface must announce itself as inert when unconfigured, so an
        # agent in a project without tracker credentials never brings it up.
        self.assertIn("HAS NO TRACKER", rules)
        self.assertIn("do not mention it", rules)
    def test_surface_is_silent_when_the_tracker_is_unconfigured(self):
        skill = (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text()
        self.assertIn("Silence when unconfigured", skill)
        self.assertIn("say nothing about the tracker", skill)
        self.assertIn("do not offer to connect or configure it", skill)
        command = (RECIPE_DIR / "commands" / "trello-workflow.md").read_text()
        self.assertIn("not configured", command)
        self.assertIn("do not offer to configure it", command)

    def test_tracking_declaration_matches_recipe_config(self):
        config_text = (ROOT / "openspec" / "config.yaml").read_text()
        board_match = re.search(r"^  board_id:\s*\"([^\"]+)\"", config_text, re.MULTILINE)
        mode_match = re.search(r"^  gate_mode:\s*(\w+)", config_text, re.MULTILINE)
        manifest = tomllib.loads((ROOT / "ai-specs" / "ai-specs.toml").read_text())
        configured = manifest["recipes"]["trello-mcp-workflow"]["config"]
        self.assertIsNotNone(board_match)
        self.assertIsNotNone(mode_match)
        self.assertEqual(board_match.group(1), configured["board_id"])
        self.assertEqual(mode_match.group(1), configured["gate_mode"])

    def _make_project(self, config_block: str = "") -> Path:
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
        text = (
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude', 'cursor', 'omp']\n\n"
            f'[recipes.trello-mcp-workflow]\nenabled = true\nversion = "{version}"\n'
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
        )
        if config_block:
            text = text.rstrip() + "\n" + config_block + "\n"
        (ai_specs / "ai-specs.toml").write_text(text)
        return root

    def test_sync_stamps_tracker_gate_mode_default_off(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        hook = (
            root / "ai-specs" / "recipes" / "trello-mcp-workflow" / "hooks"
            / "tracker-card-gate.sh"
        )
        self.assertTrue(hook.is_file())
        content = hook.read_text()
        # Default is off: a tracker nobody configured must not gate any write.
        self.assertIn('stamped_gate_mode="off"', content)
        self.assertNotIn("__TRACKER_CARD_GATE_MODE__", content)
        # CLI home stamped to resolved ROOT
        self.assertIn(f'stamped_cli_home="{ROOT.resolve()}"', content)
        self.assertNotIn("__TRACKER_CLI_HOME__", content)

    def test_sync_stamps_tracker_gate_mode_override(self):
        root = self._make_project('gate_mode = "always"')
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)
        hook = (
            root / "ai-specs" / "recipes" / "trello-mcp-workflow" / "hooks"
            / "tracker-card-gate.sh"
        )
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="always"', content)

    def test_materialize_hook_script_map_and_cli_home(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        recipe_dir = project / "recipe"
        hooks = recipe_dir / "hooks"
        hooks.mkdir(parents=True)
        script = hooks / "tracker-card-gate.sh"
        script.write_text(
            'stamped_gate_mode="__TRACKER_CARD_GATE_MODE__"\n'
            'stamped_cli_home="__TRACKER_CLI_HOME__"\n'
        )
        hook = SimpleNamespace(script="hooks/tracker-card-gate.sh", id="tracker-card-gate")
        rel = self.materialize.materialize_hook_script(
            recipe_dir,
            hook,
            project,
            "trello-mcp-workflow",
            {"gate_mode": "warn"},
            cli_home=ROOT,
        )
        dest = project / rel
        text = dest.read_text()
        self.assertIn('stamped_gate_mode="warn"', text)
        self.assertIn(str(ROOT.resolve()), text)
        self.assertNotIn("__TRACKER_CARD_GATE_MODE__", text)
        self.assertNotIn("__TRACKER_CLI_HOME__", text)

        # cli_home None → empty string stamp
        script.write_text('stamped_cli_home="__TRACKER_CLI_HOME__"\n')
        self.materialize.materialize_hook_script(
            recipe_dir, hook, project, "trello-mcp-workflow", {}, cli_home=None
        )
        self.assertIn('stamped_cli_home=""', (project / rel).read_text())

        # worktree-gate without TRACKER_CLI_HOME unaffected
        wt = hooks / "worktree-gate.sh"
        wt.write_text('stamped_gate_mode="__WORKTREE_GATE_MODE__"\n')
        wt_hook = SimpleNamespace(script="hooks/worktree-gate.sh", id="worktree-gate")
        # Use a fake worktree recipe id path
        rel2 = self.materialize.materialize_hook_script(
            recipe_dir,
            wt_hook,
            project,
            "worktree-flow",
            {"gate_mode": "ask"},
            cli_home=ROOT,
        )
        wt_text = (project / rel2).read_text()
        self.assertIn('stamped_gate_mode="ask"', wt_text)
        self.assertNotIn("__TRACKER_CLI_HOME__", wt_text)
        self.assertNotIn(str(ROOT.resolve()), wt_text)

    def test_claude_dual_pretooluse_same_script(self):
        project = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(project, ignore_errors=True))
        script = "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        hooks = [
            {
                "recipe": "trello-mcp-workflow",
                "id": "tracker-card-gate",
                "event": "pre-tool-use",
                "matcher": "Edit|Write|MultiEdit|NotebookEdit",
                "blocking": True,
                "script_path": script,
                "env": {},
            },
            {
                "recipe": "trello-mcp-workflow",
                "id": "tracker-card-gate-shell",
                "event": "pre-tool-use",
                "matcher": "Bash|Shell|Execute|Terminal",
                "blocking": True,
                "script_path": script,
                "env": {},
            },
        ]
        resolved = project / "resolved-hooks.json"
        resolved.write_text(json.dumps({"enabled_agents": ["claude"], "hooks": hooks}))
        self.hooks_render.render(resolved, "claude", project)
        settings = json.loads((project / ".claude" / "settings.json").read_text())
        pre = settings["hooks"]["PreToolUse"]
        managed = [e for e in pre if script in json.dumps(e)]
        self.assertEqual(len(managed), 2)
        matchers = {e.get("matcher") for e in managed}
        self.assertIn("Edit|Write|MultiEdit|NotebookEdit", matchers)
        self.assertIn("Bash|Shell|Execute|Terminal", matchers)

    def test_cursor_shell_registers_filewrite_skipped(self):
        project = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(project, ignore_errors=True))
        script = "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        hooks = [
            {
                "recipe": "trello-mcp-workflow",
                "id": "tracker-card-gate",
                "event": "pre-tool-use",
                "matcher": "Edit|Write|MultiEdit|NotebookEdit",
                "blocking": True,
                "script_path": script,
                "env": {},
            },
            {
                "recipe": "trello-mcp-workflow",
                "id": "tracker-card-gate-shell",
                "event": "pre-tool-use",
                "matcher": "Bash|Shell|Execute|Terminal",
                "blocking": True,
                "script_path": script,
                "env": {},
            },
        ]
        resolved = project / "resolved-hooks.json"
        resolved.write_text(json.dumps({"enabled_agents": ["cursor"], "hooks": hooks}))
        warnings = self.hooks_render.render(resolved, "cursor", project)
        file_wrapper = project / ".cursor" / "hooks" / "trello-mcp-workflow-tracker-card-gate.sh"
        shell_wrapper = (
            project / ".cursor" / "hooks" / "trello-mcp-workflow-tracker-card-gate-shell.sh"
        )
        self.assertFalse(file_wrapper.exists(), "cursor skips file-write matcher")
        self.assertTrue(shell_wrapper.is_file(), "cursor registers shell id")
        hooks_json = json.loads((project / ".cursor" / "hooks.json").read_text())
        self.assertIn("beforeShellExecution", json.dumps(hooks_json))
        joined = " ".join(warnings).lower()
        self.assertIn("tracker-card-gate", joined)

    def test_omp_both_matchers_case_insensitive(self):
        project = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(project, ignore_errors=True))
        script = "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        hooks = [
            {
                "recipe": "trello-mcp-workflow",
                "id": "tracker-card-gate",
                "event": "pre-tool-use",
                "matcher": "Edit|Write|MultiEdit|NotebookEdit",
                "blocking": True,
                "script_path": script,
                "env": {},
            },
            {
                "recipe": "trello-mcp-workflow",
                "id": "tracker-card-gate-shell",
                "event": "pre-tool-use",
                "matcher": "Bash|Shell|Execute|Terminal",
                "blocking": True,
                "script_path": script,
                "env": {},
            },
        ]
        resolved = project / "resolved-hooks.json"
        resolved.write_text(json.dumps({"enabled_agents": ["omp"], "hooks": hooks}))
        self.hooks_render.render(resolved, "omp", project)
        ext_path = project / ".omp" / "extensions"
        files = list(ext_path.glob("*.ts")) if ext_path.is_dir() else []
        self.assertTrue(files, "omp extensions should be generated")
        blob = "\n".join(f.read_text() for f in files).lower()
        # Case-insensitive matcher coverage for file + shell tool names
        self.assertTrue("write" in blob or "edit" in blob)
        self.assertTrue("bash" in blob or "shell" in blob)


    def test_skill_doc_content_contract(self):
        skill = (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text()
        self.assertNotIn("Allow the agent to skip card creation", skill)
        self.assertIn("## Tracker", skill)
        self.assertIn("tracker.none", skill)
        self.assertIn("missing link", skill.lower())
        self.assertIn("cache/projects/", skill)
        # auto_invoke triggers present
        self.assertIn("New structured change or feature request", skill)
        self.assertIn("missing a linked Trello card", skill)
        # unavailable excuse forbidden for missing artifact
        self.assertIn("availability failure", skill.lower())
        self.assertIn("do not claim", skill.lower())
        cmd = (RECIPE_DIR / "commands" / "trello-workflow.md").read_text()
        self.assertIn("## Tracker", cmd)
        boot = (
            ROOT / "catalog" / "recipes" / "session-context" / "skills"
            / "session-bootstrap" / "SKILL.md"
        ).read_text()
        self.assertIn("mandatory", boot.lower())
        self.assertIn("## Tracker", boot)
        self.assertNotIn("only if needed", boot)


if __name__ == "__main__":
    unittest.main()
