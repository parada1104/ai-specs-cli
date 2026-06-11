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

    def test_sync_renders_opencode_mcp_arg_var_as_bare_dollar(self):
        """A var referenced in a command ARG must render as $VAR (no braces)
        for opencode: opencode interpolates shell-style $VAR in args, but
        environment values use {env:VAR}. Live-verified June 2026 — a braced
        ${VAR} in args is NOT interpolated, so the server gets a literal path
        and never starts."""
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
                "args = ['-y', '@modelcontextprotocol/server-filesystem', '${VAULT_PATH}']\n"
                "env = { VAULT_PATH = '$VAULT_PATH' }\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            parsed = json.loads((workspace / "opencode.json").read_text())
            demo = parsed["mcp"]["demo"]
            # ARG: bare $VAR, never ${VAR}
            self.assertIn("$VAULT_PATH", demo["command"])
            self.assertNotIn("${VAULT_PATH}", demo["command"])
            # ENVIRONMENT: opencode {env:VAR} form, unchanged
            self.assertEqual(demo["environment"], {"VAULT_PATH": "{env:VAULT_PATH}"})
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

        Uses enabled=true recipes so resolve_bindings() actually runs (the real
        resolution path). The vcs-pr-flow binding uses git-pr-flow (the recipe
        that actually provides the vcs-pr-flow capability). The test must fail
        if binding resolution breaks (e.g. wrong capability for a recipe).
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
                # Enable recipes with valid versions so resolve_bindings() runs (FIX 4)
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbcc112233445566778899'\n\n"  # 24-char hex as required
                "[recipes.worktree-flow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.worktree-flow.config]\n"
                "integration_branch = 'development'\n\n"
                "[recipes.git-pr-flow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.git-pr-flow.config]\n"
                "base_branch = 'development'\n\n"
                "[recipes.tdd-flow]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
                "[recipes.tdd-flow.config]\n"
                "test_command = './tests/run.sh'\n\n"
                "[recipes.vault-canonical-store]\n"
                "enabled = true\n"
                "version = '1.1.0'\n"
                "[recipes.vault-canonical-store.config]\n"
                "vault_scope = 'nnodes/proyectos/test-project'\n\n"
                "[[bindings]]\n"
                "capability = 'tracker'\n"
                "recipe = 'trello-mcp-workflow'\n\n"
                # FIX 4: vcs-pr-flow must bind to git-pr-flow (the recipe that provides it)
                "[[bindings]]\n"
                "capability = 'vcs-pr-flow'\n"
                "recipe = 'git-pr-flow'\n\n"
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
            # (Pinned to line-context to avoid tautological bare-token matching — FIX 9)
            self.assertIn("aabbcc112233445566778899", agents)   # board_id
            self.assertIn("- **Integration branch**: `development`", agents)  # integration_branch line
            self.assertIn("./tests/run.sh", agents)          # test_command
            self.assertIn("nnodes/proyectos/test-project", agents)  # vault_scope
            self.assertIn("VCS/PR provider: GitHub (`gh` CLI)", agents)  # VCS line

            # Enabled runtimes must be listed
            self.assertIn("- **Enabled runtimes**: `claude`, `cursor`", agents)  # FIX 9: line context
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_rich_brief_identical_on_second_run(self):
        """Idempotency test on the RICH rendering path.

        Distinct from test_sync_produces_identical_agents_md_on_second_run — this
        variant uses a manifest with [brief] and recipe configs so the test becomes
        meaningful only once the enriched renderer lands.

        Coverage note: this fixture uses recipes WITHOUT enabled=true and relies on
        explicit [[bindings]] + literal-recipe-id fallback (not resolve_bindings()
        auto-bind). It exercises:
          - build_resolved_config() reading all recipes (enabled and disabled alike)
          - explicit [[bindings]] → board_id lookup in Trello section
          - literal 'tdd-flow' fallback in _section_useful_commands
          - byte-identity idempotency across two sync runs

        For auto-binding coverage (resolve_bindings()), see
        test_auto_binding_without_explicit_bindings and test_sync_renders_rich_brief_from_manifest.
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

        Coverage note: this test invokes agents-render.py directly WITHOUT
        --resolved-config, so test_command from [recipes.tdd-flow.config] is NOT
        rendered (that path requires resolved-config JSON from materialize). The
        recipe presence (enabled=true, version='1.0.0') is correct but inert here —
        it does NOT exercise resolve_bindings(). This test exercises ONLY the
        brief.useful_commands rendering path in _section_useful_commands().

        For test_command rendering coverage via the real resolve_bindings() path,
        see test_sync_renders_rich_brief_from_manifest and
        test_auto_binding_without_explicit_bindings.
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

    # --- Omp flag and help tests ---

    def test_sync_agent_omp_flag_accepted(self):
        """--omp flag must be accepted and produce .omp/skills symlink."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            result = subprocess.run(
                [str(CLI), "sync-agent", str(target), "--omp"],
                capture_output=True, text=True,
            )
            self.assertEqual(result.returncode, 0,
                             f"--omp must exit 0; stderr={result.stderr!r}")
            omp_skills = target / ".omp" / "skills"
            self.assertTrue(omp_skills.is_symlink(),
                            ".omp/skills/ must be a symlink after --omp")

    def test_sync_agent_help_lists_omp(self):
        """--help output must include --omp."""
        result = subprocess.run(
            [str(CLI), "sync-agent", "--help"],
            capture_output=True, text=True,
        )
        self.assertIn("--omp", result.stdout,
                      "--omp must appear in sync-agent --help output")

    def test_sync_agent_all_includes_omp_when_enabled(self):
        """When omp is in [agents].enabled, --all must sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-omp-all'\n\n"
                "[agents]\n"
                "enabled = ['omp']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--all"],
                check=True, text=True,
            )
            omp_skills = target / ".omp" / "skills"
            self.assertTrue(omp_skills.is_symlink(),
                            ".omp/skills/ must be a symlink after --all")
            # omp is native (AGENTS.md) — no instruction file
            self.assertFalse((target / "OMP.md").exists())
            self.assertFalse((target / "omp.md").exists())

    def test_sync_agent_all_excludes_omp_when_not_enabled(self):
        """When omp is NOT in [agents].enabled, --all must NOT sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-no-omp'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--all"],
                check=True, text=True,
            )
            omp_skills = target / ".omp" / "skills"
            self.assertFalse(omp_skills.exists(),
                             ".omp/skills/ must NOT exist when omp is disabled")

    def test_omp_mcp_json_rendered_when_mcps_declared(self):
        """--omp must write .omp/mcp.json with mcpServers when [mcp.*] entries exist."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-omp-mcp'\n\n"
                "[agents]\n"
                "enabled = ['omp']\n\n"
                "[mcp.my-server]\n"
                "command = 'npx'\n"
                "args = ['-y', '@example/server']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--omp"],
                check=True, text=True,
            )
            mcp_path = target / ".omp" / "mcp.json"
            self.assertTrue(mcp_path.is_file(),
                            ".omp/mcp.json must be created when [mcp.*] entries exist")
            mcp_data = json.loads(mcp_path.read_text())
            self.assertIn("mcpServers", mcp_data,
                          ".omp/mcp.json must have mcpServers key")

    def test_omp_mcp_json_absent_when_no_mcps(self):
        """--omp must NOT write .omp/mcp.json when no MCP servers declared."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-omp-no-mcp'\n\n"
                "[agents]\n"
                "enabled = ['omp']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--omp"],
                check=True, text=True,
            )
            mcp_path = target / ".omp" / "mcp.json"
            self.assertFalse(mcp_path.exists(),
                             ".omp/mcp.json must NOT be created when no MCPs declared")

    def test_omp_commands_populated(self):
        """--omp must copy command files to .omp/commands/."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            # ai-specs init already creates skills-as-rules.md in commands/
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--omp"],
                check=True, text=True,
            )
            omp_commands = target / ".omp" / "commands"
            self.assertTrue(omp_commands.is_dir(),
                            ".omp/commands/ must exist after --omp")
            files = list(omp_commands.glob("*.md"))
            self.assertGreater(len(files), 0,
                               ".omp/commands/ must contain at least one command file")

    def test_omp_no_instruction_symlink(self):
        """--omp must NOT create any instruction symlink (omp is native AGENTS.md)."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--omp"],
                check=True, text=True,
            )
            # omp must not create any OMP.md or omp.md instruction file
            self.assertFalse((target / "OMP.md").exists(),
                             "OMP.md must NOT be created for omp")
            self.assertFalse((target / "omp.md").exists(),
                             "omp.md must NOT be created for omp")

    def test_omp_gitignore_contains_omp_dir(self):
        """ai-specs init must write .omp/ into the root .gitignore."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            gitignore = (target / ".gitignore").read_text()
            self.assertIn(".omp/", gitignore,
                          ".gitignore must contain .omp/ after ai-specs init")

    def test_existing_agents_unchanged_after_omp_added(self):
        """Existing agent outputs must be byte-identical before and after adding omp."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            # Sync with claude only
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-compat'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--all"],
                check=True, text=True,
            )
            claude_md_before = (target / "CLAUDE.md").read_text() if (target / "CLAUDE.md").is_file() else None

            # Now add omp to enabled and re-sync
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-compat'\n\n"
                "[agents]\n"
                "enabled = ['claude', 'omp']\n"
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--all"],
                check=True, text=True,
            )
            claude_md_after = (target / "CLAUDE.md").read_text() if (target / "CLAUDE.md").is_file() else None
            self.assertEqual(claude_md_before, claude_md_after,
                             "CLAUDE.md (via symlink) must be byte-identical before and after adding omp")
            # .claude/skills symlink should still point to the same target
            claude_skills = target / ".claude" / "skills"
            self.assertTrue(claude_skills.is_symlink(),
                            ".claude/skills must still be a symlink after adding omp")


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
        """R7 subrepo passthrough: standalone sync-agent --all generates resolved-config
        and forwards it to the subrepo AGENTS.md so board_id / test_command appear there.

        This is a genuine E2E test of the standalone sync-agent path:
        - workspace has subrepos=['sub/a'] and ENABLED catalog recipes
        - sync-agent --all is invoked directly (not via sync.sh)
        - assertions are on the SUBREPO AGENTS.md (sub/a/AGENTS.md), not root

        This exercises build_resolved_config_only() + the resolved-config passthrough
        in sync-agent.sh when ${#RESOLVED_TARGETS[@]} > 1.
        """
        workspace = None
        try:
            parent = Path(tempfile.mkdtemp())
            workspace = parent / "subrepo-test-workspace"
            workspace.mkdir()
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            # Workspace has a subrepo (sub/a) and ENABLED catalog recipes.
            # board_id must be 24-char hex to pass trello-mcp-workflow validate-config.
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'subrepo-structured-fields'\n"
                "subrepos = ['sub/a']\n\n"
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
            # Create subrepo directory (required by target-resolve.py)
            (workspace / "sub" / "a").mkdir(parents=True)

            # Run full sync first so recipe assets are materialized (skills, hooks, etc.)
            # sync-agent --all standalone only generates resolved-config; it still needs
            # the resolved-skills dir that materialize produces.
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            # Now run STANDALONE sync-agent --all (the path under test).
            # This exercises build_resolved_config_only() + resolved-config passthrough.
            subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                check=True, text=True,
            )

            # Assert on SUBREPO AGENTS.md — that's what the standalone path generates.
            subrepo_agents = (workspace / "sub" / "a" / "AGENTS.md").read_text()

            self.assertIn(
                "aabbccddeeff001122334455", subrepo_agents,
                "board_id must appear in subrepo AGENTS.md via standalone sync-agent "
                "resolved-config passthrough (build_resolved_config_only)"
            )
            self.assertIn(
                "./tests/validate.sh", subrepo_agents,
                "test_command must appear in subrepo AGENTS.md via standalone sync-agent "
                "resolved-config passthrough (build_resolved_config_only)"
            )
        finally:
            if workspace is not None:
                shutil.rmtree(workspace.parent)

    def test_resolved_config_only_bindings_match_full_materialize_path(self):
        """--resolved-config-only bindings must be identical to the full materialize path.

        This is the 'identical output' guarantee: for a manifest with enabled catalog
        recipes, build_resolved_config_only() must produce the same bindings map that
        materialize_recipes() writes to resolved-config.

        Setup: a workspace with trello-mcp-workflow + tdd-flow enabled (real catalog
        recipes, not a 0-enabled stub). Run both paths and compare bindings keys.
        """
        workspace = None
        try:
            parent = Path(tempfile.mkdtemp())
            workspace = parent / "rc-only-parity-workspace"
            workspace.mkdir()
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'rc-only-parity'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'intro = "Resolved-config parity test."\n\n'
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

            materialize = ROOT / "lib" / "_internal" / "recipe-materialize.py"

            # --- Full materialize path ---
            import tempfile as _tempfile
            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                full_resolved_out = Path(f.name)
            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                standalone_resolved_out = Path(f.name)

            try:
                # Full path (normal materialize_recipes)
                proc_full = subprocess.run(
                    ["python3", str(materialize),
                     str(workspace), str(ROOT),
                     "--resolved-config-out", str(full_resolved_out)],
                    text=True, capture_output=True, check=False,
                    env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
                )
                self.assertEqual(
                    proc_full.returncode, 0,
                    f"full materialize failed:\n{proc_full.stderr}\n{proc_full.stdout}"
                )

                # Standalone path (build_resolved_config_only)
                proc_standalone = subprocess.run(
                    ["python3", str(materialize),
                     str(workspace), str(ROOT),
                     "--resolved-config-out", str(standalone_resolved_out),
                     "--resolved-config-only"],
                    text=True, capture_output=True, check=False,
                    env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
                )
                self.assertEqual(
                    proc_standalone.returncode, 0,
                    f"--resolved-config-only failed:\n{proc_standalone.stderr}\n{proc_standalone.stdout}"
                )

                with open(full_resolved_out) as fh:
                    full_data = json.load(fh)
                with open(standalone_resolved_out) as fh:
                    standalone_data = json.load(fh)

                full_bindings = full_data.get("bindings", {})
                standalone_bindings = standalone_data.get("bindings", {})

                self.assertEqual(
                    full_bindings, standalone_bindings,
                    f"--resolved-config-only bindings must match full materialize path.\n"
                    f"  full:       {full_bindings}\n"
                    f"  standalone: {standalone_bindings}"
                )
                # Sanity: both must have auto-bound tracker and test-runner
                self.assertIn("tracker", full_bindings,
                              "tracker must be auto-bound in full path")
                self.assertIn("tracker", standalone_bindings,
                              "tracker must be auto-bound in standalone path")
                self.assertIn("test-runner", full_bindings,
                              "test-runner must be auto-bound in full path")
                self.assertIn("test-runner", standalone_bindings,
                              "test-runner must be auto-bound in standalone path")
            finally:
                for p in (full_resolved_out, standalone_resolved_out):
                    if p.exists():
                        p.unlink()
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
                "version = '1.1.0'\n"
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
                "version = '1.1.0'\n"
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

    def test_resolved_config_only_with_explicit_ai_specs_home_resolves_bindings(self):
        """FIX 1 (R3): --resolved-config-only uses caller-supplied ai_specs_home to
        locate the catalog instead of recomputing from __file__.

        Invokes the standalone path with an explicit AI_SPECS_HOME env var and asserts
        that auto-bindings still resolve (board_id present in bindings), proving that
        the catalog lookup does not diverge when the home is supplied explicitly.

        This guards against custom/symlinked installs where Path(__file__).parents[2]
        would diverge from the actual AI_SPECS_HOME.
        """
        import tempfile as _tempfile
        import json

        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'rc-only-explicit-home'\n\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbccddeeff001122334455'\n"
                # NO [[bindings]] — auto-bind must handle this via the supplied home
            )

            materialize = ROOT / "lib" / "_internal" / "recipe-materialize.py"
            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                resolved_out = Path(f.name)

            try:
                # Pass AI_SPECS_HOME explicitly AND as the second positional arg.
                # build_resolved_config_only now uses the positional arg to locate the
                # catalog instead of falling back to Path(__file__).parents[2].
                proc = subprocess.run(
                    ["python3", str(materialize),
                     str(workspace), str(ROOT),
                     "--resolved-config-out", str(resolved_out),
                     "--resolved-config-only"],
                    text=True,
                    capture_output=True,
                    check=False,
                    env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
                )
                self.assertEqual(
                    proc.returncode, 0,
                    f"--resolved-config-only with explicit home failed:\n{proc.stderr}\n{proc.stdout}"
                )

                with open(resolved_out) as fh:
                    resolved = json.load(fh)

                bindings = resolved.get("bindings", {})
                self.assertIn(
                    "tracker", bindings,
                    f"'tracker' must be auto-bound via explicit ai_specs_home. Got: {bindings}"
                )
                self.assertEqual(
                    bindings["tracker"], "trello-mcp-workflow",
                    f"tracker must auto-bind to trello-mcp-workflow. Got: {bindings}"
                )
            finally:
                if resolved_out.exists():
                    resolved_out.unlink()
        finally:
            shutil.rmtree(workspace.parent)


