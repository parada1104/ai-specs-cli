import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"


class SyncPipelineTests(unittest.TestCase):
    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-sync-"))
        shutil.copytree(FIXTURE_ROOT, tmp / "workspace")
        return tmp / "workspace"

    def init_workspace(self, workspace: Path) -> None:
        subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'fixture-sync'\n"
            "subrepos = ['packages/a', 'packages/b']\n\n"
            "[agents]\n"
            "enabled = ['claude', 'cursor', 'opencode']\n"
        )
        skill_dir = workspace / "ai-specs" / "skills" / "local-demo"
        skill_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text(
            "---\n"
            "name: local-demo\n"
            "description: >\n"
            "  Demo local skill.\n"
            "metadata:\n"
            "  scope: [root]\n"
            "  auto_invoke:\n"
            "    - \"Syncing root workspace\"\n"
            "---\n\n"
            "# Local Demo\n"
        )

    def test_sync_accepts_minimal_manifest_with_omitted_sections(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'fixture-sync'\n")

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertTrue((workspace / "AGENTS.md").is_file())
            self.assertFalse((workspace / "packages" / "a" / "AGENTS.md").exists())
            self.assertFalse((workspace / ".mcp.json").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_accepts_mcp_environment_alias_and_renders_canonical_output(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['opencode']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "environment = { API_KEY = '$DEMO_API_KEY' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(
                (workspace / "opencode.json").read_text(),
                '{\n'
                '  "mcp": {\n'
                '    "demo": {\n'
                '      "type": "local",\n'
                '      "command": [\n'
                '        "npx",\n'
                '        "-y",\n'
                '        "@demo/server"\n'
                '      ],\n'
                '      "environment": {\n'
                '        "API_KEY": "{env:DEMO_API_KEY}"\n'
                '      },\n'
                '      "timeout": 30000,\n'
                '      "enabled": true\n'
                '    }\n'
                '  }\n'
                '}\n',
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_keeps_single_project_behavior_without_subrepos(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            self.assertTrue((workspace / "AGENTS.md").is_file())
            self.assertFalse((workspace / "packages" / "a" / "AGENTS.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_fans_out_root_managed_artifacts_to_subrepos(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            subrepo = workspace / "packages" / "a"
            self.assertTrue((workspace / "AGENTS.md").is_file())
            self.assertTrue((subrepo / "AGENTS.md").is_file())
            self.assertTrue((subrepo / "ai-specs" / ".gitignore").is_file())
            self.assertTrue((subrepo / "ai-specs" / "skills" / "local-demo" / "SKILL.md").is_file())
            self.assertTrue((subrepo / "ai-specs" / "commands" / "skills-as-rules.md").is_file())
            self.assertTrue((subrepo / "CLAUDE.md").is_symlink())
            self.assertTrue((subrepo / ".claude" / "skills").is_symlink())
            self.assertTrue((subrepo / ".cursor" / "commands" / "skills-as-rules.md").is_file())
            self.assertTrue((subrepo / ".opencode" / "skills" / "local-demo" / "SKILL.md").is_file())
            self.assertTrue((subrepo / ".opencode" / "commands" / "skills-as-rules.md").is_file())
            self.assertFalse((subrepo / ".opencode" / "command").exists())
            self.assertIn("fixture-sync", (subrepo / "AGENTS.md").read_text())
        finally:
            shutil.rmtree(workspace.parent)

    def test_synced_subrepo_supports_local_agent_startup_read_paths(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            subrepo = workspace / "packages" / "a"
            proc = subprocess.run(
                [
                    "python3",
                    "-c",
                    (
                        "from pathlib import Path\n"
                        "cwd = Path.cwd().resolve()\n"
                        "claude = (cwd / 'CLAUDE.md').resolve()\n"
                        "skills = (cwd / '.claude' / 'skills').resolve()\n"
                        "cursor_cmd = cwd / '.cursor' / 'commands' / 'skills-as-rules.md'\n"
                        "assert claude == cwd / 'AGENTS.md', claude\n"
                        "assert skills == cwd / 'ai-specs' / 'skills', skills\n"
                        "assert 'fixture-sync' in claude.read_text(), 'missing AGENTS content'\n"
                        "assert (skills / 'local-demo' / 'SKILL.md').is_file(), 'missing local skill'\n"
                        "assert cursor_cmd.is_file(), 'missing cursor command'\n"
                        "assert (cwd / '.opencode' / 'skills' / 'local-demo' / 'SKILL.md').is_file(), 'missing opencode skill'\n"
                        "assert (cwd / '.opencode' / 'commands' / 'skills-as-rules.md').is_file(), 'missing opencode command'\n"
                        "print('ok')\n"
                    ),
                ],
                cwd=subrepo,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), "ok")
        finally:
            shutil.rmtree(workspace.parent)

    def test_public_root_sync_agent_fans_out_to_all_declared_subrepos(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            subprocess.run([str(CLI), "sync-agent", str(workspace), "--all"], check=True, text=True)

            for target in (workspace, workspace / "packages" / "a", workspace / "packages" / "b"):
                self.assertTrue((target / "AGENTS.md").is_file())
                self.assertTrue((target / ".cursor" / "commands" / "skills-as-rules.md").is_file())

            self.assertTrue((workspace / "packages" / "a" / "ai-specs" / "skills" / "local-demo" / "SKILL.md").is_file())
            self.assertTrue((workspace / "packages" / "b" / "CLAUDE.md").is_symlink())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_stops_on_first_incompatible_target_write(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            blocked = workspace / "packages" / "a" / "CLAUDE.md"
            blocked.write_text("manual file")
            proc = subprocess.run(
                [str(CLI), "sync", str(workspace)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("Stopped on first failure", proc.stderr)
            self.assertFalse((workspace / "packages" / "b" / "AGENTS.md").exists())
        finally:
            shutil.rmtree(workspace.parent)


if __name__ == "__main__":
    unittest.main()
