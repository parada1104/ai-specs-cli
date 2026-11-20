"""T7 (S3): sync removes what it generated for a removed agent, and only that.

When an agent leaves ``[agents].enabled``, the artifacts ai-specs wrote for it
(skills link, instruction link, recorded command copies, recorded MCP servers,
hook wiring and shims) and their lock records go, while content ai-specs never
generated — an unrecorded shim, a user MCP server, an unrecorded command copy —
stays byte-identical (with one notice for the ambiguous copies).
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(Path(__file__).resolve().parent))

import _go_cli  # noqa: E402
from _blackbox import isolated_home, populate_catalog  # noqa: E402

CLI = _go_cli.cli()

RECIPE = """\
[recipe]
id = "agentdemo"
name = "Demo"
description = "fixture"
version = "1.0.0"

[provides]
commands = [{ id = "go", path = "commands/go.md" }]

[[provides.hooks]]
id = "main"
event = "pre-tool-use"
script = "hooks/main.sh"
matcher = "Bash"
"""

MANIFEST = """\
[project]
name = 'leftovers'

[brief]
render = false

[agents]
enabled = {enabled}

[mcp.demo]
command = 'demo'

[recipes.agentdemo]
enabled = true
"""


class SyncAgentLeftoversTests(unittest.TestCase):
    def setUp(self) -> None:
        base = Path(tempfile.mkdtemp(prefix="ai-specs-leftovers-", dir="/private/tmp"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        self.base = base
        self.home = isolated_home(base, catalog=False)
        recipe = populate_catalog(self.home, "agentdemo", RECIPE)
        (recipe / "hooks").mkdir()
        (recipe / "hooks" / "main.sh").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        (recipe / "commands").mkdir()
        (recipe / "commands" / "go.md").write_text("# go\n", encoding="utf-8")
        self.project = base / "project"
        for name in ("skills", "commands"):
            (self.project / "ai-specs" / name).mkdir(parents=True)
        (self.project / "AGENTS.md").write_text("# brief\n", encoding="utf-8")
        self.manifest = self.project / "ai-specs" / "ai-specs.toml"
        self.set_agents("['claude', 'pi']")

    def set_agents(self, enabled: str) -> None:
        self.manifest.write_text(MANIFEST.format(enabled=enabled), encoding="utf-8")

    def sync(self) -> subprocess.CompletedProcess:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.base / "home"),
            "TMPDIR": str(self.base / "tmp"),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_GATE_OFFLINE": "1",
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        for name in ("home", "tmp"):
            (self.base / name).mkdir(exist_ok=True)
        return subprocess.run(
            [str(CLI), "sync", str(self.project)],
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
            check=False,
            input="",
        )

    def seed(self) -> None:
        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertEqual(proc.stderr, "", proc.stderr)

    def lock_text(self) -> str:
        return (self.project / "ai-specs" / ".ai-specs.lock").read_text(encoding="utf-8")

    def mcp_servers(self, rel: str) -> dict:
        doc = json.loads((self.project / rel).read_text(encoding="utf-8"))
        return doc.get("mcpServers", {})

    def test_retired_agent_leftovers_are_pruned_and_user_content_kept(self) -> None:
        self.seed()
        self.assertTrue((self.project / ".pi" / "skills").is_symlink())
        # A user MCP server and a hand-written extension in the retired agent's
        # directories: neither is ai-specs' output.
        doc = json.loads((self.project / ".pi" / "mcp.json").read_text(encoding="utf-8"))
        doc["mcpServers"]["manual"] = {"command": "manual"}
        (self.project / ".pi" / "mcp.json").write_text(json.dumps(doc), encoding="utf-8")
        user_shim = self.project / ".pi" / "extensions" / "mine.ts"
        user_shim.write_text("// user extension\n", encoding="utf-8")
        claude_commands = sorted(p.name for p in (self.project / ".claude" / "commands").iterdir())

        self.set_agents("['claude']")
        proc = self.sync()

        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertEqual(proc.stderr, "", proc.stderr)
        self.assertIn("  ✓ removed retired agent link .pi/skills", proc.stdout)
        self.assertIn("  ✓ removed retired agent pi MCP record .pi/mcp.json", proc.stdout)
        self.assertRegex(
            proc.stdout,
            r"    ✓ removed managed MCP server \"demo\" from \S*/\.pi/mcp\.json\n",
        )
        # The retired agent's managed artifacts are gone; its user's are not.
        self.assertFalse((self.project / ".pi" / "skills").is_symlink())
        self.assertFalse((self.project / ".pi" / "extensions" / "agentdemo-main.ts").exists())
        self.assertEqual(user_shim.read_text(encoding="utf-8"), "// user extension\n")
        self.assertEqual(sorted(self.mcp_servers(".pi/mcp.json")), ["manual"])
        lock = self.lock_text()
        self.assertNotIn(".pi/mcp.json", lock)
        self.assertNotIn(".pi/extensions/agentdemo-main.ts", lock)
        # The agent that is still enabled keeps everything.
        self.assertTrue((self.project / ".claude" / "skills").is_symlink())
        self.assertEqual(sorted(self.mcp_servers(".mcp.json")), ["demo"])
        self.assertEqual(
            sorted(p.name for p in (self.project / ".claude" / "commands").iterdir()),
            claude_commands,
        )

    def test_unrecorded_command_copies_of_a_retired_agent_are_kept_with_a_notice(self) -> None:
        self.seed()
        self.set_agents("['pi']")
        proc = self.sync()

        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        # One notice for the retired claude's command directory: its bundled
        # copies carry no ownership record, so they stay (pi has no commands
        # directory at all).
        self.assertRegex(
            proc.stderr,
            r"(?m)^  ! \.claude/commands: \d+ command copy\(ies\) not recorded as "
            r"ai-specs-managed \(kept; delete them if ai-specs did not write them\)$",
        )
        names = sorted(p.name for p in (self.project / ".claude" / "commands").iterdir())
        self.assertIn("rules-audit.md", names)  # unrecorded: kept
        self.assertNotIn("go.md", names)  # recorded recipe copy: pruned
        self.assertFalse((self.project / ".claude" / "skills").is_symlink())
        self.assertFalse((self.project / "CLAUDE.md").exists())
        # pi is still enabled and keeps its generated artifacts.
        self.assertTrue((self.project / ".pi" / "skills").is_symlink())
        self.assertTrue((self.project / ".pi" / "extensions" / "agentdemo-main.ts").exists())


if __name__ == "__main__":
    unittest.main()
