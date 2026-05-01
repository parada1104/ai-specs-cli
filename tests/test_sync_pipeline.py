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
            lines.extend(f"    - \"{entry}\"" for entry in scope)
        if auto_invoke:
            lines.append("  auto_invoke:")
            lines.extend(f"    - \"{entry}\"" for entry in auto_invoke)
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
                "  version: \"1.0\"\n"
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

            root_skill = workspace / ".deps" / "vendored-demo" / "skills" / "vendored-demo" / "SKILL.md"
            subrepo_skill = workspace / "packages" / "a" / "ai-specs" / "skills" / "vendored-demo" / "SKILL.md"
            content = root_skill.read_text()

            self.assertTrue(root_skill.is_file())
            self.assertEqual(root_skill.read_bytes(), subrepo_skill.read_bytes())
            self.assertIn('author: "fixture-org"', content)
            self.assertIn('version: "1.0"', content)
            self.assertIn(f'source: "{dep_repo}"', content)
            self.assertIn('vendor_attribution: "fixture-org"', content)
            self.assertIn('auto_invoke:', content)
            self.assertIn("`vendored-demo`", (workspace / "ai-specs" / ".skill-registry.md").read_text())
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

            root_skill = workspace / ".deps" / "vendored-demo" / "skills" / "vendored-demo" / "SKILL.md"
            root_skill.write_text(
                "---\n"
                "name: vendored-demo\n"
                "description: Manual tamper.\n"
                "license: GPL-3.0\n"
                "metadata:\n"
                "  author: manual-edit\n"
                "  version: \"9.9\"\n"
                "  scope: [root]\n"
                "  auto_invoke:\n"
                "    - \"Manual trigger\"\n"
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
            registry = (workspace / "ai-specs" / ".skill-registry.md").read_text()

            self.assertIn("name: local-demo", content)
            self.assertIn("license: Apache-2.0", content)
            self.assertIn("author: fixture-suite", content)
            self.assertIn('version: "1.0"', content)
            self.assertIn('scope:\n    - "root"', content)
            self.assertIn('auto_invoke:\n    - "Syncing root workspace"', content)
            self.assertIn("| Syncing root workspace | `local-demo` | `root` |", registry)
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
            agents = (workspace / "AGENTS.md").read_text()
            registry = (workspace / "ai-specs" / ".skill-registry.md").read_text()

            self.assertIn("name: local-docs", content)
            self.assertIn("license: Apache-2.0", content)
            self.assertIn("author: fixture-suite", content)
            self.assertIn('version: "1.0"', content)
            self.assertNotIn("scope:", content)
            self.assertNotIn("auto_invoke:", content)
            self.assertNotIn("`local-docs`", agents)
            self.assertIn("`local-docs`", registry)
            auto_invoke_section = registry.split("## Auto-invoke Mappings", 1)[1]
            self.assertNotIn("`local-docs`", auto_invoke_section)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_fails_with_actionable_contract_error_for_invalid_skill_metadata(self):
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
                "  version: \"1.0\"\n"
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

            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("metadata.auto_invoke", proc.stderr)
            self.assertIn("bad-sync", proc.stderr)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_fails_on_auto_invoke_without_scope(self):
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

            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("metadata.scope", proc.stderr)
            self.assertIn("bad-scope", proc.stderr)
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

    def test_sync_produces_identical_registry_on_second_run(self):
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            first = (workspace / "ai-specs" / ".skill-registry.md").read_bytes()
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)
            second = (workspace / "ai-specs" / ".skill-registry.md").read_bytes()
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
            self.assertTrue((workspace / "ai-specs" / ".skill-registry.md").is_file())
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

    def test_registry_artifact_indexes_all_skill_sources(self):
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

            # Recipe skill
            recipe_dir = workspace / ".recipe" / "test-recipe" / "skills" / "recipe-skill"
            recipe_dir.mkdir(parents=True)
            (recipe_dir / "SKILL.md").write_text(
                "---\n"
                "name: recipe-skill\n"
                "description: A recipe skill.\n"
                "license: MIT\n"
                "metadata:\n"
                "  author: recipe-author\n"
                "  version: \"1.0\"\n"
                "  scope:\n"
                "    - \"root\"\n"
                "  auto_invoke:\n"
                "    - \"Do recipe thing\"\n"
                "---\n\n"
                "# Recipe Skill\n"
            )

            # Dep skill
            dep_dir = workspace / ".deps" / "test-dep" / "skills" / "dep-skill"
            dep_dir.mkdir(parents=True)
            (dep_dir / "SKILL.md").write_text(
                "---\n"
                "name: dep-skill\n"
                "description: A dep skill.\n"
                "license: MIT\n"
                "metadata:\n"
                "  author: dep-author\n"
                "  version: \"1.0\"\n"
                "  scope:\n"
                "    - \"root\"\n"
                "  auto_invoke:\n"
                "    - \"Do dep thing\"\n"
                "---\n\n"
                "# Dep Skill\n"
            )

            subprocess.run(
                [
                    "python3",
                    str(ROOT / "lib" / "_internal" / "registry-render.py"),
                    str(workspace),
                ],
                check=True,
                text=True,
            )

            registry = (workspace / "ai-specs" / ".skill-registry.md").read_text()

            self.assertIn("| `local-skill` | local |", registry)
            self.assertIn("| `recipe-skill` | recipe |", registry)
            self.assertIn("| `dep-skill` | dep |", registry)

            self.assertIn("| Do local thing | `local-skill` | `root` |", registry)
            self.assertIn("| Do recipe thing | `recipe-skill` | `root` |", registry)
            self.assertIn("| Do dep thing | `dep-skill` | `root` |", registry)
        finally:
            shutil.rmtree(workspace.parent)


