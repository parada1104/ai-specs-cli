"""E2E tests for [brief].render = false AGENTS.md opt-out."""
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
MULTI_TARGET_FIXTURE = ROOT / "tests" / "fixtures" / "target-resolve" / "multi-target"
PLACEHOLDER = "# AGENTS.md - Runtime context"


def append_brief_render_false(toml_path: Path) -> None:
    """Set [brief].render = false (template comments must not satisfy detection)."""
    text = toml_path.read_text().rstrip()
    text += "\n\n[brief]\nrender = false\n"
    toml_path.write_text(text + "\n")


class AgentsMdRenderOptOutTests(unittest.TestCase):
    def _make_target(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def _init(self, target: Path) -> None:
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_sync_skips_agents_md_when_render_false(self):
        target = self._make_target()
        self._init(target)
        agents_md = target / "AGENTS.md"
        manual = "# Manual runtime brief\n\nHands-off content.\n"
        agents_md.write_text(manual)
        append_brief_render_false(target / "ai-specs" / "ai-specs.toml")

        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(agents_md.read_text(), manual)
        self.assertIn("skipped AGENTS.md (brief.render = false)", result.stdout)

    def test_sync_default_render_true_preserves_divergent_brief(self):
        """`init` records a baseline first, so this exercises `user_modified`,
        not `untracked` as the name previously claimed. The assertions passed
        either way because the remedy text is state-agnostic. The true
        no-baseline path is covered in test_runtime_brief_baseline.py.
        """
        target = self._make_target()
        self._init(target)
        agents_md = target / "AGENTS.md"
        stale = "# Stale — sync must preserve this until ownership is explicit.\n"
        agents_md.write_text(stale)

        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(agents_md.read_text(), stale)
        combined = result.stdout + result.stderr
        self.assertNotIn("skipped AGENTS.md (brief.render = false)", combined)
        self.assertIn("--adopt-brief", combined)
        self.assertIn("ai-specs:runtime-brief", combined)

    def test_init_placeholder_when_render_false(self):
        target = self._make_target()
        self._init(target)
        agents_md = target / "AGENTS.md"
        agents_md.unlink()
        append_brief_render_false(target / "ai-specs" / "ai-specs.toml")

        result = subprocess.run(
            [str(CLI), "init", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(agents_md.is_file())
        self.assertEqual(agents_md.read_text().strip(), PLACEHOLDER)
        self.assertIn("placeholder", result.stderr.lower())

    def test_init_preserves_manual_agents_md_when_render_false(self):
        target = self._make_target()
        self._init(target)
        agents_md = target / "AGENTS.md"
        manual = "# My curated brief\n\nDo not touch.\n"
        agents_md.write_text(manual)
        append_brief_render_false(target / "ai-specs" / "ai-specs.toml")

        result = subprocess.run(
            [str(CLI), "init", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(agents_md.read_text(), manual)

    def test_two_syncs_with_render_false_are_byte_stable(self):
        target = self._make_target()
        self._init(target)
        manual = "# Stable manual brief\n"
        (target / "AGENTS.md").write_text(manual)
        append_brief_render_false(target / "ai-specs" / "ai-specs.toml")

        for _ in range(2):
            result = subprocess.run(
                [str(CLI), "sync", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((target / "AGENTS.md").read_text(), manual)

    def test_render_false_without_marker_leaves_file_untouched(self):
        target = self._make_target()
        self._init(target)
        manual = "# No marker manual brief\n"
        (target / "AGENTS.md").write_text(manual)
        append_brief_render_false(target / "ai-specs" / "ai-specs.toml")

        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((target / "AGENTS.md").read_text(), manual)

    def test_render_true_marker_still_preserves_file(self):
        target = self._make_target()
        self._init(target)
        hand_managed = (
            "# Hand-Managed\n"
            "<!-- ai-specs:runtime-brief -->\n\n"
            "Marker path still works.\n"
        )
        (target / "AGENTS.md").write_text(hand_managed)

        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((target / "AGENTS.md").read_text(), hand_managed)


class SubrepoRenderOptOutTests(unittest.TestCase):
    def _make_multi_workspace(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        workspace = Path(tmp.name) / "workspace"
        shutil.copytree(MULTI_TARGET_FIXTURE, workspace)
        (workspace / "packages" / "a").mkdir(parents=True, exist_ok=True)
        (workspace / "packages" / "b").mkdir(parents=True, exist_ok=True)
        return workspace

    def test_subrepo_skips_render_when_root_render_false(self):
        workspace = self._make_multi_workspace()
        subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
        toml = workspace / "ai-specs" / "ai-specs.toml"
        text = toml.read_text()
        text = re.sub(
            r"(?m)^subrepos\s*=\s*\[.*\]",
            'subrepos = ["packages/a"]',
            text,
            count=1,
        )
        toml.write_text(text)
        append_brief_render_false(toml)

        sub_agents = workspace / "packages" / "a" / "AGENTS.md"
        manual = "# Subrepo manual brief\n"
        sub_agents.write_text(manual)

        result = subprocess.run(
            [str(CLI), "sync", str(workspace)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sub_agents.read_text(), manual)
        self.assertTrue((workspace / "packages" / "a" / "ai-specs" / "skills").is_dir())

    def test_subrepo_missing_agents_md_errors_when_render_false(self):
        workspace = self._make_multi_workspace()
        subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
        toml = workspace / "ai-specs" / "ai-specs.toml"
        append_brief_render_false(toml)
        # Root AGENTS.md exists from init; subrepo packages/a has no AGENTS.md
        sub_agents = workspace / "packages" / "a" / "AGENTS.md"
        if sub_agents.exists():
            sub_agents.unlink()

        result = subprocess.run(
            [str(CLI), "sync", str(workspace)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(result.returncode, 0)
        combined = result.stdout + result.stderr
        self.assertIn("brief.render = false", combined)


if __name__ == "__main__":
    unittest.main()
