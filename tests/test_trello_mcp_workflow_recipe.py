"""Black-box validation and materialization tests for the Trello recipe."""
from __future__ import annotations

import json
import re
import sys
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import cache_project_dir, isolated_home, invoke, temp_project  # noqa: E402

RECIPE_DIR = ROOT / "catalog" / "recipes" / "trello-mcp-workflow"


class TrelloMcpWorkflowRecipeTests(unittest.TestCase):
    def _make_project(
        self,
        *,
        agents: tuple[str, ...] = ("claude",),
        config_block: str = "",
    ) -> tuple[Path, Path]:
        """Create an enabled Trello fixture and one shared CLI home."""
        td, root = temp_project(name="fixture", agents=agents)
        self.addCleanup(td.cleanup)
        version = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())["recipe"]["version"]
        manifest = root / "ai-specs" / "ai-specs.toml"
        text = manifest.read_text() + (
            f"\n[recipes.trello-mcp-workflow]\nenabled = true\nversion = \"{version}\"\n"
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
        )
        if config_block:
            text += config_block.rstrip() + "\n"
        manifest.write_text(text)
        return root, isolated_home(Path(td.name))

    def _invoke(self, root: Path, home: Path, *args: str):
        return invoke(root, *args, cli_home=home, tmpdir=home.parent)

    def _sync(self, root: Path, home: Path):
        result = self._invoke(root, home, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def _recipe_cache(self, root: Path, home: Path) -> Path:
        return cache_project_dir(root, home) / ".recipe" / "trello-mcp-workflow"

    def test_recipe_validates_with_dual_hooks_and_gate_mode(self):
        root, home = self._make_project()

        listed = self._invoke(root, home, "recipe", "list")
        self.assertEqual(listed.returncode, 0, listed.stderr)
        self.assertIn("trello-mcp-workflow", listed.stdout)
        self.assertIn("1.3.0", listed.stdout)
        self.assertIn("Trello MCP Workflow", listed.stdout)

        inspected = self._invoke(
            root, home, "recipe", "configure", "trello-mcp-workflow", "--inspect", "--json"
        )
        self.assertEqual(inspected.returncode, 0, inspected.stderr)
        data = json.loads(inspected.stdout)
        self.assertEqual(data["recipe"]["id"], "trello-mcp-workflow")
        self.assertTrue(data["recipe"]["enabled"])
        self.assertTrue(data["recipe"]["present_in_manifest"])
        fields = {field["key"]: field for field in data["schema"]["fields"]}
        self.assertIn("gate_mode", fields)
        self.assertEqual(fields["gate_mode"]["default"], "warn")
        self.assertEqual(set(fields["gate_mode"]["enum"]), {"off", "warn", "always"})
        self.assertIn("board_id", fields)

        sync = self._sync(root, home)
        self.assertIn("syncing recipes", sync.stdout)
        self.assertIn("trello-mcp-workflow", sync.stdout)
        hook = root / "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        self.assertTrue(hook.is_file())
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="warn"', content)
        self.assertIn(f'stamped_cli_home="{home.resolve()}"', content)
        self.assertNotIn("__TRACKER_CARD_GATE_MODE__", content)
        self.assertNotIn("__TRACKER_CLI_HOME__", content)

        settings = json.loads((root / ".claude/settings.json").read_text())
        pre_tool_use = settings["hooks"]["PreToolUse"]
        self.assertEqual(len(pre_tool_use), 2)
        self.assertEqual(
            {entry["matcher"] for entry in pre_tool_use},
            {"Edit|Write|MultiEdit|NotebookEdit", "Bash|Shell|Execute|Terminal"},
        )
        self.assertTrue(all("tracker-card-gate.sh" in json.dumps(entry) for entry in pre_tool_use))

    def test_tracking_declaration_matches_recipe_config(self):
        root, home = self._make_project(config_block='gate_mode = "warn"')
        inspected = self._invoke(
            root, home, "recipe", "configure", "trello-mcp-workflow", "--inspect", "--json"
        )
        self.assertEqual(inspected.returncode, 0, inspected.stderr)
        data = json.loads(inspected.stdout)
        configured = data["current_config"]
        self.assertEqual(configured["board_id"], "69ec097f13e2d38ecd89a557")
        self.assertEqual(configured["gate_mode"], "warn")

        project_manifest = tomllib.loads((root / "ai-specs/ai-specs.toml").read_text())
        manifest_config = project_manifest["recipes"]["trello-mcp-workflow"]["config"]
        self.assertEqual(manifest_config["board_id"], configured["board_id"])
        self.assertEqual(manifest_config["gate_mode"], configured["gate_mode"])

        config_text = (ROOT / "openspec/config.yaml").read_text()
        board_match = re.search(r'^  board_id:\s*"([^"]+)"', config_text, re.MULTILINE)
        mode_match = re.search(r"^  gate_mode:\s*(\w+)", config_text, re.MULTILINE)
        self.assertIsNotNone(board_match)
        self.assertIsNotNone(mode_match)
        self.assertEqual(board_match.group(1), configured["board_id"])
        self.assertEqual(mode_match.group(1), configured["gate_mode"])

    def test_sync_stamps_tracker_gate_mode_default_warn(self):
        root, home = self._make_project()
        sync = self._sync(root, home)
        self.assertIn("ai-specs sync", sync.stdout)
        self.assertIn("syncing recipes → trello-mcp-workflow", sync.stdout)
        hook = root / "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        self.assertTrue(hook.is_file())
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="warn"', content)
        self.assertIn(f'stamped_cli_home="{home.resolve()}"', content)
        self.assertNotIn("__TRACKER_CARD_GATE_MODE__", content)
        self.assertNotIn("__TRACKER_CLI_HOME__", content)

    def test_sync_stamps_tracker_gate_mode_override(self):
        root, home = self._make_project(config_block='gate_mode = "always"')
        sync = self._sync(root, home)
        self.assertIn("trello-mcp-workflow", sync.stdout)
        hook = root / "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        self.assertTrue(hook.is_file())
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="always"', content)
        self.assertNotIn('stamped_gate_mode="__TRACKER_CARD_GATE_MODE__"', content)

    def test_materialize_hook_script_map_and_cli_home(self):
        root, home = self._make_project(agents=("claude", "omp"))
        sync = self._sync(root, home)
        self.assertEqual(sync.returncode, 0)
        hook = root / "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        self.assertTrue(hook.is_file())
        text = hook.read_text()
        self.assertIn('stamped_gate_mode="warn"', text)
        self.assertIn(f'stamped_cli_home="{home.resolve()}"', text)
        self.assertNotIn("__TRACKER_CARD_GATE_MODE__", text)
        self.assertNotIn("__TRACKER_CLI_HOME__", text)

        settings = json.loads((root / ".claude/settings.json").read_text())
        self.assertEqual(len(settings["hooks"]["PreToolUse"]), 2)
        self.assertTrue((root / ".omp/extensions").is_dir())
        extensions = list((root / ".omp/extensions").glob("*.ts"))
        self.assertGreaterEqual(len(extensions), 2)
        extension_text = "\n".join(path.read_text() for path in extensions)
        self.assertIn("tracker-card-gate.sh", extension_text)
        self.assertIn("Edit|Write|MultiEdit|NotebookEdit", extension_text)
        self.assertIn("Bash|Shell|Execute|Terminal", extension_text)
        # TRIAGE: the former direct `materialize_hook_script(..., cli_home=None)`
        # and fake worktree recipe assertions covered arguments unavailable at
        # the `bin/ai-specs sync` boundary; sync always resolves one CLI home and
        # only exposes the stamped Trello hook plus generated runtime adapters.

    def test_claude_dual_pretooluse_same_script(self):
        root, home = self._make_project(agents=("claude",))
        self._sync(root, home)
        script = "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        settings = json.loads((root / ".claude/settings.json").read_text())
        pre = settings["hooks"]["PreToolUse"]
        managed = [entry for entry in pre if script in json.dumps(entry)]
        self.assertEqual(len(managed), 2)
        self.assertTrue(all(entry["hooks"][0]["type"] == "command" for entry in managed))
        self.assertEqual(
            {entry["matcher"] for entry in managed},
            {"Edit|Write|MultiEdit|NotebookEdit", "Bash|Shell|Execute|Terminal"},
        )
        commands = {entry["hooks"][0]["command"] for entry in managed}
        self.assertEqual(len(commands), 1)
        self.assertIn("tracker-card-gate.sh", next(iter(commands)))

    def test_cursor_shell_registers_filewrite_skipped(self):
        root, home = self._make_project(agents=("cursor",))
        sync = self._sync(root, home)
        script = "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"
        file_wrapper = root / ".cursor/hooks/trello-mcp-workflow-tracker-card-gate.sh"
        shell_wrapper = root / ".cursor/hooks/trello-mcp-workflow-tracker-card-gate-shell.sh"
        self.assertFalse(file_wrapper.exists())
        self.assertTrue(shell_wrapper.is_file())
        self.assertIn("no pre-file-write hook exists", sync.stderr.lower())
        self.assertIn("tracker-card-gate", shell_wrapper.read_text())
        hooks_json = json.loads((root / ".cursor/hooks.json").read_text())
        self.assertIn("beforeShellExecution", json.dumps(hooks_json))
        self.assertIn(script, shell_wrapper.read_text())

    def test_omp_both_matchers_case_insensitive(self):
        root, home = self._make_project(agents=("omp",))
        self._sync(root, home)
        ext_path = root / ".omp/extensions"
        self.assertTrue(ext_path.is_dir())
        files = list(ext_path.glob("*.ts"))
        self.assertGreaterEqual(len(files), 2)
        names = {path.name for path in files}
        self.assertIn("trello-mcp-workflow-tracker-card-gate.ts", names)
        self.assertIn("trello-mcp-workflow-tracker-card-gate-shell.ts", names)
        blob = "\n".join(path.read_text() for path in files).lower()
        self.assertIn("tracker-card-gate.sh", blob)
        self.assertTrue("write" in blob or "edit" in blob)
        self.assertTrue("bash" in blob or "shell" in blob)

    def test_skill_doc_content_contract(self):
        root, home = self._make_project(agents=("claude",))
        sync = self._sync(root, home)
        self.assertEqual(sync.returncode, 0)
        skill = self._recipe_cache(root, home) / "skills/trello-mcp-workflow/SKILL.md"
        self.assertTrue(skill.is_file())
        skill_text = skill.read_text()
        self.assertNotIn("Allow the agent to skip card creation", skill_text)
        self.assertIn("## Tracker", skill_text)
        self.assertIn("tracker.none", skill_text)
        self.assertIn("missing link", skill_text.lower())
        self.assertIn("cache/projects/", skill_text)
        self.assertIn("New structured change or feature request", skill_text)
        self.assertIn("missing a linked Trello card", skill_text)
        self.assertIn("availability failure", skill_text.lower())
        self.assertIn("do not claim", skill_text.lower())

        command = root / ".claude/commands/trello-workflow.md"
        self.assertTrue(command.is_file())
        self.assertIn("## Tracker", command.read_text())
        bootstrap = ROOT / "catalog/recipes/session-context/skills/session-bootstrap/SKILL.md"
        bootstrap_text = bootstrap.read_text()
        self.assertIn("mandatory", bootstrap_text.lower())
        self.assertIn("## Tracker", bootstrap_text)
        self.assertNotIn("only if needed", bootstrap_text)


if __name__ == "__main__":
    unittest.main()
