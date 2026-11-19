"""T6 (S1): sync never removes a real directory at an agent skills path.

Parity contract §4: skills dirs are an absolute symlink, and an existing real
directory (or file) there is a hard error, never overwritten. Before T6 the
spine ran ``rm -rf`` on it, so a user's own skills were deleted with exit 0.
"""

from __future__ import annotations

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
from _blackbox import isolated_home  # noqa: E402

CLI = _go_cli.cli()
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"


class SyncSkillsOwnershipTests(unittest.TestCase):
    def setUp(self) -> None:
        base = Path(tempfile.mkdtemp(prefix="ai-specs-skills-", dir="/private/tmp"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        self.base = base
        self.home = isolated_home(base)
        self.project = base / "project"
        for name in ("skills", "commands"):
            (self.project / "ai-specs" / name).mkdir(parents=True)
        (self.project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 't'\n\n[agents]\nenabled = ['claude']\n",
            encoding="utf-8",
        )
        self.skills = self.project / ".claude" / "skills"
        self.user_skill = self.skills / "mine" / "SKILL.md"
        self.user_skill.parent.mkdir(parents=True)
        self.user_skill.write_text("# mine\n", encoding="utf-8")

    def link_agent_parent(self, target: str) -> None:
        """Replace the real `.claude` dir with a symlink to `target`."""
        shutil.rmtree(self.project / ".claude")
        (self.project / ".claude").symlink_to(target)

    def sync(self) -> subprocess.CompletedProcess:
        return self.run_cli("sync", str(self.project))

    def sync_agent(self, *args: str) -> subprocess.CompletedProcess:
        return self.run_cli("sync-agent", *args, str(self.project))

    def run_cli(self, *args: str) -> subprocess.CompletedProcess:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.base / "home"),
            "TMPDIR": str(self.base / "tmp"),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_GATE_OFFLINE": "1",
            "AI_SPECS_NO_NETWORK": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            "LC_ALL": "C",
            "LANG": "C",
        }
        for name in ("home", "tmp"):
            (self.base / name).mkdir(exist_ok=True)
        return subprocess.run(
            [str(CLI), *args],
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
            check=False,
            input="",
        )

    def test_real_skills_dir_is_refused_and_left_untouched(self) -> None:
        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        refusal = f"    ✗ refuse to overwrite non-symlink: {self.skills}"
        self.assertIn(refusal, proc.stderr)
        self.assertIn("move or delete it so ai-specs can create the symlink", proc.stderr)
        # Nothing in the directory was touched, and the refusal happened before
        # any of this agent's other writes.
        self.assertEqual(self.user_skill.read_text(encoding="utf-8"), "# mine\n")
        self.assertFalse(self.skills.is_symlink())
        self.assertFalse((self.project / ".claude" / "commands").exists())
        self.assertFalse((self.project / "CLAUDE.md").exists())

    def test_full_sync_skills_refusal_leaves_project_unchanged(self) -> None:
        from _blackbox import snapshot

        before = snapshot(self.project)
        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertEqual(snapshot(self.project), before)

    def test_standalone_sync_agent_refuses_symlinked_agent_parent(self) -> None:
        from _blackbox import snapshot

        # A user-managed `.claude` symlink to a directory outside the project:
        # writing through it would land outside the project. Both entry points
        # share one check, so both refuse and neither writes anything.
        outside = self.base / "outside"
        outside.mkdir()
        self.link_agent_parent(str(outside))
        # The standalone body requires the root AGENTS.md before its agent loop.
        (self.project / "AGENTS.md").write_text("# brief\n", encoding="utf-8")
        outside_before = snapshot(outside)

        proc = self.sync_agent("--claude")
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn(
            f"    ✗ ancestor path is a symlink: {self.project / '.claude'}",
            proc.stderr,
        )
        self.assertEqual(snapshot(outside), outside_before, "sync-agent wrote outside the project")

        before_sync = snapshot(self.project)
        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn(
            f"    ✗ ancestor path is a symlink: {self.project / '.claude'}",
            proc.stderr,
        )
        self.assertEqual(snapshot(self.project), before_sync, "full sync wrote before refusing")
        self.assertEqual(snapshot(outside), outside_before, "full sync wrote outside the project")

    # ── T6b: inside-root symlinked agent parent is allowed ───────────────

    def test_symlinked_agent_parent_inside_root_syncs_normally(self) -> None:
        # `.claude -> ./config/claude` stays inside the project root: the sync
        # writes through it, landing inside the project.
        inside = self.project / "config" / "claude"
        inside.mkdir(parents=True)
        self.link_agent_parent("./config/claude")

        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertTrue((inside / "skills").is_symlink(), "skills link not created inside the project")
        self.assertTrue((inside / "commands").is_dir(), "commands dir not created inside the project")
        self.assertTrue((self.project / "CLAUDE.md").is_symlink())

        # The standalone entry point runs the same check and also allows it.
        proc = self.sync_agent("--claude")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertTrue((inside / "skills").is_symlink())

    def test_chained_symlinked_agent_parent_ending_outside_is_refused(self) -> None:
        from _blackbox import snapshot

        outside = self.base / "outside"
        outside.mkdir()
        chained = self.project / "config" / "claude"
        chained.parent.mkdir(parents=True)
        chained.symlink_to(outside)
        self.link_agent_parent("./config/claude")
        outside_before = snapshot(outside)

        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("    ✗ ancestor path is a symlink: ", proc.stderr)
        self.assertEqual(snapshot(outside), outside_before, "wrote outside the project")

    def test_dangling_symlinked_agent_parent_is_refused(self) -> None:
        self.link_agent_parent("./nowhere")

        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("    ✗ ancestor path is a symlink: ", proc.stderr)
        self.assertFalse((self.project / "nowhere").exists())
        self.assertFalse((self.project / "CLAUDE.md").exists())

    def test_inside_root_link_syncs_and_prunes_retired_hook(self) -> None:
        # A retired claude hook is pruned INSIDE the linked directory: the hook
        # prune plan follows the same inside-root rule as the agent preflight.
        inside = self.project / "config" / "claude"
        inside.mkdir(parents=True)
        settings = inside / "settings.json"
        settings.write_text(
            '{"hooks": {"PreToolUse": [{"_ai_specs_managed": "ai-specs:hooks:hookdemo:gone"}]}}',
            encoding="utf-8",
        )
        self.link_agent_parent("./config/claude")

        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertNotIn("ai-specs:hooks:hookdemo:gone", settings.read_text(encoding="utf-8"))
        self.assertTrue((inside / "skills").is_symlink())

    def test_outside_root_link_with_recorded_hook_refuses_before_writes(self) -> None:
        from _blackbox import snapshot

        outside = self.base / "outside"
        outside.mkdir()
        (outside / "settings.json").write_text(
            '{"hooks": {"PreToolUse": [{"_ai_specs_managed": "ai-specs:hooks:hookdemo:gone"}]}}',
            encoding="utf-8",
        )
        self.link_agent_parent(str(outside))
        outside_before = snapshot(outside)

        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("    ✗ ancestor path is a symlink: ", proc.stderr)
        self.assertIn("ai-specs:hooks:hookdemo:gone", (outside / "settings.json").read_text(encoding="utf-8"))
        self.assertEqual(snapshot(outside), outside_before, "pruned something outside the project")

    # ── write containment: a leaf link below an allowed parent ──────────

    def test_inside_agent_parent_does_not_write_through_outside_mcp_leaf(self):
        from _blackbox import snapshot

        inside = self.project / "config" / "pi"
        inside.mkdir(parents=True)
        (self.project / ".pi").symlink_to("./config/pi")
        outside = self.base / "outside-client.json"
        original = b'{"manual": true}\n'
        outside.write_bytes(original)
        (inside / "mcp.json").symlink_to(outside)
        manifest = self.project / "ai-specs/ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 't'\n"
            "[agents]\nenabled = ['pi']\n"
            "[mcp.demo]\ncommand = 'echo'\n"
        )
        before = snapshot(self.project)

        result = self.sync()

        self.assertEqual(outside.read_bytes(), original)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(snapshot(self.project), before)

    def test_real_agent_dir_does_not_write_through_outside_mcp_leaf(self):
        from _blackbox import snapshot

        # A REAL (non-symlinked) `.pi`: the leaf `mcp.json` is the link.
        inside = self.project / ".pi"
        inside.mkdir()
        outside = self.base / "outside-client.json"
        original = b'{"manual": true}\n'
        outside.write_bytes(original)
        (inside / "mcp.json").symlink_to(outside)
        manifest = self.project / "ai-specs/ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 't'\n"
            "[agents]\nenabled = ['pi']\n"
            "[mcp.demo]\ncommand = 'echo'\n"
        )
        before = snapshot(self.project)

        result = self.sync()

        self.assertEqual(outside.read_bytes(), original)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(snapshot(self.project), before)

    def test_inside_agent_parent_does_not_write_through_outside_commands_dir(self):
        from _blackbox import snapshot

        inside = self.project / "config" / "claude"
        inside.mkdir(parents=True)
        outside = self.base / "outside-commands"
        outside.mkdir()
        (outside / "manual.md").write_text("user owned\n", encoding="utf-8")
        (inside / "commands").symlink_to(outside)
        self.link_agent_parent("./config/claude")
        before = snapshot(self.project)
        outside_before = snapshot(outside)

        result = self.sync()

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(snapshot(self.project), before)
        self.assertEqual(snapshot(outside), outside_before, "wrote commands outside the project")

    # ── T6b follow-ups ──────────────────────────────────────────────────

    def test_relative_instruction_link_uses_the_resolved_link_directory(self):
        # `.omp -> ./config/omp`: the link is created at the RESOLVED dir, so the
        # relative target must be computed from there (../../AGENTS.md), not from
        # the lexical `.omp` spelling (../AGENTS.md, which dangles).
        real = self.project / "config" / "omp"
        real.mkdir(parents=True)
        manifest = self.project / "ai-specs/ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 't'\n[agents]\nenabled = ['omp']\n", encoding="utf-8"
        )
        (self.project / ".omp").symlink_to("./config/omp")

        result = self.sync()

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        link = real / "AGENTS.md"
        self.assertTrue(link.is_symlink(), "instruction link not created in the linked dir")
        self.assertTrue(link.exists(), f"instruction link dangles: {os.readlink(link)}")

    def test_agent_parent_inside_protected_tree_is_refused(self):
        self.link_agent_parent("./ai-specs/claude")

        result = self.sync()

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("    ✗ ", result.stderr)
        self.assertFalse((self.project / "ai-specs" / "claude").exists())


if __name__ == "__main__":
    unittest.main()