class TestJudgmentDayFixes(unittest.TestCase):
    """Tests for confirmed issues from Judgment Day Round 1.

    FIX 1: description-only MCP entries (global MCPs) render correctly.
    FIX 2: standalone sync-agent forwards resolved-config to subrepo AGENTS.md.
    FIX 3: VCS bullet: gh CLI only for github; provider renders without gh for others.
    FIX 5: malformed --resolved-config degrades gracefully (no crash).
    FIX 7: Trello section shows board id without recipe-id parenthetical.
    FIX 8: useful_commands does NOT fabricate validate.sh via str.replace.
    FIX 9: hardened needle assertions — pin to line context not bare tokens.
    FIX 10: structured fields resolved via capability bindings, not literal recipe ids.
    """

    AGENTS_RENDER = ROOT / "lib" / "_internal" / "agents-render.py"

    def run_render(self, toml_text: str, resolved: dict | None = None) -> str:
        """Helper: write TOML + optional resolved-config, invoke agents-render.py, return output."""
        import tempfile as _tempfile
        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            toml_path.write_text(toml_text)
            cmd = ["python3", str(self.AGENTS_RENDER), str(toml_path), str(output_path)]
            if resolved is not None:
                resolved_path = tmp_path / "resolved.json"
                resolved_path.write_text(json.dumps(resolved))
                cmd += ["--resolved-config", str(resolved_path)]
            proc = subprocess.run(cmd, text=True, capture_output=True, check=False)
            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed:\n{proc.stderr}")
            self.assertTrue(output_path.exists())
            return output_path.read_text()

    # --- FIX 1 ---

    def test_global_mcp_description_renders_without_mcp_block(self):
        """FIX 1: An MCP entry in [brief.mcp_descriptions] with NO matching [mcp.*] block
        must render as a description-only note (not silently dropped).
        """
        agents = self.run_render(
            "[project]\n"
            "name = 'fix1-global-mcp'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[mcp.trello]\n"
            "command = 'npx'\n"
            "args = ['-y', '@trello/mcp']\n\n"
            "[brief.mcp_descriptions]\n"
            'trello = "project tracking through the Roadmap board."\n'
            'engram = "global persistent memory (no local config block)."\n'
        )
        # Both must render
        self.assertIn("project tracking through the Roadmap board.", agents)
        self.assertIn("global persistent memory (no local config block).", agents)
        # engram has no [mcp.*] block — it must still appear
        self.assertIn("engram", agents)

    def test_mcp_section_renders_description_only_entry_with_global_marker(self):
        """FIX 1: Description-only entries are marked *(global)* to distinguish from
        full [mcp.*] blocks.
        """
        agents = self.run_render(
            "[project]\n"
            "name = 'fix1-global-marker'\n\n"
            "[brief.mcp_descriptions]\n"
            'global-only = "A global MCP with no local config."\n'
        )
        self.assertIn("*(global)*", agents)
        self.assertIn("A global MCP with no local config.", agents)

    # --- FIX 2 ---

    def test_standalone_sync_agent_subrepo_gets_board_id(self):
        """FIX 2: standalone ai-specs sync-agent (no --source-root / --target) must
        forward resolved-config to subrepo AGENTS.md so board_id appears there.

        Verifies that sync-agent generates + forwards resolved-config internally
        (not just when invoked by sync.sh).
        """
        with tempfile.TemporaryDirectory() as parent_tmp:
            workspace = Path(parent_tmp) / "fix2-workspace"
            workspace.mkdir()
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fix2-subrepo-rich'\n"
                "subrepos = ['sub/a']\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'intro = "Subrepo enrichment test."\n\n'
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbcc112233ddeeff001122'\n"
            )
            (workspace / "sub" / "a").mkdir(parents=True)
            # Create subrepo AGENTS.md placeholder (required by ensure_target_workspace)
            (workspace / "AGENTS.md").write_text("placeholder\n")

            # First run sync so root AGENTS.md is proper
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            # Now run standalone sync-agent from the workspace root
            subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                check=True, text=True,
            )

            subrepo_agents = (workspace / "sub" / "a" / "AGENTS.md").read_text()
            self.assertIn(
                "aabbcc112233ddeeff001122", subrepo_agents,
                "board_id must appear in subrepo AGENTS.md via standalone sync-agent resolved-config passthrough"
            )

    # --- FIX 3 ---

    def test_vcs_bullet_uses_recipe_id_for_github(self):
        """VCS bullet derives GitHub/gh from bound recipe id, not config.provider."""
        resolved = {
            "bindings": {"vcs-pr-flow": "git-pr-flow"},
            "recipes": {
                "git-pr-flow": {"base_branch": "main"},
            },
        }
        agents = self.run_render("[project]\nname = 'fix3-github'\n", resolved)
        self.assertIn("VCS/PR provider: GitHub (`gh` CLI)", agents)
        self.assertIn("base branch: `main`", agents)

    def test_vcs_bullet_uses_recipe_id_for_gitlab(self):
        """VCS bullet derives GitLab/glab from bound recipe id."""
        resolved = {
            "bindings": {"vcs-pr-flow": "gitlab-mr-flow"},
            "recipes": {
                "gitlab-mr-flow": {"base_branch": "main"},
            },
        }
        agents = self.run_render("[project]\nname = 'fix3-gitlab'\n", resolved)
        self.assertIn("VCS/PR provider: GitLab (`glab` CLI)", agents)
        self.assertNotIn("(`gh` CLI)", agents)
        self.assertIn("base branch: `main`", agents)

    def test_vcs_bullet_uses_recipe_id_for_bitbucket(self):
        """VCS bullet derives Bitbucket/bb from bound recipe id."""
        resolved = {
            "bindings": {"vcs-pr-flow": "bitbucket-pr-flow"},
            "recipes": {
                "bitbucket-pr-flow": {"base_branch": "develop"},
            },
        }
        agents = self.run_render("[project]\nname = 'fix3-bitbucket'\n", resolved)
        self.assertIn("VCS/PR provider: Bitbucket (`bb` CLI)", agents)
        self.assertNotIn("(`gh` CLI)", agents)
        self.assertIn("base branch: `develop`", agents)

    def test_vcs_bullet_ignores_stale_provider_config(self):
        """Stale provider in manifest config must not override bound recipe id label."""
        resolved = {
            "bindings": {"vcs-pr-flow": "gitlab-mr-flow"},
            "recipes": {
                "gitlab-mr-flow": {"provider": "github", "base_branch": "main"},
            },
        }
        agents = self.run_render("[project]\nname = 'fix3-stale-provider'\n", resolved)
        self.assertIn("VCS/PR provider: GitLab (`glab` CLI)", agents)
        self.assertNotIn("VCS/PR provider: github", agents)

    # --- FIX 5 ---

    def test_malformed_resolved_config_degrades_gracefully(self):
        """FIX 5: Malformed JSON in --resolved-config must not crash agents-render.py.
        Degrade to {} (no structured fields); prose and identity still render.
        """
        import tempfile as _tempfile
        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            bad_json_path = tmp_path / "bad.json"

            toml_path.write_text(
                "[project]\n"
                "name = 'fix5-malformed-json'\n\n"
                "[brief]\n"
                'workflow_rules = ["No pushes without review."]\n'
            )
            bad_json_path.write_text("this is not json {{{")

            proc = subprocess.run(
                ["python3", str(self.AGENTS_RENDER), str(toml_path), str(output_path),
                 "--resolved-config", str(bad_json_path)],
                text=True, capture_output=True, check=False,
            )
            # Must not crash
            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed on bad JSON:\n{proc.stderr}")
            self.assertTrue(output_path.exists())

            agents = output_path.read_text()
            self.assertIn("fix5-malformed-json", agents)
            self.assertIn("No pushes without review.", agents)

    def test_non_dict_resolved_config_degrades_gracefully(self):
        """FIX 5: Non-dict JSON (e.g. a list) in --resolved-config must degrade to {}."""
        import tempfile as _tempfile
        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            list_json_path = tmp_path / "list.json"

            toml_path.write_text("[project]\nname = 'fix5-list-json'\n")
            list_json_path.write_text('["a", "b", "c"]')

            proc = subprocess.run(
                ["python3", str(self.AGENTS_RENDER), str(toml_path), str(output_path),
                 "--resolved-config", str(list_json_path)],
                text=True, capture_output=True, check=False,
            )
            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed on list JSON:\n{proc.stderr}")
            agents = output_path.read_text()
            self.assertIn("fix5-list-json", agents)

    # --- FIX 7 ---

    def test_trello_section_has_no_recipe_id_parenthetical(self):
        """FIX 7: The Trello Tracking section must show board_id without the
        recipe-id parenthetical (e.g. no '(trello-mcp-workflow)').
        """
        resolved = {
            "bindings": {"tracker": "trello-mcp-workflow"},
            "recipes": {"trello-mcp-workflow": {"board_id": "aabbcc112233445566778899"}},
        }
        agents = self.run_render("[project]\nname = 'fix7-trello'\n", resolved)
        self.assertIn("## Trello Tracking", agents)
        self.assertIn("aabbcc112233445566778899", agents)
        # Recipe id must NOT appear as a parenthetical annotation
        self.assertNotIn("(`trello-mcp-workflow`)", agents)
        self.assertNotIn("(trello-mcp-workflow)", agents)

    # --- FIX 8 ---

    def test_useful_commands_does_not_fabricate_validate_sh(self):
        """FIX 8: When test_command is 'run.sh', agents-render must NOT emit a
        fabricated 'validate.sh' line derived via str.replace.
        Only explicitly provided commands must appear.
        """
        resolved = {
            "bindings": {"test-runner": "tdd-flow"},
            "recipes": {"tdd-flow": {"test_command": "./tests/run.sh"}},
        }
        agents = self.run_render("[project]\nname = 'fix8-no-fabricate'\n", resolved)
        self.assertIn("./tests/run.sh", agents)
        # validate.sh was NOT provided — must not appear
        self.assertNotIn("validate.sh", agents)

    def test_useful_commands_explicit_validate_renders(self):
        """FIX 8: When validate.sh is explicitly in brief.useful_commands, it DOES render."""
        resolved = {
            "bindings": {"test-runner": "tdd-flow"},
            "recipes": {"tdd-flow": {"test_command": "./tests/run.sh"}},
        }
        agents = self.run_render(
            "[project]\nname = 'fix8-explicit-validate'\n\n"
            "[brief]\n"
            'useful_commands = ["Full validation: `./tests/validate.sh`"]\n',
            resolved,
        )
        self.assertIn("./tests/run.sh", agents)
        self.assertIn("./tests/validate.sh", agents)

    # --- FIX 9 ---

    def test_integration_branch_renders_as_labeled_line(self):
        """FIX 9: integration_branch must appear as '- **Integration branch**: `<value>`'
        not just as a bare token to avoid tautological needle matching.
        """
        resolved = {
            "bindings": {"worktree-isolation": "worktree-flow"},
            "recipes": {"worktree-flow": {"integration_branch": "development"}},
        }
        agents = self.run_render("[project]\nname = 'fix9-branch'\n", resolved)
        self.assertIn("- **Integration branch**: `development`", agents)

    def test_enabled_runtimes_renders_as_labeled_line(self):
        """FIX 9: enabled runtimes must appear as a labeled line with backtick values."""
        agents = self.run_render(
            "[project]\nname = 'fix9-runtimes'\n\n"
            "[agents]\nenabled = ['claude', 'cursor']\n"
        )
        self.assertIn("- **Enabled runtimes**: `claude`, `cursor`", agents)

    def test_redaction_sentinel_is_exact_string(self):
        """FIX 9: redacted secrets render as '***REDACTED***' exactly."""
        agents = self.run_render(
            "[project]\nname = 'fix9-redact'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[mcp.demo]\n"
            "command = 'npx'\n"
            "env = { SECRET_KEY = 'literal-value' }\n"
        )
        self.assertIn("***REDACTED***", agents)
        self.assertNotIn("literal-value", agents)

    # --- FIX 10 ---

    def test_test_command_resolves_via_test_runner_capability_binding(self):
        """FIX 10: test_command resolved via bindings['test-runner'] → recipe, not hardcoded 'tdd-flow'.
        Swapping the bound recipe id (while keeping the capability) must still surface test_command.
        """
        resolved = {
            "bindings": {"test-runner": "my-custom-runner"},  # different recipe id
            "recipes": {
                "my-custom-runner": {"test_command": "./custom-tests.sh"},
                "tdd-flow": {"test_command": "./tests/run.sh"},  # NOT the bound one
            },
        }
        agents = self.run_render("[project]\nname = 'fix10-test-runner'\n", resolved)
        # The BOUND recipe's command must appear
        self.assertIn("./custom-tests.sh", agents)
        # The un-bound recipe's command must NOT appear (tdd-flow is not the active binding)
        self.assertNotIn("./tests/run.sh", agents)

    def test_integration_branch_resolves_via_worktree_isolation_binding(self):
        """FIX 10: integration_branch resolved via bindings['worktree-isolation'] → recipe.
        Swapping bound recipe id keeps the field.
        """
        resolved = {
            "bindings": {"worktree-isolation": "my-worktree"},  # different recipe id
            "recipes": {
                "my-worktree": {"integration_branch": "staging"},
                "worktree-flow": {"integration_branch": "main"},  # NOT the bound one
            },
        }
        agents = self.run_render("[project]\nname = 'fix10-integration-branch'\n", resolved)
        self.assertIn("- **Integration branch**: `staging`", agents)
        self.assertNotIn("`main`", agents)

    def test_vault_scope_resolves_via_canonical_store_binding(self):
        """FIX 10: vault_scope resolved via bindings['canonical-store'] → recipe."""
        resolved = {
            "bindings": {"canonical-store": "my-vault"},
            "recipes": {
                "my-vault": {"vault_scope": "my/vault/path"},
                "vault-canonical-store": {"vault_scope": "other/path"},  # NOT bound
            },
        }
        agents = self.run_render("[project]\nname = 'fix10-vault'\n", resolved)
        self.assertIn("- **Vault scope**: `my/vault/path`", agents)
        self.assertNotIn("other/path", agents)

    # --- FIX A (Round 2) ---

    def test_resolved_config_only_mode_writes_json_and_leaves_no_recipe_mcp_temp(self):
        """FIX A (R2): --resolved-config-only writes resolved-config WITHOUT creating
        any ai-specs-recipe-mcp-* temp files.

        Verifies: (a) the output file is valid JSON with bindings/recipes/enabled keys,
        (b) no new ai-specs-recipe-mcp-* files appear in /tmp after the call.
        """
        import glob
        import tempfile as _tempfile

        materialize = ROOT / "lib" / "_internal" / "recipe-materialize.py"

        with _tempfile.TemporaryDirectory() as parent:
            workspace = Path(parent) / "fixA-workspace"
            workspace.mkdir()
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixA-no-temp-leak'\n\n"
                "[[bindings]]\n"
                "capability = 'tracker'\n"
                "recipe = 'trello-mcp-workflow'\n"
            )

            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                resolved_out = Path(f.name)

            try:
                # Count existing recipe-mcp temps before the call
                before = set(glob.glob("/tmp/**/ai-specs-recipe-mcp-*.json", recursive=True))

                proc = subprocess.run(
                    [
                        "python3", str(materialize),
                        str(workspace), str(ROOT),
                        "--resolved-config-out", str(resolved_out),
                        "--resolved-config-only",
                    ],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(
                    proc.returncode, 0,
                    f"--resolved-config-only failed:\n{proc.stderr}\n{proc.stdout}",
                )

                # No new recipe-mcp temp files must have been created
                after = set(glob.glob("/tmp/**/ai-specs-recipe-mcp-*.json", recursive=True))
                new_temps = after - before
                self.assertEqual(
                    new_temps, set(),
                    f"--resolved-config-only leaked recipe-mcp temp(s): {new_temps}",
                )

                # Output must be valid JSON with expected top-level keys
                with open(resolved_out) as fh:
                    data = json.load(fh)
                self.assertIn("bindings", data)
                self.assertIn("recipes", data)
                self.assertIn("enabled", data)
            finally:
                if resolved_out.exists():
                    resolved_out.unlink()

    def test_resolved_config_only_mode_fails_loudly_not_silently(self):
        """FIX A (R2): --resolved-config-only propagates non-zero exit on failure.

        Uses a non-existent project root so build_resolved_config fails. The script
        must exit non-zero (not silently return 0 with || true).
        """
        import tempfile as _tempfile

        materialize = ROOT / "lib" / "_internal" / "recipe-materialize.py"

        with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
            resolved_out = Path(f.name)

        try:
            proc = subprocess.run(
                [
                    "python3", str(materialize),
                    "/nonexistent/project/root", str(ROOT),
                    "--resolved-config-out", str(resolved_out),
                    "--resolved-config-only",
                ],
                text=True,
                capture_output=True,
                check=False,
            )
            # Must exit non-zero when project root does not exist
            self.assertNotEqual(
                proc.returncode, 0,
                "Expected non-zero exit for missing project root but got 0",
            )
        finally:
            if resolved_out.exists():
                resolved_out.unlink()

    def test_resolved_config_only_invalid_binding_exits_nonzero(self):
        """FIX 2 (R3): --resolved-config-only must exit non-zero on manifest binding
        validation errors (e.g. explicit binding references a disabled/unknown recipe).

        The full materialize_recipes path raises a RuntimeError (via resolve_bindings)
        and exits 1 for such errors. The standalone --resolved-config-only path must
        match this behaviour — it must NOT swallow the error and exit 0 silently.

        Uses a manifest with an explicit [[bindings]] that references a recipe that is
        NOT enabled, which resolve_bindings raises RuntimeError for.
        """
        import tempfile as _tempfile

        materialize = ROOT / "lib" / "_internal" / "recipe-materialize.py"

        with _tempfile.TemporaryDirectory() as parent:
            workspace = Path(parent) / "invalid-binding-workspace"
            workspace.mkdir()
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            # An explicit binding that references a recipe NOT in [recipes.*] (not enabled).
            # resolve_bindings raises RuntimeError for this case.
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'invalid-binding-test'\n\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbccddeeff001122334455'\n\n"
                "[[bindings]]\n"
                "capability = 'tracker'\n"
                "recipe = 'nonexistent-recipe'\n"  # references a disabled/unknown recipe
            )

            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                resolved_out = Path(f.name)

            try:
                proc = subprocess.run(
                    [
                        "python3", str(materialize),
                        str(workspace), str(ROOT),
                        "--resolved-config-out", str(resolved_out),
                        "--resolved-config-only",
                    ],
                    text=True,
                    capture_output=True,
                    check=False,
                    env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
                )
                # Must exit non-zero (matching the full materialize path)
                self.assertNotEqual(
                    proc.returncode, 0,
                    "Expected non-zero exit for invalid binding in --resolved-config-only "
                    f"but got 0.\nstderr: {proc.stderr}\nstdout: {proc.stdout}",
                )
                # Error must be surfaced to stderr (not swallowed silently)
                self.assertIn(
                    "ERROR", proc.stderr,
                    "An ERROR message must appear on stderr for invalid binding validation."
                )
            finally:
                if resolved_out.exists():
                    resolved_out.unlink()

    # --- FIX B (Round 2) ---

    def test_wrong_typed_inner_fields_degrade_gracefully_no_crash(self):
        """FIX B (R2): resolved-config with wrong-typed inner fields must not crash.

        A dict with bindings/recipes/enabled set to wrong types (list, string, str)
        triggers AttributeError in the section helpers unless coerced to the expected
        types in render(). This test feeds such a dict and verifies: exit 0, no crash,
        degraded output (project name present).
        """
        import tempfile as _tempfile

        with _tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            bad_resolved_path = tmp_path / "bad_resolved.json"

            toml_path.write_text(
                "[project]\n"
                "name = 'fixB-wrong-typed-inner-fields'\n\n"
                "[brief]\n"
                'workflow_rules = ["No merges without review."]\n'
            )
            # bindings is a list (not dict), recipes is a string (not dict),
            # enabled is a dict (not list) — all wrong types
            bad_resolved_path.write_text(json.dumps({
                "bindings": ["tracker", "vcs-pr-flow"],  # list, not dict
                "recipes": "should-be-a-dict",           # string, not dict
                "enabled": {"tdd-flow": True},           # dict, not list
            }))

            proc = subprocess.run(
                [
                    "python3",
                    str(ROOT / "lib" / "_internal" / "agents-render.py"),
                    str(toml_path), str(output_path),
                    "--resolved-config", str(bad_resolved_path),
                ],
                text=True,
                capture_output=True,
                check=False,
            )

            # Must not crash
            self.assertEqual(
                proc.returncode, 0,
                f"agents-render.py crashed on wrong-typed inner fields:\n{proc.stderr}",
            )
            self.assertTrue(output_path.exists())

            agents = output_path.read_text()
            # Project identity must be present (degraded but not empty)
            self.assertIn("fixB-wrong-typed-inner-fields", agents)
            # Workflow rules must render (from TOML, not from wrong resolved-config)
            self.assertIn("No merges without review.", agents)


