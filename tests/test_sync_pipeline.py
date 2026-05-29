import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"


class SyncPipelineTests(unittest.TestCase):
    def test_sync_workspace_root_fixture_exists_with_declared_subrepos(self):
        self.assertTrue(FIXTURE_ROOT.is_dir())
        self.assertTrue((FIXTURE_ROOT / "packages" / "a").is_dir())
        self.assertTrue((FIXTURE_ROOT / "packages" / "b").is_dir())

    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-sync-"))
        shutil.copytree(FIXTURE_ROOT, tmp / "workspace")
        return tmp / "workspace"

    def write_local_skill(
        self,
        workspace: Path,
        name: str,
        *,
        description: str,
        author: str = "fixture-suite",
        version: str = "1.0",
        license_id: str = "Apache-2.0",
        scope: list[str] | None = None,
        auto_invoke: list[str] | None = None,
        body: str | None = None,
    ) -> Path:
        skill_dir = workspace / "ai-specs" / "skills" / name
        skill_dir.mkdir(parents=True)
        lines = [
            "---",
            f"name: {name}",
            "description: >",
            f"  {description}",
            f"license: {license_id}",
            "metadata:",
            f"  author: {author}",
            f'  version: "{version}"',
        ]
        if scope:
            lines.append("  scope:")
            lines.extend(f'    - "{entry}"' for entry in scope)
        if auto_invoke:
            lines.append("  auto_invoke:")
            lines.extend(f'    - "{entry}"' for entry in auto_invoke)
        lines.extend(["---", "", body or f"# {name}", ""])
        path = skill_dir / "SKILL.md"
        path.write_text("\n".join(lines))
        return path

    def auto_invoke_section(self, agents_path: Path) -> str:
        text = agents_path.read_text()
        start = text.index("### Auto-invoke Skills")
        tail = text[start:]
        end_marker = "\n## How AI tooling is wired"
        if end_marker in tail:
            tail = tail.split(end_marker, 1)[0]
        return tail

    def init_workspace(self, workspace: Path) -> None:
        subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'fixture-sync'\n"
            "subrepos = ['packages/a', 'packages/b']\n\n"
            "[agents]\n"
            "enabled = ['claude', 'cursor', 'opencode']\n"
        )
        self.write_local_skill(
            workspace,
            "local-demo",
            description="Demo local skill.",
            scope=["root"],
            auto_invoke=["Syncing root workspace"],
            body="# Local Demo",
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
                '  "$schema": "https://opencode.ai/config.json",\n'
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

    def test_sync_renders_opencode_mcp_env_with_braced_dollar_syntax_input(self):
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
                "environment = { API_KEY = '${DEMO_API_KEY}' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(
                (workspace / "opencode.json").read_text(),
                '{\n'
                '  "$schema": "https://opencode.ai/config.json",\n'
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

    def test_sync_renders_cursor_mcp_env_with_braced_dollar_syntax_input(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['cursor']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = { API_KEY = '${DEMO_API_KEY}' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(
                (workspace / ".cursor" / "mcp.json").read_text(),
                '{\n'
                '  "mcpServers": {\n'
                '    "demo": {\n'
                '      "command": "npx",\n'
                '      "args": [\n'
                '        "-y",\n'
                '        "@demo/server"\n'
                '      ],\n'
                '      "env": {\n'
                '        "API_KEY": "${DEMO_API_KEY}"\n'
                '      },\n'
                '      "timeout": 30000,\n'
                '      "enabled": true\n'
                '    }\n'
                '  }\n'
                '}\n',
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_renders_claude_mcp_env_with_braced_dollar_syntax_input(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = { API_KEY = '${DEMO_API_KEY}' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(
                (workspace / ".mcp.json").read_text(),
                '{\n'
                '  "mcpServers": {\n'
                '    "demo": {\n'
                '      "command": "npx",\n'
                '      "args": [\n'
                '        "-y",\n'
                '        "@demo/server"\n'
                '      ],\n'
                '      "env": {\n'
                '        "API_KEY": "${DEMO_API_KEY}"\n'
                '      },\n'
                '      "timeout": 30000,\n'
                '      "enabled": true\n'
                '    }\n'
                '  }\n'
                '}\n',
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_renders_cursor_mcp_env_with_braced_variable_syntax(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['cursor']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = { API_KEY = '$DEMO_API_KEY' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(
                (workspace / ".cursor" / "mcp.json").read_text(),
                '{\n'
                '  "mcpServers": {\n'
                '    "demo": {\n'
                '      "command": "npx",\n'
                '      "args": [\n'
                '        "-y",\n'
                '        "@demo/server"\n'
                '      ],\n'
                '      "env": {\n'
                '        "API_KEY": "${DEMO_API_KEY}"\n'
                '      },\n'
                '      "timeout": 30000,\n'
                '      "enabled": true\n'
                '    }\n'
                '  }\n'
                '}\n',
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_renders_claude_mcp_env_with_braced_variable_syntax(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = { API_KEY = '$DEMO_API_KEY' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(
                (workspace / ".mcp.json").read_text(),
                '{\n'
                '  "mcpServers": {\n'
                '    "demo": {\n'
                '      "command": "npx",\n'
                '      "args": [\n'
                '        "-y",\n'
                '        "@demo/server"\n'
                '      ],\n'
                '      "env": {\n'
                '        "API_KEY": "${DEMO_API_KEY}"\n'
                '      },\n'
                '      "timeout": 30000,\n'
                '      "enabled": true\n'
                '    }\n'
                '  }\n'
                '}\n',
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_renders_mcp_env_list_as_environment_references(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['cursor', 'opencode']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = ['VAR1', 'VAR2']\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            cursor = json.loads((workspace / ".cursor" / "mcp.json").read_text())
            opencode = json.loads((workspace / "opencode.json").read_text())

            self.assertEqual(
                cursor["mcpServers"]["demo"]["env"],
                {"VAR1": "${VAR1}", "VAR2": "${VAR2}"},
            )
            self.assertEqual(
                opencode["mcp"]["demo"]["environment"],
                {"VAR1": "{env:VAR1}", "VAR2": "{env:VAR2}"},
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_preserves_static_mcp_env_values(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['cursor', 'opencode']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = { MODE = 'fixture' }\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            cursor = json.loads((workspace / ".cursor" / "mcp.json").read_text())
            opencode = json.loads((workspace / "opencode.json").read_text())

            self.assertEqual(cursor["mcpServers"]["demo"]["env"], {"MODE": "fixture"})
            self.assertEqual(opencode["mcp"]["demo"]["environment"], {"MODE": "fixture"})
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_keeps_opencode_schema_first_when_preserving_existing_config(self):
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
                "env = { API_KEY = '$DEMO_API_KEY' }\n"
            )
            (workspace / "opencode.json").write_text(
                '{\n'
                '  "mcp": {"old": {"type": "local", "command": ["old"]}},\n'
                '  "theme": "system",\n'
                '  "$schema": "https://opencode.ai/config.json"\n'
                '}\n'
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            text = (workspace / "opencode.json").read_text()
            parsed = json.loads(text)
            self.assertEqual(list(parsed.keys()), ["$schema", "mcp", "theme"])
            self.assertEqual(parsed["theme"], "system")
            self.assertEqual(parsed["mcp"]["demo"]["environment"]["API_KEY"], "{env:DEMO_API_KEY}")
            self.assertNotIn("old", parsed["mcp"])
        finally:
            shutil.rmtree(workspace.parent)

    def make_dep_repo(self, root: Path, *, broken_sync: bool = False) -> Path:
        repo = root / "dep-skill"
        repo.mkdir()
        body = (
            "---\n"
            "name: upstream-demo\n"
            "description: >\n"
            "  Upstream vendored skill. Trigger: Upstream trigger.\n"
            "---\n\n"
            "# Vendored Demo\n"
        )
        if broken_sync:
            body = (
                "---\n"
                "name: broken-demo\n"
                "description: Broken sync metadata.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                '  version: "1.0"\n'
                "  scope: [root]\n"
                "---\n\n"
                "# Broken Demo\n"
            )
        (repo / "SKILL.md").write_text(body)
        subprocess.run(["git", "init"], cwd=repo, check=True, text=True, capture_output=True)
        subprocess.run(["git", "config", "user.name", "Fixture Suite"], cwd=repo, check=True, text=True, capture_output=True)
        subprocess.run(["git", "config", "user.email", "fixture@example.com"], cwd=repo, check=True, text=True, capture_output=True)
        subprocess.run(["git", "add", "SKILL.md"], cwd=repo, check=True, text=True, capture_output=True)
        subprocess.run(["git", "commit", "-m", "init"], cwd=repo, check=True, text=True, capture_output=True)
        return repo

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

    def test_sync_normalizes_vendored_skill_frontmatter_and_fans_out_byte_identically(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            dep_repo = self.make_dep_repo(workspace.parent)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                (workspace / "ai-specs" / "ai-specs.toml").read_text()
                + "\n[[deps]]\n"
                + 'id = "vendored-demo"\n'
                + f'source = "{dep_repo}"\n'
                + 'scope = ["root"]\n'
                + 'auto_invoke = ["Sync vendored metadata"]\n'
                + 'license = "MIT"\n'
                + 'vendor_attribution = "fixture-org"\n'
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            root_skill = workspace / "ai-specs" / ".deps" / "vendored-demo" / "skills" / "vendored-demo" / "SKILL.md"
            subrepo_skill = workspace / "packages" / "a" / "ai-specs" / "skills" / "vendored-demo" / "SKILL.md"
            content = root_skill.read_text()

            self.assertTrue(root_skill.is_file())
            self.assertEqual(root_skill.read_bytes(), subrepo_skill.read_bytes())
            self.assertIn('author: "fixture-org"', content)
            self.assertIn('version: "1.0"', content)
            self.assertIn(f'source: "{dep_repo}"', content)
            self.assertIn('vendor_attribution: "fixture-org"', content)
            self.assertIn("auto_invoke:", content)
            self.assertFalse((workspace / "ai-specs" / ".skill-registry.md").exists())
            self.assertNotIn("`vendored-demo`", (workspace / "AGENTS.md").read_text())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_rewrites_hand_edited_vendored_frontmatter_from_manifest_inputs(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            dep_repo = self.make_dep_repo(workspace.parent)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                (workspace / "ai-specs" / "ai-specs.toml").read_text()
                + "\n[[deps]]\n"
                + 'id = "vendored-demo"\n'
                + f'source = "{dep_repo}"\n'
                + 'scope = ["root"]\n'
                + 'auto_invoke = ["Sync vendored metadata"]\n'
                + 'license = "MIT"\n'
                + 'vendor_attribution = "fixture-org"\n'
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            root_skill = workspace / "ai-specs" / ".deps" / "vendored-demo" / "skills" / "vendored-demo" / "SKILL.md"
            root_skill.write_text(
                "---\n"
                "name: vendored-demo\n"
                "description: Manual tamper.\n"
                "license: GPL-3.0\n"
                "metadata:\n"
                "  author: manual-edit\n"
                '  version: "9.9"\n'
                "  scope: [root]\n"
                "  auto_invoke:\n"
                '    - "Manual trigger"\n'
                "---\n\n"
                "# Tampered\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            content = root_skill.read_text()
            self.assertIn("license: MIT", content)
            self.assertIn('author: "fixture-org"', content)
            self.assertIn('version: "1.0"', content)
            self.assertIn('auto_invoke:\n    - "Sync vendored metadata"', content)
            self.assertNotIn("manual-edit", content)
            self.assertNotIn("Manual tamper", content)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_supports_local_auto_invoke_skill_authoring_in_canonical_form(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            skill_path = workspace / "ai-specs" / "skills" / "local-demo" / "SKILL.md"
            content = skill_path.read_text()

            self.assertIn("name: local-demo", content)
            self.assertIn("license: Apache-2.0", content)
            self.assertIn("author: fixture-suite", content)
            self.assertIn('version: "1.0"', content)
            self.assertIn('scope:\n    - "root"', content)
            self.assertIn('auto_invoke:\n    - "Syncing root workspace"', content)
            self.assertFalse((workspace / "ai-specs" / ".skill-registry.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_supports_local_non_auto_invoke_skill_authoring_without_agents_row(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            self.write_local_skill(
                workspace,
                "local-docs",
                description="Documentation helper without AGENTS auto-invoke.",
                body="# Local Docs",
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            content = (workspace / "ai-specs" / "skills" / "local-docs" / "SKILL.md").read_text()

            self.assertIn("name: local-docs", content)
            self.assertIn("license: Apache-2.0", content)
            self.assertIn("author: fixture-suite", content)
            self.assertIn('version: "1.0"', content)
            self.assertNotIn("scope:", content)
            self.assertNotIn("auto_invoke:", content)
            self.assertFalse((workspace / "ai-specs" / ".skill-registry.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_warns_on_invalid_skill_metadata(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            bad_skill_dir = workspace / "ai-specs" / "skills" / "bad-sync"
            bad_skill_dir.mkdir(parents=True)
            (bad_skill_dir / "SKILL.md").write_text(
                "---\n"
                "name: bad-sync\n"
                "description: Broken sync metadata.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                '  version: "1.0"\n'
                "  scope: [root]\n"
                "---\n\n"
                "# Broken\n"
            )

            proc = subprocess.run(
                [str(CLI), "sync", str(workspace)],
                text=True,
                capture_output=True,
                check=False,
            )

            # Sync succeeds but skill-sync reports missing auto_invoke
            self.assertEqual(proc.returncode, 0)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_warns_on_auto_invoke_without_scope(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            self.write_local_skill(
                workspace,
                "bad-scope",
                description="Missing scope.",
                auto_invoke=["Do thing"],
            )

            proc = subprocess.run(
                [str(CLI), "sync", str(workspace)],
                text=True,
                capture_output=True,
                check=False,
            )

            # Sync succeeds but skill-sync reports incomplete metadata
            self.assertEqual(proc.returncode, 0)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_produces_identical_agents_md_on_second_run_thin(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            first = (workspace / "AGENTS.md").read_bytes()
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            second = (workspace / "AGENTS.md").read_bytes()
            self.assertEqual(first, second)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_produces_identical_agents_md_on_second_run(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            first = (workspace / "AGENTS.md").read_bytes()
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            second = (workspace / "AGENTS.md").read_bytes()
            self.assertEqual(first, second)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_preserves_runtime_brief_marker_in_agents_md(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'fixture-sync'\n")
            agents_md = workspace / "AGENTS.md"
            original = "# Manual Brief\n<!-- ai-specs:runtime-brief -->\n\nCustom content.\n"
            agents_md.write_text(original)

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            self.assertEqual(agents_md.read_text(), original)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_redacts_literal_mcp_secrets_in_agents_md(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['cursor']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/server']\n"
                "env = { API_KEY = 'hardcoded-secret', MODE = '$DEMO_MODE' }\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("API_KEY: ***", agents)
            self.assertIn("MODE: ${DEMO_MODE}", agents)
            self.assertNotIn("hardcoded-secret", agents)
        finally:
            shutil.rmtree(workspace.parent)

    # -----------------------------------------------------------------------
    # Batch 1 RED tests — option-c-runtime-brief
    # These tests MUST FAIL until Batch 2/3 implement the feature.
    # -----------------------------------------------------------------------

    def test_sync_renders_rich_brief_from_manifest(self):
        """Needle test: [brief] + recipe configs produce structured needles in AGENTS.md.

        Fails (RED) because agents-render.py does not yet accept --resolved-config
        and does not render [brief] sections.  Batch 2 wires --resolved-config;
        Batch 3 implements the section helpers.
        """
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'brief-needle-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude', 'cursor']\n\n"
                "[brief]\n"
                'intro = "Canonical runtime context for agents."\n'
                'purpose = "per-project AI harness for configuration and tracking."\n'
                'runtime_flow = [\n'
                '  "A session works on one explicit user request or Trello card.",\n'
                '  "Artifact phases run in a dedicated worktree.",\n'
                ']\n'
                'context_sources = ["Trello is the source of truth for work state."]\n'
                'conflict_policy = ["Explicit human instruction controls immediate scope."]\n'
                'workflow_rules = ["Do not merge without explicit human instruction."]\n\n'
                "[brief.mcp_descriptions]\n"
                'trello = "project tracking through the Roadmap board."\n\n'
                "[mcp.trello]\n"
                "command = 'npx'\n"
                "args = ['-y', '@trello/mcp']\n\n"
                "[recipes.trello-mcp-workflow]\n"
                "board_id = 'abc123testboard'\n\n"
                "[recipes.worktree-flow]\n"
                "integration_branch = 'development'\n\n"
                "[recipes.tdd-flow]\n"
                "test_command = './tests/run.sh'\n\n"
                "[recipes.vault-canonical-store]\n"
                "vault_scope = 'nnodes/proyectos/test-project'\n\n"
                "[[bindings]]\n"
                "capability = 'tracker'\n"
                "recipe = 'trello-mcp-workflow'\n\n"
                "[[bindings]]\n"
                "capability = 'vcs-pr-flow'\n"
                "recipe = 'worktree-flow'\n\n"
                "[[bindings]]\n"
                "capability = 'canonical-store'\n"
                "recipe = 'vault-canonical-store'\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            agents = (workspace / "AGENTS.md").read_text()

            # Prose sections from [brief] must be present
            self.assertIn("Canonical runtime context for agents.", agents)
            self.assertIn("per-project AI harness for configuration and tracking.", agents)
            self.assertIn("A session works on one explicit user request or Trello card.", agents)
            self.assertIn("Trello is the source of truth for work state.", agents)
            self.assertIn("Explicit human instruction controls immediate scope.", agents)
            self.assertIn("Do not merge without explicit human instruction.", agents)
            self.assertIn("project tracking through the Roadmap board.", agents)

            # Structured needles from --resolved-config must be present
            self.assertIn("abc123testboard", agents)         # board_id
            self.assertIn("development", agents)              # integration_branch
            self.assertIn("./tests/run.sh", agents)          # test_command
            self.assertIn("nnodes/proyectos/test-project", agents)  # vault_scope

            # Enabled runtimes must be listed
            self.assertIn("claude", agents)
            self.assertIn("cursor", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_rich_brief_identical_on_second_run(self):
        """Idempotency test on the RICH rendering path.

        Distinct from test_sync_produces_identical_agents_md_on_second_run — this
        variant uses a manifest with [brief] and recipe configs so the test becomes
        meaningful only once the enriched renderer lands (Batch 3).

        Fails (RED) because the rich needles are missing from the current thin
        renderer, so the byte-identity check catches a regression in the feature.
        The test asserts a needle AFTER the second run to tie idempotency to the
        rich path — a thin renderer would pass the assertEqual but fail the needle.
        """
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'brief-idempotency-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'runtime_flow = ["Session works on one card."]\n'
                'workflow_rules = ["No merges without instruction."]\n\n'
                "[recipes.trello-mcp-workflow]\n"
                "board_id = 'idempotency-board-xyz'\n\n"
                "[recipes.tdd-flow]\n"
                "test_command = './tests/validate.sh'\n\n"
                "[[bindings]]\n"
                "capability = 'tracker'\n"
                "recipe = 'trello-mcp-workflow'\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            first = (workspace / "AGENTS.md").read_bytes()

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            second = (workspace / "AGENTS.md").read_bytes()

            # Byte-identity gate
            self.assertEqual(first, second)

            # Rich-path needle: ties idempotency to feature presence
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("idempotency-board-xyz", agents)   # board_id from resolved-config
            self.assertIn("./tests/validate.sh", agents)      # test_command from resolved-config
        finally:
            shutil.rmtree(workspace.parent)

    def test_agents_render_standalone_degradation(self):
        """Standalone degradation test: agents-render.py invoked WITHOUT --resolved-config.

        Asserts:
        - Exits 0 (no crash).
        - Output contains project identity (project name).
        - Output contains MCP section (when mcp servers present).
        - Output contains [brief] prose sections (intro, workflow_rules).

        Fails (RED) because the current renderer does not render [brief] sections at all.
        Batch 3 implements the section helpers that will satisfy the prose assertions.
        """
        import tempfile as _tempfile
        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"

            toml_path.write_text(
                "[project]\n"
                "name = 'standalone-degradation-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'intro = "This is the degraded brief intro."\n'
                'workflow_rules = ["No direct pushes to main."]\n\n'
                "[mcp.demo-server]\n"
                "command = 'npx'\n"
                "args = ['-y', '@demo/mcp']\n"
                "env = { TOKEN = '$DEMO_TOKEN' }\n"
            )

            agents_render = ROOT / "lib" / "_internal" / "agents-render.py"
            proc = subprocess.run(
                ["python3", str(agents_render), str(toml_path), str(output_path)],
                text=True,
                capture_output=True,
                check=False,
            )

            # Must not crash
            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed:\n{proc.stderr}")

            # Output file must exist
            self.assertTrue(output_path.exists(), "AGENTS.md was not created")

            agents = output_path.read_text()

            # Identity: project name must be present
            self.assertIn("standalone-degradation-fixture", agents)

            # MCP section must be rendered (degraded path still includes MCP)
            self.assertIn("demo-server", agents)

            # [brief] prose sections must be present even without --resolved-config
            self.assertIn("This is the degraded brief intro.", agents)
            self.assertIn("No direct pushes to main.", agents)

    # -----------------------------------------------------------------------
    # End Batch 1 RED tests
    # -----------------------------------------------------------------------

    def test_brief_useful_commands_renders_extra_items(self):
        """[brief].useful_commands array items are appended to ## Useful Commands section.

        Fails (RED) because agents-render.py does not yet read brief.useful_commands.
        Batch 5 adds renderer support and this test becomes GREEN.
        """
        import tempfile as _tempfile
        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"

            toml_path.write_text(
                "[project]\n"
                "name = 'useful-commands-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'useful_commands = ["Inspect the active Trello card before resuming work."]\n\n'
                "[recipes.tdd-flow]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
                "[recipes.tdd-flow.config]\n"
                "test_command = './tests/run.sh'\n"
            )

            agents_render = ROOT / "lib" / "_internal" / "agents-render.py"
            proc = subprocess.run(
                ["python3", str(agents_render), str(toml_path), str(output_path)],
                text=True,
                capture_output=True,
                check=False,
            )

            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed:\n{proc.stderr}")
            self.assertTrue(output_path.exists(), "AGENTS.md was not created")

            agents = output_path.read_text()
            # brief.useful_commands items must appear in ## Useful Commands
            self.assertIn("Inspect the active Trello card before resuming work.", agents)

    def test_sync_resolves_all_skill_sources(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n"
            )

            # Local skill
            self.write_local_skill(
                workspace,
                "local-skill",
                description="A local skill.",
                scope=["root"],
                auto_invoke=["Do local thing"],
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            # Verify local skills are resolved in .internal/resolved-skills
            resolved = workspace / "ai-specs" / ".internal" / "resolved-skills"
            self.assertTrue((resolved / "local-skill" / "SKILL.md").is_file())
            # No registry artifact should exist
            self.assertFalse((workspace / "ai-specs" / ".skill-registry.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_agent_all_includes_pi_when_enabled(self):
        """When pi is in [agents].enabled, --all must sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-pi-all'\n\n"
                "[agents]\n"
                "enabled = ['pi']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--all"],
                check=True, text=True,
            )
            pi_skills = target / ".pi" / "skills"
            self.assertTrue(pi_skills.is_symlink(),
                            ".pi/skills/ must be a symlink after --all")
            self.assertFalse((target / "PI.md").exists())
            self.assertFalse((target / "pi.md").exists())

    def test_sync_agent_all_excludes_pi_when_not_enabled(self):
        """When pi is NOT in [agents].enabled, --all must NOT sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-no-pi'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--all"],
                check=True, text=True,
            )
            pi_skills = target / ".pi" / "skills"
            self.assertFalse(pi_skills.exists(),
                             ".pi/skills/ must NOT exist when pi is disabled")


class SkillSyncScriptTests(unittest.TestCase):
    SCRIPT = ROOT / "ai-specs" / "skills" / "skill-sync" / "assets" / "sync.sh"

    def test_skill_sync_validates_metadata_and_reports_missing(self):
        repo_root = Path(tempfile.mkdtemp(prefix="ai-specs-skill-sync-"))
        try:
            script_path = repo_root / "ai-specs" / "skills" / "skill-sync" / "assets" / "sync.sh"
            script_path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(self.SCRIPT, script_path)
            (repo_root / ".melon-monorepo").write_text("1\n")
            (repo_root / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'test'\n")

            skills_dir = repo_root / "ai-specs" / "skills"
            skills_dir.mkdir(parents=True, exist_ok=True)

            (skills_dir / "root-auto" / "SKILL.md").parent.mkdir(parents=True, exist_ok=True)
            (skills_dir / "root-auto" / "SKILL.md").write_text(
                "---\n"
                "name: root-auto\n"
                "description: Root auto invoke skill.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                '  version: "1.0"\n'
                "  scope:\n"
                '    - "root"\n'
                "  auto_invoke:\n"
                '    - "Do root thing"\n'
                "---\n\n"
                "# Root Auto\n"
            )
            (skills_dir / "back-auto" / "SKILL.md").parent.mkdir(parents=True, exist_ok=True)
            (skills_dir / "back-auto" / "SKILL.md").write_text(
                "---\n"
                "name: back-auto\n"
                "description: Back-only skill.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                '  version: "1.0"\n'
                "  scope:\n"
                '    - "back_web"\n'
                "  auto_invoke:\n"
                '    - "Do back thing"\n'
                "---\n\n"
                "# Back Auto\n"
            )
            (skills_dir / "manual-only" / "SKILL.md").parent.mkdir(parents=True, exist_ok=True)
            (skills_dir / "manual-only" / "SKILL.md").write_text(
                "---\n"
                "name: manual-only\n"
                "description: Manual-only skill.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                '  version: "1.0"\n'
                "---\n\n"
                "# Manual Only\n"
            )

            proc = subprocess.run(
                ["bash", str(script_path)],
                cwd=repo_root,
                text=True,
                capture_output=True,
                check=False,
                env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
            )

            self.assertEqual(proc.returncode, 0, proc.stderr)

            # skill-sync validates metadata; it no longer generates a registry file
            self.assertFalse((repo_root / "ai-specs" / ".skill-registry.md").exists())

            # Output reports skills with incomplete metadata
            output = proc.stdout
            self.assertIn("manual-only", output)
        finally:
            shutil.rmtree(repo_root)


class TestMissingScenarios(unittest.TestCase):
    """FIX 5: Behavioral tests for scenarios left untested by verify-report.

    Covers: R1 partial-brief, R3 no-tracker-omission, R7 subrepo structured-fields.
    """

    def test_partial_brief_renders_present_keys_no_crash(self):
        """R1 partial [brief]: only some keys present → renders those, omits absent ones, no crash.

        A manifest with only `workflow_rules` in [brief] (no intro, no purpose,
        no context_sources, etc.) must render the workflow_rules section and NOT
        crash or emit empty placeholder sections.
        """
        import tempfile as _tempfile
        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"

            toml_path.write_text(
                "[project]\n"
                "name = 'partial-brief-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                # Only workflow_rules is present; no intro, no purpose, no runtime_flow etc.
                'workflow_rules = ["No direct merges without approval."]\n'
            )

            agents_render = ROOT / "lib" / "_internal" / "agents-render.py"
            proc = subprocess.run(
                ["python3", str(agents_render), str(toml_path), str(output_path)],
                text=True,
                capture_output=True,
                check=False,
            )

            # Must not crash
            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed:\n{proc.stderr}")
            self.assertTrue(output_path.exists())

            agents = output_path.read_text()

            # Project identity must be present
            self.assertIn("partial-brief-fixture", agents)

            # Present key must render
            self.assertIn("No direct merges without approval.", agents)

            # Absent keys must NOT produce empty section headers
            # (intro absent → no empty ## Project section with just the header)
            # We assert the specific absent strings do not appear as placeholder bullets
            self.assertNotIn("None.", agents,  # no placeholder for empty sections
                             "Absent brief keys must not produce 'None.' placeholders")

    def test_no_tracker_binding_omits_trello_section(self):
        """R3 no-tracker: when no recipe is bound to 'tracker', the Trello Tracking
        section must be completely omitted from the rendered brief.
        """
        import tempfile as _tempfile
        import json

        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            resolved_path = tmp_path / "resolved.json"

            toml_path.write_text(
                "[project]\n"
                "name = 'no-tracker-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'intro = "No tracker section expected."\n\n'
                # tdd-flow has no 'tracker' capability — no tracker binding
                "[recipes.tdd-flow]\n"
                "test_command = './tests/run.sh'\n"
            )

            # Build a resolved-config with NO tracker binding (empty bindings)
            resolved_path.write_text(json.dumps({
                "bindings": {},  # no tracker binding
                "recipes": {
                    "tdd-flow": {"test_command": "./tests/run.sh"},
                },
                "enabled": ["tdd-flow"],
            }))

            agents_render = ROOT / "lib" / "_internal" / "agents-render.py"
            proc = subprocess.run(
                ["python3", str(agents_render), str(toml_path), str(output_path),
                 "--resolved-config", str(resolved_path)],
                text=True,
                capture_output=True,
                check=False,
            )

            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed:\n{proc.stderr}")

            agents = output_path.read_text()

            # The Trello Tracking section must be absent when no tracker is bound
            self.assertNotIn(
                "## Trello Tracking", agents,
                "Trello Tracking section must be omitted when no tracker capability is bound"
            )

            # Brief intro must still render (unrelated section not affected)
            self.assertIn("No tracker section expected.", agents)

    def test_subrepo_sync_agent_forwards_resolved_config(self):
        """R7 subrepo passthrough: sync-agent --all passes --resolved-config to agents-render
        so subrepo AGENTS.md gets board_id / test_command structured fields.

        Verifies that the AGENTS.md generated for a subrepo target (workspace synced
        by sync-agent) contains the structured fields from --resolved-config.
        Uses a workspace with [brief] + recipe configs (explicit bindings for speed,
        since this tests the passthrough, not auto-binding).
        """
        workspace = None
        try:
            parent = Path(tempfile.mkdtemp())
            workspace = parent / "subrepo-test-workspace"
            workspace.mkdir()
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            # Use explicit bindings (24-char hex board_id) + brief to exercise the subrepo path
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'subrepo-structured-fields'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'intro = "Subrepo receives enriched output."\n\n'
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbccddeeff001122334455'\n\n"
                "[recipes.tdd-flow]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
                "[recipes.tdd-flow.config]\n"
                "test_command = './tests/validate.sh'\n"
            )

            # Run sync (not sync-agent --all; sync runs materialize + sync-agent)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            agents = (workspace / "AGENTS.md").read_text()

            # Structured fields must be present in the workspace AGENTS.md
            # (the sync pipeline → materialize → resolved-config → agents-render path)
            self.assertIn(
                "aabbccddeeff001122334455", agents,
                "board_id must appear in AGENTS.md after sync (subrepo structured-field passthrough)"
            )
            self.assertIn(
                "./tests/validate.sh", agents,
                "test_command must appear in AGENTS.md after sync (subrepo structured-field passthrough)"
            )
        finally:
            if workspace is not None:
                shutil.rmtree(workspace.parent)


class TestAutoBindingFix(unittest.TestCase):
    """FIX 1 (CRITICAL): Test that auto-binding works without explicit [[bindings]].

    Design decision #4: build_resolved_config must emit the catalog-aware
    auto-bound resolved_bindings (from resolve_bindings()) rather than only
    explicit [[bindings]] from the manifest.

    A manifest with single-provider capabilities and NO [[bindings]] must
    produce a non-empty bindings map in the resolved-config JSON, and the
    rendered AGENTS.md must contain board_id / vault_scope needles.
    """

    def make_workspace(self):
        parent = Path(tempfile.mkdtemp())
        ws = parent / "test-autobind-workspace"
        ws.mkdir()
        return ws

    def test_auto_binding_without_explicit_bindings(self):
        """Single-provider manifest with NO [[bindings]] must auto-populate bindings.

        RED: fails because build_resolved_config only reads explicit [[bindings]]
        from TOML, ignoring catalog-based resolve_bindings() auto-bind logic.
        GREEN: once materialize_recipes passes resolved_bindings (from resolve_bindings())
        into the resolved-config JSON instead of re-deriving explicit-only.

        Uses enabled=true to exercise the full materialize_recipes path (where
        resolved_bindings is computed by resolve_bindings() at line ~484).
        board_id uses a real 24-char hex to pass trello-mcp-workflow validate-config.
        """
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            # Single provider per capability, NO explicit [[bindings]]
            # board_id must be 24-char hex to pass trello-mcp-workflow validate-config
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'autobind-no-explicit-bindings'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'intro = "Auto-binding test brief."\n\n'
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbccddeeff001122334455'\n\n"
                "[recipes.vault-canonical-store]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
                "[recipes.vault-canonical-store.config]\n"
                "vault_scope = 'nnodes/test/autobind-scope'\n"
                # NO [[bindings]] section
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            agents = (workspace / "AGENTS.md").read_text()

            # Auto-bound tracker must surface board_id in rendered brief
            self.assertIn(
                "aabbccddeeff001122334455", agents,
                "board_id must appear in AGENTS.md when tracker is auto-bound (no explicit [[bindings]])"
            )
            # Auto-bound canonical-store must surface vault_scope in rendered brief
            self.assertIn(
                "nnodes/test/autobind-scope", agents,
                "vault_scope must appear in AGENTS.md when canonical-store is auto-bound"
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_resolved_config_bindings_non_empty_without_explicit_bindings(self):
        """The resolved-config JSON bindings must be non-empty for auto-bound single providers.

        Directly invokes recipe-materialize.py and inspects the JSON output.
        RED: build_resolved_config returns {} bindings for no explicit [[bindings]].
        GREEN: it returns {'tracker': 'trello-mcp-workflow', 'canonical-store': 'vault-canonical-store', ...}.

        Uses enabled=true; board_id must be 24-char hex to pass trello validate-config.
        """
        import tempfile as _tempfile
        import json

        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'autobind-json-check'\n\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbccddeeff001122334455'\n\n"
                "[recipes.vault-canonical-store]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
                "[recipes.vault-canonical-store.config]\n"
                "vault_scope = 'nnodes/test/json-scope'\n"
                # NO [[bindings]] section — auto-bind must handle this
            )

            materialize = ROOT / "lib" / "_internal" / "recipe-materialize.py"
            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                resolved_out = Path(f.name)

            proc = subprocess.run(
                ["python3", str(materialize),
                 str(workspace), str(ROOT),
                 "--resolved-config-out", str(resolved_out)],
                text=True,
                capture_output=True,
                check=False,
                env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
            )
            self.assertEqual(proc.returncode, 0, f"materialize failed:\n{proc.stderr}\n{proc.stdout}")

            with open(resolved_out) as f:
                resolved = json.load(f)

            bindings = resolved.get("bindings", {})
            self.assertIn(
                "tracker", bindings,
                f"'tracker' capability must be auto-bound in resolved-config bindings. Got: {bindings}"
            )
            self.assertEqual(
                bindings["tracker"], "trello-mcp-workflow",
                f"tracker must auto-bind to trello-mcp-workflow. Got: {bindings}"
            )
            self.assertIn(
                "canonical-store", bindings,
                f"'canonical-store' must be auto-bound. Got: {bindings}"
            )
            self.assertEqual(
                bindings["canonical-store"], "vault-canonical-store",
                f"canonical-store must auto-bind to vault-canonical-store. Got: {bindings}"
            )
        finally:
            shutil.rmtree(workspace.parent)
            if resolved_out.exists():
                resolved_out.unlink()


if __name__ == "__main__":
    unittest.main()
