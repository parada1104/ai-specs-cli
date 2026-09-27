"""Black-box regression tests for the false-success / user-work-clobber card.

Defects covered (see docs/go-migration-parity-contract.md §9):
  - D5  : `hub` non-interactive status discards Doctor's exit code.
  - D3' : sync_one_agent rm -rf's agent commands dirs, destroying
          user-added command files.
  - D24 : `sync-agent` exits 0 with "no agents to sync" without ever running
          ensure_target_workspace, so subrepos get no AGENTS.md.
  - D4' : materialize_doc clobbers user-edited recipe docs unconditionally.

Every test drives `bin/ai-specs` as a subprocess and asserts on exit codes,
stdout/stderr, and emitted file trees. No lib/_internal imports.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"


def _cli_env() -> dict:
    return {
        **os.environ,
        "AI_SPECS_GATE_OFFLINE": "1",
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"),
    }


def _run(*args: str, cwd: Path | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(
        [str(CLI), *args],
        capture_output=True,
        text=True,
        env=_cli_env(),
        cwd=str(cwd) if cwd else None,
        check=False,
    )


class TestHubPropagatesDoctorExitCode(unittest.TestCase):
    """D5 — `ai-specs hub` piped must exit like `ai-specs doctor`."""

    def test_hub_piped_exits_1_when_doctor_reports_errors(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            proc = _run("init", str(root))
            self.assertEqual(proc.returncode, 0, proc.stderr)
            # Break the project so doctor reports at least one ERROR.
            (root / "AGENTS.md").unlink()

            doctor = _run("doctor", str(root))
            self.assertEqual(doctor.returncode, 1, "sanity: doctor must exit 1 here")

            hub = _run("hub", str(root))
            self.assertEqual(
                hub.returncode,
                1,
                f"hub piped must propagate doctor's exit code; stderr={hub.stderr!r}",
            )
            self.assertIn("Summary", hub.stdout)

    def test_hub_piped_still_exits_0_when_healthy(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            proc = _run("init", str(root))
            self.assertEqual(proc.returncode, 0, proc.stderr)
            sync = _run("sync", str(root))
            self.assertEqual(sync.returncode, 0, sync.stderr)

            hub = _run("hub", str(root))
            self.assertEqual(hub.returncode, 0, hub.stderr)


class TestSyncAgentPreservesUserCommandFiles(unittest.TestCase):
    """D3' — agent commands dirs must not be rm -rf'd on every sync."""

    def _init_with_claude(self, root: Path) -> None:
        proc = _run("init", str(root))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'preserve-commands-fixture'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n",
            encoding="utf-8",
        )

    def test_user_added_command_file_survives_resync(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            self._init_with_claude(root)

            first = _run("sync-agent", str(root), "--claude")
            self.assertEqual(first.returncode, 0, first.stderr)

            commands_dir = root / ".claude" / "commands"
            commands_dir.mkdir(parents=True, exist_ok=True)
            user_cmd = commands_dir / "my-user-cmd.md"
            user_cmd.write_text("# user-owned command\n", encoding="utf-8")

            second = _run("sync-agent", str(root), "--claude")
            self.assertEqual(second.returncode, 0, second.stderr)

            self.assertTrue(
                user_cmd.is_file(),
                "user-added command file must survive sync-agent (D3')",
            )
            self.assertEqual(
                user_cmd.read_text(encoding="utf-8"),
                "# user-owned command\n",
                "user-added command content must not be altered",
            )
            combined = first.stdout + first.stderr + second.stdout + second.stderr
            self.assertIn("my-user-cmd.md", combined)

    def test_managed_command_files_still_synced(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            root.mkdir()
            self._init_with_claude(root)
            (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
            (root / "ai-specs" / "commands" / "local-cmd.md").write_text(
                "# local managed command\n", encoding="utf-8"
            )
            proc = _run("sync-agent", str(root), "--claude")
            self.assertEqual(proc.returncode, 0, proc.stderr)
            synced = root / ".claude" / "commands" / "local-cmd.md"
            self.assertTrue(synced.is_file(), "managed commands must still be copied")
            self.assertEqual(
                synced.read_text(encoding="utf-8"), "# local managed command\n"
            )


class TestSyncAgentNoAgentsStillEnsuresWorkspace(unittest.TestCase):
    """D24 — no-agents exit must not skip ensure_target_workspace."""

    def _init_workspace(self, workspace: Path) -> None:
        proc = _run("init", str(workspace))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        # Deliberately NO [agents] section: zero agents to sync.
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'no-agents-fixture'\n"
            "subrepos = ['packages/a', 'packages/b']\n",
            encoding="utf-8",
        )

    def test_subrepos_get_workspace_even_with_no_agents(self):
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "workspace"
            shutil.copytree(FIXTURE_ROOT, workspace)
            self._init_workspace(workspace)

            proc = _run("sync-agent", str(workspace))
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertIn("no agents to sync", proc.stderr)

            for subrepo in ("packages/a", "packages/b"):
                agents_md = workspace / subrepo / "AGENTS.md"
                self.assertTrue(
                    agents_md.is_file(),
                    f"{subrepo} must receive AGENTS.md even with no agents (D24)",
                )

    def test_root_workspace_guard_still_applies_with_no_agents(self):
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "workspace"
            shutil.copytree(FIXTURE_ROOT, workspace)
            self._init_workspace(workspace)
            (workspace / "AGENTS.md").unlink()

            proc = _run("sync-agent", str(workspace))
            self.assertEqual(
                proc.returncode,
                1,
                "root workspace without AGENTS.md must fail, not report success",
            )


class TestSyncPreservesUserEditedRecipeDoc(unittest.TestCase):
    """D4' — recipe docs must follow the lock-tracked preservation policy."""

    def test_user_edited_recipe_doc_survives_resync(self):
        with tempfile.TemporaryDirectory() as tmp:
            workspace = Path(tmp) / "prj"
            workspace.mkdir()
            proc = _run("init", str(workspace))
            self.assertEqual(proc.returncode, 0, proc.stderr)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'preserve-doc-fixture'\n\n"
                "[agents]\n"
                "enabled = ['claude']\n\n"
                "[recipes.git-pr-flow]\n"
                "enabled = true\n"
                "version = '1.3.0'\n"
                "[recipes.git-pr-flow.config]\n"
                "base_branch = 'development'\n",
                encoding="utf-8",
            )

            first = _run("sync", str(workspace))
            self.assertEqual(first.returncode, 0, first.stderr)

            doc = workspace / "ai-specs" / "recipes" / "git-pr-flow" / "README.md"
            self.assertTrue(doc.is_file(), "recipe doc must materialize on first sync")

            original = doc.read_text(encoding="utf-8")
            doc.write_text(original + "\n<!-- user edit: local note -->\n", encoding="utf-8")

            second = _run("sync", str(workspace))
            self.assertEqual(second.returncode, 0, second.stderr)

            self.assertIn(
                "<!-- user edit: local note -->",
                doc.read_text(encoding="utf-8"),
                "user-edited recipe doc must not be clobbered by re-sync (D4')",
            )
            combined = second.stdout + second.stderr
            self.assertIn(
                "README.md",
                combined,
                "sync output must mention the preserved doc",
            )


if __name__ == "__main__":
    unittest.main()