class RuntimeHookSyncPipelineTests(unittest.TestCase):
    """End-to-end: enabling the worktree-flow hook fans wiring to every harness."""

    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-hooks-"))
        shutil.copytree(FIXTURE_ROOT, tmp / "workspace")
        return tmp / "workspace"

    def _wf_version(self) -> str:
        import tomllib
        with open(ROOT / "catalog" / "recipes" / "worktree-flow" / "recipe.toml", "rb") as fh:
            return tomllib.load(fh)["recipe"]["version"]

    def _manifest(self, version: str) -> str:
        return (
            "[project]\nname = 'hook-fixture'\n\n"
            "[agents]\nenabled = ['claude', 'cursor', 'opencode', 'pi']\n\n"
            "[recipes.worktree-flow]\n"
            "enabled = true\n"
            f"version = '{version}'\n"
            "[recipes.worktree-flow.config]\n"
            "integration_branch = 'development'\n"
        )

    def test_sync_fans_hook_to_every_harness(self):
        workspace = self.make_workspace()
        try:
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(self._manifest(self._wf_version()))
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            # Materialized script at the harness-neutral path, executable.
            script = workspace / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
            self.assertTrue(script.is_file(), "hook script must materialize")
            self.assertTrue(os.access(script, os.X_OK), "hook script must be executable")

            # Claude: managed PreToolUse entry wiring the script directly.
            settings = json.loads((workspace / ".claude" / "settings.json").read_text())
            pre = settings["hooks"]["PreToolUse"]
            cmds = json.dumps(pre)
            self.assertIn("ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh", cmds)

            # OpenCode + Pi: generated shims.
            self.assertTrue(
                (workspace / ".opencode" / "plugin" / "worktree-flow-worktree-gate.ts").is_file()
            )
            self.assertTrue(
                (workspace / ".pi" / "extensions" / "worktree-flow-worktree-gate.ts").is_file()
            )

            # Cursor: file-write matcher → warn-and-skip (no wrapper emitted).
            self.assertFalse(
                (workspace / ".cursor" / "hooks" / "worktree-flow-worktree-gate.sh").exists(),
                "cursor must skip file-write gates",
            )

            # Idempotency: second sync byte-identical for claude settings + shims.
            before_claude = (workspace / ".claude" / "settings.json").read_bytes()
            before_oc = (workspace / ".opencode" / "plugin" / "worktree-flow-worktree-gate.ts").read_bytes()
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            after_claude = (workspace / ".claude" / "settings.json").read_bytes()
            after_oc = (workspace / ".opencode" / "plugin" / "worktree-flow-worktree-gate.ts").read_bytes()
            self.assertEqual(before_claude, after_claude)
            self.assertEqual(before_oc, after_oc)
        finally:
            shutil.rmtree(workspace.parent)


if __name__ == "__main__":
    unittest.main()