class SkillSyncScriptTests(unittest.TestCase):
    SCRIPT = ROOT / "ai-specs" / "skills" / "skill-sync" / "assets" / "sync.sh"

    def test_skill_sync_filters_scopes_and_derives_one_agents_row_per_trigger(self):
        repo_root = Path(tempfile.mkdtemp(prefix="ai-specs-skill-sync-"))
        try:
            script_path = repo_root / "ai-specs" / "skills" / "skill-sync" / "assets" / "sync.sh"
            script_path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(self.SCRIPT, script_path)
            (repo_root / ".melon-monorepo").write_text("1\n")
            (repo_root / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'test'\n")

            for agents_path in [
                repo_root / "AGENTS.md",
                repo_root / "alquimia-front-web" / "AGENTS.md",
                repo_root / "alquimia-back-web" / "AGENTS.md",
            ]:
                agents_path.parent.mkdir(parents=True, exist_ok=True)
                agents_path.write_text(
                    "# Test Agents\n\n"
                    "> [SKILL.md](ai-specs/skills/demo/SKILL.md)\n\n"
                    "### Auto-invoke Skills\n\n"
                    "When performing these actions, ALWAYS invoke the corresponding skill FIRST:\n\n"
                    "| Action | Skill |\n"
                    "|--------|-------|\n\n"
                    "## Footer\n"
                )

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
                "  version: \"1.0\"\n"
                "  scope:\n"
                "    - \"root\"\n"
                "    - \"front_web\"\n"
                "  auto_invoke:\n"
                "    - \"Do root thing\"\n"
                "    - \"Do shared thing\"\n"
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
                "  version: \"1.0\"\n"
                "  scope:\n"
                "    - \"back_web\"\n"
                "  auto_invoke:\n"
                "    - \"Do back thing\"\n"
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
                "  version: \"1.0\"\n"
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

            registry = (repo_root / "ai-specs" / ".skill-registry.md").read_text()

            # Skill Index
            self.assertIn("| `root-auto` | local |", registry)
            self.assertIn("| `back-auto` | local |", registry)
            self.assertIn("| `manual-only` | local |", registry)

            # Auto-invoke Mappings
            self.assertIn("| Do root thing | `root-auto` | `root` |", registry)
            self.assertIn("| Do shared thing | `root-auto` | `root` |", registry)
            self.assertIn("| Do root thing | `root-auto` | `front_web` |", registry)
            self.assertIn("| Do shared thing | `root-auto` | `front_web` |", registry)
            self.assertIn("| Do back thing | `back-auto` | `back_web` |", registry)

            auto_invoke_section = registry.split("## Auto-invoke Mappings", 1)[1]
            self.assertNotIn("manual-only", auto_invoke_section)
        finally:
            shutil.rmtree(repo_root)


if __name__ == "__main__":
    unittest.main()
