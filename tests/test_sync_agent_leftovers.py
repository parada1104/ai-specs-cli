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

    def test_retiring_claude_keeps_edited_command_and_repointed_skills(self) -> None:
        self.seed()
        command = self.project / ".claude/commands/go.md"
        command.write_bytes(b"# my local edit\r\n")
        skills = self.project / ".claude/skills"
        personal = self.base / "personal-skills"
        personal.mkdir()
        skills.unlink()
        skills.symlink_to(personal, target_is_directory=True)
        self.set_agents("['pi']")
        proc = self.sync()

        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertEqual(command.read_bytes(), b"# my local edit\r\n")
        self.assertEqual(skills.resolve(), personal.resolve())
        self.assertIn(".claude/commands/go.md", self.lock_text())

    def test_retiring_claude_drops_missing_command_record(self) -> None:
        self.seed()
        command = self.project / ".claude/commands/go.md"
        command.unlink()
        self.set_agents("['pi']")
        proc = self.sync()

        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertNotIn(".claude/commands/go.md", self.lock_text())
        command.write_bytes(b"# handwritten replacement\r\n")
        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertEqual(command.read_bytes(), b"# handwritten replacement\r\n")

    def test_retirement_unlink_refusal_leaves_project_unchanged(self) -> None:
        from _blackbox import snapshot

        self.seed()
        commands = self.project / ".claude/commands"
        commands.chmod(0o555)
        self.addCleanup(commands.chmod, 0o755)
        self.set_agents("['pi']")
        before = snapshot(self.project)
        proc = self.sync()

        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertEqual(snapshot(self.project), before)

    def test_retirement_removes_subrepo_generated_skills_link(self) -> None:
        self.manifest.write_text(
            self.manifest.read_text().replace(
                "[project]\n", '[project]\nsubrepos = ["child"]\n', 1
            )
        )
        child = self.project / "child"
        child.mkdir()
        (child / "AGENTS.md").write_text("# child brief\n")
        self.seed()
        link = child / ".claude/skills"
        self.assertTrue(link.is_symlink())
        self.assertEqual(link.resolve(), (child / "ai-specs/skills").resolve())

        before = self.manifest.read_text()
        self.manifest.write_text(before.replace("['claude', 'pi']", "['pi']"))
        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertFalse(link.is_symlink())
        self.assertTrue((child / "ai-specs/skills").is_dir())

    def test_retirement_keeps_skills_link_to_prior_cli_home(self) -> None:
        self.seed()
        link = self.project / ".claude/skills"
        old_cache = self.base / "prior-install/cache/projects" / link.resolve().parent.name
        target = old_cache / "resolved-skills"
        target.mkdir(parents=True)
        link.unlink()
        link.symlink_to(target)
        self.set_agents("['pi']")

        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertTrue(link.is_symlink())
        self.assertEqual(link.resolve(), target.resolve())

    def test_retirement_link_unlink_refusal_leaves_project_unchanged(self) -> None:
        from _blackbox import snapshot

        self.seed()
        self.set_agents("['pi']")
        claude = self.project / ".claude"
        claude.chmod(0o555)
        self.addCleanup(claude.chmod, 0o755)
        before = snapshot(self.project)

        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("permission denied", proc.stderr)
        self.assertEqual(snapshot(self.project), before)

    def test_retired_command_notice_does_not_repeat(self) -> None:
        self.seed()
        self.set_agents("['pi']")
        first, second = self.sync(), self.sync()

        self.assertEqual((first.returncode, second.returncode), (0, 0))
        notice = ".claude/commands:"
        self.assertIn(notice, first.stderr)
        self.assertNotIn(notice, second.stderr)


if __name__ == "__main__":
    unittest.main()
