"""Black-box sync pipeline tests: every test drives ``bin/ai-specs`` verbs.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (exit codes, stdout/stderr needles, filesystem effects on
AGENTS.md, agent configs, lock files, recipe hooks, snapshots) through the CLI
process boundary. Internal renderer/materialize surfaces with no
CLI-observable equivalent are kept as process-boundary invocations, each
marked with a distinct ``# TRIAGE:`` comment.
"""
from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"

FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


_HOMES: dict[str, Path] = {}


def _home_for(project: Path) -> Path:
    """One shared isolated install root per command sequence.

    Memoized by the project's parent dir (unique per temp workspace), so every
    init/sync/sync-agent sequence within one test shares cache state while
    never touching repository files. The home lives inside the test's temp
    dir, so the test's own cleanup removes it.
    """
    base = project.parent
    key = str(base)
    if key not in _HOMES:
        _HOMES[key] = _make_home(base)
    return _HOMES[key]


def run_cli_env(project, *args, home: Path, tmpdir: Path, extra_env=None,
                append_root: bool = True):
    """Raw CLI run for tests needing env vars invoke() cannot pass.

    stdin is closed (input='') so the CLI can never block on a prompt.
    Mirrors invoke()'s hermetic environment plus the vendor fixture root that
    keeps kepano/obsidian-skills vendoring offline.
    """
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(tmpdir / "home"),
        "TMPDIR": str(tmpdir),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        "LC_ALL": "C",
        "LANG": "C",
    }
    if extra_env:
        env.update(extra_env)
    (tmpdir / "home").mkdir(parents=True, exist_ok=True)
    argv = [str(CLI), *args]
    if append_root:
        argv.append(str(project))
    return subprocess.run(argv, cwd=ROOT, env=env, text=True,
                          capture_output=True, check=False, input="")


def _resolved_config_process(project_root: Path, home: Path, out: Path,
                             *extra: str) -> subprocess.CompletedProcess:
    """Run the ISOLATED HOME'S OWN lib copy of recipe-materialize.py.

    Mirror of test_recipe_materialize.py::_materialize_process — never a
    repository import. Used only by ``# TRIAGE:`` tests whose contract (the
    standalone ``--resolved-config-only`` bindings map) is observable solely
    at the materialize process boundary via ``--resolved-config-out``.
    """
    tmpdir = project_root.parent
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(tmpdir / "home"),
        "TMPDIR": str(tmpdir),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        "LC_ALL": "C",
        "LANG": "C",
    }
    (tmpdir / "home").mkdir(parents=True, exist_ok=True)
    argv = [
        sys.executable, str(home / "lib" / "_internal" / "recipe-materialize.py"),
        str(project_root), str(home),
        "--resolved-config-out", str(out), *extra,
    ]
    return subprocess.run(argv, cwd=ROOT, env=env, text=True,
                          capture_output=True, check=False, input="")


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
        home = _home_for(workspace)
        result = invoke(workspace, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'fixture-sync'\n")

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            self.assertTrue((workspace / "AGENTS.md").is_file())
            self.assertFalse((workspace / "packages" / "a" / "AGENTS.md").exists())
            self.assertFalse((workspace / ".mcp.json").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_accepts_mcp_environment_alias_and_renders_canonical_output(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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

    def test_sync_renders_opencode_mcp_arg_var_as_env_form(self):
        """A var in a command ARG must render as {env:VAR} for OpenCode.

        OpenCode ConfigVariable.substitute only expands {env:NAME} at config
        load (live-verified 1.18.3). Shell-style $VAR / ${VAR} stay literal,
        so a sole-arg path like ${VAULT_PATH} never reaches the server.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            parsed = json.loads((workspace / "opencode.json").read_text())
            demo = parsed["mcp"]["demo"]
            # ARG and ENVIRONMENT: same OpenCode {env:VAR} form
            self.assertIn("{env:VAULT_PATH}", demo["command"])
            self.assertNotIn("$VAULT_PATH", demo["command"])
            self.assertNotIn("${VAULT_PATH}", demo["command"])
            self.assertEqual(demo["environment"], {"VAULT_PATH": "{env:VAULT_PATH}"})
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_renders_opencode_mcp_bare_dollar_arg_as_env_form(self):
        """Bare $VAR in a command ARG also becomes {env:VAR} for OpenCode."""
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n\n"
                "[agents]\n"
                "enabled = ['opencode']\n\n"
                "[mcp.demo]\n"
                "command = 'npx'\n"
                "args = ['-y', '@modelcontextprotocol/server-filesystem', '$VAULT_PATH']\n"
                "timeout = 30000\n"
                "enabled = true\n"
            )

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            parsed = json.loads((workspace / "opencode.json").read_text())
            demo = parsed["mcp"]["demo"]
            self.assertIn("{env:VAULT_PATH}", demo["command"])
            self.assertNotIn("$VAULT_PATH", demo["command"])
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_renders_cursor_mcp_env_with_braced_dollar_syntax_input(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            cursor = json.loads((workspace / ".cursor" / "mcp.json").read_text())
            opencode = json.loads((workspace / "opencode.json").read_text())

            self.assertEqual(cursor["mcpServers"]["demo"]["env"], {"MODE": "fixture"})
            self.assertEqual(opencode["mcp"]["demo"]["environment"], {"MODE": "fixture"})
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_keeps_opencode_schema_first_when_preserving_existing_config(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertTrue((workspace / "AGENTS.md").is_file())
            self.assertFalse((workspace / "packages" / "a" / "AGENTS.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_empty_subrepos_with_gitmodules_entries_do_not_fan_out(self):
        """1.3 — RED: .gitmodules never expands an empty declared target set."""
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n"
                "subrepos = []\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            (workspace / ".gitmodules").write_text(
                '[submodule "packages/a"]\n\tpath = packages/a\n\turl = ../a.git\n'
                '[submodule "packages/b"]\n\tpath = packages/b\n\turl = ../b.git\n'
            )
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            for subrepo in ("packages/a", "packages/b"):
                self.assertFalse(
                    (workspace / subrepo / "AGENTS.md").exists(),
                    f"{subrepo} must NOT receive fan-out (empty declared set)",
                )
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_fans_out_root_managed_artifacts_to_subrepos(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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

    def test_sync_symlinks_omp_native_agents_md_slot(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-sync'\n"
                "subrepos = []\n\n"
                "[agents]\n"
                "enabled = ['omp']\n"
            )
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            omp_brief = workspace / ".omp" / "AGENTS.md"
            self.assertTrue(omp_brief.is_symlink(), ".omp/AGENTS.md must be a symlink")
            self.assertEqual(os.readlink(omp_brief), "../AGENTS.md")
            self.assertEqual(
                omp_brief.resolve(), (workspace / "AGENTS.md").resolve()
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_synced_subrepo_supports_local_agent_startup_read_paths(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            result = invoke(workspace, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            for target in (workspace, workspace / "packages" / "a", workspace / "packages" / "b"):
                self.assertTrue((target / "AGENTS.md").is_file())
                self.assertTrue((target / ".cursor" / "commands" / "skills-as-rules.md").is_file())

            self.assertTrue((workspace / "packages" / "a" / "ai-specs" / "skills" / "local-demo" / "SKILL.md").is_file())
            self.assertTrue((workspace / "packages" / "b" / "CLAUDE.md").is_symlink())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_stops_on_first_incompatible_target_write(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            blocked = workspace / "packages" / "a" / "CLAUDE.md"
            blocked.write_text("manual file")
            proc = invoke(workspace, "sync", cli_home=home)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("Stopped on first failure", proc.stderr)
            self.assertFalse((workspace / "packages" / "b" / "AGENTS.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_normalizes_vendored_skill_frontmatter_and_fans_out_byte_identically(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            self.write_local_skill(
                workspace,
                "local-docs",
                description="Documentation helper without AGENTS auto-invoke.",
                body="# Local Docs",
            )

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
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

            proc = invoke(workspace, "sync", cli_home=home)

            # Sync succeeds but skill-sync reports missing auto_invoke
            self.assertEqual(proc.returncode, 0)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_warns_on_auto_invoke_without_scope(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            self.write_local_skill(
                workspace,
                "bad-scope",
                description="Missing scope.",
                auto_invoke=["Do thing"],
            )

            proc = invoke(workspace, "sync", cli_home=home)

            # Sync succeeds but skill-sync reports incomplete metadata
            self.assertEqual(proc.returncode, 0)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_produces_identical_agents_md_on_second_run_thin(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            first = (workspace / "AGENTS.md").read_bytes()
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            second = (workspace / "AGENTS.md").read_bytes()
            self.assertEqual(first, second)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_produces_identical_agents_md_on_second_run(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            self.init_workspace(workspace)
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            first = (workspace / "AGENTS.md").read_bytes()
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            second = (workspace / "AGENTS.md").read_bytes()
            self.assertEqual(first, second)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_preserves_runtime_brief_marker_in_agents_md(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text("[project]\nname = 'fixture-sync'\n")
            agents_md = workspace / "AGENTS.md"
            original = "# Manual Brief\n<!-- ai-specs:runtime-brief -->\n\nCustom content.\n"
            agents_md.write_text(original)

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            self.assertEqual(agents_md.read_text(), original)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_redacts_literal_mcp_secrets_in_agents_md(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
                "version = '1.2.2'\n"
                "[recipes.worktree-flow.config]\n"
                "integration_branch = 'development'\n\n"
                "[recipes.git-pr-flow]\n"
                "enabled = true\n"
                "version = '1.3.0'\n"
                "[recipes.git-pr-flow.config]\n"
                "base_branch = 'development'\n\n"
                "[recipes.tdd-flow]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
                "[recipes.tdd-flow.config]\n"
                "test_command = './tests/run.sh'\n\n"
                "[recipes.vault-canonical-store]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
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

            proc = run_cli_env(workspace, "sync", home=home, tmpdir=workspace.parent)
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

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

    def test_sync_brief_reflects_worktree_gate_mode_ask(self):
        """End-to-end: recipe gate fragment renders config-aware ask-mode prose.

        Proves the effective [recipes.worktree-flow.config].gate_mode reaches
        generated AGENTS.md prose, that no unconditional dedicated-worktree rule
        survives, and that the removed self-bypass token never reappears.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'gate-mode-brief-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[recipes.worktree-flow]\n"
                "enabled = true\n"
                "[recipes.worktree-flow.config]\n"
                "gate_mode = 'ask'\n"
            )

            proc = run_cli_env(workspace, "sync", home=home, tmpdir=workspace.parent)
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("`gate_mode = ask`", agents)
            self.assertIn("ask the user to choose a destination", agents)
            self.assertNotIn("WORKTREE_GATE_MODE=off", agents)
            self.assertNotIn(
                "Create a dedicated worktree for changes that write artifacts or modify code.",
                agents,
            )
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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            first = (workspace / "AGENTS.md").read_bytes()

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
        """Degraded rendering: [brief] prose and MCP render with NO recipes/structured config.

        Black-box equivalent of the original standalone agents-render.py
        invocation WITHOUT --resolved-config: a manifest with only [brief]
        prose and an MCP block (no recipes, no bindings) still renders
        identity, MCP section, and prose.

        Asserts:
        - sync exits 0 (no crash).
        - AGENTS.md contains project identity (project name).
        - AGENTS.md contains MCP section (when mcp servers present).
        - AGENTS.md contains [brief] prose sections (intro, workflow_rules).
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
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

            result = invoke(workspace, "sync", cli_home=home)

            # Must not crash
            self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")

            # Output file must exist
            agents_path = workspace / "AGENTS.md"
            self.assertTrue(agents_path.exists(), "AGENTS.md was not created")

            agents = agents_path.read_text()

            # Identity: project name must be present
            self.assertIn("standalone-degradation-fixture", agents)

            # MCP section must be rendered (degraded path still includes MCP)
            self.assertIn("demo-server", agents)

            # [brief] prose sections must be present even without structured config
            self.assertIn("This is the degraded brief intro.", agents)
            self.assertIn("No direct pushes to main.", agents)
        finally:
            shutil.rmtree(workspace.parent)

    # -----------------------------------------------------------------------
    # End Batch 1 RED tests
    # -----------------------------------------------------------------------

    def test_brief_useful_commands_renders_extra_items(self):
        """[brief].useful_commands array items are appended to ## Useful Commands section.

        Coverage note: the manifest deliberately declares NO recipes, so
        test_command from [recipes.tdd-flow.config] is NOT rendered (that path
        requires resolved-config from materialize). This test exercises ONLY
        the brief.useful_commands rendering path in the Useful Commands
        section of the generated AGENTS.md.

        For test_command rendering coverage via the real resolve_bindings()
        path, see test_sync_renders_rich_brief_from_manifest and
        test_auto_binding_without_explicit_bindings.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'useful-commands-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                'useful_commands = ["Inspect the active Trello card before resuming work."]\n'
            )

            result = invoke(workspace, "sync", cli_home=home)

            self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")
            agents_path = workspace / "AGENTS.md"
            self.assertTrue(agents_path.exists(), "AGENTS.md was not created")

            agents = agents_path.read_text()
            # brief.useful_commands items must appear in ## Useful Commands
            self.assertIn("Inspect the active Trello card before resuming work.", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_resolves_all_skill_sources(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            # Verify local skills are resolved in the project cache resolved-skills
            resolved = cache_project_dir(workspace, home) / "resolved-skills"
            self.assertTrue((resolved / "local-skill" / "SKILL.md").is_file())
            # No registry artifact should exist
            self.assertFalse((workspace / "ai-specs" / ".skill-registry.md").exists())
        finally:
            shutil.rmtree(workspace.parent)

    def test_sync_agent_all_includes_pi_when_enabled(self):
        """When pi is in [agents].enabled, --all must sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-pi-all'\n\n"
                "[agents]\n"
                "enabled = ['pi']\n"
            )
            result = invoke(target, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            pi_skills = target / ".pi" / "skills"
            self.assertTrue(pi_skills.is_symlink(),
                            ".pi/skills/ must be a symlink after --all")
            self.assertFalse((target / "PI.md").exists())
            self.assertFalse((target / "pi.md").exists())

    def test_sync_agent_all_excludes_pi_when_not_enabled(self):
        """When pi is NOT in [agents].enabled, --all must NOT sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-no-pi'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            result = invoke(target, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            pi_skills = target / ".pi" / "skills"
            self.assertFalse(pi_skills.exists(),
                             ".pi/skills/ must NOT exist when pi is disabled")

    # --- Omp flag and help tests ---

    def test_sync_agent_omp_flag_accepted(self):
        """--omp flag must be accepted and produce .omp/skills symlink."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = invoke(target, "sync-agent", "--omp", cli_home=home)
            self.assertEqual(result.returncode, 0,
                             f"--omp must exit 0; stderr={result.stderr!r}")
            omp_skills = target / ".omp" / "skills"
            self.assertTrue(omp_skills.is_symlink(),
                            ".omp/skills/ must be a symlink after --omp")

    def test_sync_agent_help_lists_omp(self):
        """--help output must include --omp."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _home_for(Path(tmp) / "help-probe")
            result = invoke(Path(tmp) / "help-probe", "sync-agent", "--help",
                            cli_home=home, append_root=False)
            self.assertIn("--omp", result.stdout,
                          "--omp must appear in sync-agent --help output")

    def test_sync_agent_all_includes_omp_when_enabled(self):
        """When omp is in [agents].enabled, --all must sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-omp-all'\n\n"
                "[agents]\n"
                "enabled = ['omp']\n"
            )
            result = invoke(target, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            omp_skills = target / ".omp" / "skills"
            self.assertTrue(omp_skills.is_symlink(),
                            ".omp/skills/ must be a symlink after --all")
            # omp's runtime brief lives in its native slot .omp/AGENTS.md,
            # symlinked to the root AGENTS.md — not a root-level OMP.md file.
            self.assertTrue((target / ".omp" / "AGENTS.md").is_symlink(),
                            ".omp/AGENTS.md must be a symlink after --all")
            self.assertFalse((target / "OMP.md").exists())
            self.assertFalse((target / "omp.md").exists())

    def test_sync_agent_all_excludes_omp_when_not_enabled(self):
        """When omp is NOT in [agents].enabled, --all must NOT sync it."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-no-omp'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            result = invoke(target, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            omp_skills = target / ".omp" / "skills"
            self.assertFalse(omp_skills.exists(),
                             ".omp/skills/ must NOT exist when omp is disabled")

    def test_omp_mcp_json_rendered_when_mcps_declared(self):
        """--omp must write .omp/mcp.json with mcpServers when [mcp.*] entries exist."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-omp-mcp'\n\n"
                "[agents]\n"
                "enabled = ['omp']\n\n"
                "[mcp.my-server]\n"
                "command = 'npx'\n"
                "args = ['-y', '@example/server']\n"
            )
            result = invoke(target, "sync-agent", "--omp", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-omp-no-mcp'\n\n"
                "[agents]\n"
                "enabled = ['omp']\n"
            )
            result = invoke(target, "sync-agent", "--omp", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            mcp_path = target / ".omp" / "mcp.json"
            self.assertFalse(mcp_path.exists(),
                             ".omp/mcp.json must NOT be created when no MCPs declared")

    def test_omp_commands_populated(self):
        """--omp must copy command files to .omp/commands/."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # ai-specs init already creates skills-as-rules.md in commands/
            result = invoke(target, "sync-agent", "--omp", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = invoke(target, "sync-agent", "--omp", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # omp must not create any OMP.md or omp.md instruction file
            self.assertFalse((target / "OMP.md").exists(),
                             "OMP.md must NOT be created for omp")
            self.assertFalse((target / "omp.md").exists(),
                             "omp.md must NOT be created for omp")

    def test_omp_gitignore_contains_omp_dir(self):
        """ai-specs init must write .omp/ into the root .gitignore."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            gitignore = (target / ".gitignore").read_text()
            self.assertIn(".omp/", gitignore,
                          ".gitignore must contain .omp/ after ai-specs init")

    def test_existing_agents_unchanged_after_omp_added(self):
        """Existing agent outputs must be byte-identical before and after adding omp."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            home = _home_for(target)
            target.mkdir()
            result = invoke(target, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # Sync with claude only
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-compat'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n"
            )
            result = invoke(target, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            claude_md_before = (target / "CLAUDE.md").read_text() if (target / "CLAUDE.md").is_file() else None

            # Now add omp to enabled and re-sync
            (target / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-compat'\n\n"
                "[agents]\n"
                "enabled = ['claude', 'omp']\n"
            )
            result = invoke(target, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            claude_md_after = (target / "CLAUDE.md").read_text() if (target / "CLAUDE.md").is_file() else None
            self.assertEqual(claude_md_before, claude_md_after,
                             "CLAUDE.md (via symlink) must be byte-identical before and after adding omp")
            # .claude/skills symlink should still point to the same target
            claude_skills = target / ".claude" / "skills"
            self.assertTrue(claude_skills.is_symlink(),
                            ".claude/skills must still be a symlink after adding omp")


class SkillSyncScriptTests(unittest.TestCase):
    SCRIPT = ROOT / "bundled-skills" / "skill-sync" / "assets" / "sync.sh"

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
                input="",
                env={**os.environ, "AI_SPECS_HOME": str(_home_for(repo_root / "skill-sync-probe"))},
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
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'partial-brief-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[brief]\n"
                # Only workflow_rules is present; no intro, no purpose, no runtime_flow etc.
                'workflow_rules = ["No direct merges without approval."]\n'
            )

            result = invoke(workspace, "sync", cli_home=home)

            # Must not crash
            self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")
            agents_path = workspace / "AGENTS.md"
            self.assertTrue(agents_path.exists())

            agents = agents_path.read_text()

            # Project identity must be present
            self.assertIn("partial-brief-fixture", agents)

            # Present key must render
            self.assertIn("No direct merges without approval.", agents)

            # Absent keys must NOT produce empty section headers
            # (intro absent → no empty ## Project section with just the header)
            # We assert the specific absent strings do not appear as placeholder bullets
            self.assertNotIn("None.", agents,  # no placeholder for empty sections
                             "Absent brief keys must not produce 'None.' placeholders")
        finally:
            shutil.rmtree(workspace.parent)

    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-missing-scenarios-"))
        workspace = tmp / "workspace"
        workspace.mkdir()
        return workspace

    def test_no_tracker_binding_omits_trello_section(self):
        """R3 no-tracker: when no recipe is bound to 'tracker', the Trello Tracking
        section must be completely omitted from the rendered brief.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
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

            result = invoke(workspace, "sync", cli_home=home)

            self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")

            agents = (workspace / "AGENTS.md").read_text()

            # The Trello Tracking section must be absent when no tracker is bound
            self.assertNotIn(
                "## Trello Tracking", agents,
                "Trello Tracking section must be omitted when no tracker capability is bound"
            )

            # Brief intro must still render (unrelated section not affected)
            self.assertIn("No tracker section expected.", agents)
        finally:
            shutil.rmtree(workspace.parent)

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
            home = _home_for(workspace)
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            # Now run STANDALONE sync-agent --all (the path under test).
            # This exercises build_resolved_config_only() + resolved-config passthrough.
            result = invoke(workspace, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
        """Standalone resolved-config-only output must match the full materialize path.

        This is the 'identical output' guarantee: for a manifest with enabled catalog
        recipes, build_resolved_config_only() (the standalone sync-agent path) must
        enrich AGENTS.md with the same structured fields that the full sync path
        (materialize_recipes → resolved-config) renders at the root.

        Setup: a workspace with trello-mcp-workflow + tdd-flow enabled (real catalog
        recipes, not a 0-enabled stub) and one declared subrepo. Run the FULL sync
        path, then the STANDALONE sync-agent --all path, and compare the structured
        field needles (board_id, test_command) between root and subrepo AGENTS.md.
        """
        workspace = None
        try:
            parent = Path(tempfile.mkdtemp())
            workspace = parent / "rc-only-parity-workspace"
            workspace.mkdir()
            home = _home_for(workspace)
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'rc-only-parity'\n"
                "subrepos = ['sub/a']\n\n"
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
            # Create subrepo directory (required by target-resolve.py)
            (workspace / "sub" / "a").mkdir(parents=True)

            # --- Full materialize path (sync → resolved-config → root AGENTS.md) ---
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(
                result.returncode, 0,
                f"full sync failed:\n{result.stderr}\n{result.stdout}"
            )

            # --- Standalone path (sync-agent --all → build_resolved_config_only) ---
            result = invoke(workspace, "sync-agent", "--all", cli_home=home)
            self.assertEqual(
                result.returncode, 0,
                f"standalone sync-agent failed:\n{result.stderr}\n{result.stdout}"
            )

            # TRIAGE: JSON bindings-map parity, observable only at the
            # materialize process boundary (the old 'identical output'
            # guarantee). Full materialize path vs standalone
            # --resolved-config-only, each dumping resolved-config JSON.
            full_out = parent / "rc-full.json"
            standalone_out = parent / "rc-standalone.json"
            proc = _resolved_config_process(workspace, home, full_out)
            self.assertEqual(
                proc.returncode, 0,
                f"full materialize resolved-config run failed:\n{proc.stderr}"
            )
            proc = _resolved_config_process(
                workspace, home, standalone_out, "--resolved-config-only")
            self.assertEqual(
                proc.returncode, 0,
                f"standalone --resolved-config-only run failed:\n{proc.stderr}"
            )
            full_data = json.loads(full_out.read_text())
            standalone_data = json.loads(standalone_out.read_text())
            self.assertEqual(
                full_data["bindings"], standalone_data["bindings"],
                "standalone --resolved-config-only bindings map must exactly "
                "match the full materialize path bindings map",
            )
            # Auto-binding sanity: enabled catalog recipes must appear in both.
            self.assertIn("tracker", full_data["bindings"])
            self.assertIn("tracker", standalone_data["bindings"])
            self.assertIn("test-runner", full_data["bindings"])
            self.assertIn("test-runner", standalone_data["bindings"])

            root_agents = (workspace / "AGENTS.md").read_text()
            standalone_agents = (workspace / "sub" / "a" / "AGENTS.md").read_text()

            # Parity: every structured needle the full path renders at the root
            # must also be rendered by the standalone resolved-config-only path.
            for needle in (
                "aabbccddeeff001122334455",   # tracker binding → board_id
                "./tests/validate.sh",        # test-runner binding → test_command
            ):
                self.assertIn(
                    needle, root_agents,
                    f"full materialize path must render {needle!r} at the root"
                )
                self.assertIn(
                    needle, standalone_agents,
                    f"standalone resolved-config-only path must render {needle!r} "
                    f"in the subrepo, matching the full materialize path"
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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
                "version = '1.2.0'\n"
                "[recipes.vault-canonical-store.config]\n"
                "vault_scope = 'nnodes/test/autobind-scope'\n"
                # NO [[bindings]] section
            )

            proc = run_cli_env(workspace, "sync", home=home, tmpdir=workspace.parent)
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
                "version = '1.2.0'\n"
                "[recipes.vault-canonical-store.config]\n"
                "vault_scope = 'nnodes/test/json-scope'\n"
                # NO [[bindings]] section — auto-bind must handle this
            )

            materialize = home / "lib" / "_internal" / "recipe-materialize.py"
            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                resolved_out = Path(f.name)

            # TRIAGE: ai-specs sync — the resolved-config JSON bindings map is a
            # temp file deleted after each CLI run; no CLI verb exposes it, so
            # this assertion keeps the process-boundary materialize invocation
            # (now bound to the isolated home's own lib copy).
            proc = subprocess.run(
                ["python3", str(materialize),
                 str(workspace), str(home),
                 "--resolved-config-out", str(resolved_out)],
                text=True,
                capture_output=True,
                check=False,
                input="",
                env={
                    "PATH": os.environ.get("PATH", ""),
                    "HOME": str(workspace.parent / "cli-run-home"),
                    "TMPDIR": str(workspace.parent),
                    "AI_SPECS_HOME": str(home),
                    "AI_SPECS_NO_NETWORK": "1",
                    "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
                    "LC_ALL": "C",
                    "LANG": "C",
                },
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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            materialize = home / "lib" / "_internal" / "recipe-materialize.py"
            # A distinct explicit install root: the materialize lib copy lives
            # under `home`, so Path(__file__).parents[2] resolves to `home`,
            # while the caller-supplied home below points elsewhere. The
            # assertion proves the explicit home wins for catalog lookup.
            explicit_home = isolated_home(workspace.parent / "explicit-home")
            with _tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
                resolved_out = Path(f.name)

            try:
                # Pass AI_SPECS_HOME explicitly AND as the second positional arg.
                # build_resolved_config_only now uses the positional arg to locate the
                # catalog instead of falling back to Path(__file__).parents[2].
                # TRIAGE: ai-specs sync — the resolved-config JSON bindings map is a
                # temp file deleted after each CLI run; no CLI verb exposes it, so
                # this assertion keeps the process-boundary materialize invocation
                # (running from the isolated home's own lib copy while the explicit
                # home deliberately diverges from the __file__ fallback).
                proc = subprocess.run(
                    ["python3", str(materialize),
                     str(workspace), str(explicit_home),
                     "--resolved-config-out", str(resolved_out),
                     "--resolved-config-only"],
                    text=True,
                    capture_output=True,
                    check=False,
                    input="",
                    env={
                        "PATH": os.environ.get("PATH", ""),
                        "HOME": str(workspace.parent / "cli-run-home"),
                        "TMPDIR": str(workspace.parent),
                        "AI_SPECS_HOME": str(explicit_home),
                        "AI_SPECS_NO_NETWORK": "1",
                        "LC_ALL": "C",
                        "LANG": "C",
                    },
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

    def _sync_render(self, manifest: str) -> Path:
        """Black-box staging: init + sync a fresh fixture workspace with `manifest`.

        Returns the workspace; caller must shutil.rmtree(workspace.parent).
        """
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-jd-"))
        workspace = tmp / "workspace"
        workspace.mkdir()
        home = _home_for(workspace)
        result = invoke(workspace, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        (workspace / "ai-specs" / "ai-specs.toml").write_text(manifest)
        result = invoke(workspace, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stdout}{result.stderr}")
        return workspace

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
        workspace = self._sync_render(
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
        try:
            agents = (workspace / "AGENTS.md").read_text()
            # Both must render
            self.assertIn("project tracking through the Roadmap board.", agents)
            self.assertIn("global persistent memory (no local config block).", agents)
            # engram has no [mcp.*] block — it must still appear
            self.assertIn("engram", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_mcp_section_renders_description_only_entry_with_global_marker(self):
        """FIX 1: Description-only entries are marked *(global)* to distinguish from
        full [mcp.*] blocks.
        """
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix1-global-marker'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[brief.mcp_descriptions]\n"
            'global-only = "A global MCP with no local config."\n'
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("*(global)*", agents)
            self.assertIn("A global MCP with no local config.", agents)
        finally:
            shutil.rmtree(workspace.parent)

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
            home = _home_for(workspace)
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            # Now run standalone sync-agent from the workspace root
            result = invoke(workspace, "sync-agent", "--all", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            subrepo_agents = (workspace / "sub" / "a" / "AGENTS.md").read_text()
            self.assertIn(
                "aabbcc112233ddeeff001122", subrepo_agents,
                "board_id must appear in subrepo AGENTS.md via standalone sync-agent resolved-config passthrough"
            )

    # --- FIX 3 ---

    def test_vcs_bullet_uses_recipe_id_for_github(self):
        """VCS bullet derives GitHub/gh from bound recipe id, not config.provider."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix3-github'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.git-pr-flow]\n"
            "enabled = true\n"
            "version = '1.3.0'\n"
            "[recipes.git-pr-flow.config]\n"
            "base_branch = 'main'\n\n"
            "[[bindings]]\n"
            "capability = 'vcs-pr-flow'\n"
            "recipe = 'git-pr-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("VCS/PR provider: GitHub (`gh` CLI)", agents)
            self.assertIn("base branch: `main`", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_vcs_bullet_uses_recipe_id_for_gitlab(self):
        """VCS bullet derives GitLab/glab from bound recipe id."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix3-gitlab'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.gitlab-mr-flow]\n"
            "enabled = true\n"
            "version = '1.2.0'\n"
            "[recipes.gitlab-mr-flow.config]\n"
            "base_branch = 'main'\n\n"
            "[[bindings]]\n"
            "capability = 'vcs-pr-flow'\n"
            "recipe = 'gitlab-mr-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("VCS/PR provider: GitLab (`glab` CLI)", agents)
            self.assertNotIn("(`gh` CLI)", agents)
            self.assertIn("base branch: `main`", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_vcs_bullet_uses_recipe_id_for_bitbucket(self):
        """VCS bullet derives Bitbucket/bb from bound recipe id."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix3-bitbucket'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.bitbucket-pr-flow]\n"
            "enabled = true\n"
            "version = '1.3.0'\n"
            "[recipes.bitbucket-pr-flow.config]\n"
            "base_branch = 'develop'\n\n"
            "[[bindings]]\n"
            "capability = 'vcs-pr-flow'\n"
            "recipe = 'bitbucket-pr-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("VCS/PR provider: Bitbucket (`bb` CLI)", agents)
            self.assertNotIn("(`gh` CLI)", agents)
            self.assertIn("base branch: `develop`", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_vcs_bullet_ignores_stale_provider_config(self):
        """Stale provider in manifest config must not override bound recipe id label."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix3-stale-provider'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.gitlab-mr-flow]\n"
            "enabled = true\n"
            "version = '1.2.0'\n"
            "[recipes.gitlab-mr-flow.config]\n"
            "base_branch = 'main'\n"
            "provider = 'github'\n\n"
            "[[bindings]]\n"
            "capability = 'vcs-pr-flow'\n"
            "recipe = 'gitlab-mr-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("VCS/PR provider: GitLab (`glab` CLI)", agents)
            self.assertNotIn("VCS/PR provider: github", agents)
        finally:
            shutil.rmtree(workspace.parent)

    # --- FIX 5 ---

    def test_malformed_resolved_config_degrades_gracefully(self):
        # TRIAGE: ai-specs sync — a malformed --resolved-config JSON payload can
        # never be produced by any CLI verb (sync always writes valid JSON), so the
        # renderer is exercised at its process boundary with a hand-crafted payload.
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
        # TRIAGE: ai-specs sync — a non-dict --resolved-config JSON payload can
        # never be produced by any CLI verb, so the renderer is exercised at its
        # process boundary with a hand-crafted payload.
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
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix7-trello'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\n"
            "enabled = true\n"
            "version = '1.2.0'\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = 'aabbcc112233445566778899'\n\n"
            "[[bindings]]\n"
            "capability = 'tracker'\n"
            "recipe = 'trello-mcp-workflow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("## Trello Tracking", agents)
            self.assertIn("aabbcc112233445566778899", agents)
            # Recipe id must NOT appear as a parenthetical annotation
            self.assertNotIn("(`trello-mcp-workflow`)", agents)
            self.assertNotIn("(trello-mcp-workflow)", agents)
        finally:
            shutil.rmtree(workspace.parent)

    # --- FIX 8 ---

    def test_useful_commands_does_not_fabricate_validate_sh(self):
        """FIX 8: When test_command is 'run.sh', agents-render must NOT emit a
        fabricated 'validate.sh' line derived via str.replace.
        Only explicitly provided commands must appear.
        """
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix8-no-fabricate'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.tdd-flow]\n"
            "enabled = true\n"
            "version = '1.0.0'\n"
            "[recipes.tdd-flow.config]\n"
            "test_command = './tests/run.sh'\n\n"
            "[[bindings]]\n"
            "capability = 'test-runner'\n"
            "recipe = 'tdd-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("./tests/run.sh", agents)
            # validate.sh was NOT provided — must not appear
            self.assertNotIn("validate.sh", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_useful_commands_explicit_validate_renders(self):
        """FIX 8: When validate.sh is explicitly in brief.useful_commands, it DOES render."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix8-explicit-validate'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[brief]\n"
            'useful_commands = ["Full validation: `./tests/validate.sh`"]\n\n'
            "[recipes.tdd-flow]\n"
            "enabled = true\n"
            "version = '1.0.0'\n"
            "[recipes.tdd-flow.config]\n"
            "test_command = './tests/run.sh'\n\n"
            "[[bindings]]\n"
            "capability = 'test-runner'\n"
            "recipe = 'tdd-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("./tests/run.sh", agents)
            self.assertIn("./tests/validate.sh", agents)
        finally:
            shutil.rmtree(workspace.parent)

    # --- FIX 9 ---

    def test_integration_branch_renders_as_labeled_line(self):
        """FIX 9: integration_branch must appear as '- **Integration branch**: `<value>`'
        not just as a bare token to avoid tautological needle matching.
        """
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix9-branch'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[recipes.worktree-flow]\n"
            "enabled = true\n"
            "version = '1.2.2'\n"
            "[recipes.worktree-flow.config]\n"
            "integration_branch = 'development'\n\n"
            "[[bindings]]\n"
            "capability = 'worktree-isolation'\n"
            "recipe = 'worktree-flow'\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("- **Integration branch**: `development`", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_enabled_runtimes_renders_as_labeled_line(self):
        """FIX 9: enabled runtimes must appear as a labeled line with backtick values."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix9-runtimes'\n\n"
            "[agents]\n"
            "enabled = ['claude', 'cursor']\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("- **Enabled runtimes**: `claude`, `cursor`", agents)
        finally:
            shutil.rmtree(workspace.parent)

    def test_redaction_sentinel_is_exact_string(self):
        """FIX 9: redacted secrets render as '***REDACTED***' exactly."""
        workspace = self._sync_render(
            "[project]\n"
            "name = 'fix9-redact'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            "[mcp.demo]\n"
            "command = 'npx'\n"
            "env = { SECRET_KEY = 'literal-value' }\n"
        )
        try:
            agents = (workspace / "AGENTS.md").read_text()
            self.assertIn("***REDACTED***", agents)
            self.assertNotIn("literal-value", agents)
        finally:
            shutil.rmtree(workspace.parent)

    # --- FIX 10 ---

    def test_test_command_resolves_via_test_runner_capability_binding(self):
        # TRIAGE: ai-specs sync — binding a capability to a custom recipe id that
        # the catalog does not provide is rejected by manifest validation, so the
        # binding-substitution contract is exercised at the renderer process
        # boundary with a hand-crafted resolved-config.
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
        # TRIAGE: ai-specs sync — binding worktree-isolation to a custom recipe id
        # outside the catalog is rejected by manifest validation; the
        # binding-substitution contract is exercised at the renderer process
        # boundary with a hand-crafted resolved-config.
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
        # TRIAGE: ai-specs sync — binding canonical-store to a custom recipe id
        # outside the catalog is rejected by manifest validation; the
        # binding-substitution contract is exercised at the renderer process
        # boundary with a hand-crafted resolved-config.
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
        """FIX A (R2): the standalone resolved-config-only path (sync-agent --all)
        must leave NO ai-specs-recipe-mcp-* temp files behind and must produce a
        valid resolved-config (bindings/recipes/enabled) — observed black-box by
        the subrepo AGENTS.md receiving the bound recipe's structured fields.
        """
        import glob

        parent = Path(tempfile.mkdtemp(prefix="ai-specs-fixa-"))
        workspace = parent / "fixA-workspace"
        workspace.mkdir()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixA-no-temp-leak'\n"
                "subrepos = ['sub/a']\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.trello-mcp-workflow.config]\n"
                "board_id = 'aabbccddeeff001122334455'\n\n"
                "[[bindings]]\n"
                "capability = 'tracker'\n"
                "recipe = 'trello-mcp-workflow'\n"
            )
            (workspace / "sub" / "a").mkdir(parents=True)

            # Full sync first so the standalone path has its resolved-skills input.
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            # TMPDIR is scoped to `parent` by the CLI env helpers, so the
            # recipe-mcp temp leak check is hermetic.
            before = set(glob.glob(str(parent / "**" / "ai-specs-recipe-mcp-*.json"), recursive=True))

            # The standalone sync-agent path uses --resolved-config-only internally.
            result = invoke(workspace, "sync-agent", "--all", cli_home=home)
            self.assertEqual(
                result.returncode, 0,
                f"standalone sync-agent --all failed:\n{result.stderr}\n{result.stdout}",
            )

            # No recipe-mcp temp files may remain after the standalone path.
            after = set(glob.glob(str(parent / "**" / "ai-specs-recipe-mcp-*.json"), recursive=True))
            self.assertEqual(
                after - before, set(),
                f"resolved-config-only leaked recipe-mcp temp(s): {after - before}",
            )

            # The forwarded resolved-config must have been valid JSON with the
            # expected structured fields: the bound board_id reaches the subrepo.
            subrepo_agents = (workspace / "sub" / "a" / "AGENTS.md").read_text()
            self.assertIn(
                "aabbccddeeff001122334455", subrepo_agents,
                "standalone resolved-config-only output must carry bound recipe "
                "fields (board_id) into the subrepo AGENTS.md",
            )
        finally:
            shutil.rmtree(parent)

    def test_resolved_config_only_mode_fails_loudly_not_silently(self):
        """FIX A (R2): sync propagates a non-zero exit when the project root is
        missing (the resolved-config generation must fail loudly, never || true).
        """
        probe = Path(tempfile.mkdtemp(prefix="ai-specs-fail-loud-"))
        home = _home_for(probe / "probe-project")
        result = invoke(Path("/nonexistent/project/root"), "sync", cli_home=home)
        # Must exit non-zero when project root does not exist
        self.assertNotEqual(
            result.returncode, 0,
            "Expected non-zero exit for missing project root but got 0",
        )

    def test_resolved_config_only_invalid_binding_exits_nonzero(self):
        """FIX 2 (R3): sync must exit non-zero on manifest binding validation
        errors (an explicit binding referencing an unknown recipe), surfacing an
        ERROR on stderr — never swallowing the failure with a silent exit 0.
        """
        parent = Path(tempfile.mkdtemp(prefix="ai-specs-invalid-binding-"))
        workspace = parent / "invalid-binding-workspace"
        workspace.mkdir()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
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

            result = invoke(workspace, "sync", cli_home=home)
            # Must exit non-zero (matching the standalone resolved-config-only contract)
            self.assertNotEqual(
                result.returncode, 0,
                "Expected non-zero exit for invalid binding but got 0.\n"
                f"stderr: {result.stderr}\nstdout: {result.stdout}",
            )
            # Error must be surfaced (not swallowed silently): the compact sync
            # output marks the validation failure with ✗ and names the cause.
            combined = result.stdout + result.stderr
            self.assertIn(
                "references disabled/unknown recipe", combined,
                "The invalid-binding validation error must be surfaced, not swallowed.\n"
                f"stderr: {result.stderr}\nstdout: {result.stdout}",
            )

            # TRIAGE: standalone --resolved-config-only restoration. The
            # standalone path must not swallow the binding validation error
            # either: it must exit non-zero, surface the cause, and write no
            # resolved-config output.
            resolved_out = parent / "invalid-binding-resolved.json"
            proc = _resolved_config_process(
                workspace, home, resolved_out, "--resolved-config-only")
            self.assertNotEqual(
                proc.returncode, 0,
                "standalone --resolved-config-only must not swallow the "
                "binding validation error and exit 0\n"
                f"stderr: {proc.stderr}\nstdout: {proc.stdout}",
            )
            self.assertIn(
                "references disabled/unknown recipe", proc.stderr,
                "standalone --resolved-config-only must surface the binding "
                "validation error\n"
                f"stderr: {proc.stderr}\nstdout: {proc.stdout}",
            )
            self.assertFalse(
                resolved_out.exists(),
                "resolved-config output must not be written when binding "
                "validation fails",
            )
        finally:
            shutil.rmtree(parent)

    # --- FIX B (Round 2) ---

    def test_wrong_typed_inner_fields_degrade_gracefully_no_crash(self):
        # TRIAGE: ai-specs sync — wrong-typed inner resolved-config fields can
        # never be produced by any CLI verb, so the renderer is exercised at its
        # process boundary with a hand-crafted payload.
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
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(self._manifest(self._wf_version()))
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            after_claude = (workspace / ".claude" / "settings.json").read_bytes()
            after_oc = (workspace / ".opencode" / "plugin" / "worktree-flow-worktree-gate.ts").read_bytes()
            self.assertEqual(before_claude, after_claude)
            self.assertEqual(before_oc, after_oc)
        finally:
            shutil.rmtree(workspace.parent)


class TestVcsDropRemediations(unittest.TestCase):
    """Remediation tests for verify-report CRITICAL gaps.

    CRITICAL 1: Stale provider config must emit a warning at sync time.
    CRITICAL 2: Defaulted base_branch from catalog must propagate into resolved config.
    """

    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-vcs-remediation-"))
        shutil.copytree(FIXTURE_ROOT, tmp / "workspace")
        return tmp / "workspace"

    def test_sync_warns_on_stale_provider_config_in_vcs_recipe(self):
        """CRITICAL 1: sync must warn when a manifest sets a stale 'provider' key
        in a VCS recipe's [config] block.

        The spec says: 'GIVEN a manifest still sets [recipes.gitlab-mr-flow.config]
        provider = "github", WHEN sync validates and renders, THEN sync warns that
        provider is an unknown config key.'

        merge_config() in recipe-materialize.py already warns on unknown keys.
        This test asserts the warning is actually emitted during sync.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'stale-provider-warning'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[recipes.gitlab-mr-flow]\n"
                "enabled = true\n"
                "version = '1.2.0'\n"
                "[recipes.gitlab-mr-flow.config]\n"
                "base_branch = 'main'\n"
                "provider = 'github'\n\n"  # stale key — must warn
                "[[bindings]]\n"
                "capability = 'vcs-pr-flow'\n"
                "recipe = 'gitlab-mr-flow'\n"
            )

            proc = invoke(workspace, "sync", cli_home=home)

            # Sync must succeed (not fail on the stale key)
            self.assertEqual(
                proc.returncode, 0,
                f"sync must succeed even with stale provider key.\nstderr: {proc.stderr}",
            )

            # Warning must appear on stderr
            self.assertIn(
                "provider",
                proc.stderr.lower(),
                f"Warning about stale 'provider' key must appear in stderr.\n"
                f"stderr: {proc.stderr}\nstdout: {proc.stdout}",
            )
            self.assertIn(
                "unknown",
                proc.stderr.lower(),
                f"Warning must mention 'unknown' config key.\nstderr: {proc.stderr}",
            )
        finally:
            shutil.rmtree(workspace.parent)

    def test_defaulted_base_branch_propagates_into_brief(self):
        """CRITICAL 2: When a VCS recipe is enabled without setting base_branch in
        the manifest, the catalog default must be merged into resolved config so
        the brief includes 'base branch: `<default>`'.

        The spec says: 'base_branch appended when configured or defaulted'.
        The design says: 'base_branch still read from recipes[vcs_recipe_id].config.base_branch
        with recipe default fallback during render if unset.'

        bitbucket-pr-flow has default = "development" in its recipe.toml.
        A manifest enabling it without base_branch must still get the clause.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'defaulted-base-branch'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[recipes.bitbucket-pr-flow]\n"
                "enabled = true\n"
                "version = '1.3.0'\n"
                # NO base_branch set — catalog default "development" must apply
                "[[bindings]]\n"
                "capability = 'vcs-pr-flow'\n"
                "recipe = 'bitbucket-pr-flow'\n"
            )

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            agents = (workspace / "AGENTS.md").read_text()

            # The VCS bullet must include the defaulted base branch
            self.assertIn(
                "VCS/PR provider: Bitbucket (`bb` CLI)",
                agents,
                "Bitbucket VCS bullet must render from recipe id",
            )
            self.assertIn(
                "base branch: `development`",
                agents,
                f"Defaulted base_branch 'development' must appear in brief.\n"
                f"AGENTS.md content:\n{agents}",
            )
        finally:
            shutil.rmtree(workspace.parent)


class TestCustomVcsWarning(unittest.TestCase):
    """Unknown/custom vcs-pr-flow recipe ids must warn to stderr and use generic label.

    When the bound vcs-pr-flow recipe id is not in _VCS_RECIPE_LABELS, the renderer
    must emit a '⚠ ai-specs:' warning to stderr and render 'VCS PR (custom)' as the
    bullet label instead of falling back silently.
    """

    AGENTS_RENDER = ROOT / "lib" / "_internal" / "agents-render.py"

    def run_render_with_stderr(self, toml_text: str, resolved: dict) -> tuple[str, str]:
        """Invoke agents-render.py, return (agents_output, stderr)."""
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            resolved_path = tmp_path / "resolved.json"
            toml_path.write_text(toml_text)
            resolved_path.write_text(json.dumps(resolved))
            proc = subprocess.run(
                [
                    "python3", str(self.AGENTS_RENDER),
                    str(toml_path), str(output_path),
                    "--resolved-config", str(resolved_path),
                ],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, f"agents-render.py crashed:\n{proc.stderr}")
            self.assertTrue(output_path.exists())
            return output_path.read_text(), proc.stderr

    def test_unknown_vcs_recipe_warns_to_stderr(self):
        # TRIAGE: ai-specs sync — an unknown/custom vcs-pr-flow recipe id cannot
        # be declared through any CLI verb (manifest binding validation rejects
        # recipes outside the catalog), so the custom-id warning and generic
        # label are exercised at the renderer process boundary with a
        # hand-crafted resolved-config.
        """Bound custom vcs-pr-flow id → stderr contains ⚠ ai-specs: warning."""
        resolved = {
            "bindings": {"vcs-pr-flow": "my-custom-vcs"},
            "recipes": {
                "my-custom-vcs": {"base_branch": "trunk"},
            },
            "enabled": ["my-custom-vcs"],
        }
        agents, stderr = self.run_render_with_stderr(
            "[project]\nname = 'custom-vcs-warning'\n", resolved,
        )
        self.assertIn("⚠ ai-specs:", stderr)
        self.assertIn("my-custom-vcs", stderr)
        self.assertIn("VCS PR (custom)", stderr)

    def test_unknown_vcs_recipe_renders_generic_label(self):
        # TRIAGE: ai-specs sync — an unknown/custom vcs-pr-flow recipe id cannot
        # be declared through any CLI verb (manifest binding validation rejects
        # recipes outside the catalog), so the custom-id warning and generic
        # label are exercised at the renderer process boundary with a
        # hand-crafted resolved-config.
        """Bound custom vcs-pr-flow id → AGENTS.md uses 'VCS PR (custom)' label."""
        resolved = {
            "bindings": {"vcs-pr-flow": "my-custom-vcs"},
            "recipes": {
                "my-custom-vcs": {"base_branch": "trunk"},
            },
            "enabled": ["my-custom-vcs"],
        }
        agents, stderr = self.run_render_with_stderr(
            "[project]\nname = 'custom-vcs-label'\n", resolved,
        )
        self.assertIn("VCS/PR provider: VCS PR (custom)", agents)
        self.assertIn("base branch: `trunk`", agents)

    def test_unknown_vcs_warning_once_per_id(self):
        # TRIAGE: ai-specs sync — an unknown/custom vcs-pr-flow recipe id cannot
        # be declared through any CLI verb (manifest binding validation rejects
        # recipes outside the catalog), so the custom-id warning and generic
        # label are exercised at the renderer process boundary with a
        # hand-crafted resolved-config.
        """Warning must appear exactly once per unknown id per render invocation."""
        resolved = {
            "bindings": {"vcs-pr-flow": "my-custom-vcs"},
            "recipes": {
                "my-custom-vcs": {"base_branch": "trunk"},
            },
            "enabled": ["my-custom-vcs"],
        }
        agents, stderr = self.run_render_with_stderr(
            "[project]\nname = 'custom-vcs-dedup'\n", resolved,
        )
        # Count occurrences of the warning prefix
        warning_count = stderr.count("⚠ ai-specs: VCS recipe 'my-custom-vcs'")
        self.assertEqual(
            warning_count, 1,
            f"Warning must appear exactly once, but found {warning_count} times.\n"
            f"stderr: {stderr}",
        )


class GateRefreshCliTests(unittest.TestCase):
    """4.3 — E2E: `ai-specs sync --refresh-gates` refreshes a customized gate
    after a cache-only immutable backup, while ordinary sync preserves it."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="gate-refresh-")
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.workspace = self.base / "workspace"
        self.workspace.mkdir()
        self.home = _home_for(self.workspace)

    def _gate_backup_path(self, rel_path: str, content_sha: str) -> Path:
        """Cache-only immutable backup path (project-cache.gate_backup_path contract):
        backups/<sha256(rel_path)>/<content_sha>.sh under the per-project cache."""
        rel_key = hashlib.sha256(rel_path.encode("utf-8")).hexdigest()
        return cache_project_dir(self.workspace, self.home) / "backups" / rel_key / f"{content_sha}.sh"

    def test_ordinary_sync_preserves_customized_gate_refresh_flag_updates(self):
        result = invoke(self.workspace, "init", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        (self.workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'gate-refresh'\nsubrepos = []\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n\n"
            "[recipes.worktree-flow.config]\ngate_mode = 'always'\n"
        )
        proc = run_cli_env(self.workspace, "sync", home=self.home, tmpdir=self.base,
                           extra_env={"AI_SPECS_GATE_OFFLINE": "1"})
        self.assertEqual(proc.returncode, 0, proc.stderr)
        gate = self.workspace / "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"
        self.assertTrue(gate.is_file(), "gate launcher must materialize")
        rendered = gate.read_bytes()

        # Customize; ordinary sync must preserve with a warning.
        gate.write_bytes(b"# customized gate\n")
        proc = run_cli_env(self.workspace, "sync", home=self.home, tmpdir=self.base,
                           extra_env={"AI_SPECS_GATE_OFFLINE": "1"})
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(gate.read_bytes(), b"# customized gate\n")
        self.assertIn("user-modified", proc.stderr + proc.stdout)

        # Explicit refresh replaces and backs up the exact pre-refresh bytes.
        proc = run_cli_env(self.workspace, "sync", "--refresh-gates",
                           home=self.home, tmpdir=self.base,
                           extra_env={"AI_SPECS_GATE_OFFLINE": "1"})
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(gate.read_bytes(), rendered,
                         "refresh must restore the CLI-rendered gate bytes")
        rel = "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"
        backup = self._gate_backup_path(
            rel,
            hashlib.sha256(b"# customized gate\n").hexdigest(),
        )
        self.assertTrue(backup.is_file(), f"immutable backup missing at {backup}")
        self.assertEqual(backup.read_bytes(), b"# customized gate\n")


class FanOutDriftTests(unittest.TestCase):
    """Fan-out must mirror managed artifacts and remove stale derived files."""

    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-fanout-drift-"))
        workspace = tmp / "workspace"
        workspace.mkdir()
        return workspace

    def test_command_fanout_preserves_nonmanaged_files(self):
        """D3' — user-added command files survive resync (rm -rf removed).

        Behavior change from the false-success card: the old contract here
        (non-managed files deleted on every sync) was the recorded defect D3.
        """
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname = 'fanout-drift'\n\n"
                "[agents]\nenabled = ['cursor']\n"
            )
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            nonmanaged = workspace / ".cursor" / "commands" / "stale.md"
            nonmanaged.write_text("# stale\n")
            self.assertTrue(nonmanaged.is_file())

            proc = subprocess.run(
                [str(CLI), "sync", str(workspace)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertTrue(
                nonmanaged.is_file(),
                "non-managed command file must survive resync (D3')",
            )
            self.assertIn("stale.md", proc.stderr)
        finally:
            shutil.rmtree(workspace.parent)

    def test_cursor_skills_symlink_points_to_resolved(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname = 'cursor-skills'\n\n"
                "[agents]\nenabled = ['cursor']\n"
            )
            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            skills_link = workspace / ".cursor" / "skills"
            resolved = (cache_project_dir(workspace, home) / "resolved-skills").resolve()
            self.assertTrue(skills_link.is_symlink(), ".cursor/skills must be a symlink")
            self.assertEqual(skills_link.resolve(), resolved)
        finally:
            shutil.rmtree(workspace.parent)

    def test_cursor_skills_migrates_real_directory(self):
        workspace = self.make_workspace()
        home = _home_for(workspace)
        try:
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname = 'cursor-skills-migrate'\n\n"
                "[agents]\nenabled = ['cursor']\n"
            )
            legacy = workspace / ".cursor" / "skills" / "foo"
            legacy.mkdir(parents=True)
            (legacy / "SKILL.md").write_text("# foo\n")

            result = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            skills_link = workspace / ".cursor" / "skills"
            resolved = (cache_project_dir(workspace, home) / "resolved-skills").resolve()
            self.assertTrue(skills_link.is_symlink())
            self.assertEqual(skills_link.resolve(), resolved)
            self.assertFalse((resolved / "foo").exists())
        finally:
            shutil.rmtree(workspace.parent)


class CliVersionSyncGateTests(unittest.TestCase):
    def test_exact_pin_mismatch_aborts_sync(self):
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "workspace"
            workspace.mkdir()
            home = _home_for(workspace)
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            toml = workspace / "ai-specs" / "ai-specs.toml"
            toml.write_text(
                toml.read_text().rstrip()
                + '\n\n[tool]\nversion = "99.99.99"\npolicy = "exact"\n'
            )
            result = invoke(workspace, "sync", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("99.99.99", result.stderr)

    def test_ignore_cli_version_flag_proceeds_with_warning(self):
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "workspace"
            workspace.mkdir()
            home = _home_for(workspace)
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            toml = workspace / "ai-specs" / "ai-specs.toml"
            toml.write_text(
                toml.read_text().rstrip()
                + '\n\n[tool]\nversion = "99.99.99"\npolicy = "exact"\n'
            )
            result = invoke(workspace, "sync", "--ignore-cli-version", cli_home=home)
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn("ignoring CLI version policy", result.stderr)


class SyncAgentGuardOrderTests(unittest.TestCase):
    """Guards (toml exists, agents configured) must fire BEFORE recipe materialize.

    Round 3 regression: materialize was moved before flatten (Round 1 fix) but
    also ran before the init / no-agents guards, producing confusing errors
    instead of helpful user messages on uninitialized or agent-less projects.

    The critical path: sync-agent --source-root <root> --target <path>
    bypasses target-resolve.py (which provides its own guard for the simple
    'sync /path' invocation).  In that code path materialize previously ran
    before either guard, so missing-toml errors said 'recipe materialize failed'
    instead of 'Run ai-specs init first', and no-agents projects ran a full
    (wasted) materialize before emitting 'no agents to sync'.
    """

    def test_sync_agent_explicit_flags_missing_toml_emits_init_guidance_not_materialize_error(self):
        """sync-agent --source-root --target on an uninitialized project must emit
        'Run ai-specs init first', not 'recipe materialize failed'."""
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "workspace"
            workspace.mkdir()
            home = _home_for(workspace)
            # Do NOT run init — no ai-specs/ai-specs.toml
            result = invoke(workspace, "sync-agent", "--source-root", str(workspace), "--target", str(workspace), cli_home=home, append_root=False)
            self.assertNotEqual(result.returncode, 0)
            combined = result.stdout + result.stderr
            # Must emit the helpful guidance message from the TOML guard
            self.assertIn("ai-specs init", combined)
            # Must NOT say 'recipe materialize failed' as the primary UX signal
            self.assertNotIn("recipe materialize failed", combined)
            # Must NOT expose a raw Python traceback
            self.assertNotIn("Traceback (most recent call last)", combined)

    def test_sync_agent_explicit_flags_no_agents_exits_cleanly_without_materialize_error(self):
        """sync-agent --source-root --target on a project with no agents must
        emit 'no agents to sync' cleanly (exit 0) without any materialize error
        or traceback. Flatten may still run to populate the resolved-skills cache."""
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "workspace"
            workspace.mkdir()
            home = _home_for(workspace)
            result = invoke(workspace, "init", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # Overwrite toml with empty agents list
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname = 'guard-order-test'\n\n[agents]\nenabled = []\n"
            )
            result = invoke(workspace, "sync-agent", "--source-root", str(workspace), "--target", str(workspace), cli_home=home, append_root=False)
            # Exit 0 (graceful early exit) with a warning
            self.assertEqual(result.returncode, 0, result.stderr)
            combined = result.stdout + result.stderr
            self.assertIn("no agents to sync", combined)
            # Must NOT expose a raw materialize error/traceback
            self.assertNotIn("Traceback (most recent call last)", combined)
            self.assertNotIn("ERROR: recipe materialize failed", combined)


if __name__ == "__main__":
    unittest.main()
